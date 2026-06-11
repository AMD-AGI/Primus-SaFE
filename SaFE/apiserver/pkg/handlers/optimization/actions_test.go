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
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// newOptHandlerWithClaw builds a handler whose ClawClient points at the given
// httptest backend and whose DB is a gomock mock.
func newOptHandlerWithClaw(t *testing.T, ctrl *gomock.Controller, clawURL string) (*Handler, *mock_client.MockInterface) {
	t.Helper()
	mockDB := mock_client.NewMockInterface(ctrl)
	k8sClient := ctrlfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	h := &Handler{
		dbClient:   mockDB,
		k8sClient:  k8sClient,
		clawClient: NewClawClient(clawURL, "test-key"),
		hubs:       newHubRegistry(),
	}
	return h, mockDB
}

func TestGetTaskForActionNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandlerWithClaw(t, ctrl, "")
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "missing").Return(nil, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	_, err := h.getTaskForAction(c)
	assert.Error(t, err)
}

func TestInterruptTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	_, srv := newClawTestServer(t)
	h, mockDB := newOptHandlerWithClaw(t, ctrl, srv.URL)
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "t1").
		Return(&dbclient.OptimizationTask{ID: "t1", ClawSessionID: "sess-1"}, nil)
	mockDB.EXPECT().UpdateOptimizationTaskStatus(
		gomock.Any(), "t1", dbclient.OptimizationTaskStatusInterrupted, gomock.Any(), gomock.Any()).Return(nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "t1"}}
	h.InterruptTask(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestInterruptTaskNoSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandlerWithClaw(t, ctrl, "")
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "t1").
		Return(&dbclient.OptimizationTask{ID: "t1", ClawSessionID: ""}, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "t1"}}
	h.InterruptTask(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestRetryTaskNonRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandlerWithClaw(t, ctrl, "")
	// A running task cannot be retried.
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "t1").
		Return(&dbclient.OptimizationTask{ID: "t1", Status: dbclient.OptimizationTaskStatusRunning}, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "t1"}}
	h.RetryTask(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestGetTaskForActionDBError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockDB := newOptHandlerWithClaw(t, ctrl, "")
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), "t1").Return(nil, errors.New("db down"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "t1"}}
	_, err := h.getTaskForAction(c)
	assert.Error(t, err)
}

func TestInitRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Nil handler -> no routes registered.
	engine := gin.New()
	InitRoutes(engine, nil)
	assert.Empty(t, engine.Routes())

	// Non-nil handler -> routes registered.
	engine2 := gin.New()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h, _ := newOptHandlerWithClaw(t, ctrl, "")
	InitRoutes(engine2, h)
	assert.NotEmpty(t, engine2.Routes())
}
