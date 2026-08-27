/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bufio"
	"context"
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

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// TestAcceptorScriptAgainstRealSocat runs the scripts we send into the Pod against a
// real socat, so the shell the apiserver depends on is exercised rather than only
// string-matched. It is skipped where socat is unavailable.
func TestAcceptorScriptAgainstRealSocat(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat is not installed")
	}

	dir := filepath.Join(t.TempDir(), "rfwd")
	port := freeTCPPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	acceptor := startAcceptor(t, ctx, dir, port)
	announcements := acceptor.stderr

	// A connection to the Pod-side port is announced with its peer address.
	client, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
	testifyassert.NoError(t, err)
	defer client.Close()

	var announced acceptedConn
	var diagnostics []string
	deadline := time.After(30 * time.Second)
	for announced.id == "" {
		select {
		case line := <-announcements:
			if conn, ok := parseConnAnnouncement(line); ok {
				announced = conn
				break
			}
			diagnostics = append(diagnostics, line)
		case <-deadline:
			t.Fatalf("timed out waiting for a connection announcement; relay said: %v", diagnostics)
		}
	}
	testifyassert.Equal(t, "127.0.0.1", announced.peerAddr)
	testifyassert.NotZero(t, announced.peerPort)

	// Attaching to the announced rendezvous socket yields that connection's bytes.
	attach := exec.CommandContext(ctx, "/bin/sh", "-c", connectScript(dir, announced.id))
	attachIn, err := attach.StdinPipe()
	testifyassert.NoError(t, err)
	attachOut, err := attach.StdoutPipe()
	testifyassert.NoError(t, err)
	attachErr, err := attach.StderrPipe()
	testifyassert.NoError(t, err)
	attachDiag := make(chan string, 64)
	go scanLines(attachErr, attachDiag)
	defer func() {
		for {
			select {
			case line := <-attachDiag:
				t.Logf("attach stderr: %s", line)
			default:
				return
			}
		}
	}()
	testifyassert.NoError(t, attach.Start())
	defer func() {
		_ = attach.Process.Kill()
		_, _ = attach.Process.Wait()
	}()

	// Pod -> apiserver.
	_, err = client.Write([]byte("from-pod\n"))
	testifyassert.NoError(t, err)
	line, err := bufio.NewReader(attachOut).ReadString('\n')
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-pod\n", line)

	// apiserver -> Pod.
	_, err = attachIn.Write([]byte("from-client\n"))
	testifyassert.NoError(t, err)
	_ = client.SetReadDeadline(time.Now().Add(30 * time.Second))
	back, err := bufio.NewReader(client).ReadString('\n')
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "from-client\n", back)

	// SIGKILL is what the container runtime uses, and it leaves the script no
	// chance to run its trap: only the detached watcher can free the Pod port.
	_ = acceptor.cmd.Process.Kill()
	_, _ = acceptor.cmd.Process.Wait()
	waitForPortFree(t, port)
	waitFor(t, func() bool {
		_, statErr := os.Stat(dir)
		return os.IsNotExist(statErr)
	}, "the rendezvous directory to be removed")
}

// TestAcceptorScriptStdinCloseStopsListener pins the shutdown signal the apiserver
// actually sends: it ends the exec's stdin rather than signalling the script, which
// is the only teardown a runtime that leaves the exec'd process running will honour.
func TestAcceptorScriptStdinCloseStopsListener(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat is not installed")
	}

	dir := filepath.Join(t.TempDir(), "rfwd")
	port := freeTCPPort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	acceptor := startAcceptor(t, ctx, dir, port)

	// A connection the apiserver never attached to still has a relay pair behind it
	// in the pod, and ending the session has to release that too.
	orphan, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
	testifyassert.NoError(t, err)
	defer orphan.Close()
	select {
	case line := <-acceptor.stderr:
		_, ok := parseConnAnnouncement(line)
		testifyassert.Truef(t, ok, "unexpected relay output: %s", line)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the pod-side relay to announce the connection")
	}

	testifyassert.NoError(t, acceptor.stdin.Close())

	waitForPortFree(t, port)
	waitFor(t, func() bool {
		_, statErr := os.Stat(dir)
		return os.IsNotExist(statErr)
	}, "the rendezvous directory to be removed")

	_ = orphan.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, err = orphan.Read(make([]byte, 1))
	testifyassert.ErrorIs(t, err, io.EOF, "the pod-side relay must let the connection go")
}

// runningAcceptor is an acceptor script started for a test, with the streams the
// apiserver would hold open in production.
type runningAcceptor struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr chan string
}

// startAcceptor runs the acceptor script and returns once it reports it is bound.
func startAcceptor(t *testing.T, ctx context.Context, dir string, port uint32) *runningAcceptor {
	t.Helper()

	script := acceptorScript(dir, "127.0.0.1", port)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
	// The exec stream's stdin stays open for the life of the session; without it
	// the script would read EOF at once and shut itself down.
	stdin, err := cmd.StdinPipe()
	testifyassert.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	testifyassert.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	ready := make(chan string, 64)
	go scanLines(stdout, ready)
	announcements := make(chan string, 64)
	go scanLines(stderr, announcements)

	// The listener announces readiness only once it is actually bound.
	select {
	case line := <-ready:
		testifyassert.Equal(t, rfwdReadyMarker, line)
	case line := <-announcements:
		t.Fatalf("pod listener failed to bind: %s", line)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the pod listener to bind")
	}
	return &runningAcceptor{cmd: cmd, stdin: stdin, stderr: announcements}
}

// waitForPortFree blocks until nothing in the pod holds the listen port any more.
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

// TestAcceptorScriptWithoutSocat verifies the script reports a usable error when the
// workload image has no socat, instead of hanging until the readiness timeout.
func TestAcceptorScriptWithoutSocat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rfwd")
	cmd := exec.Command("/bin/sh", "-c", acceptorScript(dir, "127.0.0.1", 10001))
	// An empty PATH makes `command -v socat` fail the way a stripped image would.
	cmd.Env = append(os.Environ(), "PATH=")
	out, err := cmd.CombinedOutput()
	testifyassert.Error(t, err)
	testifyassert.Contains(t, string(out), rfwdErrMarker)
	testifyassert.Contains(t, string(out), "socat is not installed")
}

// scanLines forwards each line of r onto ch until r ends.
func scanLines(r io.Reader, ch chan<- string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		select {
		case ch <- line:
		default:
		}
	}
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

// --- the Go side of the listener, without a Kubernetes API server ------------

// fakePodExec stands in for one exec'd relay script inside the Pod.
type fakePodExec struct {
	script string
	stdin  bool

	mu   sync.Mutex
	opts remotecommand.StreamOptions
	// stdinEOF records that stdin ended cleanly. A stream torn down underneath the
	// script is a different thing and must not look like one.
	stdinEOF  bool
	output    *acceptorOutput
	started   chan struct{}
	copied    chan struct{}
	startOnce sync.Once
}

func newFakePodExec(script string, stdin bool) *fakePodExec {
	return &fakePodExec{
		script:  script,
		stdin:   stdin,
		started: make(chan struct{}),
		copied:  make(chan struct{}),
	}
}

// acceptorOutput overrides what a stubbed acceptor writes instead of reporting
// itself ready: relay diagnostics, or a failure the pod side detected.
type acceptorOutput struct {
	stdout, stderr string
	exitErr        error
}

func (f *fakePodExec) Stream(opts remotecommand.StreamOptions) error {
	return f.StreamWithContext(context.Background(), opts)
}

// StreamWithContext behaves like the relay script: it reports itself ready and then
// runs until either its stdin ends or the exec stream is torn down.
func (f *fakePodExec) StreamWithContext(ctx context.Context, opts remotecommand.StreamOptions) error {
	f.mu.Lock()
	f.opts = opts
	f.mu.Unlock()
	f.startOnce.Do(func() { close(f.started) })

	if strings.Contains(f.script, "UNIX-CONNECT") {
		// An attached connection: echo whatever the apiserver writes back at it.
		_, err := io.Copy(opts.Stdout, opts.Stdin)
		return err
	}

	if f.output != nil {
		if f.output.stdout != "" {
			_, _ = io.WriteString(opts.Stdout, f.output.stdout+"\n")
		}
		if f.output.stderr != "" {
			_, _ = io.WriteString(opts.Stderr, f.output.stderr+"\n")
		}
		if f.output.exitErr != nil {
			return f.output.exitErr
		}
	} else {
		_, _ = io.WriteString(opts.Stdout, rfwdReadyMarker+"\n")
	}
	go func() {
		defer close(f.copied)
		_, err := io.Copy(io.Discard, opts.Stdin)
		f.mu.Lock()
		f.stdinEOF = err == nil
		f.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		// A real container keeps reading until its stdin actually ends, so an EOF
		// already in flight must be seen before the stream is reported as gone.
		select {
		case <-f.copied:
		case <-time.After(time.Second):
		}
		return ctx.Err()
	case <-f.copied:
		return nil
	}
}

// announce feeds a connection announcement down the acceptor's stderr.
func (f *fakePodExec) announce(t *testing.T, line string) {
	t.Helper()
	<-f.started
	f.mu.Lock()
	stderr := f.opts.Stderr
	f.mu.Unlock()
	_, err := io.WriteString(stderr, line+"\n")
	testifyassert.NoError(t, err)
}

// stdinEnded reports whether the exec's stdin reached EOF, which is how the relay
// script learns the session is over.
func (f *fakePodExec) stdinEnded() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stdinEOF
}

// stubPodExec routes every relay script to a recorded fake for the test's duration.
func stubPodExec(t *testing.T, output ...*acceptorOutput) *sync.Map {
	t.Helper()
	execs := &sync.Map{}
	previous := newPodExecutor
	newPodExecutor = func(_ *execPodListener, script string, stdin bool) (remotecommand.Executor, error) {
		fake := newFakePodExec(script, stdin)
		if len(output) > 0 && !strings.Contains(script, "UNIX-CONNECT") {
			fake.output = output[0]
		}
		key := "connect"
		if !strings.Contains(script, "UNIX-CONNECT") {
			key = "acceptor"
		}
		execs.Store(key, fake)
		return fake, nil
	}
	t.Cleanup(func() { newPodExecutor = previous })
	return execs
}

// execFor returns the fake standing in for the named relay script.
func execFor(t *testing.T, execs *sync.Map, key string) *fakePodExec {
	t.Helper()
	var fake *fakePodExec
	waitFor(t, func() bool {
		value, ok := execs.Load(key)
		if !ok {
			return false
		}
		fake = value.(*fakePodExec)
		return true
	}, "the "+key+" exec to be started")
	return fake
}

// TestExecPodListenerCloseEndsStdin pins the shutdown signal the apiserver sends to
// the Pod: the acceptor exec must carry a stdin stream, and closing the listener
// must end it. That is the only teardown a runtime honours when it leaves the
// exec'd process running after the stream is torn down.
func TestExecPodListenerCloseEndsStdin(t *testing.T) {
	execs := stubPodExec(t)

	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)

	acceptor := execFor(t, execs, "acceptor")
	testifyassert.True(t, acceptor.stdin, "the acceptor exec must request stdin")
	testifyassert.False(t, acceptor.stdinEnded())

	testifyassert.NoError(t, listener.Close())
	waitFor(t, acceptor.stdinEnded, "the acceptor's stdin to end")

	// Accept must not block once the listener is gone.
	_, err = listener.Accept(context.Background())
	testifyassert.Error(t, err)
}

// TestExecPodListenerAcceptBridgesAnnouncedConn verifies an announcement from the
// Pod turns into a byte stream attached to that connection.
func TestExecPodListenerAcceptBridgesAnnouncedConn(t *testing.T) {
	execs := stubPodExec(t)

	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	execFor(t, execs, "acceptor").announce(t, rfwdConnMarker+" 4242 10.0.0.9 51234")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := listener.Accept(ctx)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "10.0.0.9", conn.OriginAddr())
	testifyassert.Equal(t, uint32(51234), conn.OriginPort())

	// The connect exec attaches to the announced rendezvous socket, not another one.
	connect := execFor(t, execs, "connect")
	testifyassert.Contains(t, connect.script, "/4242")
	testifyassert.True(t, connect.stdin)

	_, err = conn.Write([]byte("to-pod\n"))
	testifyassert.NoError(t, err)
	line, err := bufio.NewReader(conn).ReadString('\n')
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "to-pod\n", line)

	testifyassert.NoError(t, conn.Close())
}

// TestExecPodListenerIgnoresMalformedAnnouncement verifies a garbled announcement is
// dropped rather than becoming a connection or killing the listener.
func TestExecPodListenerIgnoresMalformedAnnouncement(t *testing.T) {
	execs := stubPodExec(t)

	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	acceptor := execFor(t, execs, "acceptor")
	acceptor.announce(t, rfwdConnMarker+" ../escape 10.0.0.9 51234")
	acceptor.announce(t, rfwdConnMarker+" 4242 10.0.0.9 51234")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := listener.Accept(ctx)
	testifyassert.NoError(t, err)
	connect := execFor(t, execs, "connect")
	testifyassert.Contains(t, connect.script, "/4242")
	testifyassert.NotContains(t, connect.script, "escape")
	testifyassert.NoError(t, conn.Close())
}

// TestChildScriptDoesNotAnnounceWithoutItsSocket pins that a child which never
// managed to publish its rendezvous socket says so instead of announcing a
// connection. An announcement without a socket sends the apiserver to attach to a
// path that will never exist, and the pod-side application waits out the connect
// retries before its connection dies for no stated reason.
func TestChildScriptDoesNotAnnounceWithoutItsSocket(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", childScript(dir))
	// An empty PATH makes the child's socat fail, so the socket never appears.
	cmd.Env = append(os.Environ(), "PATH=")
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()

	testifyassert.Error(t, err, "a child with no socket must not exit successfully")
	testifyassert.NotContains(t, string(out), rfwdConnMarker,
		"a connection was announced although its rendezvous socket never appeared")
	// A child that cannot publish its socket loses that one connection; the
	// listener itself is fine, so this must not be the listener's error marker.
	testifyassert.Contains(t, string(out), rfwdDropMarker)
	testifyassert.NotContains(t, string(out), rfwdErrMarker)
}

// startChild runs one child relay and returns the id it announced, leaving it alive.
func startChild(t *testing.T, ctx context.Context, dir, idBase string) string {
	t.Helper()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", childScript(dir))
	cmd.Env = append(os.Environ(), childIDBaseEnv+"="+idBase)
	// The accepted socket is the child's stdin; hold it open so the child stays up.
	stdin, err := cmd.StdinPipe()
	testifyassert.NoError(t, err)
	t.Cleanup(func() { _ = stdin.Close() })
	stderr, err := cmd.StderrPipe()
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	announcements := make(chan string, 8)
	go scanLines(stderr, announcements)
	select {
	case line := <-announcements:
		conn, ok := parseConnAnnouncement(line)
		testifyassert.Truef(t, ok, "unexpected child output: %s", line)
		return conn.id
	case <-time.After(30 * time.Second):
		t.Fatal("the child never announced itself")
		return ""
	}
}

// TestChildScriptClaimsAnIdNoSiblingHolds pins that two children cannot end up on the
// same rendezvous path. The id started life as the child's PID, which is unique among
// live processes but comes back once one exits - and a returning id would have the
// new child delete and re-create a path an older connection was still announced
// under, delivering one connection's bytes to the other's channel.
func TestChildScriptClaimsAnIdNoSiblingHolds(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat is not installed")
	}

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	first := startChild(t, ctx, dir, "4242")
	testifyassert.Equal(t, "4242", first)

	// A second child that wants the same id has to take another one.
	second := startChild(t, ctx, dir, "4242")
	testifyassert.NotEqual(t, first, second, "two children claimed the same rendezvous id")

	// Both are reachable, each at its own path.
	for _, id := range []string{first, second} {
		info, err := os.Stat(filepath.Join(dir, id, "s"))
		testifyassert.NoErrorf(t, err, "no rendezvous socket for id %s", id)
		if err == nil {
			testifyassert.NotEqual(t, 0, info.Mode()&os.ModeSocket, "id %s is not a socket", id)
		}
	}
}

// TestExecPodListenerSurvivesADroppedConnection pins the difference between "this
// listener never came up" and "this one connection could not be handed over". Both
// are reported by the relay on the same stream, and treating the second as the first
// would take a user's whole forward down because one connection went wrong.
func TestExecPodListenerSurvivesADroppedConnection(t *testing.T) {
	execs := stubPodExec(t)

	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	acceptor := execFor(t, execs, "acceptor")
	acceptor.announce(t, rfwdDropMarker+" the relay socket for 4242 never appeared")
	acceptor.announce(t, rfwdConnMarker+" 4243 10.0.0.9 51234")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := listener.Accept(ctx)
	testifyassert.NoError(t, err, "one dropped connection took the whole forward down")
	if err == nil {
		testifyassert.Equal(t, uint32(51234), conn.OriginPort())
		testifyassert.NoError(t, conn.Close())
	}
}

// TestExecRequestShape pins the exec request the relay is actually launched with.
// The rest of the suite replaces newPodExecutor, so without this the container, the
// command and the stdin flag would only ever be checked against a real API server.
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

	url := l.execRequest("echo relay", true).URL()
	testifyassert.Contains(t, url.Path, "/namespaces/ns/pods/pod-0/exec")
	query := url.Query()
	testifyassert.Equal(t, "main", query.Get("container"))
	// The acceptor learns the session ended from its stdin closing, so the exec has
	// to carry one; stdout and stderr carry the readiness and connection markers.
	testifyassert.Equal(t, "true", query.Get("stdin"))
	testifyassert.Equal(t, "true", query.Get("stdout"))
	testifyassert.Equal(t, "true", query.Get("stderr"))
	// A false flag is left out of the query rather than spelled out.
	testifyassert.Empty(t, query.Get("tty"))
	testifyassert.Equal(t, []string{"/bin/sh", "-c", "echo relay"}, query["command"])

	// A relay that does not need stdin must not be given one.
	testifyassert.Empty(t, l.execRequest("echo relay", false).URL().Query().Get("stdin"))

	executor, err := l.newExecutor("echo relay", true)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, executor)
}

// TestRelayLogWriter verifies relay diagnostics are absorbed rather than treated as
// stream errors: a short write would make the exec stream fail mid-connection.
func TestRelayLogWriter(t *testing.T) {
	w := newLogWriter("pod-0")
	for _, chunk := range []string{"socat: warning\n", "   \n", ""} {
		n, err := w.Write([]byte(chunk))
		testifyassert.NoError(t, err)
		testifyassert.Equal(t, len(chunk), n, "a short write would abort the exec stream")
	}
}

// TestExecPodListenerReportsPodSideFailure pins the consumer of the error marker the
// relay script emits. Without it a pod that cannot bind - no socat in the image, or
// the port already taken - would leave the caller waiting out the readiness timeout
// instead of being told what went wrong.
func TestExecPodListenerReportsPodSideFailure(t *testing.T) {
	stubPodExec(t, &acceptorOutput{stderr: rfwdErrMarker + " failed to listen on 127.0.0.1:10001"})

	start := time.Now()
	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "failed to listen on 127.0.0.1:10001")
	testifyassert.Less(t, time.Since(start), listenerReadyTimeout,
		"the failure must be reported, not waited out")
}

// TestExecPodListenerReportsRelayExit covers the other way a listener never comes up:
// the exec stream ends before the relay reports itself ready.
func TestExecPodListenerReportsRelayExit(t *testing.T) {
	stubPodExec(t, &acceptorOutput{exitErr: errSSH("container terminated")})

	_, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "container terminated")
}

// TestExecPodListenerIgnoresRelayChatter verifies ordinary relay output is logged and
// dropped rather than mistaken for a marker.
func TestExecPodListenerIgnoresRelayChatter(t *testing.T) {
	stubPodExec(t, &acceptorOutput{stderr: "socat: W address is opened in read-write mode", stdout: rfwdReadyMarker})

	listener, err := newExecPodListener(context.Background(),
		&UserInfo{Namespace: "ns", Pod: "pod-0", Container: "main"}, nil, "127.0.0.1", 10001)
	testifyassert.NoError(t, err)
	if err == nil {
		testifyassert.NoError(t, listener.Close())
	}
}
