/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"time"
)

// ListModelQuery represents query parameters for listing models.
type ListModelQuery struct {
	Limit      int    `form:"limit" binding:"omitempty,min=1"`
	Offset     int    `form:"offset" binding:"omitempty,min=0"`
	AccessMode string `form:"accessMode" binding:"omitempty"` // Filter by access mode: "local" or "remote_api"
	Workspace  string `form:"workspace" binding:"omitempty"`  // Filter by workspace (for local models)
}

// ChatRequest represents the unified request to chat with a model or workload.
// Backend auto-detects whether serviceId refers to a Model or Workload.
type ChatRequest struct {
	// Unified service identification (Model ID or Workload ID)
	ServiceId string `json:"serviceId" binding:"required"` // Model ID or Workload ID

	// Model name for the inference service
	// - For remote_api models: auto-detected from model.spec.source.modelName (can be overridden)
	// - For workloads: required, specifies the model path (e.g., "/models/llama-2-7b")
	ModelName string `json:"modelName,omitempty"`

	// Optional overrides
	BaseUrl string `json:"baseUrl,omitempty"` // Override service URL (useful for workloads with custom endpoints)
	ApiKey  string `json:"apiKey,omitempty"`  // Override API key

	// Chat parameters
	Messages         []map[string]interface{} `json:"messages" binding:"required"` // OpenAI format messages
	Stream           bool                     `json:"stream"`                      // Enable streaming (SSE)
	Temperature      float64                  `json:"temperature"`                 // Controls randomness (0.0-2.0)
	TopP             float64                  `json:"topP"`                        // Nucleus sampling (0.0-1.0)
	MaxTokens        int                      `json:"maxTokens"`                   // Maximum tokens to generate
	FrequencyPenalty float64                  `json:"frequencyPenalty"`            // Penalize frequent tokens (-2.0 to 2.0)
	PresencePenalty  float64                  `json:"presencePenalty"`             // Penalize tokens based on presence (-2.0 to 2.0)
	N                int                      `json:"n"`                           // Number of completions to generate (1-10)
}

// SaveSessionRequest represents the request to save a chat session.
type SaveSessionRequest struct {
	Id           int64            `json:"id"` // Optional: if provided, update existing session
	ModelName    string           `json:"modelName" binding:"required"`
	DisplayName  string           `json:"displayName" binding:"required"`
	SystemPrompt string           `json:"systemPrompt"`
	Messages     []MessageHistory `json:"messages" binding:"required"` // Full chat history from frontend
}

// SaveSessionResponse represents the response after saving a session.
type SaveSessionResponse struct {
	Id int64 `json:"id"`
}

// ListPlaygroundSessionQuery represents query parameters for listing sessions.
type ListPlaygroundSessionQuery struct {
	Limit     int    `form:"limit" binding:"omitempty,min=1"`
	Offset    int    `form:"offset" binding:"omitempty,min=0"`
	ModelName string `form:"modelName" binding:"omitempty"`
}

// ListPlaygroundSessionResponse represents the response for listing sessions.
type ListPlaygroundSessionResponse struct {
	Total int                     `json:"total"`
	Items []PlaygroundSessionInfo `json:"items"`
}

// PlaygroundSessionInfo represents basic session information.
type PlaygroundSessionInfo struct {
	Id           int64  `json:"id"`
	UserId       string `json:"userId"`
	ModelName    string `json:"modelName"`
	DisplayName  string `json:"displayName"`
	SystemPrompt string `json:"systemPrompt"`
	Messages     string `json:"messages"`
	CreationTime string `json:"creationTime"`
	UpdateTime   string `json:"updateTime"`
}

// PlaygroundSessionDetail represents detailed session information.
type PlaygroundSessionDetail struct {
	Id           int64  `json:"id"`
	UserId       string `json:"userId"`
	ModelName    string `json:"modelName"`
	DisplayName  string `json:"displayName"`
	SystemPrompt string `json:"systemPrompt"`
	Messages     string `json:"messages"`
	CreationTime string `json:"creationTime"`
	UpdateTime   string `json:"updateTime"`
}

// MessageHistory represents a chat message.
type MessageHistory struct {
	Role      string    `json:"role"` // system, user, assistant
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
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
}

// ModelSourceReq represents the model source configuration.
type ModelSourceReq struct {
	URL        string `json:"url"`        // Model URL (HuggingFace repo ID or API endpoint)
	AccessMode string `json:"accessMode"` // "remote_api" or "local"
	ModelName  string `json:"modelName"`  // Model name for API calls (required for remote_api)
	Token      string `json:"token"`      // HuggingFace token for pulling private models (local mode)
	ApiKey     string `json:"apiKey"`     // API key for remote API access (remote_api mode)
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
	CreatedAt       string            `json:"createdAt,omitempty"`
	UpdatedAt       string            `json:"updatedAt,omitempty"`
	DeletionTime    string            `json:"deletionTime,omitempty"`
	IsDeleted       bool              `json:"isDeleted"`
}

// LocalPathInfo represents local path status for a workspace.
type LocalPathInfo struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// ListModelResponse represents the response for listing models.
type ListModelResponse struct {
	Total int64       `json:"total"`
	Items []ModelInfo `json:"items"`
}

// PatchModelRequest represents the request to update a model's mutable fields.
// All fields are optional - only provided fields will be updated.
type PatchModelRequest struct {
	ModelName   *string `json:"modelName,omitempty"`   // Update model name for API calls
	DisplayName *string `json:"displayName,omitempty"` // Update display name
	Description *string `json:"description,omitempty"` // Update description
}

// --- Playground Services Types ---

// PlaygroundServiceItem represents an item in the playground services list.
// Can be either a remote_api model or a running inference workload.
type PlaygroundServiceItem struct {
	Type        string `json:"type"`        // "remote_api" or "workload"
	ID          string `json:"id"`          // Model ID or Workload ID (use as serviceId for chat)
	DisplayName string `json:"displayName"` // Display name
	ModelName   string `json:"modelName"`   // Model name for API calls
	Phase       string `json:"phase"`       // Status/Phase
	Workspace   string `json:"workspace"`   // Workspace (for workloads)

	// URL for accessing the service (can be used as baseUrl override in chat)
	// - For remote_api: the API endpoint URL
	// - For workload: external domain via Higress (if configured) or internal domain
	BaseUrl string `json:"baseUrl,omitempty"`

	// For workloads only
	SourceModelID   string `json:"sourceModelId,omitempty"`   // Source Model ID (from annotation)
	SourceModelName string `json:"sourceModelName,omitempty"` // Source Model display name
}

// ListPlaygroundServicesQuery represents query parameters for listing playground services.
type ListPlaygroundServicesQuery struct {
	Workspace string `form:"workspace" binding:"omitempty"` // Filter workloads by workspace
}

// ListPlaygroundServicesResponse represents the response for listing playground services.
type ListPlaygroundServicesResponse struct {
	Total int                     `json:"total"`
	Items []PlaygroundServiceItem `json:"items"`
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

// --- Chat URL Types ---

// ChatURLResponse represents the response for getting the chat URL.
type ChatURLResponse struct {
	URL       string `json:"url"`       // Base URL for chat API
	ModelName string `json:"modelName"` // Model name to use
	HasApiKey bool   `json:"hasApiKey"` // Whether an API key is configured
}
