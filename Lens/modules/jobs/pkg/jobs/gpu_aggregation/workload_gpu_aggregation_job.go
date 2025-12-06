package gpu_aggregation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
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
	"gorm.io/gorm"
)

const (
	// DefaultPromQueryStep is the default step for Prometheus queries (in seconds)
	DefaultPromQueryStep = 60

	// WorkloadUtilizationQueryTemplate is the PromQL query template for workload GPU utilization
	WorkloadUtilizationQueryTemplate = `avg(workload_gpu_utilization{workload_uid="%s"})`

	// WorkloadGpuMemoryUsedQueryTemplate is the PromQL query template for workload GPU memory used (bytes)
	WorkloadGpuMemoryUsedQueryTemplate = `avg(workload_gpu_used_vram{workload_uid="%s"})`

	// WorkloadGpuMemoryTotalQueryTemplate is the PromQL query template for workload GPU memory total (bytes)
	WorkloadGpuMemoryTotalQueryTemplate = `avg(workload_gpu_total_vram{workload_uid="%s"})`

	// CacheKeyWorkloadGpuAggregationLastHour is the cache key for storing the last processed hour
	CacheKeyWorkloadGpuAggregationLastHour = "job.workload_gpu_aggregation.last_processed_hour"
)

// WorkloadGpuAggregationConfig is the configuration for workload GPU aggregation job
type WorkloadGpuAggregationConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// PromQueryStep is the step for Prometheus queries (in seconds)
	PromQueryStep int `json:"prom_query_step"`
}

// WorkloadGpuAggregationJob aggregates workload-level GPU statistics by querying Prometheus
type WorkloadGpuAggregationJob struct {
	config      *WorkloadGpuAggregationConfig
	clusterName string
}

// NewWorkloadGpuAggregationJob creates a new workload GPU aggregation job
func NewWorkloadGpuAggregationJob() *WorkloadGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &WorkloadGpuAggregationJob{
		config: &WorkloadGpuAggregationConfig{
			Enabled:       true,
			PromQueryStep: DefaultPromQueryStep,
		},
		clusterName: clusterName,
	}
}

// NewWorkloadGpuAggregationJobWithConfig creates a new workload GPU aggregation job with custom config
func NewWorkloadGpuAggregationJobWithConfig(cfg *WorkloadGpuAggregationConfig) *WorkloadGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &WorkloadGpuAggregationJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the workload GPU aggregation job
func (j *WorkloadGpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "workload_gpu_aggregation_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "workload_gpu_aggregation"),
		attribute.String("cluster.name", clusterName),
	)

	if !j.config.Enabled {
		log.Debugf("Workload GPU aggregation job is disabled")
		stats.AddMessage("Workload GPU aggregation job is disabled")
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

		log.Infof("Processing workload aggregation for hour: %v (last processed: %v)", hourToProcess, lastProcessedHour)

		aggSpan, aggCtx := trace.StartSpanFromContext(ctx, "aggregateWorkloadStats")
		aggSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hourToProcess.Format(time.RFC3339)),
		)

		count, err := j.aggregateWorkloadStats(aggCtx, clusterName, hourToProcess, storageClientSet)
		if err != nil {
			aggSpan.RecordError(err)
			aggSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(aggSpan)
			stats.ErrorCount++
			log.Errorf("Failed to aggregate workload stats: %v", err)
		} else {
			aggSpan.SetAttributes(attribute.Int64("workloads.count", count))
			aggSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(aggSpan)
			stats.ItemsCreated = count
			stats.AddMessage(fmt.Sprintf("Aggregated %d workload stats for %v", count, hourToProcess))

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
func (j *WorkloadGpuAggregationJob) getLastProcessedHour(ctx context.Context, clusterName string) (time.Time, error) {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	var lastHourStr string
	err := cacheFacade.Get(ctx, CacheKeyWorkloadGpuAggregationLastHour, &lastHourStr)
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
func (j *WorkloadGpuAggregationJob) setLastProcessedHour(ctx context.Context, clusterName string, hour time.Time) error {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	hourStr := hour.Format(time.RFC3339)
	return cacheFacade.Set(ctx, CacheKeyWorkloadGpuAggregationLastHour, hourStr, nil)
}

// aggregateWorkloadStats aggregates workload-level statistics by querying Prometheus
func (j *WorkloadGpuAggregationJob) aggregateWorkloadStats(
	ctx context.Context,
	clusterName string,
	hour time.Time,
	storageClientSet *clientsets.StorageClientSet) (int64, error) {

	hourStart := hour
	hourEnd := hour.Add(time.Hour)

	// Get active top-level workloads during this hour
	workloads, err := j.getActiveTopLevelWorkloads(ctx, clusterName, hourStart, hourEnd)
	if err != nil {
		return 0, fmt.Errorf("failed to get active workloads: %w", err)
	}

	log.Infof("Found %d active top-level workloads for hour %v", len(workloads), hour)

	if len(workloads) == 0 {
		return 0, nil
	}

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	var createdCount int64
	var errorCount int64

	for _, workload := range workloads {
		// Query GPU utilization from Prometheus for this workload
		utilizationValues, err := j.queryWorkloadUtilizationForHour(ctx, storageClientSet, workload.UID, hourStart, hourEnd)
		if err != nil {
			log.Warnf("Failed to query utilization for workload %s/%s: %v",
				workload.Namespace, workload.Name, err)
			errorCount++
			// Continue with empty utilization values
			utilizationValues = []float64{}
		}

		// Query GPU memory usage from Prometheus
		avgMemoryUsedGB, maxMemoryUsedGB, avgMemoryTotalGB := j.queryWorkloadGpuMemoryForHour(
			ctx, storageClientSet, workload.UID, hourStart, hourEnd)

		// Get replica count from database (active pods during this hour)
		avgReplicaCount, maxReplicaCount, minReplicaCount := j.getWorkloadReplicaCountForHour(
			ctx, clusterName, workload.UID, hourStart, hourEnd)

		// Ensure Labels and Annotations are not nil (required for JSONB fields)
		labels := workload.Labels
		if labels == nil {
			labels = dbmodel.ExtType{}
		}
		annotations := workload.Annotations
		if annotations == nil {
			annotations = dbmodel.ExtType{}
		}

		// Build workload hourly stats
		stats := &dbmodel.WorkloadGpuHourlyStats{
			ClusterName:       clusterName,
			Namespace:         workload.Namespace,
			WorkloadName:      workload.Name,
			WorkloadType:      workload.Kind,
			StatHour:          hour,
			AllocatedGpuCount: float64(workload.GpuRequest),
			RequestedGpuCount: float64(workload.GpuRequest),
			AvgGpuMemoryUsed:  avgMemoryUsedGB,
			MaxGpuMemoryUsed:  maxMemoryUsedGB,
			AvgGpuMemoryTotal: avgMemoryTotalGB,
			AvgReplicaCount:   avgReplicaCount,
			MaxReplicaCount:   maxReplicaCount,
			MinReplicaCount:   minReplicaCount,
			WorkloadStatus:    workload.Status,
			SampleCount:       int32(len(utilizationValues)),
			OwnerUID:          workload.ParentUID,
			OwnerName:         "",
			Labels:            labels,
			Annotations:       annotations,
		}

		// Calculate utilization statistics
		if len(utilizationValues) > 0 {
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

		// Save to database
		if err := facade.SaveWorkloadHourlyStats(ctx, stats); err != nil {
			log.Errorf("Failed to save workload stats for %s/%s at %v: %v",
				workload.Namespace, workload.Name, hour, err)
			errorCount++
			continue
		}

		createdCount++
		log.Debugf("Workload stats saved for %s/%s at %v: utilization=%.2f%%, memUsed=%.2fGB, replicas=%d",
			workload.Namespace, workload.Name, hour, stats.AvgUtilization, avgMemoryUsedGB, maxReplicaCount)
	}

	if errorCount > 0 {
		log.Warnf("Workload aggregation completed with %d errors", errorCount)
	}

	return createdCount, nil
}

// getActiveTopLevelWorkloads gets workloads that were active during the specified hour
// Only returns top-level workloads (parent_uid is empty)
func (j *WorkloadGpuAggregationJob) getActiveTopLevelWorkloads(
	ctx context.Context,
	clusterName string,
	startTime, endTime time.Time) ([]*dbmodel.GpuWorkload, error) {

	facade := database.GetFacadeForCluster(clusterName).GetWorkload()

	// Get all workloads that have not ended
	allWorkloads, err := facade.GetWorkloadNotEnd(ctx)
	if err != nil {
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

	return activeWorkloads, nil
}

// queryWorkloadUtilizationForHour queries the GPU utilization for a workload in a specific hour
// Returns all data points for detailed statistics
func (j *WorkloadGpuAggregationJob) queryWorkloadUtilizationForHour(
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

// queryWorkloadGpuMemoryForHour queries the GPU memory usage for a workload in a specific hour
// Returns (avgMemoryUsedGB, maxMemoryUsedGB, avgMemoryTotalGB)
func (j *WorkloadGpuAggregationJob) queryWorkloadGpuMemoryForHour(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string,
	startTime, endTime time.Time) (float64, float64, float64) {

	span, ctx := trace.StartSpanFromContext(ctx, "queryWorkloadGpuMemoryForHour")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("workload.uid", workloadUID),
		attribute.String("start_time", startTime.Format(time.RFC3339)),
		attribute.String("end_time", endTime.Format(time.RFC3339)),
	)

	var avgMemoryUsedGB, maxMemoryUsedGB, avgMemoryTotalGB float64

	// Query GPU memory used
	memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, workloadUID)
	memUsedSeries, err := prom.QueryRange(ctx, storageClientSet, memUsedQuery, startTime, endTime,
		j.config.PromQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		log.Debugf("Failed to query GPU memory used for workload %s: %v", workloadUID, err)
	} else if len(memUsedSeries) > 0 && len(memUsedSeries[0].Values) > 0 {
		sum := 0.0
		maxVal := 0.0
		for _, point := range memUsedSeries[0].Values {
			sum += point.Value
			if point.Value > maxVal {
				maxVal = point.Value
			}
		}
		// Convert from bytes to GB
		avgMemoryUsedGB = (sum / float64(len(memUsedSeries[0].Values))) / (1024 * 1024 * 1024)
		maxMemoryUsedGB = maxVal / (1024 * 1024 * 1024)
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
		attribute.Float64("memory.avg_used_gb", avgMemoryUsedGB),
		attribute.Float64("memory.max_used_gb", maxMemoryUsedGB),
		attribute.Float64("memory.avg_total_gb", avgMemoryTotalGB),
	)
	span.SetStatus(codes.Ok, "")

	return avgMemoryUsedGB, maxMemoryUsedGB, avgMemoryTotalGB
}

// getWorkloadReplicaCountForHour gets the replica count for a workload during a specific hour
// Returns (avgReplicaCount, maxReplicaCount, minReplicaCount)
func (j *WorkloadGpuAggregationJob) getWorkloadReplicaCountForHour(
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

	// For now, we use the same value for avg/max/min since we don't have granular data
	return float64(activePodCount), activePodCount, activePodCount
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

// Schedule returns the job's scheduling expression
func (j *WorkloadGpuAggregationJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *WorkloadGpuAggregationJob) SetConfig(cfg *WorkloadGpuAggregationConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *WorkloadGpuAggregationJob) GetConfig() *WorkloadGpuAggregationConfig {
	return j.config
}
