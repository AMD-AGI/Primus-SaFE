// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for realtime status operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
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

// handleRealtimeStatus handles realtime status requests.
// Reuses: buildRealtimeStatus, filterRealtimeResponse, cache logic
func handleRealtimeStatus(ctx context.Context, req *RealtimeStatusRequest) (*RealtimeStatusResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Parse include fields
	includeMap := make(map[string]bool)
	for _, field := range req.Include {
		includeMap[strings.ToLower(field)] = true
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := fmt.Sprintf("realtime_status:%s", req.Cluster)

	var response RealtimeStatusResponse
	err = cacheFacade.Get(ctx, cacheKey, &response)
	if err == nil && response.Cluster != "" && !response.Timestamp.IsZero() {
		// Cache hit - apply include filters
		filtered := filterRealtimeResponse(response, includeMap)
		return &filtered, nil
	}

	// Cache miss - build response using existing helper
	response, err = buildRealtimeStatus(ctx, clients.ClusterName, includeMap)
	if err != nil {
		return nil, err
	}

	// Cache the result
	expiresAt := time.Now().Add(30 * time.Second)
	_ = cacheFacade.Set(ctx, cacheKey, response, &expiresAt)

	return &response, nil
}

// handleRunningTasks handles running tasks requests.
func handleRunningTasks(ctx context.Context, req *RunningTasksRequestUnified) (*RunningTasksResponseUnified, error) {
	if req.Cluster == "" {
		return nil, fmt.Errorf("cluster is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := fmt.Sprintf("running_tasks:%s:%s", req.Cluster, req.Namespace)

	var response RunningTasksResponse
	err = cacheFacade.Get(ctx, cacheKey, &response)
	if err == nil && response.Cluster != "" && !response.Timestamp.IsZero() {
		// Cache hit
		return &RunningTasksResponseUnified{
			Cluster:    response.Cluster,
			Timestamp:  response.Timestamp,
			TotalTasks: response.TotalTasks,
			Tasks:      response.Tasks,
		}, nil
	}

	// Cache miss - build response
	response, err = buildRunningTasksResponse(ctx, clients.ClusterName, req.Namespace)
	if err != nil {
		return nil, err
	}

	// Cache the result
	expiresAt := time.Now().Add(30 * time.Second)
	_ = cacheFacade.Set(ctx, cacheKey, response, &expiresAt)

	return &RunningTasksResponseUnified{
		Cluster:    response.Cluster,
		Timestamp:  response.Timestamp,
		TotalTasks: response.TotalTasks,
		Tasks:      response.Tasks,
	}, nil
}
