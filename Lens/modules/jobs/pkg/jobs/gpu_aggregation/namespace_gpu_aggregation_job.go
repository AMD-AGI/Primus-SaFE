// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_aggregation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
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
	// CacheKeyNamespaceGpuAggregationLastHour is the cache key for storing the last processed hour
	CacheKeyNamespaceGpuAggregationLastHour = "job.namespace_gpu_aggregation.last_processed_hour"
)

// SystemNamespaces is the list of system namespaces to exclude by default
var SystemNamespaces = []string{"kube-system", "kube-public", "kube-node-lease"}

// NamespaceFacadeGetter is the function signature for getting database facade
type NamespaceFacadeGetter func(clusterName string) database.FacadeInterface

// NamespaceClusterNameGetter is the function signature for getting cluster name
type NamespaceClusterNameGetter func() string

// AllocationCalculatorFactory creates a GpuAllocationCalculator
type AllocationCalculatorFactory func(clusterName string) AllocationCalculatorInterface

// UtilizationCalculatorFactory creates a NamespaceUtilizationCalculator
type UtilizationCalculatorFactory func(clusterName string, storageClientSet *clientsets.StorageClientSet) UtilizationCalculatorInterface

// AllocationCalculatorInterface defines the interface for GPU allocation calculation
type AllocationCalculatorInterface interface {
	CalculateHourlyNamespaceGpuAllocation(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error)
}

// UtilizationCalculatorInterface defines the interface for utilization calculation
type UtilizationCalculatorInterface interface {
	CalculateHourlyNamespaceUtilization(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult
}

// NamespaceGpuAggregationConfig is the configuration for namespace GPU aggregation job
type NamespaceGpuAggregationConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// ExcludeNamespaces is the list of namespaces to exclude
	ExcludeNamespaces []string `json:"exclude_namespaces"`

	// IncludeSystemNamespaces controls whether to include system namespaces
	IncludeSystemNamespaces bool `json:"include_system_namespaces"`
}

// NamespaceGpuAggregationJob aggregates namespace-level GPU statistics using time-weighted calculation
type NamespaceGpuAggregationJob struct {
	config                       *NamespaceGpuAggregationConfig
	clusterName                  string
	facadeGetter                 NamespaceFacadeGetter
	clusterNameGetter            NamespaceClusterNameGetter
	allocationCalculatorFactory  AllocationCalculatorFactory
	utilizationCalculatorFactory UtilizationCalculatorFactory
}

// NamespaceJobOption is a function that configures a NamespaceGpuAggregationJob
type NamespaceJobOption func(*NamespaceGpuAggregationJob)

// WithNamespaceFacadeGetter sets the facade getter function
func WithNamespaceFacadeGetter(getter NamespaceFacadeGetter) NamespaceJobOption {
	return func(j *NamespaceGpuAggregationJob) {
		j.facadeGetter = getter
	}
}

// WithNamespaceClusterNameGetter sets the cluster name getter function
func WithNamespaceClusterNameGetter(getter NamespaceClusterNameGetter) NamespaceJobOption {
	return func(j *NamespaceGpuAggregationJob) {
		j.clusterNameGetter = getter
	}
}

// WithNamespaceClusterName sets the cluster name directly
func WithNamespaceClusterName(name string) NamespaceJobOption {
	return func(j *NamespaceGpuAggregationJob) {
		j.clusterName = name
	}
}

// WithAllocationCalculatorFactory sets the allocation calculator factory
func WithAllocationCalculatorFactory(factory AllocationCalculatorFactory) NamespaceJobOption {
	return func(j *NamespaceGpuAggregationJob) {
		j.allocationCalculatorFactory = factory
	}
}

// WithUtilizationCalculatorFactory sets the utilization calculator factory
func WithUtilizationCalculatorFactory(factory UtilizationCalculatorFactory) NamespaceJobOption {
	return func(j *NamespaceGpuAggregationJob) {
		j.utilizationCalculatorFactory = factory
	}
}

// defaultNamespaceFacadeGetter is the default implementation using database package
func defaultNamespaceFacadeGetter(clusterName string) database.FacadeInterface {
	return database.GetFacadeForCluster(clusterName)
}

// defaultNamespaceClusterNameGetter is the default implementation using clientsets package
func defaultNamespaceClusterNameGetter() string {
	return clientsets.GetClusterManager().GetCurrentClusterName()
}

// defaultAllocationCalculatorFactory is the default implementation
func defaultAllocationCalculatorFactory(clusterName string) AllocationCalculatorInterface {
	return statistics.NewGpuAllocationCalculator(clusterName)
}

// defaultUtilizationCalculatorFactory is the default implementation
func defaultUtilizationCalculatorFactory(clusterName string, storageClientSet *clientsets.StorageClientSet) UtilizationCalculatorInterface {
	return statistics.NewNamespaceUtilizationCalculator(clusterName, storageClientSet)
}

// NewNamespaceGpuAggregationJob creates a new namespace GPU aggregation job
func NewNamespaceGpuAggregationJob(opts ...NamespaceJobOption) *NamespaceGpuAggregationJob {
	j := &NamespaceGpuAggregationJob{
		config: &NamespaceGpuAggregationConfig{
			Enabled:                 true,
			ExcludeNamespaces:       []string{},
			IncludeSystemNamespaces: false,
		},
		facadeGetter:                 defaultNamespaceFacadeGetter,
		clusterNameGetter:            defaultNamespaceClusterNameGetter,
		allocationCalculatorFactory:  defaultAllocationCalculatorFactory,
		utilizationCalculatorFactory: defaultUtilizationCalculatorFactory,
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

// NewNamespaceGpuAggregationJobWithConfig creates a new namespace GPU aggregation job with custom config
func NewNamespaceGpuAggregationJobWithConfig(cfg *NamespaceGpuAggregationConfig, opts ...NamespaceJobOption) *NamespaceGpuAggregationJob {
	j := &NamespaceGpuAggregationJob{
		config:                       cfg,
		facadeGetter:                 defaultNamespaceFacadeGetter,
		clusterNameGetter:            defaultNamespaceClusterNameGetter,
		allocationCalculatorFactory:  defaultAllocationCalculatorFactory,
		utilizationCalculatorFactory: defaultUtilizationCalculatorFactory,
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

// Run executes the namespace GPU aggregation job
func (j *NamespaceGpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "namespace_gpu_aggregation_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = j.clusterNameGetter()
	}

	span.SetAttributes(
		attribute.String("job.name", "namespace_gpu_aggregation"),
		attribute.String("cluster.name", clusterName),
	)

	if !j.config.Enabled {
		log.Debugf("Namespace GPU aggregation job is disabled")
		stats.AddMessage("Namespace GPU aggregation job is disabled")
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

		log.Infof("Processing namespace aggregation for hour: %v (last processed: %v)", hourToProcess, lastProcessedHour)

		aggSpan, aggCtx := trace.StartSpanFromContext(ctx, "aggregateNamespaceStats")
		aggSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hourToProcess.Format(time.RFC3339)),
		)

		count, err := j.aggregateNamespaceStats(aggCtx, clusterName, hourToProcess, storageClientSet)
		if err != nil {
			aggSpan.RecordError(err)
			aggSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(aggSpan)
			stats.ErrorCount++
			log.Errorf("Failed to aggregate namespace stats: %v", err)
		} else {
			aggSpan.SetAttributes(attribute.Int64("namespaces.count", count))
			aggSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(aggSpan)
			stats.ItemsCreated = count
			stats.AddMessage(fmt.Sprintf("Aggregated %d namespace stats for %v", count, hourToProcess))

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
func (j *NamespaceGpuAggregationJob) getLastProcessedHour(ctx context.Context, clusterName string) (time.Time, error) {
	cacheFacade := j.facadeGetter(clusterName).GetGenericCache()

	var lastHourStr string
	err := cacheFacade.Get(ctx, CacheKeyNamespaceGpuAggregationLastHour, &lastHourStr)
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
func (j *NamespaceGpuAggregationJob) setLastProcessedHour(ctx context.Context, clusterName string, hour time.Time) error {
	cacheFacade := j.facadeGetter(clusterName).GetGenericCache()

	hourStr := hour.Format(time.RFC3339)
	return cacheFacade.Set(ctx, CacheKeyNamespaceGpuAggregationLastHour, hourStr, nil)
}

// aggregateNamespaceStats aggregates namespace-level statistics using time-weighted calculation
func (j *NamespaceGpuAggregationJob) aggregateNamespaceStats(
	ctx context.Context,
	clusterName string,
	hour time.Time,
	storageClientSet *clientsets.StorageClientSet) (int64, error) {

	// Get all namespaces from namespace_info table
	namespaceInfoList, err := j.facadeGetter(clusterName).GetNamespaceInfo().List(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list namespace info: %w", err)
	}

	// Build namespace list
	namespaces := make([]string, 0, len(namespaceInfoList))
	for _, nsInfo := range namespaceInfoList {
		if !ShouldExcludeNamespace(nsInfo.Name, j.config.ExcludeNamespaces, j.config.IncludeSystemNamespaces) {
			namespaces = append(namespaces, nsInfo.Name)
		}
	}

	// Calculate GPU capacity per namespace from active node_namespace_mapping + node.gpu_count
	namespaceQuotas, err := j.facadeGetter(clusterName).GetNodeNamespaceMapping().GetNamespaceGpuCapacityMap(ctx)
	if err != nil {
		log.Warnf("Failed to get namespace GPU capacity from node mappings, falling back to namespace_info: %v", err)
		namespaceQuotas = make(map[string]int32)
		for _, nsInfo := range namespaceInfoList {
			namespaceQuotas[nsInfo.Name] = nsInfo.GpuResource
		}
	}

	log.Infof("Aggregating stats for %d namespaces at hour %v", len(namespaces), hour)

	allocationCalculator := j.allocationCalculatorFactory(clusterName)
	utilizationCalculator := j.utilizationCalculatorFactory(clusterName, storageClientSet)
	facade := j.facadeGetter(clusterName).GetGpuAggregation()
	var createdCount int64

	for _, namespace := range namespaces {
		// Use time-weighted calculation for this namespace
		result, err := allocationCalculator.CalculateHourlyNamespaceGpuAllocation(ctx, namespace, hour)
		if err != nil {
			log.Warnf("Failed to calculate GPU allocation for namespace %s at hour %v: %v",
				namespace, hour, err)
			result = &statistics.GpuAllocationResult{}
		}

		// Query utilization from Prometheus using the shared calculator
		utilizationResult := utilizationCalculator.CalculateHourlyNamespaceUtilization(ctx, namespace, result, hour)

		// Build namespace stats
		nsStats := BuildNamespaceGpuHourlyStats(clusterName, namespace, hour, result, utilizationResult, namespaceQuotas[namespace])

		// Save namespace stats
		if err := facade.SaveNamespaceHourlyStats(ctx, nsStats); err != nil {
			log.Errorf("Failed to save namespace stats for %s at %v: %v", namespace, hour, err)
			continue
		}

		createdCount++
		log.Debugf("Namespace stats saved for %s at %v: allocated=%.2f, workloads=%d, avg_util=%.2f%%",
			namespace, hour, nsStats.AllocatedGpuCount, result.WorkloadCount, nsStats.AvgUtilization)
	}

	return createdCount, nil
}

// BuildNamespaceGpuHourlyStats builds a namespace GPU hourly stats record
// This is exported for testing purposes
func BuildNamespaceGpuHourlyStats(
	clusterName string,
	namespace string,
	hour time.Time,
	allocationResult *statistics.GpuAllocationResult,
	utilizationResult *statistics.NamespaceUtilizationResult,
	gpuQuota int32,
) *dbmodel.NamespaceGpuHourlyStats {
	nsStats := &dbmodel.NamespaceGpuHourlyStats{
		ClusterName:         clusterName,
		Namespace:           namespace,
		StatHour:            hour,
		AllocatedGpuCount:   allocationResult.TotalAllocatedGpu,
		ActiveWorkloadCount: int32(allocationResult.WorkloadCount),
	}

	// Set GPU quota if available and calculate allocation rate
	if gpuQuota > 0 {
		nsStats.TotalGpuCapacity = gpuQuota
		if nsStats.AllocatedGpuCount > 0 {
			nsStats.AllocationRate = (nsStats.AllocatedGpuCount / float64(gpuQuota)) * 100
		}
	}

	// Set utilization stats
	if utilizationResult != nil {
		nsStats.AvgUtilization = utilizationResult.AvgUtilization
		nsStats.MinUtilization = utilizationResult.MinUtilization
		nsStats.MaxUtilization = utilizationResult.MaxUtilization
	}

	return nsStats
}

// ShouldExcludeNamespace checks if a namespace should be excluded
// This is exported for testing purposes
func ShouldExcludeNamespace(namespace string, excludeList []string, includeSystemNamespaces bool) bool {
	// Check if in exclusion list
	for _, excluded := range excludeList {
		if namespace == excluded {
			return true
		}
	}

	// Check if it's a system namespace
	if !includeSystemNamespaces {
		for _, sysNs := range SystemNamespaces {
			if namespace == sysNs {
				return true
			}
		}
	}

	return false
}

// Schedule returns the job's scheduling expression
func (j *NamespaceGpuAggregationJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *NamespaceGpuAggregationJob) SetConfig(cfg *NamespaceGpuAggregationConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *NamespaceGpuAggregationJob) GetConfig() *NamespaceGpuAggregationConfig {
	return j.config
}
