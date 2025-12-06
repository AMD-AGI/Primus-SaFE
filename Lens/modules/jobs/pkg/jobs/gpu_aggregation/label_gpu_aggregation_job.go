package gpu_aggregation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
)

const (
	// CacheKeyLabelGpuAggregationLastHour is the cache key for storing the last processed hour
	CacheKeyLabelGpuAggregationLastHour = "job.label_gpu_aggregation.last_processed_hour"
)

// LabelGpuAggregationConfig is the configuration for label GPU aggregation job
type LabelGpuAggregationConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// LabelKeys is the list of label keys to aggregate
	LabelKeys []string `json:"label_keys"`

	// AnnotationKeys is the list of annotation keys to aggregate
	AnnotationKeys []string `json:"annotation_keys"`

	// DefaultValue is the default value when label/annotation is not found
	DefaultValue string `json:"default_value"`

	// PromQueryStep is the step for Prometheus queries (in seconds)
	PromQueryStep int `json:"prom_query_step"`
}

// LabelGpuAggregationJob aggregates GPU statistics by label/annotation dimensions
type LabelGpuAggregationJob struct {
	config      *LabelGpuAggregationConfig
	clusterName string
}

// NewLabelGpuAggregationJob creates a new label GPU aggregation job
func NewLabelGpuAggregationJob() *LabelGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &LabelGpuAggregationJob{
		config: &LabelGpuAggregationConfig{
			Enabled:        true,
			LabelKeys:      []string{},
			AnnotationKeys: []string{},
			DefaultValue:   "unknown",
			PromQueryStep:  DefaultPromQueryStep,
		},
		clusterName: clusterName,
	}
}

// NewLabelGpuAggregationJobWithConfig creates a new label GPU aggregation job with custom config
func NewLabelGpuAggregationJobWithConfig(cfg *LabelGpuAggregationConfig) *LabelGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &LabelGpuAggregationJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the label GPU aggregation job
func (j *LabelGpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "label_gpu_aggregation_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "label_gpu_aggregation"),
		attribute.String("cluster.name", clusterName),
	)

	if !j.config.Enabled {
		log.Debugf("Label GPU aggregation job is disabled")
		stats.AddMessage("Label GPU aggregation job is disabled")
		return stats, nil
	}

	// Check if there are any keys to aggregate
	if len(j.config.LabelKeys) == 0 && len(j.config.AnnotationKeys) == 0 {
		log.Debugf("No label or annotation keys configured for aggregation")
		stats.AddMessage("No label or annotation keys configured")
		return stats, nil
	}

	// Get the last processed hour from cache
	lastProcessedHour, err := j.getLastProcessedHour(ctx, clusterName)
	if err != nil {
		// If no cache entry exists, use current hour minus 1
		lastProcessedHour = time.Now().Truncate(time.Hour).Add(-time.Hour)
		log.Infof("No last processed hour found in cache, using default: %v", lastProcessedHour)
	}

	// Check if hour has changed - aggregate data for the previous hour
	now := time.Now()
	currentHour := now.Truncate(time.Hour)
	previousHour := currentHour.Add(-time.Hour)

	// If the last processed hour is before the previous hour, we need to aggregate
	if lastProcessedHour.Before(previousHour) || lastProcessedHour.Equal(previousHour) {
		hourToProcess := previousHour
		if lastProcessedHour.Before(previousHour) {
			// Process the hour after the last processed hour
			hourToProcess = lastProcessedHour.Add(time.Hour)
		}

		log.Infof("Processing label aggregation for hour: %v (last processed: %v)", hourToProcess, lastProcessedHour)

		aggSpan, aggCtx := trace.StartSpanFromContext(ctx, "aggregateLabelStats")
		aggSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hourToProcess.Format(time.RFC3339)),
		)

		count, err := j.aggregateLabelStats(aggCtx, clusterName, hourToProcess, storageClientSet)
		if err != nil {
			aggSpan.RecordError(err)
			aggSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(aggSpan)
			stats.ErrorCount++
			log.Errorf("Failed to aggregate label stats: %v", err)
		} else {
			aggSpan.SetAttributes(attribute.Int64("labels.count", count))
			aggSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(aggSpan)
			stats.ItemsCreated = count
			stats.AddMessage(fmt.Sprintf("Aggregated %d label stats for %v", count, hourToProcess))

			// Update the last processed hour in cache
			if err := j.setLastProcessedHour(ctx, clusterName, hourToProcess); err != nil {
				log.Warnf("Failed to update last processed hour in cache: %v", err)
			}
		}
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	return stats, nil
}

// getLastProcessedHour retrieves the last processed hour from cache
func (j *LabelGpuAggregationJob) getLastProcessedHour(ctx context.Context, clusterName string) (time.Time, error) {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	var lastHourStr string
	err := cacheFacade.Get(ctx, CacheKeyLabelGpuAggregationLastHour, &lastHourStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return time.Time{}, fmt.Errorf("no cache entry found")
		}
		return time.Time{}, err
	}

	lastHour, err := time.Parse(time.RFC3339, lastHourStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse last processed hour: %w", err)
	}

	return lastHour, nil
}

// setLastProcessedHour stores the last processed hour in cache
func (j *LabelGpuAggregationJob) setLastProcessedHour(ctx context.Context, clusterName string, hour time.Time) error {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	hourStr := hour.Format(time.RFC3339)
	return cacheFacade.Set(ctx, CacheKeyLabelGpuAggregationLastHour, hourStr, nil)
}

// aggregateLabelStats aggregates label/annotation-level statistics
func (j *LabelGpuAggregationJob) aggregateLabelStats(
	ctx context.Context,
	clusterName string,
	hour time.Time,
	storageClientSet *clientsets.StorageClientSet) (int64, error) {

	// Create calculator with configuration
	calculator := statistics.NewLabelAggregationCalculator(clusterName, &statistics.LabelAggregationConfig{
		LabelKeys:      j.config.LabelKeys,
		AnnotationKeys: j.config.AnnotationKeys,
		DefaultValue:   j.config.DefaultValue,
	})

	// Calculate aggregation
	summary, err := calculator.CalculateHourlyLabelAggregation(ctx, hour)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate label aggregation: %w", err)
	}

	log.Infof("Found %d active top-level workloads for label aggregation at hour %v", summary.TotalWorkloads, hour)

	if len(summary.Results) == 0 {
		return 0, nil
	}

	hourStart := hour
	hourEnd := hour.Add(time.Hour)

	// Query utilization for each aggregation and save to database
	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	var createdCount int64

	for _, agg := range summary.Results {
		// Query utilization for all workloads in this aggregation
		for _, workloadUID := range agg.WorkloadUIDs {
			utilizationValues, err := j.queryWorkloadUtilizationForHour(ctx, storageClientSet, workloadUID, hourStart, hourEnd)
			if err != nil {
				log.Warnf("Failed to query utilization for workload %s: %v", workloadUID, err)
				continue
			}
			agg.AddUtilizationValues(utilizationValues)
		}

		// Calculate utilization statistics
		utilizationStats := agg.CalculateUtilizationStats()

		// Build label hourly stats
		stats := &dbmodel.LabelGpuHourlyStats{
			ClusterName:         clusterName,
			DimensionType:       agg.DimensionType,
			DimensionKey:        agg.DimensionKey,
			DimensionValue:      agg.DimensionValue,
			StatHour:            hour,
			AllocatedGpuCount:   agg.TotalAllocatedGpu,
			ActiveWorkloadCount: int32(agg.ActiveWorkloadCount),
			AvgUtilization:      utilizationStats.AvgUtilization,
			MaxUtilization:      utilizationStats.MaxUtilization,
			MinUtilization:      utilizationStats.MinUtilization,
		}

		// Save to database
		if err := facade.SaveLabelHourlyStats(ctx, stats); err != nil {
			log.Errorf("Failed to save label stats for %s:%s=%s at %v: %v",
				agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour, err)
			continue
		}

		createdCount++
		log.Debugf("Label stats saved for %s:%s=%s at %v: allocated=%.2f, utilization=%.2f%%, workloads=%d",
			agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour,
			stats.AllocatedGpuCount, stats.AvgUtilization, agg.ActiveWorkloadCount)
	}

	return createdCount, nil
}

// queryWorkloadUtilizationForHour queries the GPU utilization for a workload in a specific hour
func (j *LabelGpuAggregationJob) queryWorkloadUtilizationForHour(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string,
	startTime, endTime time.Time) ([]float64, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "queryWorkloadUtilizationForHour")
	defer trace.FinishSpan(span)

	query := fmt.Sprintf(WorkloadUtilizationQueryTemplate, workloadUID)

	span.SetAttributes(
		attribute.String("workload.uid", workloadUID),
		attribute.String("prometheus.query", query),
		attribute.String("start_time", startTime.Format(time.RFC3339)),
		attribute.String("end_time", endTime.Format(time.RFC3339)),
	)

	series, err := prom.QueryRange(ctx, storageClientSet, query, startTime, endTime,
		j.config.PromQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		span.SetAttributes(attribute.Int("data_points.count", 0))
		span.SetStatus(codes.Ok, "No data points")
		return []float64{}, nil
	}

	// Collect all data points
	values := make([]float64, 0, len(series[0].Values))
	for _, point := range series[0].Values {
		values = append(values, point.Value)
	}

	span.SetAttributes(
		attribute.Int("series.count", len(series)),
		attribute.Int("data_points.count", len(values)),
	)
	span.SetStatus(codes.Ok, "")

	return values, nil
}

// Schedule returns the job's scheduling expression
func (j *LabelGpuAggregationJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *LabelGpuAggregationJob) SetConfig(cfg *LabelGpuAggregationConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *LabelGpuAggregationJob) GetConfig() *LabelGpuAggregationConfig {
	return j.config
}
