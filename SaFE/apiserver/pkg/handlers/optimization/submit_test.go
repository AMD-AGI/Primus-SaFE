/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestResolveModelForOptimization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// modelID empty -> error.
	mockDB := mock_client.NewMockInterface(ctrl)
	_, err := ResolveModelForOptimization(context.Background(), mockDB, nil, "", "ws")
	assert.Error(t, err)

	// nil db -> error.
	_, err = ResolveModelForOptimization(context.Background(), nil, nil, "m1", "ws")
	assert.Error(t, err)

	// model not found.
	mockDB.EXPECT().GetModelByID(gomock.Any(), "m1").Return(nil, errors.New("missing"))
	_, err = ResolveModelForOptimization(context.Background(), mockDB, nil, "m1", "ws")
	assert.Error(t, err)

	// wrong access mode.
	mockDB.EXPECT().GetModelByID(gomock.Any(), "m2").Return(&dbclient.Model{ID: "m2", AccessMode: "remote_api"}, nil)
	_, err = ResolveModelForOptimization(context.Background(), mockDB, nil, "m2", "ws")
	assert.Error(t, err)

	// not Ready.
	mockDB.EXPECT().GetModelByID(gomock.Any(), "m3").Return(&dbclient.Model{ID: "m3", AccessMode: "local", Phase: "Downloading"}, nil)
	_, err = ResolveModelForOptimization(context.Background(), mockDB, nil, "m3", "ws")
	assert.Error(t, err)

	// success.
	mockDB.EXPECT().GetModelByID(gomock.Any(), "m4").Return(&dbclient.Model{
		ID:          "m4",
		DisplayName: "Model 4",
		AccessMode:  "local",
		Phase:       "Ready",
		LocalPaths:  `[{"workspace":"ws","status":"Ready","path":"/data/m4"}]`,
	}, nil)
	resolved, err := ResolveModelForOptimization(context.Background(), mockDB, nil, "m4", "ws")
	assert.NoError(t, err)
	assert.Equal(t, "/data/m4", resolved.LocalPath)
}

func TestSubmitTaskBranches(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// baseURL empty -> internal error.
	h := &Handler{clawClient: NewClawClient("", ""), hubs: newHubRegistry()}
	_, err := h.submitTask(context.Background(), &CreateTaskRequest{ModelID: "m1"}, "u1", "user", "", "bearer")
	assert.Error(t, err)

	// baseURL set but empty bearer -> unauthorized.
	h2 := &Handler{clawClient: NewClawClient("https://claw", ""), hubs: newHubRegistry()}
	_, err = h2.submitTask(context.Background(), &CreateTaskRequest{ModelID: "m1"}, "u1", "user", "", "")
	assert.Error(t, err)

	// concurrency limit exceeded -> bad request.
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().CountWorkloads(gomock.Any(), gomock.Any()).Return(5, nil)
	h3 := &Handler{
		clawClient:    NewClawClient("https://claw", ""),
		dbClient:      mockDB,
		hubs:          newHubRegistry(),
		maxConcurrent: 3,
		defaultWS:     "ws",
	}
	_, err = h3.submitTask(context.Background(), &CreateTaskRequest{ModelID: "m1"}, "u1", "user", "", "bearer")
	assert.Error(t, err)
}

func TestCreateTaskInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{clawClient: NewClawClient("", ""), hubs: newHubRegistry()}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{not-json"))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateTask(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestRecoverRunningTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Empty list -> no-op.
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)
	h := &Handler{dbClient: mockDB, hubs: newHubRegistry(), clawClient: NewClawClient("", "")}
	h.recoverRunningTasks(context.Background())

	// List error -> no-op (logged).
	mockDB2 := mock_client.NewMockInterface(ctrl)
	mockDB2.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).Return(nil, int64(0), errors.New("db down"))
	h2 := &Handler{dbClient: mockDB2, hubs: newHubRegistry(), clawClient: NewClawClient("", "")}
	h2.recoverRunningTasks(context.Background())

	// Task with no claw session -> marked failed (synchronous).
	mockDB3 := mock_client.NewMockInterface(ctrl)
	mockDB3.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).
		Return([]*dbclient.OptimizationTask{{ID: "t1", ClawSessionID: ""}}, int64(1), nil)
	mockDB3.EXPECT().UpdateOptimizationTaskStatus(
		gomock.Any(), "t1", dbclient.OptimizationTaskStatusFailed, gomock.Any(), gomock.Any()).Return(nil)
	h3 := &Handler{dbClient: mockDB3, hubs: newHubRegistry(), clawClient: NewClawClient("", "")}
	h3.recoverRunningTasks(context.Background())
}
