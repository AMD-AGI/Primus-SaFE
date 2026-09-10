/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers/muxbin"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers/rfwdmux"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// Markers exchanged with the multiplexer running inside the Pod. Every other line
// on its stderr is a diagnostic.
const (
	rfwdReadyMarker = rfwdmux.ReadyMarker
	rfwdErrMarker   = rfwdmux.ErrMarker
	rfwdStatMarker  = rfwdmux.StatMarker
)

// relayMaxLineBytes caps a single line of Pod-side output, whose default scanner
// line is 64 KiB.
const relayMaxLineBytes = 1 << 20

// muxMaxStreams bounds the connections one forward carries at once. Past it the
// Pod side refuses the connection rather than queueing it: the failure the
// multiplexer replaces was a listener that accepted connections it had no capacity
// to serve and left them waiting for the life of the session.
const muxMaxStreams = rfwdmux.DefaultMaxStreams

// muxKeepaliveInterval feeds the Pod side's own idle timer. Without traffic in
// either direction the multiplexer decides the apiserver is gone and gives the
// listen port back, so a healthy but quiet forward has to say so.
const muxKeepaliveInterval = 30 * time.Second

// muxIdleTimeout ends a forward whose pod side has stopped answering. Both ends run
// the same timer against the same keepalive, so a wedged exec stream is reported as
// a listener that stopped - which the client can act on by asking again - rather
// than as a forward that is up and silently carries nothing. That silence is the
// shape the failure this replaces took.
const muxIdleTimeout = 5 * time.Minute

// installTimeout bounds the two short execs that put the multiplexer into the
// container. It shares a budget with forwardResolveTimeout and
// listenerReadyTimeout: all three run inside one SSH global request, which is
// answered before the next request on the connection is looked at, and their sum
// has to stay inside the roughly three minutes a client tolerates before it gives
// up on the connection. It is a variable so tests do not wait it out.
var installTimeout = 60 * time.Second

// listenerReadyTimeout bounds how long we wait for the Pod-side listener to bind.
// It is a variable so a test can reach the branch where the pod never binds.
var listenerReadyTimeout = 15 * time.Second

// listenerShutdownGrace bounds how long Close waits for the Pod-side multiplexer
// to exit and give the listen port back. It is a variable so tests do not wait it out.
var listenerShutdownGrace = 5 * time.Second

// installDirs are the directories the Pod side may install the multiplexer into,
// in the order it tries them. A container whose /tmp is mounted noexec still has
// somewhere to run from, and restricting the set means the path the Pod reports
// back cannot become a path we did not choose.
var installDirs = []string{"/tmp", "/dev/shm", "/var/tmp"}

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

// muxBinaryFor looks up the embedded multiplexer for a container's architecture. It
// is a variable so tests can drive the listener without the image build having
// produced the binaries, which a source checkout has not.
var muxBinaryFor = muxbin.For

// newPodExecutor builds the exec that runs one command in the target container. It
// is a variable so tests can drive the listener without a Kubernetes API server.
var newPodExecutor = func(l *execPodListener, script string, stdin bool) (remotecommand.Executor, error) {
	return l.newExecutor(script, stdin)
}

// podListenerFactory creates a Pod-side listener. It is injected into the forward
// manager so tests can drive the SSH side without a Kubernetes API server.
type podListenerFactory func(ctx context.Context, userInfo *UserInfo,
	clients *commonclient.ClientFactory, bindAddr string, bindPort uint32) (podListener, error)

// execPodListener implements podListener with one long-lived exec per forward.
//
// A short exec reports the container's architecture and a directory it can execute
// from; a second writes the matching multiplexer there; a third runs it, with the
// exec's stdin and stdout carrying every forwarded connection as a multiplexed
// stream. One forward therefore costs one exec no matter how much traffic goes
// through it, which is the whole point: the previous design took an exec per
// connection, ran out of them, and left a listener that accepted and never
// answered.
type execPodListener struct {
	clients   *commonclient.ClientFactory
	userInfo  *UserInfo
	container string
	dir       string
	bindAddr  string
	bindPort  uint32

	session *rfwdmux.Session
	cancel  context.CancelFunc
	// stdinW is the write half of the multiplexed session, and closing it is how
	// the Pod side is told the forward has ended.
	stdinW *io.PipeWriter

	closeOnce sync.Once
	doneCh    chan struct{}
	// streamDone closes when the run exec has ended, which is the moment the Pod
	// has let go of the listen port.
	streamDone chan struct{}

	mu  sync.Mutex
	err error
}

// newExecPodListener installs the multiplexer in the Pod and starts it.
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
		bindAddr:   bindAddr,
		bindPort:   bindPort,
		cancel:     cancel,
		doneCh:     make(chan struct{}),
		streamDone: make(chan struct{}),
	}

	if err = l.install(runCtx, token); err != nil {
		cancel()
		return nil, err
	}
	if err = l.run(runCtx); err != nil {
		_ = l.Close()
		// The run script's trap and the multiplexer's own cleanup both need the
		// multiplexer to have started. This is the path where neither did, and
		// nothing else in the pod would ever clear the install away.
		l.removeInstall()
		return nil, err
	}

	klog.Infof("reverse forward listener ready in pod %s/%s on %s:%d",
		userInfo.Namespace, userInfo.Pod, bindAddr, bindPort)
	return l, nil
}

// install puts the multiplexer into the container and records where it went.
func (l *execPodListener) install(ctx context.Context, token string) error {
	installCtx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()

	stdout, stderr, err := l.runSetup(installCtx, probeScript(token), nil)
	if err != nil {
		return fmt.Errorf("failed to inspect pod %s: %v%s", l.userInfo.Pod, err, reportedReason(stderr))
	}
	machine, base, err := parseProbe(stdout)
	if err != nil {
		return fmt.Errorf("failed to inspect pod %s: %v%s", l.userInfo.Pod, err, reportedReason(stderr))
	}

	binary, arch, err := muxBinaryFor(machine)
	if err != nil {
		return err
	}
	l.dir = base + "/.safe-rfwd-" + token

	if _, stderr, err = l.runSetup(installCtx, installScript(l.dir), bytes.NewReader(binary)); err != nil {
		return fmt.Errorf("failed to install the %s reverse forward multiplexer in pod %s: %v%s",
			arch, l.userInfo.Pod, err, reportedReason(stderr))
	}
	klog.V(2).Infof("installed the %s reverse forward multiplexer in pod %s/%s at %s",
		arch, l.userInfo.Namespace, l.userInfo.Pod, l.dir)
	return nil
}

// run starts the multiplexer and waits for it to report the port bound.
func (l *execPodListener) run(ctx context.Context) error {
	executor, err := newPodExecutor(l, runScript(l.dir, l.bindAddr, l.bindPort), true)
	if err != nil {
		// Nothing was started, so nothing will close this - and Close waits on it.
		close(l.streamDone)
		return err
	}

	readyCh := make(chan error, 1)
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	l.stdinW = stdinW

	go l.scan(stderrR, readyCh)
	go func() {
		defer close(l.streamDone)
		streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  stdinR,
			Stdout: stdoutW,
			Stderr: stderrW,
		})
		if streamErr == nil {
			streamErr = fmt.Errorf("pod listener on %s:%d exited", l.bindAddr, l.bindPort)
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

	// The session is started before the port is known to be bound, because the
	// multiplexer speaks the moment it is up and nothing must be read late.
	l.session = rfwdmux.NewSession(rfwdmux.Join(stdoutR, stdinW, stdoutR, stdinW), rfwdmux.Config{
		MaxStreams:        muxMaxStreams,
		KeepaliveInterval: muxKeepaliveInterval,
		IdleTimeout:       muxIdleTimeout,
		Logf: func(format string, args ...any) {
			klog.V(4).Infof("pod %s reverse forward: %s", l.userInfo.Pod, fmt.Sprintf(format, args...))
		},
	})
	go func() {
		<-l.session.Done()
		l.fail(l.session.Err())
	}()

	select {
	case err = <-readyCh:
		if err != nil {
			return err
		}
	case <-time.After(listenerReadyTimeout):
		return fmt.Errorf("timed out waiting for pod listener on %s:%d", l.bindAddr, l.bindPort)
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// removeInstall deletes the install directory from the container. It runs on its own
// context: the forward's is cancelled by the time this is wanted, and leaving a
// binary in a user's pod is worse than one more short exec.
func (l *execPodListener) removeInstall() {
	if l.dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	if _, _, err := l.runSetup(ctx, "rm -rf "+l.dir, nil); err != nil {
		klog.Warningf("could not remove %s from pod %s/%s: %v",
			l.dir, l.userInfo.Namespace, l.userInfo.Pod, err)
	}
}

// runSetup runs one short-lived command in the container and collects its output.
func (l *execPodListener) runSetup(ctx context.Context, script string, stdin io.Reader) (string, string, error) {
	executor, err := newPodExecutor(l, script, stdin != nil)
	if err != nil {
		return "", "", err
	}
	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: &limitedWriter{w: &stdout, left: relayMaxLineBytes},
		Stderr: &limitedWriter{w: &stderr, left: relayMaxLineBytes},
	})
	return stdout.String(), stderr.String(), err
}

// scan consumes the multiplexer's stderr, turning marker lines into events.
func (l *execPodListener) scan(r *io.PipeReader, readyCh chan<- error) {
	// Whatever ends this loop, the stream is still writing into the other end of
	// this pipe. Leaving it there strands the copier mid-write, so the exec never
	// returns, the listener never reports itself finished, and Close waits out its
	// whole grace before giving up - delaying the very port release it is there for.
	defer r.CloseWithError(io.EOF)
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
		case strings.HasPrefix(line, rfwdStatMarker):
			klog.V(2).Infof("pod %s reverse forward on %s:%d:%s", l.userInfo.Pod,
				l.bindAddr, l.bindPort, strings.TrimPrefix(line, rfwdStatMarker))
		default:
			if line != "" {
				klog.V(4).Infof("pod %s reverse forward: %s", l.userInfo.Pod, line)
			}
		}
	}
	// Reaching here without an error is the stream ending, which the exec goroutine
	// already reports. With one, the multiplexer has stopped being readable while
	// the listener still looks alive - say so, or Accept waits for connections that
	// are never coming and nothing in the log explains it.
	if err := scanner.Err(); err != nil {
		l.fail(fmt.Errorf("pod listener output could not be read: %v", err))
	}
}

// Accept waits for the Pod side to hand over the next connection it accepted.
func (l *execPodListener) Accept(ctx context.Context) (podConn, error) {
	stream, err := l.session.Accept(ctx)
	if err != nil {
		select {
		case <-l.doneCh:
			// The listener's own account of what happened is the useful one; the
			// session only knows that its connection ended.
			return nil, l.closeErr()
		default:
		}
		return nil, err
	}
	return stream, nil
}

// execRequest builds the exec request that runs one command in the container.
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

// Close stops the multiplexer; it removes its own files inside the Pod.
func (l *execPodListener) Close() error {
	l.fail(fmt.Errorf("pod listener on %s:%d closed", l.bindAddr, l.bindPort))
	// Ending the session ends the exec's stdin, which is the shutdown signal that
	// reaches a runtime that leaves the exec'd process running after the stream is
	// torn down. It also releases every connection still on the session.
	if l.session != nil {
		_ = l.session.Close()
	} else if l.stdinW != nil {
		_ = l.stdinW.Close()
	}
	// Wait for the multiplexer to actually exit before reporting the listener gone.
	// A client that reconnects asks for the same port straight away, and reuseaddr
	// does not cover a socket another live process is still listening on - so
	// returning early turns a reconnect into "the port you just released is busy".
	// Bounded, because a stuck exec must not hold up the rest of the teardown.
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
	if err == nil {
		err = io.EOF
	}
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

// probeScript reports the container's architecture and a directory the multiplexer
// can be executed from.
//
// Both answers have to come from inside the container: the node's architecture is
// not necessarily the container's, and an image whose /tmp is mounted noexec would
// otherwise fail with nothing but "permission denied" at the moment of running the
// binary, long after the reason could be explained.
//
// The candidates are shared, world-writable directories, so the probe path is named
// by the same unguessable token as the install and is created with a plain mkdir.
// Neither is decoration: a predictable name plus `mkdir -p` is a directory another
// process in the container can pre-create as a symlink, in which case the probe
// would write its test file wherever that symlink pointed.
func probeScript(token string) string {
	return fmt.Sprintf(`M=$(uname -m 2>/dev/null) || M=
if [ -z "$M" ]; then
  echo "%[2]s uname is not available in the container" >&2
  exit 1
fi
echo "ARCH $M"
for D in %[1]s; do
  [ -d "$D" ] || continue
  P="$D/.safe-rfwd-%[3]s"
  # No -p, and no removing whatever is already there: mkdir must be what creates
  # this directory, so that anything else of that name is a reason to move on.
  # -m rather than a following chmod: otherwise it exists, briefly, with whatever
  # the image's umask allows.
  mkdir -m 700 "$P" 2>/dev/null || continue
  if printf '#!/bin/sh\nexit 0\n' > "$P/t" 2>/dev/null && chmod 700 "$P/t" 2>/dev/null &&
     "$P/t" 2>/dev/null; then
    rm -rf "$P"
    echo "DIR $D"
    exit 0
  fi
  rm -rf "$P"
done
echo "%[2]s no directory among %[1]s is both writable and executable" >&2
exit 1
`, strings.Join(installDirs, " "), rfwdErrMarker, token)
}

// installScript writes the multiplexer arriving on stdin and proves it runs.
//
// dir is built from a hex token and one of installDirs, so nothing interpolated
// here comes from outside this process. As in the probe, mkdir has to be what
// creates it: these are shared directories, and a path that already exists is one
// this process did not make.
func installScript(dir string) string {
	return fmt.Sprintf(`set -e
D=%[1]s
if ! mkdir -m 700 "$D" 2>/dev/null; then
  echo "%[2]s could not create $D in this container" >&2
  exit 1
fi
cat > "$D/mux"
chmod 700 "$D/mux"
# Proving it runs here turns the two failures that look identical later - a binary
# for the wrong architecture, and a filesystem that turned out not to be executable
# after all - into a reason reported before any connection depends on it.
if ! "$D/mux" check >/dev/null 2>&1; then
  echo "%[2]s the multiplexer written to $D could not run in this container" >&2
  rm -rf "$D"
  exit 1
fi
`, dir, rfwdErrMarker)
}

// runScript runs the multiplexer with the exec's stdin and stdout as its session.
//
// The multiplexer removes its own directory once the port is bound, so the trap is
// only for the paths where it never got that far.
func runScript(dir, bindAddr string, bindPort uint32) string {
	return fmt.Sprintf(`D=%[1]s
trap 'rm -rf "$D"' EXIT INT TERM
"$D/mux" listen -max-streams %[4]d -remove-dir "$D" %[2]s %[3]d
`, dir, bindAddr, bindPort, muxMaxStreams)
}

// parseProbe reads the architecture and install directory out of the probe's output.
func parseProbe(stdout string) (machine, dir string, err error) {
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "ARCH":
			machine = fields[1]
		case "DIR":
			dir = fields[1]
		}
	}
	if machine == "" {
		return "", "", fmt.Errorf("the container did not report its architecture")
	}
	// The directory is checked against the list we asked for rather than trusted:
	// it becomes a path this process writes to and executes.
	for _, allowed := range installDirs {
		if dir == allowed {
			return machine, dir, nil
		}
	}
	return "", "", fmt.Errorf("the container reported no directory it can execute from")
}

// reportedReason picks the Pod side's own account out of a failed setup exec, so
// the user is told what the container said rather than only that a command failed.
func reportedReason(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, rfwdErrMarker) {
			return ":" + strings.TrimPrefix(line, rfwdErrMarker)
		}
	}
	return ""
}

// randomToken returns a hex token used to name the Pod-side install directory.
func randomToken() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate reverse forward token: %v", err)
	}
	return hex.EncodeToString(buf), nil
}

// limitedWriter keeps a container's output from becoming this process's memory.
type limitedWriter struct {
	w    io.Writer
	left int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	// Everything is accounted for even once nothing more is kept: a short write
	// would be reported as an error and abort the exec stream.
	total := len(p)
	if l.left <= 0 {
		return total, nil
	}
	if len(p) > l.left {
		p = p[:l.left]
	}
	l.left -= len(p)
	if _, err := l.w.Write(p); err != nil {
		return 0, err
	}
	return total, nil
}
