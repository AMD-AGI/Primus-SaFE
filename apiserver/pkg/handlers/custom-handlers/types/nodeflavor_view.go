/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type DiskFlavor struct {
	// storage device type. nvme/ssd/hdd
	Type v1.StorageType `json:"type"`
	// capacity, e.g. 300Gi
	Quantity string `json:"quantity"`
	// the count of device
	Count int `json:"count"`
}

type CreateNodeFlavorRequest struct {
	// flavor's name
	Name string `json:"name"`
	// VirtualMachine/BareMetal
	FlavorType string `json:"flavorType"`
	// cpu core, e.g. 128
	CPU int64 `json:"cpu"`
	// the product name of cpu. e.g. AMD EPYC 9554
	CPUProduct string `json:"cpuProduct,omitempty"`
	// gpu card, e.g. 8
	GPU int64 `json:"gpu,omitempty"`
	// gpu resource name of k8s node, e.g. amd.com/gpu
	GPUName string `json:"gpuName,omitempty"`
	// the product name of gpu. e.g. AMD MI300X
	GPUProduct string `json:"gpuProduct,omitempty"`
	// memory size, e.g. 1073741824
	Memory   int64       `json:"memory"`
	RootDisk *DiskFlavor `json:"rootDisk,omitempty"`
	DataDisk *DiskFlavor `json:"dataDisk,omitempty"`
	// other extend parameters，e.g. rdma/hca
	Extends corev1.ResourceList `json:"extends,omitempty"`
}

type CreateNodeFlavorResponse struct {
	FlavorId string `json:"flavorId"`
}

type GetNodeFlavorResponse struct {
	TotalCount int                         `json:"totalCount"`
	Items      []GetNodeFlavorResponseItem `json:"items"`
}

type GetNodeFlavorResponseItem struct {
	// flavor's id
	FlavorId string `json:"flavorId"`
	// VirtualMachine/BareMetal
	FlavorType string `json:"flavorType"`
	// resource list. e.g. {"cpu": 8}
	Resources ResourceList `json:"resources"`
}
