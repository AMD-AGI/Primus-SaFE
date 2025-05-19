/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateStorageClusterRequest struct {
	*v1.StorageClusterSpec
	Name string `json:"name"`
	// 用户指定的标签
	Labels *map[string]string `json:"labels,omitempty"`
	// 描述
	Description *string `json:"description,omitempty"`
}

type CreateStorageClusterResponse struct {
	ClusterId string `json:"clusterId"`
}

type StorageClusterView struct {
	Name      string                                 `json:"name"`
	Flavor    string                                 `json:"flavor"`
	Cluster   string                                 `json:"cluster"`
	Count     int                                    `json:"count"`
	Resources map[string]corev1.ResourceRequirements `json:"resources,omitempty"`
	Image     *string                                `json:"image,omitempty"`
	ClusterId string                                 `json:"clusterId,omitempty"`
	// 描述
	Description string      `json:"description,omitempty"`
	Phase       v1.Phase    `json:"phase,omitempty"`
	Health      string      `json:"health,omitempty"`
	Capacity    v1.Capacity `json:"capacity,omitempty"`
}

type ListStorageClusterResponse struct {
	TotalCount int                   `json:"totalCount"`
	Items      []*StorageClusterView `json:"items,omitempty"`
}

type StorageBindingRequest struct {
	Name           string             `json:"name"`
	StorageCluster string             `json:"storageCluster"`
	Type           v1.StorageUseType  `json:"type"`
	StorageClass   string             `json:"storageClass,omitempty"`
	Secret         string             `json:"secret,omitempty"`
	Namespace      string             `json:"namespace,omitempty"`
	Replicated     *v1.ReplicatedSpec `json:"replicated,omitempty"`
	ErasureCoded   bool               `json:"erasureCoded,omitempty"`
}

type DeleteStorageClusterResponse struct {
	Name string `json:"name"`
}

type StorageBindingResponse struct {
	Name string `json:"name"`
}

type StorageUnbindingRequest struct {
	Name string `json:"name"`
}

type StorageUnbindingResponse struct {
	Name string `json:"name"`
}

type StorageNodeFlavorResponse struct {
	TotalCount int                             `json:"totalCount"`
	Items      []StorageNodeFlavorResponseItem `json:"nodeflavors,omitempty"`
}

type StorageNodeFlavorResponseItem struct {
	GetNodeFlavorResponseItem
	Storages *v1.DiskFlavor `json:"storage,omitempty"`
	Count    int            `json:"count"`
}

type BindingStorageResponseItem struct {
	Name           string             `json:"name"`
	StorageCluster string             `json:"storageCluster"`
	Type           v1.StorageUseType  `json:"type"`
	StorageClass   string             `json:"storageClass,omitempty"`
	Secret         string             `json:"secret,omitempty"`
	Namespace      string             `json:"namespace,omitempty"`
	Replicated     *v1.ReplicatedSpec `json:"replicated,omitempty"`
	ErasureCoded   bool               `json:"erasureCoded,omitempty"`
	Phase          v1.Phase           `json:"phase,omitempty"`
}

type BindingStorageResponse struct {
	Storages []BindingStorageResponseItem `json:"storages,omitempty"`
	Count    int                          `json:"count"`
}
