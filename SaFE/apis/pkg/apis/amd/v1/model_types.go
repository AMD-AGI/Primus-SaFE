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
)

type (
	// ModelPhase represents the current phase of the model
	ModelPhase string

	// AccessMode represents how to access the model
	AccessMode string
)

const (
	// Model Phases
	ModelPhasePending ModelPhase = "Pending"
	ModelPhasePulling ModelPhase = "Pulling"
	ModelPhaseReady   ModelPhase = "Ready"
	ModelPhaseFailed  ModelPhase = "Failed"

	// Access Mode Types
	AccessModeRemoteAPI AccessMode = "remote_api" // Call external API directly
	AccessModeLocal     AccessMode = "local"      // Download model and run locally
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="DisplayName",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="AccessMode",type=string,JSONPath=`.spec.source.accessMode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="InferenceID",type=string,JSONPath=`.status.inferenceID`
// +kubebuilder:printcolumn:name="InferencePhase",type=string,JSONPath=`.status.inferencePhase`
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
	}

	// ModelSource describes the model storage location
	ModelSource struct {
		// URL is the pull address (e.g., "meta-llama/Llama-2-7b", "s3://bucket/model", "https://api.openai.com")
		URL string `json:"url"`
		// AccessMode defines how to access the model:
		//   - "remote_api": Call external API directly (e.g., OpenAI, DeepSeek)
		//   - "local": Download model and run inference service locally
		AccessMode AccessMode `json:"accessMode,omitempty"`
		// Token references a Secret containing the auth token for pulling the model or API access
		Token *corev1.LocalObjectReference `json:"token,omitempty"`
	}

	// ModelStatus defines the observed state of Model
	ModelStatus struct {
		// Phase is the current phase of the model
		Phase ModelPhase `json:"phase,omitempty"`
		// Message contains additional status information
		Message string `json:"message,omitempty"`
		// InferenceID is the ID of the associated Inference CR (when user starts the model)
		// Empty if the model hasn't been started yet
		InferenceID string `json:"inferenceID,omitempty"`
		// InferencePhase is the current phase of the associated Inference service
		// Empty if no inference is running
		InferencePhase string `json:"inferencePhase,omitempty"`
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

// IsPulling returns true if the model is being pulled
func (m *Model) IsPulling() bool {
	return m.Status.Phase == ModelPhasePulling
}

// IsReady returns true if the model is ready
func (m *Model) IsReady() bool {
	return m.Status.Phase == ModelPhaseReady
}

// IsFailed returns true if the model failed
func (m *Model) IsFailed() bool {
	return m.Status.Phase == ModelPhaseFailed
}

// HasInference returns true if the model has an associated inference service
func (m *Model) HasInference() bool {
	return m.Status.InferenceID != ""
}

// IsRemoteAPI returns true if the model uses remote API access
func (m *Model) IsRemoteAPI() bool {
	return m.Spec.Source.AccessMode == AccessModeRemoteAPI
}

// IsLocal returns true if the model uses local deployment
func (m *Model) IsLocal() bool {
	return m.Spec.Source.AccessMode == AccessModeLocal
}

// GetS3Path returns the S3 path for the model (models/{safeDisplayName})
func (m *Model) GetS3Path() string {
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
