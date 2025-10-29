package gpu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promModel "github.com/prometheus/common/model"
)

func GetHistoryGpuUsage(ctx context.Context, clientSets *clientsets.StorageClientSet, vendor metadata.GpuVendor, start, end time.Time, step int) ([]model.TimePoint, error) {
	query := "avg(gpu_utilization)"
	ts, err := prom.QueryRange(ctx, clientSets, query, start, end, step, map[string]struct{}{
		"__name__": {},
	})
	if err != nil {
		return nil, err
	}
	if len(ts) == 0 {
		return nil, err
	}
	return ts[0].Values, nil
}

func CalculateGpuUsage(ctx context.Context, clientsets *clientsets.StorageClientSet, vendor metadata.GpuVendor) (float64, error) {
	promClient := clientsets.PrometheusRead
	if promClient == nil {
		return 0.0, fmt.Errorf("Prometheus client is not initialized")
	}

	promAPI := v1.NewAPI(promClient)

	type metricResult struct {
		Metric string  `json:"metric"`
		Value  float64 `json:"value"`
	}

	metrics := []string{"gpu_utilization"}
	results := make([]metricResult, 0, len(metrics))

	for _, metric := range metrics {
		value, err := queryPrometheusInstant(ctx, promAPI, metric)
		if err != nil {
			return 0.0, fmt.Errorf("failed to query %s: %w", metric, err)
		}
		results = append(results, metricResult{Metric: metric, Value: value})
	}

	return results[0].Value, nil
}

func CalculateNodeGpuUsage(ctx context.Context, nodeName string, clientsets *clientsets.StorageClientSet, vendor metadata.GpuVendor) (float64, error) {
	promClient := clientsets.PrometheusRead
	if promClient == nil {
		return 0.0, fmt.Errorf("Prometheus client is not initialized")
	}
	promAPI := v1.NewAPI(promClient)
	query := fmt.Sprintf("avg(gpu_utilization{primus_lens_node_name=\"%s\"})", nodeName)
	now := time.Now()
	result, warnings, err := promAPI.Query(ctx, query, now)
	if err != nil {
		return 0, err
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings: %v\n", warnings)
	}
	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Warnf("No data returned for metric %s", query)
		return 0, nil
	}

	return float64(vectorVal[0].Value), nil
}

func queryPrometheusInstant(ctx context.Context, promAPI v1.API, metric string) (float64, error) {
	query := fmt.Sprintf("avg(%s)", metric)
	now := time.Now()

	result, warnings, err := promAPI.Query(ctx, query, now)
	if err != nil {
		return 0, err
	}
	if len(warnings) > 0 {
		log.Warnf("Prometheus query warnings for %s: %v\n", metric, warnings)
	}

	vectorVal, ok := result.(promModel.Vector)
	if !ok || len(vectorVal) == 0 {
		log.Warnf("No data returned for metric %s", metric)
		return 0, nil
	}

	return float64(vectorVal[0].Value), nil
}

func TopKGpuUtilizationInstant(ctx context.Context, k int, clientSets *clientsets.StorageClientSet) ([]model.ClusterOverviewHeatmapItem, error) {
	return getTopKInstant(ctx, k, "gpu_utilization", clientSets)
}

func TopKGpuTemperatureInstant(ctx context.Context, k int, clientSets *clientsets.StorageClientSet) ([]model.ClusterOverviewHeatmapItem, error) {
	return getTopKInstant(ctx, k, "gpu_junction_temperature", clientSets)
}

func TopKGpuPowerInstant(ctx context.Context, k int, clientSets *clientsets.StorageClientSet) ([]model.ClusterOverviewHeatmapItem, error) {
	return getTopKInstant(ctx, k, "gpu_power_usage", clientSets)
}

func getTopKInstant(ctx context.Context, k int, metric string, clientSets *clientsets.StorageClientSet) ([]model.ClusterOverviewHeatmapItem, error) {
	topKNodes, err := prom.QueryInstant(ctx, clientSets, fmt.Sprintf("topk(%d, avg by (primus_lens_node_name) (%s))", k, metric))
	if err != nil {
		return nil, err
	}
	if len(topKNodes) == 0 {
		return []model.ClusterOverviewHeatmapItem{}, nil
	}
	nodeName := []string{}
	for _, node := range topKNodes {
		nodeName = append(nodeName, string(node.Metric[promModel.LabelName("primus_lens_node_name")]))
	}
	topKNodesValue, err := prom.QueryInstant(ctx, clientSets, fmt.Sprintf(`%s{primus_lens_node_name=~"%s"}`, metric, strings.Join(nodeName, "|")))
	if err != nil {
		return nil, err
	}
	if len(topKNodesValue) == 0 {
		return []model.ClusterOverviewHeatmapItem{}, nil
	}
	result := []model.ClusterOverviewHeatmapItem{}
	for _, node := range topKNodesValue {
		gpuIdStr := string(node.Metric[promModel.LabelName("gpu_id")])
		gpuId, _ := strconv.Atoi(gpuIdStr)

		result = append(result, model.ClusterOverviewHeatmapItem{
			NodeName: string(node.Metric[promModel.LabelName("primus_lens_node_name")]),
			GpuId:    gpuId,
			Value:    float64(node.Value),
		})
	}
	return result, nil
}
