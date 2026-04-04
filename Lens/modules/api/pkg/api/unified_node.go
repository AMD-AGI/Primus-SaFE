// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for node operations.
// All data-plane queries are delegated to the Robust API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/robust"
)

// ===== Node List =====

type NodeListRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name     string `json:"name" query:"name" mcp:"name,description=Filter by node name (partial match)"`
	GpuName  string `json:"gpu_name" query:"gpu_name" mcp:"gpu_name,description=Filter by GPU model name"`
	Status   string `json:"status" query:"status" mcp:"status,description=Filter by node status (comma-separated: ready,notready)"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
	OrderBy  string `json:"order_by" query:"order_by" mcp:"order_by,description=Field to order by"`
	Desc     bool   `json:"desc" query:"desc" mcp:"desc,description=Sort descending"`
}

type NodeListResponse struct {
	Data        []model.GPUNode `json:"data"`
	Total       int             `json:"total"`
	ClusterName string          `json:"cluster_name"`
}

// ===== GPU Allocation =====

type GPUAllocationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

type GPUAllocationResponse []model.GpuAllocation

// ===== Node Detail =====

type NodeDetailRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
}

type NodeDetailResponse struct {
	model.GpuNodeDetail
	ClusterName string `json:"cluster_name"`
}

// ===== Node GPU Devices =====

type NodeGPUDevicesRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
}

type NodeGPUDevicesResponse struct {
	NodeName    string                `json:"node_name"`
	ClusterName string                `json:"cluster_name"`
	Devices     []model.GpuDeviceInfo `json:"devices"`
}

// ===== GPU Utilization =====

type GPUUtilizationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

type GPUUtilizationResponse = model.GPUUtilization

// ===== GPU Utilization History =====

type GPUUtilizationHistoryRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Start   string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End     string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step    string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type GPUUtilizationHistoryResponse = model.GpuUtilizationHistory

// ===== Node Utilization =====

type NodeUtilizationRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
}

type NodeUtilizationResponse = model.NodeUtilization

// ===== Node Utilization History =====

type NodeUtilizationHistoryRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	Start    string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End      string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step     string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type NodeUtilizationHistoryResponse struct {
	NodeName       string                `json:"node_name"`
	CpuUtilization []model.MetricsSeries `json:"cpu_utilization"`
	MemUtilization []model.MetricsSeries `json:"mem_utilization"`
	GpuUtilization []model.MetricsSeries `json:"gpu_utilization"`
	GpuAllocation  []model.MetricsSeries `json:"gpu_allocation"`
}

// ===== Node GPU Metrics =====

type NodeGPUMetricsRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	Start    string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End      string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step     string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type NodeGPUMetricsResponse struct {
	GpuUtilization    model.MetricsGraph `json:"gpu_utilization"`
	GpuAllocationRate model.MetricsGraph `json:"gpu_allocation_rate"`
}

// ===== Node Workloads =====

type NodeWorkloadsRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
}

type NodeWorkloadsResponse struct {
	Data  []model.WorkloadNodeView `json:"data"`
	Total int                      `json:"total"`
}

// ===== Node Workloads History =====

type NodeWorkloadsHistoryRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
}

type NodeWorkloadsHistoryResponse struct {
	Data  []model.WorkloadHistoryNodeView `json:"data"`
	Total int                             `json:"total"`
}

// ===== Register Node Endpoints =====

func init() {
	unified.Register(&unified.EndpointDef[NodeListRequest, NodeListResponse]{
		Name:        "node_list",
		Description: "List GPU nodes in the cluster with filtering and pagination. Returns node name, IP, GPU model, GPU count, allocation, utilization and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes",
		MCPToolName: "lens_node_list",
		Handler:     handleNodeList,
	})

	unified.Register(&unified.EndpointDef[GPUAllocationRequest, GPUAllocationResponse]{
		Name:        "gpu_allocation",
		Description: "Get per-node GPU allocation showing capacity, allocated count, and allocation rate for each node.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/gpuAllocation",
		MCPToolName: "lens_gpu_allocation",
		Handler:     handleGPUAllocation,
	})

	unified.Register(&unified.EndpointDef[NodeDetailRequest, NodeDetailResponse]{
		Name:        "node_detail",
		Description: "Get detailed information about a specific GPU node including CPU, memory, OS, GPU driver version, kubelet version and health status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name",
		MCPToolName: "lens_node_detail",
		Handler:     handleNodeDetail,
	})

	unified.Register(&unified.EndpointDef[NodeGPUDevicesRequest, NodeGPUDevicesResponse]{
		Name:        "node_gpu_devices",
		Description: "Get GPU device information for a specific node including device ID, model, memory, utilization, temperature and power.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/gpuDevices",
		MCPToolName: "lens_node_gpu_devices",
		Handler:     handleNodeGPUDevices,
	})

	unified.Register(&unified.EndpointDef[GPUUtilizationRequest, GPUUtilizationResponse]{
		Name:        "gpu_utilization",
		Description: "Get current cluster GPU utilization metrics including allocation rate and average utilization percentage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/gpuUtilization",
		MCPToolName: "lens_gpu_utilization",
		Handler:     handleGPUUtilization,
	})

	unified.Register(&unified.EndpointDef[GPUUtilizationHistoryRequest, GPUUtilizationHistoryResponse]{
		Name:        "gpu_utilization_history",
		Description: "Get historical GPU utilization data over a time range. Returns allocation rate, utilization, and VRAM utilization as time series.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/gpuUtilizationHistory",
		MCPToolName: "lens_gpu_utilization_history",
		Handler:     handleGPUUtilizationHistory,
	})

	unified.Register(&unified.EndpointDef[NodeUtilizationRequest, NodeUtilizationResponse]{
		Name:        "node_utilization",
		Description: "Get current utilization metrics for a specific node including CPU, memory, and GPU utilization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/utilization",
		MCPToolName: "lens_node_utilization",
		Handler:     handleNodeUtilization,
	})

	unified.Register(&unified.EndpointDef[NodeUtilizationHistoryRequest, NodeUtilizationHistoryResponse]{
		Name:        "node_utilization_history",
		Description: "Get historical utilization metrics for a node over a time range including CPU, memory, GPU utilization and allocation.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/utilizationHistory",
		MCPToolName: "lens_node_utilization_history",
		Handler:     handleNodeUtilizationHistory,
	})

	unified.Register(&unified.EndpointDef[NodeGPUMetricsRequest, NodeGPUMetricsResponse]{
		Name:        "node_gpu_metrics",
		Description: "Get detailed GPU metrics for a node over a time range including utilization, allocation, VRAM, power and temperature.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/gpuMetrics",
		MCPToolName: "lens_node_gpu_metrics",
		Handler:     handleNodeGPUMetrics,
	})

	unified.Register(&unified.EndpointDef[NodeWorkloadsRequest, NodeWorkloadsResponse]{
		Name:        "node_workloads",
		Description: "Get currently running GPU workloads on a specific node.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/workloads",
		MCPToolName: "lens_node_workloads",
		Handler:     handleNodeWorkloads,
	})

	unified.Register(&unified.EndpointDef[NodeWorkloadsHistoryRequest, NodeWorkloadsHistoryResponse]{
		Name:        "node_workloads_history",
		Description: "Get historical GPU workloads that ran on a specific node.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/workloadsHistory",
		MCPToolName: "lens_node_workloads_history",
		Handler:     handleNodeWorkloadsHistory,
	})
}

// ===== Handler Implementations =====

func handleNodeList(ctx context.Context, req *NodeListRequest) (*NodeListResponse, error) {
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

	resp, err := rc.GetNodes(ctx, req.Status, "", pageNum, pageSize)
	if err != nil {
		return nil, fmt.Errorf("robust node list: %w", err)
	}

	nodes := make([]model.GPUNode, 0, len(resp.Nodes))
	for _, n := range resp.Nodes {
		nodes = append(nodes, model.GPUNode{
			Name:           n.NodeName,
			Status:         n.HealthStatus,
			GpuCount:       n.GPUCount,
			GpuAllocation:  n.GPUsAllocated,
			GpuUtilization: n.AllocationRate * 100,
		})
	}
	return &NodeListResponse{
		Data:        nodes,
		Total:       resp.TotalNodes,
		ClusterName: rc.ClusterName(),
	}, nil
}

func handleGPUAllocation(ctx context.Context, req *GPUAllocationRequest) (*GPUAllocationResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/nodes/gpu-allocation", nil)
	if err != nil {
		return nil, fmt.Errorf("robust gpu allocation: %w", err)
	}

	var result GPUAllocationResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode gpu allocation: %w", err)
	}
	return &result, nil
}

func handleNodeDetail(ctx context.Context, req *NodeDetailRequest) (*NodeDetailResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetNodeDetail(ctx, req.NodeName)
	if err != nil {
		return nil, fmt.Errorf("robust node detail: %w", err)
	}

	detail := model.GpuNodeDetail{
		Name:             resp.NodeName,
		Health:           resp.HealthStatus,
		StaticGpuDetails: fmt.Sprintf("%d GPUs", resp.GPUCount),
	}
	return &NodeDetailResponse{
		GpuNodeDetail: detail,
		ClusterName:   rc.ClusterName(),
	}, nil
}

func handleNodeGPUDevices(ctx context.Context, req *NodeGPUDevicesRequest) (*NodeGPUDevicesResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetNodeDevices(ctx, req.NodeName)
	if err != nil {
		return nil, fmt.Errorf("robust node devices: %w", err)
	}

	devices := make([]model.GpuDeviceInfo, 0, len(resp.GPUs))
	for _, g := range resp.GPUs {
		gpuID, _ := g["gpu_id"].(float64)
		gpuModel, _ := g["model"].(string)
		memory, _ := g["memory"].(string)
		util, _ := g["utilization"].(float64)
		temp, _ := g["temperature"].(float64)
		power, _ := g["power"].(float64)
		devices = append(devices, model.GpuDeviceInfo{
			DeviceId:    int(gpuID),
			Model:       gpuModel,
			Memory:      memory,
			Utilization: util,
			Temperature: temp,
			Power:       power,
		})
	}
	return &NodeGPUDevicesResponse{
		NodeName:    req.NodeName,
		ClusterName: rc.ClusterName(),
		Devices:     devices,
	}, nil
}

func handleGPUUtilization(ctx context.Context, req *GPUUtilizationRequest) (*GPUUtilizationResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetClusterGPUUtilization(ctx)
	if err != nil {
		return nil, fmt.Errorf("robust gpu utilization: %w", err)
	}

	utilPct := toFloat64(resp.AvgUtilization)
	return &model.GPUUtilization{
		Utilization: utilPct,
	}, nil
}

func handleGPUUtilizationHistory(ctx context.Context, req *GPUUtilizationHistoryRequest) (*GPUUtilizationHistoryResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.Step != "" {
		p.Set("step", req.Step)
	}

	raw, err := rc.GetRaw(ctx, "/cluster/gpu-utilization-history", p)
	if err != nil {
		return nil, fmt.Errorf("robust gpu utilization history: %w", err)
	}

	var result GPUUtilizationHistoryResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode gpu utilization history: %w", err)
	}
	return &result, nil
}

func handleNodeUtilization(ctx context.Context, req *NodeUtilizationRequest) (*NodeUtilizationResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetNodeUtilization(ctx, req.NodeName)
	if err != nil {
		return nil, fmt.Errorf("robust node utilization: %w", err)
	}

	return &model.NodeUtilization{
		NodeName:       resp.NodeName,
		GpuUtilization: toFloat64(resp.AvgGPUUtilization),
	}, nil
}

func handleNodeUtilizationHistory(ctx context.Context, req *NodeUtilizationHistoryRequest) (*NodeUtilizationHistoryResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.Step != "" {
		p.Set("step", req.Step)
	}

	raw, err := rc.GetRaw(ctx, "/nodes/"+req.NodeName+"/utilizationHistory", p)
	if err != nil {
		return nil, fmt.Errorf("robust node utilization history: %w", err)
	}

	var result NodeUtilizationHistoryResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode node utilization history: %w", err)
	}
	result.NodeName = req.NodeName
	return &result, nil
}

func handleNodeGPUMetrics(ctx context.Context, req *NodeGPUMetricsRequest) (*NodeGPUMetricsResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.Step != "" {
		p.Set("step", req.Step)
	}

	raw, err := rc.GetRaw(ctx, "/nodes/"+req.NodeName+"/gpu-metrics", p)
	if err != nil {
		return nil, fmt.Errorf("robust node gpu metrics: %w", err)
	}

	var result NodeGPUMetricsResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode node gpu metrics: %w", err)
	}
	return &result, nil
}

func handleNodeWorkloads(ctx context.Context, req *NodeWorkloadsRequest) (*NodeWorkloadsResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetNodeWorkloads(ctx, req.NodeName)
	if err != nil {
		return nil, fmt.Errorf("robust node workloads: %w", err)
	}

	views := make([]model.WorkloadNodeView, 0, len(resp.Workloads))
	for _, w := range resp.Workloads {
		views = append(views, model.WorkloadNodeView{
			Uid:          w.WorkloadID,
			Name:         w.Name,
			Namespace:    w.Namespace,
			GpuAllocated: w.GPUAllocated,
		})
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	start := (pageNum - 1) * pageSize
	end := start + pageSize
	if start > len(views) {
		start = len(views)
	}
	if end > len(views) {
		end = len(views)
	}

	return &NodeWorkloadsResponse{
		Data:  views[start:end],
		Total: resp.Count,
	}, nil
}

func handleNodeWorkloadsHistory(ctx context.Context, req *NodeWorkloadsHistoryRequest) (*NodeWorkloadsHistoryResponse, error) {
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

	p := url.Values{
		"page_num":  {strconv.Itoa(pageNum)},
		"page_size": {strconv.Itoa(pageSize)},
	}

	raw, err := rc.GetRaw(ctx, "/nodes/"+req.NodeName+"/workloadsHistory", p)
	if err != nil {
		return nil, fmt.Errorf("robust node workloads history: %w", err)
	}

	var result NodeWorkloadsHistoryResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode node workloads history: %w", err)
	}
	return &result, nil
}

// toFloat64 converts interface{} (string or float64) to float64.
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}

// ensure getRobustClient is used (defined in unified_cluster.go)
var _ = (*robust.Client)(nil)
