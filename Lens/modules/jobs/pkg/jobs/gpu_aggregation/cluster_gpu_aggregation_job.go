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

// ClusterFacadeGetter is the function signature for getting database facade
type ClusterFacadeGetter func(clusterName string) database.FacadeInterface

// ClusterClusterNameGetter is the function signature for getting cluster name
type ClusterClusterNameGetter func() string

// ClusterAllocationCalculatorFactory creates a ClusterAllocationCalculatorInterface
type ClusterAllocationCalculatorFactory func(clusterName string) ClusterAllocationCalculatorInterface

// ClusterUtilizationQueryFunc queries cluster-level GPU utilization statistics
type ClusterUtilizationQueryFunc func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error)

// ClusterAllocationCalculatorInterface defines the interface for cluster GPU allocation calculation
type ClusterAllocationCalculatorInterface interface {
	CalculateHourlyGpuAllocation(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error)
}

// ClusterGpuAggregationConfig is the configuration for cluster GPU aggregation job
type ClusterGpuAggregationConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`
}

// ClusterGpuAggregationJob aggregates cluster-level GPU statistics using time-weighted calculation
type ClusterGpuAggregationJob struct {
	config                      *ClusterGpuAggregationConfig
	clusterName                 string
	facadeGetter                ClusterFacadeGetter
	clusterNameGetter           ClusterClusterNameGetter
	allocationCalculatorFactory ClusterAllocationCalculatorFactory
	utilizationQueryFunc        ClusterUtilizationQueryFunc
}

// ClusterJobOption is a function that configures a ClusterGpuAggregationJob
type ClusterJobOption func(*ClusterGpuAggregationJob)

// WithClusterFacadeGetter sets the facade getter function
func WithClusterFacadeGetter(getter ClusterFacadeGetter) ClusterJobOption {
	return func(j *ClusterGpuAggregationJob) {
		j.facadeGetter = getter
	}
}

// WithClusterClusterNameGetter sets the cluster name getter function
func WithClusterClusterNameGetter(getter ClusterClusterNameGetter) ClusterJobOption {
	return func(j *ClusterGpuAggregationJob) {
		j.clusterNameGetter = getter
	}
}

// WithClusterClusterName sets the cluster name directly
func WithClusterClusterName(name string) ClusterJobOption {
	return func(j *ClusterGpuAggregationJob) {
		j.clusterName = name
	}
}

// WithClusterAllocationCalculatorFactory sets the allocation calculator factory
func WithClusterAllocationCalculatorFactory(factory ClusterAllocationCalculatorFactory) ClusterJobOption {
	return func(j *ClusterGpuAggregationJob) {
		j.allocationCalculatorFactory = factory
	}
}

// WithClusterUtilizationQueryFunc sets the utilization query function
func WithClusterUtilizationQueryFunc(fn ClusterUtilizationQueryFunc) ClusterJobOption {
	return func(j *ClusterGpuAggregationJob) {
		j.utilizationQueryFunc = fn
	}
}

// defaultClusterFacadeGetter is the default implementation using database package
func defaultClusterFacadeGetter(clusterName string) database.FacadeInterface {
	return database.GetFacadeForCluster(clusterName)
}

// defaultClusterClusterNameGetter is the default implementation using clientsets package
func defaultClusterClusterNameGetter() string {
	return clientsets.GetClusterManager().GetCurrentClusterName()
}

// defaultClusterAllocationCalculatorFactory is the default implementation
func defaultClusterAllocationCalculatorFactory(clusterName string) ClusterAllocationCalculatorInterface {
	return statistics.NewGpuAllocationCalculator(clusterName)
}

// defaultClusterUtilizationQueryFunc is the default implementation
func defaultClusterUtilizationQueryFunc(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
	return statistics.QueryClusterHourlyGpuUtilizationStats(ctx, storageClientSet, hour)
}

// NewClusterGpuAggregationJob creates a new cluster GPU aggregation job
func NewClusterGpuAggregationJob(opts ...ClusterJobOption) *ClusterGpuAggregationJob {
	j := &ClusterGpuAggregationJob{
		config: &ClusterGpuAggregationConfig{
			Enabled: true,
		},
		facadeGetter:                defaultClusterFacadeGetter,
		clusterNameGetter:           defaultClusterClusterNameGetter,
		allocationCalculatorFactory: defaultClusterAllocationCalculatorFactory,
		utilizationQueryFunc:        defaultClusterUtilizationQueryFunc,
	}

	// Apply options
	for _, opt := range opts {
		opt(j)
	}

	// Set cluster name if not already set
	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
}

// NewClusterGpuAggregationJobWithConfig creates a new cluster GPU aggregation job with custom config
func NewClusterGpuAggregationJobWithConfig(cfg *ClusterGpuAggregationConfig, opts ...ClusterJobOption) *ClusterGpuAggregationJob {
	j := &ClusterGpuAggregationJob{
		config:                      cfg,
		facadeGetter:                defaultClusterFacadeGetter,
		clusterNameGetter:           defaultClusterClusterNameGetter,
		allocationCalculatorFactory: defaultClusterAllocationCalculatorFactory,
		utilizationQueryFunc:        defaultClusterUtilizationQueryFunc,
	}

	// Apply options
	for _, opt := range opts {
		opt(j)
	}

	// Set cluster name if not already set
	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
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
		clusterName = j.clusterNameGetter()
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
	cacheFacade := j.facadeGetter(clusterName).GetGenericCache()

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
	cacheFacade := j.facadeGetter(clusterName).GetGenericCache()

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
	calculator := j.allocationCalculatorFactory(clusterName)
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

	// Query cluster-level GPU utilization statistics from Prometheus
	utilizationStats, err := j.utilizationQueryFunc(ctx, storageClientSet, hour)
	if err != nil {
		log.Warnf("Failed to query cluster GPU utilization: %v", err)
		utilizationStats = &statistics.ClusterGpuUtilizationStats{}
	}

	// Build cluster stats from calculation result
	clusterStats := BuildClusterGpuHourlyStats(clusterName, hour, result, totalCapacity, utilizationStats)

	// Save cluster stats
	facade := j.facadeGetter(clusterName).GetGpuAggregation()
	if err := facade.SaveClusterHourlyStats(ctx, clusterStats); err != nil {
		return fmt.Errorf("failed to save cluster stats: %w", err)
	}

	log.Infof("Cluster stats saved for %s at %v: allocated=%.2f/%d, rate=%.2f%%, utilization=%.2f%% (max=%.2f%%, min=%.2f%%)",
		clusterName, hour, clusterStats.AllocatedGpuCount, totalCapacity, clusterStats.AllocationRate,
		clusterStats.AvgUtilization, clusterStats.MaxUtilization, clusterStats.MinUtilization)

	return nil
}

// BuildClusterGpuHourlyStats builds cluster GPU hourly stats record
// This is exported for testing purposes
func BuildClusterGpuHourlyStats(
	clusterName string,
	hour time.Time,
	allocationResult *statistics.GpuAllocationResult,
	totalCapacity int,
	utilizationStats *statistics.ClusterGpuUtilizationStats,
) *dbmodel.ClusterGpuHourlyStats {
	clusterStats := &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       clusterName,
		StatHour:          hour,
		TotalGpuCapacity:  int32(totalCapacity),
		AllocatedGpuCount: allocationResult.TotalAllocatedGpu,
		SampleCount:       int32(allocationResult.WorkloadCount),
		AvgUtilization:    utilizationStats.AvgUtilization,
		MaxUtilization:    utilizationStats.MaxUtilization,
		MinUtilization:    utilizationStats.MinUtilization,
		P50Utilization:    utilizationStats.P50Utilization,
		P95Utilization:    utilizationStats.P95Utilization,
	}

	// Calculate allocation rate
	if totalCapacity > 0 && clusterStats.AllocatedGpuCount > 0 {
		clusterStats.AllocationRate = (clusterStats.AllocatedGpuCount / float64(totalCapacity)) * 100
	}

	return clusterStats
}

// getClusterGpuCapacity gets the total GPU capacity of the cluster
func (j *ClusterGpuAggregationJob) getClusterGpuCapacity(ctx context.Context, clusterName string) (int, error) {
	readyStatus := "Ready"
	nodes, _, err := j.facadeGetter(clusterName).GetNode().
		SearchNode(ctx, filter.NodeFilter{
			K8sStatus: &readyStatus,
			Limit:     10000,
		})

	if err != nil {
		return 0, fmt.Errorf("failed to query nodes: %w", err)
	}

	totalCapacity := CalculateClusterGpuCapacity(nodes)
	return totalCapacity, nil
}

// CalculateClusterGpuCapacity calculates total GPU capacity from nodes
// This is exported for testing purposes
func CalculateClusterGpuCapacity(nodes []*dbmodel.Node) int {
	totalCapacity := 0
	for _, node := range nodes {
		if node.GpuCount > 0 {
			totalCapacity += int(node.GpuCount)
		}
	}
	return totalCapacity
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
