// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"math"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// GPU Aggregation: Metadata queries
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

	// GPU Aggregation: Hourly stats
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

	// GPU Aggregation: Workload summary & distribution
	unified.Register(&unified.EndpointDef[GpuAggWorkloadSummaryRequest, GpuAggWorkloadSummaryResponse]{
		Name:        "gpu_agg_workload_summary",
		Description: "Get aggregated summary statistics for workloads (avg/max utilization, total GPU hours). Groups hourly data by workload.",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/workloads/stats",
		MCPToolName: "lens_gpu_agg_workload_summary",
		Handler:     handleGpuAggWorkloadSummary,
	})

	unified.Register(&unified.EndpointDef[GpuAggWorkloadTypeDistRequest, GpuAggWorkloadTypeDistResponse]{
		Name:        "gpu_agg_workload_type_dist",
		Description: "Get GPU usage distribution grouped by workload type (Job, Deployment, StatefulSet, etc.)",
		HTTPMethod:  "GET",
		HTTPPath:    "/gpu-aggregation/workloads/type-distribution",
		MCPToolName: "lens_gpu_agg_workload_type_dist",
		Handler:     handleGpuAggWorkloadTypeDist,
	})

	// GPU Aggregation: Snapshots
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
	StartTime      string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime        string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
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

type GpuAggWorkloadSummaryRequest struct {
	Cluster        string `json:"cluster" query:"cluster" mcp:"description=Cluster name,required"`
	Namespace      string `json:"namespace" query:"namespace" mcp:"description=Namespace filter (optional)"`
	WorkloadType   string `json:"workload_type" query:"workload_type" mcp:"description=Workload type filter (optional)"`
	StartTime      string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime        string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
	OrderBy        string `json:"order_by" query:"order_by" mcp:"description=Sort field: avg_utilization allocated_gpu_count or gpu_hours"`
	OrderDirection string `json:"order_direction" query:"order_direction" mcp:"description=Sort direction: asc or desc"`
	TopN           int    `json:"top_n" query:"top_n" mcp:"description=Limit number of results (default 50)"`
}

type GpuAggWorkloadTypeDistRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"description=Namespace filter (optional)"`
	StartTime string `json:"start_time" query:"start_time" mcp:"description=Start time (RFC3339 format),required"`
	EndTime   string `json:"end_time" query:"end_time" mcp:"description=End time (RFC3339 format),required"`
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

type WorkloadSummaryItem struct {
	Namespace         string  `json:"namespace"`
	WorkloadName      string  `json:"workload_name"`
	WorkloadType      string  `json:"workload_type"`
	AvgUtilization    float64 `json:"avg_utilization"`
	MaxUtilization    float64 `json:"max_utilization"`
	AvgAllocatedGpus  float64 `json:"avg_allocated_gpus"`
	TotalGpuHours     float64 `json:"total_gpu_hours"`
	AvgGpuMemoryUsed  float64 `json:"avg_gpu_memory_used"`
	HoursActive       int     `json:"hours_active"`
}

type GpuAggWorkloadSummaryResponse struct {
	Workloads []WorkloadSummaryItem `json:"workloads"`
	Count     int                   `json:"count"`
}

type WorkloadTypeDistItem struct {
	WorkloadType      string  `json:"workload_type"`
	WorkloadCount     int     `json:"workload_count"`
	TotalGpuHours     float64 `json:"total_gpu_hours"`
	AvgUtilization    float64 `json:"avg_utilization"`
	AvgAllocatedGpus  float64 `json:"avg_allocated_gpus"`
}

type GpuAggWorkloadTypeDistResponse struct {
	Distribution []WorkloadTypeDistItem `json:"distribution"`
}

type GpuAggDimensionKeysResponse []string

type GpuAggDimensionValuesResponse []string

// ======================== Handler Implementations ========================

func handleGpuAggClusters(ctx context.Context, req *GpuAggClustersRequest) (*GpuAggClustersResponse, error) {
	cm := clientsets.GetClusterManager()
	clusterNames := cm.GetClusterNames()

	// Filter out "default" cluster
	filteredClusters := make([]string, 0, len(clusterNames))
	for _, name := range clusterNames {
		if name != "default" {
			filteredClusters = append(filteredClusters, name)
		}
	}

	result := GpuAggClustersResponse(filteredClusters)
	return &result, nil
}

func handleGpuAggNamespaces(ctx context.Context, req *GpuAggNamespacesRequest) (*GpuAggNamespacesResponse, error) {
	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Load config for namespace filtering
	cfg, configErr := getGpuAggregationConfig(ctx, clients.ClusterName)
	if configErr != nil {
		log.Warnf("Failed to load GPU aggregation config for namespace filtering: %v", configErr)
	}

	excludeNamespaces := getExcludeNamespacesList(cfg)

	namespaces, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetDistinctNamespacesWithExclusion(ctx, startTime, endTime, excludeNamespaces)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get namespaces", errors.CodeDatabaseError)
	}

	result := GpuAggNamespacesResponse(namespaces)
	return &result, nil
}

func handleGpuAggDimensionKeys(ctx context.Context, req *GpuAggDimensionKeysRequest) (*GpuAggDimensionKeysResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	keys, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetDistinctDimensionKeys(ctx, req.DimensionType, startTime, endTime)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get dimension keys", errors.CodeDatabaseError)
	}

	result := GpuAggDimensionKeysResponse(keys)
	return &result, nil
}

func handleGpuAggDimensionValues(ctx context.Context, req *GpuAggDimensionValuesRequest) (*GpuAggDimensionValuesResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	values, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetDistinctDimensionValues(ctx, req.DimensionType, req.DimensionKey, startTime, endTime)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get dimension values", errors.CodeDatabaseError)
	}

	result := GpuAggDimensionValuesResponse(values)
	return &result, nil
}

func handleGpuAggClusterHourlyStats(ctx context.Context, req *GpuAggClusterHourlyStatsRequest) (*PaginatedResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	result, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetClusterHourlyStatsPaginated(ctx, startTime, endTime, opts)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get cluster hourly stats", errors.CodeDatabaseError)
	}

	return &PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}, nil
}

func handleGpuAggNamespaceHourlyStats(ctx context.Context, req *GpuAggNamespaceHourlyStatsRequest) (*PaginatedResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	cfg, _ := getGpuAggregationConfig(ctx, clients.ClusterName)
	excludeNamespaces := getExcludeNamespacesList(cfg)

	var result *database.PaginatedResult
	facade := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation()

	if req.Namespace != "" {
		result, err = facade.GetNamespaceHourlyStatsPaginated(ctx, req.Namespace, startTime, endTime, opts)
	} else {
		result, err = facade.ListNamespaceHourlyStatsPaginatedWithExclusion(ctx, startTime, endTime, excludeNamespaces, opts)
	}

	if err != nil {
		return nil, errors.WrapError(err, "Failed to get namespace hourly stats", errors.CodeDatabaseError)
	}

	return &PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}, nil
}

func handleGpuAggLabelHourlyStats(ctx context.Context, req *GpuAggLabelHourlyStatsRequest) (*PaginatedResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	var result *database.PaginatedResult
	facade := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation()

	if req.DimensionValue != "" {
		result, err = facade.GetLabelHourlyStatsPaginated(ctx, req.DimensionType,
			req.DimensionKey, req.DimensionValue, startTime, endTime, opts)
	} else {
		result, err = facade.ListLabelHourlyStatsByKeyPaginated(ctx, req.DimensionType,
			req.DimensionKey, startTime, endTime, opts)
	}

	if err != nil {
		return nil, errors.WrapError(err, "Failed to get label hourly stats", errors.CodeDatabaseError)
	}

	return &PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}, nil
}

func handleGpuAggWorkloadHourlyStats(ctx context.Context, req *GpuAggWorkloadHourlyStatsRequest) (*PaginatedResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	cfg, _ := getGpuAggregationConfig(ctx, clients.ClusterName)

	// Check if the requested namespace should be excluded
	if req.Namespace != "" && cfg != nil && shouldExcludeNamespace(req.Namespace, cfg) {
		return &PaginatedResponse{
			Total:      0,
			Page:       1,
			PageSize:   opts.PageSize,
			TotalPages: 0,
			Data:       []*dbmodel.WorkloadGpuHourlyStats{},
		}, nil
	}

	excludeNamespaces := getExcludeNamespacesList(cfg)

	result, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetWorkloadHourlyStatsPaginatedWithExclusion(ctx, req.Namespace, req.WorkloadName, req.WorkloadType, startTime, endTime, excludeNamespaces, opts)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get workload hourly stats", errors.CodeDatabaseError)
	}

	return &PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}, nil
}

func handleGpuAggLatestSnapshot(ctx context.Context, req *GpuAggSnapshotRequest) (*dbmodel.GpuAllocationSnapshots, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	snapshot, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetLatestSnapshot(ctx)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get latest snapshot", errors.CodeDatabaseError)
	}

	if snapshot == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("No snapshot found")
	}

	return snapshot, nil
}

func handleGpuAggSnapshots(ctx context.Context, req *GpuAggSnapshotsListRequest) (*[]*dbmodel.GpuAllocationSnapshots, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Default query last 24 hours
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	if req.StartTime != "" {
		startTime, err = time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
		}
	}

	if req.EndTime != "" {
		endTime, err = time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
		}
	}

	snapshots, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		ListSnapshots(ctx, startTime, endTime)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to list snapshots", errors.CodeDatabaseError)
	}

	return &snapshots, nil
}

func handleGpuAggWorkloadSummary(ctx context.Context, req *GpuAggWorkloadSummaryRequest) (*GpuAggWorkloadSummaryResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	topN := req.TopN
	if topN <= 0 {
		topN = 50
	}
	if topN > 200 {
		topN = 200
	}

	orderCol := "total_gpu_hours"
	switch req.OrderBy {
	case "avg_utilization":
		orderCol = "avg_util"
	case "allocated_gpu_count":
		orderCol = "avg_alloc"
	}
	orderDir := "DESC"
	if req.OrderDirection == "asc" {
		orderDir = "ASC"
	}

	cfg, _ := getGpuAggregationConfig(ctx, clients.ClusterName)
	excludeNamespaces := getExcludeNamespacesList(cfg)

	db := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().(*database.GpuAggregationFacade)
	rawDB := db.GetDB()

	query := rawDB.Table("workload_gpu_hourly_stats").
		Select(`namespace,
			workload_name,
			workload_type,
			AVG(avg_utilization) as avg_util,
			MAX(max_utilization) as max_util,
			AVG(allocated_gpu_count) as avg_alloc,
			SUM(allocated_gpu_count) as total_gpu_hours,
			AVG(avg_gpu_memory_used) as avg_mem,
			COUNT(*) as hours_active`).
		Where("stat_hour >= ? AND stat_hour <= ?", startTime, endTime).
		Group("namespace, workload_name, workload_type")

	if req.Namespace != "" {
		query = query.Where("namespace = ?", req.Namespace)
	}
	if req.WorkloadType != "" {
		query = query.Where("workload_type = ?", req.WorkloadType)
	}
	if len(excludeNamespaces) > 0 {
		query = query.Where("namespace NOT IN ?", excludeNamespaces)
	}

	query = query.Order(orderCol + " " + orderDir).Limit(topN)

	type rawRow struct {
		Namespace    string  `gorm:"column:namespace"`
		WorkloadName string  `gorm:"column:workload_name"`
		WorkloadType string  `gorm:"column:workload_type"`
		AvgUtil      float64 `gorm:"column:avg_util"`
		MaxUtil      float64 `gorm:"column:max_util"`
		AvgAlloc     float64 `gorm:"column:avg_alloc"`
		TotalGpuHrs  float64 `gorm:"column:total_gpu_hours"`
		AvgMem       float64 `gorm:"column:avg_mem"`
		HoursActive  int     `gorm:"column:hours_active"`
	}

	var rows []rawRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, errors.WrapError(err, "Failed to query workload summary", errors.CodeDatabaseError)
	}

	items := make([]WorkloadSummaryItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, WorkloadSummaryItem{
			Namespace:        r.Namespace,
			WorkloadName:     r.WorkloadName,
			WorkloadType:     r.WorkloadType,
			AvgUtilization:   math.Round(r.AvgUtil*100) / 100,
			MaxUtilization:   math.Round(r.MaxUtil*100) / 100,
			AvgAllocatedGpus: math.Round(r.AvgAlloc*100) / 100,
			TotalGpuHours:    math.Round(r.TotalGpuHrs*100) / 100,
			AvgGpuMemoryUsed: math.Round(r.AvgMem*100) / 100,
			HoursActive:      r.HoursActive,
		})
	}

	return &GpuAggWorkloadSummaryResponse{
		Workloads: items,
		Count:     len(items),
	}, nil
}

func handleGpuAggWorkloadTypeDist(ctx context.Context, req *GpuAggWorkloadTypeDistRequest) (*GpuAggWorkloadTypeDistResponse, error) {
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid)
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	cfg, _ := getGpuAggregationConfig(ctx, clients.ClusterName)
	excludeNamespaces := getExcludeNamespacesList(cfg)

	db := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().(*database.GpuAggregationFacade)
	rawDB := db.GetDB()

	query := rawDB.Table("workload_gpu_hourly_stats").
		Select(`workload_type,
			COUNT(DISTINCT workload_name) as wl_count,
			SUM(allocated_gpu_count) as total_gpu_hours,
			AVG(avg_utilization) as avg_util,
			AVG(allocated_gpu_count) as avg_alloc`).
		Where("stat_hour >= ? AND stat_hour <= ?", startTime, endTime).
		Group("workload_type").
		Order("total_gpu_hours DESC")

	if req.Namespace != "" {
		query = query.Where("namespace = ?", req.Namespace)
	}
	if len(excludeNamespaces) > 0 {
		query = query.Where("namespace NOT IN ?", excludeNamespaces)
	}

	type rawRow struct {
		WorkloadType string  `gorm:"column:workload_type"`
		WlCount      int     `gorm:"column:wl_count"`
		TotalGpuHrs  float64 `gorm:"column:total_gpu_hours"`
		AvgUtil      float64 `gorm:"column:avg_util"`
		AvgAlloc     float64 `gorm:"column:avg_alloc"`
	}

	var rows []rawRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, errors.WrapError(err, "Failed to query workload type distribution", errors.CodeDatabaseError)
	}

	items := make([]WorkloadTypeDistItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, WorkloadTypeDistItem{
			WorkloadType:     r.WorkloadType,
			WorkloadCount:    r.WlCount,
			TotalGpuHours:    math.Round(r.TotalGpuHrs*100) / 100,
			AvgUtilization:   math.Round(r.AvgUtil*100) / 100,
			AvgAllocatedGpus: math.Round(r.AvgAlloc*100) / 100,
		})
	}

	return &GpuAggWorkloadTypeDistResponse{
		Distribution: items,
	}, nil
}
