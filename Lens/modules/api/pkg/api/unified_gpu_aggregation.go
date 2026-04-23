// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for GPU aggregation operations.
// All data-plane queries are delegated to the Robust API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/robust"
)

func init() {
	unified.Register(&unified.EndpointDef[GpuAggClustersRequest, GpuAggClustersResponse]{
		Name:        "gpu_agg_clusters",
		Description: "Get list of all available clusters for GPU aggregation",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/clusters",
		MCPToolName: "lens_gpu_agg_clusters",
		Handler:     handleGpuAggClusters,
	})

	unified.Register(&unified.EndpointDef[GpuAggNamespacesRequest, GpuAggNamespacesResponse]{
		Name:        "gpu_agg_namespaces",
		Description: "Get namespace list within specified time range with GPU usage",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/namespaces",
		MCPToolName: "lens_gpu_agg_namespaces",
		Handler:     handleGpuAggNamespaces,
	})

	unified.Register(&unified.EndpointDef[GpuAggDimensionKeysRequest, GpuAggDimensionKeysResponse]{
		Name:        "gpu_agg_dimension_keys",
		Description: "Get label/annotation keys used for GPU aggregation within time range",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/dimension-keys",
		MCPToolName: "lens_gpu_agg_dimension_keys",
		Handler:     handleGpuAggDimensionKeys,
	})

	unified.Register(&unified.EndpointDef[GpuAggDimensionValuesRequest, GpuAggDimensionValuesResponse]{
		Name:        "gpu_agg_dimension_values",
		Description: "Get values for a specific dimension key within time range",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/dimension-values",
		MCPToolName: "lens_gpu_agg_dimension_values",
		Handler:     handleGpuAggDimensionValues,
	})

	unified.Register(&unified.EndpointDef[GpuAggClusterHourlyStatsRequest, PaginatedResponse]{
		Name:        "gpu_agg_cluster_stats",
		Description: "Query cluster-level GPU hourly statistics",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/cluster/hourly-stats",
		MCPToolName: "lens_gpu_agg_cluster_stats",
		Handler:     handleGpuAggClusterHourlyStats,
	})

	unified.Register(&unified.EndpointDef[GpuAggNamespaceHourlyStatsRequest, PaginatedResponse]{
		Name:        "gpu_agg_namespace_stats",
		Description: "Query namespace-level GPU hourly statistics",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/namespaces/hourly-stats",
		MCPToolName: "lens_gpu_agg_namespace_stats",
		Handler:     handleGpuAggNamespaceHourlyStats,
	})

	unified.Register(&unified.EndpointDef[GpuAggLabelHourlyStatsRequest, PaginatedResponse]{
		Name:        "gpu_agg_label_stats",
		Description: "Query label/annotation-level GPU hourly statistics",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/labels/hourly-stats",
		MCPToolName: "lens_gpu_agg_label_stats",
		Handler:     handleGpuAggLabelHourlyStats,
	})

	unified.Register(&unified.EndpointDef[GpuAggWorkloadHourlyStatsRequest, PaginatedResponse]{
		Name:        "gpu_agg_workload_stats",
		Description: "Query workload-level GPU hourly statistics",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/workloads/hourly-stats",
		MCPToolName: "lens_gpu_agg_workload_stats",
		Handler:     handleGpuAggWorkloadHourlyStats,
	})

	unified.Register(&unified.EndpointDef[GpuAggSnapshotRequest, dbmodel.GpuAllocationSnapshots]{
		Name:        "gpu_agg_latest_snapshot",
		Description: "Get the latest GPU allocation snapshot",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/snapshots/latest",
		MCPToolName: "lens_gpu_agg_latest_snapshot",
		Handler:     handleGpuAggLatestSnapshot,
	})

	unified.Register(&unified.EndpointDef[GpuAggSnapshotsListRequest, []*dbmodel.GpuAllocationSnapshots]{
		Name:        "gpu_agg_snapshots",
		Description: "Query historical GPU allocation snapshots",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/snapshots",
		MCPToolName: "lens_gpu_agg_snapshots",
		Handler:     handleGpuAggSnapshots,
	})
}

// ======================== Request Types ========================

type GpuAggClustersRequest struct{}

type GpuAggNamespacesRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	StartTime string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime   string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
}

type GpuAggDimensionKeysRequest struct {
	Cluster       string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	DimensionType string `json:"dimension_type" query:"dimension_type" mcp:"description=Dimension type: label or annotation,required"`
	StartTime     string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime       string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
}

type GpuAggDimensionValuesRequest struct {
	Cluster       string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	DimensionType string `json:"dimension_type" query:"dimension_type" mcp:"description=Dimension type: label or annotation,required"`
	DimensionKey  string `json:"dimension_key" query:"dimension_key" mcp:"description=Dimension key name,required"`
	StartTime     string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime       string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
}

type GpuAggClusterHourlyStatsRequest struct {
	Cluster        string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	StartTime      string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format)"`
	EndTime        string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format)"`
	Hours          int    `json:"hours" query:"hours" mcp:"description=Shortcut: query last N hours (used if start_time/end_time not set)"`
	Page           int    `json:"page" query:"page" mcp:"description=Page number (default 1)"`
	PageSize       int    `json:"page_size" query:"page_size" mcp:"description=Items per page (default 20 max 1000)"`
	OrderBy        string `json:"order_by" query:"order_by" mcp:"description=Sort field: time or utilization"`
	OrderDirection string `json:"order_direction" query:"order_direction" mcp:"description=Sort direction: asc or desc"`
}

type GpuAggNamespaceHourlyStatsRequest struct {
	Cluster        string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Namespace      string `json:"namespace" query:"namespace" mcp:"description=Namespace name (optional query all if empty)"`
	StartTime      string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime        string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
	Page           int    `json:"page" query:"page" mcp:"description=Page number (default 1)"`
	PageSize       int    `json:"page_size" query:"page_size" mcp:"description=Items per page (default 20 max 1000)"`
	OrderBy        string `json:"order_by" query:"order_by" mcp:"description=Sort field: time or utilization"`
	OrderDirection string `json:"order_direction" query:"order_direction" mcp:"description=Sort direction: asc or desc"`
}

type GpuAggLabelHourlyStatsRequest struct {
	Cluster        string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	DimensionType  string `json:"dimension_type" query:"dimension_type" mcp:"description=Dimension type: label or annotation,required"`
	DimensionKey   string `json:"dimension_key" query:"dimension_key" mcp:"description=Dimension key name,required"`
	DimensionValue string `json:"dimension_value" query:"dimension_value" mcp:"description=Dimension value (optional query all values if empty)"`
	StartTime      string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime        string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
	Page           int    `json:"page" query:"page" mcp:"description=Page number (default 1)"`
	PageSize       int    `json:"page_size" query:"page_size" mcp:"description=Items per page (default 20 max 1000)"`
	OrderBy        string `json:"order_by" query:"order_by" mcp:"description=Sort field: time utilization or allocated_gpu_count"`
	OrderDirection string `json:"order_direction" query:"order_direction" mcp:"description=Sort direction: asc or desc"`
}

type GpuAggWorkloadHourlyStatsRequest struct {
	Cluster        string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Namespace      string `json:"namespace" query:"namespace" mcp:"description=Namespace name (optional)"`
	WorkloadName   string `json:"workload_name" query:"workload_name" mcp:"description=Workload name (optional)"`
	WorkloadType   string `json:"workload_type" query:"workload_type" mcp:"description=Workload type (Job Deployment StatefulSet etc)"`
	StartTime      string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime        string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
	Page           int    `json:"page" query:"page" mcp:"description=Page number (default 1)"`
	PageSize       int    `json:"page_size" query:"page_size" mcp:"description=Items per page (default 20 max 1000)"`
	OrderBy        string `json:"order_by" query:"order_by" mcp:"description=Sort field: time utilization or allocated_gpu_count"`
	OrderDirection string `json:"order_direction" query:"order_direction" mcp:"description=Sort direction: asc or desc"`
}

type GpuAggSnapshotRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
}

type GpuAggSnapshotsListRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	StartTime string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format optional)"`
	EndTime   string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format optional)"`
}

// ======================== Response Types ========================

type GpuAggClustersResponse []string
type GpuAggNamespacesResponse []string
type GpuAggDimensionKeysResponse []string
type GpuAggDimensionValuesResponse []string

// ======================== Handler Implementations ========================

func handleGpuAggClusters(_ context.Context, _ *GpuAggClustersRequest) (*GpuAggClustersResponse, error) {
	seen := map[string]struct{}{}
	cm := clientsets.GetClusterManager()
	for _, name := range cm.GetClusterNames() {
		if name != "default" {
			seen[name] = struct{}{}
		}
	}
	for _, name := range robust.GetRegistry().ListClusters() {
		if name != "default" {
			seen[name] = struct{}{}
		}
	}

	clusters := make([]string, 0, len(seen))
	for name := range seen {
		clusters = append(clusters, name)
	}
	result := GpuAggClustersResponse(clusters)
	return &result, nil
}

func handleGpuAggNamespaces(ctx context.Context, req *GpuAggNamespacesRequest) (*GpuAggNamespacesResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	resp, err := rc.GetGpuAggNamespaces(ctx, req.StartTime, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("robust gpu agg namespaces: %w", err)
	}
	result := GpuAggNamespacesResponse(resp.Namespaces)
	return &result, nil
}

func handleGpuAggDimensionKeys(ctx context.Context, req *GpuAggDimensionKeysRequest) (*GpuAggDimensionKeysResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{
		"dimension_type": {req.DimensionType},
		"start_time":     {req.StartTime},
		"end_time":       {req.EndTime},
	}

	raw, err := rc.GetRaw(ctx, "/gpu-aggregation/dimension-keys", p)
	if err != nil {
		return nil, fmt.Errorf("robust gpu agg dimension keys: %w", err)
	}

	var resp robust.GpuAggDimensionKeysResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode dimension keys: %w", err)
	}
	result := GpuAggDimensionKeysResponse(resp.Keys)
	return &result, nil
}

func handleGpuAggDimensionValues(ctx context.Context, req *GpuAggDimensionValuesRequest) (*GpuAggDimensionValuesResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{
		"dimension_type": {req.DimensionType},
		"dimension_key":  {req.DimensionKey},
		"start_time":     {req.StartTime},
		"end_time":       {req.EndTime},
	}

	raw, err := rc.GetRaw(ctx, "/gpu-aggregation/dimension-values", p)
	if err != nil {
		return nil, fmt.Errorf("robust gpu agg dimension values: %w", err)
	}

	var resp robust.GpuAggDimensionValuesResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode dimension values: %w", err)
	}
	result := GpuAggDimensionValuesResponse(resp.Values)
	return &result, nil
}

func handleGpuAggClusterHourlyStats(ctx context.Context, req *GpuAggClusterHourlyStatsRequest) (*PaginatedResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	startTime := req.StartTime
	endTime := req.EndTime
	if startTime == "" && endTime == "" && req.Hours > 0 {
		now := time.Now().UTC()
		endTime = now.Format(time.RFC3339)
		startTime = now.Add(-time.Duration(req.Hours) * time.Hour).Format(time.RFC3339)
	}
	if startTime == "" || endTime == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).
			WithMessage("start_time and end_time are required, or use hours parameter")
	}

	return proxyGpuAggPaginated(ctx, rc, "/gpu-aggregation/cluster/hourly-stats", url.Values{
		"start_time":      {startTime},
		"end_time":        {endTime},
		"page":            {strconv.Itoa(req.Page)},
		"page_size":       {strconv.Itoa(req.PageSize)},
		"order_by":        {req.OrderBy},
		"order_direction": {req.OrderDirection},
	})
}

func handleGpuAggNamespaceHourlyStats(ctx context.Context, req *GpuAggNamespaceHourlyStatsRequest) (*PaginatedResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{
		"start_time":      {req.StartTime},
		"end_time":        {req.EndTime},
		"page":            {strconv.Itoa(req.Page)},
		"page_size":       {strconv.Itoa(req.PageSize)},
		"order_by":        {req.OrderBy},
		"order_direction": {req.OrderDirection},
	}
	if req.Namespace != "" {
		p.Set("namespace", req.Namespace)
	}

	return proxyGpuAggPaginated(ctx, rc, "/gpu-aggregation/namespaces/hourly-stats", p)
}

func handleGpuAggLabelHourlyStats(ctx context.Context, req *GpuAggLabelHourlyStatsRequest) (*PaginatedResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{
		"dimension_type":  {req.DimensionType},
		"dimension_key":   {req.DimensionKey},
		"start_time":      {req.StartTime},
		"end_time":        {req.EndTime},
		"page":            {strconv.Itoa(req.Page)},
		"page_size":       {strconv.Itoa(req.PageSize)},
		"order_by":        {req.OrderBy},
		"order_direction": {req.OrderDirection},
	}
	if req.DimensionValue != "" {
		p.Set("dimension_value", req.DimensionValue)
	}

	return proxyGpuAggPaginated(ctx, rc, "/gpu-aggregation/labels/hourly-stats", p)
}

func handleGpuAggWorkloadHourlyStats(ctx context.Context, req *GpuAggWorkloadHourlyStatsRequest) (*PaginatedResponse, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	p := url.Values{
		"start_time":      {req.StartTime},
		"end_time":        {req.EndTime},
		"page":            {strconv.Itoa(req.Page)},
		"page_size":       {strconv.Itoa(req.PageSize)},
		"order_by":        {req.OrderBy},
		"order_direction": {req.OrderDirection},
	}
	if req.Namespace != "" {
		p.Set("namespace", req.Namespace)
	}
	if req.WorkloadName != "" {
		p.Set("workload_name", req.WorkloadName)
	}
	if req.WorkloadType != "" {
		p.Set("workload_type", req.WorkloadType)
	}

	return proxyGpuAggPaginated(ctx, rc, "/gpu-aggregation/workloads/hourly-stats", p)
}

func handleGpuAggLatestSnapshot(ctx context.Context, req *GpuAggSnapshotRequest) (*dbmodel.GpuAllocationSnapshots, error) {
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/gpu-aggregation/snapshots/latest", nil)
	if err != nil {
		return nil, fmt.Errorf("robust gpu agg latest snapshot: %w", err)
	}

	var result dbmodel.GpuAllocationSnapshots
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode latest snapshot: %w", err)
	}
	return &result, nil
}

func handleGpuAggSnapshots(ctx context.Context, req *GpuAggSnapshotsListRequest) (*[]*dbmodel.GpuAllocationSnapshots, error) {
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

	raw, err := rc.GetRaw(ctx, "/gpu-aggregation/snapshots", p)
	if err != nil {
		return nil, fmt.Errorf("robust gpu agg snapshots: %w", err)
	}

	var result []*dbmodel.GpuAllocationSnapshots
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode snapshots: %w", err)
	}
	return &result, nil
}

// proxyGpuAggPaginated proxies a paginated GPU aggregation query to Robust.
// The frontend expects all hourly data in a single page (legacy behavior used
// PageSize=10000), so we override page_size regardless of the request value.
func proxyGpuAggPaginated(ctx context.Context, rc *robust.Client, path string, p url.Values) (*PaginatedResponse, error) {
	p.Set("page_size", "10000")
	raw, err := rc.GetRaw(ctx, path, p)
	if err != nil {
		return nil, fmt.Errorf("robust %s: %w", path, err)
	}

	var resp robust.GpuAggPaginatedResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	return &PaginatedResponse{
		Total:      int64(resp.Total),
		Page:       resp.Page,
		PageSize:   resp.PageSize,
		TotalPages: resp.TotalPages,
		Data:       resp.Data,
	}, nil
}
