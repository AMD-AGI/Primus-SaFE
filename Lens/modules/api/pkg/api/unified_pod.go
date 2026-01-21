// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for pod operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
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

// ===== Pod Detail (Phase 4) =====

// PodDetailRequest represents the request for pod detail.
type PodDetailRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	PodUID  string `json:"pod_uid" param:"pod_uid" mcp:"pod_uid,description=Pod UID,required"`
}

// PodDetailResult is the same as PodDetailResponse for backward compatibility.
type PodDetailResult = PodDetailResponse

// ===== Phase 5: Pod GPU History =====

// PodGPUHistoryRequest represents the request for pod GPU history.
type PodGPUHistoryRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	PodUID      string `json:"pod_uid" param:"pod_uid" mcp:"pod_uid,description=Pod UID,required"`
	StartTime   string `json:"start_time" query:"start_time" mcp:"start_time,description=Start time (RFC3339 format)"`
	EndTime     string `json:"end_time" query:"end_time" mcp:"end_time,description=End time (RFC3339 format)"`
	Hours       int    `json:"hours" query:"hours" mcp:"hours,description=Hours back from now (alternative to start_time/end_time)"`
	Granularity string `json:"granularity" query:"granularity" mcp:"granularity,description=Data granularity (minute/hourly/daily)"`
}

// PodGPUHistoryResult is the same as PodGPUHistoryResponse for backward compatibility.
type PodGPUHistoryResult = PodGPUHistoryResponse

// ===== Phase 5: Pod Events =====

// PodEventsRequest represents the request for pod events.
type PodEventsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	PodUID  string `json:"pod_uid" param:"pod_uid" mcp:"pod_uid,description=Pod UID,required"`
}

// PodEventsResult is the same as PodEventsResponse for backward compatibility.
type PodEventsResult = PodEventsResponse

// ===== Phase 5: Pod Comparison =====

// PodComparisonRequest represents the request for pod comparison.
type PodComparisonRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	PodUIDs string `json:"pod_uids" query:"pod_uids" mcp:"pod_uids,description=Comma-separated pod UIDs (2-10 pods),required"`
}

// PodComparisonResult is the same as PodComparisonResponse for backward compatibility.
type PodComparisonResult = PodComparisonResponse

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

	// Phase 4: Register pod detail endpoint - replaces getPodDetail
	unified.Register(&unified.EndpointDef[PodDetailRequest, PodDetailResult]{
		Name:        "pod_detail",
		Description: "Get detailed information about a specific GPU pod by UID including allocated GPUs, status, metrics, and owner information.",
		HTTPMethod:  "GET",
		HTTPPath:    "/pods/:pod_uid",
		MCPToolName: "lens_pod_detail",
		Handler:     handlePodDetail,
	})

	// Phase 5: Register pod GPU history endpoint - replaces getPodGPUHistory
	unified.Register(&unified.EndpointDef[PodGPUHistoryRequest, PodGPUHistoryResult]{
		Name:        "pod_gpu_history",
		Description: "Get GPU utilization history for a specific pod. Use either 'hours' parameter to get recent data or 'start_time' and 'end_time' for a specific range.",
		HTTPMethod:  "GET",
		HTTPPath:    "/pods/:pod_uid/gpu-history",
		MCPToolName: "lens_pod_gpu_history",
		Handler:     handlePodGPUHistory,
	})

	// Phase 5: Register pod events endpoint - replaces getPodEvents
	unified.Register(&unified.EndpointDef[PodEventsRequest, PodEventsResult]{
		Name:        "pod_events",
		Description: "Get events for a specific pod including phase changes, restarts, and other lifecycle events.",
		HTTPMethod:  "GET",
		HTTPPath:    "/pods/:pod_uid/events",
		MCPToolName: "lens_pod_events",
		Handler:     handlePodEvents,
	})

	// Phase 5: Register pod comparison endpoint - replaces comparePods
	unified.Register(&unified.EndpointDef[PodComparisonRequest, PodComparisonResult]{
		Name:        "pod_comparison",
		Description: "Compare GPU metrics across multiple pods. Provide 2-10 pod UIDs separated by commas to see utilization comparison and statistics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/pods/comparison",
		MCPToolName: "lens_pod_comparison",
		Handler:     handlePodComparison,
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

// ===== Phase 4 Handler Implementations =====

// handlePodDetail handles pod detail requests.
// Reuses: database.GetPod().GetGpuPodsByPodUid, getCurrentPodMetrics
func handlePodDetail(ctx context.Context, req *PodDetailRequest) (*PodDetailResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	pod, err := podFacade.GetGpuPodsByPodUid(ctx, req.PodUID)
	if err != nil {
		return nil, err
	}
	if pod == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("pod not found")
	}

	// Get current metrics - reuse existing helper
	metrics := getCurrentPodMetrics(ctx, podFacade, pod.NodeName)

	return &PodDetailResponse{
		PodUID:         pod.UID,
		PodName:        pod.Name,
		Namespace:      pod.Namespace,
		NodeName:       pod.NodeName,
		Status:         getStatusFromPhase(pod.Phase, pod.Running),
		Phase:          pod.Phase,
		CreatedAt:      pod.CreatedAt,
		UpdatedAt:      pod.UpdatedAt,
		AllocatedGPUs:  pod.GpuAllocated,
		Running:        pod.Running,
		Deleted:        pod.Deleted,
		IP:             pod.IP,
		OwnerUID:       pod.OwnerUID,
		CurrentMetrics: metrics,
	}, nil
}

// ===== Phase 5 Handler Implementations =====

// handlePodGPUHistory handles pod GPU history requests.
// Reuses: database.GetPod().GetGpuPodsByPodUid, queryGPUHistory
func handlePodGPUHistory(ctx context.Context, req *PodGPUHistoryRequest) (*PodGPUHistoryResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Default granularity
	granularity := req.Granularity
	if granularity == "" {
		granularity = "hourly"
	}

	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Get pod info first
	pod, err := podFacade.GetGpuPodsByPodUid(ctx, req.PodUID)
	if err != nil {
		return nil, err
	}
	if pod == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("pod not found")
	}

	// Parse time range - reuse existing logic
	var startTime, endTime time.Time

	if req.Hours > 0 {
		endTime = time.Now()
		startTime = endTime.Add(-time.Duration(req.Hours) * time.Hour)
	} else if req.StartTime != "" && req.EndTime != "" {
		startTime, err = time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start_time format, use RFC3339")
		}
		endTime, err = time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end_time format, use RFC3339")
		}
	} else {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("either 'hours' or both 'start_time' and 'end_time' are required")
	}

	// Query GPU history - reuse existing helper
	dataPoints := queryGPUHistory(ctx, podFacade, pod.NodeName, startTime, endTime, granularity)

	return &PodGPUHistoryResponse{
		PodUID:      req.PodUID,
		PodName:     pod.Name,
		Granularity: granularity,
		DataPoints:  dataPoints,
	}, nil
}

// handlePodEvents handles pod events requests.
// Reuses: database.GetPod().ListPodEventsByUID, queryPodEvents
func handlePodEvents(ctx context.Context, req *PodEventsRequest) (*PodEventsResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Query pod events - reuse existing helper
	events := queryPodEvents(ctx, podFacade, req.PodUID)

	return &PodEventsResponse{
		PodUID: req.PodUID,
		Events: events,
	}, nil
}

// handlePodComparison handles pod comparison requests.
// Reuses: database.GetPod().ListPodsByUids, GetAverageGPUUtilizationByNode
func handlePodComparison(ctx context.Context, req *PodComparisonRequest) (*PodComparisonResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Parse pod UIDs (comma-separated)
	podUIDs := strings.Split(req.PodUIDs, ",")
	if len(podUIDs) < 2 || len(podUIDs) > 10 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("pod_uids must contain 2-10 UIDs")
	}

	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Query pods
	pods, err := podFacade.ListPodsByUids(ctx, podUIDs)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("no pods found")
	}

	// Build comparison items - same logic as original
	comparisonItems := make([]PodComparisonItem, 0, len(pods))
	var totalUtil float64
	var highestUtil float64
	var lowestUtil float64 = 100.0
	var highestPod, lowestPod string

	for _, pod := range pods {
		avgUtil, _ := podFacade.GetAverageGPUUtilizationByNode(ctx, pod.NodeName)

		metrics := map[string]float64{
			"gpu_utilization": avgUtil,
		}

		comparisonItems = append(comparisonItems, PodComparisonItem{
			PodUID:        pod.UID,
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			AllocatedGPUs: pod.GpuAllocated,
			Metrics:       metrics,
		})

		totalUtil += avgUtil
		if avgUtil > highestUtil {
			highestUtil = avgUtil
			highestPod = pod.Name
		}
		if avgUtil < lowestUtil {
			lowestUtil = avgUtil
			lowestPod = pod.Name
		}
	}

	summary := ComparisonSummary{
		HighestUtilization: highestPod,
		LowestUtilization:  lowestPod,
		AvgUtilization:     totalUtil / float64(len(pods)),
	}

	return &PodComparisonResponse{
		Pods:       comparisonItems,
		Comparison: summary,
	}, nil
}
