/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	ComponentModelDownloader = "model_downloader"
	ComponentOpsDownload     = "ops_download"
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
	ImageModelDownloader = "model-downloader"
	ImageOpsDownload     = "s3-downloader"
)

// YAML key constants for values.yaml
const (
	YAMLKeyCICDRunner      = "cicd.runner"
	YAMLKeyCICDUnifiedJob  = "cicd.unified_job"
	YAMLKeyNodeAgentImage  = "image"
	YAMLKeyModelDownloader = "model.downloader_image"
	YAMLKeyOpsDownload     = "ops_job.download_image"
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
	ComponentModelDownloader: ImageModelDownloader,
	ComponentOpsDownload:     ImageOpsDownload,
}

// CICDComponentsMap maps CICD component names to their YAML keys
var CICDComponentsMap = map[string]string{
	ComponentCICDRunner:     YAMLKeyCICDRunner,
	ComponentCICDUnifiedJob: YAMLKeyCICDUnifiedJob,
}

// SpecialComponentsMap maps special component names to their custom YAML keys
// These components use non-standard YAML paths (not "component.image" format)
var SpecialComponentsMap = map[string]string{
	ComponentModelDownloader: YAMLKeyModelDownloader,
	ComponentOpsDownload:     YAMLKeyOpsDownload,
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

// Deploy type constants
const (
	DeployTypeSafe = "safe" // Default for backward compatibility
	DeployTypeLens = "lens"
)

// CreateDeploymentRequestReq defines the payload for creating a deployment request
type CreateDeploymentRequestReq struct {
	Type          string            `json:"type"`           // "safe" or "lens", default "safe"
	Branch        string            `json:"branch"`         // Git branch to deploy (optional, defaults to "main")
	ImageVersions map[string]string `json:"image_versions"` // Safe: component versions
	EnvFileConfig string            `json:"env_file_config"`
	// Lens specific fields (used when type=lens)
	ControlPlaneConfig string `json:"control_plane_config,omitempty"` // Full values.yaml content
	DataPlaneConfig    string `json:"data_plane_config,omitempty"`    // Full values.yaml content
	Description        string `json:"description"`
}

// DeploymentConfig wraps config for storage (unified for both safe and lens)
type DeploymentConfig struct {
	Type          string            `json:"type,omitempty"`           // "safe" or "lens"
	Branch        string            `json:"branch,omitempty"`         // Git branch to deploy
	ImageVersions map[string]string `json:"image_versions,omitempty"` // Safe: component versions
	EnvFileConfig string            `json:"env_file_config,omitempty"`
	// Lens specific (full YAML content)
	ControlPlaneConfig string `json:"control_plane_config,omitempty"`
	DataPlaneConfig    string `json:"data_plane_config,omitempty"`
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
	Id         int64  `json:"id"`
	Status     string `json:"status"`
	WorkloadId string `json:"workload_id,omitempty"` // Associated workload/opsjob ID for tracking
	Message    string `json:"message"`
}

// DeploymentRequestItem is the view model for the list
type DeploymentRequestItem struct {
	Id              int64  `json:"id"`
	DeployName      string `json:"deploy_name"`
	DeployType      string `json:"deploy_type"` // "safe" or "lens"
	Status          string `json:"status"`
	ApproverName    string `json:"approver_name"`
	ApprovalResult  string `json:"approval_result"`
	Description     string `json:"description"`
	RejectionReason string `json:"rejection_reason,omitempty"`
	FailureReason   string `json:"failure_reason,omitempty"`
	RollbackFromId  int64  `json:"rollback_from_id,omitempty"`
	WorkloadId      string `json:"workload_id,omitempty"`
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
	Branch             string            `json:"branch,omitempty"`               // Git branch
	ImageVersions      map[string]string `json:"image_versions,omitempty"`       // Safe: component versions
	EnvFileConfig      string            `json:"env_file_config,omitempty"`      // Safe: .env file content
	ControlPlaneConfig string            `json:"control_plane_config,omitempty"` // Lens: CP values.yaml (only for GET /env-config)
	DataPlaneConfig    string            `json:"data_plane_config,omitempty"`    // Lens: DP values.yaml (only for GET /env-config)
	ControlPlaneDiff   string            `json:"control_plane_diff,omitempty"`   // Lens: CP diff against latest snapshot
	DataPlaneDiff      string            `json:"data_plane_diff,omitempty"`      // Lens: DP diff against latest snapshot
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

// GetLatestConfigResp returns the latest deployment configuration
// Used by GET /env-config?type=safe|lens
type GetLatestConfigResp struct {
	Type               string            `json:"type"`                             // "safe" or "lens"
	Branch             string            `json:"branch,omitempty"`                 // Git branch
	ImageVersions      map[string]string `json:"image_versions,omitempty"`         // Safe: component versions
	EnvFileConfig      string            `json:"env_file_config,omitempty"`        // Safe: .env file content
	ControlPlaneConfig string            `json:"control_plane_config,omitempty"`   // Lens: CP values.yaml
	DataPlaneConfig    string            `json:"data_plane_config,omitempty"`      // Lens: DP values.yaml
	SnapshotId         int64             `json:"snapshot_id,omitempty"`            // ID of the snapshot
	CreatedAt          string            `json:"created_at,omitempty"`             // Snapshot creation time
}

// GetDeployableComponentsResp returns the list of deployable component names
type GetDeployableComponentsResp struct {
	Components []string `json:"components"`
}
