/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// clawHandler builds a Handler whose ClawClient points at the given httptest URL.
func clawHandler(url string) *Handler {
	return &Handler{clawClient: NewClawClient(url, "test-key")}
}

// sessionServer returns a server replying to GET /sessions/{id} with the given
// status/agentStatus JSON.
func sessionServer(t *testing.T, status, agentStatus string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"session_id":"s1","status":"` + status + `","agent_status":"` + agentStatus + `"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveStatusFromClawSucceeded(t *testing.T) {
	srv := sessionServer(t, "completed", "idle")
	h := clawHandler(srv.URL)
	status, _ := h.resolveStatusFromClaw("s1", errors.New("transient"), "bearer", true)
	assert.Equal(t, dbclient.OptimizationTaskStatusSucceeded, status)
}

func TestResolveStatusFromClawInitialIdleStillRunning(t *testing.T) {
	srv := sessionServer(t, "active", "idle")
	h := clawHandler(srv.URL)
	status, msg := h.resolveStatusFromClaw("s1", errors.New("transient"), "bearer", false)
	assert.Equal(t, dbclient.OptimizationTaskStatusRunning, status)
	assert.Empty(t, msg)
}

func TestResolveStatusFromClawFailed(t *testing.T) {
	srv := sessionServer(t, "failed", "failed")
	h := clawHandler(srv.URL)
	status, msg := h.resolveStatusFromClaw("s1", errors.New("transient"), "bearer", false)
	assert.Equal(t, dbclient.OptimizationTaskStatusFailed, status)
	assert.Equal(t, "claw session failed", msg)
}

func TestResolveStatusFromClawStillRunning(t *testing.T) {
	srv := sessionServer(t, "running", "busy")
	h := clawHandler(srv.URL)
	status, _ := h.resolveStatusFromClaw("s1", errors.New("transient"), "bearer", false)
	assert.Equal(t, dbclient.OptimizationTaskStatusRunning, status)
}

func TestResolveStatusFromClawGetSessionError(t *testing.T) {
	// Server returns malformed JSON -> GetSession error -> fallback to failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	h := clawHandler(srv.URL)
	status, _ := h.resolveStatusFromClaw("s1", errors.New("stream broke"), "bearer", false)
	assert.Equal(t, dbclient.OptimizationTaskStatusFailed, status)
}

func TestResolveStatusFromClawCleanEOFGetSessionErrorKeepsRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	h := clawHandler(srv.URL)
	status, msg := h.resolveStatusFromClaw("s1", nil, "bearer", false)
	assert.Equal(t, dbclient.OptimizationTaskStatusRunning, status)
	assert.Empty(t, msg)
}

func TestHasOptimizationReport(t *testing.T) {
	// Report present.
	withReport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"path":"claw-1/optimization_report.md","size":10}]}`))
	}))
	t.Cleanup(withReport.Close)
	assert.True(t, clawHandler(withReport.URL).hasOptimizationReport("s1", "bearer"))

	// No report file.
	noReport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"path":"claw-1/other.txt","size":10}]}`))
	}))
	t.Cleanup(noReport.Close)
	assert.False(t, clawHandler(noReport.URL).hasOptimizationReport("s1", "bearer"))

	// List error -> assume present (true) so transient failures don't flip status.
	listErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(listErr.Close)
	assert.True(t, clawHandler(listErr.URL).hasOptimizationReport("s1", "bearer"))
}
