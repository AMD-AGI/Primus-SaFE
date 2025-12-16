/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=MI300X;MI325X;MI355X
type GpuProduct string

const (
	NodeFlavorKind = "NodeFlavor"
)

type CpuChip struct {
	// Cpu product name, e.g. AMD EPYC 9554
	Product string `json:"product,omitempty"`
	// CPU cores (resource.Quantity), e.g. "256"
	Quantity resource.Quantity `json:"quantity"`
}

type GpuChip struct {
	// Gpu product name, e.g. MI300X/MI325X/MI355X
	Product GpuProduct `json:"product,omitempty"`
	// K8s resource name when gpu is set, e.g. "amd.com/gpu"
	ResourceName string `json:"resourceName"`
	// GPU count (resource.Quantity) when gpu is set, e.g. "8"
	Quantity resource.Quantity `json:"quantity"`
}

// NodeFlavorSpec defines the desired state of NodeFlavor
type NodeFlavorSpec struct {
	// CPU configuration, required
	Cpu CpuChip `json:"cpu"`
	// Memory size (resource.Quantity), required, e.g. "1024Gi"
	Memory resource.Quantity `json:"memory"`
	// GPU configuration, optional
	Gpu *GpuChip `json:"gpu,omitempty"`
	// root disk configuration, optional, Usually this refers to the system disk size
	RootDisk *DiskFlavor `json:"rootDisk,omitempty"`
	// data disk configuration, optional, Usually this refers to the disk size mounted on the node, e.g. an NVMe disk.
	DataDisk *DiskFlavor `json:"dataDisk,omitempty"`
	// Extra resources map: key:string -> value:resource.Quantity
	ExtendResources corev1.ResourceList `json:"extendedResources,omitempty"`
}

type DiskFlavor struct {
	// disk type, e.g. "ssd", "sata", "nvme"
	Type StorageType `json:"type,omitempty"`
	// disk size (resource.Quantity) when diskFlavor is set
	Quantity resource.Quantity `json:"quantity"`
	// Number of disks when diskFlavor is set
	Count int `json:"count"`
}

// NodeFlavorStatus defines the observed state of NodeFlavor
type NodeFlavorStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-nodeflavor,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=nodeflavors,verbs=create;update,versions=v1,name=mnodeflavor.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-nodeflavor,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=nodeflavors,verbs=create;update,versions=v1,name=vnodeflavor.kb.io,admissionReviewVersions={v1,v1beta1}
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

// HasGpu returns true if the node flavor includes GPU resources.
func (nf *NodeFlavor) HasGpu() bool {
	if nf != nil && nf.Spec.Gpu != nil && !nf.Spec.Gpu.Quantity.IsZero() {
		return true
	}
	return false
}

// ToResourceList converts node flavor resources to a Kubernetes ResourceList.
func (nf *NodeFlavor) ToResourceList(rdmaName string) corev1.ResourceList {
	if nf == nil {
		return nil
	}
	result := make(corev1.ResourceList)
	result[corev1.ResourceCPU] = nf.Spec.Cpu.Quantity
	result[corev1.ResourceMemory] = nf.Spec.Memory
	storage, ok := nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage]
	if ok {
		result[corev1.ResourceEphemeralStorage] = storage
	}
	if rdmaName != "" {
		rdma, ok := nf.Spec.ExtendResources[corev1.ResourceName(rdmaName)]
		if ok {
			result[corev1.ResourceName(rdmaName)] = rdma
		}
	}
	if nf.HasGpu() {
		result[corev1.ResourceName(nf.Spec.Gpu.ResourceName)] = nf.Spec.Gpu.Quantity
	}
	return result
}
