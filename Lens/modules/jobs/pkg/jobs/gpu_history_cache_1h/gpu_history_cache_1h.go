package gpu_history_cache_1h

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
)

const (
	// Cache key constant
	CacheKeyGpuUsageHistory1h = "cluster:gpu:usage_history:1h"

	// Cache expiration duration (5 minutes)
	CacheExpirationDuration = 5 * time.Minute

	// History duration
	HistoryDuration = time.Hour

	// Default step for history queries (60 seconds)
	DefaultHistoryStep = 60
)

type GpuHistoryCache1hJob struct {
}

// Run executes the GPU history cache (1 hour) job
func (j *GpuHistoryCache1hJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	log.Debugf("Starting GPU history cache (1h) job for cluster: %s", clusterName)
	startTime := time.Now()

	// Get generic cache facade
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	// Cache GPU usage history
	queryStart := time.Now()
	dataPoints, err := j.cacheGpuUsageHistory(ctx, storageClientSet, clusterName, cacheFacade)
	stats.QueryDuration = time.Since(queryStart).Seconds()
	if err != nil {
		log.Errorf("Failed to cache GPU usage history (1h): %v", err)
		return stats, err
	}

	duration := time.Since(startTime)
	stats.RecordsProcessed = int64(dataPoints)
	stats.ItemsUpdated = 1
	stats.AddCustomMetric("data_points", dataPoints)
	stats.AddCustomMetric("time_range_hours", 1)
	stats.AddMessage("GPU usage history (1h) cached successfully")
	log.Debugf("GPU history cache (1h) job completed for cluster: %s, took: %v", clusterName, duration)

	return stats, nil
}

// cacheGpuUsageHistory caches GPU usage history for 1 hour
func (j *GpuHistoryCache1hJob) cacheGpuUsageHistory(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	clusterName string,
	cacheFacade database.GenericCacheFacadeInterface,
) (int, error) {
	endTime := time.Now()
	startTime := endTime.Add(-HistoryDuration)

	// Get usage history
	usageHistory, err := gpu.GetHistoryGpuUsage(ctx, storageClientSet, metadata.GpuVendorAMD, startTime, endTime, DefaultHistoryStep)
	if err != nil {
		log.Warnf("Failed to get history GPU usage: %v, using empty array", err)
		usageHistory = []model.TimePoint{}
	}

	// Get allocation history
	allocationHistory, err := gpu.GetHistoryGpuAllocationRate(ctx, storageClientSet, metadata.GpuVendorAMD, startTime, endTime, DefaultHistoryStep)
	if err != nil {
		log.Warnf("Failed to get history GPU allocation rate: %v, using empty array", err)
		allocationHistory = []model.TimePoint{}
	}

	// Get VRAM utilization history
	vramUtilizationHistory, err := gpu.GetNodeGpuVramUsageHistory(ctx, storageClientSet, metadata.GpuVendorAMD, startTime, endTime, DefaultHistoryStep)
	if err != nil {
		log.Warnf("Failed to get node GPU VRAM usage history: %v, using empty array", err)
		vramUtilizationHistory = []model.TimePoint{}
	}

	// Build result
	result := model.GpuUtilizationHistory{
		AllocationRate:  allocationHistory,
		Utilization:     usageHistory,
		VramUtilization: vramUtilizationHistory,
	}

	// Set cache key with cluster name
	cacheKey := CacheKeyGpuUsageHistory1h
	expiresAt := time.Now().Add(CacheExpirationDuration)

	// Store in cache
	if err := cacheFacade.Set(ctx, cacheKey, result, &expiresAt); err != nil {
		return 0, fmt.Errorf("failed to set cache for GPU usage history (1h): %w", err)
	}

	log.Debugf("Successfully cached GPU usage history (1h) for cluster: %s, data points: %d",
		clusterName, len(usageHistory))
	return len(usageHistory), nil
}

// Schedule returns the cron schedule for this job
func (j *GpuHistoryCache1hJob) Schedule() string {
	// Run every 1 minute - medium frequency for 1 hour history
	return "@every 1m"
}
