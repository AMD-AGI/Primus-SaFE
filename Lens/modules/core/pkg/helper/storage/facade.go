package storage

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
)

func GetStorageStat(ctx context.Context) (*model.StorageStat, error) {
	storage, _, err := database.ListStorage(ctx, 1, 1)
	if err != nil {
		return nil, err
	}
	if len(storage) == 0 {
		return &model.StorageStat{}, nil
	}
	storageName := storage[0].Name
	query := getStorageQuery(storage[0].Kind)
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
	switch kind {
	case "juicefs":
		return &JuicefsQuery{
			clientSet: clientsets.GetCurrentClusterStorageClientSet(),
		}
	}
	return nil
}
