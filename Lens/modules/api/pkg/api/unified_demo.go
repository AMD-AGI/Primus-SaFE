// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides the unified framework demo endpoints.
// This file demonstrates how to use the new unified API framework
// to create endpoints that work for both HTTP REST and MCP.
package api

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/cluster"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/fault"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/sliceUtil"
)

// ===== Request/Response Structs with MCP Tags =====

// ClusterHealthRequest represents the request for cluster health check.
type ClusterHealthRequest struct {
	Cluster string `query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// ClusterHealthResponse represents the response for cluster health check.
type ClusterHealthResponse struct {
	ClusterName   string    `json:"cluster_name"`
	Status        string    `json:"status"` // healthy, degraded, critical
	TotalNodes    int       `json:"total_nodes"`
	HealthyNodes  int       `json:"healthy_nodes"`
	GPUCount      int       `json:"gpu_count"`
	Timestamp     time.Time `json:"timestamp"`
	Message       string    `json:"message"`
}

// GPUQuickStatsRequest represents the request for quick GPU stats.
type GPUQuickStatsRequest struct {
	Cluster string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUQuickStatsResponse represents quick GPU statistics.
type GPUQuickStatsResponse struct {
	ClusterName      string  `json:"cluster_name"`
	TotalGPUs        int     `json:"total_gpus"`
	AllocatedGPUs    int     `json:"allocated_gpus"`
	AvailableGPUs    int     `json:"available_gpus"`
	AllocationRate   float64 `json:"allocation_rate"`
	AvgUtilization   float64 `json:"avg_utilization"`
	Timestamp        time.Time `json:"timestamp"`
}

// EchoRequest represents a simple echo request for testing.
type EchoRequest struct {
	Message string `json:"message" query:"message" mcp:"message,description=Message to echo back,required"`
	Count   int    `json:"count" query:"count" mcp:"count,description=Number of times to repeat (default 1)"`
}

// EchoResponse represents the echo response.
type EchoResponse struct {
	Echo      string    `json:"echo"`
	Count     int       `json:"count"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // "http" or "mcp"
}

// ===== Phase 2: Cluster Overview & GPU Allocation =====

// ClusterOverviewRequest represents the request for cluster overview.
type ClusterOverviewRequest struct {
	Cluster string `query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// ClusterOverviewResponse wraps the GpuClusterOverview model.
type ClusterOverviewResponse struct {
	model.GpuClusterOverview
	ClusterName string    `json:"cluster_name"`
	Timestamp   time.Time `json:"timestamp"`
}

// ClusterConsumersRequest represents the request for cluster consumers.
type ClusterConsumersRequest struct {
	Cluster  string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	PageNum  int    `query:"page_num" mcp:"page_num,description=Page number for pagination (default 1)"`
	PageSize int    `query:"page_size" mcp:"page_size,description=Number of items per page (default 10)"`
}

// ClusterConsumersResponse represents the cluster consumers response.
type ClusterConsumersResponse struct {
	Data        []model.TopLevelGpuResource `json:"data"`
	Total       int                         `json:"total"`
	ClusterName string                      `json:"cluster_name"`
}

// GPUNodeListRequest represents the request for GPU node list.
type GPUNodeListRequest struct {
	Cluster  string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name     string `query:"name" mcp:"name,description=Filter by node name (partial match)"`
	GpuName  string `query:"gpu_name" mcp:"gpu_name,description=Filter by GPU model name"`
	Status   string `query:"status" mcp:"status,description=Filter by node status (comma-separated for multiple: ready,notready)"`
	PageNum  int    `query:"page_num" mcp:"page_num,description=Page number for pagination (default 1)"`
	PageSize int    `query:"page_size" mcp:"page_size,description=Number of items per page (default 10)"`
	OrderBy  string `query:"order_by" mcp:"order_by,description=Field to order by"`
	Desc     bool   `query:"desc" mcp:"desc,description=Sort in descending order"`
}

// GPUNodeListResponse represents the GPU node list response.
type GPUNodeListResponse struct {
	Data        []model.GPUNode `json:"data"`
	Total       int             `json:"total"`
	ClusterName string          `json:"cluster_name"`
}

// GPUAllocationRequest represents the request for GPU allocation info.
type GPUAllocationRequest struct {
	Cluster string `query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// GPUAllocationResponse represents the GPU allocation response.
type GPUAllocationResponse struct {
	Data        []model.GpuAllocation `json:"data"`
	ClusterName string                `json:"cluster_name"`
	Summary     GPUAllocationSummary  `json:"summary"`
	Timestamp   time.Time             `json:"timestamp"`
}

// GPUAllocationSummary provides aggregated stats.
type GPUAllocationSummary struct {
	TotalNodes     int     `json:"total_nodes"`
	TotalGPUs      int     `json:"total_gpus"`
	AllocatedGPUs  int     `json:"allocated_gpus"`
	AvailableGPUs  int     `json:"available_gpus"`
	AllocationRate float64 `json:"allocation_rate_percent"`
}

// ===== Register Unified Endpoints =====

func init() {
	// Register cluster health endpoint
	unified.Register(&unified.EndpointDef[ClusterHealthRequest, ClusterHealthResponse]{
		Name:        "cluster_health",
		Description: "Get GPU cluster health status including node count and overall health",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/cluster/health",
		MCPToolName: "lens_cluster_health",
		Handler:     handleClusterHealth,
	})

	// Register GPU quick stats endpoint
	unified.Register(&unified.EndpointDef[GPUQuickStatsRequest, GPUQuickStatsResponse]{
		Name:        "gpu_quick_stats",
		Description: "Get quick GPU statistics including allocation and utilization",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/gpu/stats",
		MCPToolName: "lens_gpu_quick_stats",
		Handler:     handleGPUQuickStats,
	})

	// Register echo endpoint for simple testing
	unified.Register(&unified.EndpointDef[EchoRequest, EchoResponse]{
		Name:        "echo",
		Description: "Echo back the provided message - useful for testing MCP connectivity",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/echo",
		MCPToolName: "lens_echo",
		Handler:     handleEcho,
	})

	// ===== Phase 2: Cluster & Node APIs =====

	// Register cluster overview endpoint (mirrors /clusters/overview)
	unified.Register(&unified.EndpointDef[ClusterOverviewRequest, ClusterOverviewResponse]{
		Name:        "cluster_overview",
		Description: "Get comprehensive GPU cluster overview including node counts, health status, allocation rate, utilization, storage and RDMA statistics. Returns total/healthy/faulty nodes, idle/busy breakdown, and resource utilization metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/clusters/overview",
		MCPToolName: "lens_cluster_overview",
		Handler:     handleClusterOverview,
	})

	// Register cluster consumers endpoint (mirrors /clusters/consumers)
	unified.Register(&unified.EndpointDef[ClusterConsumersRequest, ClusterConsumersResponse]{
		Name:        "cluster_consumers",
		Description: "List GPU resource consumers (workloads) in the cluster with their GPU allocation and utilization. Shows which workloads are using GPU resources and their current utilization percentage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/clusters/consumers",
		MCPToolName: "lens_cluster_consumers",
		Handler:     handleClusterConsumers,
	})

	// Register GPU node list endpoint (mirrors /nodes)
	unified.Register(&unified.EndpointDef[GPUNodeListRequest, GPUNodeListResponse]{
		Name:        "node_list",
		Description: "List GPU nodes in the cluster with filtering and pagination. Returns node name, IP, GPU model, GPU count, allocation, utilization and status. Supports filtering by name, GPU model, and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes",
		MCPToolName: "lens_node_list",
		Handler:     handleGPUNodeList,
	})

	// Register GPU allocation endpoint (mirrors /nodes/gpuAllocation)
	unified.Register(&unified.EndpointDef[GPUAllocationRequest, GPUAllocationResponse]{
		Name:        "gpu_allocation",
		Description: "Get per-node GPU allocation information showing capacity, allocated count, and allocation rate for each node. Includes summary statistics for the entire cluster.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/gpuAllocation",
		MCPToolName: "lens_gpu_allocation",
		Handler:     handleGPUAllocation,
	})
}

// ===== Handler Implementations =====

// handleClusterHealth handles cluster health requests.
// Works for both HTTP and MCP.
func handleClusterHealth(ctx context.Context, req *ClusterHealthRequest) (*ClusterHealthResponse, error) {
	cm := clientsets.GetClusterManager()
	
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get GPU node names
	gpuNodeNames, err := gpu.GetGpuNodes(ctx, clients.K8SClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}

	totalNodes := len(gpuNodeNames)
	healthyNodes := totalNodes // Simplified - real implementation would check node conditions

	// Get GPU allocation info for GPU count
	gpuCount := 0
	allocations, err := gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err == nil {
		for _, alloc := range allocations {
			gpuCount += alloc.Capacity
		}
	}

	// Determine health status
	status := "healthy"
	message := "All systems operational"
	if totalNodes == 0 {
		status = "critical"
		message = "No GPU nodes found in cluster"
	} else if healthyNodes < totalNodes {
		healthyRatio := float64(healthyNodes) / float64(totalNodes)
		if healthyRatio < 0.5 {
			status = "critical"
			message = "More than 50% of nodes are unhealthy"
		} else if healthyRatio < 0.9 {
			status = "degraded"
			message = "Some nodes are unhealthy"
		}
	}

	return &ClusterHealthResponse{
		ClusterName:  clients.ClusterName,
		Status:       status,
		TotalNodes:   totalNodes,
		HealthyNodes: healthyNodes,
		GPUCount:     gpuCount,
		Timestamp:    time.Now(),
		Message:      message,
	}, nil
}

// handleGPUQuickStats handles GPU quick stats requests.
func handleGPUQuickStats(ctx context.Context, req *GPUQuickStatsRequest) (*GPUQuickStatsResponse, error) {
	cm := clientsets.GetClusterManager()
	
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get GPU allocation info
	allocations, err := gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}

	// Calculate stats
	totalGPUs := 0
	allocatedGPUs := 0
	for _, alloc := range allocations {
		totalGPUs += alloc.Capacity
		allocatedGPUs += alloc.Allocated
	}
	availableGPUs := totalGPUs - allocatedGPUs

	allocationRate := float64(0)
	if totalGPUs > 0 {
		allocationRate = float64(allocatedGPUs) / float64(totalGPUs) * 100
	}

	// Get utilization (simplified - real implementation would query metrics)
	avgUtilization := float64(0)
	usage, err := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err == nil {
		avgUtilization = usage
	}

	return &GPUQuickStatsResponse{
		ClusterName:    clients.ClusterName,
		TotalGPUs:      totalGPUs,
		AllocatedGPUs:  allocatedGPUs,
		AvailableGPUs:  availableGPUs,
		AllocationRate: allocationRate,
		AvgUtilization: avgUtilization,
		Timestamp:      time.Now(),
	}, nil
}

// handleEcho handles echo requests - simple test endpoint.
func handleEcho(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		count = 100 // Limit to prevent abuse
	}

	// Build repeated message
	echo := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			echo += " "
		}
		echo += req.Message
	}

	return &EchoResponse{
		Echo:      echo,
		Count:     count,
		Timestamp: time.Now(),
		Source:    "unified", // Handler doesn't know source, it's the same for both
	}, nil
}

// ===== Phase 2 Handler Implementations =====

// handleClusterOverview handles cluster overview requests.
// Mirrors the logic from getClusterOverview in cluster.go
func handleClusterOverview(ctx context.Context, req *ClusterOverviewRequest) (*ClusterOverviewResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get cached data first
	result, err := cluster.GetClusterOverviewFromCache(ctx, clients.ClusterName)
	if err == nil && result != nil {
		return &ClusterOverviewResponse{
			GpuClusterOverview: *result,
			ClusterName:        clients.ClusterName,
			Timestamp:          time.Now(),
		}, nil
	}

	// Cache miss - fall back to real-time calculation
	gpuNodes, err := gpu.GetGpuNodes(ctx, clients.K8SClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	faultyNodes, err := fault.GetFaultyNodes(ctx, clients.K8SClientSet, gpuNodes)
	if err != nil {
		return nil, err
	}
	idle, partialIdle, busy, err := gpu.GetGpuNodeIdleInfo(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	usage, err := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRateFromDB(ctx, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	if err != nil {
		return nil, err
	}
	storageStat, err := storage.GetStorageStat(ctx)
	if err != nil {
		return nil, err
	}
	rdmaStat, err := rdma.GetRdmaClusterStat(ctx, clients.StorageClientSet)
	if err != nil {
		return nil, err
	}

	overview := model.GpuClusterOverview{
		RdmaClusterStat:    rdmaStat,
		StorageStat:        *storageStat,
		TotalNodes:         len(gpuNodes),
		HealthyNodes:       len(gpuNodes) - len(faultyNodes),
		FaultyNodes:        len(faultyNodes),
		FullyIdleNodes:     idle,
		PartiallyIdleNodes: partialIdle,
		BusyNodes:          busy,
		AllocationRate:     allocationRate,
		Utilization:        usage,
	}

	return &ClusterOverviewResponse{
		GpuClusterOverview: overview,
		ClusterName:        clients.ClusterName,
		Timestamp:          time.Now(),
	}, nil
}

// handleClusterConsumers handles cluster consumers requests.
// Mirrors the logic from getConsumerInfo in workload.go
func handleClusterConsumers(ctx context.Context, req *ClusterConsumersRequest) (*ClusterConsumersResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Set default pagination
	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	runningWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().ListRunningWorkload(ctx)
	if err != nil {
		return nil, err
	}

	result := []model.TopLevelGpuResource{}
	for _, dbWorkload := range runningWorkload {
		r := model.TopLevelGpuResource{
			Kind:      dbWorkload.Kind,
			Name:      dbWorkload.Name,
			Namespace: dbWorkload.Namespace,
			Uid:       dbWorkload.UID,
			Stat: model.GpuStat{
				GpuRequest:     int(dbWorkload.GpuRequest),
				GpuUtilization: 0,
			},
			Pods:   nil,
			Source: getSource(dbWorkload),
		}
		// Get GPU utilization
		r.Stat.GpuUtilization, _ = workload.GetCurrentWorkloadGpuUtilization(ctx, dbWorkload.UID, clients.StorageClientSet)
		result = append(result, r)
	}

	data, _, total, _ := sliceUtil.PaginateSlice(result, pageNum, pageSize)

	return &ClusterConsumersResponse{
		Data:        data,
		Total:       total,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleGPUNodeList handles GPU node list requests.
// Mirrors the logic from getGPUNodeList in node.go
func handleGPUNodeList(ctx context.Context, req *GPUNodeListRequest) (*GPUNodeListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Set default pagination
	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	// Build search request
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

	dbNodes, total, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().SearchNode(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &GPUNodeListResponse{
		Data:        batchCvtDbNode2GpuNodeListNode(dbNodes),
		Total:       total,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleGPUAllocation handles GPU allocation info requests.
// Mirrors the logic from getClusterGpuAllocationInfo in node.go
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
		// Cache miss, fallback to real-time calculation
		result, err = gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
		if err != nil {
			return nil, err
		}
	}

	// Calculate summary
	summary := GPUAllocationSummary{
		TotalNodes: len(result),
	}
	for _, alloc := range result {
		summary.TotalGPUs += alloc.Capacity
		summary.AllocatedGPUs += alloc.Allocated
	}
	summary.AvailableGPUs = summary.TotalGPUs - summary.AllocatedGPUs
	if summary.TotalGPUs > 0 {
		summary.AllocationRate = float64(summary.AllocatedGPUs) / float64(summary.TotalGPUs) * 100
	}

	return &GPUAllocationResponse{
		Data:        result,
		ClusterName: clients.ClusterName,
		Summary:     summary,
		Timestamp:   time.Now(),
	}, nil
}
