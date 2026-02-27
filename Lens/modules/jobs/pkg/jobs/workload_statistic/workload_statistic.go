// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workload_statistic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	// DefaultQueryWindow is the default query time window: last 1 hour (for short-running workloads)
	DefaultQueryWindow = 1 * time.Hour
	// MaxQueryWindow is the maximum query time window: 24 hours (to avoid excessive pressure on Prometheus)
	MaxQueryWindow = 24 * time.Hour
	// DefaultQueryStep is the default query step: 30 seconds
	DefaultQueryStep = 30
	// MaxConcurrentQueries is the maximum number of concurrent queries
	MaxConcurrentQueries = 5
)

// WorkloadStatisticJob is the job for collecting workload GPU utilization statistics
type WorkloadStatisticJob struct {
	// queryWindow is the time window for historical data query
	queryWindow time.Duration
	// queryStep is the query step in seconds
	queryStep int
}

func NewWorkloadStatisticJob() *WorkloadStatisticJob {
	return &WorkloadStatisticJob{
		queryWindow: DefaultQueryWindow,
		queryStep:   DefaultQueryStep,
	}
}

func (j *WorkloadStatisticJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "workload_statistic_job.Run")
	defer trace.FinishSpan(span)

	// Record job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "workload_statistic"),
		attribute.String("cluster.name", clusterName),
	)

	// Get all active workloads
	getWorkloadsSpan, getWorkloadsCtx := trace.StartSpanFromContext(ctx, "getActiveWorkloads")
	getWorkloadsSpan.SetAttributes(attribute.String("cluster.name", clusterName))

	startTime := time.Now()
	workloads, err := j.getActiveWorkloads(getWorkloadsCtx, clusterName)
	duration := time.Since(startTime)

	if err != nil {
		getWorkloadsSpan.RecordError(err)
		getWorkloadsSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		getWorkloadsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getWorkloadsSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get workloads")
		return stats, err
	}

	getWorkloadsSpan.SetAttributes(
		attribute.Int("workloads_count", len(workloads)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getWorkloadsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getWorkloadsSpan)

	if len(workloads) == 0 {
		log.Infof("No active workloads found, skipping statistic collection")
		stats.AddMessage("No active workloads found")
		totalDuration := time.Since(jobStartTime)
		span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
		span.SetStatus(codes.Ok, "")
		return stats, nil
	}

	// Process workloads concurrently
	processWorkloadsSpan, processCtx := trace.StartSpanFromContext(ctx, "processWorkloads")
	processWorkloadsSpan.SetAttributes(attribute.Int("workloads_count", len(workloads)))

	startTime = time.Now()
	err = j.processWorkloads(processCtx, clusterName, workloads, storageClientSet, stats)
	duration = time.Since(startTime)

	if err != nil {
		processWorkloadsSpan.RecordError(err)
		processWorkloadsSpan.SetAttributes(
			attribute.String("error.message", err.Error()),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		processWorkloadsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(processWorkloadsSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to process workloads")
		return stats, err
	}

	processWorkloadsSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Int64("items_created", stats.ItemsCreated),
	)
	processWorkloadsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(processWorkloadsSpan)

	stats.RecordsProcessed = int64(len(workloads))
	stats.AddCustomMetric("workloads_count", len(workloads))
	stats.AddMessage(fmt.Sprintf("Processed %d workloads successfully", len(workloads)))

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("workloads_count", len(workloads)),
		attribute.Int64("records_processed", stats.RecordsProcessed),
		attribute.Int64("items_created", stats.ItemsCreated),
		attribute.Int64("error_count", stats.ErrorCount),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

func (j *WorkloadStatisticJob) getActiveWorkloads(ctx context.Context, clusterName string) ([]*dbModel.GpuWorkload, error) {
	// Get all running workloads from database
	workloads, err := database.GetFacadeForCluster(clusterName).GetWorkload().GetWorkloadNotEnd(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active workloads: %w", err)
	}

	// Deduplicate workloads by (namespace, name, uid) to avoid concurrent processing of duplicate records
	// Use a map to track unique workloads, keeping the latest record (highest ID)
	uniqueWorkloads := make(map[string]*dbModel.GpuWorkload)
	for _, workload := range workloads {
		// Use workload's own UID (consistent with GetOrCreate logic)
		// Each workload is tracked independently, regardless of parent-child relationships
		key := fmt.Sprintf("%s/%s/%s", workload.Namespace, workload.Name, workload.UID)

		// Keep the record with the highest ID (latest)
		if existing, found := uniqueWorkloads[key]; !found || workload.ID > existing.ID {
			uniqueWorkloads[key] = workload
		}
	}

	// Convert map back to slice
	result := make([]*dbModel.GpuWorkload, 0, len(uniqueWorkloads))
	for _, workload := range uniqueWorkloads {
		result = append(result, workload)
	}

	if len(workloads) != len(result) {
		log.Warnf("Deduplicated %d workload records to %d unique workloads", len(workloads), len(result))
	}

	return result, nil
}

func (j *WorkloadStatisticJob) processWorkloads(ctx context.Context, clusterName string, workloads []*dbModel.GpuWorkload, storageClientSet *clientsets.StorageClientSet, stats *common.ExecutionStats) error {
	// Use buffered channel to limit concurrency
	semaphore := make(chan struct{}, MaxConcurrentQueries)
	wg := &sync.WaitGroup{}

	for i := range workloads {
		workload := workloads[i]
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			err := j.processWorkload(ctx, clusterName, workload, storageClientSet)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Failed to process workload %s/%s: %v", workload.Namespace, workload.Name, err)
			} else {
				atomic.AddInt64(&stats.ItemsCreated, 1)
			}
		}()
	}

	wg.Wait()
	return nil
}

func (j *WorkloadStatisticJob) processWorkload(ctx context.Context, clusterName string, workload *dbModel.GpuWorkload, storageClientSet *clientsets.StorageClientSet) error {
	span, ctx := trace.StartSpanFromContext(ctx, "processWorkload")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("workload.name", workload.Name),
		attribute.String("workload.namespace", workload.Namespace),
		attribute.String("workload.uid", workload.UID),
		attribute.String("workload.kind", workload.Kind),
	)

	// 1. Get or create statistic record
	getRecordSpan, getRecordCtx := trace.StartSpanFromContext(ctx, "getOrCreateRecord")
	facade := database.GetFacadeForCluster(clusterName).GetWorkloadStatistic()
	record, isNew, err := facade.GetOrCreate(getRecordCtx, clusterName, workload)
	if err != nil {
		getRecordSpan.RecordError(err)
		getRecordSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getRecordSpan)
		return fmt.Errorf("failed to get or create record: %w", err)
	}

	// Initialize histogram for new record or if histogram is nil/empty
	if isNew || record.Histogram == nil || len(record.Histogram) == 0 {
		hist := NewHistogram()
		histJSON, _ := hist.ToJSON()
		record.Histogram = dbModel.ExtJSON(histJSON)
	}

	getRecordSpan.SetAttributes(attribute.Bool("is_new_record", isNew))
	getRecordSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getRecordSpan)

	// 2. Calculate incremental query time range
	endTime := time.Now()
	startTime := j.calculateIncrementalStartTime(record, workload, endTime)

	// Skip if time interval is too short
	if endTime.Sub(startTime) < 30*time.Second {
		log.Debugf("Time interval too short for workload %s/%s, skipping", workload.Namespace, workload.Name)
		span.SetStatus(codes.Ok, "Time interval too short")
		return nil
	}

	span.SetAttributes(
		attribute.String("query_start", startTime.Format(time.RFC3339)),
		attribute.String("query_end", endTime.Format(time.RFC3339)),
		attribute.Float64("query_duration_minutes", endTime.Sub(startTime).Minutes()),
	)

	// 3. Query incremental data (only new data points)
	historySpan, historyCtx := trace.StartSpanFromContext(ctx, "queryIncrementalData")
	newValues, err := j.queryWorkloadHistoricalUtilization(historyCtx, workload, startTime, endTime, storageClientSet)
	if err != nil {
		historySpan.RecordError(err)
		historySpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(historySpan)
		return fmt.Errorf("failed to query incremental data: %w", err)
	}

	historySpan.SetAttributes(
		attribute.Int("new_samples", len(newValues)),
		attribute.String("time_window", endTime.Sub(startTime).String()),
	)
	historySpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(historySpan)

	// If no new data, only update instant utilization using lightweight column-only update
	// This avoids rewriting the entire row (including large JSONB fields) to reduce dead tuples
	if len(newValues) == 0 {
		log.Debugf("No new data for workload %s/%s, only updating instant utilization", workload.Namespace, workload.Name)

		// Query instant utilization
		instantUtilization, _ := j.queryWorkloadInstantUtilization(ctx, workload, storageClientSet)

		// For existing records, use lightweight column-only update
		if record.ID > 0 {
			updateFacade := database.GetFacadeForCluster(clusterName).GetWorkloadStatistic()
			if err := updateFacade.UpdateInstantOnly(ctx, record.ID, instantUtilization, endTime, endTime); err != nil {
				return fmt.Errorf("failed to update instant utilization: %w", err)
			}
		} else {
			// New record: need full create
			record.InstantGpuUtilization = instantUtilization
			record.LastQueryTime = endTime
			record.StatEndTime = endTime
			if len(record.Histogram) == 0 {
				record.Histogram = dbModel.ExtJSON(`{"buckets": []}`)
			}
			if record.Labels == nil {
				record.Labels = dbModel.ExtType{}
			}
			if record.Annotations == nil {
				record.Annotations = dbModel.ExtType{}
			}
			updateFacade := database.GetFacadeForCluster(clusterName).GetWorkloadStatistic()
			if err := updateFacade.Update(ctx, record); err != nil {
				return fmt.Errorf("failed to create record: %w", err)
			}
		}

		span.SetStatus(codes.Ok, "No new data")
		return nil
	}

	// 4. Incrementally update statistics
	updateSpan, _ := trace.StartSpanFromContext(ctx, "updateStatisticsIncremental")
	j.updateStatisticsIncremental(record, newValues, startTime, endTime)
	updateSpan.SetAttributes(
		attribute.Int("new_samples", len(newValues)),
		attribute.Int("total_samples", int(record.SampleCount)),
		attribute.Float64("avg_utilization", record.AvgGpuUtilization),
		attribute.Float64("p95_utilization", record.P95GpuUtilization),
	)
	updateSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(updateSpan)

	// 5. Update instant utilization
	instantSpan, instantCtx := trace.StartSpanFromContext(ctx, "queryInstantUtilization")
	instantUtilization, err := j.queryWorkloadInstantUtilization(instantCtx, workload, storageClientSet)
	if err != nil {
		log.Warnf("Failed to query instant utilization for workload %s/%s: %v", workload.Namespace, workload.Name, err)
		instantUtilization = 0
	}
	record.InstantGpuUtilization = instantUtilization
	instantSpan.SetAttributes(attribute.Float64("instant_utilization", instantUtilization))
	instantSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(instantSpan)

	// 6. Update timestamps and status
	record.LastQueryTime = endTime
	record.StatEndTime = endTime
	record.WorkloadStatus = string(workload.Status)
	record.AllocatedGpuCount = float64(workload.GpuRequest)

	// Ensure ExtJSON/ExtType fields are not nil before update
	if len(record.Histogram) == 0 {
		record.Histogram = dbModel.ExtJSON(`{"buckets": []}`)
	}

	// 7. Save to database
	// For existing records, use UpdateStatistics which only updates changed columns
	// (skipping immutable fields like uid, cluster_name, labels, annotations).
	// This reduces dead tuple size and improves autovacuum efficiency.
	saveSpan, saveCtx := trace.StartSpanFromContext(ctx, "saveToDatabase")
	saveFacade := database.GetFacadeForCluster(clusterName).GetWorkloadStatistic()

	if record.ID > 0 {
		err = saveFacade.UpdateStatistics(saveCtx, record)
	} else {
		// New record: need full create with all fields
		if record.Labels == nil {
			record.Labels = dbModel.ExtType{}
		}
		if record.Annotations == nil {
			record.Annotations = dbModel.ExtType{}
		}
		err = saveFacade.Update(saveCtx, record)
	}

	if err != nil {
		saveSpan.RecordError(err)
		saveSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(saveSpan)
		return fmt.Errorf("failed to save record: %w", err)
	}

	saveSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(saveSpan)

	log.Debugf("Updated statistic for workload %s/%s: avg=%.2f%%, p95=%.2f%%, samples=%d",
		workload.Namespace, workload.Name, record.AvgGpuUtilization, record.P95GpuUtilization, record.SampleCount)

	span.SetStatus(codes.Ok, "")
	return nil
}

// calculateIncrementalStartTime calculates the start time for incremental query
func (j *WorkloadStatisticJob) calculateIncrementalStartTime(record *dbModel.WorkloadStatistic, workload *dbModel.GpuWorkload, endTime time.Time) time.Time {
	// If has last query time, start from last query time
	if !record.LastQueryTime.IsZero() {
		return record.LastQueryTime
	}

	// If new record, start from workload creation time, but not exceeding max window
	workloadStartTime := workload.CreatedAt
	maxStartTime := endTime.Add(-MaxQueryWindow)

	if workloadStartTime.Before(maxStartTime) {
		log.Debugf("Workload %s/%s created at %v, limiting query start time to %v (max window: %v)",
			workload.Namespace, workload.Name, workloadStartTime, maxStartTime, MaxQueryWindow)
		return maxStartTime
	}

	return workloadStartTime
}

// updateStatisticsIncremental incrementally updates statistics with new values
func (j *WorkloadStatisticJob) updateStatisticsIncremental(record *dbModel.WorkloadStatistic, newValues []float64, startTime, endTime time.Time) {
	if len(newValues) == 0 {
		return
	}

	// 1. Update sample count and sum
	newSampleCount := len(newValues)
	newSum := 0.0
	newMin := newValues[0]
	newMax := newValues[0]

	for _, v := range newValues {
		newSum += v
		if v < newMin {
			newMin = v
		}
		if v > newMax {
			newMax = v
		}
	}

	// 2. Update global statistics
	oldSampleCount := record.SampleCount
	record.SampleCount += int32(newSampleCount)
	record.TotalSum += newSum
	record.AvgGpuUtilization = record.TotalSum / float64(record.SampleCount)

	// 3. Update min/max values
	if oldSampleCount == 0 {
		record.MinGpuUtilization = newMin
		record.MaxGpuUtilization = newMax
	} else {
		if newMin < record.MinGpuUtilization {
			record.MinGpuUtilization = newMin
		}
		if newMax > record.MaxGpuUtilization {
			record.MaxGpuUtilization = newMax
		}
	}

	// 4. Update histogram
	// Convert ExtType to JSON bytes
	histJSON, err := json.Marshal(record.Histogram)
	if err != nil {
		log.Errorf("Failed to marshal histogram from ExtType: %v", err)
		histJSON = []byte("{}")
	}

	hist, err := FromJSON(histJSON)
	if err != nil {
		log.Errorf("Failed to parse histogram: %v, creating new one", err)
		hist = NewHistogram()
	}

	hist.AddValues(newValues)

	// 5. Calculate percentiles from histogram
	p50, p90, p95 := calculatePercentilesFromHistogram(hist)
	record.P50GpuUtilization = p50
	record.P90GpuUtilization = p90
	record.P95GpuUtilization = p95

	// 6. Save updated histogram
	updatedHistJSON, err := hist.ToJSON()
	if err != nil {
		log.Errorf("Failed to marshal histogram: %v", err)
	} else {
		record.Histogram = dbModel.ExtJSON(updatedHistJSON)
	}

	// 7. Update statistics time range
	if record.StatStartTime.IsZero() {
		record.StatStartTime = startTime
	}
}

// calculateQueryStartTime calculates query start time (deprecated, kept for compatibility)
// Prefers workload creation time, but not exceeding max window to avoid excessive pressure on Prometheus
func (j *WorkloadStatisticJob) calculateQueryStartTime(workload *dbModel.GpuWorkload, endTime time.Time) time.Time {
	// Start from workload creation time
	workloadStartTime := workload.CreatedAt

	// Calculate earliest queryable time (not exceeding max window)
	maxStartTime := endTime.Add(-MaxQueryWindow)

	// Use the later time as start time
	if workloadStartTime.Before(maxStartTime) {
		log.Debugf("Workload %s/%s created at %v, limiting query start time to %v (max window: %v)",
			workload.Namespace, workload.Name, workloadStartTime, maxStartTime, MaxQueryWindow)
		return maxStartTime
	}

	log.Debugf("Using workload %s/%s creation time %v as query start time",
		workload.Namespace, workload.Name, workloadStartTime)
	return workloadStartTime
}

// queryWorkloadInstantUtilization queries the instant GPU utilization of a workload
func (j *WorkloadStatisticJob) queryWorkloadInstantUtilization(ctx context.Context, workload *dbModel.GpuWorkload, storageClientSet *clientsets.StorageClientSet) (float64, error) {
	// Build Prometheus query, aggregate by workload_uid
	query := fmt.Sprintf(`avg(workload_gpu_utilization{workload_uid="%s"})`, workload.UID)

	samples, err := prom.QueryInstant(ctx, storageClientSet, query)
	if err != nil {
		return 0, fmt.Errorf("failed to query instant utilization: %w", err)
	}

	if len(samples) == 0 {
		return 0, nil
	}

	return float64(samples[0].Value), nil
}

// queryWorkloadHistoricalUtilization queries historical GPU utilization data of a workload
func (j *WorkloadStatisticJob) queryWorkloadHistoricalUtilization(ctx context.Context, workload *dbModel.GpuWorkload, start, end time.Time, storageClientSet *clientsets.StorageClientSet) ([]float64, error) {
	// Build Prometheus range query, aggregate by workload_uid
	query := fmt.Sprintf(`avg(workload_gpu_utilization{workload_uid="%s"})`, workload.UID)

	// Don't filter labels, get all labels
	series, err := prom.QueryRange(ctx, storageClientSet, query, start, end, j.queryStep, map[string]struct{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to query range: %w", err)
	}

	if len(series) == 0 {
		return []float64{}, nil
	}

	// Extract values from all time points
	values := make([]float64, 0, len(series[0].Values))
	for _, point := range series[0].Values {
		if !math.IsNaN(point.Value) && !math.IsInf(point.Value, 0) {
			values = append(values, point.Value)
		}
	}

	return values, nil
}

// ============================================
// Legacy methods (kept for reference, not used in incremental mode)
// ============================================

// StatisticData is the statistics data structure (deprecated)
type StatisticData struct {
	InstantUtilization float64
	AvgUtilization     float64
	MaxUtilization     float64
	MinUtilization     float64
	P50Utilization     float64
	P90Utilization     float64
	P95Utilization     float64
}

// Schedule returns the job schedule expression
// Runs every 60 seconds (reduced from 30s to lower dead tuple generation).
// 60s still provides timely GPU utilization updates while halving the DB write pressure.
func (j *WorkloadStatisticJob) Schedule() string {
	return "@every 60s"
}
