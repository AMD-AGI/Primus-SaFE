/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type BindingStorageResponseItem struct {
	Name           string             `json:"name"`
	StorageCluster string             `json:"storageCluster"`
	Type           v1.StorageUseType  `json:"type"`
	StorageClass   string             `json:"storageClass"`
	Secret         string             `json:"secret"`
	Namespace      string             `json:"namespace"`
	Replicated     *v1.ReplicatedSpec `json:"replicated"`
	ErasureCoded   bool               `json:"erasureCoded"`
	Phase          v1.Phase           `json:"phase"`
}
