/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestAuditResponseWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	arw := &auditResponseWriter{ResponseWriter: c.Writer, body: bytes.NewBufferString("")}

	n, err := arw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", arw.body.String())

	// Writing beyond maxAuditBodySize only captures up to the cap.
	big := strings.Repeat("x", maxAuditBodySize+100)
	_, err = arw.Write([]byte(big))
	assert.NoError(t, err)
	assert.Equal(t, maxAuditBodySize, arw.body.Len())
}

func TestAuditBufferSend(t *testing.T) {
	buf := &auditLogBuffer{ch: make(chan *dbclient.AuditLog, 1)}
	// First send fits in the buffer.
	assert.True(t, buf.send(&dbclient.AuditLog{UserId: "u1"}))
	// Second send finds the buffer full and is dropped.
	assert.False(t, buf.send(&dbclient.AuditLog{UserId: "u2"}))
}

func TestAuditBufferWriteBatchEmpty(t *testing.T) {
	// Empty batch returns before touching the (nil) DB client.
	buf := &auditLogBuffer{}
	buf.writeBatch(nil)
	buf.writeBatch([]*dbclient.AuditLog{})
}

func TestFlushWorkerClosedChannel(t *testing.T) {
	// A closed, empty channel makes flushWorker flush nothing and return,
	// so the nil DB client is never dereferenced.
	buf := &auditLogBuffer{ch: make(chan *dbclient.AuditLog)}
	done := make(chan struct{})
	go func() {
		buf.flushWorker()
		close(done)
	}()
	close(buf.ch)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flushWorker did not exit on closed channel")
	}
}

func TestInitAuditBuffer(t *testing.T) {
	buf := initAuditBuffer(nil)
	assert.NotNil(t, buf)
	assert.NotNil(t, buf.ch)
	// Stop the background worker (empty flush, no client access).
	close(buf.ch)
	time.Sleep(50 * time.Millisecond)
}

func TestAuditMiddlewareDBEnabledNoClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// db.enable=true but no real DB -> dbclient.NewClient() returns nil ->
	// Audit returns a passthrough handler.
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "false")

	mw := Audit("workload", "create")
	engine := gin.New()
	engine.POST("/w", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/w", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
