/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FaultPhase string

const (
	FaultPhaseSucceeded FaultPhase = "Succeeded"
	FaultPhaseFailed    FaultPhase = "Failed"

	FaultRepairManual = "manual"
	FaultRepairReport = "report"
	FaultRepairReboot = "reboot"
)

type FaultNode struct {
	Cluster string `json:"cluster,omitempty"`
	// k8s node name
	Name string `json:"name"`
	// 管理面节点name
	AdminName string `json:"adminName"`
	// 节点ip
	InternalIP string `json:"internalIP,omitempty"`
}

type FaultSpec struct {
	// 故障码
	Code string `json:"code"`
	// 故障信息
	Message string `json:"message,omitempty"`
	// 是否是节点无关的故障
	IsNodeIndependent bool `json:"isNodeIndependent,omitempty"`
	// 故障对应的节点信息
	Node FaultNode `json:"node"`
	// 故障对应的action处理, 比如taint,reboot
	Action string `json:"action,omitempty"`
	// 故障是否会自动修复，目前xid错误无法自动修复
	DisableAutoRepair bool `json:"disableAutoRepair,omitempty"`
	// 修复建议，目前主要用于前端展示用。 目前主要有reboot/manual/report
	RepairSuggestion string `json:"repairSuggestion,omitempty"`
}

type FaultStatus struct {
	Phase FaultPhase `json:"phase,omitempty"`
	// 最后一次更新Status的时间
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd.com-v1-fault,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=faults,verbs=create;update,versions=v1,name=mfault.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd.com-v1-fault,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=faults,verbs=create;update,versions=v1,name=vfault.kb.io,admissionReviewVersions={v1,v1beta1}
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

func (f *Fault) IsEnd() bool {
	if f.Status.Phase == FaultPhaseSucceeded || f.Status.Phase == FaultPhaseFailed {
		return true
	}
	return false
}
