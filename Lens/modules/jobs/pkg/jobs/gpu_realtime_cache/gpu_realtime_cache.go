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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_realtime_cache_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "gpu_realtime_cache"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("cache_expiration_minutes", int(CacheExpirationDuration.Minutes())),
	)

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

	stats.RecordsProcessed = 2
	stats.AddMessage("GPU realtime cache updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int64("items_updated", stats.ItemsUpdated),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Float64("query_duration_seconds", stats.QueryDuration),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)

	if stats.ErrorCount > 0 {
		span.SetStatus(codes.Error, "Some cache operations failed")
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return stats, nil
}

// cacheGpuAllocationInfo caches the GPU nodes allocation information
func (j *GpuRealtimeCacheJob) cacheGpuAllocationInfo(
	ctx context.Context,
	clientSets *clientsets.K8SClientSet,
	clusterName string,
	cacheFacade database.GenericCacheFacadeInterface,
) (int, error) {
	span, ctx := trace.StartSpanFromContext(ctx, "cacheGpuAllocationInfo")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	// Get GPU nodes allocation
	getAllocationSpan, getAllocationCtx := trace.StartSpanFromContext(ctx, "getGpuNodesAllocation")
	getAllocationSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	startTime := time.Now()
	allocationInfo, err := gpu.GetGpuNodesAllocation(getAllocationCtx, clientSets, clusterName, metadata.GpuVendorAMD)
	duration := time.Since(startTime)

	if err != nil {
		getAllocationSpan.RecordError(err)
		getAllocationSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		getAllocationSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getAllocationSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get GPU nodes allocation")
		return 0, fmt.Errorf("failed to get GPU nodes allocation: %w", err)
	}

	getAllocationSpan.SetAttributes(
		attribute.Int("nodes_count", len(allocationInfo)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getAllocationSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getAllocationSpan)

	// Store in cache
	setCacheSpan, setCacheCtx := trace.StartSpanFromContext(ctx, "setCacheData")
	setCacheSpan.SetAttributes(
		attribute.String("cache.key", CacheKeyGpuAllocationInfo),
		attribute.Int("cache.expiration_minutes", int(CacheExpirationDuration.Minutes())),
		attribute.Int("nodes_count", len(allocationInfo)),
	)

	cacheKey := CacheKeyGpuAllocationInfo
	expiresAt := time.Now().Add(CacheExpirationDuration)

	startTime = time.Now()
	if err := cacheFacade.Set(setCacheCtx, cacheKey, allocationInfo, &expiresAt); err != nil {
		duration := time.Since(startTime)
		setCacheSpan.RecordError(err)
		setCacheSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		setCacheSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(setCacheSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to set cache")
		return 0, fmt.Errorf("failed to set cache for GPU allocation info: %w", err)
	}

	duration = time.Since(startTime)
	setCacheSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	setCacheSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(setCacheSpan)

	span.SetAttributes(attribute.Int("nodes_count", len(allocationInfo)))
	span.SetStatus(codes.Ok, "")
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
	span, ctx := trace.StartSpanFromContext(ctx, "cacheGpuUtilization")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	// Calculate GPU usage
	calculateUsageSpan, calculateUsageCtx := trace.StartSpanFromContext(ctx, "calculateGpuUsage")
	calculateUsageSpan.SetAttributes(attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)))

	startTime := time.Now()
	usage, err := gpu.CalculateGpuUsage(calculateUsageCtx, storageClientSet, metadata.GpuVendorAMD)
	duration := time.Since(startTime)

	if err != nil {
		calculateUsageSpan.RecordError(err)
		calculateUsageSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
			attribute.Float64("usage_fallback", 0),
		)
		calculateUsageSpan.SetStatus(codes.Error, "Using fallback value 0")
		trace.FinishSpan(calculateUsageSpan)

		log.Warnf("Failed to calculate GPU usage: %v, using 0", err)
		usage = 0
	} else {
		calculateUsageSpan.SetAttributes(
			attribute.Float64("usage_percent", usage),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		calculateUsageSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(calculateUsageSpan)
	}

	// Get cluster GPU allocation rate
	getAllocationRateSpan, getAllocationRateCtx := trace.StartSpanFromContext(ctx, "getClusterGpuAllocationRate")
	getAllocationRateSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	startTime = time.Now()
	allocationRate, err := gpu.GetClusterGpuAllocationRateFromDB(getAllocationRateCtx, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	duration = time.Since(startTime)

	if err != nil {
		getAllocationRateSpan.RecordError(err)
		getAllocationRateSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		getAllocationRateSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getAllocationRateSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get allocation rate")
		return fmt.Errorf("failed to get cluster GPU allocation rate: %w", err)
	}

	getAllocationRateSpan.SetAttributes(
		attribute.Float64("allocation_rate_percent", allocationRate),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getAllocationRateSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getAllocationRateSpan)

	// Build utilization result
	result := &model.GPUUtilization{
		AllocationRate: allocationRate,
		Utilization:    usage,
	}

	// Store in cache
	setCacheSpan, setCacheCtx := trace.StartSpanFromContext(ctx, "setCacheData")
	setCacheSpan.SetAttributes(
		attribute.String("cache.key", CacheKeyGpuUtilization),
		attribute.Int("cache.expiration_minutes", int(CacheExpirationDuration.Minutes())),
		attribute.Float64("allocation_rate_percent", allocationRate),
		attribute.Float64("utilization_percent", usage),
	)

	cacheKey := CacheKeyGpuUtilization
	expiresAt := time.Now().Add(CacheExpirationDuration)

	startTime = time.Now()
	if err := cacheFacade.Set(setCacheCtx, cacheKey, result, &expiresAt); err != nil {
		duration := time.Since(startTime)
		setCacheSpan.RecordError(err)
		setCacheSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		setCacheSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(setCacheSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to set cache")
		return fmt.Errorf("failed to set cache for GPU utilization: %w", err)
	}

	duration = time.Since(startTime)
	setCacheSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	setCacheSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(setCacheSpan)

	span.SetAttributes(
		attribute.Float64("allocation_rate_percent", allocationRate),
		attribute.Float64("utilization_percent", usage),
	)
	span.SetStatus(codes.Ok, "")
	return nil
}

// Schedule returns the cron schedule for this job
func (j *GpuRealtimeCacheJob) Schedule() string {
	// Run every 30 seconds - high frequency for realtime data
	return "@every 30s"
}
