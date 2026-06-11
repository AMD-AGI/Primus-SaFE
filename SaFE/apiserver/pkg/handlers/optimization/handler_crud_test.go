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
	assert.Error(t, err)

	k8sClient := ctrlfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	_, err = NewHandler(k8sClient, nil)
	assert.Error(t, err)
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
	assert.Contains(t, rsp.Body.String(), "t1")

	// DB error path.
	h2, mockDB2 := newOptHandler(t, ctrl)
	mockDB2.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).Return(nil, int64(0), errors.New("db down"))
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h2.ListTasks(c2)
	assert.NotEqual(t, http.StatusOK, rsp2.Code)
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
