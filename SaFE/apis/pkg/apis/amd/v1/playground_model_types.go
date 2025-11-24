/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlaygroundModel defines a model item in the model playground
type PlaygroundModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlaygroundModelSpec   `json:"spec,omitempty"`
	Status PlaygroundModelStatus `json:"status,omitempty"`
}

const (
	// PlaygroundModelKind is the Kind name for PlaygroundModel
	PlaygroundModelKind = "PlaygroundModel"

	// Download Target Types
	DownloadTypeLocal = "Local"
	DownloadTypeS3    = "S3"

	ProtocolRemoteAPI = "remote_api"
	ProtocolLocal     = "local"

	// Status Phases
	ModelPhasePending = "Pending"
	ModelPhasePulling = "Pulling"
	ModelPhaseReady   = "Ready"
	ModelPhaseFailed  = "Failed"
)

// PlaygroundModelSpec defines the detailed configuration of the model
type PlaygroundModelSpec struct {
	// DisplayName is the friendly name shown in the UI
	DisplayName string `json:"displayName,omitempty"`
	// Description describes the model
	Description string `json:"description,omitempty"`
	// Icon is the URL or Base64 of the model icon
	Icon string `json:"icon,omitempty"`
	// Tags are used for search and classification (e.g. "LLM", "CV", "ASR")
	Tags []string `json:"tags,omitempty"`

	// Version is the model version/tag
	Version string `json:"version"`
	// Source defines where to pull the model from
	Source ModelSource `json:"source"`

	// DownloadTarget defines where to store the pulled model.
	DownloadTarget *DownloadTarget `json:"downloadTarget,omitempty"`

	// Command overrides the container entrypoint
	Command []string `json:"command,omitempty"`
	// Args are arguments passed to the command
	Args []string `json:"args,omitempty"`
	// Env adds extra environment variables
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources defines the compute resources (CPU, Memory, GPU) required
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// DownloadTarget defines the storage location for the model
type DownloadTarget struct {
	// Type specifies where to store the model: "Local" or "S3"
	Type string `json:"type"`
	// LocalPath is the absolute path on the host (for type "Local")
	LocalPath string `json:"localPath,omitempty"`
	// S3Config specifies the S3 bucket details (for type "S3")
	S3Config *S3TargetConfig `json:"s3Config,omitempty"`
}

// S3TargetConfig defines S3 storage configuration
type S3TargetConfig struct {
	Endpoint        string `json:"endpoint,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	Region          string `json:"region,omitempty"`
	AccessKeyID     string `json:"accessKeyID,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// ModelSource describes the model storage location
type ModelSource struct {
	// URL is the pull address (e.g., "meta-llama/Llama-2-7b", "s3://bucket/model")
	URL string `json:"url"`

	// Access Protocols: "remote_api" (API access), "local" (existing path)
	Protocol string `json:"protocol,omitempty"`

	// Token references a Secret containing the auth token for pulling the model
	Token *corev1.LocalObjectReference `json:"token,omitempty"`
}

// PlaygroundModelStatus defines the status of the model
type PlaygroundModelStatus struct {
	// Phase is the current lifecycle phase (e.g., "Pending", "Pulling", "Ready", "Failed")
	Phase string `json:"phase,omitempty"`
	// Message provides details about the status
	Message string `json:"message,omitempty"`
	// RunningStatus indicates if the model service is currently running (e.g. "Running", "Stopped")
	RunningStatus string `json:"runningStatus,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlaygroundModelList contains a list of PlaygroundModel
type PlaygroundModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlaygroundModel `json:"items"`
}
