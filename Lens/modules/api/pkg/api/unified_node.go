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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
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
