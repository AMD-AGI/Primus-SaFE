/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FaultPhase string

const (
	FaultKind                      = "Fault"
	FaultPhaseSucceeded FaultPhase = "Succeeded"
	FaultPhaseFailed    FaultPhase = "Failed"
)

type FaultNode struct {
	// The cluster which fault belongs to
	ClusterName string `json:"clusterName"`
	// Fault-associated k8s node name
	K8sName string `json:"k8sName"`
	// Fault-associated admin node name
	AdminName string `json:"adminName"`
}

type FaultSpec struct {
	// The id used by NodeAgent for monitoring.
	MonitorId string `json:"monitorId"`
	// Fault message
	Message string `json:"message,omitempty"`
	// Node information related to the fault
	Node *FaultNode `json:"node,omitempty"`
	// Handling actions for the fault. e.g. reboot,taint
	Action string `json:"action,omitempty"`
	// Whether the fault is auto repaired or not. default true
	IsAutoRepairEnabled bool `json:"isAutoRepairEnabled,omitempty"`
}

type FaultStatus struct {
	// The status of fault, e.g. Succeeded, Failed
	Phase FaultPhase `json:"phase,omitempty"`
	// The last update time of fault
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-fault,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=faults,verbs=create;update,versions=v1,name=mfault.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-fault,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=faults,verbs=create;update,versions=v1,name=vfault.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=faults,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=faults/status,verbs=get;update;patch

type Fault struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FaultSpec   `json:"spec,omitempty"`
	Status FaultStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FaultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Fault `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Fault{}, &FaultList{})
}

// IsEnd returns true if the fault has ended (completed or failed).
func (f *Fault) IsEnd() bool {
	if f != nil && f.Status.Phase != "" {
		return true
	}
	return false
}
