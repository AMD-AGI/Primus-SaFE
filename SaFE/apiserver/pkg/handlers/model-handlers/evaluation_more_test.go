/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

var assertErr = errors.New("db error")

// TestListAvailableEvalServices verifies remote_api models and local workloads are aggregated.
func TestListAvailableEvalServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), "remote_api", "", false).
		Return([]*dbclient.Model{{ID: "m1", DisplayName: "GPT", ModelName: "gpt", SourceURL: "http://api"}}, nil)
	m.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Workload{{WorkloadId: "w1", DisplayName: "infer"}}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListAvailableEvalServices(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestListAvailableEvalServicesDBErrors verifies the handler still responds when DB calls fail.
func TestListAvailableEvalServicesDBErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, assertErr)
	m.EXPECT().SelectWorkloads(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, assertErr)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, nil)
	h.ListAvailableEvalServices(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetEvaluationReport verifies a task report is rendered (no S3 client -> metadata only).
func TestGetEvaluationReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetEvaluationTask(gomock.Any(), "t1").
		Return(&dbclient.EvaluationTask{
			TaskId:      "t1",
			TaskName:    "task",
			ServiceName: "svc",
			Status:      dbclient.EvaluationTaskStatusSucceeded,
		}, nil)

	h := &Handler{dbClient: m}
	c, w := evalCtx(t, gin.Params{{Key: "id", Value: "t1"}})
	h.GetEvaluationReport(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetEvaluationReportEmptyID verifies the empty-id branch.
func TestGetEvaluationReportEmptyID(t *testing.T) {
	h := &Handler{}
	c, w := evalCtx(t, nil)
	h.GetEvaluationReport(c)
	assert.NotEqual(t, http.StatusOK, w.Code)
}
