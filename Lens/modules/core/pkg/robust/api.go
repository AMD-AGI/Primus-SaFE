// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import (
	"context"
	"fmt"
	"net/url"
)

// Nodes

func (c *Client) ListNodes(ctx context.Context, status string, pageNum, pageSize int) (*NodeListResponse, error) {
	params := url.Values{}
	if status != "" {
		params.Set("status", status)
	}
	if pageNum > 0 {
		params.Set("page_num", fmt.Sprint(pageNum))
	}
	if pageSize > 0 {
		params.Set("page_size", fmt.Sprint(pageSize))
	}
	var resp NodeListResponse
	err := c.Get(ctx, "/api/v1/nodes", params, &resp)
	return &resp, err
}

func (c *Client) GetNode(ctx context.Context, name string) (*NodeDetailResponse, error) {
	var resp NodeDetailResponse
	err := c.Get(ctx, "/api/v1/nodes/"+name, nil, &resp)
	return &resp, err
}

func (c *Client) GetNodeUtilization(ctx context.Context, name string) (*NodeUtilResponse, error) {
	var resp NodeUtilResponse
	err := c.Get(ctx, "/api/v1/nodes/"+name+"/utilization", nil, &resp)
	return &resp, err
}

func (c *Client) GetNodeWorkloads(ctx context.Context, name string) (*NodeWorkloadsResponse, error) {
	var resp NodeWorkloadsResponse
	err := c.Get(ctx, "/api/v1/nodes/"+name+"/workloads", nil, &resp)
	return &resp, err
}

func (c *Client) GetNodeDevices(ctx context.Context, name string) (*NodeDevicesResponse, error) {
	var resp NodeDevicesResponse
	err := c.Get(ctx, "/api/v1/nodes/"+name+"/devices", nil, &resp)
	return &resp, err
}

func (c *Client) GetGPUAllocation(ctx context.Context) (*NodeListResponse, error) {
	var resp NodeListResponse
	err := c.Get(ctx, "/api/v1/nodes/gpu-allocation", nil, &resp)
	return &resp, err
}

func (c *Client) GetNodeUtilizationHistory(ctx context.Context, name, start, end, step string) (map[string]interface{}, error) {
	params := url.Values{}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if step != "" {
		params.Set("step", step)
	}
	var resp map[string]interface{}
	err := c.Get(ctx, "/api/v1/nodes/"+name+"/utilizationHistory", params, &resp)
	return resp, err
}

func (c *Client) GetNodeWorkloadsHistory(ctx context.Context, name string, limit int) (*NodeWorkloadsResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprint(limit))
	}
	var resp NodeWorkloadsResponse
	err := c.Get(ctx, "/api/v1/nodes/"+name+"/workloadsHistory", params, &resp)
	return &resp, err
}

// Workloads

func (c *Client) ListWorkloads(ctx context.Context, state, workspace string, limit int) ([]WorkloadBrief, error) {
	params := url.Values{}
	if state != "" {
		params.Set("state", state)
	}
	if workspace != "" {
		params.Set("workspace", workspace)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprint(limit))
	}
	var resp []WorkloadBrief
	err := c.Get(ctx, "/api/v1/workloads", params, &resp)
	return resp, err
}

func (c *Client) GetWorkload(ctx context.Context, uid string) (*WorkloadDetailResponse, error) {
	var resp WorkloadDetailResponse
	err := c.Get(ctx, "/api/v1/workloads/"+uid, nil, &resp)
	return &resp, err
}

func (c *Client) GetWorkloadHierarchy(ctx context.Context, uid string) (*WorkloadHierarchyResponse, error) {
	var resp WorkloadHierarchyResponse
	err := c.Get(ctx, "/api/v1/workloads/"+uid+"/hierarchy", nil, &resp)
	return &resp, err
}

func (c *Client) GetWorkloadStatistic(ctx context.Context) (*ClusterStatisticResponse, error) {
	var resp ClusterStatisticResponse
	err := c.Get(ctx, "/api/v1/workloads/statistic", nil, &resp)
	return &resp, err
}

func (c *Client) GetWorkloadMetadata(ctx context.Context) (*WorkloadMetadataResponse, error) {
	var resp WorkloadMetadataResponse
	err := c.Get(ctx, "/api/v1/workloadMetadata", nil, &resp)
	return &resp, err
}

func (c *Client) GetWorkloadGPUMetrics(ctx context.Context, uid, start, end, step string) (map[string]interface{}, error) {
	params := url.Values{}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if step != "" {
		params.Set("step", step)
	}
	var resp map[string]interface{}
	err := c.Get(ctx, "/api/v1/workloads/"+uid+"/gpu-metrics", params, &resp)
	return resp, err
}

// Pods

func (c *Client) ListPods(ctx context.Context, namespace, podName string, pageNum, pageSize int) (*PodStatsResponse, error) {
	params := url.Values{}
	if namespace != "" {
		params.Set("namespace", namespace)
	}
	if podName != "" {
		params.Set("pod_name", podName)
	}
	if pageNum > 0 {
		params.Set("page_num", fmt.Sprint(pageNum))
	}
	if pageSize > 0 {
		params.Set("page_size", fmt.Sprint(pageSize))
	}
	var resp PodStatsResponse
	err := c.Get(ctx, "/api/v1/pods/stats", params, &resp)
	return &resp, err
}

func (c *Client) GetPod(ctx context.Context, podUID string) (*PodDetail, error) {
	var resp PodDetail
	err := c.Get(ctx, "/api/v1/pods/"+podUID, nil, &resp)
	return &resp, err
}

func (c *Client) GetPodGPUHistory(ctx context.Context, podUID, step, duration string) (map[string]interface{}, error) {
	params := url.Values{}
	if step != "" {
		params.Set("step", step)
	}
	if duration != "" {
		params.Set("duration", duration)
	}
	var resp map[string]interface{}
	err := c.Get(ctx, "/api/v1/pods/"+podUID+"/gpu-history", params, &resp)
	return resp, err
}

// Alerts

func (c *Client) ListAlerts(ctx context.Context, status, severity, nodeName, workloadID string, limit int) (*AlertListResponse, error) {
	params := url.Values{}
	if status != "" {
		params.Set("status", status)
	}
	if severity != "" {
		params.Set("severity", severity)
	}
	if nodeName != "" {
		params.Set("node_name", nodeName)
	}
	if workloadID != "" {
		params.Set("workload_id", workloadID)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprint(limit))
	}
	var resp AlertListResponse
	err := c.Get(ctx, "/api/v1/alerts", params, &resp)
	return &resp, err
}

func (c *Client) GetAlertSummary(ctx context.Context) (*AlertSummaryResponse, error) {
	var resp AlertSummaryResponse
	err := c.Get(ctx, "/api/v1/alerts/summary", nil, &resp)
	return &resp, err
}

// Training

func (c *Client) GetTrainingProgress(ctx context.Context, workloadID string) (*TrainingProgressResponse, error) {
	var resp TrainingProgressResponse
	err := c.Get(ctx, "/api/v1/training/"+workloadID, nil, &resp)
	return &resp, err
}

func (c *Client) GetTrainingMetricsData(ctx context.Context, workloadUID string, metrics []string, start, end, step string) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("workload_uid", workloadUID)
	for _, m := range metrics {
		params.Add("metrics", m)
	}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if step != "" {
		params.Set("step", step)
	}
	var resp map[string]interface{}
	err := c.Get(ctx, "/api/v1/training-metrics/data", params, &resp)
	return resp, err
}

// GPU Aggregation

func (c *Client) GetGPUAggregation(ctx context.Context, dimension, value string, hours, pageNum, pageSize int) (*GPUAggregationResponse, error) {
	params := url.Values{}
	if value != "" {
		params.Set("value", value)
	}
	if hours > 0 {
		params.Set("hours", fmt.Sprint(hours))
	}
	if pageNum > 0 {
		params.Set("page_num", fmt.Sprint(pageNum))
	}
	if pageSize > 0 {
		params.Set("page_size", fmt.Sprint(pageSize))
	}
	var resp GPUAggregationResponse
	err := c.Get(ctx, "/api/v1/gpu-aggregation/"+dimension, params, &resp)
	return &resp, err
}

// Diagnostics

func (c *Client) GetDiagProfile(ctx context.Context, uid string) (*DiagProfileResponse, error) {
	var resp DiagProfileResponse
	err := c.Get(ctx, "/api/v1/workload-diag/"+uid+"/profile", nil, &resp)
	return &resp, err
}

func (c *Client) GetRealtimeStatus(ctx context.Context) (*RealtimeStatusResponse, error) {
	var resp RealtimeStatusResponse
	err := c.Get(ctx, "/api/v1/realtime/status", nil, &resp)
	return &resp, err
}

// Profiler files

func (c *Client) ListProfilerFiles(ctx context.Context, workloadID string) (*ProfilerFileListResponse, error) {
	params := url.Values{}
	if workloadID != "" {
		params.Set("workload_id", workloadID)
	}
	var resp ProfilerFileListResponse
	err := c.Get(ctx, "/api/v1/profiler/files", params, &resp)
	return &resp, err
}

func (c *Client) GetProfilerFileContent(ctx context.Context, fileID string) (*StreamResponse, error) {
	body, contentLength, contentType, err := c.GetStream(ctx, "/api/v1/profiler/files/"+fileID+"/content", nil)
	if err != nil {
		return nil, err
	}
	return &StreamResponse{
		Body:          body,
		ContentLength: contentLength,
		ContentType:   contentType,
	}, nil
}

type StreamResponse struct {
	Body          interface{ Read([]byte) (int, error); Close() error }
	ContentLength int64
	ContentType   string
}
