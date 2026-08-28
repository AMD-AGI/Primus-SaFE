/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// Markers exchanged with the relay script running inside the Pod. The acceptor
// script announces its state with these prefixes; every other line is diagnostics.
const (
	rfwdReadyMarker = "SAFE-RFWD-READY"
	rfwdConnMarker  = "SAFE-RFWD-CONN"
	rfwdErrMarker   = "SAFE-RFWD-ERR"
	// rfwdDropMarker reports that one connection could not be handed over, as
	// opposed to the listener itself failing. It deliberately does not start with
	// the connection marker, so neither prefix test can match the other.
	rfwdDropMarker = "SAFE-RFWD-DROP"
)

// childIDBaseEnv lets a test choose the id a child starts from, so two children can
// be made to want the same one. Nothing sets it in production, where the starting
// point is the child's own PID.
const childIDBaseEnv = "SAFE_RFWD_ID_BASE"

// relayMaxLineBytes caps a single line of relay output. socat can be verbose about
// a failure, and the scanner's default line is 64 KiB.
const relayMaxLineBytes = 1 << 20

// relayHalfCloseGrace is how long a relay keeps a half-closed connection open.
// socat's default is half a second, after which one direction ending closes the
// whole connection - which turns every half-close into a truncation, because the
// side still talking has barely started. The forward's own teardown is what really
// bounds these, so this only has to be longer than any exchange worth carrying.
const relayHalfCloseGrace = 3600

// listenerReadyTimeout bounds how long we wait for the Pod-side listener to bind.
// It shares a budget with forwardResolveTimeout: both run inside one global request,
// which is answered before the next one on the connection is looked at.
// It is a variable so a test can reach the branch where the pod never binds.
var listenerReadyTimeout = 15 * time.Second

// listenerShutdownGrace bounds how long Close waits for the Pod-side relay to exit
// and give the listen port back. It is a variable so tests do not wait it out.
var listenerShutdownGrace = 5 * time.Second

// podListener is a TCP listener living inside the target Pod's network namespace.
type podListener interface {
	// Accept returns the next connection made to the Pod-side listen socket.
	Accept(ctx context.Context) (podConn, error)
	// Close tears the Pod-side listener down.
	Close() error
}

// podConn is a byte stream bridged to one connection accepted inside the Pod.
type podConn interface {
	io.ReadWriteCloser
	// CloseWrite reports that nothing further will be sent to the Pod, leaving what
	// the Pod still has to say on its way.
	CloseWrite() error
	// OriginAddr is the Pod-side peer address that opened the connection.
	OriginAddr() string
	// OriginPort is the Pod-side peer port that opened the connection.
	OriginPort() uint32
}

// newPodExecutor builds the exec that runs one relay script in the target
// container. It is a variable so tests can drive the listener without a
// Kubernetes API server.
var newPodExecutor = func(l *execPodListener, script string, stdin bool) (remotecommand.Executor, error) {
	return l.newExecutor(script, stdin)
}

// podListenerFactory creates a Pod-side listener. It is injected into the forward
// manager so tests can drive the SSH side without a Kubernetes API server.
type podListenerFactory func(ctx context.Context, userInfo *UserInfo,
	clients *commonclient.ClientFactory, bindAddr string, bindPort uint32) (podListener, error)

// acceptedConn is one connection announcement emitted by the Pod-side acceptor.
type acceptedConn struct {
	id       string
	peerAddr string
	peerPort uint32
}

// execPodListener implements podListener by exec'ing a socat relay inside the Pod.
//
// A long-lived acceptor exec runs `socat TCP-LISTEN:<port>,bind=<addr>,fork`.
// For every accepted connection socat forks a child whose stdin/stdout are the
// accepted socket; the child announces itself on the exec's stderr and then
// re-publishes the socket as a unix socket under a private directory. The
// apiserver picks that connection up with a second, short-lived exec that pipes
// the unix socket to its own stdin/stdout.
type execPodListener struct {
	clients   *commonclient.ClientFactory
	userInfo  *UserInfo
	container string
	dir       string
	bindAddr  string
	bindPort  uint32

	accepted chan acceptedConn
	cancel   context.CancelFunc
	// stdinW is never written to. Closing it is how the acceptor script inside the
	// Pod is told the session has ended.
	stdinW *io.PipeWriter

	closeOnce sync.Once
	doneCh    chan struct{}
	// streamDone closes when the acceptor exec has ended, which is the moment the
	// Pod-side relay has let go of the listen port.
	streamDone chan struct{}

	mu  sync.Mutex
	err error
}

// newExecPodListener starts the acceptor script in the Pod and waits for it to bind.
func newExecPodListener(ctx context.Context, userInfo *UserInfo,
	clients *commonclient.ClientFactory, bindAddr string, bindPort uint32) (podListener, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	l := &execPodListener{
		clients:    clients,
		userInfo:   userInfo,
		container:  userInfo.Container,
		dir:        "/tmp/.safe-rfwd-" + token,
		bindAddr:   bindAddr,
		bindPort:   bindPort,
		accepted:   make(chan acceptedConn, 16),
		cancel:     cancel,
		doneCh:     make(chan struct{}),
		streamDone: make(chan struct{}),
	}

	// The acceptor reads stdin only to learn when it ends, so it needs a stdin
	// stream even though nothing is ever sent on it.
	// The script gives up a little before we do, so its account of what went wrong
	// is the one that reaches the caller.
	readySeconds := int(listenerReadyTimeout.Seconds()) - 3
	if readySeconds < 1 {
		readySeconds = 1
	}
	executor, err := newPodExecutor(l, acceptorScript(l.dir, bindAddr, bindPort, readySeconds), true)
	if err != nil {
		cancel()
		return nil, err
	}

	readyCh := make(chan error, 1)
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	l.stdinW = stdinW

	go l.scan(stdoutR, readyCh)
	go l.scan(stderrR, readyCh)
	go func() {
		defer close(l.streamDone)
		streamErr := executor.StreamWithContext(runCtx, remotecommand.StreamOptions{
			Stdin:  stdinR,
			Stdout: stdoutW,
			Stderr: stderrW,
		})
		if streamErr == nil {
			streamErr = fmt.Errorf("pod listener on %s:%d exited", bindAddr, bindPort)
		}
		l.fail(streamErr)
		_ = stdinR.CloseWithError(streamErr)
		_ = stdoutW.CloseWithError(streamErr)
		_ = stderrW.CloseWithError(streamErr)
		select {
		case readyCh <- streamErr:
		default:
		}
	}()

	select {
	case err = <-readyCh:
		if err != nil {
			_ = l.Close()
			return nil, err
		}
	case <-time.After(listenerReadyTimeout):
		_ = l.Close()
		return nil, fmt.Errorf("timed out waiting for pod listener on %s:%d", bindAddr, bindPort)
	case <-runCtx.Done():
		_ = l.Close()
		return nil, runCtx.Err()
	}

	klog.Infof("reverse forward listener ready in pod %s/%s on %s:%d",
		userInfo.Namespace, userInfo.Pod, bindAddr, bindPort)
	return l, nil
}

// scan consumes one relay output stream, turning marker lines into events.
func (l *execPodListener) scan(r io.Reader, readyCh chan<- error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), relayMaxLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == rfwdReadyMarker:
			select {
			case readyCh <- nil:
			default:
			}
		case strings.HasPrefix(line, rfwdErrMarker):
			err := fmt.Errorf("pod listener failed:%s", strings.TrimPrefix(line, rfwdErrMarker))
			l.fail(err)
			select {
			case readyCh <- err:
			default:
			}
		case strings.HasPrefix(line, rfwdDropMarker):
			// One connection lost, not the listener: the pod-side application sees
			// its connection close, and every other one carries on.
			klog.Warningf("pod %s dropped a reverse forward connection:%s",
				l.userInfo.Pod, strings.TrimPrefix(line, rfwdDropMarker))
		case strings.HasPrefix(line, rfwdConnMarker):
			conn, ok := parseConnAnnouncement(line)
			if !ok {
				klog.Warningf("ignoring malformed reverse forward announcement: %q", line)
				continue
			}
			select {
			case l.accepted <- conn:
			case <-l.doneCh:
				return
			}
		default:
			if line != "" {
				klog.V(4).Infof("pod %s reverse forward relay: %s", l.userInfo.Pod, line)
			}
		}
	}
	// Reaching here without an error is the stream ending, which the exec goroutine
	// already reports. With one, the relay has stopped being readable while the
	// listener still looks alive - say so, or Accept waits for announcements that
	// are never coming and nothing in the log explains it.
	if err := scanner.Err(); err != nil {
		l.fail(fmt.Errorf("pod listener output could not be read: %v", err))
	}
}

// Accept waits for the Pod-side acceptor to report a connection, then opens a
// byte stream carrying it.
func (l *execPodListener) Accept(ctx context.Context) (podConn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-l.doneCh:
			return nil, l.closeErr()
		case conn := <-l.accepted:
			pc, err := l.dial(ctx, conn)
			if err != nil {
				// One connection failing to hand off must not kill the listener.
				klog.ErrorS(err, "failed to attach to pod reverse forward connection",
					"pod", l.userInfo.Pod, "id", conn.id)
				continue
			}
			return pc, nil
		}
	}
}

// dial attaches to the unix socket the announced socat child is listening on.
func (l *execPodListener) dial(ctx context.Context, conn acceptedConn) (podConn, error) {
	executor, err := newPodExecutor(l, connectScript(l.dir, conn.id), true)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	c := &execPodConn{
		stdinW:     stdinW,
		stdoutR:    stdoutR,
		cancel:     cancel,
		originAddr: conn.peerAddr,
		originPort: conn.peerPort,
	}

	go func() {
		streamErr := executor.StreamWithContext(runCtx, remotecommand.StreamOptions{
			Stdin:  stdinR,
			Stdout: stdoutW,
			Stderr: newLogWriter(l.userInfo.Pod),
		})
		if streamErr == nil {
			streamErr = io.EOF
		}
		_ = stdoutW.CloseWithError(streamErr)
		_ = stdinR.CloseWithError(streamErr)
		cancel()
	}()
	return c, nil
}

// execRequest builds the exec request that runs one relay script in the container.
// It is separate from newExecutor so a test can read back what we ask the API server
// for without needing an API server to ask.
func (l *execPodListener) execRequest(script string, stdin bool) *rest.Request {
	return l.clients.ClientSet().CoreV1().RESTClient().Post().
		Resource("pods").
		Name(l.userInfo.Pod).
		Namespace(l.userInfo.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: l.container,
			Command:   []string{"/bin/sh", "-c", script},
			Stdin:     stdin,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
}

// newExecutor builds an exec request running the given shell script in the container.
func (l *execPodListener) newExecutor(script string, stdin bool) (remotecommand.Executor, error) {
	return remotecommand.NewSPDYExecutor(l.clients.RestConfig(), "POST", l.execRequest(script, stdin).URL())
}

// Close stops the acceptor exec; the script's trap removes the Pod-side sockets.
func (l *execPodListener) Close() error {
	l.fail(fmt.Errorf("pod listener on %s:%d closed", l.bindAddr, l.bindPort))
	// Ending stdin is the script's shutdown signal, and it is the only one that
	// reaches a runtime that leaves the exec'd process running after the stream is
	// torn down. Closing it also releases the stream's stdin copier.
	if l.stdinW != nil {
		_ = l.stdinW.Close()
	}
	// Wait for the relay to actually exit before reporting the listener gone. A
	// client that reconnects asks for the same port straight away, and reuseaddr
	// does not cover a socket another live process is still listening on - so
	// returning early turns a reconnect into "the port you just released is busy".
	// Bounded, because a stuck relay must not hold up the rest of the teardown.
	select {
	case <-l.streamDone:
	case <-time.After(listenerShutdownGrace):
		klog.Warningf("pod listener on %s:%d did not exit within %s", l.bindAddr, l.bindPort, listenerShutdownGrace)
	}
	l.cancel()
	return nil
}

// fail records the first terminal error and releases everyone blocked on the listener.
func (l *execPodListener) fail(err error) {
	l.mu.Lock()
	if l.err == nil {
		l.err = err
	}
	l.mu.Unlock()
	l.closeOnce.Do(func() { close(l.doneCh) })
}

// closeErr reports why the listener stopped.
func (l *execPodListener) closeErr() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	return io.EOF
}

// execPodConn bridges one Pod-side connection over an exec stream.
type execPodConn struct {
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	cancel  context.CancelFunc

	originAddr string
	originPort uint32

	closeOnce sync.Once
}

func (c *execPodConn) Read(p []byte) (int, error) { return c.stdoutR.Read(p) }

// CloseWrite ends the relay's stdin, which is how the Pod-side socket learns the
// other end has finished writing, without disturbing what it is still sending back.
func (c *execPodConn) CloseWrite() error { return c.stdinW.Close() }

func (c *execPodConn) Write(p []byte) (int, error) { return c.stdinW.Write(p) }
func (c *execPodConn) OriginAddr() string          { return c.originAddr }
func (c *execPodConn) OriginPort() uint32          { return c.originPort }

// Close tears down the exec stream carrying this connection.
func (c *execPodConn) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.stdinW.Close()
		_ = c.stdoutR.Close()
	})
	return nil
}

// parseConnAnnouncement parses "SAFE-RFWD-CONN <id> <peer-addr> <peer-port>".
func parseConnAnnouncement(line string) (acceptedConn, bool) {
	fields := strings.Fields(line)
	if len(fields) != 4 || fields[0] != rfwdConnMarker {
		return acceptedConn{}, false
	}
	// The id becomes a path component of the rendezvous socket, so it must not be
	// able to escape the private directory.
	id := fields[1]
	if !isRendezvousID(id) {
		return acceptedConn{}, false
	}
	port, err := strconv.ParseUint(fields[3], 10, 16)
	if err != nil {
		return acceptedConn{}, false
	}
	addr := fields[2]
	if addr == "" {
		addr = "127.0.0.1"
	}
	return acceptedConn{id: id, peerAddr: addr, peerPort: uint32(port)}, true
}

// isRendezvousID reports whether id is safe to interpolate into a socket path.
func isRendezvousID(id string) bool {
	if id == "" || len(id) > 32 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// acceptorScript builds the long-lived relay script that owns the Pod listen socket.
//
// bindAddr is always one of the configured literal addresses and bindPort has
// already been range-checked, so neither can inject shell syntax here.
//
// The per-connection work lives in a script file rather than inline in the SYSTEM:
// address. socat parses its address strings itself - splitting on commas to find
// options and consuming quotes - before handing the command to a shell, which
// silently mangles anything more involved than a plain command. Keeping the
// SYSTEM: command down to a single `sh <dir>/child.sh` leaves nothing to mangle.
func acceptorScript(dir, bindAddr string, bindPort uint32, readySeconds int) string {
	script := fmt.Sprintf(`set -e
for tool in socat awk date grep mkdir; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "%[4]s $tool is not installed in the container" >&2
    exit 127
  fi
done
if [ ! -r /proc/net/tcp ]; then
  echo "%[4]s /proc/net/tcp is not readable, cannot confirm the listen port" >&2
  exit 1
fi
D=%[1]s
rm -rf "$D"
# -m rather than a following chmod: otherwise the directory exists, briefly, with
# whatever the image's umask allows.
mkdir -m 700 -p "$D"
cat > "$D/child.sh" <<'SAFE_RFWD_CHILD_EOF'
CHILD_SCRIPT_PLACEHOLDER
SAFE_RFWD_CHILD_EOF
socat -t %[7]d TCP-LISTEN:%[3]d,bind=%[2]s,reuseaddr,fork SYSTEM:'sh %[1]s/child.sh' &
SPID=$!
# The exec stream going away is how a session ends, and it shows up here as EOF on
# stdin. A background job's stdin is /dev/null unless it is handed our own, so dup
# it first - otherwise this watcher fires immediately.
exec 3<&0
( cat <&3 >/dev/null 2>&1; kill "$SPID" 2>/dev/null ) >/dev/null 2>&1 &
WPID=$!
# If the runtime kills this script outright, the trap never runs. A detached
# watcher reaps the listener once the script that owns it is gone, so the pod is
# not left with a bound port after the SSH session ends.
MPID=$$
( while [ -e "/proc/$MPID" ] && [ -e "/proc/$SPID" ]; do sleep 2; done
  kill "$SPID" "$WPID" 2>/dev/null || true
  rm -rf "$D" ) >/dev/null 2>&1 &
# set -e is still in force inside the trap, and by the time it runs the processes
# it kills have usually exited already - without the || true, that failed kill would
# end the trap before the rendezvous directory is removed.
trap 'kill "$SPID" "$WPID" 2>/dev/null || true; rm -rf "$D"' EXIT INT TERM
# Ready has to mean the port is bound, not that socat has not died yet: a fixed wait
# is both too long on an idle node and too short on a busy one, and a bind that fails
# slowly would be reported as success. Ask the kernel instead - state 0A is LISTEN.
#
# A listening port alone is not the answer either, because something else may already
# hold it - which is precisely the case where our socat is about to die. Match the
# listening socket's inode against socat's own descriptors, so what we report is that
# this relay bound the port, not that somebody did.
HEXPORT=$(printf '%%04X' %[3]d)
# Give up before the apiserver does, so the reason below is what the user is told
# rather than a generic "timed out waiting for pod listener" - and so a bind that
# lands late is still ours to report rather than something already abandoned.
DEADLINE=$(( $(date +%%s) + %[6]d ))
READY=
while [ -z "$READY" ]; do
  INODE=$(awk -v p=":$HEXPORT" '$2 ~ (p "$") && $4 == "0A" { print $10; exit }' /proc/net/tcp 2>/dev/null)
  if [ -n "$INODE" ]; then
    for FD in /proc/$SPID/fd/*; do
      if [ "$(readlink "$FD" 2>/dev/null)" = "socket:[$INODE]" ]; then
        READY=1
        break
      fi
    done
  fi
  if [ -n "$READY" ]; then
    break
  fi
  # A socat that lost the bind is an unreaped zombie here, and a zombie answers
  # kill -0, so ask /proc whether it is actually still running.
  if [ ! -r "/proc/$SPID/status" ] || grep -qi '^State:[[:space:]]*Z' "/proc/$SPID/status"; then
    echo "%[4]s failed to listen on %[2]s:%[3]d" >&2
    exit 1
  fi
  if [ "$(date +%%s)" -ge "$DEADLINE" ]; then
    echo "%[4]s timed out waiting for %[2]s:%[3]d to be listening" >&2
    exit 1
  fi
  sleep 0.1 2>/dev/null || sleep 1
done
echo "%[5]s"
wait "$SPID"
`, dir, bindAddr, bindPort, rfwdErrMarker, rfwdReadyMarker, readySeconds, relayHalfCloseGrace)
	return strings.Replace(script, "CHILD_SCRIPT_PLACEHOLDER", childScript(dir), 1)
}

// childScript is what socat runs for each accepted connection, with the accepted
// socket as its stdin and stdout. It republishes that socket as a unix socket the
// apiserver can attach to and only then announces itself, because socat fails a
// UNIX-CONNECT to a socket that does not exist yet outright instead of retrying.
func childScript(dir string) string {
	return fmt.Sprintf(`D=%[1]s
# Claim an id no live sibling holds. This used to be the PID alone, which is unique
# among running processes but comes back once one exits - and a returning id would
# have this child remove and re-create a path an earlier connection is still
# announced under, handing one connection's bytes to the other's channel. mkdir
# either creates the directory or fails, so exactly one child can own an id.
N=${%[3]s:-$$}
c=0
while ! mkdir "$D/$N" 2>/dev/null; do
  N=$((N+1))
  c=$((c+1))
  # Bounded, because mkdir can also be failing for a reason moving on will never
  # fix - the rendezvous directory is gone because the forward ended, or there is
  # no mkdir to run - and an unbounded retry would spin on a core forever.
  if [ $c -ge 100 ]; then
    echo "%[4]s could not claim a rendezvous id under $D" >&2
    exit 1
  fi
done
S=$D/$N/s
# The accepted socket arrives as this script's stdin and stdout, but a shell gives a
# background job /dev/null for stdin. Without this dup socat would relay an
# immediate EOF and nothing from the pod would ever reach the client.
exec 3<&0
socat -t %[5]d - UNIX-LISTEN:"$S" <&3 &
P=$!
# Wait on the clock, not the CPU. A spin here is both useless - socat cannot fork
# and bind inside a few hundred shell iterations - and harmful, because this pod may
# have a CPU limit and the spin would spend it starving the socat being waited for.
i=0
while [ ! -S "$S" ] && [ $i -lt 100 ]; do
  sleep 0.1 2>/dev/null || sleep 1
  i=$((i+1))
done
# Announce only what the apiserver can actually attach to. Falling out of the spin
# without a socket means socat never got it published, and announcing anyway would
# send the apiserver to a path that is never going to exist.
if [ ! -S "$S" ]; then
  echo "%[4]s the relay socket for $N never appeared" >&2
  kill "$P" 2>/dev/null || true
  rm -f "$S"
  exit 1
fi
echo "%[2]s $N ${SOCAT_PEERADDR:-127.0.0.1} ${SOCAT_PEERPORT:-0}" >&2
# A connection the apiserver never attaches to would otherwise hold this pair open
# for as long as the pod lives; the rendezvous directory disappearing is how the
# forward reports that it has ended.
( while [ -d "$D" ] && [ -e "/proc/$P" ]; do sleep 2; done
  kill "$P" 2>/dev/null ) >/dev/null 2>&1 &
wait "$P"
# Remove the socket but keep the directory: it is this connection's claim on the id,
# and releasing it would let a later child take the same one. An announcement still
# queued for the first connection would then be attached to the second.
rm -f "$S"`, dir, rfwdConnMarker, childIDBaseEnv, rfwdDropMarker, relayHalfCloseGrace)
}

// connectScript builds the short-lived script that hands one accepted connection
// to the apiserver over the exec stream.
func connectScript(dir, id string) string {
	// Each child owns a directory named by its id, with the socket inside it.
	return fmt.Sprintf("exec socat -t %d - UNIX-CONNECT:%s/%s/s,retry=100,interval=0.1",
		relayHalfCloseGrace, dir, id)
}

// randomToken returns a hex token used to name the Pod-side rendezvous directory.
func randomToken() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate reverse forward token: %v", err)
	}
	return hex.EncodeToString(buf), nil
}

// logWriter forwards relay stderr to the apiserver log.
type logWriter struct{ pod string }

func newLogWriter(pod string) io.Writer { return &logWriter{pod: pod} }

func (w *logWriter) Write(p []byte) (int, error) {
	if msg := strings.TrimSpace(string(p)); msg != "" {
		klog.V(4).Infof("pod %s reverse forward relay: %s", w.pod, msg)
	}
	return len(p), nil
}
