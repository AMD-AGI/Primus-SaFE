// Package statistics provides statistical calculation utilities for GPU resource analysis.
package statistics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// DefaultPromQueryStep is the default step for Prometheus queries (in seconds)
	DefaultPromQueryStep = 30
)

// NamespaceUtilizationResult represents the utilization statistics for a namespace
type NamespaceUtilizationResult struct {
	AvgUtilization float64
	MinUtilization float64
	MaxUtilization float64
	Values         []float64
}

// NamespaceUtilizationCalculator calculates GPU utilization for namespaces
type NamespaceUtilizationCalculator struct {
	clusterName      string
	mappingFacade    database.NodeNamespaceMappingFacadeInterface
	storageClientSet *clientsets.StorageClientSet
	promQueryStep    int
}

// NewNamespaceUtilizationCalculator creates a new NamespaceUtilizationCalculator
func NewNamespaceUtilizationCalculator(
	clusterName string,
	storageClientSet *clientsets.StorageClientSet,
) *NamespaceUtilizationCalculator {
	return &NamespaceUtilizationCalculator{
		clusterName:      clusterName,
		mappingFacade:    database.GetFacadeForCluster(clusterName).GetNodeNamespaceMapping(),
		storageClientSet: storageClientSet,
		promQueryStep:    DefaultPromQueryStep,
	}
}

// SetPromQueryStep sets the Prometheus query step
func (c *NamespaceUtilizationCalculator) SetPromQueryStep(step int) {
	c.promQueryStep = step
}

// CalculateNamespaceUtilization calculates GPU utilization for a namespace
// Strategy:
// 1. First try to find node-namespace mapping history and query by node
// 2. If no mapping found, query by workload UIDs
func (c *NamespaceUtilizationCalculator) CalculateNamespaceUtilization(
	ctx context.Context,
	namespace string,
	allocationResult *GpuAllocationResult,
	startTime, endTime time.Time,
) *NamespaceUtilizationResult {
	// Strategy 1: Try to get utilization by node-namespace mapping
	nodeUtilization := c.queryUtilizationByNodeMapping(ctx, namespace, startTime, endTime)
	if len(nodeUtilization) > 0 {
		log.Debugf("Got utilization for namespace %s from node mapping: %d values", namespace, len(nodeUtilization))
		return c.buildUtilizationResult(nodeUtilization)
	}

	// Strategy 2: Fallback to workload-based utilization
	workloadUtilization := c.queryUtilizationByWorkloads(ctx, allocationResult, startTime, endTime)
	if len(workloadUtilization) > 0 {
		log.Debugf("Got utilization for namespace %s from workloads: %d values", namespace, len(workloadUtilization))
		return c.buildUtilizationResult(workloadUtilization)
	}

	log.Debugf("No utilization data found for namespace %s", namespace)
	return &NamespaceUtilizationResult{}
}

// CalculateHourlyNamespaceUtilization calculates GPU utilization for a namespace for a specific hour
func (c *NamespaceUtilizationCalculator) CalculateHourlyNamespaceUtilization(
	ctx context.Context,
	namespace string,
	allocationResult *GpuAllocationResult,
	hour time.Time,
) *NamespaceUtilizationResult {
	startTime := hour.Truncate(time.Hour)
	endTime := startTime.Add(time.Hour)
	return c.CalculateNamespaceUtilization(ctx, namespace, allocationResult, startTime, endTime)
}

// buildUtilizationResult builds a NamespaceUtilizationResult from utilization values
func (c *NamespaceUtilizationCalculator) buildUtilizationResult(values []float64) *NamespaceUtilizationResult {
	if len(values) == 0 {
		return &NamespaceUtilizationResult{}
	}

	result := &NamespaceUtilizationResult{
		Values: values,
	}

	sort.Float64s(values)
	result.MinUtilization = values[0]
	result.MaxUtilization = values[len(values)-1]

	var sum float64
	for _, v := range values {
		sum += v
	}
	result.AvgUtilization = sum / float64(len(values))

	return result
}

// queryUtilizationByNodeMapping queries GPU utilization using node-namespace mapping history
// Uses query: avg(gpu_utilization{primus_lens_node_name="nodename"})
func (c *NamespaceUtilizationCalculator) queryUtilizationByNodeMapping(
	ctx context.Context,
	namespace string,
	startTime, endTime time.Time,
) []float64 {
	// Get node mappings for this namespace at the query time
	// Use the middle of the time range as the reference time
	midTime := startTime.Add(endTime.Sub(startTime) / 2)
	mappings, err := c.mappingFacade.ListHistoryByNamespaceNameAtTime(ctx, namespace, midTime)
	if err != nil {
		log.Warnf("Failed to get node mappings for namespace %s: %v", namespace, err)
		return nil
	}

	if len(mappings) == 0 {
		return nil
	}

	// Query utilization for each node and collect all values
	allValues := make([]float64, 0)
	for _, mapping := range mappings {
		nodeValues := c.queryNodeUtilization(ctx, mapping.NodeName, startTime, endTime)
		if len(nodeValues) > 0 {
			// Calculate average for this node
			var sum float64
			for _, v := range nodeValues {
				sum += v
			}
			avgValue := sum / float64(len(nodeValues))
			allValues = append(allValues, avgValue)
		}
	}

	return allValues
}

// queryNodeUtilization queries GPU utilization for a specific node
func (c *NamespaceUtilizationCalculator) queryNodeUtilization(
	ctx context.Context,
	nodeName string,
	startTime, endTime time.Time,
) []float64 {
	query := fmt.Sprintf(`avg(gpu_utilization{primus_lens_node_name="%s"})`, nodeName)

	series, err := prom.QueryRange(ctx, c.storageClientSet, query, startTime, endTime,
		c.promQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		log.Warnf("Failed to query GPU utilization for node %s: %v", nodeName, err)
		return nil
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		return nil
	}

	values := make([]float64, 0, len(series[0].Values))
	for _, point := range series[0].Values {
		values = append(values, point.Value)
	}

	return values
}

// queryUtilizationByWorkloads queries GPU utilization using workload UIDs
// Uses query: avg(workload_gpu_utilization{workload_uid="%s"})
func (c *NamespaceUtilizationCalculator) queryUtilizationByWorkloads(
	ctx context.Context,
	allocationResult *GpuAllocationResult,
	startTime, endTime time.Time,
) []float64 {
	if allocationResult == nil || len(allocationResult.Details) == 0 {
		return nil
	}

	allValues := make([]float64, 0)
	for _, detail := range allocationResult.Details {
		workloadValues := c.queryWorkloadUtilization(ctx, detail.WorkloadUID, startTime, endTime)
		if len(workloadValues) > 0 {
			// Calculate average for this workload
			var sum float64
			for _, v := range workloadValues {
				sum += v
			}
			avgValue := sum / float64(len(workloadValues))
			allValues = append(allValues, avgValue)
		}
	}

	return allValues
}

// queryWorkloadUtilization queries GPU utilization for a specific workload
func (c *NamespaceUtilizationCalculator) queryWorkloadUtilization(
	ctx context.Context,
	workloadUID string,
	startTime, endTime time.Time,
) []float64 {
	query := fmt.Sprintf(`avg(workload_gpu_utilization{workload_uid="%s"})`, workloadUID)

	series, err := prom.QueryRange(ctx, c.storageClientSet, query, startTime, endTime,
		c.promQueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		log.Warnf("Failed to query GPU utilization for workload %s: %v", workloadUID, err)
		return nil
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		return nil
	}

	values := make([]float64, 0, len(series[0].Values))
	for _, point := range series[0].Values {
		values = append(values, point.Value)
	}

	return values
}
