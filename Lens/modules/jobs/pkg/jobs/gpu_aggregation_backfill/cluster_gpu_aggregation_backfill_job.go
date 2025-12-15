package gpu_aggregation_backfill

import (
	"context"
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
)

const (
	// DefaultClusterBackfillDays is the default number of days to backfill for cluster stats
	DefaultClusterBackfillDays = 7

	// DefaultClusterBatchSize is the default batch size for processing hours
	DefaultClusterBatchSize = 24
)

// ClusterBackfillFacadeGetter is the function signature for getting database facade
type ClusterBackfillFacadeGetter func(clusterName string) database.FacadeInterface

// ClusterBackfillClusterNameGetter is the function signature for getting cluster name
type ClusterBackfillClusterNameGetter func() string

// ClusterBackfillAllocationCalculatorFactory creates an allocation calculator
type ClusterBackfillAllocationCalculatorFactory func(clusterName string) ClusterBackfillAllocationCalculatorInterface

// ClusterBackfillUtilizationQueryFunc queries cluster-level GPU utilization statistics
type ClusterBackfillUtilizationQueryFunc func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error)

// ClusterBackfillAllocationCalculatorInterface defines the interface for cluster GPU allocation calculation
type ClusterBackfillAllocationCalculatorInterface interface {
	CalculateHourlyGpuAllocation(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error)
}

// ClusterGpuAggregationBackfillConfig is the configuration for cluster backfill job
type ClusterGpuAggregationBackfillConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// BackfillDays is the number of days to scan for missing data
	BackfillDays int `json:"backfill_days"`

	// BatchSize is the number of hours to process in each batch
	BatchSize int `json:"batch_size"`
}

// ClusterGpuAggregationBackfillJob is the job for backfilling missing cluster GPU aggregation data
type ClusterGpuAggregationBackfillJob struct {
	config                      *ClusterGpuAggregationBackfillConfig
	clusterName                 string
	facadeGetter                ClusterBackfillFacadeGetter
	clusterNameGetter           ClusterBackfillClusterNameGetter
	allocationCalculatorFactory ClusterBackfillAllocationCalculatorFactory
	utilizationQueryFunc        ClusterBackfillUtilizationQueryFunc
}

// ClusterBackfillJobOption is a function that configures a ClusterGpuAggregationBackfillJob
type ClusterBackfillJobOption func(*ClusterGpuAggregationBackfillJob)

// WithClusterBackfillFacadeGetter sets the facade getter function
func WithClusterBackfillFacadeGetter(getter ClusterBackfillFacadeGetter) ClusterBackfillJobOption {
	return func(j *ClusterGpuAggregationBackfillJob) {
		j.facadeGetter = getter
	}
}

// WithClusterBackfillClusterNameGetter sets the cluster name getter function
func WithClusterBackfillClusterNameGetter(getter ClusterBackfillClusterNameGetter) ClusterBackfillJobOption {
	return func(j *ClusterGpuAggregationBackfillJob) {
		j.clusterNameGetter = getter
	}
}

// WithClusterBackfillClusterName sets the cluster name directly
func WithClusterBackfillClusterName(name string) ClusterBackfillJobOption {
	return func(j *ClusterGpuAggregationBackfillJob) {
		j.clusterName = name
	}
}

// WithClusterBackfillAllocationCalculatorFactory sets the allocation calculator factory
func WithClusterBackfillAllocationCalculatorFactory(factory ClusterBackfillAllocationCalculatorFactory) ClusterBackfillJobOption {
	return func(j *ClusterGpuAggregationBackfillJob) {
		j.allocationCalculatorFactory = factory
	}
}

// WithClusterBackfillUtilizationQueryFunc sets the utilization query function
func WithClusterBackfillUtilizationQueryFunc(fn ClusterBackfillUtilizationQueryFunc) ClusterBackfillJobOption {
	return func(j *ClusterGpuAggregationBackfillJob) {
		j.utilizationQueryFunc = fn
	}
}

// defaultClusterBackfillFacadeGetter is the default implementation
func defaultClusterBackfillFacadeGetter(clusterName string) database.FacadeInterface {
	return database.GetFacadeForCluster(clusterName)
}

// defaultClusterBackfillClusterNameGetter is the default implementation
func defaultClusterBackfillClusterNameGetter() string {
	return clientsets.GetClusterManager().GetCurrentClusterName()
}

// defaultClusterBackfillAllocationCalculatorFactory is the default implementation
func defaultClusterBackfillAllocationCalculatorFactory(clusterName string) ClusterBackfillAllocationCalculatorInterface {
	return statistics.NewGpuAllocationCalculator(clusterName)
}

// defaultClusterBackfillUtilizationQueryFunc is the default implementation
func defaultClusterBackfillUtilizationQueryFunc(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
	return statistics.QueryClusterHourlyGpuUtilizationStats(ctx, storageClientSet, hour)
}

// NewClusterGpuAggregationBackfillJob creates a new cluster backfill job with default config
func NewClusterGpuAggregationBackfillJob(opts ...ClusterBackfillJobOption) *ClusterGpuAggregationBackfillJob {
	j := &ClusterGpuAggregationBackfillJob{
		config: &ClusterGpuAggregationBackfillConfig{
			Enabled:      true,
			BackfillDays: DefaultClusterBackfillDays,
			BatchSize:    DefaultClusterBatchSize,
		},
		facadeGetter:                defaultClusterBackfillFacadeGetter,
		clusterNameGetter:           defaultClusterBackfillClusterNameGetter,
		allocationCalculatorFactory: defaultClusterBackfillAllocationCalculatorFactory,
		utilizationQueryFunc:        defaultClusterBackfillUtilizationQueryFunc,
	}

	for _, opt := range opts {
		opt(j)
	}

	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
}

// NewClusterGpuAggregationBackfillJobWithConfig creates a new cluster backfill job with custom config
func NewClusterGpuAggregationBackfillJobWithConfig(cfg *ClusterGpuAggregationBackfillConfig, opts ...ClusterBackfillJobOption) *ClusterGpuAggregationBackfillJob {
	j := &ClusterGpuAggregationBackfillJob{
		config:                      cfg,
		facadeGetter:                defaultClusterBackfillFacadeGetter,
		clusterNameGetter:           defaultClusterBackfillClusterNameGetter,
		allocationCalculatorFactory: defaultClusterBackfillAllocationCalculatorFactory,
		utilizationQueryFunc:        defaultClusterBackfillUtilizationQueryFunc,
	}

	for _, opt := range opts {
		opt(j)
	}

	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
}

// Run executes the cluster backfill job
func (j *ClusterGpuAggregationBackfillJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "cluster_gpu_aggregation_backfill_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = j.clusterNameGetter()
	}

	span.SetAttributes(
		attribute.String("job.name", "cluster_gpu_aggregation_backfill"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("config.backfill_days", j.config.BackfillDays),
	)

	if !j.config.Enabled {
		log.Debugf("Cluster GPU aggregation backfill job is disabled")
		stats.AddMessage("Cluster GPU aggregation backfill job is disabled")
		return stats, nil
	}

	// Calculate time range
	// Exclude current hour to avoid conflict with ongoing aggregation
	// e.g., if now is 18:30, endTime should be 17:00 (last completed hour)
	endTime := time.Now().Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-time.Duration(j.config.BackfillDays) * 24 * time.Hour)

	log.Infof("Starting cluster GPU aggregation backfill job for cluster: %s, time range: %v to %v (excluding current hour)",
		clusterName, startTime, endTime)

	// 1. Generate all hours in the time range
	allHours := generateAllHours(startTime, endTime)
	log.Infof("Generated %d hours to check for cluster backfill", len(allHours))

	if len(allHours) == 0 {
		log.Infof("No hours to process")
		stats.AddMessage("No hours to process")
		return stats, nil
	}

	// 2. Find missing cluster stats
	missingSpan, missingCtx := trace.StartSpanFromContext(ctx, "findMissingClusterStats")
	missingClusterHours, err := j.findMissingClusterStats(missingCtx, clusterName, allHours)
	if err != nil {
		missingSpan.RecordError(err)
		missingSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(missingSpan)
		return stats, fmt.Errorf("failed to find missing cluster stats: %w", err)
	}
	missingSpan.SetAttributes(attribute.Int("missing.cluster_hours", len(missingClusterHours)))
	missingSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(missingSpan)

	log.Infof("Found %d missing cluster hours", len(missingClusterHours))
	stats.AddCustomMetric("missing_cluster_hours", len(missingClusterHours))

	// 3. Backfill cluster stats using time-weighted calculation
	if len(missingClusterHours) > 0 {
		backfillSpan, backfillCtx := trace.StartSpanFromContext(ctx, "backfillClusterStats")
		backfillSpan.SetAttributes(attribute.Int("hours.count", len(missingClusterHours)))

		count, backfillErr := j.backfillClusterStats(backfillCtx, clusterName, missingClusterHours, storageClientSet)
		if backfillErr != nil {
			backfillSpan.RecordError(backfillErr)
			backfillSpan.SetStatus(codes.Error, backfillErr.Error())
			trace.FinishSpan(backfillSpan)
			stats.ErrorCount++
			log.Errorf("Failed to backfill cluster stats: %v", backfillErr)
		} else {
			backfillSpan.SetAttributes(attribute.Int64("backfilled.count", count))
			backfillSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(backfillSpan)
			stats.ItemsCreated = count
			log.Infof("Backfilled %d cluster hourly stats", count)
		}
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	stats.AddMessage(fmt.Sprintf("Cluster backfill completed: %d cluster stats created", stats.ItemsCreated))

	log.Infof("Cluster GPU aggregation backfill job completed in %v", totalDuration)
	return stats, nil
}

// findMissingClusterStats finds hours that are missing cluster stats
func (j *ClusterGpuAggregationBackfillJob) findMissingClusterStats(
	ctx context.Context,
	clusterName string,
	allHours []time.Time) ([]time.Time, error) {

	if len(allHours) == 0 {
		return nil, nil
	}

	facade := j.facadeGetter(clusterName).GetGpuAggregation()

	startTime := allHours[0]
	endTime := allHours[len(allHours)-1].Add(time.Hour)

	// Get existing cluster stats
	clusterStats, err := facade.GetClusterHourlyStats(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster hourly stats: %w", err)
	}

	// Debug logging to help diagnose the issue
	log.Debugf("Backfill query range: %v to %v", startTime, endTime)
	log.Debugf("Database returned %d existing cluster stats records", len(clusterStats))
	if len(clusterStats) > 0 {
		log.Debugf("First existing record time: %v", clusterStats[0].StatHour)
		log.Debugf("Last existing record time: %v", clusterStats[len(clusterStats)-1].StatHour)
	}
	log.Debugf("Total hours to check: %d", len(allHours))

	// Find missing hours using helper function
	missingClusterHours := FindMissingClusterHours(allHours, clusterStats)

	return missingClusterHours, nil
}

// FindMissingClusterHours finds hours that are missing from existing stats
// This is exported for testing purposes
func FindMissingClusterHours(allHours []time.Time, existingStats []*dbmodel.ClusterGpuHourlyStats) []time.Time {
	existingClusterHours := make(map[time.Time]struct{})
	for _, stat := range existingStats {
		existingClusterHours[stat.StatHour.Truncate(time.Hour)] = struct{}{}
	}

	missingClusterHours := make([]time.Time, 0)
	for _, hour := range allHours {
		if _, exists := existingClusterHours[hour]; !exists {
			missingClusterHours = append(missingClusterHours, hour)
		}
	}

	return missingClusterHours
}

// backfillClusterStats backfills missing cluster hourly stats using time-weighted calculation
func (j *ClusterGpuAggregationBackfillJob) backfillClusterStats(
	ctx context.Context,
	clusterName string,
	missingHours []time.Time,
	storageClientSet *clientsets.StorageClientSet) (int64, error) {

	if len(missingHours) == 0 {
		return 0, nil
	}

	facade := j.facadeGetter(clusterName).GetGpuAggregation()
	calculator := j.allocationCalculatorFactory(clusterName)
	var createdCount int64

	// Get cluster GPU capacity once (reuse for all hours)
	totalCapacity, err := j.getClusterGpuCapacity(ctx, clusterName)
	if err != nil {
		log.Warnf("Failed to get cluster GPU capacity: %v", err)
		totalCapacity = 0
	}

	for _, hour := range missingHours {
		log.Debugf("Processing cluster GPU aggregation backfill for hour %v", hour)
		// Use time-weighted calculation from statistics package
		result, err := calculator.CalculateHourlyGpuAllocation(ctx, hour)
		if err != nil {
			log.Warnf("Failed to calculate GPU allocation for hour %v: %v", hour, err)
			// Create zero-value stats on error
			result = &statistics.GpuAllocationResult{}
		}

		// Query cluster-level GPU utilization statistics from Prometheus
		utilizationStats, err := j.utilizationQueryFunc(ctx, storageClientSet, hour)
		if err != nil {
			log.Warnf("Failed to query cluster GPU utilization for hour %v: %v", hour, err)
			utilizationStats = &statistics.ClusterGpuUtilizationStats{}
		}

		// Build cluster stats
		var clusterStats *dbmodel.ClusterGpuHourlyStats
		if result.WorkloadCount == 0 {
			// No workload data for this hour, fill with zero values
			clusterStats = CreateZeroClusterStats(clusterName, hour)
			log.Debugf("Creating zero-value cluster stats for hour %v (no workload data)", hour)
		} else {
			// Build cluster stats from time-weighted calculation result
			clusterStats = BuildClusterStatsFromResult(clusterName, hour, result)
		}

		// Set GPU utilization statistics from Prometheus query
		clusterStats.AvgUtilization = utilizationStats.AvgUtilization
		clusterStats.MaxUtilization = utilizationStats.MaxUtilization
		clusterStats.MinUtilization = utilizationStats.MinUtilization
		clusterStats.P50Utilization = utilizationStats.P50Utilization
		clusterStats.P95Utilization = utilizationStats.P95Utilization

		// Set GPU capacity and calculate allocation rate
		clusterStats.TotalGpuCapacity = int32(totalCapacity)
		if totalCapacity > 0 && clusterStats.AllocatedGpuCount > 0 {
			clusterStats.AllocationRate = (clusterStats.AllocatedGpuCount / float64(totalCapacity)) * 100
		}

		// Save cluster stats
		if err := facade.SaveClusterHourlyStats(ctx, clusterStats); err != nil {
			log.Errorf("Failed to save cluster stats for hour %v: %v", hour, err)
			continue
		}

		createdCount++
		log.Debugf("Backfilled cluster stats for hour %v: allocated=%.2f, workloads=%d, utilization=%.2f%% (max=%.2f%%, min=%.2f%%)",
			hour, clusterStats.AllocatedGpuCount, result.WorkloadCount,
			clusterStats.AvgUtilization, clusterStats.MaxUtilization, clusterStats.MinUtilization)
	}

	return createdCount, nil
}

// BuildClusterStatsFromResult builds ClusterGpuHourlyStats from time-weighted calculation result
// This is exported for testing purposes
func BuildClusterStatsFromResult(
	clusterName string,
	hour time.Time,
	result *statistics.GpuAllocationResult) *dbmodel.ClusterGpuHourlyStats {

	return &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       clusterName,
		StatHour:          hour,
		AllocatedGpuCount: result.TotalAllocatedGpu,
		SampleCount:       int32(result.WorkloadCount),
	}
}

// CreateZeroClusterStats creates a cluster stats record with zero values
// This is exported for testing purposes
func CreateZeroClusterStats(
	clusterName string,
	hour time.Time) *dbmodel.ClusterGpuHourlyStats {

	return &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       clusterName,
		StatHour:          hour,
		TotalGpuCapacity:  0,
		AllocatedGpuCount: 0,
		AllocationRate:    0,
		AvgUtilization:    0,
		MaxUtilization:    0,
		MinUtilization:    0,
		P50Utilization:    0,
		P95Utilization:    0,
		SampleCount:       0,
	}
}

// getClusterGpuCapacity gets the total GPU capacity of the cluster
func (j *ClusterGpuAggregationBackfillJob) getClusterGpuCapacity(ctx context.Context, clusterName string) (int, error) {
	// Query all GPU nodes from database and sum capacity
	readyStatus := "Ready"
	nodes, _, err := j.facadeGetter(clusterName).GetNode().
		SearchNode(ctx, filter.NodeFilter{
			K8sStatus: &readyStatus,
			Limit:     10000,
		})

	if err != nil {
		return 0, fmt.Errorf("failed to query nodes: %w", err)
	}

	totalCapacity := CalculateClusterBackfillGpuCapacity(nodes)
	return totalCapacity, nil
}

// CalculateClusterBackfillGpuCapacity calculates total GPU capacity from nodes
// This is exported for testing purposes
func CalculateClusterBackfillGpuCapacity(nodes []*dbmodel.Node) int {
	totalCapacity := 0
	for _, node := range nodes {
		if node.GpuCount > 0 {
			totalCapacity += int(node.GpuCount)
		}
	}
	return totalCapacity
}

// Schedule returns the job's scheduling expression
func (j *ClusterGpuAggregationBackfillJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *ClusterGpuAggregationBackfillJob) SetConfig(cfg *ClusterGpuAggregationBackfillConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *ClusterGpuAggregationBackfillJob) GetConfig() *ClusterGpuAggregationBackfillConfig {
	return j.config
}
