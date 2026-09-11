/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers/muxbin"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers/rfwdmux"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// --- the scripts we send into the pod, against a real shell -----------------

// runScriptLocally runs one of the pod-side scripts the way a container would, so
// the shell the apiserver depends on is exercised rather than only string-matched.
func runScriptLocally(t *testing.T, script string, stdin io.Reader) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestProbeScriptReportsArchitectureAndAnExecutableDirectory(t *testing.T) {
	stdout, stderr, err := runScriptLocally(t, probeScript(testToken(t)), nil)
	testifyassert.NoError(t, err, stderr)

	machine, dir, err := parseProbe(stdout)
	testifyassert.NoError(t, err)
	testifyassert.NotEmpty(t, machine)
	testifyassert.Contains(t, installDirs, dir)
}

// TestProbeScriptFallsPastADirectoryItCannotUse covers the noexec /tmp the plan
// calls out: the probe has to keep looking rather than report the first writable
// directory it finds.
func TestProbeScriptFallsPastADirectoryItCannotUse(t *testing.T) {
	unusable := filepath.Join(t.TempDir(), "not-a-directory")
	testifyassert.NoError(t, os.WriteFile(unusable, []byte("x"), 0o600))
	restore := installDirs
	installDirs = []string{unusable, "/tmp"}
	t.Cleanup(func() { installDirs = restore })

	stdout, stderr, err := runScriptLocally(t, probeScript(testToken(t)), nil)
	testifyassert.NoError(t, err, stderr)
	_, dir, err := parseProbe(stdout)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "/tmp", dir)
}

// TestInstallScriptRefusesADirectoryItDidNotCreate covers the shared, world-writable
// directories the multiplexer is installed into: mkdir has to be what creates the
// path, so that something already sitting there - a symlink another process in the
// container put in the way - is refused rather than written through.
func TestInstallScriptRefusesADirectoryItDidNotCreate(t *testing.T) {
	elsewhere := t.TempDir()
	dir := filepath.Join(t.TempDir(), ".safe-rfwd-"+testToken(t))
	testifyassert.NoError(t, os.Symlink(elsewhere, dir))

	_, stderr, err := runScriptLocally(t, installScript(dir), strings.NewReader("payload"))
	testifyassert.Error(t, err)
	testifyassert.Contains(t, stderr, rfwdErrMarker)
	testifyassert.Contains(t, stderr, "could not create")

	// Nothing was written through the symlink.
	entries, err := os.ReadDir(elsewhere)
	testifyassert.NoError(t, err)
	testifyassert.Empty(t, entries)
}

// TestProbeScriptNamesTheProblem keeps a container with nowhere to run from being
// reported as a mysterious failure to listen.
func TestProbeScriptNamesTheProblem(t *testing.T) {
	restore := installDirs
	installDirs = []string{filepath.Join(t.TempDir(), "absent")}
	t.Cleanup(func() { installDirs = restore })

	_, stderr, err := runScriptLocally(t, probeScript(testToken(t)), nil)
	testifyassert.Error(t, err)
	testifyassert.Contains(t, stderr, rfwdErrMarker)
	testifyassert.Contains(t, stderr, "writable and executable")
}

func TestInstallScriptWritesABinaryThatRuns(t *testing.T) {
	binary := hostMuxBinary(t)
	dir := filepath.Join(t.TempDir(), ".safe-rfwd-"+testToken(t))

	_, stderr, err := runScriptLocally(t, installScript(dir), bytes.NewReader(binary))
	testifyassert.NoError(t, err, stderr)

	info, err := os.Stat(filepath.Join(dir, "mux"))
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	testifyassert.Equal(t, int64(len(binary)), info.Size())
}

// TestInstallScriptRejectsABinaryThatCannotRun is the check that turns the two
// failures that look identical later - the wrong architecture, and a filesystem
// that is not executable after all - into a reason reported before any connection
// depends on it.
func TestInstallScriptRejectsABinaryThatCannotRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".safe-rfwd-"+testToken(t))

	_, stderr, err := runScriptLocally(t, installScript(dir), strings.NewReader("not a binary"))
	testifyassert.Error(t, err)
	testifyassert.Contains(t, stderr, rfwdErrMarker)
	testifyassert.Contains(t, stderr, "could not run in this container")

	// Nothing is left behind for a later session to trip over.
	_, err = os.Stat(dir)
	testifyassert.True(t, os.IsNotExist(err))
}

func TestRunScriptStartsTheInstalledMultiplexer(t *testing.T) {
	script := runScript("/tmp/.safe-rfwd-abcd", "127.0.0.1", 10001)
	testifyassert.Contains(t, script,
		fmt.Sprintf(`"$D/mux" listen -max-streams %d -remove-dir "$D" 127.0.0.1 10001`, muxMaxStreams))
	// The multiplexer removes its own files once the port is bound; the trap is for
	// the paths where it never got that far.
	testifyassert.Contains(t, script, `trap 'rm -rf "$D"' EXIT INT TERM`)
	testifyassert.Contains(t, script, "D=/tmp/.safe-rfwd-abcd")
}

func TestParseProbe(t *testing.T) {
	machine, dir, err := parseProbe("ARCH x86_64\nDIR /dev/shm\n")
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "x86_64", machine)
	testifyassert.Equal(t, "/dev/shm", dir)

	// A directory the apiserver did not offer must not become a path it writes to
	// and executes, however the pod came to report it.
	_, _, err = parseProbe("ARCH x86_64\nDIR /etc\n")
	testifyassert.Error(t, err)
	_, _, err = parseProbe("ARCH x86_64\n")
	testifyassert.Error(t, err)
	_, _, err = parseProbe("DIR /tmp\n")
	testifyassert.Error(t, err)
}

func TestMuxBinaryForUnknownMachine(t *testing.T) {
	_, _, err := muxbin.For("s390x")
	testifyassert.ErrorContains(t, err, "s390x")
	testifyassert.True(t, muxbin.Available("x86_64") == muxbin.Available("amd64"))
}

// testToken names the paths a script test creates. The probe writes into the same
// shared directories production does, so a fixed name would collide with a leftover
// from an interrupted run or with another copy of the suite running beside it.
func testToken(t *testing.T) string {
	t.Helper()
	token, err := randomToken()
	testifyassert.NoError(t, err)
	return token
}

// hostMux is the multiplexer built for the machine running the tests, built once
// for the whole run.
var hostMux struct {
	once   sync.Once
	binary []byte
	err    error
}

// hostMuxBinary returns a multiplexer the tests can actually inject.
//
// It builds one rather than reading the embedded copy: a source checkout carries a
// placeholder, and the tests that exercise the real binary - the install script, and
// the whole stack over a local exec - are the ones most worth not skipping. The
// build is the same one the image build runs, and Go caches it after the first.
func hostMuxBinary(t *testing.T) []byte {
	t.Helper()
	hostMux.once.Do(func() {
		if _, err := exec.LookPath("go"); err != nil {
			hostMux.err = fmt.Errorf("no go toolchain to build the multiplexer with: %v", err)
			return
		}
		out := filepath.Join(t.TempDir(), "safe-rfwd-mux")
		build := exec.Command("go", "build", "-o", out,
			"github.com/AMD-AIG-AIMA/SAFE/apiserver/cmd/safe-rfwd-mux")
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if output, err := build.CombinedOutput(); err != nil {
			hostMux.err = fmt.Errorf("could not build the multiplexer: %v: %s", err, output)
			return
		}
		hostMux.binary, hostMux.err = os.ReadFile(out)
	})
	if hostMux.err != nil {
		t.Skipf("%v", hostMux.err)
	}
	return hostMux.binary
}

// injectHostMux makes the listener install a multiplexer that really runs, in place
// of whatever the tree has embedded.
func injectHostMux(t *testing.T) {
	t.Helper()
	binary := hostMuxBinary(t)
	previous := muxBinaryFor
	muxBinaryFor = func(machine string) ([]byte, string, error) {
		return binary, "linux/" + machine, nil
	}
	t.Cleanup(func() { muxBinaryFor = previous })
}

// --- the go side of the listener, without a Kubernetes API server ------------

// podScript is which of the three scripts an exec is running.
type podScript int

const (
	probeExec podScript = iota
	installExec
	runExec
	cleanupExec
)

// classify tells the scripts apart the way a reader of the exec log would.
func classify(script string) podScript {
	switch {
	case strings.Contains(script, "uname -m"):
		return probeExec
	case strings.Contains(script, `cat > "$D/mux"`):
		return installExec
	case strings.HasPrefix(script, "rm -rf "):
		return cleanupExec
	default:
		return runExec
	}
}

// fakePodBehaviour bends what the stubbed pod does, so a test can reach the
// failures a real container would produce.
type fakePodBehaviour struct {
	probeStdout   string
	probeStderr   string
	probeErr      error
	installStdin  io.Writer
	installErr    error
	installStderr string
	// silent models a multiplexer that starts but never reports the port bound.
	silent    bool
	runErr    error
	runStderr string
	// linger models a run exec that keeps going after its stdin ends, which is what
	// a runtime that leaves the exec'd process running looks like.
	linger bool
}

// fakePod stands in for one target container across the three execs a forward takes.
type fakePod struct {
	t         *testing.T
	behave    fakePodBehaviour
	release   chan struct{}
	installed chan []byte
	cleaned   chan string

	mu          sync.Mutex
	scripts     map[podScript]string
	stdinSet    map[podScript]bool
	session     *rfwdmux.Session
	sessionUp   chan struct{}
	sessionOnce sync.Once
}

// testMuxBinary stands in for the embedded multiplexer. Its leading bytes are what
// muxbin uses to tell a real binary from the placeholder a source checkout carries.
var testMuxBinary = append([]byte{0x7f, 'E', 'L', 'F'}, []byte(" not really a multiplexer")...)

// stubMuxBinary answers for the architectures the apiserver builds for, and refuses
// the rest the way the real lookup does.
func stubMuxBinary(t *testing.T) {
	t.Helper()
	previous := muxBinaryFor
	muxBinaryFor = func(machine string) ([]byte, string, error) {
		switch machine {
		case "x86_64", "amd64":
			return testMuxBinary, "linux/amd64", nil
		case "aarch64", "arm64":
			return testMuxBinary, "linux/arm64", nil
		default:
			return nil, "", fmt.Errorf("no reverse forward multiplexer is built for machine %q", machine)
		}
	}
	t.Cleanup(func() { muxBinaryFor = previous })
}

// stubPod routes every exec to a fake container for the test's duration.
func stubPod(t *testing.T, behave fakePodBehaviour) *fakePod {
	t.Helper()
	stubMuxBinary(t)
	p := &fakePod{
		t:         t,
		behave:    behave,
		release:   make(chan struct{}),
		installed: make(chan []byte, 1),
		cleaned:   make(chan string, 4),
		scripts:   map[podScript]string{},
		stdinSet:  map[podScript]bool{},
		sessionUp: make(chan struct{}),
	}
	previous := newPodExecutor
	newPodExecutor = func(_ *execPodListener, script string, stdin bool) (remotecommand.Executor, error) {
		kind := classify(script)
		p.mu.Lock()
		p.scripts[kind] = script
		p.stdinSet[kind] = stdin
		p.mu.Unlock()
		return &fakeExec{pod: p, kind: kind}, nil
	}
	t.Cleanup(func() {
		newPodExecutor = previous
		close(p.release)
	})
	return p
}

// podSession is the multiplexer end of the run exec, once it is up.
func (p *fakePod) podSession() *rfwdmux.Session {
	select {
	case <-p.sessionUp:
	case <-time.After(20 * time.Second):
		p.t.Fatal("the run exec never started a session")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session
}

// script returns what the apiserver asked the container to run.
func (p *fakePod) script(kind podScript) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scripts[kind]
}

// wantsStdin reports whether an exec was given a stdin stream.
func (p *fakePod) wantsStdin(kind podScript) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stdinSet[kind]
}

// fakeExec is one exec against the fake container.
type fakeExec struct {
	pod  *fakePod
	kind podScript
}

func (f *fakeExec) Stream(opts remotecommand.StreamOptions) error {
	return f.StreamWithContext(context.Background(), opts)
}

func (f *fakeExec) StreamWithContext(ctx context.Context, opts remotecommand.StreamOptions) error {
	b := f.pod.behave
	switch f.kind {
	case probeExec:
		stdout := b.probeStdout
		if stdout == "" {
			stdout = "ARCH x86_64\nDIR /tmp\n"
		}
		_, _ = io.WriteString(opts.Stdout, stdout)
		if b.probeStderr != "" {
			_, _ = io.WriteString(opts.Stderr, b.probeStderr+"\n")
		}
		return b.probeErr
	case cleanupExec:
		select {
		case f.pod.cleaned <- f.pod.script(cleanupExec):
		default:
		}
		return nil
	case installExec:
		var written bytes.Buffer
		if opts.Stdin != nil {
			_, _ = io.Copy(&written, opts.Stdin)
		}
		select {
		case f.pod.installed <- written.Bytes():
		default:
		}
		if b.installStderr != "" {
			_, _ = io.WriteString(opts.Stderr, b.installStderr+"\n")
		}
		return b.installErr
	default:
		return f.runStream(ctx, opts)
	}
}

// runStream behaves like the multiplexer: it speaks the real framing over the
// exec's stdin and stdout, and stops when its stdin ends.
func (f *fakeExec) runStream(ctx context.Context, opts remotecommand.StreamOptions) error {
	b := f.pod.behave
	if b.runStderr != "" {
		_, _ = io.WriteString(opts.Stderr, b.runStderr+"\n")
	}
	if b.runErr != nil {
		return b.runErr
	}
	session := rfwdmux.NewSession(
		rfwdmux.Join(opts.Stdin, opts.Stdout),
		rfwdmux.Config{Initiator: true})
	f.pod.mu.Lock()
	f.pod.session = session
	f.pod.mu.Unlock()
	f.pod.sessionOnce.Do(func() { close(f.pod.sessionUp) })

	if !b.silent {
		_, _ = io.WriteString(opts.Stderr, rfwdmux.ReadyMarker+"\n")
	}
	select {
	case <-session.Done():
		if b.linger {
			// The stream is gone but the process is not; only tearing the exec down
			// ends it.
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	case <-ctx.Done():
		_ = session.Close()
		return ctx.Err()
	case <-f.pod.release:
		return nil
	}
}

// newTestListener starts a listener against the fake container.
func newTestListener(t *testing.T) (podListener, *fakePod) {
	t.Helper()
	pod := stubPod(t, fakePodBehaviour{})
	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)
	if err != nil {
		t.FailNow()
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener, pod
}

// TestExecPodListenerInstallsTheMultiplexer pins the shape of a forward's setup:
// one probe, one install carrying the binary itself, and one long-lived run exec.
func TestExecPodListenerInstallsTheMultiplexer(t *testing.T) {
	_, pod := newTestListener(t)

	testifyassert.False(t, pod.wantsStdin(probeExec), "the probe has nothing to be sent")
	testifyassert.True(t, pod.wantsStdin(installExec), "the binary arrives on the install exec's stdin")
	testifyassert.True(t, pod.wantsStdin(runExec), "ending stdin is how the multiplexer is stopped")

	select {
	case written := <-pod.installed:
		testifyassert.True(t, bytes.Equal(testMuxBinary, written),
			"the install exec carries the embedded binary itself")
	case <-time.After(20 * time.Second):
		t.Fatal("nothing was written to the install exec")
	}

	// The install directory is under the probe's answer, named by a token, and the
	// run exec is the one that binds the port.
	testifyassert.Contains(t, pod.script(installExec), "/tmp/.safe-rfwd-")
	testifyassert.Contains(t, pod.script(runExec), "127.0.0.1 10001")
}

// TestExecPodListenerAcceptCarriesAConnection is the whole point of the session:
// a connection the pod accepted arrives as a stream, with its origin intact.
func TestExecPodListenerAcceptCarriesAConnection(t *testing.T) {
	listener, pod := newTestListener(t)

	podStream, err := pod.podSession().Open("10.0.0.9", 51234)
	testifyassert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := listener.Accept(ctx)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "10.0.0.9", conn.OriginAddr())
	testifyassert.Equal(t, uint32(51234), conn.OriginPort())

	_, err = podStream.Write([]byte("from-pod"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("from-pod"))
	_, err = io.ReadFull(conn, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-pod", string(buf))

	// And a half-close in the other direction leaves the reply on its way.
	_, err = conn.Write([]byte("from-apiserver"))
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, conn.CloseWrite())
	got, err := io.ReadAll(podStream)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-apiserver", string(got))
}

// TestExecPodListenerCloseEndsTheSession pins the shutdown signal the apiserver
// sends to the pod: it ends the run exec's stdin, which is the only teardown a
// runtime honours when it leaves the exec'd process running.
func TestExecPodListenerCloseEndsTheSession(t *testing.T) {
	listener, pod := newTestListener(t)
	session := pod.podSession()

	testifyassert.NoError(t, listener.Close())
	select {
	case <-session.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("the pod-side session was never told the forward ended")
	}

	// Accept must not block once the listener is gone.
	_, err := listener.Accept(context.Background())
	testifyassert.Error(t, err)
}

// TestExecPodListenerCloseGivesUpOnAStuckRun keeps one wedged container from
// holding up the rest of a session's teardown.
func TestExecPodListenerCloseGivesUpOnAStuckRun(t *testing.T) {
	previous := listenerShutdownGrace
	listenerShutdownGrace = 200 * time.Millisecond
	t.Cleanup(func() { listenerShutdownGrace = previous })

	stubPod(t, fakePodBehaviour{linger: true})
	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)

	start := time.Now()
	testifyassert.NoError(t, listener.Close())
	testifyassert.Less(t, time.Since(start), 5*time.Second)
}

// TestExecPodListenerCloseIsPromptWithABacklog covers teardown while connections
// nobody has accepted are queued: the queue must not be what Close waits on.
func TestExecPodListenerCloseIsPromptWithABacklog(t *testing.T) {
	previous := listenerShutdownGrace
	listenerShutdownGrace = 5 * time.Second
	t.Cleanup(func() { listenerShutdownGrace = previous })

	listener, pod := newTestListener(t)
	session := pod.podSession()
	for i := 0; i < 64; i++ {
		_, err := session.Open("10.0.0.9", uint32(40000+i))
		testifyassert.NoError(t, err)
	}
	waitFor(t, func() bool { return session.NumStreams() == 64 }, "the backlog to build")

	start := time.Now()
	testifyassert.NoError(t, listener.Close())
	testifyassert.Less(t, time.Since(start), 2*time.Second,
		"Close waited out its grace because a backlog was left unread")
}

// TestExecPodListenerReportsPodSideFailure pins the consumer of the error marker.
// Without it a pod that cannot bind - the port already taken - would leave the
// caller waiting out the readiness timeout instead of being told what went wrong.
func TestExecPodListenerReportsPodSideFailure(t *testing.T) {
	stubPod(t, fakePodBehaviour{
		silent:    true,
		runStderr: rfwdErrMarker + " failed to listen on 127.0.0.1:10001",
	})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "failed to listen on 127.0.0.1:10001")
}

// TestExecPodListenerReportsTheRunExecDying covers the multiplexer exiting before
// it ever reports itself ready.
func TestExecPodListenerReportsTheRunExecDying(t *testing.T) {
	stubPod(t, fakePodBehaviour{runErr: fmt.Errorf("command terminated with exit code 126")})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "exit code 126")
}

// TestExecPodListenerRemovesAnInstallThatNeverRan covers the one path where nothing
// inside the pod would clear the multiplexer away: the run script's trap and the
// multiplexer's own cleanup both need it to have started, and here it never did.
func TestExecPodListenerRemovesAnInstallThatNeverRan(t *testing.T) {
	previous := listenerReadyTimeout
	listenerReadyTimeout = 200 * time.Millisecond
	t.Cleanup(func() { listenerReadyTimeout = previous })

	pod := stubPod(t, fakePodBehaviour{silent: true})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.Error(t, err)

	select {
	case script := <-pod.cleaned:
		testifyassert.Contains(t, script, "rm -rf /tmp/.safe-rfwd-")
	case <-time.After(20 * time.Second):
		t.Fatal("a failed setup left the multiplexer in the pod")
	}
}

// TestExecPodListenerRemovesAnInstallThatFailedHalfway covers the other arm of the
// same promise: a setup that died inside the install script. The script only
// removes its own directory on the branch where the binary would not run, so an
// exec cut short anywhere before that - a cancelled request, a full filesystem -
// leaves a multi-megabyte binary in the user's container for the life of the pod,
// under a fresh token each time it is retried.
func TestExecPodListenerRemovesAnInstallThatFailedHalfway(t *testing.T) {
	pod := stubPod(t, fakePodBehaviour{installErr: context.Canceled})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.Error(t, err)

	select {
	case script := <-pod.cleaned:
		// Every directory the probe could have written under, not just the one it
		// settled on: a setup cancelled during the probe has no chosen directory
		// yet, and what it left behind still has to go.
		for _, dir := range installDirs {
			testifyassert.Containsf(t, script, dir+"/.safe-rfwd-",
				"the cleanup must cover %s", dir)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a failed install left the multiplexer in the pod")
	}
}

// TestExecPodListenerCleansUpAfterAFailedProbe covers the stage before any
// directory has been chosen, where there is no install path to name yet.
func TestExecPodListenerCleansUpAfterAFailedProbe(t *testing.T) {
	pod := stubPod(t, fakePodBehaviour{probeErr: context.Canceled})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.Error(t, err)

	select {
	case script := <-pod.cleaned:
		testifyassert.Contains(t, script, "/.safe-rfwd-")
	case <-time.After(20 * time.Second):
		t.Fatal("a cancelled probe left its test directory in the pod")
	}
}

// TestExecPodListenerTimesOutOnASilentPod keeps a container that starts the
// multiplexer but never binds from holding the SSH global request forever.
func TestExecPodListenerTimesOutOnASilentPod(t *testing.T) {
	previous := listenerReadyTimeout
	listenerReadyTimeout = 200 * time.Millisecond
	t.Cleanup(func() { listenerReadyTimeout = previous })

	stubPod(t, fakePodBehaviour{silent: true})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "timed out waiting for pod listener")
}

// TestExecPodListenerReportsAnUnreachableProbe surfaces the container's own words
// rather than only that a command failed.
func TestExecPodListenerReportsAnUnreachableProbe(t *testing.T) {
	stubPod(t, fakePodBehaviour{
		probeErr:    fmt.Errorf("command terminated with exit code 1"),
		probeStderr: rfwdErrMarker + " no directory among /tmp is both writable and executable",
	})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "writable and executable")
}

// TestExecPodListenerReportsAnUnsupportedArchitecture stops before writing
// anything, because there is nothing that would run.
func TestExecPodListenerReportsAnUnsupportedArchitecture(t *testing.T) {
	stubPod(t, fakePodBehaviour{probeStdout: "ARCH s390x\nDIR /tmp\n"})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "s390x")
}

// TestExecPodListenerReportsAFailedInstall names the install rather than leaving
// the caller to guess which of the three execs failed.
func TestExecPodListenerReportsAFailedInstall(t *testing.T) {
	stubPod(t, fakePodBehaviour{
		installErr:    fmt.Errorf("command terminated with exit code 1"),
		installStderr: rfwdErrMarker + " the multiplexer written to /tmp/x could not run in this container",
	})
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "could not run in this container")
}

// TestExecPodListenerSurvivesChatter keeps diagnostics from the container - which
// are neither markers nor connections - from being read as anything else.
func TestExecPodListenerSurvivesChatter(t *testing.T) {
	pod := stubPod(t, fakePodBehaviour{runStderr: "libfoo: warning about nothing in particular"})
	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	session := pod.podSession()

	stream, err := session.Open("10.0.0.9", 1)
	testifyassert.NoError(t, err)
	defer stream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := listener.Accept(ctx)
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, conn.Close())
}

// TestExecPodListenerRefusesPastTheCap is the overload contract seen from the
// apiserver: the cap is what the pod side is told to enforce, and the session on
// this end enforces the same number.
func TestExecPodListenerRefusesPastTheCap(t *testing.T) {
	listener, pod := newTestListener(t)
	testifyassert.Contains(t, pod.script(runExec), fmt.Sprintf("-max-streams %d", muxMaxStreams))

	l, ok := listener.(*execPodListener)
	testifyassert.True(t, ok)
	testifyassert.Equal(t, muxMaxStreams, l.session.MaxStreams())
}

func TestExecRequestShape(t *testing.T) {
	clientSet, err := kubernetes.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	testifyassert.NoError(t, err)
	clients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", clientSet)
	clients.AttachRestConfigForTest(&rest.Config{Host: "https://example.invalid"})

	l := &execPodListener{
		clients:   clients,
		userInfo:  &UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"},
		container: "main",
	}

	url := l.execRequest("echo hello", true).URL()
	testifyassert.Contains(t, url.Path, "/namespaces/ns/pods/pod-0/exec")
	query := url.Query()
	testifyassert.Equal(t, "main", query.Get("container"))
	// The multiplexer learns the session ended from its stdin closing, so the exec
	// has to carry one; stdout carries the multiplexed session and stderr the
	// readiness marker.
	testifyassert.Equal(t, "true", query.Get("stdin"))
	testifyassert.Equal(t, "true", query.Get("stdout"))
	testifyassert.Equal(t, "true", query.Get("stderr"))
	// A false flag is left out of the query rather than spelled out.
	testifyassert.Empty(t, query.Get("tty"))
	testifyassert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, query["command"])

	// An exec that does not need stdin must not be given one.
	testifyassert.Empty(t, l.execRequest("echo hello", false).URL().Query().Get("stdin"))

	executor, err := l.newExecutor("echo hello", true)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, executor)
}

func TestLimitedWriterAbsorbsAFlood(t *testing.T) {
	var sink bytes.Buffer
	w := &limitedWriter{w: &sink, left: 8}
	// A short write would abort the exec stream, so everything is accounted for
	// even once nothing more is kept.
	n, err := w.Write([]byte("0123456789"))
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, 10, n)
	n, err = w.Write([]byte("more"))
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, 4, n)
	testifyassert.Equal(t, "01234567", sink.String())
}

func TestReportedReason(t *testing.T) {
	testifyassert.Equal(t, ": nothing is executable",
		reportedReason("some noise\n"+rfwdErrMarker+" nothing is executable\n"))
	testifyassert.Empty(t, reportedReason("just noise\n"))
}

// --- shared port helpers ----------------------------------------------------

// waitForPortFree blocks until nothing holds the listen port any more.
func waitForPortFree(t *testing.T, port uint32) {
	t.Helper()
	waitFor(t, func() bool {
		l, listenErr := net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
		if listenErr != nil {
			return false
		}
		_ = l.Close()
		return true
	}, "the pod listen port to be released")
}

// freeTCPPort returns a loopback port that is free right now.
func freeTCPPort(t *testing.T) uint32 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	testifyassert.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	testifyassert.NoError(t, l.Close())
	return uint32(port)
}

// itoa renders a port for net.JoinHostPort.
func itoa(port uint32) string {
	return strconv.FormatUint(uint64(port), 10)
}

// TestExecPodListenerFailsPromptlyWhenTheRunExecCannotStart covers teardown of a
// listener that never got as far as a stream: Close has nothing to wait for, and
// must not sit out its grace discovering that.
func TestExecPodListenerFailsPromptlyWhenTheRunExecCannotStart(t *testing.T) {
	previous := listenerShutdownGrace
	listenerShutdownGrace = 30 * time.Second
	t.Cleanup(func() { listenerShutdownGrace = previous })

	stubPod(t, fakePodBehaviour{})
	wrapped := newPodExecutor
	newPodExecutor = func(l *execPodListener, script string, stdin bool) (remotecommand.Executor, error) {
		if classify(script) == runExec {
			return nil, fmt.Errorf("no route to the api server")
		}
		return wrapped(l, script, stdin)
	}

	start := time.Now()
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.ErrorContains(t, err, "no route to the api server")
	testifyassert.Less(t, time.Since(start), 5*time.Second)
}
