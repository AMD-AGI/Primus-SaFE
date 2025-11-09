package gpu_realtime_cache

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
	// Cache key constants
	CacheKeyGpuAllocationInfo = "cluster:gpu:allocation_info"
	CacheKeyGpuUtilization    = "cluster:gpu:utilization"

	// Cache expiration duration (5 minutes)
	CacheExpirationDuration = 5 * time.Minute
)

type GpuRealtimeCacheJob struct {
}

// Run executes the GPU realtime cache job
func (j *GpuRealtimeCacheJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	log.Debugf("Starting GPU realtime cache job for cluster: %s", clusterName)
	startTime := time.Now()

	// Get generic cache facade
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	// 1. Cache GPU allocation info
	queryStart := time.Now()
	allocationCount, err := j.cacheGpuAllocationInfo(ctx, clientSets, clusterName, cacheFacade)
	stats.QueryDuration += time.Since(queryStart).Seconds()
	if err != nil {
		stats.ErrorCount++
		log.Errorf("Failed to cache GPU allocation info: %v", err)
		// Don't return error, continue with next cache
	} else {
		stats.ItemsUpdated++
		stats.AddCustomMetric("gpu_allocation_nodes_cached", allocationCount)
	}

	// 2. Cache GPU utilization
	queryStart = time.Now()
	if err := j.cacheGpuUtilization(ctx, clientSets, storageClientSet, clusterName, cacheFacade); err != nil {
		stats.ErrorCount++
		stats.QueryDuration += time.Since(queryStart).Seconds()
		log.Errorf("Failed to cache GPU utilization: %v", err)
		// Don't return error
	} else {
		stats.QueryDuration += time.Since(queryStart).Seconds()
		stats.ItemsUpdated++
	}

	duration := time.Since(startTime)
	stats.RecordsProcessed = 2
	stats.AddMessage("GPU realtime cache updated successfully")
	log.Debugf("GPU realtime cache job completed for cluster: %s, took: %v", clusterName, duration)

	return stats, nil
}

// cacheGpuAllocationInfo caches the GPU nodes allocation information
func (j *GpuRealtimeCacheJob) cacheGpuAllocationInfo(
	ctx context.Context,
	clientSets *clientsets.K8SClientSet,
	clusterName string,
	cacheFacade database.GenericCacheFacadeInterface,
) (int, error) {
	// Get GPU nodes allocation
	allocationInfo, err := gpu.GetGpuNodesAllocation(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		return 0, fmt.Errorf("failed to get GPU nodes allocation: %w", err)
	}

	// Set cache key with cluster name
	cacheKey := CacheKeyGpuAllocationInfo
	expiresAt := time.Now().Add(CacheExpirationDuration)

	// Store in cache
	if err := cacheFacade.Set(ctx, cacheKey, allocationInfo, &expiresAt); err != nil {
		return 0, fmt.Errorf("failed to set cache for GPU allocation info: %w", err)
	}

	log.Debugf("Successfully cached GPU allocation info for cluster: %s, count: %d", clusterName, len(allocationInfo))
	return len(allocationInfo), nil
}

// cacheGpuUtilization caches the GPU utilization information
func (j *GpuRealtimeCacheJob) cacheGpuUtilization(
	ctx context.Context,
	clientSets *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet,
	clusterName string,
	cacheFacade database.GenericCacheFacadeInterface,
) error {
	// Calculate GPU usage
	usage, err := gpu.CalculateGpuUsage(ctx, storageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		log.Warnf("Failed to calculate GPU usage: %v, using 0", err)
		usage = 0
	}

	// Get cluster GPU allocation rate
	allocationRate, err := gpu.GetClusterGpuAllocationRate(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		return fmt.Errorf("failed to get cluster GPU allocation rate: %w", err)
	}

	// Build utilization result
	result := &model.GPUUtilization{
		AllocationRate: allocationRate,
		Utilization:    usage,
	}

	// Set cache key with cluster name
	cacheKey := CacheKeyGpuUtilization
	expiresAt := time.Now().Add(CacheExpirationDuration)

	// Store in cache
	if err := cacheFacade.Set(ctx, cacheKey, result, &expiresAt); err != nil {
		return fmt.Errorf("failed to set cache for GPU utilization: %w", err)
	}

	log.Debugf("Successfully cached GPU utilization for cluster: %s, allocation: %.2f%%, utilization: %.2f%%",
		clusterName, allocationRate, usage)
	return nil
}

// Schedule returns the cron schedule for this job
func (j *GpuRealtimeCacheJob) Schedule() string {
	// Run every 30 seconds - high frequency for realtime data
	return "@every 30s"
}
