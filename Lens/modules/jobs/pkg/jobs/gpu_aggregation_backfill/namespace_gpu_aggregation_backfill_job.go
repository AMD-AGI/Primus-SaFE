package gpu_aggregation_backfill

import (
	"context"
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
)

const (
	// DefaultNamespaceBackfillDays is the default number of days to backfill for namespace stats
	DefaultNamespaceBackfillDays = 7

	// DefaultNamespaceBatchSize is the default batch size for processing hours
	DefaultNamespaceBatchSize = 24
)

// NamespaceBackfillSystemNamespaces is the list of system namespaces
var NamespaceBackfillSystemNamespaces = []string{"kube-system", "kube-public", "kube-node-lease"}

// NamespaceBackfillFacadeGetter is the function signature for getting database facade
type NamespaceBackfillFacadeGetter func(clusterName string) database.FacadeInterface

// NamespaceBackfillClusterNameGetter is the function signature for getting cluster name
type NamespaceBackfillClusterNameGetter func() string

// NamespaceBackfillAllocationCalculatorFactory creates an allocation calculator
type NamespaceBackfillAllocationCalculatorFactory func(clusterName string) NamespaceBackfillAllocationCalculatorInterface

// NamespaceBackfillUtilizationCalculatorFactory creates a utilization calculator
type NamespaceBackfillUtilizationCalculatorFactory func(clusterName string, storageClientSet *clientsets.StorageClientSet) NamespaceBackfillUtilizationCalculatorInterface

// NamespaceBackfillAllocationCalculatorInterface defines the interface for namespace GPU allocation calculation
type NamespaceBackfillAllocationCalculatorInterface interface {
	CalculateHourlyNamespaceGpuAllocation(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error)
}

// NamespaceBackfillUtilizationCalculatorInterface defines the interface for namespace utilization calculation
type NamespaceBackfillUtilizationCalculatorInterface interface {
	CalculateHourlyNamespaceUtilization(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult
}

// NamespaceGpuAggregationBackfillConfig is the configuration for namespace backfill job
type NamespaceGpuAggregationBackfillConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// BackfillDays is the number of days to scan for missing data
	BackfillDays int `json:"backfill_days"`

	// BatchSize is the number of hours to process in each batch
	BatchSize int `json:"batch_size"`

	// ExcludeNamespaces is the list of namespaces to exclude from backfill
	ExcludeNamespaces []string `json:"exclude_namespaces"`

	// IncludeSystemNamespaces controls whether to include system namespaces
	IncludeSystemNamespaces bool `json:"include_system_namespaces"`
}

// NamespaceGpuAggregationBackfillJob is the job for backfilling missing namespace GPU aggregation data
type NamespaceGpuAggregationBackfillJob struct {
	config                       *NamespaceGpuAggregationBackfillConfig
	clusterName                  string
	facadeGetter                 NamespaceBackfillFacadeGetter
	clusterNameGetter            NamespaceBackfillClusterNameGetter
	allocationCalculatorFactory  NamespaceBackfillAllocationCalculatorFactory
	utilizationCalculatorFactory NamespaceBackfillUtilizationCalculatorFactory
}

// NamespaceBackfillJobOption is a function that configures a NamespaceGpuAggregationBackfillJob
type NamespaceBackfillJobOption func(*NamespaceGpuAggregationBackfillJob)

// WithNamespaceBackfillFacadeGetter sets the facade getter function
func WithNamespaceBackfillFacadeGetter(getter NamespaceBackfillFacadeGetter) NamespaceBackfillJobOption {
	return func(j *NamespaceGpuAggregationBackfillJob) {
		j.facadeGetter = getter
	}
}

// WithNamespaceBackfillClusterNameGetter sets the cluster name getter function
func WithNamespaceBackfillClusterNameGetter(getter NamespaceBackfillClusterNameGetter) NamespaceBackfillJobOption {
	return func(j *NamespaceGpuAggregationBackfillJob) {
		j.clusterNameGetter = getter
	}
}

// WithNamespaceBackfillClusterName sets the cluster name directly
func WithNamespaceBackfillClusterName(name string) NamespaceBackfillJobOption {
	return func(j *NamespaceGpuAggregationBackfillJob) {
		j.clusterName = name
	}
}

// WithNamespaceBackfillAllocationCalculatorFactory sets the allocation calculator factory
func WithNamespaceBackfillAllocationCalculatorFactory(factory NamespaceBackfillAllocationCalculatorFactory) NamespaceBackfillJobOption {
	return func(j *NamespaceGpuAggregationBackfillJob) {
		j.allocationCalculatorFactory = factory
	}
}

// WithNamespaceBackfillUtilizationCalculatorFactory sets the utilization calculator factory
func WithNamespaceBackfillUtilizationCalculatorFactory(factory NamespaceBackfillUtilizationCalculatorFactory) NamespaceBackfillJobOption {
	return func(j *NamespaceGpuAggregationBackfillJob) {
		j.utilizationCalculatorFactory = factory
	}
}

// defaultNamespaceBackfillFacadeGetter is the default implementation
func defaultNamespaceBackfillFacadeGetter(clusterName string) database.FacadeInterface {
	return database.GetFacadeForCluster(clusterName)
}

// defaultNamespaceBackfillClusterNameGetter is the default implementation
func defaultNamespaceBackfillClusterNameGetter() string {
	return clientsets.GetClusterManager().GetCurrentClusterName()
}

// defaultNamespaceBackfillAllocationCalculatorFactory is the default implementation
func defaultNamespaceBackfillAllocationCalculatorFactory(clusterName string) NamespaceBackfillAllocationCalculatorInterface {
	return statistics.NewGpuAllocationCalculator(clusterName)
}

// defaultNamespaceBackfillUtilizationCalculatorFactory is the default implementation
func defaultNamespaceBackfillUtilizationCalculatorFactory(clusterName string, storageClientSet *clientsets.StorageClientSet) NamespaceBackfillUtilizationCalculatorInterface {
	return statistics.NewNamespaceUtilizationCalculator(clusterName, storageClientSet)
}

// NewNamespaceGpuAggregationBackfillJob creates a new namespace backfill job with default config
func NewNamespaceGpuAggregationBackfillJob(opts ...NamespaceBackfillJobOption) *NamespaceGpuAggregationBackfillJob {
	j := &NamespaceGpuAggregationBackfillJob{
		config: &NamespaceGpuAggregationBackfillConfig{
			Enabled:                 true,
			BackfillDays:            DefaultNamespaceBackfillDays,
			BatchSize:               DefaultNamespaceBatchSize,
			ExcludeNamespaces:       []string{},
			IncludeSystemNamespaces: false,
		},
		facadeGetter:                 defaultNamespaceBackfillFacadeGetter,
		clusterNameGetter:            defaultNamespaceBackfillClusterNameGetter,
		allocationCalculatorFactory:  defaultNamespaceBackfillAllocationCalculatorFactory,
		utilizationCalculatorFactory: defaultNamespaceBackfillUtilizationCalculatorFactory,
	}

	for _, opt := range opts {
		opt(j)
	}

	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
}

// NewNamespaceGpuAggregationBackfillJobWithConfig creates a new namespace backfill job with custom config
func NewNamespaceGpuAggregationBackfillJobWithConfig(cfg *NamespaceGpuAggregationBackfillConfig, opts ...NamespaceBackfillJobOption) *NamespaceGpuAggregationBackfillJob {
	j := &NamespaceGpuAggregationBackfillJob{
		config:                       cfg,
		facadeGetter:                 defaultNamespaceBackfillFacadeGetter,
		clusterNameGetter:            defaultNamespaceBackfillClusterNameGetter,
		allocationCalculatorFactory:  defaultNamespaceBackfillAllocationCalculatorFactory,
		utilizationCalculatorFactory: defaultNamespaceBackfillUtilizationCalculatorFactory,
	}

	for _, opt := range opts {
		opt(j)
	}

	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
}

// Run executes the namespace backfill job
func (j *NamespaceGpuAggregationBackfillJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "namespace_gpu_aggregation_backfill_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = j.clusterNameGetter()
	}

	span.SetAttributes(
		attribute.String("job.name", "namespace_gpu_aggregation_backfill"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("config.backfill_days", j.config.BackfillDays),
	)

	if !j.config.Enabled {
		log.Debugf("Namespace GPU aggregation backfill job is disabled")
		stats.AddMessage("Namespace GPU aggregation backfill job is disabled")
		return stats, nil
	}

	// Calculate time range
	// Exclude current hour to avoid conflict with ongoing aggregation
	endTime := time.Now().Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-time.Duration(j.config.BackfillDays) * 24 * time.Hour)

	log.Infof("Starting namespace GPU aggregation backfill job for cluster: %s, time range: %v to %v (excluding current hour)",
		clusterName, startTime, endTime)

	// 1. Generate all hours in the time range
	allHours := generateAllHours(startTime, endTime)
	log.Infof("Generated %d hours to check for namespace backfill", len(allHours))

	if len(allHours) == 0 {
		log.Infof("No hours to process")
		stats.AddMessage("No hours to process")
		return stats, nil
	}

	// 2. Find missing namespace stats for all hours
	missingSpan, missingCtx := trace.StartSpanFromContext(ctx, "findMissingNamespaceStats")
	missingNamespaceHours, err := j.findMissingNamespaceStats(missingCtx, clusterName, allHours)
	if err != nil {
		missingSpan.RecordError(err)
		missingSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(missingSpan)
		return stats, fmt.Errorf("failed to find missing namespace stats: %w", err)
	}
	missingSpan.SetAttributes(attribute.Int("missing.namespace_hours", len(missingNamespaceHours)))
	missingSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(missingSpan)

	log.Infof("Found %d missing namespace hours", len(missingNamespaceHours))
	stats.AddCustomMetric("missing_namespace_hours", len(missingNamespaceHours))

	// 3. Backfill namespace stats using time-weighted calculation
	if len(missingNamespaceHours) > 0 {
		backfillSpan, backfillCtx := trace.StartSpanFromContext(ctx, "backfillNamespaceStats")
		backfillSpan.SetAttributes(attribute.Int("hours.count", len(missingNamespaceHours)))

		count, backfillErr := j.backfillNamespaceStats(backfillCtx, clusterName, missingNamespaceHours, storageClientSet)
		if backfillErr != nil {
			backfillSpan.RecordError(backfillErr)
			backfillSpan.SetStatus(codes.Error, backfillErr.Error())
			trace.FinishSpan(backfillSpan)
			stats.ErrorCount++
			log.Errorf("Failed to backfill namespace stats: %v", backfillErr)
		} else {
			backfillSpan.SetAttributes(attribute.Int64("backfilled.count", count))
			backfillSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(backfillSpan)
			stats.ItemsCreated = count
			log.Infof("Backfilled %d namespace hourly stats", count)
		}
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	stats.AddMessage(fmt.Sprintf("Namespace backfill completed: %d namespace stats created", stats.ItemsCreated))

	log.Infof("Namespace GPU aggregation backfill job completed in %v", totalDuration)
	return stats, nil
}

// findMissingNamespaceStats finds hours and namespaces that are missing stats
// Uses namespace_info table as the source of truth for namespace list
func (j *NamespaceGpuAggregationBackfillJob) findMissingNamespaceStats(
	ctx context.Context,
	clusterName string,
	allHours []time.Time) (map[time.Time][]string, error) {

	if len(allHours) == 0 {
		return nil, nil
	}

	facade := j.facadeGetter(clusterName).GetGpuAggregation()

	startTime := allHours[0]
	endTime := allHours[len(allHours)-1].Add(time.Hour)

	// Get all namespaces from namespace_info table (source of truth)
	namespaceInfoList, err := j.facadeGetter(clusterName).GetNamespaceInfo().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespace info: %w", err)
	}

	// Build namespace list from namespace_info
	allNamespaces := make([]string, 0, len(namespaceInfoList))
	for _, nsInfo := range namespaceInfoList {
		if !ShouldExcludeNamespaceBackfill(nsInfo.Name, j.config.ExcludeNamespaces, j.config.IncludeSystemNamespaces) {
			allNamespaces = append(allNamespaces, nsInfo.Name)
		}
	}

	log.Infof("Found %d namespaces from namespace_info table", len(allNamespaces))

	// Get existing namespace stats
	namespaceStats, err := facade.ListNamespaceHourlyStats(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespace hourly stats: %w", err)
	}

	// Find missing namespace hours using helper function
	missingNamespaceHours := FindMissingNamespaceHours(allHours, allNamespaces, namespaceStats)

	return missingNamespaceHours, nil
}

// ShouldExcludeNamespaceBackfill checks if a namespace should be excluded from backfill
// This is exported for testing purposes
func ShouldExcludeNamespaceBackfill(namespace string, excludeList []string, includeSystemNamespaces bool) bool {
	// Check if in exclusion list
	for _, excluded := range excludeList {
		if namespace == excluded {
			return true
		}
	}

	// Check if it's a system namespace
	if !includeSystemNamespaces {
		for _, sysNs := range NamespaceBackfillSystemNamespaces {
			if namespace == sysNs {
				return true
			}
		}
	}

	return false
}

// FindMissingNamespaceHours finds hours and namespaces that are missing from existing stats
// This is exported for testing purposes
func FindMissingNamespaceHours(allHours []time.Time, allNamespaces []string, existingStats []*dbmodel.NamespaceGpuHourlyStats) map[time.Time][]string {
	// Build hour -> namespaces map for existing stats
	existingNamespaceHours := make(map[time.Time]map[string]struct{})
	for _, stat := range existingStats {
		hour := stat.StatHour.Truncate(time.Hour)
		if _, exists := existingNamespaceHours[hour]; !exists {
			existingNamespaceHours[hour] = make(map[string]struct{})
		}
		existingNamespaceHours[hour][stat.Namespace] = struct{}{}
	}

	// Find missing namespace hours
	missingNamespaceHours := make(map[time.Time][]string)
	for _, hour := range allHours {
		existingNamespaces := existingNamespaceHours[hour]

		for _, namespace := range allNamespaces {
			// Check if already exists
			if existingNamespaces != nil {
				if _, exists := existingNamespaces[namespace]; exists {
					continue
				}
			}

			// Missing namespace for this hour
			if missingNamespaceHours[hour] == nil {
				missingNamespaceHours[hour] = make([]string, 0)
			}
			missingNamespaceHours[hour] = append(missingNamespaceHours[hour], namespace)
		}
	}

	return missingNamespaceHours
}

// backfillNamespaceStats backfills missing namespace hourly stats using time-weighted calculation
func (j *NamespaceGpuAggregationBackfillJob) backfillNamespaceStats(
	ctx context.Context,
	clusterName string,
	missingNamespaceHours map[time.Time][]string,
	storageClientSet *clientsets.StorageClientSet) (int64, error) {

	if len(missingNamespaceHours) == 0 {
		return 0, nil
	}

	facade := j.facadeGetter(clusterName).GetGpuAggregation()
	allocationCalculator := j.allocationCalculatorFactory(clusterName)
	utilizationCalculator := j.utilizationCalculatorFactory(clusterName, storageClientSet)
	var createdCount int64

	// Get namespace GPU quotas
	namespaceQuotas, err := j.getNamespaceGpuQuotas(ctx, clusterName)
	if err != nil {
		log.Warnf("Failed to get namespace GPU quotas: %v", err)
		namespaceQuotas = make(map[string]int32)
	}

	for hour, namespaces := range missingNamespaceHours {
		// Create namespace stats for each missing namespace
		for _, namespace := range namespaces {
			// Use time-weighted calculation for this namespace
			result, err := allocationCalculator.CalculateHourlyNamespaceGpuAllocation(ctx, namespace, hour)
			if err != nil {
				log.Warnf("Failed to calculate GPU allocation for namespace %s at hour %v: %v", namespace, hour, err)
				result = &statistics.GpuAllocationResult{}
			}

			var nsStats *dbmodel.NamespaceGpuHourlyStats
			if result.WorkloadCount == 0 {
				// No workload data for this namespace in this hour, fill with zero values
				nsStats = CreateZeroNamespaceStats(clusterName, namespace, hour)
				log.Debugf("Creating zero-value namespace stats for %s at %v (no workload data)", namespace, hour)
			} else {
				// Build namespace stats from time-weighted calculation result
				utilizationResult := utilizationCalculator.CalculateHourlyNamespaceUtilization(ctx, namespace, result, hour)
				nsStats = BuildNamespaceStatsFromResult(clusterName, namespace, hour, result, utilizationResult)
			}

			// Set GPU quota if available and calculate allocation rate
			if quota, exists := namespaceQuotas[namespace]; exists && quota > 0 {
				nsStats.TotalGpuCapacity = quota
				if nsStats.AllocatedGpuCount > 0 {
					nsStats.AllocationRate = (nsStats.AllocatedGpuCount / float64(quota)) * 100
				}
			}

			// Save namespace stats
			if err := facade.SaveNamespaceHourlyStats(ctx, nsStats); err != nil {
				log.Errorf("Failed to save namespace stats for %s at %v: %v", namespace, hour, err)
				continue
			}

			createdCount++
			log.Debugf("Backfilled namespace stats for %s at %v: allocated=%.2f, workloads=%d, avg_util=%.2f%%",
				namespace, hour, nsStats.AllocatedGpuCount, result.WorkloadCount, nsStats.AvgUtilization)
		}
	}

	return createdCount, nil
}

// BuildNamespaceStatsFromResult builds NamespaceGpuHourlyStats from time-weighted calculation result
// This is exported for testing purposes
func BuildNamespaceStatsFromResult(
	clusterName, namespace string,
	hour time.Time,
	result *statistics.GpuAllocationResult,
	utilizationResult *statistics.NamespaceUtilizationResult) *dbmodel.NamespaceGpuHourlyStats {

	stats := &dbmodel.NamespaceGpuHourlyStats{
		ClusterName:         clusterName,
		Namespace:           namespace,
		StatHour:            hour,
		AllocatedGpuCount:   result.TotalAllocatedGpu,
		ActiveWorkloadCount: int32(result.WorkloadCount),
	}

	if utilizationResult != nil {
		stats.AvgUtilization = utilizationResult.AvgUtilization
		stats.MinUtilization = utilizationResult.MinUtilization
		stats.MaxUtilization = utilizationResult.MaxUtilization
	}

	return stats
}

// CreateZeroNamespaceStats creates a namespace stats record with zero values
// This is exported for testing purposes
func CreateZeroNamespaceStats(
	clusterName, namespace string,
	hour time.Time) *dbmodel.NamespaceGpuHourlyStats {

	return &dbmodel.NamespaceGpuHourlyStats{
		ClusterName:         clusterName,
		Namespace:           namespace,
		StatHour:            hour,
		TotalGpuCapacity:    0,
		AllocatedGpuCount:   0,
		AllocationRate:      0,
		AvgUtilization:      0,
		MaxUtilization:      0,
		MinUtilization:      0,
		ActiveWorkloadCount: 0,
	}
}

// getNamespaceGpuQuotas gets the GPU quotas for all namespaces
func (j *NamespaceGpuAggregationBackfillJob) getNamespaceGpuQuotas(ctx context.Context, clusterName string) (map[string]int32, error) {
	namespaceInfoList, err := j.facadeGetter(clusterName).GetNamespaceInfo().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespace info: %w", err)
	}

	quotas := make(map[string]int32)
	for _, nsInfo := range namespaceInfoList {
		quotas[nsInfo.Name] = nsInfo.GpuResource
	}

	return quotas, nil
}

// Schedule returns the job's scheduling expression
func (j *NamespaceGpuAggregationBackfillJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *NamespaceGpuAggregationBackfillJob) SetConfig(cfg *NamespaceGpuAggregationBackfillConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *NamespaceGpuAggregationBackfillJob) GetConfig() *NamespaceGpuAggregationBackfillConfig {
	return j.config
}
