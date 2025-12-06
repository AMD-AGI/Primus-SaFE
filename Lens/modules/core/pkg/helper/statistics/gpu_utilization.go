// Package statistics provides statistical calculation utilities for GPU resource analysis.
package statistics

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promModel "github.com/prometheus/common/model"
)

// GpuUtilizationResult represents the result of GPU utilization query
type GpuUtilizationResult struct {
	// AvgUtilization is the average GPU utilization percentage
	AvgUtilization float64

	// QueryTime is the time used for the query
	QueryTime time.Time
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
