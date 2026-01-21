// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for node operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/node"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/sliceUtil"
)

// ===== Node List =====

// NodeListRequest represents the request for GPU node list.
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

// NodeListResponse represents the GPU node list response.
type NodeListResponse struct {
	Data        []model.GPUNode `json:"data"`
	Total       int             `json:"total"`
	ClusterName string          `json:"cluster_name"`
}

// ===== GPU Allocation =====

// GPUAllocationRequest represents the request for GPU allocation info.
type GPUAllocationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUAllocationResponse represents the GPU allocation response.
// Returns array directly for backward compatibility with existing API.
type GPUAllocationResponse []model.GpuAllocation

// ===== Node Detail =====

// NodeDetailRequest represents the request for node detail.
type NodeDetailRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
}

// NodeDetailResponse represents the node detail response.
type NodeDetailResponse struct {
	model.GpuNodeDetail
	ClusterName string `json:"cluster_name"`
}

// ===== Node GPU Devices =====

// NodeGPUDevicesRequest represents the request for node GPU devices.
type NodeGPUDevicesRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
}

// NodeGPUDevicesResponse represents the node GPU devices response.
type NodeGPUDevicesResponse struct {
	NodeName    string                `json:"node_name"`
	ClusterName string                `json:"cluster_name"`
	Devices     []model.GpuDeviceInfo `json:"devices"`
}

// ===== GPU Utilization =====

// GPUUtilizationRequest represents the request for GPU utilization.
type GPUUtilizationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUUtilizationResponse is model.GPUUtilization for backward compatibility.
type GPUUtilizationResponse = model.GPUUtilization

// ===== GPU Utilization History =====

// GPUUtilizationHistoryRequest represents the request for GPU utilization history.
type GPUUtilizationHistoryRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Start   string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End     string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step    string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// GPUUtilizationHistoryResponse is model.GpuUtilizationHistory for backward compatibility.
type GPUUtilizationHistoryResponse = model.GpuUtilizationHistory

// ===== Node Utilization (Phase 4) =====

// NodeUtilizationRequest represents the request for node utilization.
type NodeUtilizationRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
}

// NodeUtilizationResponse is model.NodeUtilization for backward compatibility.
type NodeUtilizationResponse = model.NodeUtilization

// ===== Node Utilization History (Phase 4) =====

// NodeUtilizationHistoryRequest represents the request for node utilization history.
type NodeUtilizationHistoryRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	Start    string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End      string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step     string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// NodeUtilizationHistoryResponse represents the node utilization history response.
type NodeUtilizationHistoryResponse struct {
	NodeName       string                 `json:"node_name"`
	CpuUtilization []model.MetricsSeries  `json:"cpu_utilization"`
	MemUtilization []model.MetricsSeries  `json:"mem_utilization"`
	GpuUtilization []model.MetricsSeries  `json:"gpu_utilization"`
	GpuAllocation  []model.MetricsSeries  `json:"gpu_allocation"`
}

// ===== Node GPU Metrics (Phase 4) =====

// NodeGPUMetricsRequest represents the request for node GPU metrics.
type NodeGPUMetricsRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	Start    string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End      string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step     string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// NodeGPUMetricsResponse represents the node GPU metrics response.
type NodeGPUMetricsResponse struct {
	GpuUtilization    model.MetricsGraph `json:"gpu_utilization"`
	GpuAllocationRate model.MetricsGraph `json:"gpu_allocation_rate"`
}

// ===== Node Workloads (Phase 4) =====

// NodeWorkloadsRequest represents the request for node workloads.
type NodeWorkloadsRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
}

// NodeWorkloadsResponse represents the node workloads response.
type NodeWorkloadsResponse struct {
	Data  []model.WorkloadNodeView `json:"data"`
	Total int                      `json:"total"`
}

// ===== Node Workloads History (Phase 4) =====

// NodeWorkloadsHistoryRequest represents the request for node workloads history.
type NodeWorkloadsHistoryRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name,required"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
}

// NodeWorkloadsHistoryResponse represents the node workloads history response.
type NodeWorkloadsHistoryResponse struct {
	Data  []model.WorkloadHistoryNodeView `json:"data"`
	Total int                             `json:"total"`
}

// ===== Register Node Endpoints =====

func init() {
	// Register node list endpoint - replaces getGPUNodeList
	unified.Register(&unified.EndpointDef[NodeListRequest, NodeListResponse]{
		Name:        "node_list",
		Description: "List GPU nodes in the cluster with filtering and pagination. Returns node name, IP, GPU model, GPU count, allocation, utilization and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes",
		MCPToolName: "lens_node_list",
		Handler:     handleNodeList,
	})

	// Register GPU allocation endpoint - replaces getClusterGpuAllocationInfo
	unified.Register(&unified.EndpointDef[GPUAllocationRequest, GPUAllocationResponse]{
		Name:        "gpu_allocation",
		Description: "Get per-node GPU allocation showing capacity, allocated count, and allocation rate for each node.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/gpuAllocation",
		MCPToolName: "lens_gpu_allocation",
		Handler:     handleGPUAllocation,
	})

	// Register node detail endpoint - replaces getNodeInfoByName
	unified.Register(&unified.EndpointDef[NodeDetailRequest, NodeDetailResponse]{
		Name:        "node_detail",
		Description: "Get detailed information about a specific GPU node including CPU, memory, OS, GPU driver version, kubelet version and health status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name",
		MCPToolName: "lens_node_detail",
		Handler:     handleNodeDetail,
	})

	// Register node GPU devices endpoint - replaces getGpuDevice
	unified.Register(&unified.EndpointDef[NodeGPUDevicesRequest, NodeGPUDevicesResponse]{
		Name:        "node_gpu_devices",
		Description: "Get GPU device information for a specific node including device ID, model, memory, utilization, temperature and power.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/gpuDevices",
		MCPToolName: "lens_node_gpu_devices",
		Handler:     handleNodeGPUDevices,
	})

	// Register GPU utilization endpoint - replaces getClusterGPUUtilization
	unified.Register(&unified.EndpointDef[GPUUtilizationRequest, GPUUtilizationResponse]{
		Name:        "gpu_utilization",
		Description: "Get current cluster GPU utilization metrics including allocation rate and average utilization percentage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/gpuUtilization",
		MCPToolName: "lens_gpu_utilization",
		Handler:     handleGPUUtilization,
	})

	// Register GPU utilization history endpoint - replaces getGpuUsageHistory
	unified.Register(&unified.EndpointDef[GPUUtilizationHistoryRequest, GPUUtilizationHistoryResponse]{
		Name:        "gpu_utilization_history",
		Description: "Get historical GPU utilization data over a time range. Returns allocation rate, utilization, and VRAM utilization as time series.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/gpuUtilizationHistory",
		MCPToolName: "lens_gpu_utilization_history",
		Handler:     handleGPUUtilizationHistory,
	})

	// Phase 4: Register node utilization endpoint - replaces getNodeUtilization
	unified.Register(&unified.EndpointDef[NodeUtilizationRequest, NodeUtilizationResponse]{
		Name:        "node_utilization",
		Description: "Get current utilization metrics for a specific node including CPU, memory, and GPU utilization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/utilization",
		MCPToolName: "lens_node_utilization",
		Handler:     handleNodeUtilization,
	})

	// Phase 4: Register node utilization history endpoint - replaces getNodeUtilizationHistory
	unified.Register(&unified.EndpointDef[NodeUtilizationHistoryRequest, NodeUtilizationHistoryResponse]{
		Name:        "node_utilization_history",
		Description: "Get historical utilization metrics for a node over a time range including CPU, memory, GPU utilization and allocation.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/utilizationHistory",
		MCPToolName: "lens_node_utilization_history",
		Handler:     handleNodeUtilizationHistory,
	})

	// Phase 4: Register node GPU metrics endpoint - replaces getNodeGpuMetrics
	unified.Register(&unified.EndpointDef[NodeGPUMetricsRequest, NodeGPUMetricsResponse]{
		Name:        "node_gpu_metrics",
		Description: "Get detailed GPU metrics for a node over a time range including utilization, allocation, VRAM, power and temperature.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/gpuMetrics",
		MCPToolName: "lens_node_gpu_metrics",
		Handler:     handleNodeGPUMetrics,
	})

	// Phase 4: Register node workloads endpoint - replaces getNodeWorkload
	unified.Register(&unified.EndpointDef[NodeWorkloadsRequest, NodeWorkloadsResponse]{
		Name:        "node_workloads",
		Description: "Get currently running GPU workloads on a specific node.",
		HTTPMethod:  "GET",
		HTTPPath:    "/nodes/:name/workloads",
		MCPToolName: "lens_node_workloads",
		Handler:     handleNodeWorkloads,
	})

	// Phase 4: Register node workloads history endpoint - replaces getNodeWorkloadHistory
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

// handleNodeList handles node list requests.
// Reuses: database.GetFacade().GetNode().SearchNode, batchCvtDbNode2GpuNodeListNode
func handleNodeList(ctx context.Context, req *NodeListRequest) (*NodeListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
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

	// Reuse existing search request structure
	searchReq := model.SearchGpuNodeReq{
		Name:    req.Name,
		GpuName: req.GpuName,
		Status:  req.Status,
		OrderBy: req.OrderBy,
		Desc:    req.Desc,
	}
	searchReq.PageNum = pageNum
	searchReq.PageSize = pageSize

	filter := searchReq.ToNodeFilter()

	// Reuse existing database query
	dbNodes, total, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().SearchNode(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Reuse existing conversion function
	return &NodeListResponse{
		Data:        batchCvtDbNode2GpuNodeListNode(dbNodes),
		Total:       total,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleGPUAllocation handles GPU allocation info requests.
// Reuses: gpu.GetGpuNodesAllocation, database cache
// Returns array directly for backward compatibility.
func handleGPUAllocation(ctx context.Context, req *GPUAllocationRequest) (*GPUAllocationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:allocation_info"

	var result []model.GpuAllocation
	err = cacheFacade.Get(ctx, cacheKey, &result)
	if err != nil {
		// Cache miss, use real-time calculation
		result, err = gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
		if err != nil {
			return nil, err
		}
	}

	resp := GPUAllocationResponse(result)
	return &resp, nil
}

// handleNodeDetail handles node detail requests.
// Reuses: database.GetFacade().GetNode().GetNodeByName
func handleNodeDetail(ctx context.Context, req *NodeDetailRequest) (*NodeDetailResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Reuse existing database query
	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if dbNode == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

	// Convert to response model
	detail := model.GpuNodeDetail{
		Name:              dbNode.Name,
		Health:            dbNode.Status,
		Cpu:               fmt.Sprintf("%d X %s", dbNode.CPUCount, dbNode.CPU),
		Memory:            dbNode.Memory,
		OS:                dbNode.Os,
		StaticGpuDetails:  fmt.Sprintf("%d X %s", dbNode.GpuCount, dbNode.GpuName),
		KubeletVersion:    dbNode.KubeletVersion,
		ContainerdVersion: dbNode.ContainerdVersion,
		GPUDriverVersion:  dbNode.DriverVersion,
	}

	return &NodeDetailResponse{
		GpuNodeDetail: detail,
		ClusterName:   clients.ClusterName,
	}, nil
}

// handleNodeGPUDevices handles node GPU devices requests.
// Reuses: database.GetFacade().GetNode().GetNodeByName, ListGpuDeviceByNodeId
func handleNodeGPUDevices(ctx context.Context, req *NodeGPUDevicesRequest) (*NodeGPUDevicesResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Reuse existing database queries
	node, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

	devices, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().ListGpuDeviceByNodeId(ctx, node.ID)
	if err != nil {
		return nil, err
	}

	deviceInfos := make([]model.GpuDeviceInfo, 0, len(devices))
	for _, d := range devices {
		deviceInfos = append(deviceInfos, model.GpuDeviceInfo{
			DeviceId:    int(d.GpuID),
			Model:       d.GpuModel,
			Memory:      fmt.Sprintf("%dGB", d.Memory/1024),
			Utilization: d.Utilization,
			Temperature: d.Temperature,
			Power:       d.Power,
		})
	}

	return &NodeGPUDevicesResponse{
		NodeName:    req.NodeName,
		ClusterName: clients.ClusterName,
		Devices:     deviceInfos,
	}, nil
}

// handleGPUUtilization handles GPU utilization requests.
// Reuses: gpu.CalculateGpuUsage, gpu.GetClusterGpuAllocationRateFromDB, cache
func handleGPUUtilization(ctx context.Context, req *GPUUtilizationRequest) (*GPUUtilizationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:utilization"

	var result model.GPUUtilization
	err = cacheFacade.Get(ctx, cacheKey, &result)
	if err == nil {
		return &result, nil
	}

	// Cache miss, fallback to real-time calculation
	usage, err := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRateFromDB(ctx, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	if err != nil {
		return nil, err
	}

	return &model.GPUUtilization{
		AllocationRate: allocationRate,
		Utilization:    usage,
	}, nil
}

// handleGPUUtilizationHistory handles GPU utilization history requests.
// Reuses: gpu.GetHistoryGpuUsage, gpu.GetHistoryGpuAllocationRate, gpu.GetNodeGpuVramUsageHistory
func handleGPUUtilizationHistory(ctx context.Context, req *GPUUtilizationHistoryRequest) (*GPUUtilizationHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value")
		}
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	// Try cache for default step
	if step == 60 {
		cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
		cacheKey := getGpuUsageHistoryCacheKey(startTime, endTime)
		if cacheKey != "" {
			var result model.GpuUtilizationHistory
			err = cacheFacade.Get(ctx, cacheKey, &result)
			if err == nil {
				filteredResult := filterGpuUsageHistoryByTimeRange(result, startTime, endTime)
				return &filteredResult, nil
			}
		}
	}

	// Cache miss or non-standard query
	usageHistory, err := gpu.GetHistoryGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		return nil, err
	}
	allocationHistory, err := gpu.GetHistoryGpuAllocationRate(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		return nil, err
	}
	vramHistory, err := gpu.GetNodeGpuVramUsageHistory(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		return nil, err
	}

	return &model.GpuUtilizationHistory{
		AllocationRate:  allocationHistory,
		Utilization:     usageHistory,
		VramUtilization: vramHistory,
	}, nil
}

// ===== Phase 4 Handler Implementations =====

// handleNodeUtilization handles node utilization requests.
// Reuses: database.GetNode().GetNodeByName, node.GetNodeCpuUtilHistory, node.GetNodeMemUtilHistory
func handleNodeUtilization(ctx context.Context, req *NodeUtilizationRequest) (*NodeUtilizationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get node from database
	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if dbNode == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

	// Get current utilization (last 1 minute average)
	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	cpuUtil, err := node.GetNodeCpuUtilHistory(ctx, clients.StorageClientSet, req.NodeName, oneMinuteAgo, now, 60)
	cpuUtilValue := 0.0
	if err == nil && len(cpuUtil) > 0 && len(cpuUtil[0].Values) > 0 {
		cpuUtilValue = cpuUtil[0].Values[len(cpuUtil[0].Values)-1].Value
	}

	memUtil, err := node.GetNodeMemUtilHistory(ctx, clients.StorageClientSet, req.NodeName, oneMinuteAgo, now, 60)
	memUtilValue := 0.0
	if err == nil && len(memUtil) > 0 && len(memUtil[0].Values) > 0 {
		memUtilValue = memUtil[0].Values[len(memUtil[0].Values)-1].Value
	}

	return &model.NodeUtilization{
		NodeName:       dbNode.Name,
		CpuUtilization: cpuUtilValue,
		MemUtilization: memUtilValue,
		GpuUtilization: dbNode.GpuUtilization,
		GpuAllocation:  int(dbNode.GpuAllocation),
		Timestamp:      time.Now().Unix(),
	}, nil
}

// handleNodeUtilizationHistory handles node utilization history requests.
// Reuses: node.GetNodeCpuUtilHistory, node.GetNodeMemUtilHistory, node.GetNodeGpuUtilHistory, node.GetNodeGpuAllocationHistory
func handleNodeUtilizationHistory(ctx context.Context, req *NodeUtilizationHistoryRequest) (*NodeUtilizationHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value")
		}
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)
	storageClient := clients.StorageClientSet

	cpuUtil, _ := node.GetNodeCpuUtilHistory(ctx, storageClient, req.NodeName, startTime, endTime, step)
	memUtil, _ := node.GetNodeMemUtilHistory(ctx, storageClient, req.NodeName, startTime, endTime, step)
	gpuUtil, _ := node.GetNodeGpuUtilHistory(ctx, storageClient, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)
	gpuAlloc, _ := node.GetNodeGpuAllocationHistory(ctx, storageClient, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)

	return &NodeUtilizationHistoryResponse{
		NodeName:       req.NodeName,
		CpuUtilization: cpuUtil,
		MemUtilization: memUtil,
		GpuUtilization: gpuUtil,
		GpuAllocation:  gpuAlloc,
	}, nil
}

// handleNodeGPUMetrics handles node GPU metrics requests.
// Reuses: node.GetNodeGpuUtilHistory, node.GetNodeGpuAllocationHistory
func handleNodeGPUMetrics(ctx context.Context, req *NodeGPUMetricsRequest) (*NodeGPUMetricsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value")
		}
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)
	storageClient := clients.StorageClientSet

	gpuUtil, err := node.GetNodeGpuUtilHistory(ctx, storageClient, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, err
	}
	gpuAlloc, err := node.GetNodeGpuAllocationHistory(ctx, storageClient, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, err
	}

	return &NodeGPUMetricsResponse{
		GpuUtilization: model.MetricsGraph{
			Series: gpuUtil,
			Config: model.MetricsGraphConfig{YAxisUnit: "%"},
		},
		GpuAllocationRate: model.MetricsGraph{
			Series: gpuAlloc,
			Config: model.MetricsGraphConfig{YAxisUnit: "%"},
		},
	}, nil
}

// handleNodeWorkloads handles node workloads requests.
// Reuses: workload.GetRunningTopLevelGpuWorkloadByNode, batchCvtDBWorkload2TopLevelGpuResource
func handleNodeWorkloads(ctx context.Context, req *NodeWorkloadsRequest) (*NodeWorkloadsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
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

	workloads, err := workload.GetRunningTopLevelGpuWorkloadByNode(ctx, clients.ClusterName, req.NodeName)
	if err != nil {
		return nil, err
	}

	workloadsResult, _, count, _ := sliceUtil.PaginateSlice(workloads, pageNum, pageSize)
	nodeViews, err := batchCvtDBWorkload2TopLevelGpuResource(ctx, clients.ClusterName, workloadsResult, req.NodeName)
	if err != nil {
		return nil, err
	}

	return &NodeWorkloadsResponse{
		Data:  nodeViews,
		Total: count,
	}, nil
}

// handleNodeWorkloadsHistory handles node workloads history requests.
// Reuses: database.GetPod().GetHistoryGpuPodByNodeName, workload.GetTopLevelWorkloadsByPods
func handleNodeWorkloadsHistory(ctx context.Context, req *NodeWorkloadsHistoryRequest) (*NodeWorkloadsHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
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

	page := &rest.Page{PageNum: pageNum, PageSize: pageSize}

	pods, count, err := database.GetFacadeForCluster(clients.ClusterName).GetPod().GetHistoryGpuPodByNodeName(ctx, req.NodeName, page.PageNum, page.PageSize)
	if err != nil {
		return nil, err
	}

	uids := []string{}
	for _, pod := range pods {
		uids = append(uids, pod.UID)
	}

	workloadMap := map[string]*dbModel.GpuWorkload{}
	workloads, err := workload.GetTopLevelWorkloadsByPods(ctx, clients.ClusterName, pods)
	if err != nil {
		return nil, err
	}
	for i := range workloads {
		gpuWorkload := workloads[i]
		workloadMap[gpuWorkload.UID] = gpuWorkload
	}

	references := map[string]string{}
	refs, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, uids)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		references[ref.PodUID] = ref.WorkloadUID
	}

	result := []model.WorkloadHistoryNodeView{}
	for _, pod := range pods {
		workloadUid := references[pod.UID]
		gpuWorkload := workloadMap[workloadUid]
		if gpuWorkload == nil {
			continue
		}
		view := model.WorkloadHistoryNodeView{
			Kind:         gpuWorkload.Kind,
			Name:         gpuWorkload.Name,
			Namespace:    gpuWorkload.Namespace,
			Uid:          gpuWorkload.UID,
			GpuAllocated: int(pod.GpuAllocated),
			PodName:      pod.Name,
			PodNamespace: pod.Namespace,
			StartTime:    pod.CreatedAt.Unix(),
			EndTime:      pod.UpdatedAt.Unix(),
		}
		result = append(result, view)
	}

	return &NodeWorkloadsHistoryResponse{
		Data:  result,
		Total: count,
	}, nil
}
