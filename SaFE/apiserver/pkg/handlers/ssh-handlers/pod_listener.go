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

// listenerReadyTimeout bounds how long we wait for the Pod-side listener to bind.
const listenerReadyTimeout = 30 * time.Second

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
		clients:   clients,
		userInfo:  userInfo,
		container: userInfo.Container,
		dir:       "/tmp/.safe-rfwd-" + token,
		bindAddr:  bindAddr,
		bindPort:  bindPort,
		accepted:  make(chan acceptedConn, 16),
		cancel:    cancel,
		doneCh:    make(chan struct{}),
	}

	// The acceptor reads stdin only to learn when it ends, so it needs a stdin
	// stream even though nothing is ever sent on it.
	executor, err := newPodExecutor(l, acceptorScript(l.dir, bindAddr, bindPort), true)
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

func (c *execPodConn) Read(p []byte) (int, error)  { return c.stdoutR.Read(p) }
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
func acceptorScript(dir, bindAddr string, bindPort uint32) string {
	return fmt.Sprintf(`set -e
if ! command -v socat >/dev/null 2>&1; then
  echo "%[4]s socat is not installed in the container" >&2
  exit 127
fi
D=%[1]s
rm -rf "$D"
mkdir -p "$D"
chmod 700 "$D"
cat > "$D/child.sh" <<'SAFE_RFWD_CHILD_EOF'
%[6]s
SAFE_RFWD_CHILD_EOF
socat TCP-LISTEN:%[3]d,bind=%[2]s,reuseaddr,fork SYSTEM:'sh %[1]s/child.sh' &
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
sleep 1
# A socat that died on a bind error is still an unreaped zombie here, and a zombie
# answers kill -0, so ask /proc whether it is actually running.
if [ -r "/proc/$SPID/status" ] && ! grep -qi '^State:[[:space:]]*Z' "/proc/$SPID/status"; then
  echo "%[5]s"
else
  echo "%[4]s failed to listen on %[2]s:%[3]d" >&2
  exit 1
fi
wait "$SPID"
`, dir, bindAddr, bindPort, rfwdErrMarker, rfwdReadyMarker, childScript(dir))
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
socat - UNIX-LISTEN:"$S" <&3 &
P=$!
# Spin rather than sleep: the socket appears within microseconds, and a minimal pod
# image is not guaranteed a sleep that accepts sub-second intervals.
i=0
while [ ! -S "$S" ] && [ $i -lt 100000 ]; do
  i=$((i+1))
done
# Announce only what the apiserver can actually attach to. Falling out of the spin
# without a socket means socat never got it published, and announcing anyway would
# send the apiserver to a path that is never going to exist.
if [ ! -S "$S" ]; then
  echo "%[4]s the relay socket for $N never appeared" >&2
  kill "$P" 2>/dev/null || true
  rm -rf "$D/$N"
  exit 1
fi
echo "%[2]s $N ${SOCAT_PEERADDR:-127.0.0.1} ${SOCAT_PEERPORT:-0}" >&2
# A connection the apiserver never attaches to would otherwise hold this pair open
# for as long as the pod lives; the rendezvous directory disappearing is how the
# forward reports that it has ended.
( while [ -d "$D" ] && [ -e "/proc/$P" ]; do sleep 2; done
  kill "$P" 2>/dev/null ) >/dev/null 2>&1 &
wait "$P"
rm -rf "$D/$N"`, dir, rfwdConnMarker, childIDBaseEnv, rfwdDropMarker)
}

// connectScript builds the short-lived script that hands one accepted connection
// to the apiserver over the exec stream.
func connectScript(dir, id string) string {
	// Each child owns a directory named by its id, with the socket inside it.
	return fmt.Sprintf("exec socat - UNIX-CONNECT:%s/%s/s,retry=100,interval=0.1", dir, id)
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
