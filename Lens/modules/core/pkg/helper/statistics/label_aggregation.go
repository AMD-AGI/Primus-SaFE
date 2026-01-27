// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package statistics provides statistical calculation utilities for GPU resource analysis.
package statistics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// DimensionTypeLabel represents label dimension type
	DimensionTypeLabel = "label"

	// DimensionTypeAnnotation represents annotation dimension type
	DimensionTypeAnnotation = "annotation"
)

// LabelAggregationConfig is the configuration for label/annotation aggregation
type LabelAggregationConfig struct {
	// LabelKeys is the list of label keys to aggregate
	LabelKeys []string

	// AnnotationKeys is the list of annotation keys to aggregate
	AnnotationKeys []string

	// DefaultValue is the default value when label/annotation is not found
	DefaultValue string
}

// LabelAggregationResult represents the aggregated result for a label/annotation dimension
type LabelAggregationResult struct {
	// DimensionType is either "label" or "annotation"
	DimensionType string

	// DimensionKey is the label or annotation key
	DimensionKey string

	// DimensionValue is the value of the label or annotation
	DimensionValue string

	// TotalAllocatedGpu is the total GPU count allocated to workloads with this label/annotation
	TotalAllocatedGpu float64

	// ActiveWorkloadCount is the number of active workloads with this label/annotation
	ActiveWorkloadCount int

	// WorkloadUIDs is the list of workload UIDs in this aggregation
	WorkloadUIDs []string

	// WorkloadGpuCounts maps workload UID to its GPU count (for weighted utilization calculation)
	WorkloadGpuCounts map[string]int32

	// UtilizationValues contains all utilization data points for this aggregation
	UtilizationValues []float64
}

// LabelAggregationSummary contains the aggregated results for all label/annotation dimensions
type LabelAggregationSummary struct {
	// Results is the map of dimension key (type:key:value) to aggregation result
	Results map[string]*LabelAggregationResult

	// Hour is the hour for which the aggregation was performed
	Hour time.Time

	// TotalWorkloads is the total number of workloads processed
	TotalWorkloads int
}

// UtilizationStats contains calculated utilization statistics
type UtilizationStats struct {
	AvgUtilization float64
	MaxUtilization float64
	MinUtilization float64
}

// LabelAggregationCalculator calculates GPU aggregation by label/annotation dimensions
type LabelAggregationCalculator struct {
	workloadFacade          database.WorkloadFacadeInterface
	podFacade               database.PodFacadeInterface
	podRunningPeriodsFacade database.PodRunningPeriodsFacadeInterface
	clusterName             string
	config                  *LabelAggregationConfig
}

// NewLabelAggregationCalculator creates a new calculator for label/annotation aggregation
func NewLabelAggregationCalculator(clusterName string, config *LabelAggregationConfig) *LabelAggregationCalculator {
	facade := database.GetFacadeForCluster(clusterName)
	return &LabelAggregationCalculator{
		workloadFacade:          facade.GetWorkload(),
		podFacade:               facade.GetPod(),
		podRunningPeriodsFacade: facade.GetPodRunningPeriods(),
		clusterName:             clusterName,
		config:                  config,
	}
}

// NewLabelAggregationCalculatorWithFacade creates a calculator with custom facade (useful for testing)
func NewLabelAggregationCalculatorWithFacade(
	workloadFacade database.WorkloadFacadeInterface,
	podFacade database.PodFacadeInterface,
	clusterName string,
	config *LabelAggregationConfig,
) *LabelAggregationCalculator {
	return &LabelAggregationCalculator{
		workloadFacade:          workloadFacade,
		podFacade:               podFacade,
		podRunningPeriodsFacade: database.GetFacadeForCluster(clusterName).GetPodRunningPeriods(),
		clusterName:             clusterName,
		config:                  config,
	}
}

// CalculateHourlyLabelAggregation calculates GPU aggregation by label/annotation for a specific hour
//
// Parameters:
//   - ctx: context for database operations
//   - hour: the hour to calculate aggregation for
//
// Returns:
//   - LabelAggregationSummary containing all aggregation results
//   - error if any operation fails
func (c *LabelAggregationCalculator) CalculateHourlyLabelAggregation(
	ctx context.Context,
	hour time.Time,
) (*LabelAggregationSummary, error) {
	hourStart := hour.Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	return c.CalculateLabelAggregation(ctx, hourStart, hourEnd)
}

// CalculateLabelAggregation calculates GPU aggregation by label/annotation for a time range
// Uses pod_running_periods table for accurate running pod detection
// Falls back to gpu_pods table if no running periods data exists (backward compatibility)
//
// Parameters:
//   - ctx: context for database operations
//   - startTime: start of the time range
//   - endTime: end of the time range
//
// Returns:
//   - LabelAggregationSummary containing all aggregation results
//   - error if any operation fails
func (c *LabelAggregationCalculator) CalculateLabelAggregation(
	ctx context.Context,
	startTime, endTime time.Time,
) (*LabelAggregationSummary, error) {
	summary := &LabelAggregationSummary{
		Results: make(map[string]*LabelAggregationResult),
		Hour:    startTime.Truncate(time.Hour),
	}

	// Try to use pod_running_periods table first for accurate detection
	var podUIDs []string
	var err error

	if c.podRunningPeriodsFacade != nil {
		podUIDs, err = c.getRunningPodUIDsFromPeriods(ctx, startTime, endTime)
		if err != nil {
			log.Warnf("Failed to get running pod UIDs from periods, falling back to legacy method: %v", err)
			podUIDs = nil
		}
	}

	// Fallback to legacy method if no running periods data
	if len(podUIDs) == 0 {
		log.Debugf("No running periods data found for label aggregation in time range %v - %v, using legacy method",
			startTime, endTime)
		podUIDs, err = c.getActivePodUIDsLegacy(ctx, startTime, endTime)
		if err != nil {
			return nil, fmt.Errorf("failed to get active pods in time range: %w", err)
		}
	}

	if len(podUIDs) == 0 {
		log.Debugf("No active pods found for label aggregation in time range %v - %v", startTime, endTime)
		return summary, nil
	}

	// Find workload UIDs through pod references
	workloadUIDs, err := c.workloadFacade.ListWorkloadUidsByPodUids(ctx, podUIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload UIDs by pod UIDs: %w", err)
	}

	if len(workloadUIDs) == 0 {
		log.Debugf("No workloads found for active pods in time range %v - %v", startTime, endTime)
		return summary, nil
	}

	// Get top-level workloads
	workloads, err := c.workloadFacade.ListTopLevelWorkloadByUids(ctx, workloadUIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get top-level workloads: %w", err)
	}

	log.Debugf("Found %d top-level workloads with active pods for label aggregation in time range %v - %v",
		len(workloads), startTime, endTime)

	summary.TotalWorkloads = len(workloads)

	if len(workloads) == 0 {
		return summary, nil
	}

	// Build aggregation map
	c.aggregateWorkloads(workloads, summary)

	return summary, nil
}

// getRunningPodUIDsFromPeriods gets pod UIDs that have running periods in the time range
func (c *LabelAggregationCalculator) getRunningPodUIDsFromPeriods(
	ctx context.Context,
	startTime, endTime time.Time,
) ([]string, error) {
	runningPeriods, err := c.podRunningPeriodsFacade.ListRunningPeriodsInTimeRange(ctx, startTime, endTime)
	if err != nil {
		return nil, err
	}

	// Build unique pod UID list
	podUIDMap := make(map[string]struct{})
	for _, period := range runningPeriods {
		podUIDMap[period.PodUID] = struct{}{}
	}

	podUIDs := make([]string, 0, len(podUIDMap))
	for uid := range podUIDMap {
		podUIDs = append(podUIDs, uid)
	}

	return podUIDs, nil
}

// getActivePodUIDsLegacy gets pod UIDs using the legacy method (gpu_pods table)
func (c *LabelAggregationCalculator) getActivePodUIDsLegacy(
	ctx context.Context,
	startTime, endTime time.Time,
) ([]string, error) {
	activePods, err := c.podFacade.ListPodsActiveInTimeRange(ctx, startTime, endTime)
	if err != nil {
		return nil, err
	}

	podUIDs := make([]string, 0, len(activePods))
	for _, pod := range activePods {
		podUIDs = append(podUIDs, pod.UID)
	}

	return podUIDs, nil
}

// CalculateLabelAggregationFromWorkloads calculates aggregation from a pre-fetched list of workloads
// This is useful when workloads have already been fetched by the caller
func (c *LabelAggregationCalculator) CalculateLabelAggregationFromWorkloads(
	workloads []*model.GpuWorkload,
	hour time.Time,
) *LabelAggregationSummary {
	summary := &LabelAggregationSummary{
		Results:        make(map[string]*LabelAggregationResult),
		Hour:           hour,
		TotalWorkloads: len(workloads),
	}

	if len(workloads) == 0 {
		return summary
	}

	c.aggregateWorkloads(workloads, summary)
	return summary
}

// aggregateWorkloads performs the actual aggregation of workloads by label/annotation
func (c *LabelAggregationCalculator) aggregateWorkloads(
	workloads []*model.GpuWorkload,
	summary *LabelAggregationSummary,
) {
	for _, workload := range workloads {
		// Process label keys
		for _, labelKey := range c.config.LabelKeys {
			value := GetLabelValue(workload.Labels, labelKey, c.config.DefaultValue)
			key := BuildDimensionKey(DimensionTypeLabel, labelKey, value)
			if value == c.config.DefaultValue {
				continue
			}
			if _, exists := summary.Results[key]; !exists {
				summary.Results[key] = &LabelAggregationResult{
					DimensionType:     DimensionTypeLabel,
					DimensionKey:      labelKey,
					DimensionValue:    value,
					WorkloadUIDs:      make([]string, 0),
					WorkloadGpuCounts: make(map[string]int32),
				}
			}
			summary.Results[key].TotalAllocatedGpu += float64(workload.GpuRequest)
			summary.Results[key].WorkloadUIDs = append(summary.Results[key].WorkloadUIDs, workload.UID)
			summary.Results[key].WorkloadGpuCounts[workload.UID] = workload.GpuRequest
			summary.Results[key].ActiveWorkloadCount++
		}

		// Process annotation keys
		for _, annotationKey := range c.config.AnnotationKeys {
			value := GetAnnotationValue(workload.Annotations, annotationKey, c.config.DefaultValue)
			key := BuildDimensionKey(DimensionTypeAnnotation, annotationKey, value)
			if value == c.config.DefaultValue {
				continue
			}
			if _, exists := summary.Results[key]; !exists {
				summary.Results[key] = &LabelAggregationResult{
					DimensionType:     DimensionTypeAnnotation,
					DimensionKey:      annotationKey,
					DimensionValue:    value,
					WorkloadUIDs:      make([]string, 0),
					WorkloadGpuCounts: make(map[string]int32),
				}
			}
			summary.Results[key].TotalAllocatedGpu += float64(workload.GpuRequest)
			summary.Results[key].WorkloadUIDs = append(summary.Results[key].WorkloadUIDs, workload.UID)
			summary.Results[key].WorkloadGpuCounts[workload.UID] = workload.GpuRequest
			summary.Results[key].ActiveWorkloadCount++
		}
	}
}

// AddUtilizationValues adds utilization values to an aggregation result
func (r *LabelAggregationResult) AddUtilizationValues(values []float64) {
	r.UtilizationValues = append(r.UtilizationValues, values...)
}

// CalculateUtilizationStats calculates utilization statistics from the collected values
func (r *LabelAggregationResult) CalculateUtilizationStats() UtilizationStats {
	return CalculateUtilizationStats(r.UtilizationValues)
}

// CalculateUtilizationStats calculates avg, max, min utilization from a slice of values
func CalculateUtilizationStats(values []float64) UtilizationStats {
	stats := UtilizationStats{}

	if len(values) == 0 {
		return stats
	}

	// Sort for min/max
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	stats.MinUtilization = sorted[0]
	stats.MaxUtilization = sorted[len(sorted)-1]

	// Calculate average
	var sum float64
	for _, v := range values {
		sum += v
	}
	stats.AvgUtilization = sum / float64(len(values))

	return stats
}

// BuildDimensionKey builds a unique key for dimension type, key, and value
func BuildDimensionKey(dimensionType, dimensionKey, dimensionValue string) string {
	return fmt.Sprintf("%s:%s:%s", dimensionType, dimensionKey, dimensionValue)
}

// GetLabelValue gets the value of a label key from workload labels
// Returns defaultValue if the label is not found or empty
func GetLabelValue(labels model.ExtType, key string, defaultValue string) string {
	if labels == nil {
		return defaultValue
	}

	if value, ok := labels[key]; ok {
		if strValue, ok := value.(string); ok && strValue != "" {
			return strValue
		}
	}

	return defaultValue
}

// GetAnnotationValue gets the value of an annotation key from workload annotations
// Returns defaultValue if the annotation is not found or empty
func GetAnnotationValue(annotations model.ExtType, key string, defaultValue string) string {
	if annotations == nil {
		return defaultValue
	}

	if value, ok := annotations[key]; ok {
		if strValue, ok := value.(string); ok && strValue != "" {
			return strValue
		}
	}

	return defaultValue
}

// HasConfiguredKeys checks if the configuration has any label or annotation keys
func (c *LabelAggregationConfig) HasConfiguredKeys() bool {
	return len(c.LabelKeys) > 0 || len(c.AnnotationKeys) > 0
}

// GetAllKeys returns all configured keys (both labels and annotations)
func (c *LabelAggregationConfig) GetAllKeys() []struct {
	Type string
	Key  string
} {
	result := make([]struct {
		Type string
		Key  string
	}, 0, len(c.LabelKeys)+len(c.AnnotationKeys))

	for _, key := range c.LabelKeys {
		result = append(result, struct {
			Type string
			Key  string
		}{Type: DimensionTypeLabel, Key: key})
	}

	for _, key := range c.AnnotationKeys {
		result = append(result, struct {
			Type string
			Key  string
		}{Type: DimensionTypeAnnotation, Key: key})
	}

	return result
}
