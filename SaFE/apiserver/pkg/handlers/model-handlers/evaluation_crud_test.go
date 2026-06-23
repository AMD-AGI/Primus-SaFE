/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// evalCtx builds a gin context (with recorder) carrying the given params.
func evalCtx(t *testing.T, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = params
	return c, w
}

func TestGetEvaluationTaskHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", TaskName: "task", Status: dbclient.EvaluationTaskStatusRunning}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.GetEvaluationTask(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetEvaluationTaskHandlerEmptyID(t *testing.T) {
	h := &Handler{}
	c, w := evalCtx(t, nil)
	h.GetEvaluationTask(c)
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestListEvaluationTasksHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().CountEvaluationTasks(gomock.Any(), gomock.Any()).Return(1, nil)
	m.EXPECT().SelectEvaluationTasks(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.EvaluationTask{{TaskId: "t1", TaskName: "task"}}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListEvaluationTasks(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteEvaluationTaskHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// OpsJobId invalid -> no k8s delete.
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1"}, nil)
	m.EXPECT().SetEvaluationTaskDeleted(gomock.Any(), "t1").Return(nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.DeleteEvaluationTask(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestStopEvaluationTaskHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", Status: dbclient.EvaluationTaskStatusRunning}, nil)
	m.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "t1", dbclient.EvaluationTaskStatusCancelled).Return(nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.StopEvaluationTask(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestStopEvaluationTaskHandlerNotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", Status: dbclient.EvaluationTaskStatusSucceeded}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.StopEvaluationTask(c)
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestGetEvaluationReportHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// No report S3 path / nil s3 client -> returns base response.
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{TaskId: "t1", TaskName: "task"}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.GetEvaluationReport(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListAvailableEvalServicesHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), "remote_api", "", false).
		Return([]*dbclient.Model{{ID: "m1", DisplayName: "M", ModelName: "gpt"}}, nil)
	m.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Workload{}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListAvailableEvalServices(c)
	assert.Equal(t, http.StatusOK, w.Code)
}
