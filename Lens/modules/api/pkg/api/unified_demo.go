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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
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
