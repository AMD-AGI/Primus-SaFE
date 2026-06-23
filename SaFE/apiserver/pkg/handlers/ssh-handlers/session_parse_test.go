/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/remotecommand"
)

// sshString encodes an SSH binary string: 4-byte length prefix + bytes.
func sshString(s string) []byte {
	b := make([]byte, 4+len(s))
	binary.BigEndian.PutUint32(b, uint32(len(s)))
	copy(b[4:], s)
	return b
}

// sshUint32 encodes a big-endian uint32.
func sshUint32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func TestParseString(t *testing.T) {
	s, rest, ok := parseString(append(sshString("xterm"), 1, 2, 3))
	assert.True(t, ok)
	assert.Equal(t, "xterm", s)
	assert.Equal(t, []byte{1, 2, 3}, rest)

	// Too short.
	_, _, ok = parseString([]byte{0, 0})
	assert.False(t, ok)

	// Length exceeds buffer.
	_, _, ok = parseString([]byte{0, 0, 0, 10, 'a'})
	assert.False(t, ok)
}

func TestParseUint32(t *testing.T) {
	v, rest, ok := parseUint32(append(sshUint32(42), 9))
	assert.True(t, ok)
	assert.Equal(t, uint32(42), v)
	assert.Equal(t, []byte{9}, rest)

	_, _, ok = parseUint32([]byte{0, 1})
	assert.False(t, ok)
}

func TestParsePtyRequest(t *testing.T) {
	payload := append(sshString("xterm-256color"), append(sshUint32(120), sshUint32(40)...)...)
	pty, ok := parsePtyRequest(payload)
	assert.True(t, ok)
	assert.Equal(t, "xterm-256color", pty.Term)
	assert.Equal(t, 120, pty.Window.Width)
	assert.Equal(t, 40, pty.Window.Height)

	// Malformed (no term string).
	_, ok = parsePtyRequest([]byte{0, 0})
	assert.False(t, ok)
}

func TestParseWinchRequest(t *testing.T) {
	w, ok := parseWinchRequest(append(sshUint32(80), sshUint32(24)...))
	assert.True(t, ok)
	assert.Equal(t, 80, w.Width)
	assert.Equal(t, 24, w.Height)

	// Zero width rejected.
	_, ok = parseWinchRequest(append(sshUint32(0), sshUint32(24)...))
	assert.False(t, ok)

	// Too short.
	_, ok = parseWinchRequest([]byte{0, 1})
	assert.False(t, ok)
}

func TestNewSessionInfoAndNext(t *testing.T) {
	h := &SshHandler{}
	info := h.NewSessionInfo(&UserInfo{User: "u1"}, &WebsocketConn{}, 40, 120, WebShell, true)
	assert.NotNil(t, info)

	// Push a size then read it via Next.
	sz := &remotecommand.TerminalSize{Width: 100, Height: 50}
	info.size <- sz
	got := info.Next()
	assert.Equal(t, sz, got)

	// Closed channel -> nil.
	close(info.size)
	assert.Nil(t, info.Next())
}

func TestWebsocketConnAccessors(t *testing.T) {
	conn := &WebsocketConn{
		windowCh: make(chan *remotecommand.TerminalSize, 1),
		closeCh:  make(chan struct{}),
	}
	conn.SetExitReason("done")
	assert.Equal(t, "done", conn.ExitReason())
	assert.Equal(t, "", conn.RawCommand())
	assert.NotNil(t, conn.ClosedChan())

	// WindowNotify returns when ctx is cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn.WindowNotify(ctx, make(chan *remotecommand.TerminalSize, 1))

	// WritePing / WriteCloseMessage on a closed conn return errors without
	// touching the (nil) underlying websocket.
	close(conn.closeCh)
	assert.Error(t, conn.WritePing([]byte("ping")))
	assert.Error(t, conn.WriteCloseMessage(1000, "bye"))
}

func TestSSHConnAccessors(t *testing.T) {
	conn := &SSHConn{closeCh: make(chan struct{})}
	conn.SetExitReason("reason")
	assert.Equal(t, "reason", conn.ExitReason())
	assert.NotNil(t, conn.ClosedChan())

	// Close is idempotent and closes the channel.
	assert.NoError(t, conn.Close())
	assert.NoError(t, conn.Close())

	// Write after close returns an error without touching the session.
	_, err := conn.Write([]byte("data"))
	assert.Error(t, err)
}

func TestInitWebShellRouters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	InitWebShellRouters(engine, &SshHandler{})
	assert.NotEmpty(t, engine.Routes())
}
