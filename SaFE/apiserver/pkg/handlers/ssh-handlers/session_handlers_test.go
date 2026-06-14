/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/tools/remotecommand"
)

// fakeChannel implements ssh.Channel for session tests.
type fakeChannel struct {
	written  bytes.Buffer
	reqOK    bool
	reqErr   error
	closed   bool
	readData []byte
	readErr  error
}

func (c *fakeChannel) Read(p []byte) (int, error) {
	if len(c.readData) == 0 {
		if c.readErr != nil {
			return 0, c.readErr
		}
		return 0, io.EOF
	}
	n := copy(p, c.readData)
	c.readData = c.readData[n:]
	return n, nil
}
func (c *fakeChannel) Write(p []byte) (int, error) { return c.written.Write(p) }
func (c *fakeChannel) Close() error                { c.closed = true; return nil }
func (c *fakeChannel) CloseWrite() error           { return nil }
func (c *fakeChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return c.reqOK, c.reqErr
}
func (c *fakeChannel) Stderr() io.ReadWriter { return &bytes.Buffer{} }

// newSession builds a session backed by the given fake channel.
func newTestSession(ch *fakeChannel) *session {
	return &session{
		Channel: ch,
		ctx:     context.Background(),
		handler: func(Session) {},
		subsystemHandlers: map[string]SubsystemHandler{
			"sftp": func(Session) {},
		},
	}
}

func TestSessionWrite(t *testing.T) {
	ch := &fakeChannel{}
	s := newTestSession(ch)

	// No pty: passthrough.
	n, err := s.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)

	// With pty: newline normalization.
	s.pty = &Pty{Term: "xterm"}
	ch.written.Reset()
	n, err = s.Write([]byte("a\nb"))
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, "a\r\nb", ch.written.String())
}

func TestSessionContextPtyRawCommand(t *testing.T) {
	s := newTestSession(&fakeChannel{})
	assert.NotNil(t, s.Context())

	// No pty.
	_, _, ok := s.Pty()
	assert.False(t, ok)

	// With pty.
	s.pty = &Pty{Term: "xterm"}
	s.winch = make(chan Window, 1)
	pty, _, ok := s.Pty()
	assert.True(t, ok)
	assert.Equal(t, "xterm", pty.Term)

	s.rawCmd = "ls -la"
	assert.Equal(t, "ls -la", s.RawCommand())
}

func TestSessionExit(t *testing.T) {
	ch := &fakeChannel{reqOK: true}
	s := newTestSession(ch)
	assert.NoError(t, s.Exit(0))
	assert.True(t, ch.closed)

	// Second exit returns error.
	assert.Error(t, s.Exit(0))
}

func TestSessionHandleEnvRequest(t *testing.T) {
	s := newTestSession(&fakeChannel{})
	var kv struct{ Key, Value string }
	kv.Key, kv.Value = "FOO", "bar"
	req := &ssh.Request{Type: sshReqEnv, Payload: ssh.Marshal(&kv)}
	s.handleEnvRequest(req)
	assert.Contains(t, s.env, "FOO=bar")
}

func TestSessionHandleSignalRequest(t *testing.T) {
	// Buffered when no signal channel.
	s := newTestSession(&fakeChannel{})
	var p struct{ Signal string }
	p.Signal = "TERM"
	req := &ssh.Request{Type: sshReqSignal, Payload: ssh.Marshal(&p)}
	s.handleSignalRequest(req)
	assert.Len(t, s.signalBuf, 1)

	// Forwarded to channel when set.
	sigCh := make(chan Signal, 1)
	s2 := newTestSession(&fakeChannel{})
	s2.signalCh = sigCh
	s2.handleSignalRequest(req)
	assert.Equal(t, Signal("TERM"), <-sigCh)

	// Malformed payload -> reply false, no panic.
	s.handleSignalRequest(&ssh.Request{Type: sshReqSignal, Payload: []byte{0xff}})
}

func TestSessionHandlePtyAndWindowChange(t *testing.T) {
	s := newTestSession(&fakeChannel{})
	payload := append(sshString("xterm"), append(sshUint32(100), sshUint32(40)...)...)
	s.handlePtyRequest(&ssh.Request{Type: sshReqPty, Payload: payload})
	assert.NotNil(t, s.pty)
	assert.Equal(t, 100, s.pty.Window.Width)
	// Drain the initial window pushed by the pty request (winch has cap 1).
	<-s.winch

	// Window change updates pty.
	winPayload := append(sshUint32(120), sshUint32(50)...)
	s.handleWindowChangeRequest(&ssh.Request{Type: sshReqWindowChange, Payload: winPayload})
	assert.Equal(t, 120, s.pty.Window.Width)
	<-s.winch

	// Pty already set -> reply false.
	s.handlePtyRequest(&ssh.Request{Type: sshReqPty, Payload: payload})

	// Malformed pty payload.
	s3 := newTestSession(&fakeChannel{})
	s3.handlePtyRequest(&ssh.Request{Type: sshReqPty, Payload: []byte{0, 0}})
	assert.Nil(t, s3.pty)

	// Window change without pty -> reply false.
	s4 := newTestSession(&fakeChannel{})
	s4.handleWindowChangeRequest(&ssh.Request{Type: sshReqWindowChange, Payload: winPayload})
}

func TestSessionHandleBreakRequest(t *testing.T) {
	// No break channel -> reply false.
	s := newTestSession(&fakeChannel{})
	s.handleBreakRequest(&ssh.Request{Type: sshReqBreak})

	// With break channel.
	breakCh := make(chan bool, 1)
	s2 := newTestSession(&fakeChannel{})
	s2.breakCh = breakCh
	s2.handleBreakRequest(&ssh.Request{Type: sshReqBreak})
	assert.True(t, <-breakCh)
}

func TestSessionHandleSubsystemRequest(t *testing.T) {
	// Unknown subsystem and no default -> reply false.
	s := newTestSession(&fakeChannel{})
	var p struct{ Value string }
	p.Value = "unknown"
	s.handleSubsystemRequest(&ssh.Request{Type: sshReqSubsystem, Payload: ssh.Marshal(&p)})
	assert.False(t, s.handled)

	// Already handled -> reply false.
	s2 := newTestSession(&fakeChannel{})
	s2.handled = true
	s2.handleSubsystemRequest(&ssh.Request{Type: sshReqSubsystem, Payload: ssh.Marshal(&p)})

	// Malformed payload.
	s3 := newTestSession(&fakeChannel{})
	s3.handleSubsystemRequest(&ssh.Request{Type: sshReqSubsystem, Payload: []byte{0xff}})
}

func TestSessionHandleShellOrExecRequest(t *testing.T) {
	// Already handled -> reply false.
	s := newTestSession(&fakeChannel{})
	s.handled = true
	s.handleShellOrExecRequest(&ssh.Request{Type: sshReqShell})

	// Exec request triggers handler goroutine then exit.
	ch := &fakeChannel{reqOK: true}
	done := make(chan struct{})
	s2 := newTestSession(ch)
	s2.handler = func(Session) { close(done) }
	var p struct{ Value string }
	p.Value = "ls"
	s2.handleShellOrExecRequest(&ssh.Request{Type: sshReqExec, Payload: ssh.Marshal(&p)})
	assert.Equal(t, "ls", s2.rawCmd)
	<-done
	assert.Eventually(t, func() bool { return ch.closed }, time.Second, 10*time.Millisecond)

	// Malformed exec payload -> reply false.
	s3 := newTestSession(&fakeChannel{})
	s3.handleShellOrExecRequest(&ssh.Request{Type: sshReqExec, Payload: []byte{0xff}})
}

func TestSessionHandleRequests(t *testing.T) {
	s := newTestSession(&fakeChannel{})
	s.pty = &Pty{}
	s.winch = make(chan Window, 1)

	reqs := make(chan *ssh.Request, 8)
	var env struct{ Key, Value string }
	env.Key, env.Value = "A", "B"
	reqs <- &ssh.Request{Type: sshReqEnv, Payload: ssh.Marshal(&env)}
	reqs <- &ssh.Request{Type: sshReqAgentForward}
	reqs <- &ssh.Request{Type: "unknown-type"}
	close(reqs)

	s.handleRequests(reqs)
	assert.Contains(t, s.env, "A=B")
	// winch should be closed.
	_, ok := <-s.winch
	assert.False(t, ok)
}

// fakeSession implements Session for SSHConn tests.
type fakeSession struct {
	*fakeChannel
	rawCmd string
	pty    *Pty
	winch  chan Window
	hasPty bool
}

func (f *fakeSession) User() string                { return "u1" }
func (f *fakeSession) Context() context.Context    { return context.Background() }
func (f *fakeSession) RawCommand() string          { return f.rawCmd }
func (f *fakeSession) Pty() (Pty, <-chan Window, bool) {
	if f.hasPty {
		return *f.pty, f.winch, true
	}
	return Pty{}, nil, false
}

func TestSSHConnReadWrite(t *testing.T) {
	// Normal read.
	fs := &fakeSession{fakeChannel: &fakeChannel{readData: []byte("data")}}
	conn := newSSHConn(fs)
	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)

	// Write passthrough.
	n, err = conn.Write([]byte("xyz"))
	assert.NoError(t, err)
	assert.Equal(t, 3, n)

	// Write after close.
	_ = conn.Close()
	_, err = conn.Write([]byte("a"))
	assert.Error(t, err)
	_, err = conn.Read(buf)
	assert.Error(t, err)
}

func TestSSHConnReadEOFScp(t *testing.T) {
	fs := &fakeSession{fakeChannel: &fakeChannel{readErr: io.EOF}, rawCmd: "scp -t /tmp"}
	conn := newSSHConn(fs)
	buf := make([]byte, 4)
	_, err := conn.Read(buf)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, "SCP transfer completed", conn.ExitReason())
	assert.Equal(t, "scp -t /tmp", conn.RawCommand())
}

func TestSSHConnWindowNotify(t *testing.T) {
	// No pty -> returns immediately.
	fs := &fakeSession{fakeChannel: &fakeChannel{}}
	conn := newSSHConn(fs)
	conn.WindowNotify(context.Background(), make(chan *remotecommand.TerminalSize, 1))

	// With pty -> forwards then exits on ctx cancel.
	winch := make(chan Window, 1)
	winch <- Window{Width: 80, Height: 24}
	fs2 := &fakeSession{fakeChannel: &fakeChannel{}, hasPty: true, pty: &Pty{}, winch: winch}
	conn2 := newSSHConn(fs2)
	out := make(chan *remotecommand.TerminalSize, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	conn2.WindowNotify(ctx, out)
	select {
	case sz := <-out:
		assert.Equal(t, uint16(80), sz.Width)
	default:
	}
}
