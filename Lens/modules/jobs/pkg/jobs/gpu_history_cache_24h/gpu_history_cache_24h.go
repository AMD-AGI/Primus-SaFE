package gpu_history_cache_24h

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
func (j *GpuHistoryCache24hJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_history_cache_24h_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "gpu_history_cache_24h"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("time_range_hours", 24),
	)

	// Get generic cache facade
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	// Cache GPU usage history
	queryStart := time.Now()
	dataPoints, err := j.cacheGpuUsageHistory(ctx, storageClientSet, clusterName, cacheFacade)
	stats.QueryDuration = time.Since(queryStart).Seconds()
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.message", err.Error()))
		span.SetStatus(codes.Error, "Failed to cache GPU usage history")
		log.Errorf("Failed to cache GPU usage history (24h): %v", err)
		return stats, err
	}

	stats.RecordsProcessed = int64(dataPoints)
	stats.ItemsUpdated = 1
	stats.AddCustomMetric("data_points", dataPoints)
	stats.AddCustomMetric("time_range_hours", 24)
	stats.AddMessage("GPU usage history (24h) cached successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("data_points", dataPoints),
		attribute.Float64("query_duration_seconds", stats.QueryDuration),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

// cacheGpuUsageHistory caches GPU usage history for 24 hours
func (j *GpuHistoryCache24hJob) cacheGpuUsageHistory(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	clusterName string,
	cacheFacade database.GenericCacheFacadeInterface,
) (int, error) {
	span, ctx := trace.StartSpanFromContext(ctx, "cacheGpuUsageHistory")
	defer trace.FinishSpan(span)

	endTime := time.Now()
	startTime := endTime.Add(-HistoryDuration)

	span.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("time.start", startTime.Format(time.RFC3339)),
		attribute.String("time.end", endTime.Format(time.RFC3339)),
		attribute.Int("history.step_seconds", DefaultHistoryStep),
	)

	// Get usage history
	usageSpan, usageCtx := trace.StartSpanFromContext(ctx, "getHistoryGpuUsage")
	usageSpan.SetAttributes(
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
		attribute.Int("step_seconds", DefaultHistoryStep),
	)

	queryStart := time.Now()
	usageHistory, err := gpu.GetHistoryGpuUsage(usageCtx, storageClientSet, metadata.GpuVendorAMD, startTime, endTime, DefaultHistoryStep)
	if err != nil {
		usageSpan.RecordError(err)
		usageSpan.SetAttributes(attribute.String("error.message", err.Error()))
		usageSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(usageSpan)

		log.Warnf("Failed to get history GPU usage: %v, using empty array", err)
		usageHistory = []model.TimePoint{}
	} else {
		duration := time.Since(queryStart)
		usageSpan.SetAttributes(
			attribute.Int("data_points", len(usageHistory)),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		usageSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(usageSpan)
	}

	// Get allocation history
	allocationSpan, allocationCtx := trace.StartSpanFromContext(ctx, "getHistoryGpuAllocationRate")
	allocationSpan.SetAttributes(
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
		attribute.Int("step_seconds", DefaultHistoryStep),
	)

	queryStart = time.Now()
	allocationHistory, err := gpu.GetHistoryGpuAllocationRate(allocationCtx, storageClientSet, metadata.GpuVendorAMD, startTime, endTime, DefaultHistoryStep)
	if err != nil {
		allocationSpan.RecordError(err)
		allocationSpan.SetAttributes(attribute.String("error.message", err.Error()))
		allocationSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(allocationSpan)

		log.Warnf("Failed to get history GPU allocation rate: %v, using empty array", err)
		allocationHistory = []model.TimePoint{}
	} else {
		duration := time.Since(queryStart)
		allocationSpan.SetAttributes(
			attribute.Int("data_points", len(allocationHistory)),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		allocationSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(allocationSpan)
	}

	// Get VRAM utilization history
	vramSpan, vramCtx := trace.StartSpanFromContext(ctx, "getNodeGpuVramUsageHistory")
	vramSpan.SetAttributes(
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
		attribute.Int("step_seconds", DefaultHistoryStep),
	)

	queryStart = time.Now()
	vramUtilizationHistory, err := gpu.GetNodeGpuVramUsageHistory(vramCtx, storageClientSet, metadata.GpuVendorAMD, startTime, endTime, DefaultHistoryStep)
	if err != nil {
		vramSpan.RecordError(err)
		vramSpan.SetAttributes(attribute.String("error.message", err.Error()))
		vramSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(vramSpan)

		log.Warnf("Failed to get node GPU VRAM usage history: %v, using empty array", err)
		vramUtilizationHistory = []model.TimePoint{}
	} else {
		duration := time.Since(queryStart)
		vramSpan.SetAttributes(
			attribute.Int("data_points", len(vramUtilizationHistory)),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		vramSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(vramSpan)
	}

	// Build result
	result := model.GpuUtilizationHistory{
		AllocationRate:  allocationHistory,
		Utilization:     usageHistory,
		VramUtilization: vramUtilizationHistory,
	}

	// Store in cache
	cacheSpan, cacheCtx := trace.StartSpanFromContext(ctx, "setCacheData")
	cacheSpan.SetAttributes(
		attribute.String("cache.key", CacheKeyGpuUsageHistory24h),
		attribute.Int("cache.expiration_minutes", int(CacheExpirationDuration.Minutes())),
	)

	cacheKey := CacheKeyGpuUsageHistory24h
	expiresAt := time.Now().Add(CacheExpirationDuration)

	cacheStart := time.Now()
	if err := cacheFacade.Set(cacheCtx, cacheKey, result, &expiresAt); err != nil {
		cacheSpan.RecordError(err)
		cacheSpan.SetAttributes(attribute.String("error.message", err.Error()))
		cacheSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(cacheSpan)

		span.SetStatus(codes.Error, "Failed to set cache")
		return 0, fmt.Errorf("failed to set cache for GPU usage history (24h): %w", err)
	}

	duration := time.Since(cacheStart)
	cacheSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	cacheSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(cacheSpan)

	span.SetAttributes(
		attribute.Int("usage.data_points", len(usageHistory)),
		attribute.Int("allocation.data_points", len(allocationHistory)),
		attribute.Int("vram.data_points", len(vramUtilizationHistory)),
	)
	span.SetStatus(codes.Ok, "")
	return len(usageHistory), nil
}

// Schedule returns the cron schedule for this job
func (j *GpuHistoryCache24hJob) Schedule() string {
	// Run every 10 minutes - lowest frequency for 24 hour history
	return "@every 10m"
}
