// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

const (
	// ConfigKeyGpuAggregation is the configuration key for GPU aggregation
	ConfigKeyGpuAggregation = "job.gpu_aggregation.config"
)

// systemNamespaces defines the list of Kubernetes system namespaces
var systemNamespaces = []string{"kube-system", "kube-public", "kube-node-lease"}

// getGpuAggregationConfig loads GPU aggregation config from config manager
func getGpuAggregationConfig(ctx *gin.Context, clusterName string) (*model.GpuAggregationConfig, error) {
	configManager := config.GetConfigManagerForCluster(clusterName)
	var cfg model.GpuAggregationConfig
	err := configManager.Get(ctx, ConfigKeyGpuAggregation, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// shouldExcludeNamespace determines whether a namespace should be excluded based on config
func shouldExcludeNamespace(namespace string, cfg *model.GpuAggregationConfig) bool {
	if cfg == nil {
		return false
	}

	// Check if in exclusion list
	for _, excluded := range cfg.Dimensions.Namespace.ExcludeNamespaces {
		if namespace == excluded {
			return true
		}
	}

	// Check if it's a system namespace
	if !cfg.Dimensions.Namespace.IncludeSystemNamespaces {
		for _, sysNs := range systemNamespaces {
			if namespace == sysNs {
				return true
			}
		}
	}

	return false
}

// getExcludeNamespacesList returns the list of namespaces to exclude based on config
func getExcludeNamespacesList(cfg *model.GpuAggregationConfig) []string {
	if cfg == nil {
		return nil
	}

	excludeSet := make(map[string]struct{})

	// Add configured exclusion namespaces
	for _, ns := range cfg.Dimensions.Namespace.ExcludeNamespaces {
		excludeSet[ns] = struct{}{}
	}

	// Add system namespaces if not included
	if !cfg.Dimensions.Namespace.IncludeSystemNamespaces {
		for _, sysNs := range systemNamespaces {
			excludeSet[sysNs] = struct{}{}
		}
	}

	// Convert set to slice
	result := make([]string, 0, len(excludeSet))
	for ns := range excludeSet {
		result = append(result, ns)
	}
	return result
}

// ClusterHourlyStatsRequest cluster hourly statistics query request
type ClusterHourlyStatsRequest struct {
	StartTime      string `form:"start_time" binding:"required"`                       // RFC3339 format
	EndTime        string `form:"end_time" binding:"required"`                         // RFC3339 format
	Page           int    `form:"page" binding:"omitempty,min=1"`                      // Page number, starting from 1
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=1000"`        // Items per page, maximum 1000
	OrderBy        string `form:"order_by" binding:"omitempty,oneof=time utilization"` // Sort field: time or utilization
	OrderDirection string `form:"order_direction" binding:"omitempty,oneof=asc desc"`  // Sort direction: asc or desc
}

// NamespaceHourlyStatsRequest namespace hourly statistics query request
type NamespaceHourlyStatsRequest struct {
	Namespace      string `form:"namespace"`                                           // Optional, query all namespaces if empty
	StartTime      string `form:"start_time" binding:"required"`                       // RFC3339 format
	EndTime        string `form:"end_time" binding:"required"`                         // RFC3339 format
	Page           int    `form:"page" binding:"omitempty,min=1"`                      // Page number, starting from 1
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=1000"`        // Items per page, maximum 1000
	OrderBy        string `form:"order_by" binding:"omitempty,oneof=time utilization"` // Sort field: time or utilization
	OrderDirection string `form:"order_direction" binding:"omitempty,oneof=asc desc"`  // Sort direction: asc or desc
}

// LabelHourlyStatsRequest label/annotation hourly statistics query request
type LabelHourlyStatsRequest struct {
	DimensionType  string `form:"dimension_type" binding:"required,oneof=label annotation"`                // label or annotation
	DimensionKey   string `form:"dimension_key" binding:"required"`                                        // label key
	DimensionValue string `form:"dimension_value"`                                                         // Optional, query all values for this key if empty
	StartTime      string `form:"start_time" binding:"required"`                                           // RFC3339 format
	EndTime        string `form:"end_time" binding:"required"`                                             // RFC3339 format
	Page           int    `form:"page" binding:"omitempty,min=1"`                                          // Page number, starting from 1
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=1000"`                            // Items per page, maximum 1000
	OrderBy        string `form:"order_by" binding:"omitempty,oneof=time utilization allocated_gpu_count"` // Sort field: time, utilization or allocated_gpu_count
	OrderDirection string `form:"order_direction" binding:"omitempty,oneof=asc desc"`                      // Sort direction: asc or desc
}

// WorkloadHourlyStatsRequest workload hourly statistics query request
type WorkloadHourlyStatsRequest struct {
	Namespace      string `form:"namespace"`                                                               // Optional, query all namespaces if empty
	WorkloadName   string `form:"workload_name"`                                                           // Optional, query all workloads if empty
	WorkloadType   string `form:"workload_type"`                                                           // Optional, filter by workload type (Job, Deployment, StatefulSet, etc.)
	StartTime      string `form:"start_time" binding:"required"`                                           // RFC3339 format
	EndTime        string `form:"end_time" binding:"required"`                                             // RFC3339 format
	Page           int    `form:"page" binding:"omitempty,min=1"`                                          // Page number, starting from 1
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=1000"`                            // Items per page, maximum 1000
	OrderBy        string `form:"order_by" binding:"omitempty,oneof=time utilization allocated_gpu_count"` // Sort field: time, utilization or allocated_gpu_count
	OrderDirection string `form:"order_direction" binding:"omitempty,oneof=asc desc"`                      // Sort direction: asc or desc
}

// SnapshotsRequest snapshot query request
type SnapshotsRequest struct {
	StartTime string `form:"start_time"` // RFC3339 format, optional
	EndTime   string `form:"end_time"`   // RFC3339 format, optional
}

// PaginatedResponse paginated response
type PaginatedResponse struct {
	Total      int64       `json:"total"`       // Total number of records
	Page       int         `json:"page"`        // Current page number
	PageSize   int         `json:"page_size"`   // Items per page
	TotalPages int         `json:"total_pages"` // Total number of pages
	Data       interface{} `json:"data"`        // Data list
}

// MetadataTimeRangeRequest metadata time range query request
type MetadataTimeRangeRequest struct {
	StartTime string `form:"start_time" binding:"required"` // RFC3339 format
	EndTime   string `form:"end_time" binding:"required"`   // RFC3339 format
}

// DimensionKeysRequest dimension keys query request
type DimensionKeysRequest struct {
	DimensionType string `form:"dimension_type" binding:"required,oneof=label annotation"` // label or annotation
	StartTime     string `form:"start_time" binding:"required"`                            // RFC3339 format
	EndTime       string `form:"end_time" binding:"required"`                              // RFC3339 format
}

// DimensionValuesRequest dimension values query request
type DimensionValuesRequest struct {
	DimensionType string `form:"dimension_type" binding:"required,oneof=label annotation"` // label or annotation
	DimensionKey  string `form:"dimension_key" binding:"required"`                         // dimension key
	StartTime     string `form:"start_time" binding:"required"`                            // RFC3339 format
	EndTime       string `form:"end_time" binding:"required"`                              // RFC3339 format
}

// getClusterHourlyStats queries cluster-level hourly statistics
// @Summary Query cluster GPU hourly statistics
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Param page query int false "Page number, starting from 1"
// @Param page_size query int false "Items per page, default 20, maximum 1000"
// @Param order_by query string false "Sort field (time or utilization)"
// @Param order_direction query string false "Sort direction (asc or desc)"
// @Success 200 {object} rest.Response{data=PaginatedResponse}
// @Router /gpu-aggregation/cluster/hourly-stats [get]
func getClusterHourlyStats(ctx *gin.Context) {
	var req ClusterHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Build pagination options
	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	// Query data
	result, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetClusterHourlyStatsPaginated(ctx, startTime, endTime, opts)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get cluster hourly stats", errors.CodeDatabaseError))
		return
	}

	// Build response
	response := PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, response))
}

// getNamespaceHourlyStats queries namespace-level hourly statistics
// @Summary Query namespace GPU hourly statistics
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param namespace query string false "Namespace name (optional, query all if empty)"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Param page query int false "Page number, starting from 1"
// @Param page_size query int false "Items per page, default 20, maximum 1000"
// @Param order_by query string false "Sort field (time or utilization)"
// @Param order_direction query string false "Sort direction (asc or desc)"
// @Success 200 {object} rest.Response{data=PaginatedResponse}
// @Router /gpu-aggregation/namespaces/hourly-stats [get]
func getNamespaceHourlyStats(ctx *gin.Context) {
	var req NamespaceHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Build pagination options
	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	// Load config for namespace filtering
	cfg, err := getGpuAggregationConfig(ctx, clients.ClusterName)
	if err != nil {
		log.Warnf("Failed to load GPU aggregation config for namespace filtering: %v", err)
		// Continue without filtering if config not found
	}

	// Get exclude namespaces list from config
	excludeNamespaces := getExcludeNamespacesList(cfg)

	// Query data
	var result *database.PaginatedResult
	facade := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation()

	if req.Namespace != "" {
		// Query specific namespace
		result, err = facade.GetNamespaceHourlyStatsPaginated(ctx, req.Namespace, startTime, endTime, opts)
	} else {
		// Query all namespaces with exclusion at database level
		result, err = facade.ListNamespaceHourlyStatsPaginatedWithExclusion(ctx, startTime, endTime, excludeNamespaces, opts)
	}

	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get namespace hourly stats", errors.CodeDatabaseError))
		return
	}

	// Build response
	response := PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, response))
}

// getLabelHourlyStats queries label/annotation-level hourly statistics
// @Summary Query label/annotation GPU hourly statistics
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param dimension_type query string true "Dimension type (label or annotation)"
// @Param dimension_key query string true "Dimension key"
// @Param dimension_value query string false "Dimension value (optional, query all values for this key if empty)"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Param page query int false "Page number, starting from 1"
// @Param page_size query int false "Items per page, default 20, maximum 1000"
// @Param order_by query string false "Sort field (time, utilization or allocated_gpu_count)"
// @Param order_direction query string false "Sort direction (asc or desc)"
// @Success 200 {object} rest.Response{data=PaginatedResponse}
// @Router /gpu-aggregation/labels/hourly-stats [get]
func getLabelHourlyStats(ctx *gin.Context) {
	var req LabelHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Build pagination options
	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	// Query data
	var result *database.PaginatedResult
	facade := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation()

	if req.DimensionValue != "" {
		// Query specific dimension value
		result, err = facade.GetLabelHourlyStatsPaginated(ctx, req.DimensionType,
			req.DimensionKey, req.DimensionValue, startTime, endTime, opts)
	} else {
		// Query all values for this key
		result, err = facade.ListLabelHourlyStatsByKeyPaginated(ctx, req.DimensionType,
			req.DimensionKey, startTime, endTime, opts)
	}

	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get label hourly stats", errors.CodeDatabaseError))
		return
	}

	// Build response
	response := PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, response))
}

// getWorkloadHourlyStats queries workload-level hourly statistics
// @Summary Query workload GPU hourly statistics
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param namespace query string false "Namespace (optional, query all if empty)"
// @Param workload_name query string false "Workload name (optional, query all if empty)"
// @Param workload_type query string false "Workload type (optional, e.g., Job, Deployment, StatefulSet)"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Param page query int false "Page number, starting from 1"
// @Param page_size query int false "Items per page, default 20, maximum 1000"
// @Param order_by query string false "Sort field (time, utilization or allocated_gpu_count)"
// @Param order_direction query string false "Sort direction (asc or desc)"
// @Success 200 {object} rest.Response{data=PaginatedResponse}
// @Router /gpu-aggregation/workloads/hourly-stats [get]
func getWorkloadHourlyStats(ctx *gin.Context) {
	var req WorkloadHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Build pagination options
	opts := database.PaginationOptions{
		Page:           req.Page,
		PageSize:       10000,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	}

	// Load config for namespace filtering
	cfg, configErr := getGpuAggregationConfig(ctx, clients.ClusterName)
	if configErr != nil {
		log.Warnf("Failed to load GPU aggregation config for namespace filtering: %v", configErr)
		// Continue without filtering if config not found
	}

	// Check if the requested namespace should be excluded (when specific namespace is requested)
	if req.Namespace != "" && cfg != nil && shouldExcludeNamespace(req.Namespace, cfg) {
		// Return empty result for excluded namespace
		response := PaginatedResponse{
			Total:      0,
			Page:       1,
			PageSize:   opts.PageSize,
			TotalPages: 0,
			Data:       []*dbmodel.WorkloadGpuHourlyStats{},
		}
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, response))
		return
	}

	// Get exclude namespaces list from config
	excludeNamespaces := getExcludeNamespacesList(cfg)

	// Query data with exclusion at database level
	result, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetWorkloadHourlyStatsPaginatedWithExclusion(ctx, req.Namespace, req.WorkloadName, req.WorkloadType, startTime, endTime, excludeNamespaces, opts)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get workload hourly stats", errors.CodeDatabaseError))
		return
	}

	// Build response
	response := PaginatedResponse{
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		Data:       result.Data,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, response))
}

// getLatestSnapshot gets the latest GPU allocation snapshot
// @Summary Get latest GPU allocation snapshot
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Success 200 {object} rest.Response{data=dbmodel.GpuAllocationSnapshots}
// @Router /gpu-aggregation/snapshots/latest [get]
func getLatestSnapshot(ctx *gin.Context) {
	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Query latest snapshot
	snapshot, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetLatestSnapshot(ctx)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get latest snapshot", errors.CodeDatabaseError))
		return
	}

	if snapshot == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("No snapshot found"))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, snapshot))
}

// listSnapshots queries historical snapshot list
// @Summary Query GPU allocation snapshot history
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param start_time query string false "Start time (RFC3339 format, optional)"
// @Param end_time query string false "End time (RFC3339 format, optional)"
// @Success 200 {object} rest.Response{data=[]dbmodel.GpuAllocationSnapshots}
// @Router /gpu-aggregation/snapshots [get]
func listSnapshots(ctx *gin.Context) {
	var req SnapshotsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Default query last 24 hours
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	// If time parameters are provided, use them
	if req.StartTime != "" {
		startTime, err = time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
			return
		}
	}

	if req.EndTime != "" {
		endTime, err = time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
			return
		}
	}

	// Query snapshot list
	snapshots, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		ListSnapshots(ctx, startTime, endTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to list snapshots", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, snapshots))
}

// getClusters gets all cluster list
// @Summary Get cluster list
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Success 200 {object} rest.Response{data=[]string}
// @Router /gpu-aggregation/clusters [get]
func getClusters(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterNames := cm.GetClusterNames()

	// Filter out "default" cluster
	filteredClusters := make([]string, 0, len(clusterNames))
	for _, name := range clusterNames {
		if name != "default" {
			filteredClusters = append(filteredClusters, name)
		}
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, filteredClusters))
}

// getNamespaces gets namespace list within specified time range
// @Summary Get namespace list
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Success 200 {object} rest.Response{data=[]string}
// @Router /gpu-aggregation/namespaces [get]
func getNamespaces(ctx *gin.Context) {
	var req MetadataTimeRangeRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Load config for namespace filtering
	cfg, configErr := getGpuAggregationConfig(ctx, clients.ClusterName)
	if configErr != nil {
		log.Warnf("Failed to load GPU aggregation config for namespace filtering: %v", configErr)
		// Continue without filtering if config not found
	}

	// Get exclude namespaces list from config
	excludeNamespaces := getExcludeNamespacesList(cfg)

	// Query data with exclusion at database level
	namespaces, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetDistinctNamespacesWithExclusion(ctx, startTime, endTime, excludeNamespaces)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get namespaces", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, namespaces))
}

// getDimensionKeys gets dimension keys list within specified time range
// @Summary Get label/annotation key list
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param dimension_type query string true "Dimension type (label or annotation)"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Success 200 {object} rest.Response{data=[]string}
// @Router /gpu-aggregation/dimension-keys [get]
func getDimensionKeys(ctx *gin.Context) {
	var req DimensionKeysRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Query data
	keys, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetDistinctDimensionKeys(ctx, req.DimensionType, startTime, endTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get dimension keys", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, keys))
}

// getDimensionValues gets values list for a dimension key within specified time range
// @Summary Get label/annotation value list
// @Tags GPU Aggregation
// @Accept json
// @Produce json
// @Param cluster query string false "Cluster name"
// @Param dimension_type query string true "Dimension type (label or annotation)"
// @Param dimension_key query string true "Dimension key"
// @Param start_time query string true "Start time (RFC3339 format)"
// @Param end_time query string true "End time (RFC3339 format)"
// @Success 200 {object} rest.Response{data=[]string}
// @Router /gpu-aggregation/dimension-values [get]
func getDimensionValues(ctx *gin.Context) {
	var req DimensionValuesRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// Parse time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// Get cluster client
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Query data
	values, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetDistinctDimensionValues(ctx, req.DimensionType, req.DimensionKey, startTime, endTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get dimension values", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, values))
}
