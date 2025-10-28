/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
	"k8s.io/client-go/tools/remotecommand"
)

// ===========================
// ======== Data Types =======
// ===========================

// Window represents terminal window size.
type Window struct {
	Width  int
	Height int
}

// Pty represents pseudo-terminal request parameters.
type Pty struct {
	Term   string
	Window Window
}

// Signal represents an SSH signal name.
type Signal string

// SubsystemHandler defines the function type for handling SSH subsystem requests.
type SubsystemHandler func(Session)

// Handler defines the function type for handling SSH sessions.
type Handler func(Session)

// ===========================
// ======== Constants ========
// ===========================

const (
	sshReqShell        = "shell"
	sshReqExec         = "exec"
	sshReqSubsystem    = "subsystem"
	sshReqEnv          = "env"
	sshReqSignal       = "signal"
	sshReqPty          = "pty-req"
	sshReqWindowChange = "window-change"
	sshReqBreak        = "break"
	sshReqExitStatus   = "exit-status"

	sshReqAgentForward = "auth-agent-req@openssh.com"

	maxSignalBuffer = 128
)

// ===========================
// ======== Interfaces =======
// ===========================

// Session represents an SSH session.
// It wraps ssh.Channel and provides additional context.
type Session interface {
	ssh.Channel

	// User returns the SSH login username.
	User() string

	// Context returns the session context.
	Context() context.Context

	// Pty returns the pseudo-terminal information, window change channel, and existence flag.
	Pty() (Pty, <-chan Window, bool)

	// RawCommand returns the raw command string.
	RawCommand() string
}

// ===========================
// ======== Implementations ==
// ===========================

// session is the concrete implementation of the Session interface.
type session struct {
	sync.Mutex
	ssh.Channel
	conn              *ssh.ServerConn
	handler           Handler
	subsystemHandlers map[string]SubsystemHandler

	handled   bool
	exited    bool
	pty       *Pty
	winch     chan Window
	env       []string
	rawCmd    string
	subsystem string
	ctx       context.Context
	signalCh  chan<- Signal
	signalBuf []Signal
	breakCh   chan<- bool
}

// ===========================
// ======== Methods ==========
// ===========================

// Write normalizes line breaks for PTY mode output.
func (s *session) Write(p []byte) (int, error) {
	if s.pty == nil {
		return s.Channel.Write(p)
	}

	originalLen := len(p)
	p = bytes.ReplaceAll(p, []byte{'\n'}, []byte{'\r', '\n'})
	p = bytes.ReplaceAll(p, []byte{'\r', '\r', '\n'}, []byte{'\r', '\n'})
	n, err := s.Channel.Write(p)
	if n > originalLen {
		n = originalLen
	}
	return n, err
}

// Context returns the session context.
func (s *session) Context() context.Context {
	return s.ctx
}

// Exit sends the exit status and closes the session.
func (s *session) Exit(code uint32) error {
	s.Lock()
	defer s.Unlock()

	if s.exited {
		return fmt.Errorf("session already exit")
	}
	s.exited = true

	status := struct{ Status uint32 }{code}
	if _, err := s.SendRequest(sshReqExitStatus, false, ssh.Marshal(&status)); err != nil {
		return err
	}
	return s.Close()
}

// User returns the SSH username.
func (s *session) User() string {
	return s.conn.User()
}

// Pty returns pseudo-terminal information.
func (s *session) Pty() (Pty, <-chan Window, bool) {
	if s.pty != nil {
		return *s.pty, s.winch, true
	}
	return Pty{}, s.winch, false
}

// RawCommand returns the raw command string.
func (s *session) RawCommand() string {
	return s.rawCmd
}

// ===========================
// ======== Request Handling ==
// ===========================

// handleRequests processes incoming SSH requests from the client.
func (s *session) handleRequests(reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case sshReqShell, sshReqExec:
			s.handleShellOrExecRequest(req)
		case sshReqSubsystem:
			s.handleSubsystemRequest(req)
		case sshReqEnv:
			s.handleEnvRequest(req)
		case sshReqSignal:
			s.handleSignalRequest(req)
		case sshReqPty:
			s.handlePtyRequest(req)
		case sshReqWindowChange:
			s.handleWindowChangeRequest(req)
		case sshReqAgentForward:
			_ = req.Reply(true, nil)
		case sshReqBreak:
			s.handleBreakRequest(req)
		default:
			_ = req.Reply(false, nil)
		}
	}
	if s.winch != nil {
		close(s.winch)
	}
}

// ===========================
// ======== Sub Handlers =====
// ===========================

// handleShellOrExecRequest handles "shell" or "exec" requests.
func (s *session) handleShellOrExecRequest(req *ssh.Request) {
	if s.handled {
		_ = req.Reply(false, nil)
		return
	}

	// If it's an "exec" request, payload contains a command.
	var cmd string
	if req.Type == sshReqExec {
		var payload struct{ Value string }
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			_ = req.Reply(false, nil)
			return
		}
		cmd = payload.Value
	}

	s.rawCmd = cmd
	s.handled = true
	_ = req.Reply(true, nil)

	go func() {
		s.handler(s)
		_ = s.Exit(0)
	}()
}

// handleSubsystemRequest handles "subsystem" requests.
func (s *session) handleSubsystemRequest(req *ssh.Request) {
	if s.handled {
		_ = req.Reply(false, nil)
		return
	}
	var payload struct{ Value string }
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		_ = req.Reply(false, nil)
		return
	}

	s.subsystem = payload.Value
	handler := s.subsystemHandlers[payload.Value]
	if handler == nil {
		handler = s.subsystemHandlers["default"]
	}
	if handler == nil {
		_ = req.Reply(false, nil)
		return
	}

	s.handled = true
	_ = req.Reply(true, nil)

	go func() {
		handler(s)
		_ = s.Exit(0)
	}()
}

// handleEnvRequest handles environment variable requests.
func (s *session) handleEnvRequest(req *ssh.Request) {
	var kv struct{ Key, Value string }
	if err := ssh.Unmarshal(req.Payload, &kv); err == nil {
		s.env = append(s.env, fmt.Sprintf("%s=%s", kv.Key, kv.Value))
	}
	_ = req.Reply(true, nil)
}

// handleSignalRequest handles SSH signal requests.
func (s *session) handleSignalRequest(req *ssh.Request) {
	var payload struct{ Signal string }
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		_ = req.Reply(false, nil)
		return
	}

	s.Lock()
	defer s.Unlock()
	if s.signalCh != nil {
		s.signalCh <- Signal(payload.Signal)
	} else if len(s.signalBuf) < maxSignalBuffer {
		s.signalBuf = append(s.signalBuf, Signal(payload.Signal))
	}
}

// handlePtyRequest handles PTY requests.
func (s *session) handlePtyRequest(req *ssh.Request) {
	if s.handled || s.pty != nil {
		_ = req.Reply(false, nil)
		return
	}

	ptyReq, ok := parsePtyRequest(req.Payload)
	if !ok {
		_ = req.Reply(false, nil)
		return
	}

	s.pty = &ptyReq
	s.winch = make(chan Window, 1)
	s.winch <- ptyReq.Window
	_ = req.Reply(true, nil)
}

// handleWindowChangeRequest handles terminal window resize requests.
func (s *session) handleWindowChangeRequest(req *ssh.Request) {
	if s.pty == nil {
		_ = req.Reply(false, nil)
		return
	}

	win, ok := parseWinchRequest(req.Payload)
	if ok {
		s.pty.Window = win
		s.winch <- win
	}
	_ = req.Reply(ok, nil)
}

// handleBreakRequest handles SSH "break" signals.
func (s *session) handleBreakRequest(req *ssh.Request) {
	s.Lock()
	defer s.Unlock()

	ok := false
	if s.breakCh != nil {
		s.breakCh <- true
		ok = true
	}
	_ = req.Reply(ok, nil)
}

// ===========================
// ======== Helper Functions ==
// ===========================

// parsePtyRequest parses PTY request payload.
func parsePtyRequest(b []byte) (Pty, bool) {
	term, rest, ok := parseString(b)
	if !ok {
		return Pty{}, false
	}
	width, rest, ok := parseUint32(rest)
	if !ok {
		return Pty{}, false
	}
	height, _, ok := parseUint32(rest)
	if !ok {
		return Pty{}, false
	}
	return Pty{
		Term: term,
		Window: Window{
			Width:  int(width),
			Height: int(height),
		},
	}, true
}

// parseWinchRequest parses window change request payload.
func parseWinchRequest(b []byte) (Window, bool) {
	width, rest, ok := parseUint32(b)
	if !ok || width < 1 {
		return Window{}, false
	}
	height, _, ok := parseUint32(rest)
	if !ok || height < 1 {
		return Window{}, false
	}
	return Window{Width: int(width), Height: int(height)}, true
}

// parseString parses an SSH binary string.
func parseString(b []byte) (string, []byte, bool) {
	if len(b) < 4 {
		return "", nil, false
	}
	length := binary.BigEndian.Uint32(b)
	if uint32(len(b)) < 4+length {
		return "", nil, false
	}
	return string(b[4 : 4+length]), b[4+length:], true
}

// parseUint32 reads a uint32 value from byte stream.
func parseUint32(b []byte) (uint32, []byte, bool) {
	if len(b) < 4 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint32(b), b[4:], true
}

// Next retrieves the next terminal size from the SessionInfo channel.
// It returns a pointer to TerminalSize if available, or nil if the channel is closed.
func (info *SessionInfo) Next() (size *remotecommand.TerminalSize) {
	if v, ok := <-info.size; ok {
		return v
	}
	return nil
}
