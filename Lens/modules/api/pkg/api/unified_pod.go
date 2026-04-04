// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for pod operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

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

func handlePodStats(ctx context.Context, req *PodStatsRequest) (*PodStatsListResponse, error) {
	rc, err := getRobustClient(req.Cluster)
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

	p := url.Values{}
	if req.Namespace != "" {
		p.Set("namespace", req.Namespace)
	}
	if req.PodName != "" {
		p.Set("pod_name", req.PodName)
	}
	if req.StartTime != "" {
		p.Set("start_time", req.StartTime)
	}
	if req.EndTime != "" {
		p.Set("end_time", req.EndTime)
	}
	p.Set("page", strconv.Itoa(page))
	p.Set("page_num", strconv.Itoa(page))
	p.Set("page_size", strconv.Itoa(pageSize))

	raw, err := rc.GetRaw(ctx, "/pods/stats", p)
	if err != nil {
		return nil, fmt.Errorf("robust pod stats: %w", err)
	}

	var wire struct {
		Total   int        `json:"total"`
		Page    int        `json:"page"`
		PageNum int        `json:"page_num"`
		Pods    []PodStats `json:"pods"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode pod stats: %w", err)
	}
	outPage := wire.Page
	if outPage == 0 {
		outPage = wire.PageNum
	}
	if outPage == 0 {
		outPage = page
	}
	return &PodStatsResponse{
		Total: wire.Total,
		Page:  outPage,
		Pods:  wire.Pods,
	}, nil
}

func handlePodDetail(ctx context.Context, req *PodDetailRequest) (*PodDetailResult, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/pods/"+req.PodUID, nil)
	if err != nil {
		return nil, fmt.Errorf("robust pod detail: %w", err)
	}

	var wire struct {
		PodUID         string         `json:"pod_uid"`
		PodName        string         `json:"pod_name"`
		Namespace      string         `json:"namespace"`
		NodeName       string         `json:"node_name"`
		Status         string         `json:"status"`
		Phase          string         `json:"phase"`
		CreatedAt      time.Time      `json:"created_at"`
		UpdatedAt      time.Time      `json:"updated_at"`
		AllocatedGPUs  int32          `json:"allocated_gpus"`
		GPUAllocated   int            `json:"gpu_allocated"`
		Running        bool           `json:"running"`
		Deleted        bool           `json:"deleted"`
		IP             string         `json:"ip"`
		OwnerUID       string         `json:"owner_uid"`
		WorkloadID     string         `json:"workload_id"`
		CurrentMetrics *PodGPUMetrics `json:"current_metrics"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode pod detail: %w", err)
	}

	alloc := wire.AllocatedGPUs
	if alloc == 0 && wire.GPUAllocated != 0 {
		alloc = int32(wire.GPUAllocated)
	}
	owner := wire.OwnerUID
	if owner == "" {
		owner = wire.WorkloadID
	}
	status := wire.Status
	if status == "" {
		if wire.Running {
			status = "Running"
		} else {
			switch wire.Phase {
			case "Pending", "Succeeded", "Failed":
				status = wire.Phase
			default:
				status = "Unknown"
			}
		}
	}

	return &PodDetailResponse{
		PodUID:         wire.PodUID,
		PodName:        wire.PodName,
		Namespace:      wire.Namespace,
		NodeName:       wire.NodeName,
		Status:         status,
		Phase:          wire.Phase,
		CreatedAt:      wire.CreatedAt,
		UpdatedAt:      wire.UpdatedAt,
		AllocatedGPUs:  alloc,
		Running:        wire.Running,
		Deleted:        wire.Deleted,
		IP:             wire.IP,
		OwnerUID:       owner,
		CurrentMetrics: wire.CurrentMetrics,
	}, nil
}

func handlePodGPUHistory(ctx context.Context, req *PodGPUHistoryRequest) (*PodGPUHistoryResult, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	if req.StartTime != "" {
		p.Set("start_time", req.StartTime)
	}
	if req.EndTime != "" {
		p.Set("end_time", req.EndTime)
	}
	if req.Hours > 0 {
		p.Set("hours", strconv.Itoa(req.Hours))
	}
	if req.Granularity != "" {
		p.Set("granularity", req.Granularity)
	}

	raw, err := rc.GetRaw(ctx, "/pods/"+req.PodUID+"/gpu-history", p)
	if err != nil {
		return nil, fmt.Errorf("robust pod gpu history: %w", err)
	}

	var out PodGPUHistoryResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode pod gpu history: %w", err)
	}
	if out.PodUID == "" {
		out.PodUID = req.PodUID
	}
	if out.Granularity == "" {
		out.Granularity = "hourly"
	}
	return &out, nil
}

func handlePodEvents(ctx context.Context, req *PodEventsRequest) (*PodEventsResult, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/pods/"+req.PodUID+"/events", nil)
	if err != nil {
		return nil, fmt.Errorf("robust pod events: %w", err)
	}

	var out PodEventsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode pod events: %w", err)
	}
	if out.PodUID == "" {
		out.PodUID = req.PodUID
	}
	return &out, nil
}

func handlePodComparison(ctx context.Context, req *PodComparisonRequest) (*PodComparisonResult, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	p.Set("pod_uids", req.PodUIDs)

	raw, err := rc.GetRaw(ctx, "/pods/comparison", p)
	if err != nil {
		return nil, fmt.Errorf("robust pod comparison: %w", err)
	}

	var out PodComparisonResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode pod comparison: %w", err)
	}
	return &out, nil
}
