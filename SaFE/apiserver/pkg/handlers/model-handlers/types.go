/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

// ListModelQuery represents query parameters for listing models.
type ListModelQuery struct {
	Limit      int    `form:"limit" binding:"omitempty,min=1"`
	Offset     int    `form:"offset" binding:"omitempty,min=0"`
	AccessMode string `form:"accessMode" binding:"omitempty"` // Filter: "local", "remote_api", "local_path", "s3_sync" (s3_sync excludes plain HF local in cluster listing)
	Workspace  string `form:"workspace" binding:"omitempty"`  // Filter by workspace (for local models)
	Origin     string `form:"origin" binding:"omitempty"`     // Filter by origin: "external", "fine_tuned", "rl_trained", or "custom" (all non-external)
	Search     string `form:"search" binding:"omitempty"`     // Fuzzy search by displayName (case-insensitive)
	Phase      string `form:"phase" binding:"omitempty"`      // Filter by phase: "Pending", "Uploading", "Downloading", "Ready", "Failed" (case-insensitive)
	Sort       string `form:"sort" binding:"omitempty"`       // Sort order: "name"/"-name" (by displayName), "createdAt"/"-createdAt". Empty keeps default created_at DESC.
	// IncludeDeleting also returns models whose deletion is still cleaning up files
	// (isDeleted=true, deletionTime set); by default they are hidden.
	IncludeDeleting bool `form:"includeDeleting" binding:"omitempty"`
}

// --- Model Management Types ---

// CreateModelRequest represents the request to create a new model.
type CreateModelRequest struct {
	// Metadata fields (required for remote_api mode, auto-filled for local mode)
	DisplayName string   `json:"displayName"` // Model display name
	Description string   `json:"description"` // Model description
	Icon        string   `json:"icon"`        // Model icon URL
	Label       string   `json:"label"`       // Model author/organization
	Tags        []string `json:"tags"`        // Model tags
	MaxTokens   int      `json:"maxTokens"`   // Maximum context length (auto-filled from config.json for local mode)

	// Model source configuration
	Source ModelSourceReq `json:"source"`

	// Workspace for local models (empty = public, non-empty = specific workspace)
	Workspace string `json:"workspace"`

	// SFT training fields (used when accessMode=local_path)
	Origin    string `json:"origin"`              // "external" (default) or "fine_tuned"
	SftJobId  string `json:"sftJobId,omitempty"`  // SFT workload ID that produced this model
	BaseModel string `json:"baseModel,omitempty"` // Base model HF name used for fine-tuning

	// S3Source (used when accessMode=s3_sync): copy from user bucket/prefix to platform S3, then existing pipeline to PFS.
	S3Source *S3SourceReq `json:"s3Source,omitempty"`

	// Target controls which workspace volume / subdirectory the download is placed under.
	// Required when the resolved workspace has multiple volumes and the default first PFS
	// is not what you want (e.g. workspace has both a read-only PFS and a writable HostPath).
	Target *ModelTargetReq `json:"target,omitempty"`
}

// ModelTargetReq overrides where the download is placed inside the workspace.
type ModelTargetReq struct {
	// Volume is the workspace volume mountPath (or hostPath if mountPath is empty), e.g. "/wekafs".
	Volume string `json:"volume,omitempty"`
	// Subpath, if set, is the relative directory used under the volume in addition to "models/<safe-name>".
	Subpath string `json:"subpath,omitempty"`
}

// S3SourceReq optional credentials to read the source object storage (S3-compatible).
// Keys in the created Secret: access_key_id, secret_access_key, region, endpoint.
type S3SourceReq struct {
	URI                 string `json:"uri"` // required for s3_sync, e.g. s3://my-bucket/prefix
	AccessKeyID         string `json:"accessKeyId,omitempty"`
	SecretAccessKey     string `json:"secretAccessKey,omitempty"`
	Region              string `json:"region,omitempty"`
	Endpoint            string `json:"endpoint,omitempty"` // optional custom endpoint for source (MinIO, OSS, etc.)
}

// ModelSourceReq represents the model source configuration.
type ModelSourceReq struct {
	URL        string `json:"url"`        // Model URL (HuggingFace repo ID or API endpoint)
	AccessMode string `json:"accessMode"` // "remote_api" | "local" | "local_path" | "s3_sync"
	ModelName  string `json:"modelName"`  // Model name for API calls (required for remote_api)
	Token      string `json:"token"`      // HuggingFace token for pulling private models (local mode)
	ApiKey     string `json:"apiKey"`     // API key for remote API access (remote_api mode)
	LocalPath  string `json:"localPath"`  // NFS/PFS path where model files exist (local_path mode)
}

// CreateResponse represents the response after creating a model.
type CreateResponse struct {
	ID string `json:"id"`
}

// ModelInfo represents the API response for a model.
type ModelInfo struct {
	ID              string            `json:"id"`
	DisplayName     string            `json:"displayName"`
	Description     string            `json:"description"`
	Icon            string            `json:"icon"`
	Label           string            `json:"label"`
	Tags            string            `json:"tags"`                      // Plain tags string
	CategorizedTags []TagWithCategory `json:"categorizedTags,omitempty"` // Tags with category and color
	MaxTokens       int               `json:"maxTokens"`
	Version         string            `json:"version"`
	SourceURL       string            `json:"sourceURL"`
	AccessMode      string            `json:"accessMode"`
	ModelName       string            `json:"modelName"` // Model name for API calls
	Phase           string            `json:"phase"`
	Message         string            `json:"message"`
	Workspace       string            `json:"workspace"`            // Workspace ID (empty = public)
	S3Path          string            `json:"s3Path,omitempty"`     // S3 storage path
	LocalPaths      []LocalPathInfo   `json:"localPaths,omitempty"` // Local download status per workspace
	Origin          string            `json:"origin,omitempty"`
	SftJobId        string            `json:"sftJobId,omitempty"`
	BaseModel       string            `json:"baseModel,omitempty"`
	UserId          string            `json:"userId,omitempty"`
	UserName        string            `json:"userName,omitempty"`
	CreatedAt       string            `json:"createdAt,omitempty"`
	UpdatedAt       string            `json:"updatedAt,omitempty"`
	DeletionTime    string            `json:"deletionTime,omitempty"`
	IsDeleted       bool              `json:"isDeleted"`
	HasInferenceX   bool              `json:"hasInferenceX"`
	InferenceXModel string            `json:"inferenceXModel,omitempty"`

	// Optional download placement; mirrors Model.spec.targetVolume / spec.targetSubpath.
	TargetVolume  string `json:"targetVolume,omitempty"`
	TargetSubpath string `json:"targetSubpath,omitempty"`
}

// LocalPathInfo represents local path status for a workspace.
type LocalPathInfo struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	// SizeBytes is the on-disk size reported by the download, 0 when not reported.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// ListModelResponse represents the response for listing models.
type ListModelResponse struct {
	Total int64       `json:"total"`
	Items []ModelInfo `json:"items"`
}

// PatchModelRequest represents the request to update a model's mutable fields.
// All fields are optional - only provided fields will be updated.
type PatchModelRequest struct {
	ModelName   *string   `json:"modelName,omitempty"`   // Update model name for API calls
	DisplayName *string   `json:"displayName,omitempty"` // Update display name
	Description *string   `json:"description,omitempty"` // Update description
	Icon        *string   `json:"icon,omitempty"`        // Update card icon
	Label       *string   `json:"label,omitempty"`       // Update card label/org
	Tags        *[]string `json:"tags,omitempty"`        // Replace tag list
	MaxTokens   *int      `json:"maxTokens,omitempty"`   // Update max context length
}

// --- Workload Config Types ---

// WorkloadConfigRequest represents the request to get workload config for a model.
type WorkloadConfigRequest struct {
	Workspace string `form:"workspace" binding:"required"` // Target workspace for deployment
}

// WorkloadServiceConfig represents the service configuration for a workload.
type WorkloadServiceConfig struct {
	Protocol    string `json:"protocol"`    // TCP or UDP
	Port        int    `json:"port"`        // Service port
	TargetPort  int    `json:"targetPort"`  // Container port
	ServiceType string `json:"serviceType"` // ClusterIP, NodePort, etc.
}

// WorkloadConfigResponse represents the auto-generated workload configuration.
// Frontend can use this to pre-fill the workload creation form.
type WorkloadConfigResponse struct {
	// Pre-filled fields
	DisplayName string            `json:"displayName"` // Suggested workload name
	Description string            `json:"description"` // Description
	Env         map[string]string `json:"env"`         // Environment variables including MODEL_PATH and PRIMUS_SOURCE_MODEL

	// Model info for reference
	ModelID    string `json:"modelId"`
	ModelName  string `json:"modelName"`
	ModelPath  string `json:"modelPath"` // Local path in the workspace
	AccessMode string `json:"accessMode"`
	MaxTokens  int    `json:"maxTokens"`

	// Fields to be filled by user (with defaults for inference workloads)
	Image            string                `json:"image"`            // Container image (user must provide)
	EntryPoint       string                `json:"entryPoint"`       // Entry point command (user must provide)
	Workspace        string                `json:"workspace"`        // Target workspace
	CPU              string                `json:"cpu"`              // CPU request
	Memory           string                `json:"memory"`           // Memory request
	GPU              string                `json:"gpu"`              // GPU request
	Replica          string                `json:"replica"`          // Number of replicas
	EphemeralStorage string                `json:"ephemeralStorage"` // Ephemeral storage
	Service          WorkloadServiceConfig `json:"service"`          // Service configuration
}

// --- Model Workloads Types ---

// ModelWorkloadsResponse represents the response for listing workloads associated with a model.
type ModelWorkloadsResponse struct {
	Total int                  `json:"total"`
	Items []AssociatedWorkload `json:"items"`
}

// AssociatedWorkload represents a workload associated with a model.
type AssociatedWorkload struct {
	WorkloadID  string `json:"workloadId"`
	DisplayName string `json:"displayName"`
	Workspace   string `json:"workspace"`
	Phase       string `json:"phase"`
	IngressURL  string `json:"ingressUrl,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

