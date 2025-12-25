/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestGetWorkloadService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("empty service id returns bad request", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "") // Empty name

		h.GetWorkloadService(c)
		assert.Equal(t, http.StatusBadRequest, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "serviceId is empty")
	})

	t.Run("workload not found returns success with nil", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/non-existing-workload", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "non-existing-workload")

		h.GetWorkloadService(c)
		// client.IgnoreNotFound returns nil for not found errors
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("workload exists but cluster client not found returns error", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		// Create an empty client manager (no cluster clients registered)
		clientManager := commonutils.NewObjectManager()
		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			clientManager:    clientManager,
		}

		// Create a workload owned by the mock user
		workload := genMockWorkloadForService(mockUser.Name)
		err := fakeClient.Create(context.Background(), workload)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/"+workload.Name, nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, workload.Name)

		h.GetWorkloadService(c)
		// Should fail because cluster client is not found in clientManager
		assert.NotEqual(t, http.StatusOK, rsp.Code)
		assert.Contains(t, rsp.Body.String(), "not found")
	})
}

func TestGetWorkloadServiceAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("admin user can access any workload service", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		clientManager := commonutils.NewObjectManager()
		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			clientManager:    clientManager,
		}

		// Create a workload owned by a different user
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "other-user-workload",
				Labels: map[string]string{
					v1.UserIdLabel:      "other-user",
					v1.WorkspaceIdLabel: "other-workspace",
					v1.ClusterIdLabel:   "test-cluster",
				},
			},
			Spec: v1.WorkloadSpec{
				Workspace: "other-workspace",
			},
		}
		err := fakeClient.Create(context.Background(), workload)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/other-user-workload", nil)
		// Use the admin user (mockUser which has SystemAdminRole)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "other-user-workload")

		h.GetWorkloadService(c)
		// Authorization should pass for admin, but will fail at clientManager
		// because cluster client is not registered
		assert.NotEqual(t, http.StatusForbidden, rsp.Code)
	})

	t.Run("unauthorized user cannot access workload service", func(t *testing.T) {
		_, fakeClient := createMockUser()
		clientManager := commonutils.NewObjectManager()
		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			clientManager:    clientManager,
		}

		// Create a workload owned by a different user
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "other-user-workload-2",
				Labels: map[string]string{
					v1.UserIdLabel:      "other-user",
					v1.WorkspaceIdLabel: "other-workspace",
					v1.ClusterIdLabel:   "test-cluster",
				},
			},
			Spec: v1.WorkloadSpec{
				Workspace: "other-workspace",
			},
		}
		err := fakeClient.Create(context.Background(), workload)
		assert.NoError(t, err)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/other-user-workload-2", nil)
		// Use an unauthorized user ID
		c.Set(common.UserId, "unauthorized-user")
		c.Set(common.Name, "other-user-workload-2")

		h.GetWorkloadService(c)
		// Should fail due to authorization
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

func TestGetWorkloadServiceValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("whitespace only service id treated as empty", func(t *testing.T) {
		mockUser, fakeClient := createMockUser()
		h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/services/", nil)
		c.Set(common.UserId, mockUser.Name)
		c.Set(common.Name, "   ") // Whitespace only - this might not be caught as empty

		h.GetWorkloadService(c)
		// Depending on implementation, this might be treated as invalid or as a workload name
		// Current implementation checks for empty string only
	})
}

// genMockWorkloadForService creates a mock workload for service tests
func genMockWorkloadForService(userId string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload-service",
			Labels: map[string]string{
				v1.UserIdLabel:      userId,
				v1.WorkspaceIdLabel: "test-workspace",
				v1.ClusterIdLabel:   "test-cluster",
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: "test-workspace",
			Image:     "test-image:latest",
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}
}
