// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for pod operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Pod Stats =====

// PodStatsRequest represents the request for pod stats.
type PodStatsRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Filter by namespace"`
	PodName   string `json:"pod_name" query:"pod_name" mcp:"pod_name,description=Filter by pod name (partial match)"`
	StartTime string `json:"start_time" query:"start_time" mcp:"start_time,description=Filter by start time (RFC3339)"`
	EndTime   string `json:"end_time" query:"end_time" mcp:"end_time,description=Filter by end time (RFC3339)"`
	Page      int    `json:"page" query:"page" mcp:"page,description=Page number (default 1)"`
	PageSize  int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 20)"`
}

// PodStatsListResponse is the same as PodStatsResponse for backward compatibility.
type PodStatsListResponse = PodStatsResponse

// ===== Register Pod Endpoints =====

func init() {
	// Register pod stats endpoint - replaces getPodStats
	unified.Register(&unified.EndpointDef[PodStatsRequest, PodStatsListResponse]{
		Name:        "pod_stats",
		Description: "Query GPU pod statistics with filtering by namespace, pod name, and time range. Returns pod details, GPU allocation, and average utilization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/pods/stats",
		MCPToolName: "lens_pod_stats",
		Handler:     handlePodStats,
	})
}

// ===== Handler Implementations =====

// handlePodStats handles pod stats requests.
// Reuses: database.GetPod().QueryPodsWithFilters, queryPodsWithStats logic
func handlePodStats(ctx context.Context, req *PodStatsRequest) (*PodStatsListResponse, error) {
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

	// Reuse existing database facade
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Query pods with filters - reusing existing method
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

	// Convert to response format - same logic as original
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

	return &PodStatsResponse{
		Total: int(total),
		Page:  page,
		Pods:  pods,
	}, nil
}
