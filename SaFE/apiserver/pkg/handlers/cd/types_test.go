/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusConstants(t *testing.T) {
	t.Run("status constants have expected values", func(t *testing.T) {
		assert.Equal(t, "pending_approval", StatusPendingApproval)
		assert.Equal(t, "approved", StatusApproved)
		assert.Equal(t, "rejected", StatusRejected)
		assert.Equal(t, "deploying", StatusDeploying)
		assert.Equal(t, "deployed", StatusDeployed)
		assert.Equal(t, "failed", StatusFailed)
	})
}

func TestDeploymentConfigJSON(t *testing.T) {
	t.Run("marshal and unmarshal DeploymentConfig", func(t *testing.T) {
		config := DeploymentConfig{
			ImageVersions: map[string]string{
				"apiserver":        "apiserver:v1.0.0",
				"resource_manager": "resource-manager:v1.0.0",
			},
			EnvFileConfig: "key=value\nother=123",
		}

		// Marshal
		data, err := json.Marshal(config)
		require.NoError(t, err)

		// Unmarshal
		var parsed DeploymentConfig
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, config.ImageVersions, parsed.ImageVersions)
		assert.Equal(t, config.EnvFileConfig, parsed.EnvFileConfig)
	})

	t.Run("unmarshal empty ImageVersions", func(t *testing.T) {
		jsonStr := `{"image_versions":{},"env_file_config":"test"}`

		var config DeploymentConfig
		err := json.Unmarshal([]byte(jsonStr), &config)
		require.NoError(t, err)

		assert.Empty(t, config.ImageVersions)
		assert.Equal(t, "test", config.EnvFileConfig)
	})
}

func TestCreateDeploymentRequestReq(t *testing.T) {
	t.Run("marshal and unmarshal CreateDeploymentRequestReq", func(t *testing.T) {
		req := CreateDeploymentRequestReq{
			ImageVersions: map[string]string{
				"apiserver": "apiserver:v2.0.0",
			},
			EnvFileConfig: "env_content",
			Description:   "Upgrade apiserver",
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var parsed CreateDeploymentRequestReq
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, req.ImageVersions, parsed.ImageVersions)
		assert.Equal(t, req.EnvFileConfig, parsed.EnvFileConfig)
		assert.Equal(t, req.Description, parsed.Description)
	})
}

func TestApprovalReq(t *testing.T) {
	t.Run("approval request approved=true", func(t *testing.T) {
		jsonStr := `{"approved":true,"reason":""}`

		var req ApprovalReq
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)

		assert.True(t, req.Approved)
		assert.Empty(t, req.Reason)
	})

	t.Run("approval request rejected with reason", func(t *testing.T) {
		jsonStr := `{"approved":false,"reason":"Security review required"}`

		var req ApprovalReq
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)

		assert.False(t, req.Approved)
		assert.Equal(t, "Security review required", req.Reason)
	})
}

func TestApprovalResp(t *testing.T) {
	t.Run("marshal ApprovalResp", func(t *testing.T) {
		resp := ApprovalResp{
			Id:      123,
			Status:  StatusApproved,
			Message: "Deployment approved",
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"id":123`)
		assert.Contains(t, string(data), `"status":"approved"`)
		assert.Contains(t, string(data), `"message":"Deployment approved"`)
	})
}

func TestDeploymentRequestItem(t *testing.T) {
	t.Run("marshal DeploymentRequestItem with optional fields", func(t *testing.T) {
		item := DeploymentRequestItem{
			Id:              1,
			DeployName:      "test-user",
			Status:          StatusDeployed,
			ApproverName:    "admin",
			ApprovalResult:  StatusApproved,
			Description:     "Test deployment",
			RejectionReason: "", // should be omitted
			FailureReason:   "", // should be omitted
			RollbackFromId:  0,  // should be omitted
			CreatedAt:       "2025-01-01T00:00:00Z",
			UpdatedAt:       "2025-01-01T00:00:00Z",
			ApprovedAt:      "2025-01-01T00:00:00Z",
		}

		data, err := json.Marshal(item)
		require.NoError(t, err)

		// Check required fields present
		assert.Contains(t, string(data), `"id":1`)
		assert.Contains(t, string(data), `"deploy_name":"test-user"`)
		assert.Contains(t, string(data), `"status":"deployed"`)

		// Check optional fields are omitted when empty
		assert.NotContains(t, string(data), `"rejection_reason":""`)
		assert.NotContains(t, string(data), `"failure_reason":""`)
	})

	t.Run("DeploymentRequestItem with failure info", func(t *testing.T) {
		item := DeploymentRequestItem{
			Id:            2,
			DeployName:    "test-user",
			Status:        StatusFailed,
			FailureReason: "Pod crash loop",
		}

		data, err := json.Marshal(item)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"failure_reason":"Pod crash loop"`)
	})
}

func TestListDeploymentRequestsResp(t *testing.T) {
	t.Run("marshal ListDeploymentRequestsResp", func(t *testing.T) {
		resp := ListDeploymentRequestsResp{
			TotalCount: 2,
			Items: []*DeploymentRequestItem{
				{Id: 1, Status: StatusDeployed},
				{Id: 2, Status: StatusPendingApproval},
			},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"total_count":2`)
		assert.Contains(t, string(data), `"items":[`)
	})

	t.Run("empty items list", func(t *testing.T) {
		resp := ListDeploymentRequestsResp{
			TotalCount: 0,
			Items:      []*DeploymentRequestItem{},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"total_count":0`)
		assert.Contains(t, string(data), `"items":[]`)
	})
}

func TestGetDeploymentRequestResp(t *testing.T) {
	t.Run("marshal GetDeploymentRequestResp", func(t *testing.T) {
		resp := GetDeploymentRequestResp{
			DeploymentRequestItem: DeploymentRequestItem{
				Id:         1,
				DeployName: "user1",
				Status:     StatusDeployed,
			},
			ImageVersions: map[string]string{
				"apiserver": "v1.0.0",
			},
			EnvFileConfig: "key=value",
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"id":1`)
		assert.Contains(t, string(data), `"image_versions":{`)
		assert.Contains(t, string(data), `"env_file_config":"key=value"`)
	})
}

func TestGetDeployableComponentsResp(t *testing.T) {
	t.Run("marshal GetDeployableComponentsResp", func(t *testing.T) {
		resp := GetDeployableComponentsResp{
			Components: []string{"apiserver", "resource_manager", "node_agent"},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed GetDeployableComponentsResp
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, 3, len(parsed.Components))
		assert.Contains(t, parsed.Components, "apiserver")
		assert.Contains(t, parsed.Components, "resource_manager")
		assert.Contains(t, parsed.Components, "node_agent")
	})
}
