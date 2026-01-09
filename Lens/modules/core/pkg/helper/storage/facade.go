// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func GetStorageStat(ctx context.Context) (*model.StorageStat, error) {
	return GetStorageStatWithClientSet(ctx, nil)
}

// GetStorageStatWithClientSet gets storage statistics with support for specifying StorageClientSet
func GetStorageStatWithClientSet(ctx context.Context, storageClientSet *clientsets.StorageClientSet) (*model.StorageStat, error) {
	storage, _, err := database.GetFacade().GetStorage().ListStorage(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	if len(storage) == 0 {
		return &model.StorageStat{}, nil
	}
	storageName := storage[0].Name
	query := getStorageQueryWithClientSet(storage[0].Kind, storageClientSet)
	if query == nil {
		log.Warnf("Storage kind %s not supported", storage[0].Kind)
		return &model.StorageStat{}, nil
	}
	storageUsage, inodesUsage, totalStorage, totalInodes, err := query.Stat(ctx, storageName)
	if err != nil {
		return nil, err
	}
	readBandwidth, writeBandwidth, err := query.Bandwidth(ctx, storageName)
	if err != nil {
		return nil, err
	}
	totalStorageCopy := totalStorage
	if totalStorageCopy == 0 {
		totalStorageCopy = 1
	}
	totalInodesCopy := totalInodes
	if totalInodesCopy == 0 {
		totalInodesCopy = 1
	}
	return &model.StorageStat{
		TotalSpace:            totalStorage,
		UsedSpace:             storageUsage,
		UsagePercentage:       storageUsage / totalStorageCopy * 100,
		TotalInodes:           totalInodes,
		UsedInodes:            inodesUsage,
		InodesUsagePercentage: inodesUsage / totalInodesCopy * 100,
		ReadBandwidth:         readBandwidth,
		WriteBandwidth:        writeBandwidth,
	}, nil
}

func getStorageQuery(kind string) Query {
	return getStorageQueryWithClientSet(kind, nil)
}

func getStorageQueryWithClientSet(kind string, storageClientSet *clientsets.StorageClientSet) Query {
	// If storageClientSet is not specified, use the current cluster's
	if storageClientSet == nil {
		storageClientSet = clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet
	}

	switch kind {
	case "juicefs":
		return &JuicefsQuery{
			clientSet: storageClientSet,
		}
	}
	return nil
}
