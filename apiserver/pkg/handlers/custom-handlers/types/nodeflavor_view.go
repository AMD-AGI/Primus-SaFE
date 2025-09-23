/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateNodeFlavorRequest struct {
	// flavor's name
	Name string `json:"name"`
	v1.NodeFlavorSpec
}

type CreateNodeFlavorResponse struct {
	FlavorId string `json:"flavorId"`
}

type ListNodeFlavorResponse struct {
	TotalCount int                      `json:"totalCount"`
	Items      []NodeFlavorResponseItem `json:"items"`
}

type NodeFlavorResponseItem struct {
	// flavor's id
	FlavorId string `json:"flavorId"`
	v1.NodeFlavorSpec
}

type PatchNodeFlavorRequest struct {
	// cpu core, e.g. 128
	CPU *int64 `json:"cpu"`
	// the product name of cpu. e.g. AMD EPYC 9554
	CPUProduct *string `json:"cpuProduct,omitempty"`
	// memory size, e.g. 1073741824
	Memory   *int64         `json:"memory"`
	RootDisk *v1.DiskFlavor `json:"rootDisk,omitempty"`
	DataDisk *v1.DiskFlavor `json:"dataDisk,omitempty"`
	// other extend parameters，e.g. rdma/hca
	Extends *corev1.ResourceList `json:"extends,omitempty"`
}
