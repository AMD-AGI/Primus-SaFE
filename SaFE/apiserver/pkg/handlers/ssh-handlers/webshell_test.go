/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWS establishes a real websocket pair and returns the server-side
// WebsocketConn, the client connection, and a cleanup function.
func setupWS(t *testing.T) (*WebsocketConn, *websocket.Conn, func()) {
	t.Helper()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- c
	}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	serverConn := <-connCh
	wc := newWebsocketConn(serverConn).(*WebsocketConn)
	cleanup := func() {
		_ = client.Close()
		_ = serverConn.Close()
		srv.Close()
	}
	return wc, client, cleanup
}

func TestWebsocketConnReadNormal(t *testing.T) {
	wc, client, cleanup := setupWS(t)
	defer cleanup()

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte("hello")))
	buf := make([]byte, 32)
	n, err := wc.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))
}

func TestWebsocketConnReadResize(t *testing.T) {
	wc, client, cleanup := setupWS(t)
	defer cleanup()

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte("RESIZE 100 40")))
	buf := make([]byte, 32)
	n, err := wc.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	select {
	case ev := <-wc.windowCh:
		assert.Equal(t, uint16(100), ev.Width)
		assert.Equal(t, uint16(40), ev.Height)
	case <-time.After(time.Second):
		t.Fatal("expected window resize event")
	}
}

func TestWebsocketConnWrite(t *testing.T) {
	wc, client, cleanup := setupWS(t)
	defer cleanup()

	n, err := wc.Write([]byte("output"))
	assert.NoError(t, err)
	assert.Equal(t, 6, n)

	mt, msg, err := client.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, mt)
	assert.Equal(t, "output", string(msg))
}

func TestWebsocketConnPingAndClose(t *testing.T) {
	wc, _, cleanup := setupWS(t)
	defer cleanup()

	assert.NoError(t, wc.WritePing([]byte("PING")))
	assert.NoError(t, wc.WriteCloseMessage(websocket.CloseNormalClosure, "bye"))
	assert.NoError(t, wc.Close())

	// After close, read/write/ping/close all error via closeCh.
	buf := make([]byte, 8)
	_, err := wc.Read(buf)
	assert.Error(t, err)
	_, err = wc.Write([]byte("x"))
	assert.Error(t, err)
	assert.Error(t, wc.WritePing([]byte("x")))
	assert.Error(t, wc.WriteCloseMessage(websocket.CloseNormalClosure, "x"))
}

func TestWebsocketConnReadAfterClientClose(t *testing.T) {
	wc, client, cleanup := setupWS(t)
	defer cleanup()

	// Client sends a normal close; server-side Read should detect closure.
	_ = client.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"))
	_ = client.Close()

	buf := make([]byte, 32)
	_, err := wc.Read(buf)
	assert.Error(t, err)
}
