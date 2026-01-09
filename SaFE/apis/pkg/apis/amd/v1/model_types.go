/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ModelKind = "Model"

	// SourceModelLabel is the label key for associating Workloads with their source Model
	SourceModelLabel = "primus-safe/source-model"
)

type (
	// ModelPhase represents the current phase of the model
	ModelPhase string

	// AccessMode represents how to access the model
	AccessMode string

	// LocalPathStatus represents the download status of a model in a specific workspace
	LocalPathStatus string
)

const (
	// Model Phases
	ModelPhasePending     ModelPhase = "Pending"
	ModelPhaseUploading   ModelPhase = "Uploading"   // Uploading to S3
	ModelPhaseDownloading ModelPhase = "Downloading" // Downloading to local PFS
	ModelPhaseReady       ModelPhase = "Ready"
	ModelPhaseFailed      ModelPhase = "Failed"

	// Access Mode Types
	AccessModeRemoteAPI AccessMode = "remote_api" // Call external API directly
	AccessModeLocal     AccessMode = "local"      // Download model and run locally

	// Local Path Status
	LocalPathStatusPending     LocalPathStatus = "Pending"
	LocalPathStatusDownloading LocalPathStatus = "Downloading"
	LocalPathStatusReady       LocalPathStatus = "Ready"
	LocalPathStatusFailed      LocalPathStatus = "Failed"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="DisplayName",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="ModelName",type=string,JSONPath=`.spec.source.modelName`
// +kubebuilder:printcolumn:name="AccessMode",type=string,JSONPath=`.spec.source.accessMode`
// +kubebuilder:printcolumn:name="Workspace",type=string,JSONPath=`.spec.workspace`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:rbac:groups=amd.com,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=models/status,verbs=get;update;patch

// Model defines a model item in the model playground
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

type (
	// ModelSpec defines the desired state of Model
	ModelSpec struct {
		// DisplayName is the friendly name shown in the UI
		DisplayName string `json:"displayName,omitempty"`
		// Description describes the model
		Description string `json:"description,omitempty"`
		// Icon is the URL or Base64 of the model icon
		Icon string `json:"icon,omitempty"`
		// Label is the model label
		Label string `json:"label,omitempty"`
		// Tags are used for search and classification (e.g. "LLM", "CV", "ASR")
		Tags []string `json:"tags,omitempty"`
		// MaxTokens is the maximum context length of the model (from config.json max_position_embeddings)
		MaxTokens int `json:"maxTokens,omitempty"`
		// Source defines where to pull the model from
		Source ModelSource `json:"source"`
		// Workspace specifies which workspace this model belongs to (for local models only)
		// Empty string means "public" - the model will be downloaded to all workspaces
		// Non-empty means the model is private to a specific workspace
		Workspace string `json:"workspace,omitempty"`
	}

	// ModelSource describes the model storage location
	ModelSource struct {
		// URL is the pull address (e.g., "meta-llama/Llama-2-7b", "s3://bucket/model", "https://api.openai.com")
		URL string `json:"url"`
		// AccessMode defines how to access the model:
		//   - "remote_api": Call external API directly (e.g., OpenAI, DeepSeek)
		//   - "local": Download model and run inference service locally
		AccessMode AccessMode `json:"accessMode,omitempty"`
		// ModelName is the model identifier used when calling the API
		// Required for remote_api mode (e.g., "gpt-4", "deepseek-chat")
		// For local mode, this is auto-extracted from URL or user-specified
		ModelName string `json:"modelName,omitempty"`
		// Token references a Secret containing the auth token for pulling the model (HuggingFace token)
		// Used for local mode to access private models
		Token *corev1.LocalObjectReference `json:"token,omitempty"`
		// ApiKey references a Secret containing the API key for remote API access
		// Used for remote_api mode to authenticate with external services (e.g., OpenAI, DeepSeek)
		ApiKey *corev1.LocalObjectReference `json:"apiKey,omitempty"`
	}

	// ModelLocalPath represents the download status of a model in a specific workspace
	ModelLocalPath struct {
		// Workspace is the workspace ID
		Workspace string `json:"workspace"`
		// Path is the local file system path where the model is stored
		Path string `json:"path"`
		// Status is the download status for this workspace
		Status LocalPathStatus `json:"status"`
		// Message contains additional status information
		Message string `json:"message,omitempty"`
	}

	// ModelStatus defines the observed state of Model
	ModelStatus struct {
		// Phase is the current phase of the model
		Phase ModelPhase `json:"phase,omitempty"`
		// Message contains additional status information
		Message string `json:"message,omitempty"`
		// S3Path is the S3 storage path for the model (for local models)
		S3Path string `json:"s3Path,omitempty"`
		// LocalPaths contains the download status for each workspace (for local models)
		LocalPaths []ModelLocalPath `json:"localPaths,omitempty"`
		// UpdateTime is the last update time
		UpdateTime *metav1.Time `json:"updateTime,omitempty"`
	}
)

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ModelList contains a list of Model
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}

// IsPending returns true if the model is pending
func (m *Model) IsPending() bool {
	return m.Status.Phase == "" || m.Status.Phase == ModelPhasePending
}

// IsUploading returns true if the model is being uploaded to S3
func (m *Model) IsUploading() bool {
	return m.Status.Phase == ModelPhaseUploading
}

// IsDownloading returns true if the model is being downloaded to local PFS
func (m *Model) IsDownloading() bool {
	return m.Status.Phase == ModelPhaseDownloading
}

// IsReady returns true if the model is ready
func (m *Model) IsReady() bool {
	return m.Status.Phase == ModelPhaseReady
}

// IsFailed returns true if the model failed
func (m *Model) IsFailed() bool {
	return m.Status.Phase == ModelPhaseFailed
}

// IsRemoteAPI returns true if the model uses remote API access
func (m *Model) IsRemoteAPI() bool {
	return m.Spec.Source.AccessMode == AccessModeRemoteAPI
}

// IsLocal returns true if the model uses local deployment
func (m *Model) IsLocal() bool {
	return m.Spec.Source.AccessMode == AccessModeLocal
}

// IsPublic returns true if the model is public (available to all workspaces)
func (m *Model) IsPublic() bool {
	return m.Spec.Workspace == ""
}

// GetModelName returns the model name for API calls
// Falls back to display name or CR name if not set
func (m *Model) GetModelName() string {
	if m.Spec.Source.ModelName != "" {
		return m.Spec.Source.ModelName
	}
	if m.Spec.DisplayName != "" {
		return m.GetSafeDisplayName()
	}
	return m.Name
}

// GetS3Path returns the S3 path for the model
// If S3Path is set in status, use it; otherwise generate from model name
func (m *Model) GetS3Path() string {
	if m.Status.S3Path != "" {
		return m.Status.S3Path
	}
	return "models/" + m.GetSafeDisplayName()
}

// GetSafeDisplayName returns a sanitized display name safe for file paths
// Replaces /, :, and other special characters with -
func (m *Model) GetSafeDisplayName() string {
	name := m.Spec.DisplayName
	if name == "" {
		name = m.Name
	}
	// Replace special characters
	replacer := strings.NewReplacer("/", "-", ":", "-", " ", "-", "\\", "-")
	return replacer.Replace(name)
}

// GetLocalPathForWorkspace returns the local path status for a specific workspace
func (m *Model) GetLocalPathForWorkspace(workspaceID string) *ModelLocalPath {
	for i := range m.Status.LocalPaths {
		if m.Status.LocalPaths[i].Workspace == workspaceID {
			return &m.Status.LocalPaths[i]
		}
	}
	return nil
}

// IsReadyInWorkspace returns true if the model is ready in the specified workspace
func (m *Model) IsReadyInWorkspace(workspaceID string) bool {
	lp := m.GetLocalPathForWorkspace(workspaceID)
	return lp != nil && lp.Status == LocalPathStatusReady
}

// GetReadyWorkspaces returns a list of workspace IDs where the model is ready
func (m *Model) GetReadyWorkspaces() []string {
	var workspaces []string
	for _, lp := range m.Status.LocalPaths {
		if lp.Status == LocalPathStatusReady {
			workspaces = append(workspaces, lp.Workspace)
		}
	}
	return workspaces
}
