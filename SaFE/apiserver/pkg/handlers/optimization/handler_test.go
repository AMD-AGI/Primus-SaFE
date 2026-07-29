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

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	testifyassert "github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
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

// --- merged from handler_crud_test.go ---

func newOptHandler(t *testing.T, ctrl *gomock.Controller) (*Handler, *mock_client.MockInterface) {
	t.Helper()
	mockDB := mock_client.NewMockInterface(ctrl)
	k8sClient := ctrlfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	h := &Handler{
		dbClient:   mockDB,
		k8sClient:  k8sClient,
		clawClient: NewClawClient("", ""),
		hubs:       newHubRegistry(),
	}
	return h, mockDB
}

func TestNewHandlerValidation(t *testing.T) {
	_, err := NewHandler(nil, nil)
	testifyassert.Error(t, err)

	k8sClient := ctrlfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	_, err = NewHandler(k8sClient, nil)
	testifyassert.Error(t, err)
}

func TestListTasksHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandler(t, ctrl)
	mockDB.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).
		Return([]*dbclient.OptimizationTask{{ID: "t1", DisplayName: "opt-1"}}, int64(1), nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ListTasks(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
	testifyassert.Contains(t, rsp.Body.String(), "t1")

	// DB error path.
	h2, mockDB2 := newOptHandler(t, ctrl)
	mockDB2.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).Return(nil, int64(0), errors.New("db down"))
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h2.ListTasks(c2)
	testifyassert.NotEqual(t, http.StatusOK, rsp2.Code)
}

func TestGetTaskHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandler(t, ctrl)
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "t1").
		Return(&dbclient.OptimizationTask{ID: "t1", DisplayName: "opt-1"}, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "t1"}}
	h.GetTask(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Not found (nil task).
	h2, mockDB2 := newOptHandler(t, ctrl)
	mockDB2.EXPECT().GetOptimizationTask(gomock.Any(), "missing").Return(nil, nil)
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Params = gin.Params{{Key: "id", Value: "missing"}}
	h2.GetTask(c2)
	assert.Equal(t, http.StatusNotFound, rsp2.Code)
}

func TestDeleteTaskHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandler(t, ctrl)
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "t1").
		Return(&dbclient.OptimizationTask{ID: "t1"}, nil)
	mockDB.EXPECT().DeleteOptimizationTask(gomock.Any(), "t1").Return(nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "t1"}}
	h.DeleteTask(c)
	assert.Equal(t, http.StatusNoContent, c.Writer.Status())

	// Task not found.
	h2, mockDB2 := newOptHandler(t, ctrl)
	mockDB2.EXPECT().GetOptimizationTask(gomock.Any(), "missing").Return(nil, errors.New("not found"))
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c2.Params = gin.Params{{Key: "id", Value: "missing"}}
	h2.DeleteTask(c2)
	assert.Equal(t, http.StatusNotFound, rsp2.Code)
}
