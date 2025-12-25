/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import "strings"

// Status constants for DeploymentRequest
const (
	StatusPendingApproval = "pending_approval"
	StatusApproved        = "approved"
	StatusRejected        = "rejected"
	StatusDeploying       = "deploying"
	StatusDeployed        = "deployed"
	StatusFailed          = "failed"
)

// Component name constants
const (
	ComponentApiserver       = "apiserver"
	ComponentResourceManager = "resource_manager"
	ComponentJobManager      = "job_manager"
	ComponentWebhooks        = "webhooks"
	ComponentWeb             = "web"
	ComponentPreprocess      = "preprocess"
	ComponentNodeAgent       = "node_agent"
	ComponentCICDRunner      = "cicd_runner"
	ComponentCICDUnifiedJob  = "cicd_unified_job"
)

// Image name constants (used in container registry)
const (
	ImageApiserver       = "apiserver"
	ImageResourceManager = "resource-manager"
	ImageJobManager      = "job-manager"
	ImageWebhooks        = "webhooks"
	ImageWeb             = "primus-safe-web"
	ImagePreprocess      = "preprocess"
	ImageNodeAgent       = "node-agent"
	ImageCICDRunner      = "cicd-runner-proxy"
	ImageCICDUnifiedJob  = "cicd-unified-job-proxy"
)

// YAML key constants for values.yaml
const (
	YAMLKeyCICDRunner     = "cicd.runner"
	YAMLKeyCICDUnifiedJob = "cicd.unified_job"
	YAMLKeyNodeAgentImage = "image"
)

// ComponentImageMap maps component names to their image names (without version tag)
var ComponentImageMap = map[string]string{
	ComponentApiserver:       ImageApiserver,
	ComponentResourceManager: ImageResourceManager,
	ComponentJobManager:      ImageJobManager,
	ComponentWebhooks:        ImageWebhooks,
	ComponentWeb:             ImageWeb,
	ComponentPreprocess:      ImagePreprocess,
	ComponentNodeAgent:       ImageNodeAgent,
	ComponentCICDRunner:      ImageCICDRunner,
	ComponentCICDUnifiedJob:  ImageCICDUnifiedJob,
}

// CICDComponentsMap maps CICD component names to their YAML keys
var CICDComponentsMap = map[string]string{
	ComponentCICDRunner:     YAMLKeyCICDRunner,
	ComponentCICDUnifiedJob: YAMLKeyCICDUnifiedJob,
}

// NormalizeImageVersion normalizes the image version input.
// Returns the normalized image string.
func NormalizeImageVersion(component, version string) string {
	// If version already contains ":", it's considered a full image reference
	if strings.Contains(version, ":") {
		return version
	}

	// Look up the image name for this component
	imageName, ok := ComponentImageMap[component]
	if !ok {
		// Unknown component, use component name as image name
		imageName = component
	}

	// Combine image name with version tag
	return imageName + ":" + version
}

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

// ApprovalResp is the response for approval action
type ApprovalResp struct {
	Id      int64  `json:"id"`
	Status  string `json:"status"`
	JobId   string `json:"job_id,omitempty"` // OpsJob ID for tracking
	Message string `json:"message"`
}

// DeploymentRequestItem is the view model for the list
type DeploymentRequestItem struct {
	Id              int64  `json:"id"`
	DeployName      string `json:"deploy_name"`
	Status          string `json:"status"`
	ApproverName    string `json:"approver_name"`
	ApprovalResult  string `json:"approval_result"`
	Description     string `json:"description"`
	RejectionReason string `json:"rejection_reason,omitempty"`
	FailureReason   string `json:"failure_reason,omitempty"`
	RollbackFromId  int64  `json:"rollback_from_id,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	ApprovedAt      string `json:"approved_at"`
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

// GetDeployableComponentsResp returns the list of deployable component names
type GetDeployableComponentsResp struct {
	Components []string `json:"components"`
}
