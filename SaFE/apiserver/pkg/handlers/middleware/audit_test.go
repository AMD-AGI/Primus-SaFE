/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	testifyassert "github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/assert"
)

func TestInferAction(t *testing.T) {
	tests := []struct {
		method   string
		expected string
	}{
		{"POST", "create"},
		{"DELETE", "delete"},
		{"PATCH", "update"},
		{"PUT", "replace"},
		{"GET", "get"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := inferAction(tt.method)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInvalidTraceId(t *testing.T) {
	tests := []struct {
		traceId  string
		expected bool
	}{
		{"", true},
		{"00000000000000000000000000000000", true},
		{"0000", true},
		{"abc123def456", false},
		{"00000000000000000000000000000001", false},
	}

	for _, tt := range tests {
		t.Run(tt.traceId, func(t *testing.T) {
			result := isInvalidTraceId(tt.traceId)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeBody(t *testing.T) {
	// Note: sanitizeBody replaces the entire "field": "value" with "[REDACTED]"
	// It uses regex patterns: "password"\s*:\s*"[^"]*" -> "[REDACTED]"
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_body",
			input:    "",
			expected: "",
		},
		{
			name:     "no_sensitive_data",
			input:    `{"name": "test", "value": 123}`,
			expected: `{"name": "test", "value": 123}`,
		},
		{
			name:     "password_field",
			input:    `{"username": "admin", "password": "secret123"}`,
			expected: `{"username": "admin", "[REDACTED]"}`,
		},
		{
			name:     "apiKey_field",
			input:    `{"name": "test", "apiKey": "ak-` + `xxxxx"}`,
			expected: `{"name": "test", "[REDACTED]"}`,
		},
		{
			name:     "api_key_field",
			input:    `{"name": "test", "api_key": "ak-` + `xxxxx"}`,
			expected: `{"name": "test", "[REDACTED]"}`,
		},
		{
			name:     "privateKey_field",
			input:    `{"githubAuth": {"type": "github_app", "appId": "123", "installationId": "456", "privateKey": "pem-data"}}`,
			expected: `{"githubAuth": {"type": "github_app", "appId": "123", "installationId": "456", "[REDACTED]"}}`,
		},
		{
			name:     "private_key_field",
			input:    `{"name": "test", "private_key": "pem-data"}`,
			expected: `{"name": "test", "[REDACTED]"}`,
		},
		{
			name:     "github_app_private_key_field",
			input:    `{"github_app_id": "123", "github_app_private_key": "pem-data"}`,
			expected: `{"github_app_id": "123", "[REDACTED]"}`,
		},
		{
			name:     "token_field",
			input:    `{"userId": "123", "token": "jwt-token-here"}`,
			expected: `{"userId": "123", "[REDACTED]"}`,
		},
		{
			name:     "secret_field",
			input:    `{"name": "mysecret", "secret": "super-secret"}`,
			expected: `{"name": "mysecret", "[REDACTED]"}`,
		},
		{
			name:     "multiple_sensitive_fields",
			input:    `{"password": "pass1", "token": "tok1", "apiKey": "key1"}`,
			expected: `{"[REDACTED]", "[REDACTED]", "[REDACTED]"}`,
		},
		{
			name:     "password_with_spaces",
			input:    `{"password" : "secret"}`,
			expected: `{"[REDACTED]"}`,
		},
		{
			name:     "case_sensitive_password_lowercase",
			input:    `{"password": "secret"}`,
			expected: `{"[REDACTED]"}`,
		},
		{
			name:     "case_sensitive_PASSWORD_uppercase_not_matched",
			input:    `{"PASSWORD": "secret"}`,
			expected: `{"PASSWORD": "secret"}`, // regex is case-sensitive
		},
		{
			name:     "form_data_password",
			input:    `name=admin&password=` + "secret123&type=default",
			expected: `name=admin&password=` + "[REDACTED]&type=default",
		},
		{
			name:     "form_data_password_at_start",
			input:    `password=` + "secret123&name=admin",
			expected: `password=` + "[REDACTED]&name=admin",
		},
		{
			name:     "form_data_token",
			input:    `userId=123&token=` + `jwt-token&action=login`,
			expected: `userId=123&token=[REDACTED]&action=login`,
		},
		{
			name:     "form_data_privateKey",
			input:    `type=github_app&privateKey=` + `pem-data&appId=123`,
			expected: `type=github_app&privateKey=[REDACTED]&appId=123`,
		},
		{
			name:     "form_data_private_key",
			input:    `type=github_app&private_key=` + `pem-data&appId=123`,
			expected: `type=github_app&private_key=[REDACTED]&appId=123`,
		},
		{
			name:     "form_data_github_app_private_key",
			input:    `type=github_app&github_app_private_key=` + `pem-data&appId=123`,
			expected: `type=github_app&github_app_private_key=[REDACTED]&appId=123`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBody(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short_string",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact_length",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "truncated",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...(truncated)",
		},
		{
			name:     "empty_string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "zero_max_length",
			input:    "hello",
			maxLen:   0,
			expected: "...(truncated)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- merged from audit_buffer_test.go ---

func TestAuditResponseWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	arw := &auditResponseWriter{ResponseWriter: c.Writer, body: bytes.NewBufferString("")}

	n, err := arw.Write([]byte("hello"))
	testifyassert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", arw.body.String())

	// Writing beyond maxAuditBodySize only captures up to the cap.
	big := strings.Repeat("x", maxAuditBodySize+100)
	_, err = arw.Write([]byte(big))
	testifyassert.NoError(t, err)
	assert.Equal(t, maxAuditBodySize, arw.body.Len())
}

func TestAuditBufferSend(t *testing.T) {
	buf := &auditLogBuffer{ch: make(chan *dbclient.AuditLog, 1)}
	// First send fits in the buffer.
	testifyassert.True(t, buf.send(&dbclient.AuditLog{UserId: "u1"}))
	// Second send finds the buffer full and is dropped.
	testifyassert.False(t, buf.send(&dbclient.AuditLog{UserId: "u2"}))
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
	testifyassert.NotNil(t, buf)
	testifyassert.NotNil(t, buf.ch)
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

// --- merged from audit_helpers_test.go ---

func TestGetUserFromK8sNoInternalAuth(t *testing.T) {
	// InternalAuth singleton is not initialized in unit tests, so the lookup
	// returns nil rather than panicking.
	testifyassert.Nil(t, getUserFromK8s(context.Background(), "u1"))
}

// --- merged from audit_writebatch_test.go ---

func TestWriteBatchSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().BatchInsertAuditLogs(gomock.Any(), gomock.Any()).Return(nil)

	b := &auditLogBuffer{client: m}
	b.writeBatch([]*dbclient.AuditLog{{UserId: "u1"}})
}

func TestWriteBatchFallbackToIndividual(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// Batch insert fails -> fall back to individual inserts (one ok, one error).
	m.EXPECT().BatchInsertAuditLogs(gomock.Any(), gomock.Any()).Return(errors.New("batch failed"))
	m.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).Return(nil)
	m.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).Return(errors.New("insert failed"))

	b := &auditLogBuffer{client: m}
	b.writeBatch([]*dbclient.AuditLog{{UserId: "u1"}, {UserId: "u2"}})
}

func TestWriteBatchEmpty(t *testing.T) {
	b := &auditLogBuffer{}
	b.writeBatch(nil)
}
