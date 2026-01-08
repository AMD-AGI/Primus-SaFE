// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package cluster

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// GetClusterOverviewFromCache retrieves cluster overview from cache
// Returns cached data if available, otherwise returns error
func GetClusterOverviewFromCache(ctx context.Context, clusterName string) (*model.GpuClusterOverview, error) {
	cache, err := database.GetFacadeForCluster(clusterName).GetClusterOverviewCache().GetClusterOverviewCache(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster overview cache: %w", err)
	}

	if cache == nil {
		return nil, fmt.Errorf("cluster overview cache not found for cluster: %s", clusterName)
	}

	// Convert database model to API model
	overview := &model.GpuClusterOverview{
		TotalNodes:         int(cache.TotalNodes),
		HealthyNodes:       int(cache.HealthyNodes),
		FaultyNodes:        int(cache.FaultyNodes),
		FullyIdleNodes:     int(cache.FullyIdleNodes),
		PartiallyIdleNodes: int(cache.PartiallyIdleNodes),
		BusyNodes:          int(cache.BusyNodes),
		AllocationRate:     cache.AllocationRate,
		Utilization:        cache.Utilization,
		StorageStat: model.StorageStat{
			TotalSpace:            cache.StorageTotalSpace,
			UsedSpace:             cache.StorageUsedSpace,
			UsagePercentage:       cache.StorageUsagePercentage,
			TotalInodes:           cache.StorageTotalInodes,
			UsedInodes:            cache.StorageUsedInodes,
			InodesUsagePercentage: cache.StorageInodesUsagePercentage,
			ReadBandwidth:         cache.StorageReadBandwidth,
			WriteBandwidth:        cache.StorageWriteBandwidth,
		},
		RdmaClusterStat: model.RdmaClusterStat{
			TotalTx: cache.RdmaTotalTx,
			TotalRx: cache.RdmaTotalRx,
		},
	}

	return overview, nil
}
