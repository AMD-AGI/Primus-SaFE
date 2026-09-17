/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Command safe-rfwd-mux is the pod-side half of an SSH reverse forward.
//
// The apiserver injects it into the target container and runs it under a single
// long-lived Kubernetes exec. It owns the forwarded listen socket and carries
// every connection made to that socket over its stdin and stdout, so a forward
// costs one exec however many connections travel through it.
//
// It is deliberately self-contained: no configuration file, no network calls of
// its own, and nothing to clean up afterwards - it removes its own image from the
// container as soon as the socket is bound.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers/rfwdmux"
)

const (
	// keepaliveInterval keeps the apiserver's idea of this session fresh, and is
	// what stops the idle timeout below from firing on a quiet but healthy forward.
	keepaliveInterval = 30 * time.Second
	// idleTimeout is how long this process waits for any frame before deciding the
	// apiserver is gone. It is the answer to the orphan the socat relay had none
	// for: an exec stream that is neither readable nor closed used to leave the pod
	// holding the listen port until the pod itself died.
	idleTimeout = 5 * time.Minute
	// statInterval is how often the counters go to the apiserver's log.
	statInterval = time.Minute
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: safe-rfwd-mux listen <addr> <port> | safe-rfwd-mux check")
	}
	switch os.Args[1] {
	case "check":
		// Running at all is the answer: it is how the installer tells a binary that
		// matches the container's architecture and sits on an executable filesystem
		// from one that does not.
		fmt.Println("safe-rfwd-mux ok")
	case "listen":
		if err := listen(os.Args[2:]); err != nil {
			fatal("%v", err)
		}
	default:
		fatal("unknown command %q", os.Args[1])
	}
}

// fatal reports a failure the way the apiserver reads it and stops.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", rfwdmux.ErrMarker, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// listen binds the forwarded port and carries what arrives on it.
func listen(args []string) error {
	fs := flag.NewFlagSet("listen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	maxStreams := fs.Int("max-streams", rfwdmux.DefaultMaxStreams,
		"how many connections this forward carries at once")
	removeDir := fs.String("remove-dir", "",
		"directory to delete once the socket is bound, so nothing is left in the container")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: safe-rfwd-mux listen [flags] <addr> <port>")
	}
	addr := fs.Arg(0)
	port, err := strconv.ParseUint(fs.Arg(1), 10, 16)
	if err != nil {
		return fmt.Errorf("listen port %q is not a port: %v", fs.Arg(1), err)
	}

	// Bind before anything else is announced: a forward that cannot have its port
	// must fail here, where the reason is still known, rather than as a connection
	// that quietly goes nowhere.
	ln, err := net.Listen("tcp4", net.JoinHostPort(addr, strconv.FormatUint(port, 10)))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %v", addr, port, err)
	}
	defer ln.Close()

	// The image is of no further use to anyone: the running process keeps its own
	// inode, so removing it now means no cleanup can be missed later, whatever ends
	// this process.
	removeSelf(*removeDir)

	session := rfwdmux.NewSession(
		rfwdmux.Join(os.Stdin, os.Stdout, os.Stdin, os.Stdout),
		rfwdmux.Config{
			MaxStreams:        *maxStreams,
			Initiator:         true,
			KeepaliveInterval: keepaliveInterval,
			IdleTimeout:       idleTimeout,
			Logf:              diagf,
		})

	// Closing the listener is what makes the accept loop below return, and it is
	// the moment the pod lets go of the port. Everything that ends this process
	// goes through here: the apiserver ending the exec's stdin, the session idling
	// out, or the container runtime signalling us.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-session.Done():
		case sig := <-signals:
			diagf("stopping on %s", sig)
			_ = session.Close()
		}
		_ = ln.Close()
	}()

	go reportStats(session)

	fmt.Fprintln(os.Stderr, rfwdmux.ReadyMarker)
	serve(ln, session)
	// The accept loop ending is the end of the forward, whatever ended it. Without
	// this a listener that stopped accepting would sit here with the port still
	// bound and the session still answering keepalives - a forward that looks
	// healthy from the apiserver and answers nothing, which is the failure this
	// whole design exists to remove.
	_ = session.Close()
	<-session.Done()
	return nil
}

// serve hands every accepted connection to a stream, or refuses it.
func serve(ln net.Listener, session *rfwdmux.Session) {
	var conns sync.WaitGroup
	defer conns.Wait()
	// A pod that has run out of file descriptors recovers; the forward should not
	// be over because one accept failed while it did.
	backoff := time.Duration(0)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if !isTemporary(err) {
				return
			}
			if backoff == 0 {
				backoff = 5 * time.Millisecond
			} else if backoff < time.Second {
				backoff *= 2
			}
			diagf("accept failed, retrying in %s: %v", backoff, err)
			select {
			case <-time.After(backoff):
			case <-session.Done():
				return
			}
			continue
		}
		backoff = 0
		stream, err := session.Open(originOf(conn.RemoteAddr()))
		if err != nil {
			if errors.Is(err, rfwdmux.ErrTooManyStreams) {
				// Refuse now rather than queue. A connection held in a queue nobody
				// is draining is what turned a busy proxy into a listener that
				// accepted and then never answered; a refused connection at least
				// fails where the caller can see it.
				session.CountDropped()
				diagf("refusing a connection from %s: already carrying %d",
					conn.RemoteAddr(), session.NumStreams())
			}
			reset(conn)
			continue
		}
		conns.Add(1)
		go func() {
			defer conns.Done()
			splice(conn, stream, session)
		}()
	}
}

// isTemporary reports whether an accept failure is worth retrying. A listener that
// has been closed, which is how this process is stopped, is not.
func isTemporary(err error) bool {
	if errors.Is(err, net.ErrClosed) {
		return false
	}
	// The errnos come first. Accept wraps every failure in a *net.OpError, which is
	// itself a net.Error, so a net.Error test placed above this one matches
	// everything and answers Timeout() - false for exactly the exhaustion errors
	// this is here to survive.
	for _, errno := range []syscall.Errno{
		syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS, syscall.ENOMEM,
		syscall.ECONNABORTED, syscall.EINTR,
	} {
		if errors.Is(err, errno) {
			return true
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// originOf splits a peer address into the parts a forwarded-tcpip channel names.
func originOf(addr net.Addr) (string, uint32) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok || tcpAddr.IP == nil {
		return "127.0.0.1", 0
	}
	return tcpAddr.IP.String(), uint32(tcpAddr.Port)
}

// reset refuses a connection abruptly. A lingerless close sends a TCP reset, so
// the process inside the pod gets an error on the spot instead of a connection
// that was accepted and will never be answered.
func reset(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetLinger(0)
	}
	_ = conn.Close()
}

// splice copies one accepted connection onto its stream in both directions.
func splice(conn net.Conn, stream *rfwdmux.Stream, session *rfwdmux.Session) {
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	// A session that ends must not leave connections waiting on it.
	go func() {
		select {
		case <-done:
		case <-session.Done():
		}
		_ = conn.Close()
		_ = stream.Close()
	}()

	// Each direction ends on its own. One side finishing is a half-close, which
	// only closes that direction; an error is a broken connection and takes the
	// pair down.
	var copying sync.WaitGroup
	copying.Add(2)
	go func() {
		defer copying.Done()
		if _, err := io.Copy(stream, conn); err != nil {
			finish()
		}
		_ = stream.CloseWrite()
	}()
	go func() {
		defer copying.Done()
		if _, err := io.Copy(conn, stream); err != nil {
			finish()
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}()
	copying.Wait()
	finish()
}

// reportStats gives the apiserver's log the two numbers that say whether a forward
// is healthy: what it is carrying, and what it had to turn away.
func reportStats(session *rfwdmux.Session) {
	ticker := time.NewTicker(statInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.Done():
			opened, open, dropped := session.Stats()
			fmt.Fprintf(os.Stderr, "%s streams=%d opened=%d dropped=%d stopped=%v\n",
				rfwdmux.StatMarker, open, opened, dropped, session.Err())
			return
		case <-ticker.C:
			opened, open, dropped := session.Stats()
			fmt.Fprintf(os.Stderr, "%s streams=%d opened=%d dropped=%d\n",
				rfwdmux.StatMarker, open, opened, dropped)
		}
	}
}

// removeSelf deletes this binary and the directory it was installed into. On Linux
// a running executable keeps its inode after the name is gone, so the process
// carries on with nothing left behind for a later session to trip over.
func removeSelf(dir string) {
	if dir == "" {
		return
	}
	if self, err := os.Executable(); err == nil && filepath.Dir(self) == filepath.Clean(dir) {
		_ = os.Remove(self)
	}
	if err := os.RemoveAll(dir); err != nil {
		diagf("could not remove %s: %v", dir, err)
	}
}

// diagf writes a diagnostic line, which reaches the apiserver's log unmarked.
func diagf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
