package ssh_handlers

import (
	"context"
	"fmt"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
	"time"
)

var (
	EndOfTransmission = "\u0004"
)

func (h *SshHandler) NewSessionInfo(ctx context.Context, userInfo *UserInfo, userConn Conn, rows, cols int, sshType SshType) *SessionInfo {
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	return &SessionInfo{
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		sshType:    sshType,
		size:       make(chan *remotecommand.TerminalSize, 10),
		userConn:   userConn,
		userInfo:   userInfo,
		rows:       rows,
		cols:       cols,
	}
}

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
		klog.Info("fail to upgrade websocket")
		return
	}

	userInfo := &UserInfo{
		User:      c.GetString(common.UserId),
		Pod:       strings.TrimSpace(c.Param(common.PodId)),
		Container: req.Container,
		CMD:       req.CMD,
		Namespace: req.NameSpace,
	}
	rows, _ := strconv.Atoi(req.Rows)
	cols, _ := strconv.Atoi(req.Cols)
	sessionInfo := h.NewSessionInfo(c.Request.Context(), userInfo, newWebsocketConn(conn), rows, cols, WebShell)
	if err := h.SessionConn(c.Request.Context(), sessionInfo); err != nil {
		klog.Errorf("session conn err: %v", err)
	}

	if err := closeWebSocket(conn); err != nil {
		klog.Errorf("close websocket err: %v", err)
	}

	return
}

func closeWebSocket(conn *websocket.Conn) error {
	// 发送关闭帧
	err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
	if err != nil {
		return err
	}

	// 等待一小段时间让对端收到关闭帧
	time.Sleep(1 * time.Second)

	return conn.Close()
}

type WebsocketConn struct {
	conn       *websocket.Conn
	exitReason string
	windowCh   chan *remotecommand.TerminalSize
}

func newWebsocketConn(conn *websocket.Conn) Conn {
	return &WebsocketConn{
		conn:     conn,
		windowCh: make(chan *remotecommand.TerminalSize, 10),
	}
}

func (conn *WebsocketConn) Read(p []byte) (n int, err error) {
	t, msg, erro := conn.conn.ReadMessage()
	if t == websocket.CloseMessage {
		return copy(p, EndOfTransmission), fmt.Errorf("websocket CloseMessage")
	}

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

	if websocket.IsUnexpectedCloseError(erro,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseMessageTooBig) {
		klog.Infof("upload stream closed unexpectedly: %v", erro)
		conn.exitReason = fmt.Sprintf("Unexpected close: %v", erro)
		return
	}

	// 非意外关闭
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

func (conn *WebsocketConn) Write(p []byte) (n int, err error) {
	err = conn.conn.WriteMessage(websocket.BinaryMessage, p)
	return len(p), err
}

func (conn *WebsocketConn) Close() error {
	_ = conn.conn.WriteMessage(websocket.CloseMessage, []byte{})
	return conn.conn.Close()
}

func (conn *WebsocketConn) ExitReason() string {
	return conn.exitReason
}

func (conn *WebsocketConn) SetExitReason(reason string) {
	conn.exitReason = reason
}

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
