/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if os.Getenv("SAFE_RFWD_REAP_HELPER") == "1" {
		os.Exit(reapHelper())
	}
	os.Exit(m.Run())
}

// reapHelper is a stand-in for the pod-side multiplexer: it listens, and exits
// when signaled. The test copies this binary to a path whose base name is mux
// so the command line matches what ReapStaleListener is willing to signal.
func reapHelper() int {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: mux listen <addr> <port>")
		return 2
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort(os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println("ready")
	_ = os.Stdout.Sync()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig
	_ = ln.Close()
	return 0
}

func TestReapStaleListenerSignalsTheMuxHoldingThePort(t *testing.T) {
	port := freePort(t)
	cmd := startListener(t, "mux", "127.0.0.1", port)
	signaled, err := ReapStaleListener("127.0.0.1", uint16(port))
	testifyassert.NoError(t, err)
	testifyassert.True(t, signaled)
	waitCmd(t, cmd)
}

func TestReapStaleListenerLeavesADifferentCommand(t *testing.T) {
	port := freePort(t)
	cmd := startListener(t, "other", "127.0.0.1", port)
	signaled, err := ReapStaleListener("127.0.0.1", uint16(port))
	testifyassert.NoError(t, err)
	testifyassert.False(t, signaled)
	testifyassert.NoError(t, cmd.Process.Signal(syscall.Signal(0)))
}

func TestReapStaleListenerWhenNothingIsListening(t *testing.T) {
	port := freePort(t)
	signaled, err := ReapStaleListener("127.0.0.1", uint16(port))
	testifyassert.NoError(t, err)
	testifyassert.False(t, signaled)
}

func TestIsMuxListen(t *testing.T) {
	args := []string{"/tmp/.safe-rfwd-abcd/mux", "listen", "-max-streams", "256", "127.0.0.1", "7890"}
	testifyassert.True(t, isMuxListen(args, "127.0.0.1", 7890))
	testifyassert.False(t, isMuxListen(args, "127.0.0.1", 7891))
	testifyassert.False(t, isMuxListen([]string{"/tmp/notmux", "listen", "127.0.0.1", "7890"}, "127.0.0.1", 7890))
	testifyassert.False(t, isMuxListen([]string{"/tmp/mux", "check"}, "127.0.0.1", 7890))
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	testifyassert.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	testifyassert.NoError(t, ln.Close())
	return port
}

func startListener(t *testing.T, name, addr string, port int) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	testifyassert.NoError(t, err)
	bin := filepath.Join(t.TempDir(), name)
	copyFile(t, exe, bin)
	cmd := exec.Command(bin, "listen", addr, strconv.Itoa(port))
	cmd.Env = append(os.Environ(), "SAFE_RFWD_REAP_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	testifyassert.NoError(t, err)
	cmd.Stderr = os.Stderr
	testifyassert.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	waitReady(t, stdout)
	return cmd
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	testifyassert.NoError(t, err)
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	testifyassert.NoError(t, err)
	_, err = io.Copy(dst, src)
	testifyassert.NoError(t, dst.Close())
	testifyassert.NoError(t, err)
}

func waitReady(t *testing.T, r io.Reader) {
	t.Helper()
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		if scanner.Scan() {
			line <- scanner.Text()
		} else {
			line <- ""
		}
	}()
	select {
	case got := <-line:
		testifyassert.Equal(t, "ready", got)
	case <-time.After(20 * time.Second):
		t.Fatal("listener did not become ready")
	}
}

func waitCmd(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		testifyassert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("signaled listener did not exit")
	}
}
