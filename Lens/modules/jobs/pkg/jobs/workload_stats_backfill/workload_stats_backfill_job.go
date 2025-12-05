package workload_stats_backfill

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	// DefaultBackfillDays is the default number of days to check for missing workload stats
	DefaultBackfillDays = 2

	// DefaultPromQueryStep is the default step for Prometheus queries (in seconds)
	DefaultPromQueryStep = 60

	// WorkloadUtilizationQueryTemplate is the PromQL query template for workload GPU utilization
	WorkloadUtilizationQueryTemplate = `avg(workload_gpu_utilization{workload_uid="%s"})`

	// WorkloadGpuMemoryUsedQueryTemplate is the PromQL query template for workload GPU memory used (bytes)
	WorkloadGpuMemoryUsedQueryTemplate = `avg(workload_gpu_used_vram{workload_uid="%s"})`

	// WorkloadGpuMemoryTotalQueryTemplate is the PromQL query template for workload GPU memory total (bytes)
	WorkloadGpuMemoryTotalQueryTemplate = `avg(workload_gpu_total_vram{workload_uid="%s"})`
)

// WorkloadStatsBackfillConfig is the configuration for workload stats backfill job
type WorkloadStatsBackfillConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// BackfillDays is the number of days to scan for missing data
	BackfillDays int `json:"backfill_days"`

	// PromQueryStep is the step for Prometheus queries (in seconds)
	PromQueryStep int `json:"prom_query_step"`
}

// WorkloadStatsBackfillJob is the job for backfilling missing workload GPU stats
type WorkloadStatsBackfillJob struct {
	config      *WorkloadStatsBackfillConfig
	clusterName string
}

// NewWorkloadStatsBackfillJob creates a new workload stats backfill job with default config
func NewWorkloadStatsBackfillJob() *WorkloadStatsBackfillJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &WorkloadStatsBackfillJob{
		config: &WorkloadStatsBackfillConfig{
			Enabled:       true,
			BackfillDays:  DefaultBackfillDays,
			PromQueryStep: DefaultPromQueryStep,
		},
		clusterName: clusterName,
	}
}

// NewWorkloadStatsBackfillJobWithConfig creates a new workload stats backfill job with custom config
func NewWorkloadStatsBackfillJobWithConfig(cfg *WorkloadStatsBackfillConfig) *WorkloadStatsBackfillJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &WorkloadStatsBackfillJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the workload stats backfill job
func (j *WorkloadStatsBackfillJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "workload_stats_backfill_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "workload_stats_backfill"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("config.backfill_days", j.config.BackfillDays),
	)

	if !j.config.Enabled {
		log.Debugf("Workload stats backfill job is disabled")
		stats.AddMessage("Workload stats backfill job is disabled")
		return stats, nil
	}

	// Calculate time range for recent 2 days
	// Exclude current hour to avoid conflict with ongoing aggregation
	endTime := time.Now().Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-time.Duration(j.config.BackfillDays) * 24 * time.Hour)

	log.Infof("Starting workload stats backfill job for cluster: %s, time range: %v to %v",
		clusterName, startTime, endTime)

	// 1. Get recently active top-level workloads
	workloadsSpan, workloadsCtx := trace.StartSpanFromContext(ctx, "getRecentlyActiveWorkloads")
	activeWorkloads, err := j.getRecentlyActiveTopLevelWorkloads(workloadsCtx, clusterName, startTime, endTime)
	if err != nil {
		workloadsSpan.RecordError(err)
		workloadsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(workloadsSpan)
		return stats, fmt.Errorf("failed to get recently active workloads: %w", err)
	}
	workloadsSpan.SetAttributes(attribute.Int("active_workloads.count", len(activeWorkloads)))
	workloadsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(workloadsSpan)

	if len(activeWorkloads) == 0 {
		log.Infof("No active top-level workloads found in the last %d days", j.config.BackfillDays)
		stats.AddMessage("No active top-level workloads found")
		return stats, nil
	}

	log.Infof("Found %d active top-level workloads to check", len(activeWorkloads))
	stats.AddCustomMetric("active_workloads_count", len(activeWorkloads))

	// 2. Find missing stats for each workload
	missingSpan, missingCtx := trace.StartSpanFromContext(ctx, "findMissingWorkloadStats")
	missingWorkloadHours, err := j.findMissingWorkloadStats(missingCtx, clusterName, activeWorkloads, startTime, endTime)
	if err != nil {
		missingSpan.RecordError(err)
		missingSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(missingSpan)
		return stats, fmt.Errorf("failed to find missing workload stats: %w", err)
	}
	missingSpan.SetAttributes(attribute.Int("missing_entries.count", len(missingWorkloadHours)))
	missingSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(missingSpan)

	log.Infof("Found %d missing workload-hour entries to backfill", len(missingWorkloadHours))
	stats.AddCustomMetric("missing_workload_hours", len(missingWorkloadHours))

	if len(missingWorkloadHours) == 0 {
		log.Infof("No missing workload stats found")
		stats.AddMessage("No missing workload stats found")
		return stats, nil
	}

	// 3. Backfill missing stats
	backfillSpan, backfillCtx := trace.StartSpanFromContext(ctx, "backfillWorkloadStats")
	backfilledCount, errorCount := j.backfillWorkloadStats(backfillCtx, clusterName, missingWorkloadHours, storageClientSet)
	backfillSpan.SetAttributes(
		attribute.Int64("backfilled.count", backfilledCount),
		attribute.Int64("error.count", errorCount),
	)
	backfillSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(backfillSpan)

	stats.ItemsCreated = backfilledCount
	stats.ErrorCount = errorCount
	stats.RecordsProcessed = int64(len(missingWorkloadHours))

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
		attribute.Int64("backfilled_count", backfilledCount),
		attribute.Int64("error_count", errorCount),
	)
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	stats.AddMessage(fmt.Sprintf("Backfill completed: %d workload stats created, %d errors",
		backfilledCount, errorCount))

	log.Infof("Workload stats backfill job completed in %v: %d created, %d errors",
		totalDuration, backfilledCount, errorCount)
	return stats, nil
}

// getRecentlyActiveTopLevelWorkloads gets workloads that were active (created or running) in the last N days
// Only returns top-level workloads (parent_uid is empty)
func (j *WorkloadStatsBackfillJob) getRecentlyActiveTopLevelWorkloads(
	ctx context.Context,
	clusterName string,
	startTime, endTime time.Time) ([]*dbmodel.GpuWorkload, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "getRecentlyActiveTopLevelWorkloads")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("start_time", startTime.Format(time.RFC3339)),
		attribute.String("end_time", endTime.Format(time.RFC3339)),
	)

	facade := database.GetFacadeForCluster(clusterName).GetWorkload()

	// Get all workloads that have not ended (including running and pending)
	allWorkloads, err := facade.GetWorkloadNotEnd(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to get workloads not ended: %w", err)
	}

	// Filter for top-level workloads (parent_uid is empty) that were active during the time range
	var activeWorkloads []*dbmodel.GpuWorkload
	for _, workload := range allWorkloads {
		// Skip non-top-level workloads
		if workload.ParentUID != "" {
			continue
		}

		// Check if workload was active during the time range:
		// - Created before endTime AND (not ended OR ended after startTime)
		if workload.CreatedAt.After(endTime) {
			continue
		}

		// If workload has ended, check if it ended after startTime
		if !workload.EndAt.IsZero() && workload.EndAt.Before(startTime) {
			continue
		}

		activeWorkloads = append(activeWorkloads, workload)
	}

	span.SetAttributes(
		attribute.Int("total_workloads.count", len(allWorkloads)),
		attribute.Int("active_top_level_workloads.count", len(activeWorkloads)),
	)
	span.SetStatus(codes.Ok, "")
	return activeWorkloads, nil
}

// WorkloadHourEntry represents a workload and a specific hour that needs backfilling
type WorkloadHourEntry struct {
	Workload *dbmodel.GpuWorkload
	Hour     time.Time
}

// findMissingWorkloadStats finds hours that are missing stats for each workload
func (j *WorkloadStatsBackfillJob) findMissingWorkloadStats(
	ctx context.Context,
	clusterName string,
	workloads []*dbmodel.GpuWorkload,
	startTime, endTime time.Time) ([]WorkloadHourEntry, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "findMissingWorkloadStats")
	defer trace.FinishSpan(span)

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()

	// Get all existing workload stats in the time range
	existingStats, err := facade.ListWorkloadHourlyStats(ctx, startTime, endTime.Add(time.Hour))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to list existing workload stats: %w", err)
	}

	// Build a map of existing stats: namespace/workloadName/hour -> exists
	existingStatsMap := make(map[string]struct{})
	for _, stat := range existingStats {
		key := fmt.Sprintf("%s/%s/%s", stat.Namespace, stat.WorkloadName, stat.StatHour.Format(time.RFC3339))
		existingStatsMap[key] = struct{}{}
	}

	span.SetAttributes(attribute.Int("existing_stats.count", len(existingStatsMap)))

	// Find missing entries for each workload
	missingEntries := make([]WorkloadHourEntry, 0)

	for _, workload := range workloads {
		// Determine the active time range for this workload
		workloadStartTime := workload.CreatedAt
		if workloadStartTime.Before(startTime) {
			workloadStartTime = startTime
		}

		workloadEndTime := endTime
		if !workload.EndAt.IsZero() && workload.EndAt.Before(endTime) {
			workloadEndTime = workload.EndAt
		}

		// Generate all hours in the workload's active range
		currentHour := workloadStartTime.Truncate(time.Hour)
		endHour := workloadEndTime.Truncate(time.Hour)

		for !currentHour.After(endHour) {
			key := fmt.Sprintf("%s/%s/%s", workload.Namespace, workload.Name, currentHour.Format(time.RFC3339))
			if _, exists := existingStatsMap[key]; !exists {
				missingEntries = append(missingEntries, WorkloadHourEntry{
					Workload: workload,
					Hour:     currentHour,
				})
			}
			currentHour = currentHour.Add(time.Hour)
		}
	}

	span.SetAttributes(attribute.Int("missing_entries.count", len(missingEntries)))
	span.SetStatus(codes.Ok, "")
	return missingEntries, nil
}

// backfillWorkloadStats backfills missing workload stats by querying Prometheus
func (j *WorkloadStatsBackfillJob) backfillWorkloadStats(
	ctx context.Context,
	clusterName string,
	missingEntries []WorkloadHourEntry,
	storageClientSet *clientsets.StorageClientSet) (int64, int64) {

	span, ctx := trace.StartSpanFromContext(ctx, "backfillWorkloadStats")
	defer trace.FinishSpan(span)

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	var backfilledCount, errorCount int64

	for _, entry := range missingEntries {
		// Query GPU utilization from Prometheus for this workload and hour
		hourStart := entry.Hour
		hourEnd := entry.Hour.Add(time.Hour)

		avgUtilization, err := j.queryWorkloadUtilizationForHour(ctx, storageClientSet, entry.Workload.UID, hourStart, hourEnd)
		if err != nil {
			log.Warnf("Failed to query utilization for workload %s/%s at %v: %v",
				entry.Workload.Namespace, entry.Workload.Name, entry.Hour, err)
			errorCount++
			continue
		}

		// Query GPU memory usage from Prometheus
		avgMemoryUsedGB, avgMemoryTotalGB := j.queryWorkloadGpuMemoryForHour(ctx, storageClientSet, entry.Workload.UID, hourStart, hourEnd)

		// Get replica count from database (active pods during this hour)
		avgReplicaCount, maxReplicaCount, minReplicaCount := j.getWorkloadReplicaCountForHour(ctx, clusterName, entry.Workload.UID, hourStart, hourEnd)

		// Create workload hourly stats record
		stats := &dbmodel.WorkloadGpuHourlyStats{
			ClusterName:       clusterName,
			Namespace:         entry.Workload.Namespace,
			WorkloadName:      entry.Workload.Name,
			WorkloadType:      entry.Workload.Kind,
			StatHour:          entry.Hour,
			AllocatedGpuCount: float64(entry.Workload.GpuRequest),
			RequestedGpuCount: float64(entry.Workload.GpuRequest),
			AvgUtilization:    avgUtilization,
			MaxUtilization:    avgUtilization, // Using avg as approximation since we don't have granular data
			MinUtilization:    avgUtilization,
			P50Utilization:    avgUtilization,
			P95Utilization:    avgUtilization,
			AvgGpuMemoryUsed:  avgMemoryUsedGB,
			MaxGpuMemoryUsed:  avgMemoryUsedGB, // Using avg as approximation
			AvgGpuMemoryTotal: avgMemoryTotalGB,
			AvgReplicaCount:   avgReplicaCount,
			MaxReplicaCount:   maxReplicaCount,
			MinReplicaCount:   minReplicaCount,
			WorkloadStatus:    entry.Workload.Status,
			SampleCount:       1,
			OwnerUID:          entry.Workload.ParentUID,
			OwnerName:         "",
			Labels:            entry.Workload.Labels,
			Annotations:       entry.Workload.Annotations,
		}

		// Save to database
		if err := facade.SaveWorkloadHourlyStats(ctx, stats); err != nil {
			log.Errorf("Failed to save workload stats for %s/%s at %v: %v",
				entry.Workload.Namespace, entry.Workload.Name, entry.Hour, err)
			errorCount++
			continue
		}

		backfilledCount++
		log.Debugf("Backfilled workload stats for %s/%s at %v: utilization=%.2f%%, memUsed=%.2fGB, replicas=%d",
			entry.Workload.Namespace, entry.Workload.Name, entry.Hour, avgUtilization, avgMemoryUsedGB, maxReplicaCount)
	}

	span.SetAttributes(
		attribute.Int64("backfilled.count", backfilledCount),
		attribute.Int64("error.count", errorCount),
	)
	span.SetStatus(codes.Ok, "")
	return backfilledCount, errorCount
}

// queryWorkloadUtilizationForHour queries the average GPU utilization for a workload in a specific hour
func (j *WorkloadStatsBackfillJob) queryWorkloadUtilizationForHour(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string,
	startTime, endTime time.Time) (float64, error) {

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
		return 0, err
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		span.SetAttributes(
			attribute.Int("series.count", 0),
			attribute.Float64("utilization.avg", 0),
		)
		span.SetStatus(codes.Ok, "No data points")
		return 0, nil
	}

	// Calculate average utilization from all data points
	sum := 0.0
	for _, point := range series[0].Values {
		sum += point.Value
	}
	avg := sum / float64(len(series[0].Values))

	span.SetAttributes(
		attribute.Int("series.count", len(series)),
		attribute.Int("data_points.count", len(series[0].Values)),
		attribute.Float64("utilization.avg", avg),
	)
	span.SetStatus(codes.Ok, "")

	return avg, nil
}

// queryWorkloadGpuMemoryForHour queries the average GPU memory usage for a workload in a specific hour
// Returns (avgMemoryUsedGB, avgMemoryTotalGB)
func (j *WorkloadStatsBackfillJob) queryWorkloadGpuMemoryForHour(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string,
	startTime, endTime time.Time) (float64, float64) {

	span, ctx := trace.StartSpanFromContext(ctx, "queryWorkloadGpuMemoryForHour")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("workload.uid", workloadUID),
		attribute.String("start_time", startTime.Format(time.RFC3339)),
		attribute.String("end_time", endTime.Format(time.RFC3339)),
	)

	var avgMemoryUsedGB, avgMemoryTotalGB float64

	// Query GPU memory used
	memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, workloadUID)
	memUsedSeries, err := prom.QueryRange(ctx, storageClientSet, memUsedQuery, startTime, endTime,
		j.config.PromQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		log.Debugf("Failed to query GPU memory used for workload %s: %v", workloadUID, err)
	} else if len(memUsedSeries) > 0 && len(memUsedSeries[0].Values) > 0 {
		sum := 0.0
		for _, point := range memUsedSeries[0].Values {
			sum += point.Value
		}
		// Convert from bytes to GB
		avgMemoryUsedGB = (sum / float64(len(memUsedSeries[0].Values))) / (1024 * 1024 * 1024)
	}

	// Query GPU memory total
	memTotalQuery := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, workloadUID)
	memTotalSeries, err := prom.QueryRange(ctx, storageClientSet, memTotalQuery, startTime, endTime,
		j.config.PromQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		log.Debugf("Failed to query GPU memory total for workload %s: %v", workloadUID, err)
	} else if len(memTotalSeries) > 0 && len(memTotalSeries[0].Values) > 0 {
		sum := 0.0
		for _, point := range memTotalSeries[0].Values {
			sum += point.Value
		}
		// Convert from bytes to GB
		avgMemoryTotalGB = (sum / float64(len(memTotalSeries[0].Values))) / (1024 * 1024 * 1024)
	}

	span.SetAttributes(
		attribute.Float64("memory.used_gb", avgMemoryUsedGB),
		attribute.Float64("memory.total_gb", avgMemoryTotalGB),
	)
	span.SetStatus(codes.Ok, "")

	return avgMemoryUsedGB, avgMemoryTotalGB
}

// getWorkloadReplicaCountForHour gets the replica count for a workload during a specific hour
// by querying the workload_pod_reference and gpu_pods tables
// Returns (avgReplicaCount, maxReplicaCount, minReplicaCount)
func (j *WorkloadStatsBackfillJob) getWorkloadReplicaCountForHour(
	ctx context.Context,
	clusterName string,
	workloadUID string,
	hourStart, hourEnd time.Time) (float64, int32, int32) {

	span, ctx := trace.StartSpanFromContext(ctx, "getWorkloadReplicaCountForHour")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("workload.uid", workloadUID),
		attribute.String("hour_start", hourStart.Format(time.RFC3339)),
		attribute.String("hour_end", hourEnd.Format(time.RFC3339)),
	)

	facade := database.GetFacadeForCluster(clusterName)

	// Get pod references for this workload
	podRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Debugf("Failed to get pod references for workload %s: %v", workloadUID, err)
		span.SetAttributes(attribute.String("error", err.Error()))
		span.SetStatus(codes.Ok, "Error getting pod references, using default")
		return 1, 1, 1
	}

	if len(podRefs) == 0 {
		span.SetAttributes(attribute.Int("pod_refs.count", 0))
		span.SetStatus(codes.Ok, "No pod references found")
		return 1, 1, 1
	}

	// Extract pod UIDs
	podUIDs := make([]string, 0, len(podRefs))
	for _, ref := range podRefs {
		podUIDs = append(podUIDs, ref.PodUID)
	}

	// Get pods by UIDs
	pods, err := facade.GetPod().ListPodsByUids(ctx, podUIDs)
	if err != nil {
		log.Debugf("Failed to get pods for workload %s: %v", workloadUID, err)
		span.SetAttributes(attribute.String("error", err.Error()))
		span.SetStatus(codes.Ok, "Error getting pods, using default")
		return 1, 1, 1
	}

	if len(pods) == 0 {
		span.SetAttributes(attribute.Int("pods.count", 0))
		span.SetStatus(codes.Ok, "No pods found")
		return 1, 1, 1
	}

	// Count pods that were active during the hour
	// A pod is considered active during the hour if:
	// - It was created before hourEnd
	// - It was running (Running=true) or created during this hour
	activePodCount := int32(0)
	for _, pod := range pods {
		// Check if pod was created before the hour ended
		if pod.CreatedAt.After(hourEnd) {
			continue
		}

		// If pod is running or was created during this hour, count it
		if pod.Running || (pod.CreatedAt.After(hourStart) && pod.CreatedAt.Before(hourEnd)) {
			activePodCount++
		} else if !pod.Deleted && pod.CreatedAt.Before(hourStart) {
			// Pod existed before this hour and is not deleted
			activePodCount++
		}
	}

	// If no active pods found, use at least 1
	if activePodCount == 0 {
		activePodCount = 1
	}

	span.SetAttributes(
		attribute.Int("pod_refs.count", len(podRefs)),
		attribute.Int("pods.count", len(pods)),
		attribute.Int("active_pods.count", int(activePodCount)),
	)
	span.SetStatus(codes.Ok, "")

	// For backfill, we use the same value for avg/max/min since we don't have granular data
	return float64(activePodCount), activePodCount, activePodCount
}

// Schedule returns the job's scheduling expression
func (j *WorkloadStatsBackfillJob) Schedule() string {
	return "@every 1m"
}

// SetConfig sets the job configuration
func (j *WorkloadStatsBackfillJob) SetConfig(cfg *WorkloadStatsBackfillConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *WorkloadStatsBackfillJob) GetConfig() *WorkloadStatsBackfillConfig {
	return j.config
}
