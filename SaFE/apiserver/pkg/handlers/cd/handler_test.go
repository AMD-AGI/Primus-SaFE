/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	sqrl "github.com/Masterminds/squirrel"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestHandler(ctrl *gomock.Controller) (*Handler, *mock_client.MockInterface) {
	mockDB := mock_client.NewMockInterface(ctrl)
	fakeK8s := fake.NewSimpleClientset()

	h := &Handler{
		clientSet: fakeK8s,
		dbClient:  mockDB,
		service:   NewService(mockDB, fakeK8s),
	}

	return h, mockDB
}

func TestCreateDeploymentRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("create request with image versions", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		reqBody := CreateDeploymentRequestReq{
			ImageVersions: map[string]string{
				"apiserver": "apiserver:v1.0.0",
			},
			Description: "Test deployment",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockDB.EXPECT().CreateDeploymentRequest(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, req *dbclient.DeploymentRequest) (int64, error) {
				assert.Equal(t, "test-user", req.DeployName)
				assert.Equal(t, StatusPendingApproval, req.Status)
				return 123, nil
			})

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserName, "test-user")

		h.CreateDeploymentRequest(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp CreateDeploymentRequestResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, int64(123), resp.Id)
	})

	t.Run("create request without image versions or env config returns error", func(t *testing.T) {
		h, _ := setupTestHandler(ctrl)

		reqBody := CreateDeploymentRequestReq{
			ImageVersions: map[string]string{}, // Empty
			EnvFileConfig: "",                  // Empty
		}
		bodyBytes, _ := json.Marshal(reqBody)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserName, "test-user")

		h.CreateDeploymentRequest(c)

		assert.Equal(t, http.StatusBadRequest, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "at least one of")
	})

	t.Run("create request with invalid json returns error", func(t *testing.T) {
		h, _ := setupTestHandler(ctrl)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments", bytes.NewReader([]byte("invalid json")))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserName, "test-user")

		h.CreateDeploymentRequest(c)

		assert.Equal(t, http.StatusBadRequest, rsp.Code)
	})
}

func TestListDeploymentRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("list requests with default pagination", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		requests := []*dbclient.DeploymentRequest{
			{
				Id:         1,
				DeployName: "user1",
				Status:     StatusDeployed,
			},
			{
				Id:         2,
				DeployName: "user2",
				Status:     StatusPendingApproval,
			},
		}

		mockDB.EXPECT().ListDeploymentRequests(gomock.Any(), gomock.Any(), []string{"created_at DESC"}, 10, 0).
			Return(requests, nil)
		mockDB.EXPECT().CountDeploymentRequests(gomock.Any(), gomock.Any()).Return(2, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/deployments", nil)

		h.ListDeploymentRequests(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp ListDeploymentRequestsResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.TotalCount)
		assert.Equal(t, 2, len(resp.Items))
	})

	t.Run("list requests with status filter", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		mockDB.EXPECT().ListDeploymentRequests(gomock.Any(), sqrl.Eq{"status": StatusPendingApproval}, gomock.Any(), 10, 0).
			Return([]*dbclient.DeploymentRequest{}, nil)
		mockDB.EXPECT().CountDeploymentRequests(gomock.Any(), sqrl.Eq{"status": StatusPendingApproval}).Return(0, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/deployments?status=pending_approval", nil)

		h.ListDeploymentRequests(c)

		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("list requests with custom pagination", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		mockDB.EXPECT().ListDeploymentRequests(gomock.Any(), gomock.Any(), gomock.Any(), 20, 10).
			Return([]*dbclient.DeploymentRequest{}, nil)
		mockDB.EXPECT().CountDeploymentRequests(gomock.Any(), gomock.Any()).Return(0, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/deployments?limit=20&offset=10", nil)

		h.ListDeploymentRequests(c)

		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestGetDeploymentRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("get existing request", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		config := DeploymentConfig{
			ImageVersions: map[string]string{"apiserver": "v1.0.0"},
			EnvFileConfig: "test=value",
		}
		configJSON, _ := json.Marshal(config)

		req := &dbclient.DeploymentRequest{
			Id:         123,
			DeployName: "test-user",
			Status:     StatusDeployed,
			EnvConfig:  string(configJSON),
		}

		mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(123)).Return(req, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/deployments/123", nil)
		c.Params = gin.Params{{Key: "id", Value: "123"}}

		h.GetDeploymentRequest(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp GetDeploymentRequestResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, int64(123), resp.Id)
		assert.Equal(t, "v1.0.0", resp.ImageVersions["apiserver"])
		assert.Equal(t, "test=value", resp.EnvFileConfig)
	})

	t.Run("get request with invalid id", func(t *testing.T) {
		h, _ := setupTestHandler(ctrl)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/deployments/invalid", nil)
		c.Params = gin.Params{{Key: "id", Value: "invalid"}}

		h.GetDeploymentRequest(c)

		assert.Equal(t, http.StatusBadRequest, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "Invalid ID")
	})
}

func TestApproveDeploymentRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("reject request", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		req := &dbclient.DeploymentRequest{
			Id:         1,
			DeployName: "requester",
			Status:     StatusPendingApproval,
		}

		mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).Return(req, nil)
		mockDB.EXPECT().UpdateDeploymentRequest(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, r *dbclient.DeploymentRequest) error {
				assert.Equal(t, StatusRejected, r.Status)
				assert.Equal(t, "Security issue", r.RejectionReason.String)
				return nil
			})

		approvalReq := ApprovalReq{
			Approved: false,
			Reason:   "Security issue",
		}
		bodyBytes, _ := json.Marshal(approvalReq)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments/1/approve", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserName, "approver")

		h.ApproveDeploymentRequest(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp ApprovalResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, StatusRejected, resp.Status)
	})

	t.Run("cannot approve own request", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		req := &dbclient.DeploymentRequest{
			Id:         1,
			DeployName: "same-user",
			Status:     StatusPendingApproval,
		}

		mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).Return(req, nil)

		approvalReq := ApprovalReq{Approved: true}
		bodyBytes, _ := json.Marshal(approvalReq)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments/1/approve", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserName, "same-user") // Same as DeployName

		h.ApproveDeploymentRequest(c)

		assert.Equal(t, http.StatusForbidden, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "Cannot approve your own request")
	})

	t.Run("cannot approve non-pending request", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		req := &dbclient.DeploymentRequest{
			Id:         1,
			DeployName: "requester",
			Status:     StatusDeployed, // Already deployed
		}

		mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(1)).Return(req, nil)

		approvalReq := ApprovalReq{Approved: true}
		bodyBytes, _ := json.Marshal(approvalReq)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments/1/approve", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Set(common.UserName, "approver")

		h.ApproveDeploymentRequest(c)

		assert.Equal(t, http.StatusBadRequest, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "not pending approval")
	})
}

func TestRollbackDeployment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("rollback to previous version", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		targetReq := &dbclient.DeploymentRequest{
			Id:        10,
			Status:    StatusDeployed,
			EnvConfig: `{"image_versions":{"apiserver":"v1.0.0"}}`,
		}

		snapshot := &dbclient.EnvironmentSnapshot{
			Id:        1,
			EnvConfig: `{"image_versions":{"apiserver":"v1.0.0"},"env_file_config":"test"}`,
		}

		mockDB.EXPECT().GetDeploymentRequest(gomock.Any(), int64(10)).Return(targetReq, nil)
		mockDB.EXPECT().GetEnvironmentSnapshotByRequestId(gomock.Any(), int64(10)).Return(snapshot, nil)
		mockDB.EXPECT().CreateDeploymentRequest(gomock.Any(), gomock.Any()).Return(int64(11), nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments/10/rollback", nil)
		c.Params = gin.Params{{Key: "id", Value: "10"}}
		c.Set(common.UserName, "admin")

		h.RollbackDeployment(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp CreateDeploymentRequestResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, int64(11), resp.Id)
	})

	t.Run("rollback with invalid id", func(t *testing.T) {
		h, _ := setupTestHandler(ctrl)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/cd/deployments/invalid/rollback", nil)
		c.Params = gin.Params{{Key: "id", Value: "invalid"}}
		c.Set(common.UserName, "admin")

		h.RollbackDeployment(c)

		assert.Equal(t, http.StatusBadRequest, rsp.Code)
	})
}

func TestGetCurrentEnvConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("get env config from snapshot", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		config := DeploymentConfig{
			ImageVersions: map[string]string{"apiserver": "v1.0.0"},
			EnvFileConfig: "key=value\nother=123",
		}
		configJSON, _ := json.Marshal(config)

		snapshots := []*dbclient.EnvironmentSnapshot{
			{
				Id:        1,
				EnvConfig: string(configJSON),
			},
		}

		mockDB.EXPECT().ListEnvironmentSnapshots(gomock.Any(), nil, []string{"created_at DESC"}, 1, 0).
			Return(snapshots, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/env-config", nil)

		h.GetCurrentEnvConfig(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp GetCurrentEnvConfigResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "key=value\nother=123", resp.EnvFileConfig)
	})

	t.Run("no snapshot available returns error", func(t *testing.T) {
		h, mockDB := setupTestHandler(ctrl)

		mockDB.EXPECT().ListEnvironmentSnapshots(gomock.Any(), nil, []string{"created_at DESC"}, 1, 0).
			Return([]*dbclient.EnvironmentSnapshot{}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/env-config", nil)

		h.GetCurrentEnvConfig(c)

		assert.Equal(t, http.StatusInternalServerError, rsp.Code)
	})
}

func TestGetDeployableComponents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("get components list", func(t *testing.T) {
		h, _ := setupTestHandler(ctrl)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cd/components", nil)

		h.GetDeployableComponents(c)

		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp GetDeployableComponentsResp
		err := json.Unmarshal(rsp.Body.Bytes(), &resp)
		require.NoError(t, err)
		// In test mode without config, components may be nil or empty
		// Just verify the response structure is valid
	})
}

func TestHandleFunc(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("handle returns success response", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		handle(c, func(c *gin.Context) (interface{}, error) {
			return map[string]string{"status": "ok"}, nil
		})

		assert.Equal(t, http.StatusOK, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "ok")
	})

	t.Run("handle returns byte response", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		handle(c, func(c *gin.Context) (interface{}, error) {
			return []byte(`{"raw":"data"}`), nil
		})

		assert.Equal(t, http.StatusOK, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "raw")
	})

	t.Run("handle returns string response", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

		handle(c, func(c *gin.Context) (interface{}, error) {
			return `{"string":"response"}`, nil
		})

		assert.Equal(t, http.StatusOK, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "string")
	})
}

func TestConvertDBRequestToItem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("convert with all fields", func(t *testing.T) {
		h, _ := setupTestHandler(ctrl)

		req := &dbclient.DeploymentRequest{
			Id:              100,
			DeployName:      "deployer",
			Status:          StatusDeployed,
			ApproverName:    dbutils.NullString("approver"),
			ApprovalResult:  dbutils.NullString(StatusApproved),
			Description:     dbutils.NullString("Deploy v1.0"),
			RejectionReason: dbutils.NullString(""),
			FailureReason:   dbutils.NullString(""),
		}

		item := h.service.cvtDBRequestToItem(req)

		assert.Equal(t, int64(100), item.Id)
		assert.Equal(t, "deployer", item.DeployName)
		assert.Equal(t, StatusDeployed, item.Status)
		assert.Equal(t, "approver", item.ApproverName)
		assert.Equal(t, StatusApproved, item.ApprovalResult)
		assert.Equal(t, "Deploy v1.0", item.Description)
	})
}
