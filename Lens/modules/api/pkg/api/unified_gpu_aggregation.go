// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
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
