package node

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
)

var (
	nodeGpuUtilHistoryMetricsQuery = map[metadata.GpuVendor]string{
		metadata.GpuVendorAMD: `max(gpu_utilization{primus_lens_node_name="%s"}) by (gpu_id)`,
	}
	nodeGpuAllocationHistoryMetricsQuery = `max(node_k8s_gpu_allocation_rate{primus_lens_node_name="%s"})`
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

func GetRdmaThroughputHistory(ctx context.Context, clientSets *clientsets.StorageClientSet, vendor metadata.GpuVendor, nodeName string, start, end time.Time, step int) ([]model.MetricsSeries, error) {
	query := fmt.Sprintf(nodeGpuUtilHistoryMetricsQuery[vendor], nodeName)
	return prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__": {},
	})
}
