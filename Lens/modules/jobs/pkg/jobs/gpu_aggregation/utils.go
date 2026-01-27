// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_aggregation

import (
	"strings"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// splitAnnotationKey splits "key:value" into [key, value]
func splitAnnotationKey(s string) []string {
	idx := strings.Index(s, ":")
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

// convertToDBClusterStats converts application layer model to database model
func convertToDBClusterStats(stats *model.ClusterGpuHourlyStats) *dbmodel.ClusterGpuHourlyStats {
	return &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       stats.ClusterName,
		StatHour:          stats.StatHour,
		TotalGpuCapacity:  int32(stats.TotalGpuCapacity),
		AllocatedGpuCount: stats.AllocatedGpuCount,
		AllocationRate:    stats.AllocationRate,
		AvgUtilization:    stats.AvgUtilization,
		MaxUtilization:    stats.MaxUtilization,
		MinUtilization:    stats.MinUtilization,
		P50Utilization:    stats.P50Utilization,
		P95Utilization:    stats.P95Utilization,
		SampleCount:       int32(stats.SampleCount),
	}
}

// convertToDBNamespaceStats converts application layer model to database model
func convertToDBNamespaceStats(stats *model.NamespaceGpuHourlyStats) *dbmodel.NamespaceGpuHourlyStats {
	return &dbmodel.NamespaceGpuHourlyStats{
		ClusterName:         stats.ClusterName,
		Namespace:           stats.Namespace,
		StatHour:            stats.StatHour,
		TotalGpuCapacity:    int32(stats.TotalGpuCapacity),
		AllocatedGpuCount:   stats.AllocatedGpuCount,
		AllocationRate:      stats.AllocationRate,
		AvgUtilization:      stats.AvgUtilization,
		MaxUtilization:      stats.MaxUtilization,
		MinUtilization:      stats.MinUtilization,
		ActiveWorkloadCount: int32(stats.ActiveWorkloadCount),
	}
}

// convertToDBLabelStats converts application layer model to database model
func convertToDBLabelStats(stats *model.LabelGpuHourlyStats) *dbmodel.LabelGpuHourlyStats {
	return &dbmodel.LabelGpuHourlyStats{
		ClusterName:         stats.ClusterName,
		DimensionType:       stats.DimensionType,
		DimensionKey:        stats.DimensionKey,
		DimensionValue:      stats.DimensionValue,
		StatHour:            stats.StatHour,
		AllocatedGpuCount:   stats.AllocatedGpuCount,
		AvgUtilization:      stats.AvgUtilization,
		MaxUtilization:      stats.MaxUtilization,
		MinUtilization:      stats.MinUtilization,
		ActiveWorkloadCount: int32(stats.ActiveWorkloadCount),
	}
}
