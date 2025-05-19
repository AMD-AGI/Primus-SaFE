/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
)

func cvtBindingStorageView(storages []v1.StorageStatus) []types.BindingStorageResponseItem {
	items := make([]types.BindingStorageResponseItem, 0, len(storages))
	for _, s := range storages {
		items = append(items, cvtBindingStorageItemView(s))
	}
	return items
}

func cvtBindingStorageItemView(s v1.StorageStatus) types.BindingStorageResponseItem {
	item := types.BindingStorageResponseItem{
		Name:           s.Name,
		StorageCluster: s.StorageCluster,
		Type:           s.Type,
		StorageClass:   s.StorageClass,
		Secret:         s.Secret,
		Namespace:      s.Namespace,
		Replicated:     s.Replicated,
		Phase:          s.Phase,
		ErasureCoded:   false,
	}
	if s.ErasureCoded != nil {
		item.ErasureCoded = true
	}
	return item
}
