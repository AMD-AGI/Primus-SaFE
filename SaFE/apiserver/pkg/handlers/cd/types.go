/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cd

// Status constants for DeploymentRequest
const (
	StatusPendingApproval = "pending_approval"
	StatusApproved        = "approved"
	StatusRejected        = "rejected"
	StatusDeploying       = "deploying"
	StatusDeployed        = "deployed"
	StatusFailed          = "failed"
)

// CreateDeploymentRequestReq defines the payload for creating a deployment request
type CreateDeploymentRequestReq struct {
	ImageVersions map[string]string `json:"image_versions" binding:"required"` // Module image versions
	EnvFileConfig string            `json:"env_file_config"`                   // Complete .env file content (optional)
	Description   string            `json:"description"`
}

// DeploymentConfig wraps both image versions and env file config for storage
type DeploymentConfig struct {
	ImageVersions map[string]string `json:"image_versions"`  // e.g. {"apiserver": "harbor.example.com/primus/apiserver:v1.2.3"}
	EnvFileConfig string            `json:"env_file_config"` // Complete .env file content
}

// CreateDeploymentRequestResp is the response for creation
type CreateDeploymentRequestResp struct {
	Id int64 `json:"id"`
}

// ApprovalReq defines the payload for approving/rejecting
type ApprovalReq struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"` // Optional rejection reason
}

// DeploymentRequestItem is the view model for the list
type DeploymentRequestItem struct {
	Id             int64  `json:"id"`
	DeployName     string `json:"deploy_name"`
	Status         string `json:"status"`
	ApproverName   string `json:"approver_name"`
	ApprovalResult string `json:"approval_result"`
	Description    string `json:"description"`
	RollbackFromId int64  `json:"rollback_from_id,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ApprovedAt     string `json:"approved_at"`
}

// ListDeploymentRequestsResp is the list response
type ListDeploymentRequestsResp struct {
	TotalCount int                      `json:"total_count"`
	Items      []*DeploymentRequestItem `json:"items"`
}

// GetDeploymentRequestResp is the detail response
type GetDeploymentRequestResp struct {
	DeploymentRequestItem
	ImageVersions map[string]string `json:"image_versions"`  // Module image versions
	EnvFileConfig string            `json:"env_file_config"` // Complete .env file content
}

// ConfigDiffReq defines payload for config diff preview (optional helper)
type ConfigDiffReq struct {
	NewConfig string `json:"new_config"`
	OldConfig string `json:"old_config"` // Optional, if empty diff against current
}

// ConfigDiffResp returns the diff
type ConfigDiffResp struct {
	Diff string `json:"diff"`
}

// GetCurrentEnvConfigResp returns the current .env file content
type GetCurrentEnvConfigResp struct {
	EnvFileConfig string `json:"env_file_config"` // Current .env file content
}
