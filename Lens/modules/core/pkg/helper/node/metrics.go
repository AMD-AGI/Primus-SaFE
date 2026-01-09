// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

var (
	nodeGpuUtilHistoryMetricsQuery = map[metadata.GpuVendor]string{
		metadata.GpuVendorAMD: `max(gpu_utilization{primus_lens_node_name="%s"}) by (gpu_id)`,
	}
	nodeGpuAllocationHistoryMetricsQuery = `max(node_k8s_gpu_allocation_rate{primus_lens_node_name="%s"})`
	nodeCpuUtilHistoryMetricsQuery       = `100 - (avg(irate(node_cpu_seconds_total{mode="idle",primus_lens_node_name="%s"}[5m])) * 100)`
	nodeMemUtilHistoryMetricsQuery       = `(1 - (node_memory_MemAvailable_bytes{primus_lens_node_name="%s"} / node_memory_MemTotal_bytes{primus_lens_node_name="%s"})) * 100`
	rdmaThroughtputHistoryMetricsQuery   = map[metadata.GpuVendor]string{
		metadata.GpuVendorAMD: ``,
	}
)

func GetNodeGpuUtilHistory(ctx context.Context, clientSets *clientsets.StorageClientSet, vendor metadata.GpuVendor, nodeName string, start, end time.Time, step int) ([]model.MetricsSeries, error) {
	query := fmt.Sprintf(nodeGpuUtilHistoryMetricsQuery[vendor], nodeName)
	return prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"gpu_id":   {},
		"__name__": {},
	})
}

func GetNodeGpuAllocationHistory(ctx context.Context, clientSets *clientsets.StorageClientSet, vendor metadata.GpuVendor, nodeName string, start, end time.Time, step int) ([]model.MetricsSeries, error) {
	query := fmt.Sprintf(nodeGpuAllocationHistoryMetricsQuery, nodeName)
	return prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":              {},
		"primus_lens_node_name": {}})
}

func GetNodeCpuUtilHistory(ctx context.Context, clientSets *clientsets.StorageClientSet, nodeName string, start, end time.Time, step int) ([]model.MetricsSeries, error) {
	query := fmt.Sprintf(nodeCpuUtilHistoryMetricsQuery, nodeName)
	return prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":              {},
		"primus_lens_node_name": {},
	})
}

func GetNodeMemUtilHistory(ctx context.Context, clientSets *clientsets.StorageClientSet, nodeName string, start, end time.Time, step int) ([]model.MetricsSeries, error) {
	query := fmt.Sprintf(nodeMemUtilHistoryMetricsQuery, nodeName, nodeName)
	return prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__":              {},
		"primus_lens_node_name": {},
	})
}

func GetRdmaThroughputHistory(ctx context.Context, clientSets *clientsets.StorageClientSet, vendor metadata.GpuVendor, nodeName string, start, end time.Time, step int) ([]model.MetricsSeries, error) {
	query := fmt.Sprintf(nodeGpuUtilHistoryMetricsQuery[vendor], nodeName)
	return prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__": {},
	})
}
