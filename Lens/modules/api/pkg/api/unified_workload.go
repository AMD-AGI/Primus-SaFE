// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for workload operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/robust"
)

// robustWorkloadItem mirrors the Robust API workload list response fields.
type robustWorkloadItem struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	GPUs      int               `json:"gpus"`
	Nodes     string            `json:"nodes"`
	StartTime string            `json:"start_time"`
	State     string            `json:"state"`
	User      string            `json:"user"`
	Labels    map[string]string `json:"labels"`
}

type robustWorkloadListResp struct {
	Data []robustWorkloadItem `json:"data"`
}

func convertRobustWorkloadItem(r robustWorkloadItem) model.WorkloadListItem {
	item := model.WorkloadListItem{
		Kind:         r.Kind,
		Name:         r.Name,
		Namespace:    r.Namespace,
		Uid:          r.ID,
		GpuAllocated: r.GPUs,
		Source:       "k8s",
	}

	switch strings.ToUpper(r.State) {
	case "RUNNING":
		item.Status = "Running"
		item.StatusColor = "green"
	case "COMPLETED":
		item.Status = "Done"
		item.StatusColor = "blue"
	case "FAILED":
		item.Status = "Failed"
		item.StatusColor = "red"
	case "PENDING":
		item.Status = "Pending"
		item.StatusColor = "yellow"
	default:
		item.Status = r.State
		item.StatusColor = "gray"
	}

	if r.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, r.StartTime); err == nil {
			item.StartAt = t.Unix()
		}
	}

	return item
}

// ===== Workload List =====

// WorkloadListRequest represents the request for workload list.
type WorkloadListRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name      string `json:"name" query:"name" mcp:"name,description=Filter by workload name (partial match)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Filter by workload kind (Job/PyTorchJob/etc)"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Filter by namespace"`
	Status    string `json:"status" query:"status" mcp:"status,description=Filter by status (Running/Completed/Failed)"`
	PageNum   int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize  int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
	OrderBy   string `json:"order_by" query:"order_by" mcp:"order_by,description=Order by field (start_at/end_at)"`
}

// WorkloadListResponse represents the workload list response.
type WorkloadListResponse struct {
	Data  []model.WorkloadListItem `json:"data"`
	Total int                      `json:"total"`
}

// ===== Workload Detail =====

// WorkloadDetailRequest represents the request for workload detail.
type WorkloadDetailRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
}

// WorkloadDetailResponse is model.WorkloadInfo for backward compatibility.
type WorkloadDetailResponse = model.WorkloadInfo

// ===== Phase 5: Workload Statistics =====

// WorkloadStatisticsRequest represents the request for workload statistics.
type WorkloadStatisticsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// WorkloadStatisticsResponse is model.WorkloadStatisticResp for backward compatibility.
type WorkloadStatisticsResponse = model.WorkloadStatisticResp

// ===== Phase 5: Workload Hierarchy Query =====

// WorkloadHierarchyQueryRequest represents the request for workload hierarchy by kind/name.
type WorkloadHierarchyQueryRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Workload kind (e.g. Job/PyTorchJob),required"`
	Name      string `json:"name" query:"name" mcp:"name,description=Workload name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Workload namespace (optional)"`
}

// WorkloadHierarchyResponse is model.WorkloadHierarchyItem for backward compatibility.
type WorkloadHierarchyResponse = model.WorkloadHierarchyItem

// ===== Phase 5: Workload GPU Utilization History =====

// WorkloadGPUHistoryRequest represents the request for workload GPU utilization history.
type WorkloadGPUHistoryRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Workload kind,required"`
	Name      string `json:"name" query:"name" mcp:"name,description=Workload name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Workload namespace (optional)"`
	Start     string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End       string `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step      string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// WorkloadGPUHistoryResponse is the utilization history data.
type WorkloadGPUHistoryResponse = model.MetricsGraph

// ===== Phase 6: Workload Hierarchy by UID =====

// WorkloadHierarchyByUIDRequest represents the request for workload hierarchy by UID.
type WorkloadHierarchyByUIDRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
}

// ===== Phase 6: Workload Metrics =====

// WorkloadMetricsRequest represents the request for workload metrics (GPU util, memory, power).
type WorkloadMetricsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	Start   string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End     string `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step    string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// WorkloadMetricsResponse represents the workload metrics response.
type WorkloadMetricsResponse map[string]model.MetricsGraph

// ===== Phase 6: Training Performance =====

// TrainingPerformanceRequest represents the request for training performance metrics.
type TrainingPerformanceRequest struct {
	UID   string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	Start string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix milliseconds),required"`
	End   string `json:"end" query:"end" mcp:"end,description=End timestamp (unix milliseconds),required"`
}

// TrainingPerformanceResponse is a list of training performance data points.
type TrainingPerformanceResponse []GrafanaMetricsPoint

// ===== Phase 6: Data Sources =====

// WorkloadDataSourcesRequest represents the request for workload data sources.
type WorkloadDataSourcesRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
}

// WorkloadDataSourcesResponse represents data sources available for a workload.
type WorkloadDataSourcesResponse = DataSourcesResponse

// ===== Phase 6: Available Metrics =====

// WorkloadAvailableMetricsRequest represents the request for available metrics.
type WorkloadAvailableMetricsRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID        string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter by data source (log/wandb/tensorflow)"`
}

// WorkloadAvailableMetricsResponse represents available metrics for a workload.
type WorkloadAvailableMetricsResponse = AvailableMetricsResponse

// ===== Phase 6: Metrics Data =====

// WorkloadMetricsDataRequest represents the request for metrics data.
type WorkloadMetricsDataRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID        string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter by data source (log/wandb/tensorflow)"`
	Metrics    string `json:"metrics" query:"metrics" mcp:"metrics,description=Comma-separated metric names or 'all' (default: all)"`
	Start      string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix milliseconds)"`
	End        string `json:"end" query:"end" mcp:"end,description=End timestamp (unix milliseconds)"`
}

// WorkloadMetricsDataResponse represents metrics data for a workload.
type WorkloadMetricsDataResponse = MetricsDataResponse

// ===== Phase 6: Iteration Times =====

// WorkloadIterationTimesRequest represents the request for iteration times.
type WorkloadIterationTimesRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID        string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter by data source (log/wandb/tensorflow)"`
	Start      string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix milliseconds)"`
	End        string `json:"end" query:"end" mcp:"end,description=End timestamp (unix milliseconds)"`
}

// WorkloadIterationTimesResponse represents iteration time data.
type WorkloadIterationTimesResponse = MetricsDataResponse

// ===== Phase 6: Workload Metadata =====

// WorkloadMetadataRequest represents the request for workload metadata.
type WorkloadMetadataRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// WorkloadMetadataResponse represents workload metadata (namespaces and kinds).
type WorkloadMetadataResponse struct {
	Namespaces []string `json:"namespaces"`
	Kinds      []string `json:"kinds"`
}

// ===== Register Workload Endpoints =====

func init() {
	// Register workload list endpoint - replaces listWorkloads
	unified.Register(&unified.EndpointDef[WorkloadListRequest, WorkloadListResponse]{
		Name:        "workload_list",
		Description: "List GPU workloads in the cluster with filtering by name, kind, namespace, status. Returns workload metadata, GPU allocation, and utilization statistics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads",
		MCPToolName: "lens_workload_list",
		Handler:     handleWorkloadList,
	})

	// Register workload detail endpoint - replaces getWorkloadInfo
	unified.Register(&unified.EndpointDef[WorkloadDetailRequest, WorkloadDetailResponse]{
		Name:        "workload_detail",
		Description: "Get detailed information about a specific workload by UID including pods, GPU allocation per node, start/end times, and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid",
		MCPToolName: "lens_workload_detail",
		Handler:     handleWorkloadDetail,
	})

	// Phase 5: Register workload statistics endpoint - replaces getWorkloadsStatistic
	unified.Register(&unified.EndpointDef[WorkloadStatisticsRequest, WorkloadStatisticsResponse]{
		Name:        "workload_statistics",
		Description: "Get statistics about running workloads including count, average GPU allocation, average GPU utilization, and number of low utilization workloads.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/statistic",
		MCPToolName: "lens_workload_statistics",
		Handler:     handleWorkloadStatistics,
	})

	// Phase 5: Register workload hierarchy query endpoint - replaces getWorkloadHierarchyByKindName
	unified.Register(&unified.EndpointDef[WorkloadHierarchyQueryRequest, WorkloadHierarchyResponse]{
		Name:        "workload_hierarchy_query",
		Description: "Get the workload hierarchy tree by kind and name. Shows parent-child relationships between workload resources.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/hierarchy",
		MCPToolName: "lens_workload_hierarchy_query",
		Handler:     handleWorkloadHierarchyQuery,
	})

	// Phase 5: Register workload GPU history endpoint - replaces getWorkloadGpuUtilizationHistoryByKindName
	unified.Register(&unified.EndpointDef[WorkloadGPUHistoryRequest, WorkloadGPUHistoryResponse]{
		Name:        "workload_gpu_history",
		Description: "Get GPU utilization history for a workload by kind and name. Returns time series data for GPU utilization over the specified time range.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/gpuUtilizationHistory",
		MCPToolName: "lens_workload_gpu_history",
		Handler:     handleWorkloadGPUHistory,
	})

	// Phase 6: Register workload hierarchy by UID endpoint - replaces getWorkloadHierarchy
	unified.Register(&unified.EndpointDef[WorkloadHierarchyByUIDRequest, WorkloadHierarchyResponse]{
		Name:        "workload_hierarchy",
		Description: "Get the workload hierarchy tree by UID. Shows parent-child relationships between workload resources.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/hierarchy",
		MCPToolName: "lens_workload_hierarchy",
		Handler:     handleWorkloadHierarchyByUID,
	})

	// Phase 6: Register workload metrics endpoint - replaces getWorkloadMetrics
	unified.Register(&unified.EndpointDef[WorkloadMetricsRequest, WorkloadMetricsResponse]{
		Name:        "workload_metrics",
		Description: "Get GPU utilization, memory utilization, power metrics, and training performance for a workload over time.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics",
		MCPToolName: "lens_workload_metrics",
		Handler:     handleWorkloadMetrics,
	})

	// Phase 6: Register training performance endpoint - replaces GetWorkloadTrainingPerformance
	unified.Register(&unified.EndpointDef[TrainingPerformanceRequest, TrainingPerformanceResponse]{
		Name:        "training_performance",
		Description: "Get training performance metrics (TFLOPS, loss, etc.) for a workload. Returns time series data suitable for Grafana visualization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/trainingPerformance",
		MCPToolName: "lens_training_performance",
		Handler:     handleTrainingPerformance,
	})

	// Phase 6: Register data sources endpoint - replaces GetDataSources
	unified.Register(&unified.EndpointDef[WorkloadDataSourcesRequest, WorkloadDataSourcesResponse]{
		Name:        "workload_data_sources",
		Description: "Get all available data sources (log, wandb, tensorflow) for a workload's training metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/sources",
		MCPToolName: "lens_workload_data_sources",
		Handler:     handleWorkloadDataSources,
	})

	// Phase 6: Register available metrics endpoint - replaces GetAvailableMetrics
	unified.Register(&unified.EndpointDef[WorkloadAvailableMetricsRequest, WorkloadAvailableMetricsResponse]{
		Name:        "workload_available_metrics",
		Description: "Get all available training metrics for a workload, optionally filtered by data source.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/available",
		MCPToolName: "lens_workload_available_metrics",
		Handler:     handleWorkloadAvailableMetrics,
	})

	// Phase 6: Register metrics data endpoint - replaces GetMetricsData
	unified.Register(&unified.EndpointDef[WorkloadMetricsDataRequest, WorkloadMetricsDataResponse]{
		Name:        "workload_metrics_data",
		Description: "Get training metrics data for a workload with optional filtering by data source, metrics, and time range.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/data",
		MCPToolName: "lens_workload_metrics_data",
		Handler:     handleWorkloadMetricsData,
	})

	// Phase 6: Register iteration times endpoint - replaces GetIterationTimes
	unified.Register(&unified.EndpointDef[WorkloadIterationTimesRequest, WorkloadIterationTimesResponse]{
		Name:        "workload_iteration_times",
		Description: "Get iteration/step timing data for a workload's training run.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/iteration-times",
		MCPToolName: "lens_workload_iteration_times",
		Handler:     handleWorkloadIterationTimes,
	})

	// Phase 6: Register workload metadata endpoint - replaces getWorkloadsMetadata
	unified.Register(&unified.EndpointDef[WorkloadMetadataRequest, WorkloadMetadataResponse]{
		Name:        "workload_metadata",
		Description: "Get workload metadata including available namespaces and workload kinds in the cluster.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloadMetadata",
		MCPToolName: "lens_workload_metadata",
		Handler:     handleWorkloadMetadata,
	})
}

// ===== Handler Implementations =====

func handleWorkloadList(ctx context.Context, req *WorkloadListRequest) (*WorkloadListResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	p := url.Values{}
	p.Set("page_num", strconv.Itoa(pageNum))
	p.Set("page_size", strconv.Itoa(pageSize))
	if req.Name != "" {
		p.Set("name", req.Name)
	}
	if req.Kind != "" {
		p.Set("kind", req.Kind)
	}
	if req.Namespace != "" {
		p.Set("namespace", req.Namespace)
	}
	if req.Status != "" {
		p.Set("status", req.Status)
	}
	if req.OrderBy != "" {
		orderBy := req.OrderBy
		switch orderBy {
		case "start_at":
			orderBy = "created_at"
		case "end_at":
			// pass through for Robust compatibility
		}
		p.Set("order_by", orderBy)
	}

	raw, err := rc.GetRaw(ctx, "/workloads", p)
	if err != nil {
		return nil, fmt.Errorf("robust workloads list: %w", err)
	}

	var resp robustWorkloadListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode workloads list: %w", err)
	}

	items := make([]model.WorkloadListItem, 0, len(resp.Data))
	for _, r := range resp.Data {
		items = append(items, convertRobustWorkloadItem(r))
	}
	return &WorkloadListResponse{
		Data:  items,
		Total: len(items),
	}, nil
}

type robustWorkloadDetail struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Kind         string            `json:"kind"`
	InfraType    string            `json:"infra_type"`
	User         string            `json:"user"`
	State        string            `json:"state"`
	Nodes        string            `json:"nodes"`
	GPUs         int               `json:"gpus"`
	StartTime    string            `json:"start_time"`
	DurationSec  int64             `json:"duration_sec"`
	Labels       map[string]string `json:"labels"`
}

type robustWorkloadDetailResp struct {
	Data robustWorkloadDetail `json:"data"`
}

func handleWorkloadDetail(ctx context.Context, req *WorkloadDetailRequest) (*WorkloadDetailResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}

	path := "/workloads/" + url.PathEscape(req.UID)
	raw, err := rc.GetRaw(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("robust workload detail: %w", err)
	}

	var resp robustWorkloadDetailResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode workload detail: %w", err)
	}

	r := resp.Data
	if r.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).
			WithMessage(fmt.Sprintf("workload %s not found", req.UID))
	}

	out := &model.WorkloadInfo{
		Kind:      r.Kind,
		Name:      r.Name,
		Namespace: r.Namespace,
		Uid:       r.ID,
		Source:    "k8s",
		GpuAllocation: model.GpuAllocationInfo{
			"": float64(r.GPUs),
		},
	}
	if r.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, r.StartTime); err == nil {
			out.StartTime = t.Unix()
			if r.DurationSec > 0 {
				out.EndTime = t.Unix() + r.DurationSec
			}
		}
	}
	return out, nil
}

func handleWorkloadStatistics(ctx context.Context, req *WorkloadStatisticsRequest) (*WorkloadStatisticsResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/workloads/statistic", nil)
	if err != nil {
		return nil, fmt.Errorf("robust workloads statistic: %w", err)
	}

	var resp struct {
		Data model.WorkloadStatisticResp `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode workload statistic: %w", err)
	}
	return &resp.Data, nil
}

func firstWorkloadUIDFromRobustList(ctx context.Context, rc *robust.Client, kind, name, namespace string) (string, error) {
	if kind == "" || name == "" {
		return "", errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("kind and name are required")
	}
	p := url.Values{}
	p.Set("kind", kind)
	p.Set("name", name)
	if namespace != "" {
		p.Set("namespace", namespace)
	}
	p.Set("page_num", "1")
	p.Set("page_size", "1")

	raw, err := rc.GetRaw(ctx, "/workloads", p)
	if err != nil {
		return "", fmt.Errorf("robust workloads lookup: %w", err)
	}
	var resp robustWorkloadListResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("decode workloads lookup: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}
	return resp.Data[0].ID, nil
}

func handleWorkloadHierarchyQuery(ctx context.Context, req *WorkloadHierarchyQueryRequest) (*WorkloadHierarchyResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	uid, err := firstWorkloadUIDFromRobustList(ctx, rc, req.Kind, req.Name, req.Namespace)
	if err != nil {
		return nil, err
	}

	path := "/workloads/" + url.PathEscape(uid) + "/hierarchy"
	raw, err := rc.GetRaw(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("robust workload hierarchy: %w", err)
	}

	var out model.WorkloadHierarchyItem
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode workload hierarchy: %w", err)
	}
	return &out, nil
}

func handleWorkloadGPUHistory(ctx context.Context, req *WorkloadGPUHistoryRequest) (*WorkloadGPUHistoryResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	uid, err := firstWorkloadUIDFromRobustList(ctx, rc, req.Kind, req.Name, req.Namespace)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	p.Set("start", req.Start)
	p.Set("end", req.End)
	if req.Step != "" {
		p.Set("step", req.Step)
	}

	path := "/workloads/" + url.PathEscape(uid) + "/gpu-metrics"
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust workload gpu-metrics: %w", err)
	}

	var out model.MetricsGraph
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode gpu-metrics: %w", err)
	}
	return &out, nil
}

func handleWorkloadHierarchyByUID(ctx context.Context, req *WorkloadHierarchyByUIDRequest) (*WorkloadHierarchyResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}

	path := "/workloads/" + url.PathEscape(req.UID) + "/hierarchy"
	raw, err := rc.GetRaw(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("robust workload hierarchy: %w", err)
	}

	var out model.WorkloadHierarchyItem
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode workload hierarchy: %w", err)
	}
	return &out, nil
}

func handleWorkloadMetrics(ctx context.Context, req *WorkloadMetricsRequest) (*WorkloadMetricsResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}
	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	p := url.Values{}
	p.Set("start", req.Start)
	p.Set("end", req.End)
	if req.Step != "" {
		p.Set("step", req.Step)
	}

	path := "/workloads/" + url.PathEscape(req.UID) + "/gpu-metrics"
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust workload gpu-metrics: %w", err)
	}

	var out WorkloadMetricsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode workload metrics: %w", err)
	}
	return &out, nil
}

func handleTrainingPerformance(ctx context.Context, req *TrainingPerformanceRequest) (*TrainingPerformanceResponse, error) {
	rc, err := getRobustClient("")
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}
	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	p := url.Values{}
	p.Set("start", req.Start)
	p.Set("end", req.End)

	path := "/training/" + url.PathEscape(req.UID)
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust training performance: %w", err)
	}

	var out TrainingPerformanceResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode training performance: %w", err)
	}
	return &out, nil
}

func handleWorkloadDataSources(ctx context.Context, req *WorkloadDataSourcesRequest) (*WorkloadDataSourcesResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}

	path := "/training/" + url.PathEscape(req.UID) + "/sources"
	raw, err := rc.GetRaw(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("robust training sources: %w", err)
	}

	var out DataSourcesResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode data sources: %w", err)
	}
	return &out, nil
}

func handleWorkloadAvailableMetrics(ctx context.Context, req *WorkloadAvailableMetricsRequest) (*WorkloadAvailableMetricsResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}

	p := url.Values{}
	if req.DataSource != "" {
		p.Set("data_source", req.DataSource)
	}

	path := "/training/" + url.PathEscape(req.UID) + "/available"
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust training available metrics: %w", err)
	}

	var out AvailableMetricsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode available metrics: %w", err)
	}
	return &out, nil
}

func handleWorkloadMetricsData(ctx context.Context, req *WorkloadMetricsDataRequest) (*WorkloadMetricsDataResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}

	p := url.Values{}
	if req.DataSource != "" {
		p.Set("data_source", req.DataSource)
	}
	if req.Metrics != "" {
		p.Set("metrics", req.Metrics)
	}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}

	path := "/training/" + url.PathEscape(req.UID) + "/data"
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust training metrics data: %w", err)
	}

	var out MetricsDataResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode metrics data: %w", err)
	}
	return &out, nil
}

func handleWorkloadIterationTimes(ctx context.Context, req *WorkloadIterationTimesRequest) (*WorkloadIterationTimesResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	if req.UID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}

	p := url.Values{}
	if req.DataSource != "" {
		p.Set("data_source", req.DataSource)
	}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}

	path := "/training/" + url.PathEscape(req.UID) + "/iteration-times"
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust training iteration-times: %w", err)
	}

	var out MetricsDataResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode iteration times: %w", err)
	}
	return &out, nil
}

func handleWorkloadMetadata(ctx context.Context, req *WorkloadMetadataRequest) (*WorkloadMetadataResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/workloads/metadata", nil)
	if err != nil {
		return nil, fmt.Errorf("robust workload metadata: %w", err)
	}

	var out WorkloadMetadataResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode workload metadata: %w", err)
	}
	return &out, nil
}

// Helper functions

// splitMetricsForUnified splits a comma-separated metrics string and trims spaces.
func splitMetricsForUnified(s string) []string {
	parts := make([]string, 0)
	for _, p := range splitStringForUnified(s, ",") {
		trimmed := trimSpaceForUnified(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitStringForUnified(s string, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpaceForUnified(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func isNaN(f float64) bool {
	return f != f
}
