/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/constvar"
)

const (
	InferenceKind = "Inference"
)

type (
	// InferenceSpec defines the desired state of Inference
	InferenceSpec struct {
		// DisplayName is the user-defined name for the inference service
		DisplayName string `json:"displayName"`
		// Description is the description of the inference service
		Description string `json:"description,omitempty"`
		// UserID is the ID of the user who owns this inference service
		UserID string `json:"userID"`
		// UserName is the name of the user
		UserName string `json:"userName,omitempty"`
		// ModelForm specifies the source of the model (API or ModelSquare)
		ModelForm constvar.InferenceModelForm `json:"modelForm"`
		// ModelName is the name of the model
		ModelName string `json:"modelName"`
		// Instance contains the inference instance information
		Instance InferenceInstance `json:"instance"`
		// Resource contains the resource requirements for the inference service
		Resource InferenceResource `json:"resource"`
		// Config contains additional configuration for different phases
		Config InferenceConfig `json:"config,omitempty"`
	}

	// InferenceInstance contains inference instance details
	InferenceInstance struct {
		// BaseUrl is the inference service URL
		BaseUrl string `json:"baseUrl,omitempty"`
		// ApiKey is the inference service API key
		ApiKey string `json:"apiKey,omitempty"`
		// ContextLength is the context length
		ContextLength int `json:"contextLength,omitempty"`
		// WorkloadID is the workload ID
		WorkloadID string `json:"workloadID,omitempty"`
	}

	// InferenceResource contains resource requirements
	InferenceResource struct {
		// Workspace is the workspace ID
		Workspace string `json:"workspace"`
		// Gpu is the GPU count
		Gpu string `json:"gpu,omitempty"`
		// Cpu is the CPU count
		Cpu int `json:"cpu,omitempty"`
		// Memory is the memory size in GB
		Memory int `json:"memory,omitempty"`
		// Replica is the number of replicas
		Replica int `json:"replica,omitempty"`
	}

	// InferenceConfig contains configuration for the inference service
	InferenceConfig struct {
		// Image is the container image
		Image string `json:"image,omitempty"`
		// EntryPoint is the execution script
		EntryPoint string `json:"entryPoint,omitempty"`
		// ModelPath is the model path
		ModelPath string `json:"modelPath,omitempty"`
	}

	// InferenceStatus defines the observed state of Inference
	InferenceStatus struct {
		// Phase is the current phase of the inference service
		Phase constvar.InferencePhaseType `json:"phase,omitempty"`
		// Events records events for each phase
		Events []InferenceEvent `json:"events,omitempty"`
		// Message contains additional status information
		Message string `json:"message,omitempty"`
		// UpdateTime is the last update time
		UpdateTime *metav1.Time `json:"updateTime,omitempty"`
	}

	// InferenceEvent records an event in the inference lifecycle
	InferenceEvent struct {
		// WorkloadID is the associated workload ID
		WorkloadID string `json:"workloadID,omitempty"`
		// WorkloadPhase is the workload status
		WorkloadPhase WorkloadPhase `json:"workloadPhase,omitempty"`
		// Timestamp is when this event occurred
		Timestamp metav1.Time `json:"timestamp,omitempty"`
		// Message contains additional event information
		Message string `json:"message,omitempty"`
	}
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="DisplayName",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="ModelForm",type=string,JSONPath=`.spec.modelForm`
// +kubebuilder:printcolumn:name="ModelName",type=string,JSONPath=`.spec.modelName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:rbac:groups=amd.com,resources=inferences,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=inferences/status,verbs=get;update;patch

// Inference is the Schema for the inferences API
type Inference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceSpec   `json:"spec,omitempty"`
	Status InferenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InferenceList contains a list of Inference
type InferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inference `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Inference{}, &InferenceList{})
}

// IsPending returns true if the inference service is pending
func (i *Inference) IsPending() bool {
	return i.Status.Phase == "" || i.Status.Phase == constvar.InferencePhasePending
}

// IsRunning returns true if the inference service is running
func (i *Inference) IsRunning() bool {
	return i.Status.Phase == constvar.InferencePhaseRunning
}

// IsStopped returns true if the inference service is stopped
func (i *Inference) IsStopped() bool {
	return i.Status.Phase == constvar.InferencePhaseStopped
}

// IsEnd returns true if the inference service has ended (stopped or failed)
func (i *Inference) IsEnd() bool {
	return i.Status.Phase == constvar.InferencePhaseStopped ||
		i.Status.Phase == constvar.InferencePhaseFailure
}

// IsFromAPI returns true if the model is from API import
func (i *Inference) IsFromAPI() bool {
	return i.Spec.ModelForm == constvar.InferenceModelFormAPI
}

// IsFromModelSquare returns true if the model is from ModelSquare
func (i *Inference) IsFromModelSquare() bool {
	return i.Spec.ModelForm == constvar.InferenceModelFormModelSquare
}

// AddEvent adds a new event to the inference status
func (i *Inference) AddEvent(workloadID string, workloadPhase WorkloadPhase, message string) {
	event := InferenceEvent{
		WorkloadID:    workloadID,
		WorkloadPhase: workloadPhase,
		Timestamp:     metav1.Now(),
		Message:       message,
	}
	i.Status.Events = append(i.Status.Events, event)
}
