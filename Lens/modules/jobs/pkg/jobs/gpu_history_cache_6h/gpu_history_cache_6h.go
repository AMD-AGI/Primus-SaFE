package gpu_history_cache_6h

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
)

const (
	// Cache key constant
	CacheKeyGpuUsageHistory6h = "cluster:gpu:usage_history:6h"

	// Cache expiration duration (10 minutes)
	CacheExpirationDuration = 10 * time.Minute

	// History duration
	HistoryDuration = 6 * time.Hour

	// Default step for history queries (60 seconds)
	DefaultHistoryStep = 60
)

type GpuHistoryCache6hJob struct {
}

// Run executes the GPU history cache (6 hours) job
func (j *GpuHistoryCache6hJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	log.Debugf("Starting GPU history cache (6h) job for cluster: %s", clusterName)
	startTime := time.Now()

	// Get generic cache facade
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	// Cache GPU usage history
	queryStart := time.Now()
	dataPoints, err := j.cacheGpuUsageHistory(ctx, storageClientSet, clusterName, cacheFacade)
	stats.QueryDuration = time.Since(queryStart).Seconds()
	if err != nil {
		log.Errorf("Failed to cache GPU usage history (6h): %v", err)
		return stats, err
	}

	duration := time.Since(startTime)
	stats.RecordsProcessed = int64(dataPoints)
	stats.ItemsUpdated = 1
	stats.AddCustomMetric("data_points", dataPoints)
	stats.AddCustomMetric("time_range_hours", 6)
	stats.AddMessage("GPU usage history (6h) cached successfully")
	log.Debugf("GPU history cache (6h) job completed for cluster: %s, took: %v", clusterName, duration)

	return stats, nil
}

// cacheGpuUsageHistory caches GPU usage history for 6 hours
func (j *GpuHistoryCache6hJob) cacheGpuUsageHistory(
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
	cacheKey := CacheKeyGpuUsageHistory6h
	expiresAt := time.Now().Add(CacheExpirationDuration)

	// Store in cache
	if err := cacheFacade.Set(ctx, cacheKey, result, &expiresAt); err != nil {
		return 0, fmt.Errorf("failed to set cache for GPU usage history (6h): %w", err)
	}

	log.Debugf("Successfully cached GPU usage history (6h) for cluster: %s, data points: %d",
		clusterName, len(usageHistory))
	return len(usageHistory), nil
}

// Schedule returns the cron schedule for this job
func (j *GpuHistoryCache6hJob) Schedule() string {
	// Run every 5 minutes - lower frequency for 6 hour history
	return "@every 5m"
}
