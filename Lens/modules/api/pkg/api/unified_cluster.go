// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for cluster operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"

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

// ===== Cluster Overview =====

// ClusterOverviewRequest represents the request for cluster overview.
type ClusterOverviewRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// ClusterOverviewResponse is the same as model.GpuClusterOverview for backward compatibility.
type ClusterOverviewResponse = model.GpuClusterOverview

// ===== Cluster Consumers =====

// ClusterConsumersRequest represents the request for cluster consumers.
type ClusterConsumersRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number for pagination (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Number of items per page (default 10)"`
}

// ClusterConsumersResponse represents the cluster consumers response.
// Matches original API: {data: [...], total: int}
type ClusterConsumersResponse struct {
	Data  []model.TopLevelGpuResource `json:"data"`
	Total int                         `json:"total"`
}

// ===== GPU Heatmap =====

// ClusterGPUHeatmapRequest represents the request for GPU heatmap.
type ClusterGPUHeatmapRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// ClusterGPUHeatmapResponse represents the response for GPU heatmap.
type ClusterGPUHeatmapResponse struct {
	Power       model.Heatmap `json:"power"`
	Temperature model.Heatmap `json:"temperature"`
	Utilization model.Heatmap `json:"utilization"`
}

// ===== Register Cluster Endpoints =====

func init() {
	// Register cluster overview endpoint - replaces getClusterOverview in cluster.go
	unified.Register(&unified.EndpointDef[ClusterOverviewRequest, ClusterOverviewResponse]{
		Name:        "cluster_overview",
		Description: "Get comprehensive GPU cluster overview including node counts, health status, allocation rate, utilization, storage and RDMA statistics. Returns total/healthy/faulty nodes, idle/busy breakdown, and resource utilization metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/overview",
		MCPToolName: "lens_cluster_overview",
		Handler:     handleClusterOverview,
	})

	// Register cluster consumers endpoint - replaces getConsumerInfo in workload.go
	unified.Register(&unified.EndpointDef[ClusterConsumersRequest, ClusterConsumersResponse]{
		Name:        "cluster_consumers",
		Description: "List GPU resource consumers (workloads) in the cluster with their GPU allocation and utilization. Shows which workloads are using GPU resources and their current utilization percentage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/consumers",
		MCPToolName: "lens_cluster_consumers",
		Handler:     handleClusterConsumers,
	})

	// Register GPU heatmap endpoint - replaces getClusterGpuHeatmap in cluster.go
	unified.Register(&unified.EndpointDef[ClusterGPUHeatmapRequest, ClusterGPUHeatmapResponse]{
		Name:        "cluster_gpu_heatmap",
		Description: "Get GPU heatmap data showing power, temperature and utilization for top K GPUs. Useful for visualizing cluster-wide GPU health.",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/gpuHeatmap",
		MCPToolName: "lens_gpu_heatmap",
		Handler:     handleClusterGPUHeatmap,
	})
}

// ===== Handler Implementations =====
// These handlers reuse existing helper functions - no duplicate business logic

// handleClusterOverview handles cluster overview requests.
// Reuses: cluster.GetClusterOverviewFromCache, gpu.*, fault.*, storage.*, rdma.*
func handleClusterOverview(ctx context.Context, req *ClusterOverviewRequest) (*ClusterOverviewResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get cached data first (reusing existing helper)
	result, err := cluster.GetClusterOverviewFromCache(ctx, clients.ClusterName)
	if err == nil && result != nil {
		return result, nil
	}

	// Cache miss - fall back to real-time calculation (reusing existing helpers)
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

	return &model.GpuClusterOverview{
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
	}, nil
}

// handleClusterConsumers handles cluster consumers requests.
// Reuses: database.GetFacade().GetWorkload().ListRunningWorkload, workload.GetCurrentWorkloadGpuUtilization
func handleClusterConsumers(ctx context.Context, req *ClusterConsumersRequest) (*ClusterConsumersResponse, error) {
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

	// Reuse existing database query
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
		// Reuse existing helper for GPU utilization
		r.Stat.GpuUtilization, _ = workload.GetCurrentWorkloadGpuUtilization(ctx, dbWorkload.UID, clients.StorageClientSet)
		result = append(result, r)
	}

	// Reuse existing pagination helper
	data, _, total, _ := sliceUtil.PaginateSlice(result, pageNum, pageSize)

	return &ClusterConsumersResponse{
		Data:  data,
		Total: total,
	}, nil
}

// handleClusterGPUHeatmap handles GPU heatmap requests.
// Reuses: gpu.TopKGpuPowerInstant, gpu.TopKGpuUtilizationInstant, gpu.TopKGpuTemperatureInstant
func handleClusterGPUHeatmap(ctx context.Context, req *ClusterGPUHeatmapRequest) (*ClusterGPUHeatmapResponse, error) {
	k := 5 // Top K GPUs
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Reuse existing helpers
	power, err := gpu.TopKGpuPowerInstant(ctx, k, clients.StorageClientSet)
	if err != nil {
		return nil, err
	}
	util, err := gpu.TopKGpuUtilizationInstant(ctx, k, clients.StorageClientSet)
	if err != nil {
		return nil, err
	}
	temp, err := gpu.TopKGpuTemperatureInstant(ctx, k, clients.StorageClientSet)
	if err != nil {
		return nil, err
	}

	return &ClusterGPUHeatmapResponse{
		Power: model.Heatmap{
			Serial:   2,
			Unit:     "W",
			YAxisMax: 850,
			YAxisMin: 0,
			Data:     power,
		},
		Temperature: model.Heatmap{
			Serial:   3,
			Unit:     "℃",
			YAxisMax: 110,
			YAxisMin: 20,
			Data:     temp,
		},
		Utilization: model.Heatmap{
			Serial:   1,
			Unit:     "%",
			YAxisMax: 100,
			YAxisMin: 0,
			Data:     util,
		},
	}, nil
}

