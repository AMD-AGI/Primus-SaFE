// Package statistics provides statistical calculation utilities for GPU resource analysis.
package statistics

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promModel "github.com/prometheus/common/model"
)

const (
	// WorkloadUtilizationQueryTemplate is the PromQL query template for workload GPU utilization
	WorkloadUtilizationQueryTemplate = `avg(workload_gpu_utilization{workload_uid="%s"})`

	// DefaultWorkloadPromQueryStep is the default step for workload Prometheus queries (in seconds)
	DefaultWorkloadPromQueryStep = 60
)

// GpuUtilizationResult represents the result of GPU utilization query
type GpuUtilizationResult struct {
	// AvgUtilization is the average GPU utilization percentage
	AvgUtilization float64

	// QueryTime is the time used for the query
	QueryTime time.Time
}

// ClusterGpuUtilizationStats represents complete GPU utilization statistics
type ClusterGpuUtilizationStats struct {
	// AvgUtilization is the average GPU utilization percentage
	AvgUtilization float64

	// MaxUtilization is the maximum GPU utilization percentage
	MaxUtilization float64

	// MinUtilization is the minimum GPU utilization percentage
	MinUtilization float64

	// P50Utilization is the median (50th percentile) GPU utilization percentage
	P50Utilization float64

	// P95Utilization is the 95th percentile GPU utilization percentage
	P95Utilization float64
}

// QueryClusterHourlyGpuUtilization queries the average GPU utilization for the entire cluster
// for a specific hour using avg(avg_over_time(gpu_utilization{}[1h]))
//
// Parameters:
//   - ctx: context for the query
//   - storageClientSet: storage client set containing Prometheus client
//   - hour: the hour to query (start of the hour)
//
// Returns:
//   - float64: average GPU utilization percentage
//   - error: if the query fails
func QueryClusterHourlyGpuUtilization(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	hour time.Time,
) (float64, error) {
	if storageClientSet == nil || storageClientSet.PrometheusRead == nil {
		return 0, fmt.Errorf("prometheus client is not initialized")
	}

	promAPI := v1.NewAPI(storageClientSet.PrometheusRead)

	// Query avg_over_time for the hour period
	// hour is the start of the hour, we want to query at the end of the hour
	// to get the complete data for that hour
	queryTime := hour.Truncate(time.Hour).Add(time.Hour)
	query := "avg(avg_over_time(gpu_utilization{}[1h]))"

	result, warnings, err := promAPI.Query(ctx, query, queryTime)
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %w", err)
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings for cluster GPU utilization: %v", warnings)
	}

	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Debugf("No GPU utilization data returned for hour %v", hour)
		return 0, nil
	}

	return float64(vectorVal[0].Value), nil
}

// QueryNamespaceHourlyGpuUtilization queries the average GPU utilization for a specific namespace
// for a specific hour using avg(avg_over_time(gpu_utilization{namespace="..."}[1h]))
//
// Parameters:
//   - ctx: context for the query
//   - storageClientSet: storage client set containing Prometheus client
//   - namespace: the namespace to query
//   - hour: the hour to query (start of the hour)
//
// Returns:
//   - float64: average GPU utilization percentage for the namespace
//   - error: if the query fails
func QueryNamespaceHourlyGpuUtilization(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	namespace string,
	hour time.Time,
) (float64, error) {
	if storageClientSet == nil || storageClientSet.PrometheusRead == nil {
		return 0, fmt.Errorf("prometheus client is not initialized")
	}

	promAPI := v1.NewAPI(storageClientSet.PrometheusRead)

	// Query avg_over_time for the hour period with namespace filter
	queryTime := hour.Truncate(time.Hour).Add(time.Hour)
	query := fmt.Sprintf(`avg(avg_over_time(gpu_utilization{namespace="%s"}[1h]))`, namespace)

	result, warnings, err := promAPI.Query(ctx, query, queryTime)
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %w", err)
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings for namespace %s GPU utilization: %v", namespace, warnings)
	}

	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Debugf("No GPU utilization data returned for namespace %s at hour %v", namespace, hour)
		return 0, nil
	}

	return float64(vectorVal[0].Value), nil
}

// QueryClusterInstantGpuUtilization queries the current instant GPU utilization for the entire cluster
// using avg(gpu_utilization{})
//
// Parameters:
//   - ctx: context for the query
//   - storageClientSet: storage client set containing Prometheus client
//
// Returns:
//   - float64: current average GPU utilization percentage
//   - error: if the query fails
func QueryClusterInstantGpuUtilization(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
) (float64, error) {
	if storageClientSet == nil || storageClientSet.PrometheusRead == nil {
		return 0, fmt.Errorf("prometheus client is not initialized")
	}

	promAPI := v1.NewAPI(storageClientSet.PrometheusRead)
	query := "avg(gpu_utilization{})"

	result, warnings, err := promAPI.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %w", err)
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings for instant GPU utilization: %v", warnings)
	}

	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Debugf("No instant GPU utilization data returned")
		return 0, nil
	}

	return float64(vectorVal[0].Value), nil
}

// CalculateWorkloadsUtilizationWeighted queries GPU utilization for multiple workloads
// and calculates weighted average based on each workload's GPU count.
// Returns UtilizationStats with weighted avg, max, and min.
//
// Parameters:
//   - ctx: context for the query
//   - storageClientSet: storage client set containing Prometheus client
//   - workloadGpuCounts: map of workload UID to its GPU count
//   - startTime: start time of the query range
//   - endTime: end time of the query range
//   - promQueryStep: step for Prometheus queries (in seconds), use 0 for default
//
// Returns:
//   - UtilizationStats: weighted utilization statistics
func CalculateWorkloadsUtilizationWeighted(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadGpuCounts map[string]int32,
	startTime, endTime time.Time,
	promQueryStep int,
) UtilizationStats {
	result := UtilizationStats{
		AvgUtilization: 0,
		MaxUtilization: 0,
		MinUtilization: 0,
	}

	if len(workloadGpuCounts) == 0 {
		return result
	}

	if promQueryStep <= 0 {
		promQueryStep = DefaultWorkloadPromQueryStep
	}

	// Collect per-workload average utilization with GPU weight
	type workloadUtilization struct {
		avgUtilization float64
		maxUtilization float64
		minUtilization float64
		gpuCount       int32
		hasData        bool
	}

	workloadUtils := make([]workloadUtilization, 0, len(workloadGpuCounts))
	var totalGpuWithData int32

	for workloadUID, gpuCount := range workloadGpuCounts {
		values, err := QueryWorkloadUtilizationRange(ctx, storageClientSet, workloadUID, startTime, endTime, promQueryStep)
		if err != nil {
			log.Debugf("Failed to query utilization for workload %s: %v", workloadUID, err)
			continue
		}

		if len(values) == 0 {
			continue
		}

		// Calculate stats for this workload
		stats := CalculateUtilizationStats(values)
		workloadUtils = append(workloadUtils, workloadUtilization{
			avgUtilization: stats.AvgUtilization,
			maxUtilization: stats.MaxUtilization,
			minUtilization: stats.MinUtilization,
			gpuCount:       gpuCount,
			hasData:        true,
		})
		totalGpuWithData += gpuCount
	}

	if len(workloadUtils) == 0 || totalGpuWithData == 0 {
		return result
	}

	// Calculate weighted average utilization
	var weightedSum float64
	var maxUtil, minUtil float64
	minUtil = 100.0 // Initialize with max possible value

	for _, wu := range workloadUtils {
		if !wu.hasData {
			continue
		}
		// Weight by GPU count
		weightedSum += wu.avgUtilization * float64(wu.gpuCount)

		// Track overall max and min
		if wu.maxUtilization > maxUtil {
			maxUtil = wu.maxUtilization
		}
		if wu.minUtilization < minUtil {
			minUtil = wu.minUtilization
		}
	}

	result.AvgUtilization = weightedSum / float64(totalGpuWithData)
	result.MaxUtilization = maxUtil
	result.MinUtilization = minUtil

	return result
}

// QueryWorkloadUtilizationRange queries the GPU utilization for a workload in a time range
// Returns all data points for detailed statistics.
//
// Parameters:
//   - ctx: context for the query
//   - storageClientSet: storage client set containing Prometheus client
//   - workloadUID: the workload UID to query
//   - startTime: start time of the query range
//   - endTime: end time of the query range
//   - promQueryStep: step for Prometheus queries (in seconds)
//
// Returns:
//   - []float64: utilization values
//   - error: if the query fails
func QueryWorkloadUtilizationRange(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string,
	startTime, endTime time.Time,
	promQueryStep int,
) ([]float64, error) {
	if promQueryStep <= 0 {
		promQueryStep = DefaultWorkloadPromQueryStep
	}

	query := fmt.Sprintf(WorkloadUtilizationQueryTemplate, workloadUID)

	series, err := prom.QueryRange(ctx, storageClientSet, query, startTime, endTime,
		promQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		return nil, err
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		return []float64{}, nil
	}

	// Collect all data points
	values := make([]float64, 0, len(series[0].Values))
	for _, point := range series[0].Values {
		values = append(values, point.Value)
	}

	return values, nil
}

// calculateUtilizationStatsWithPercentiles calculates comprehensive statistics from a set of utilization values
// including average, max, min, and percentiles (p50, p95)
func calculateUtilizationStatsWithPercentiles(values []float64) *ClusterGpuUtilizationStats {
	stats := &ClusterGpuUtilizationStats{}

	if len(values) == 0 {
		return stats
	}

	// Calculate avg, max, min
	var sum, max, min float64
	min = 100.0 // Initialize with max possible value
	max = 0.0

	for _, v := range values {
		sum += v
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}

	stats.AvgUtilization = sum / float64(len(values))
	stats.MaxUtilization = max
	stats.MinUtilization = min

	// Calculate percentiles by sorting and indexing
	// Make a copy to avoid modifying the original slice
	sortedValues := make([]float64, len(values))
	copy(sortedValues, values)

	// Simple insertion sort (efficient for small datasets like 30 points)
	for i := 1; i < len(sortedValues); i++ {
		key := sortedValues[i]
		j := i - 1
		for j >= 0 && sortedValues[j] > key {
			sortedValues[j+1] = sortedValues[j]
			j--
		}
		sortedValues[j+1] = key
	}

	// Calculate percentiles using linear interpolation
	stats.P50Utilization = calculatePercentile(sortedValues, 0.50)
	stats.P95Utilization = calculatePercentile(sortedValues, 0.95)

	return stats
}

// calculatePercentile calculates the percentile value from a sorted array
// Uses linear interpolation between values if needed
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}

	// Calculate the index position
	pos := percentile * float64(len(sortedValues)-1)
	lower := int(pos)
	upper := lower + 1

	if upper >= len(sortedValues) {
		return sortedValues[len(sortedValues)-1]
	}

	// Linear interpolation
	fraction := pos - float64(lower)
	return sortedValues[lower]*(1-fraction) + sortedValues[upper]*fraction
}

// QueryClusterHourlyGpuUtilizationStats queries complete GPU utilization statistics for the entire cluster
// for a specific hour including avg, max, min, and percentiles (p50, p95)
//
// This function uses a single range query with 120s step to reduce Prometheus pressure,
// then calculates all statistics from the returned data points.
//
// Parameters:
//   - ctx: context for the query
//   - storageClientSet: storage client set containing Prometheus client
//   - hour: the hour to query (start of the hour)
//
// Returns:
//   - *ClusterGpuUtilizationStats: complete utilization statistics
//   - error: if the query fails
func QueryClusterHourlyGpuUtilizationStats(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	hour time.Time,
) (*ClusterGpuUtilizationStats, error) {
	if storageClientSet == nil || storageClientSet.PrometheusRead == nil {
		return nil, fmt.Errorf("prometheus client is not initialized")
	}

	// Truncate to hour boundary
	startTime := hour.Truncate(time.Hour)
	endTime := startTime.Add(time.Hour)

	// Use 120s step for the range query (30 data points per hour)
	step := 120
	query := "avg(gpu_utilization{})"

	// Query range data for the hour
	series, err := prom.QueryRange(ctx, storageClientSet, query, startTime, endTime, step, map[string]struct{}{"__name__": {}})
	if err != nil {
		return nil, fmt.Errorf("failed to query GPU utilization range: %w", err)
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		log.Debugf("No GPU utilization data returned for hour %v", hour)
		return &ClusterGpuUtilizationStats{}, nil
	}

	// Extract values from the time series
	values := make([]float64, 0, len(series[0].Values))
	for _, point := range series[0].Values {
		values = append(values, point.Value)
	}

	// Calculate statistics from the values
	stats := calculateUtilizationStatsWithPercentiles(values)

	log.Debugf("Cluster GPU utilization stats for hour %v: avg=%.2f%%, max=%.2f%%, min=%.2f%%, p50=%.2f%%, p95=%.2f%% (samples=%d)",
		hour, stats.AvgUtilization, stats.MaxUtilization, stats.MinUtilization, stats.P50Utilization, stats.P95Utilization, len(values))

	return stats, nil
}
