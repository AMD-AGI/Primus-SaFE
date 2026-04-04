// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// --- Node APIs ---

func (c *Client) GetNodes(ctx context.Context, status, workspace string, page, pageSize int) (*NodeListResp, error) {
	p := url.Values{}
	if status != "" {
		p.Set("status", status)
	}
	if workspace != "" {
		p.Set("workspace", workspace)
	}
	if page > 0 {
		p.Set("page_num", strconv.Itoa(page))
	}
	if pageSize > 0 {
		p.Set("page_size", strconv.Itoa(pageSize))
	}
	var resp NodeListResp
	if err := c.Get(ctx, "/nodes", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetNodeDetail(ctx context.Context, nodeName string) (*NodeDetailResp, error) {
	var resp NodeDetailResp
	if err := c.Get(ctx, "/nodes/"+nodeName, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetNodeUtilization(ctx context.Context, nodeName string) (*NodeUtilizationResp, error) {
	var resp NodeUtilizationResp
	if err := c.Get(ctx, "/nodes/"+nodeName+"/utilization", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetNodeWorkloads(ctx context.Context, nodeName string) (*NodeWorkloadsResp, error) {
	var resp NodeWorkloadsResp
	if err := c.Get(ctx, "/nodes/"+nodeName+"/workloads", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetNodeGPUMetrics(ctx context.Context, nodeName string) (*NodeGPUMetricsResp, error) {
	var resp NodeGPUMetricsResp
	if err := c.Get(ctx, "/nodes/"+nodeName+"/gpu-metrics", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetNodeDevices(ctx context.Context, nodeName string) (*NodeDevicesResp, error) {
	var resp NodeDevicesResp
	if err := c.Get(ctx, "/nodes/"+nodeName+"/devices", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Cluster APIs ---

func (c *Client) GetClusterRealtime(ctx context.Context) (*ClusterRealtimeResp, error) {
	var resp ClusterRealtimeResp
	if err := c.Get(ctx, "/cluster/realtime", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetClusterGPUUtilization(ctx context.Context) (*ClusterGPUUtilizationResp, error) {
	var resp ClusterGPUUtilizationResp
	if err := c.Get(ctx, "/cluster/gpu-utilization", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetClusterOverview(ctx context.Context) (*ClusterOverviewResp, error) {
	var resp ClusterOverviewResp
	if err := c.Get(ctx, "/cluster/overview", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetClusterGPUHeatmap(ctx context.Context) (*ClusterGPUHeatmapResp, error) {
	var resp ClusterGPUHeatmapResp
	if err := c.Get(ctx, "/cluster/gpu-heatmap", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetClusterGPUUtilizationHistory(ctx context.Context, rangeVal, step string) (json.RawMessage, error) {
	p := url.Values{}
	if rangeVal != "" {
		p.Set("range", rangeVal)
	}
	if step != "" {
		p.Set("step", step)
	}
	return c.GetRaw(ctx, "/cluster/gpu-utilization-history", p)
}

func (c *Client) GetNodeUtilizationHistory(ctx context.Context, nodeName, start, end, step string) (json.RawMessage, error) {
	p := url.Values{}
	if start != "" {
		p.Set("start", start)
	}
	if end != "" {
		p.Set("end", end)
	}
	if step != "" {
		p.Set("step", step)
	}
	return c.GetRaw(ctx, "/nodes/"+nodeName+"/utilizationHistory", p)
}

func (c *Client) GetNodeGPUMetricsHistory(ctx context.Context, nodeName, start, end, step string) (json.RawMessage, error) {
	p := url.Values{}
	if start != "" {
		p.Set("start", start)
	}
	if end != "" {
		p.Set("end", end)
	}
	if step != "" {
		p.Set("step", step)
	}
	return c.GetRaw(ctx, "/nodes/"+nodeName+"/gpu-metrics", p)
}

func (c *Client) GetWorkloadRanking(ctx context.Context, limit int, orderBy string) (json.RawMessage, error) {
	p := url.Values{}
	if limit > 0 {
		p.Set("limit", strconv.Itoa(limit))
	}
	if orderBy != "" {
		p.Set("order_by", orderBy)
	}
	return c.GetRaw(ctx, "/workloads/ranking", p)
}

func (c *Client) GetGpuAggDimensionKeys(ctx context.Context) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/gpu-aggregation/dimension-keys", nil)
}

func (c *Client) GetGpuAggDimensionValues(ctx context.Context, dimType, startTime, endTime string) (json.RawMessage, error) {
	p := url.Values{"dimension_type": {dimType}}
	if startTime != "" {
		p.Set("start_time", startTime)
	}
	if endTime != "" {
		p.Set("end_time", endTime)
	}
	return c.GetRaw(ctx, "/gpu-aggregation/dimension-values", p)
}

// --- Workload APIs ---

func (c *Client) GetWorkloadProfile(ctx context.Context, uid string) (*WorkloadProfileResp, error) {
	var resp WorkloadProfileResp
	if err := c.Get(ctx, "/workload-diag/"+uid+"/profile", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Pod APIs ---

func (c *Client) GetPodDetail(ctx context.Context, podUID string) (*PodDetailResp, error) {
	var resp PodDetailResp
	if err := c.Get(ctx, "/pods/"+podUID, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetPodStats(ctx context.Context, namespace, podName string, pageNum, pageSize int) (*PodStatsResp, error) {
	p := url.Values{}
	if namespace != "" {
		p.Set("namespace", namespace)
	}
	if podName != "" {
		p.Set("pod_name", podName)
	}
	if pageNum > 0 {
		p.Set("page_num", strconv.Itoa(pageNum))
	}
	if pageSize > 0 {
		p.Set("page_size", strconv.Itoa(pageSize))
	}
	var resp PodStatsResp
	if err := c.Get(ctx, "/pods/stats", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Alert APIs ---

func (c *Client) GetAlerts(ctx context.Context, status, severity, nodeName, workloadID string, limit int) (*AlertListResp, error) {
	p := url.Values{}
	if status != "" {
		p.Set("status", status)
	}
	if severity != "" {
		p.Set("severity", severity)
	}
	if nodeName != "" {
		p.Set("node_name", nodeName)
	}
	if workloadID != "" {
		p.Set("workload_id", workloadID)
	}
	if limit > 0 {
		p.Set("limit", strconv.Itoa(limit))
	}
	var resp AlertListResp
	if err := c.Get(ctx, "/alerts", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetAlertSummary(ctx context.Context) (*AlertSummaryResp, error) {
	var resp AlertSummaryResp
	if err := c.Get(ctx, "/alerts/summary", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Training Metrics APIs ---

func (c *Client) GetTrainingMetricsList(ctx context.Context, category string) (*TrainingMetricsListResp, error) {
	p := url.Values{}
	if category != "" {
		p.Set("category", category)
	}
	var resp TrainingMetricsListResp
	if err := c.Get(ctx, "/training-metrics", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetTrainingMetricsData(ctx context.Context, workloadUID, metrics, start, end, step, nodes string) (*TrainingMetricsDataResp, error) {
	p := url.Values{
		"workload_uid": {workloadUID},
		"metrics":      {metrics},
		"start":        {start},
		"end":          {end},
	}
	if step != "" {
		p.Set("step", step)
	}
	if nodes != "" {
		p.Set("nodes", nodes)
	}
	var resp TrainingMetricsDataResp
	if err := c.Get(ctx, "/training-metrics/data", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- GPU Aggregation APIs ---

func (c *Client) GetGpuAggClusters(ctx context.Context) (*GpuAggClustersResp, error) {
	var resp GpuAggClustersResp
	if err := c.Get(ctx, "/gpu-aggregation/clusters", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetGpuAggNamespaces(ctx context.Context, startTime, endTime string) (*GpuAggNamespacesResp, error) {
	p := url.Values{}
	if startTime != "" {
		p.Set("start_time", startTime)
	}
	if endTime != "" {
		p.Set("end_time", endTime)
	}
	var resp GpuAggNamespacesResp
	if err := c.Get(ctx, "/gpu-aggregation/namespaces", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetGpuAggHourlyStats(ctx context.Context, dimension, startTime, endTime string, page, pageSize int, orderBy, orderDir string) (*GpuAggPaginatedResp, error) {
	p := url.Values{
		"start_time": {startTime},
		"end_time":   {endTime},
	}
	if page > 0 {
		p.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		p.Set("page_size", strconv.Itoa(pageSize))
	}
	if orderBy != "" {
		p.Set("order_by", orderBy)
	}
	if orderDir != "" {
		p.Set("order_direction", orderDir)
	}

	path := fmt.Sprintf("/gpu-aggregation/%s/hourly-stats", dimension)
	var resp GpuAggPaginatedResp
	if err := c.Get(ctx, path, p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetGpuAggSnapshotLatest(ctx context.Context) (*GpuAggSnapshotResp, error) {
	var resp GpuAggSnapshotResp
	if err := c.Get(ctx, "/gpu-aggregation/snapshots/latest", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
