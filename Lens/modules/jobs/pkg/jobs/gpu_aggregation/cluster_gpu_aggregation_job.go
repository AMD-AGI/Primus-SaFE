package gpu_aggregation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
)

const (
	// CacheKeyClusterGpuAggregationLastHour is the cache key for storing the last processed hour
	CacheKeyClusterGpuAggregationLastHour = "job.cluster_gpu_aggregation.last_processed_hour"
)

// ClusterGpuAggregationConfig is the configuration for cluster GPU aggregation job
type ClusterGpuAggregationConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`
}

// ClusterGpuAggregationJob aggregates cluster-level GPU statistics using time-weighted calculation
type ClusterGpuAggregationJob struct {
	config      *ClusterGpuAggregationConfig
	clusterName string
}

// NewClusterGpuAggregationJob creates a new cluster GPU aggregation job
func NewClusterGpuAggregationJob() *ClusterGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &ClusterGpuAggregationJob{
		config: &ClusterGpuAggregationConfig{
			Enabled: true,
		},
		clusterName: clusterName,
	}
}

// NewClusterGpuAggregationJobWithConfig creates a new cluster GPU aggregation job with custom config
func NewClusterGpuAggregationJobWithConfig(cfg *ClusterGpuAggregationConfig) *ClusterGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &ClusterGpuAggregationJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the cluster GPU aggregation job
func (j *ClusterGpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "cluster_gpu_aggregation_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "cluster_gpu_aggregation"),
		attribute.String("cluster.name", clusterName),
	)

	if !j.config.Enabled {
		log.Debugf("Cluster GPU aggregation job is disabled")
		stats.AddMessage("Cluster GPU aggregation job is disabled")
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

		log.Infof("Processing cluster aggregation for hour: %v (last processed: %v)", hourToProcess, lastProcessedHour)

		aggSpan, aggCtx := trace.StartSpanFromContext(ctx, "aggregateClusterStats")
		aggSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hourToProcess.Format(time.RFC3339)),
		)

		if err := j.aggregateClusterStats(aggCtx, clusterName, hourToProcess, storageClientSet); err != nil {
			aggSpan.RecordError(err)
			aggSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(aggSpan)
			stats.ErrorCount++
			log.Errorf("Failed to aggregate cluster stats: %v", err)
		} else {
			aggSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(aggSpan)
			stats.ItemsCreated++
			stats.AddMessage(fmt.Sprintf("Aggregated cluster stats for %v", hourToProcess))

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
func (j *ClusterGpuAggregationJob) getLastProcessedHour(ctx context.Context, clusterName string) (time.Time, error) {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	var lastHourStr string
	err := cacheFacade.Get(ctx, CacheKeyClusterGpuAggregationLastHour, &lastHourStr)
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
func (j *ClusterGpuAggregationJob) setLastProcessedHour(ctx context.Context, clusterName string, hour time.Time) error {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	hourStr := hour.Format(time.RFC3339)
	return cacheFacade.Set(ctx, CacheKeyClusterGpuAggregationLastHour, hourStr, nil)
}

// aggregateClusterStats aggregates cluster-level statistics using time-weighted calculation
func (j *ClusterGpuAggregationJob) aggregateClusterStats(
	ctx context.Context,
	clusterName string,
	hour time.Time,
	storageClientSet *clientsets.StorageClientSet) error {

	// Use time-weighted calculation from statistics package
	calculator := statistics.NewGpuAllocationCalculator(clusterName)
	result, err := calculator.CalculateHourlyGpuAllocation(ctx, hour)
	if err != nil {
		return fmt.Errorf("failed to calculate GPU allocation: %w", err)
	}

	// Get cluster GPU capacity
	totalCapacity, err := j.getClusterGpuCapacity(ctx, clusterName)
	if err != nil {
		log.Warnf("Failed to get cluster GPU capacity: %v", err)
		totalCapacity = 0
	}

	// Build cluster stats from calculation result
	clusterStats := &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       clusterName,
		StatHour:          hour,
		TotalGpuCapacity:  int32(totalCapacity),
		AllocatedGpuCount: result.TotalAllocatedGpu,
		SampleCount:       int32(result.WorkloadCount),
	}

	// Calculate allocation rate
	if totalCapacity > 0 && clusterStats.AllocatedGpuCount > 0 {
		clusterStats.AllocationRate = (clusterStats.AllocatedGpuCount / float64(totalCapacity)) * 100
	}

	// Query cluster-level GPU utilization from Prometheus using avg(gpu_utilization{})
	avgUtilization, err := statistics.QueryClusterHourlyGpuUtilization(ctx, storageClientSet, hour)
	if err != nil {
		log.Warnf("Failed to query cluster GPU utilization: %v", err)
		avgUtilization = 0
	}
	clusterStats.AvgUtilization = avgUtilization

	// Save cluster stats
	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	if err := facade.SaveClusterHourlyStats(ctx, clusterStats); err != nil {
		return fmt.Errorf("failed to save cluster stats: %w", err)
	}

	log.Infof("Cluster stats saved for %s at %v: allocated=%.2f/%d, rate=%.2f%%, utilization=%.2f%%",
		clusterName, hour, clusterStats.AllocatedGpuCount, totalCapacity, clusterStats.AllocationRate, avgUtilization)

	return nil
}

// getClusterGpuCapacity gets the total GPU capacity of the cluster
func (j *ClusterGpuAggregationJob) getClusterGpuCapacity(ctx context.Context, clusterName string) (int, error) {
	readyStatus := "Ready"
	nodes, _, err := database.GetFacadeForCluster(clusterName).GetNode().
		SearchNode(ctx, filter.NodeFilter{
			K8sStatus: &readyStatus,
			Limit:     10000,
		})

	if err != nil {
		return 0, fmt.Errorf("failed to query nodes: %w", err)
	}

	totalCapacity := 0
	for _, node := range nodes {
		if node.GpuCount > 0 {
			totalCapacity += int(node.GpuCount)
		}
	}

	return totalCapacity, nil
}

// Schedule returns the job's scheduling expression
func (j *ClusterGpuAggregationJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *ClusterGpuAggregationJob) SetConfig(cfg *ClusterGpuAggregationConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *ClusterGpuAggregationJob) GetConfig() *ClusterGpuAggregationConfig {
	return j.config
}
