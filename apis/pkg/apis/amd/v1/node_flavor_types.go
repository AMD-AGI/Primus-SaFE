/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NodeFlavorKind = "NodeFlavor"
)

type NodeFlavorType string

const (
	BareMetal      NodeFlavorType = "BareMetal"
	VirtualMachine NodeFlavorType = "VirtualMachine"
)

type CpuChip struct {
	// e.g. AMD EPYC 9554
	Product  string            `json:"product,omitempty"`
	Quantity resource.Quantity `json:"quantity"`
}

type GpuChip struct {
	// e.g. AMD MI300X
	Product string `json:"product,omitempty"`
	// Corresponding resource names in Kubernetes ResourceList, such as amd.com/gpu or nvidia.com/gpu
	ResourceName string            `json:"resourceName"`
	Quantity     resource.Quantity `json:"quantity"`
}

// NodeFlavorSpec defines the desired state of NodeFlavor
type NodeFlavorSpec struct {
	// +kubebuilder:validation:Enum=VirtualMachine;BareMetal
	FlavorType      NodeFlavorType      `json:"flavorType,omitempty"`
	Cpu             CpuChip             `json:"cpu"`
	Memory          resource.Quantity   `json:"memory"`
	Gpu             *GpuChip            `json:"gpu,omitempty"`
	RootDisk        *DiskFlavor         `json:"rootDisk,omitempty"`
	DataDisk        *DiskFlavor         `json:"dataDisk,omitempty"`
	ExtendResources corev1.ResourceList `json:"extendedResources,omitempty"`
}

type DiskFlavor struct {
	Type     string            `json:"type,omitempty"`
	Quantity resource.Quantity `json:"quantity"`
	Count    int               `json:"count"`
}

// NodeFlavorStatus defines the observed state of NodeFlavor
type NodeFlavorStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd.com-v1-nodeflavor,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=nodeflavors,verbs=create;update,versions=v1,name=mnodeflavor.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd.com-v1-nodeflavor,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=nodeflavors,verbs=create;update,versions=v1,name=vnodeflavor.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=nodeflavors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=nodeflavors/status,verbs=get;update;patch
type NodeFlavor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeFlavorSpec   `json:"spec,omitempty"`
	Status NodeFlavorStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
type NodeFlavorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeFlavor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeFlavor{}, &NodeFlavorList{})
}

func (nf *NodeFlavor) HasGpu() bool {
	if nf.Spec.Gpu != nil && !nf.Spec.Gpu.Quantity.IsZero() {
		return true
	}
	return false
}

func (nf *NodeFlavor) ToResourceList() corev1.ResourceList {
	result := make(corev1.ResourceList)
	result[corev1.ResourceCPU] = nf.Spec.Cpu.Quantity
	result[corev1.ResourceMemory] = nf.Spec.Memory
	storage, ok := nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage]
	if ok {
		result[corev1.ResourceEphemeralStorage] = storage
	}
	if nf.HasGpu() {
		result[corev1.ResourceName(nf.Spec.Gpu.ResourceName)] = nf.Spec.Gpu.Quantity
	}
	return result
}
