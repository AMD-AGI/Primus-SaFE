/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

var (
	// EndOfTransmission is the end-of-transmission character for websocket close.
	EndOfTransmission = "\u0004"
)

// NewSessionInfo creates a new SessionInfo instance for SSH or WebShell session.
func (h *SshHandler) NewSessionInfo(userInfo *UserInfo, userConn Conn, rows, cols int, sshType SshType, isPty bool) *SessionInfo {
	return &SessionInfo{
		sshType:  sshType,
		size:     make(chan *remotecommand.TerminalSize, 10),
		userConn: userConn,
		userInfo: userInfo,
		rows:     rows,
		cols:     cols,
		isPty:    isPty,
	}
}

// WebShell handles the websocket connection for web shell access to a pod.
func (h *SshHandler) WebShell(c *gin.Context) {
	req := &WebShellRequest{
		NameSpace: c.Query("namespace"),
		Rows:      c.DefaultQuery("rows", "1800"),
		Cols:      c.DefaultQuery("cols", "40"),
		Container: c.DefaultQuery("container", "main"),
		CMD:       c.DefaultQuery("cmd", "sh"),
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		klog.Errorf("fail to upgrade websocket err: %v", err)
		return
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(appData string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	userInfo := &UserInfo{
		User:      c.GetString(common.UserId),
		Pod:       strings.TrimSpace(c.Param(common.PodId)),
		Container: req.Container,
		CMD:       req.CMD,
		Namespace: req.NameSpace,
	}
	rows, _ := strconv.Atoi(req.Rows)
	cols, _ := strconv.Atoi(req.Cols)
	wsConn := newWebsocketConn(conn).(*WebsocketConn)
	sessionInfo := h.NewSessionInfo(userInfo, wsConn, rows, cols, WebShell, true)

	// Start ping goroutine using WebsocketConn to avoid concurrent writes
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := wsConn.WritePing([]byte("PING")); err != nil {
					klog.Errorf("write ping err: %v", err)
					return
				}
			case <-c.Request.Context().Done():
				return
			}
		}
	}()
	if err := h.SessionConn(c.Request.Context(), sessionInfo); err != nil {
		klog.Errorf("session conn err: %v", err)
	}

	if err := wsConn.WriteCloseMessage(websocket.CloseNormalClosure, "bye"); err != nil {
		klog.Errorf("close websocket err: %v", err)
	}
	time.Sleep(time.Second)
}

// WebsocketConn implements Conn interface for websocket-based sessions.
type WebsocketConn struct {
	conn       *websocket.Conn
	exitReason string
	windowCh   chan *remotecommand.TerminalSize
	closeCh    chan struct{}
	once       sync.Once
	writeMu    sync.Mutex // protects concurrent writes to websocket
}

// newWebsocketConn creates a new WebsocketConn from a websocket.Conn.
func newWebsocketConn(conn *websocket.Conn) Conn {
	return &WebsocketConn{
		conn:     conn,
		windowCh: make(chan *remotecommand.TerminalSize, 10),
		closeCh:  make(chan struct{}),
	}
}

// Read reads data from the websocket connection.
func (conn *WebsocketConn) Read(p []byte) (n int, err error) {
	select {
	case <-conn.closeCh:
		return copy(p, EndOfTransmission), fmt.Errorf("websocket closed")
	default:
	}
	t, msg, erro := conn.conn.ReadMessage()
	if t == websocket.CloseMessage {
		_ = conn.Close()
		return copy(p, EndOfTransmission), fmt.Errorf("websocket CloseMessage")
	}
	_ = conn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	ps := string(msg)
	if strings.HasPrefix(ps, "RESIZE") {
		stringList := strings.Split(ps, " ")
		if len(stringList) == 3 && erro == nil {
			cols, errC := strconv.Atoi(stringList[1])
			rows, errR := strconv.Atoi(stringList[2])
			if errC == nil && errR == nil {
				ev := remotecommand.TerminalSize{
					Width:  uint16(cols),
					Height: uint16(rows),
				}
				conn.windowCh <- &ev
				return copy(p, ""), nil
			}
		}
	}

	if erro == nil {
		n = copy(p, msg)
		return n, nil
	}

	defer conn.Close()
	if websocket.IsUnexpectedCloseError(erro,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseMessageTooBig) {
		klog.Infof("upload stream closed unexpectedly: %v", erro)
		conn.exitReason = fmt.Sprintf("Unexpected close: %v", erro)
		return
	}

	switch {
	case websocket.IsCloseError(erro, websocket.CloseAbnormalClosure):
		conn.exitReason = fmt.Sprintf("Abnormal disconnection on user side: %s", erro)
	case websocket.IsCloseError(erro, websocket.CloseGoingAway):
		conn.exitReason = "User actively disconnected"
	case websocket.IsCloseError(erro, websocket.CloseNormalClosure):
		conn.exitReason = "Normal close"
	default:
		conn.exitReason = fmt.Sprintf("Closed with unhandled reason: %v", erro)
	}
	klog.Infof("upload stream closed normally: %s", conn.exitReason)

	return copy(p, EndOfTransmission), fmt.Errorf("websocket CloseMessage")
}

// Write writes data to the websocket connection.
func (conn *WebsocketConn) Write(p []byte) (n int, err error) {
	select {
	case <-conn.closeCh:
		return 0, fmt.Errorf("websocket closed")
	default:
	}
	conn.writeMu.Lock()
	err = conn.conn.WriteMessage(websocket.BinaryMessage, p)
	conn.writeMu.Unlock()
	return len(p), err
}

// Close closes the websocket connection.
func (conn *WebsocketConn) Close() error {
	conn.once.Do(func() {
		conn.writeMu.Lock()
		_ = conn.conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.writeMu.Unlock()
		close(conn.closeCh)
	})
	return nil
}

// ExitReason returns the reason for session exit.
func (conn *WebsocketConn) ExitReason() string {
	return conn.exitReason
}

// SetExitReason sets the reason for session exit.
func (conn *WebsocketConn) SetExitReason(reason string) {
	conn.exitReason = reason
}

// WindowNotify notifies about terminal window size changes.
func (conn *WebsocketConn) WindowNotify(ctx context.Context, ch chan *remotecommand.TerminalSize) {
	for {
		select {
		case <-ctx.Done():
			return
		case window := <-conn.windowCh:
			ch <- window
		}
	}
}

// ClosedChan returns a channel that is closed when the connection is closed.
func (conn *WebsocketConn) ClosedChan() chan struct{} {
	return conn.closeCh
}

// RawCommand returns the raw command string.
func (conn *WebsocketConn) RawCommand() string {
	return ""
}

// WritePing writes a ping message to the websocket connection.
func (conn *WebsocketConn) WritePing(data []byte) error {
	select {
	case <-conn.closeCh:
		return fmt.Errorf("websocket closed")
	default:
	}
	conn.writeMu.Lock()
	err := conn.conn.WriteMessage(websocket.PingMessage, data)
	conn.writeMu.Unlock()
	return err
}

// WriteCloseMessage writes a close message to the websocket connection.
func (conn *WebsocketConn) WriteCloseMessage(code int, text string) error {
	conn.writeMu.Lock()
	err := conn.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, text))
	conn.writeMu.Unlock()
	return err
}
