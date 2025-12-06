package gpu_aggregation_backfill

import (
	"context"
	"fmt"
	"math"
	"sort"
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
	// DefaultBackfillDays is the default number of days to backfill
	DefaultBackfillDays = 7

	// DefaultBatchSize is the default batch size for processing hours
	DefaultBatchSize = 24
)

// GpuAggregationBackfillConfig is the configuration for backfill job
type GpuAggregationBackfillConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// BackfillDays is the number of days to scan for missing data
	BackfillDays int `json:"backfill_days"`

	// BatchSize is the number of hours to process in each batch
	BatchSize int `json:"batch_size"`

	// ExcludeNamespaces is the list of namespaces to exclude from backfill
	ExcludeNamespaces []string `json:"exclude_namespaces"`
}

// GpuAggregationBackfillJob is the job for backfilling missing GPU aggregation data
type GpuAggregationBackfillJob struct {
	config      *GpuAggregationBackfillConfig
	clusterName string
}

// NewGpuAggregationBackfillJob creates a new backfill job with default config
func NewGpuAggregationBackfillJob() *GpuAggregationBackfillJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &GpuAggregationBackfillJob{
		config: &GpuAggregationBackfillConfig{
			Enabled:           true,
			BackfillDays:      DefaultBackfillDays,
			BatchSize:         DefaultBatchSize,
			ExcludeNamespaces: []string{},
		},
		clusterName: clusterName,
	}
}

// NewGpuAggregationBackfillJobWithConfig creates a new backfill job with custom config
func NewGpuAggregationBackfillJobWithConfig(cfg *GpuAggregationBackfillConfig) *GpuAggregationBackfillJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &GpuAggregationBackfillJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the backfill job
func (j *GpuAggregationBackfillJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "gpu_aggregation_backfill_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "gpu_aggregation_backfill"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("config.backfill_days", j.config.BackfillDays),
	)

	if !j.config.Enabled {
		log.Debugf("GPU aggregation backfill job is disabled")
		stats.AddMessage("GPU aggregation backfill job is disabled")
		return stats, nil
	}

	// Calculate time range
	// Exclude current hour to avoid conflict with ongoing aggregation
	// e.g., if now is 18:30, endTime should be 17:00 (last completed hour)
	endTime := time.Now().Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-time.Duration(j.config.BackfillDays) * 24 * time.Hour)

	log.Infof("Starting GPU aggregation backfill job for cluster: %s, time range: %v to %v (excluding current hour)",
		clusterName, startTime, endTime)

	// 1. Generate all hours in the time range (7 days * 24 hours = 168 hours)
	allHours := j.generateAllHours(startTime, endTime)
	log.Infof("Generated %d hours to check for backfill", len(allHours))

	if len(allHours) == 0 {
		log.Infof("No hours to process")
		stats.AddMessage("No hours to process")
		return stats, nil
	}

	// 2. Find missing cluster and namespace stats for all hours
	missingSpan, missingCtx := trace.StartSpanFromContext(ctx, "findMissingStats")
	missingClusterHours, missingNamespaceHours, err := j.findMissingStats(missingCtx, clusterName, allHours)
	if err != nil {
		missingSpan.RecordError(err)
		missingSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(missingSpan)
		return stats, fmt.Errorf("failed to find missing stats: %w", err)
	}
	missingSpan.SetAttributes(
		attribute.Int("missing.cluster_hours", len(missingClusterHours)),
		attribute.Int("missing.namespace_hours", len(missingNamespaceHours)),
	)
	missingSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(missingSpan)

	log.Infof("Found %d missing cluster hours and %d missing namespace hours",
		len(missingClusterHours), len(missingNamespaceHours))

	stats.AddCustomMetric("missing_cluster_hours", len(missingClusterHours))
	stats.AddCustomMetric("missing_namespace_hours", len(missingNamespaceHours))

	// 3. Backfill cluster stats using time-weighted calculation
	if len(missingClusterHours) > 0 {
		clusterBackfillSpan, clusterBackfillCtx := trace.StartSpanFromContext(ctx, "backfillClusterStats")
		clusterBackfillSpan.SetAttributes(attribute.Int("hours.count", len(missingClusterHours)))

		clusterCount, clusterErr := j.backfillClusterStats(clusterBackfillCtx, clusterName, missingClusterHours)
		if clusterErr != nil {
			clusterBackfillSpan.RecordError(clusterErr)
			clusterBackfillSpan.SetStatus(codes.Error, clusterErr.Error())
			trace.FinishSpan(clusterBackfillSpan)
			stats.ErrorCount++
			log.Errorf("Failed to backfill cluster stats: %v", clusterErr)
		} else {
			clusterBackfillSpan.SetAttributes(attribute.Int64("backfilled.count", clusterCount))
			clusterBackfillSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(clusterBackfillSpan)
			stats.ItemsCreated += clusterCount
			log.Infof("Backfilled %d cluster hourly stats", clusterCount)
		}
	}

	// 4. Backfill namespace stats using time-weighted calculation
	if len(missingNamespaceHours) > 0 {
		nsBackfillSpan, nsBackfillCtx := trace.StartSpanFromContext(ctx, "backfillNamespaceStats")
		nsBackfillSpan.SetAttributes(attribute.Int("hours.count", len(missingNamespaceHours)))

		nsCount, nsErr := j.backfillNamespaceStats(nsBackfillCtx, clusterName, missingNamespaceHours)
		if nsErr != nil {
			nsBackfillSpan.RecordError(nsErr)
			nsBackfillSpan.SetStatus(codes.Error, nsErr.Error())
			trace.FinishSpan(nsBackfillSpan)
			stats.ErrorCount++
			log.Errorf("Failed to backfill namespace stats: %v", nsErr)
		} else {
			nsBackfillSpan.SetAttributes(attribute.Int64("backfilled.count", nsCount))
			nsBackfillSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(nsBackfillSpan)
			stats.ItemsCreated += nsCount
			log.Infof("Backfilled %d namespace hourly stats", nsCount)
		}
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	stats.AddMessage(fmt.Sprintf("Backfill completed: %d cluster stats, %d namespace stats created",
		stats.ItemsCreated-int64(len(missingNamespaceHours)), stats.ItemsCreated))

	log.Infof("GPU aggregation backfill job completed in %v", totalDuration)
	return stats, nil
}

// generateAllHours generates all hours in the time range
func (j *GpuAggregationBackfillJob) generateAllHours(startTime, endTime time.Time) []time.Time {
	hours := make([]time.Time, 0)

	// Start from the first hour
	current := startTime.Truncate(time.Hour)
	end := endTime.Truncate(time.Hour)

	for !current.After(end) {
		hours = append(hours, current)
		current = current.Add(time.Hour)
	}

	return hours
}

// findMissingStats finds hours that are missing cluster or namespace stats
// It checks all hours in the time range, not just hours with workload data
// Uses namespace_info table as the source of truth for namespace list
func (j *GpuAggregationBackfillJob) findMissingStats(
	ctx context.Context,
	clusterName string,
	allHours []time.Time) ([]time.Time, map[time.Time][]string, error) {

	if len(allHours) == 0 {
		return nil, nil, nil
	}

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()

	startTime := allHours[0]
	endTime := allHours[len(allHours)-1].Add(time.Hour)

	// Get existing cluster stats
	clusterStats, err := facade.GetClusterHourlyStats(ctx, startTime, endTime)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get cluster hourly stats: %w", err)
	}

	existingClusterHours := make(map[time.Time]struct{})
	for _, stat := range clusterStats {
		existingClusterHours[stat.StatHour.Truncate(time.Hour)] = struct{}{}
	}

	// Get all namespaces from namespace_info table (source of truth)
	namespaceInfoList, err := database.GetFacadeForCluster(clusterName).GetNamespaceInfo().List(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list namespace info: %w", err)
	}

	// Build namespace list from namespace_info
	allNamespaces := make([]string, 0, len(namespaceInfoList))
	for _, nsInfo := range namespaceInfoList {
		if !j.shouldExcludeNamespace(nsInfo.Name) {
			allNamespaces = append(allNamespaces, nsInfo.Name)
		}
	}

	log.Infof("Found %d namespaces from namespace_info table", len(allNamespaces))

	// Get existing namespace stats
	namespaceStats, err := facade.ListNamespaceHourlyStats(ctx, startTime, endTime)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list namespace hourly stats: %w", err)
	}

	// Build hour -> namespaces map for existing stats
	existingNamespaceHours := make(map[time.Time]map[string]struct{})
	for _, stat := range namespaceStats {
		hour := stat.StatHour.Truncate(time.Hour)
		if _, exists := existingNamespaceHours[hour]; !exists {
			existingNamespaceHours[hour] = make(map[string]struct{})
		}
		existingNamespaceHours[hour][stat.Namespace] = struct{}{}
	}

	// Find missing cluster hours - check all hours, not just workload hours
	missingClusterHours := make([]time.Time, 0)
	for _, hour := range allHours {
		if _, exists := existingClusterHours[hour]; !exists {
			missingClusterHours = append(missingClusterHours, hour)
		}
	}

	// Find missing namespace hours
	// For each hour, check if all namespaces from namespace_info have stats
	// Key: hour, Value: list of missing namespaces
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

	return missingClusterHours, missingNamespaceHours, nil
}

// shouldExcludeNamespace checks if a namespace should be excluded from backfill
func (j *GpuAggregationBackfillJob) shouldExcludeNamespace(namespace string) bool {
	for _, excluded := range j.config.ExcludeNamespaces {
		if namespace == excluded {
			return true
		}
	}
	return false
}

// backfillClusterStats backfills missing cluster hourly stats using time-weighted calculation
// It calculates GPU allocation based on actual pod activity during each hour
func (j *GpuAggregationBackfillJob) backfillClusterStats(
	ctx context.Context,
	clusterName string,
	missingHours []time.Time) (int64, error) {

	if len(missingHours) == 0 {
		return 0, nil
	}

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	calculator := statistics.NewGpuAllocationCalculator(clusterName)
	var createdCount int64

	// Get cluster GPU capacity once (reuse for all hours)
	totalCapacity, err := j.getClusterGpuCapacity(ctx, clusterName)
	if err != nil {
		log.Warnf("Failed to get cluster GPU capacity: %v", err)
		totalCapacity = 0
	}

	for _, hour := range missingHours {
		// Use time-weighted calculation from statistics package
		result, err := calculator.CalculateHourlyGpuAllocation(ctx, hour)
		if err != nil {
			log.Warnf("Failed to calculate GPU allocation for hour %v: %v", hour, err)
			// Create zero-value stats on error
			result = &statistics.GpuAllocationResult{}
		}

		var clusterStats *dbmodel.ClusterGpuHourlyStats
		if result.WorkloadCount == 0 {
			// No workload data for this hour, fill with zero values
			clusterStats = j.createZeroClusterStats(clusterName, hour)
			log.Debugf("Creating zero-value cluster stats for hour %v (no workload data)", hour)
		} else {
			// Build cluster stats from time-weighted calculation result
			clusterStats = j.buildClusterStatsFromResult(clusterName, hour, result)
		}

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
		log.Debugf("Backfilled cluster stats for hour %v: allocated=%.2f, workloads=%d, pods=%d",
			hour, clusterStats.AllocatedGpuCount, result.WorkloadCount, result.PodCount)
	}

	return createdCount, nil
}

// buildClusterStatsFromResult builds ClusterGpuHourlyStats from time-weighted calculation result
func (j *GpuAggregationBackfillJob) buildClusterStatsFromResult(
	clusterName string,
	hour time.Time,
	result *statistics.GpuAllocationResult) *dbmodel.ClusterGpuHourlyStats {

	stats := &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       clusterName,
		StatHour:          hour,
		AllocatedGpuCount: result.TotalAllocatedGpu,
		SampleCount:       int32(result.WorkloadCount),
	}

	// Calculate utilization statistics from workload details
	if len(result.Details) > 0 {
		utilizationValues := make([]float64, 0, len(result.Details))

		// For backfill, we don't have real-time utilization data
		// We can only estimate based on allocation
		// Here we set utilization to 0 as we don't have Prometheus data for historical hours
		for range result.Details {
			utilizationValues = append(utilizationValues, 0)
		}

		sort.Float64s(utilizationValues)
		stats.MinUtilization = utilizationValues[0]
		stats.MaxUtilization = utilizationValues[len(utilizationValues)-1]
		stats.P50Utilization = calculatePercentile(utilizationValues, 0.50)
		stats.P95Utilization = calculatePercentile(utilizationValues, 0.95)

		var utilizationSum float64
		for _, v := range utilizationValues {
			utilizationSum += v
		}
		stats.AvgUtilization = utilizationSum / float64(len(utilizationValues))
	}

	return stats
}

// createZeroClusterStats creates a cluster stats record with zero values
func (j *GpuAggregationBackfillJob) createZeroClusterStats(
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

// backfillNamespaceStats backfills missing namespace hourly stats using time-weighted calculation
// It calculates GPU allocation based on actual pod activity during each hour for each namespace
func (j *GpuAggregationBackfillJob) backfillNamespaceStats(
	ctx context.Context,
	clusterName string,
	missingNamespaceHours map[time.Time][]string) (int64, error) {

	if len(missingNamespaceHours) == 0 {
		return 0, nil
	}

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	calculator := statistics.NewGpuAllocationCalculator(clusterName)
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
			result, err := calculator.CalculateHourlyNamespaceGpuAllocation(ctx, namespace, hour)
			if err != nil {
				log.Warnf("Failed to calculate GPU allocation for namespace %s at hour %v: %v", namespace, hour, err)
				result = &statistics.GpuAllocationResult{}
			}

			var nsStats *dbmodel.NamespaceGpuHourlyStats
			if result.WorkloadCount == 0 {
				// No workload data for this namespace in this hour, fill with zero values
				nsStats = j.createZeroNamespaceStats(clusterName, namespace, hour)
				log.Debugf("Creating zero-value namespace stats for %s at %v (no workload data)", namespace, hour)
			} else {
				// Build namespace stats from time-weighted calculation result
				nsStats = j.buildNamespaceStatsFromResult(clusterName, namespace, hour, result)
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
			log.Debugf("Backfilled namespace stats for %s at %v: allocated=%.2f, workloads=%d",
				namespace, hour, nsStats.AllocatedGpuCount, result.WorkloadCount)
		}
	}

	return createdCount, nil
}

// buildNamespaceStatsFromResult builds NamespaceGpuHourlyStats from time-weighted calculation result
func (j *GpuAggregationBackfillJob) buildNamespaceStatsFromResult(
	clusterName, namespace string,
	hour time.Time,
	result *statistics.GpuAllocationResult) *dbmodel.NamespaceGpuHourlyStats {

	stats := &dbmodel.NamespaceGpuHourlyStats{
		ClusterName:         clusterName,
		Namespace:           namespace,
		StatHour:            hour,
		AllocatedGpuCount:   result.TotalAllocatedGpu,
		ActiveWorkloadCount: int32(result.WorkloadCount),
	}

	// Calculate utilization statistics from workload details
	if len(result.Details) > 0 {
		utilizationValues := make([]float64, 0, len(result.Details))

		// For backfill, we don't have real-time utilization data
		// We set utilization to 0 as we don't have Prometheus data for historical hours
		for range result.Details {
			utilizationValues = append(utilizationValues, 0)
		}

		sort.Float64s(utilizationValues)
		stats.MinUtilization = utilizationValues[0]
		stats.MaxUtilization = utilizationValues[len(utilizationValues)-1]

		var utilizationSum float64
		for _, v := range utilizationValues {
			utilizationSum += v
		}
		stats.AvgUtilization = utilizationSum / float64(len(utilizationValues))
	}

	return stats
}

// createZeroNamespaceStats creates a namespace stats record with zero values
func (j *GpuAggregationBackfillJob) createZeroNamespaceStats(
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

// getClusterGpuCapacity gets the total GPU capacity of the cluster
func (j *GpuAggregationBackfillJob) getClusterGpuCapacity(ctx context.Context, clusterName string) (int, error) {
	// Query all GPU nodes from database and sum capacity
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

// getNamespaceGpuQuotas gets the GPU quotas for all namespaces
func (j *GpuAggregationBackfillJob) getNamespaceGpuQuotas(ctx context.Context, clusterName string) (map[string]int32, error) {
	namespaceInfoList, err := database.GetFacadeForCluster(clusterName).GetNamespaceInfo().List(ctx)
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
func (j *GpuAggregationBackfillJob) Schedule() string {
	return "@every 5m"
}

// calculatePercentile calculates percentile value from sorted values
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	if percentile == 0 {
		return sortedValues[0]
	}
	if percentile == 1 {
		return sortedValues[len(sortedValues)-1]
	}

	index := int(math.Ceil(percentile*float64(len(sortedValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

// SetConfig sets the job configuration
func (j *GpuAggregationBackfillJob) SetConfig(cfg *GpuAggregationBackfillConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *GpuAggregationBackfillJob) GetConfig() *GpuAggregationBackfillConfig {
	return j.config
}
