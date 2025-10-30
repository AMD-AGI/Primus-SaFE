/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateNodeFlavorRequest struct {
	// Used to generate the node flavor ID, which will do normalization processing, e.g. lowercase
	Name string `json:"name"`
	v1.NodeFlavorSpec
}

type CreateNodeFlavorResponse struct {
	// Node flavor ID
	FlavorId string `json:"flavorId"`
}

type ListNodeFlavorResponse struct {
	// The total number of node flavors, not limited by pagination
	TotalCount int                      `json:"totalCount"`
	Items      []NodeFlavorResponseItem `json:"items"`
}

type NodeFlavorResponseItem struct {
	// Node flavor ID
	FlavorId string `json:"flavorId"`
	v1.NodeFlavorSpec
}

type PatchNodeFlavorRequest struct {
	// cpu to modify on the flavor
	CPU *v1.CpuChip `json:"cpu,omitempty"`
	// memory to modify on the flavor
	Memory *resource.Quantity `json:"memory,omitempty"`
	// gpu to modify on the flavor
	Gpu *v1.GpuChip `json:"gpu,omitempty"`
	// root-disk to modify on the flavor
	RootDisk *v1.DiskFlavor `json:"rootDisk,omitempty"`
	// data-disk to modify on the flavor
	DataDisk *v1.DiskFlavor `json:"dataDisk,omitempty"`
	// other extend parameters，e.g. rdma/hca
	ExtendResources *corev1.ResourceList `json:"extendedResources,omitempty"`
}
