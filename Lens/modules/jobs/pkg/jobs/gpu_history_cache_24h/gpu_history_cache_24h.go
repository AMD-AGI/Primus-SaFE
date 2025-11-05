package gpu_history_cache_24h

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
)

const (
	// Cache key constant
	CacheKeyGpuUsageHistory24h = "cluster:gpu:usage_history:24h"
	
	// Cache expiration duration (15 minutes)
	CacheExpirationDuration = 15 * time.Minute
	
	// History duration
	HistoryDuration = 24 * time.Hour
	
	// Default step for history queries (60 seconds)
	DefaultHistoryStep = 60
)

type GpuHistoryCache24hJob struct {
}

// Run executes the GPU history cache (24 hours) job
func (j *GpuHistoryCache24hJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error {
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	log.Debugf("Starting GPU history cache (24h) job for cluster: %s", clusterName)
	startTime := time.Now()

	// Get generic cache facade
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	// Cache GPU usage history
	if err := j.cacheGpuUsageHistory(ctx, storageClientSet, clusterName, cacheFacade); err != nil {
		log.Errorf("Failed to cache GPU usage history (24h): %v", err)
		return err
	}

	duration := time.Since(startTime)
	log.Debugf("GPU history cache (24h) job completed for cluster: %s, took: %v", clusterName, duration)

	return nil
}

// cacheGpuUsageHistory caches GPU usage history for 24 hours
func (j *GpuHistoryCache24hJob) cacheGpuUsageHistory(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	clusterName string,
	cacheFacade database.GenericCacheFacadeInterface,
) error {
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
	cacheKey := fmt.Sprintf("%s:%s", CacheKeyGpuUsageHistory24h, clusterName)
	expiresAt := time.Now().Add(CacheExpirationDuration)

	// Store in cache
	if err := cacheFacade.Set(ctx, cacheKey, result, &expiresAt); err != nil {
		return fmt.Errorf("failed to set cache for GPU usage history (24h): %w", err)
	}

	log.Debugf("Successfully cached GPU usage history (24h) for cluster: %s, data points: %d",
		clusterName, len(usageHistory))
	return nil
}

// Schedule returns the cron schedule for this job
func (j *GpuHistoryCache24hJob) Schedule() string {
	// Run every 10 minutes - lowest frequency for 24 hour history
	return "@every 10m"
}

