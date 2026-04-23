// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for realtime status operations.
// These endpoints work for both HTTP REST and MCP protocols.
// All data-plane queries are delegated to the Robust API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Realtime Status =====

// RealtimeStatusRequest represents the request for realtime status.
type RealtimeStatusRequest struct {
	Cluster string   `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Include []string `json:"include" query:"include" mcp:"include,description=Fields to include (nodes/pods/workloads/alerts)"`
}

// RealtimeStatusResult is the same as RealtimeStatusResponse for backward compatibility.
type RealtimeStatusResult = RealtimeStatusResponse

// ===== Running Tasks =====

// RunningTasksRequest represents the request for running tasks.
type RunningTasksRequestUnified struct {
	Cluster   string `json:"cluster" mcp:"required,desc=Target cluster name"`
	Namespace string `json:"namespace" mcp:"desc=Filter by namespace"`
}

// RunningTasksResponseUnified is the response for running tasks.
type RunningTasksResponseUnified struct {
	Cluster    string        `json:"cluster"`
	Timestamp  time.Time     `json:"timestamp"`
	TotalTasks int           `json:"total_tasks"`
	Tasks      []RunningTask `json:"tasks"`
}

// ===== Register Realtime Endpoints =====

func init() {
	// Register realtime status endpoint - replaces getRealtimeStatus
	unified.Register(&unified.EndpointDef[RealtimeStatusRequest, RealtimeStatusResult]{
		Name:        "realtime_status",
		Description: "Get real-time cluster status including current GPU usage, running tasks count, and available resources. Provides a quick snapshot of cluster state.",
		HTTPMethod:  "GET",
		HTTPPath:    "/realtime/status",
		MCPToolName: "lens_realtime_status",
		Handler:     handleRealtimeStatus,
	})

	// Register running tasks endpoint
	unified.Register(&unified.EndpointDef[RunningTasksRequestUnified, RunningTasksResponseUnified]{
		Name:        "running_tasks",
		Description: "Get list of currently running GPU tasks with their resource allocation and status",
		HTTPMethod:  "GET",
		HTTPPath:    "/realtime/running-tasks",
		MCPToolName: "lens_running_tasks",
		Handler:     handleRunningTasks,
	})
}

// ===== Handler Implementations =====

func handleRealtimeStatus(ctx context.Context, req *RealtimeStatusRequest) (*RealtimeStatusResult, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	raw, err := rc.GetRaw(ctx, "/cluster/realtime", nil)
	if err != nil {
		return nil, fmt.Errorf("robust realtime status: %w", err)
	}
	var resp RealtimeStatusResult
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("robust realtime decode: %w", err)
	}
	return &resp, nil
}

func handleRunningTasks(ctx context.Context, req *RunningTasksRequestUnified) (*RunningTasksResponseUnified, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	p := url.Values{"status": {"Running"}}
	if req.Namespace != "" {
		p.Set("namespace", req.Namespace)
	}
	raw, err := rc.GetRaw(ctx, "/workloads", p)
	if err != nil {
		return nil, fmt.Errorf("robust running tasks: %w", err)
	}
	var resp RunningTasksResponseUnified
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("robust running tasks decode: %w", err)
	}
	return &resp, nil
}
