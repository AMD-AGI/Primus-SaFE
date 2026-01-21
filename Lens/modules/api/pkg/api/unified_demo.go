// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides the unified framework demo endpoints.
// This file demonstrates how to use the new unified API framework
// to create endpoints that work for both HTTP REST and MCP.
package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
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

// ===== Phase 3: Core Query APIs =====

// GPUUtilizationRequest represents the request for GPU utilization.
type GPUUtilizationRequest struct {
	Cluster string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUUtilizationResponse represents GPU utilization response.
type GPUUtilizationResponse struct {
	ClusterName    string  `json:"cluster_name"`
	AllocationRate float64 `json:"allocation_rate"`
	Utilization    float64 `json:"utilization"`
	Timestamp      time.Time `json:"timestamp"`
}

// GPUUtilizationHistoryRequest represents the request for GPU utilization history.
type GPUUtilizationHistoryRequest struct {
	Cluster string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Start   string `query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End     string `query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step    string `query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// GPUUtilizationHistoryResponse represents GPU utilization history.
type GPUUtilizationHistoryResponse struct {
	ClusterName     string            `json:"cluster_name"`
	AllocationRate  []model.TimePoint `json:"allocation_rate"`
	Utilization     []model.TimePoint `json:"utilization"`
	VramUtilization []model.TimePoint `json:"vram_utilization"`
}

// NodeDetailRequest represents the request for node detail.
type NodeDetailRequest struct {
	Cluster  string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `param:"name" mcp:"name,description=Node name to get details for,required"`
}

// NodeDetailResponse represents the node detail response.
type NodeDetailResponse struct {
	model.GpuNodeDetail
	ClusterName string `json:"cluster_name"`
}

// NodeGPUDevicesRequest represents the request for node GPU devices.
type NodeGPUDevicesRequest struct {
	Cluster  string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `param:"name" mcp:"name,description=Node name to get GPU devices for,required"`
}

// NodeGPUDevicesResponse represents the node GPU devices response.
type NodeGPUDevicesResponse struct {
	NodeName    string              `json:"node_name"`
	ClusterName string              `json:"cluster_name"`
	Devices     []model.GpuDeviceInfo `json:"devices"`
}

// WorkloadListRequest represents the request for workload list.
type WorkloadListRequest struct {
	Cluster   string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name      string `query:"name" mcp:"name,description=Filter by workload name (partial match)"`
	Kind      string `query:"kind" mcp:"kind,description=Filter by workload kind (Job/PyTorchJob/etc)"`
	Namespace string `query:"namespace" mcp:"namespace,description=Filter by namespace"`
	Status    string `query:"status" mcp:"status,description=Filter by status (Running/Completed/Failed)"`
	PageNum   int    `query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize  int    `query:"page_size" mcp:"page_size,description=Page size (default 10)"`
	OrderBy   string `query:"order_by" mcp:"order_by,description=Order by field (start_at/end_at)"`
}

// WorkloadListResponse represents the workload list response.
type WorkloadListResponse struct {
	Data        []model.WorkloadListItem `json:"data"`
	Total       int                      `json:"total"`
	ClusterName string                   `json:"cluster_name"`
}

// WorkloadDetailRequest represents the request for workload detail.
type WorkloadDetailRequest struct {
	Cluster     string `query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `param:"uid" mcp:"uid,description=Workload UID to get details for,required"`
}

// WorkloadDetailResponse represents the workload detail response.
type WorkloadDetailResponse struct {
	model.WorkloadInfo
	ClusterName string `json:"cluster_name"`
}

// PodStatsRequest represents the request for pod stats.
type PodStatsRequest struct {
	Cluster   string `query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Namespace string `query:"namespace" mcp:"namespace,description=Filter by namespace"`
	PodName   string `query:"pod_name" mcp:"pod_name,description=Filter by pod name (partial match)"`
	StartTime string `query:"start_time" mcp:"start_time,description=Filter by start time (RFC3339)"`
	EndTime   string `query:"end_time" mcp:"end_time,description=Filter by end time (RFC3339)"`
	Page      int    `query:"page" mcp:"page,description=Page number (default 1)"`
	PageSize  int    `query:"page_size" mcp:"page_size,description=Page size (default 20)"`
}

// UnifiedPodStatsResponse represents the pod stats response for unified API.
type UnifiedPodStatsResponse struct {
	Total       int        `json:"total"`
	Page        int        `json:"page"`
	Pods        []PodStats `json:"pods"`
	ClusterName string     `json:"cluster_name"`
}

// RealtimeStatusRequest represents the request for realtime status.
type RealtimeStatusRequest struct {
	Cluster string   `query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Include []string `query:"include" mcp:"include,description=Fields to include (nodes/pods/workloads/alerts)"`
}

// RealtimeStatusResult represents the realtime status response.
type RealtimeStatusResult struct {
	Cluster            string                `json:"cluster"`
	Timestamp          time.Time             `json:"timestamp"`
	CurrentGPUUsage    RealtimeGPUUsage      `json:"current_gpu_usage"`
	RunningTasks       int                   `json:"running_tasks"`
	AvailableResources RealtimeAvailability  `json:"available_resources"`
}

// RealtimeGPUUsage represents GPU usage in realtime status.
type RealtimeGPUUsage struct {
	TotalGPUs       int     `json:"total_gpus"`
	AllocatedGPUs   int     `json:"allocated_gpus"`
	UtilizedGPUs    int     `json:"utilized_gpus"`
	AllocationRate  float64 `json:"allocation_rate"`
	UtilizationRate float64 `json:"utilization_rate"`
}

// RealtimeAvailability represents resource availability.
type RealtimeAvailability struct {
	AvailableGPUs    int `json:"available_gpus"`
	AvailableNodes   int `json:"available_nodes"`
	MaxContiguousGPU int `json:"max_contiguous_gpu"`
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

	// ===== Phase 3: Core Query APIs =====

	// Register GPU utilization endpoint (mirrors /nodes/gpuUtilization)
	unified.Register(&unified.EndpointDef[GPUUtilizationRequest, GPUUtilizationResponse]{
		Name:        "gpu_utilization",
		Description: "Get current cluster GPU utilization metrics including allocation rate and average utilization percentage across all GPUs.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/gpuUtilization",
		MCPToolName: "lens_gpu_utilization",
		Handler:     handleGPUUtilization,
	})

	// Register GPU utilization history endpoint (mirrors /nodes/gpuUtilizationHistory)
	unified.Register(&unified.EndpointDef[GPUUtilizationHistoryRequest, GPUUtilizationHistoryResponse]{
		Name:        "gpu_utilization_history",
		Description: "Get historical GPU utilization data over a time range. Returns allocation rate, utilization, and VRAM utilization as time series data points.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/gpuUtilizationHistory",
		MCPToolName: "lens_gpu_utilization_history",
		Handler:     handleGPUUtilizationHistory,
	})

	// Register node detail endpoint (mirrors /nodes/:name)
	unified.Register(&unified.EndpointDef[NodeDetailRequest, NodeDetailResponse]{
		Name:        "node_detail",
		Description: "Get detailed information about a specific GPU node including CPU, memory, OS, GPU driver version, kubelet version and health status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name",
		MCPToolName: "lens_node_detail",
		Handler:     handleNodeDetail,
	})

	// Register node GPU devices endpoint (mirrors /nodes/:name/gpuDevices)
	unified.Register(&unified.EndpointDef[NodeGPUDevicesRequest, NodeGPUDevicesResponse]{
		Name:        "node_gpu_devices",
		Description: "Get GPU device information for a specific node including device ID, model, memory, utilization, temperature and power for each GPU.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/gpuDevices",
		MCPToolName: "lens_node_gpu_devices",
		Handler:     handleNodeGPUDevices,
	})

	// Register workload list endpoint (mirrors /workloads)
	unified.Register(&unified.EndpointDef[WorkloadListRequest, WorkloadListResponse]{
		Name:        "workload_list",
		Description: "List GPU workloads in the cluster with filtering by name, kind, namespace, status. Returns workload metadata, GPU allocation, and utilization statistics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/workloads",
		MCPToolName: "lens_workload_list",
		Handler:     handleWorkloadList,
	})

	// Register workload detail endpoint (mirrors /workloads/:uid)
	unified.Register(&unified.EndpointDef[WorkloadDetailRequest, WorkloadDetailResponse]{
		Name:        "workload_detail",
		Description: "Get detailed information about a specific workload by UID including pods, GPU allocation per node, start/end times, and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/workloads/:uid",
		MCPToolName: "lens_workload_detail",
		Handler:     handleWorkloadDetail,
	})

	// Register pod stats endpoint (mirrors /pods/stats)
	unified.Register(&unified.EndpointDef[PodStatsRequest, UnifiedPodStatsResponse]{
		Name:        "pod_stats",
		Description: "Query GPU pod statistics with filtering by namespace, pod name, and time range. Returns pod details, GPU allocation, and average utilization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/pods/stats",
		MCPToolName: "lens_pod_stats",
		Handler:     handlePodStats,
	})

	// Register realtime status endpoint (mirrors /realtime/status)
	unified.Register(&unified.EndpointDef[RealtimeStatusRequest, RealtimeStatusResult]{
		Name:        "realtime_status",
		Description: "Get real-time cluster status including current GPU usage, running tasks count, and available resources. Provides a quick snapshot of cluster state.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/realtime/status",
		MCPToolName: "lens_realtime_status",
		Handler:     handleRealtimeStatus,
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

// ===== Phase 3 Handler Implementations =====

// handleGPUUtilization handles GPU utilization requests.
func handleGPUUtilization(ctx context.Context, req *GPUUtilizationRequest) (*GPUUtilizationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:utilization"

	var cachedResult model.GPUUtilization
	err = cacheFacade.Get(ctx, cacheKey, &cachedResult)
	if err == nil {
		return &GPUUtilizationResponse{
			ClusterName:    clients.ClusterName,
			AllocationRate: cachedResult.AllocationRate,
			Utilization:    cachedResult.Utilization,
			Timestamp:      time.Now(),
		}, nil
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

	return &GPUUtilizationResponse{
		ClusterName:    clients.ClusterName,
		AllocationRate: allocationRate,
		Utilization:    usage,
		Timestamp:      time.Now(),
	}, nil
}

// handleGPUUtilizationHistory handles GPU utilization history requests.
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

	return &GPUUtilizationHistoryResponse{
		ClusterName:     clients.ClusterName,
		AllocationRate:  allocationHistory,
		Utilization:     usageHistory,
		VramUtilization: vramHistory,
	}, nil
}

// handleNodeDetail handles node detail requests.
func handleNodeDetail(ctx context.Context, req *NodeDetailRequest) (*NodeDetailResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if dbNode == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

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
func handleNodeGPUDevices(ctx context.Context, req *NodeGPUDevicesRequest) (*NodeGPUDevicesResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

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

// handleWorkloadList handles workload list requests.
func handleWorkloadList(ctx context.Context, req *WorkloadListRequest) (*WorkloadListResponse, error) {
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

	emptyParentUid := ""
	f := &filter.WorkloadFilter{
		Limit:     pageSize,
		Offset:    (pageNum - 1) * pageSize,
		ParentUid: &emptyParentUid,
	}
	if req.Name != "" {
		f.Name = &req.Name
	}
	if req.Kind != "" {
		f.Kind = &req.Kind
	}
	if req.Namespace != "" {
		f.Namespace = &req.Namespace
	}
	if req.Status != "" {
		f.Status = &req.Status
	}
	if req.OrderBy != "" {
		switch req.OrderBy {
		case "start_at":
			f.OrderBy = "created_at"
		case "end_at":
			f.OrderBy = "end_at"
		}
	}

	workloads, count, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		return nil, err
	}

	result := []model.WorkloadListItem{}
	for _, w := range workloads {
		item, _ := cvtDBWorkloadListItem(ctx, clients.ClusterName, w)
		result = append(result, item)
	}

	return &WorkloadListResponse{
		Data:        result,
		Total:       count,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleWorkloadDetail handles workload detail requests.
func handleWorkloadDetail(ctx context.Context, req *WorkloadDetailRequest) (*WorkloadDetailResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	dbWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, req.WorkloadUID)
	if err != nil {
		return nil, err
	}
	if dbWorkload == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

	workloadInfo := model.WorkloadInfo{
		ApiVersion:    dbWorkload.GroupVersion,
		Kind:          dbWorkload.Kind,
		Name:          dbWorkload.Name,
		Namespace:     dbWorkload.Namespace,
		Uid:           dbWorkload.UID,
		GpuAllocation: nil,
		Pods:          []model.WorkloadInfoPod{},
		ActivePods:    []model.WorkloadInfoPod{},
		StartTime:     dbWorkload.CreatedAt.Unix(),
		EndTime:       dbWorkload.EndAt.Unix(),
		Source:        getSource(dbWorkload),
	}
	if dbWorkload.EndAt.Unix() < int64(8*time.Hour) {
		workloadInfo.EndTime = 0
	}

	pods, err := workload.GetWorkloadPods(ctx, clients.ClusterName, dbWorkload.UID)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		podInfo := model.WorkloadInfoPod{
			PodUID:       pod.UID,
			PodNamespace: pod.Namespace,
			PodName:      pod.Name,
			NodeName:     pod.NodeName,
			Phase:        pod.Phase,
			Running:      pod.Running,
			IP:           pod.IP,
			GpuAllocated: int(pod.GpuAllocated),
			CreatedAt:    pod.CreatedAt.Unix(),
			UpdatedAt:    pod.UpdatedAt.Unix(),
		}
		workloadInfo.Pods = append(workloadInfo.Pods, podInfo)
		if pod.Running && !pod.Deleted {
			workloadInfo.ActivePods = append(workloadInfo.ActivePods, podInfo)
		}
	}

	gpuAllocation, err := workload.GetWorkloadResource(ctx, clients.ClusterName, dbWorkload.UID)
	if err == nil {
		workloadInfo.GpuAllocation = gpuAllocation
	}

	return &WorkloadDetailResponse{
		WorkloadInfo: workloadInfo,
		ClusterName:  clients.ClusterName,
	}, nil
}

// handlePodStats handles pod stats requests.
func handlePodStats(ctx context.Context, req *PodStatsRequest) (*UnifiedPodStatsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	gpuPods, total, err := podFacade.QueryPodsWithFilters(
		ctx,
		req.Namespace,
		req.PodName,
		req.StartTime,
		req.EndTime,
		page,
		pageSize,
	)
	if err != nil {
		return nil, err
	}

	pods := make([]PodStats, 0, len(gpuPods))
	for _, pod := range gpuPods {
		avgUtil, _ := podFacade.GetAverageGPUUtilizationByNode(ctx, pod.NodeName)
		status := "Unknown"
		if pod.Running {
			status = "Running"
		} else {
			switch pod.Phase {
			case "Pending":
				status = "Pending"
			case "Succeeded":
				status = "Succeeded"
			case "Failed":
				status = "Failed"
			}
		}

		pods = append(pods, PodStats{
			PodUID:         pod.UID,
			PodName:        pod.Name,
			Namespace:      pod.Namespace,
			NodeName:       pod.NodeName,
			Status:         status,
			Phase:          pod.Phase,
			CreatedAt:      pod.CreatedAt,
			AllocatedGPUs:  pod.GpuAllocated,
			AvgUtilization: avgUtil,
			Running:        pod.Running,
			OwnerUID:       pod.OwnerUID,
			IP:             pod.IP,
		})
	}

	return &UnifiedPodStatsResponse{
		Total:       int(total),
		Page:        page,
		Pods:        pods,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleRealtimeStatus handles realtime status requests.
func handleRealtimeStatus(ctx context.Context, req *RealtimeStatusRequest) (*RealtimeStatusResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	// Get GPU allocation info
	allocations, err := gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}

	totalGPUs := 0
	allocatedGPUs := 0
	for _, alloc := range allocations {
		totalGPUs += alloc.Capacity
		allocatedGPUs += alloc.Allocated
	}

	// Get utilization
	utilization, _ := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)

	// Estimate utilized GPUs (>50% utilization)
	utilizedGPUs := int(float64(allocatedGPUs) * utilization / 100)

	// Get running tasks count
	pods, _ := facade.GetPod().ListActiveGpuPods(ctx)

	// Calculate available nodes (nodes with available GPU slots)
	availableNodes := 0
	maxContiguous := 0
	for _, alloc := range allocations {
		available := alloc.Capacity - alloc.Allocated
		if available > 0 {
			availableNodes++
			if available > maxContiguous {
				maxContiguous = available
			}
		}
	}

	return &RealtimeStatusResult{
		Cluster:   clients.ClusterName,
		Timestamp: time.Now(),
		CurrentGPUUsage: RealtimeGPUUsage{
			TotalGPUs:       totalGPUs,
			AllocatedGPUs:   allocatedGPUs,
			UtilizedGPUs:    utilizedGPUs,
			AllocationRate:  float64(allocatedGPUs) / float64(totalGPUs) * 100,
			UtilizationRate: utilization,
		},
		RunningTasks: len(pods),
		AvailableResources: RealtimeAvailability{
			AvailableGPUs:    totalGPUs - allocatedGPUs,
			AvailableNodes:   availableNodes,
			MaxContiguousGPU: maxContiguous,
		},
	}, nil
}
