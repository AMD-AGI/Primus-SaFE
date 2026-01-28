// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for workload operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ===== Workload List =====

// WorkloadListRequest represents the request for workload list.
type WorkloadListRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name      string `json:"name" query:"name" mcp:"name,description=Filter by workload name (partial match)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Filter by workload kind (Job/PyTorchJob/etc)"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Filter by namespace"`
	Status    string `json:"status" query:"status" mcp:"status,description=Filter by status (Running/Completed/Failed)"`
	PageNum   int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize  int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 10)"`
	OrderBy   string `json:"order_by" query:"order_by" mcp:"order_by,description=Order by field (start_at/end_at)"`
}

// WorkloadListResponse represents the workload list response.
type WorkloadListResponse struct {
	Data  []model.WorkloadListItem `json:"data"`
	Total int                      `json:"total"`
}

// ===== Workload Detail =====

// WorkloadDetailRequest represents the request for workload detail.
type WorkloadDetailRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
}

// WorkloadDetailResponse is model.WorkloadInfo for backward compatibility.
type WorkloadDetailResponse = model.WorkloadInfo

// ===== Phase 5: Workload Statistics =====

// WorkloadStatisticsRequest represents the request for workload statistics.
type WorkloadStatisticsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// WorkloadStatisticsResponse is model.WorkloadStatisticResp for backward compatibility.
type WorkloadStatisticsResponse = model.WorkloadStatisticResp

// ===== Phase 5: Workload Hierarchy Query =====

// WorkloadHierarchyQueryRequest represents the request for workload hierarchy by kind/name.
type WorkloadHierarchyQueryRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Workload kind (e.g. Job/PyTorchJob),required"`
	Name      string `json:"name" query:"name" mcp:"name,description=Workload name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Workload namespace (optional)"`
}

// WorkloadHierarchyResponse is model.WorkloadHierarchyItem for backward compatibility.
type WorkloadHierarchyResponse = model.WorkloadHierarchyItem

// ===== Phase 5: Workload GPU Utilization History =====

// WorkloadGPUHistoryRequest represents the request for workload GPU utilization history.
type WorkloadGPUHistoryRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Workload kind,required"`
	Name      string `json:"name" query:"name" mcp:"name,description=Workload name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Workload namespace (optional)"`
	Start     string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End       string `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step      string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// WorkloadGPUHistoryResponse is the utilization history data.
type WorkloadGPUHistoryResponse = model.MetricsGraph

// ===== Phase 6: Workload Hierarchy by UID =====

// WorkloadHierarchyByUIDRequest represents the request for workload hierarchy by UID.
type WorkloadHierarchyByUIDRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
}

// ===== Phase 6: Workload Metrics =====

// WorkloadMetricsRequest represents the request for workload metrics (GPU util, memory, power).
type WorkloadMetricsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	Start   string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End     string `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step    string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// WorkloadMetricsResponse represents the workload metrics response.
type WorkloadMetricsResponse map[string]model.MetricsGraph

// ===== Phase 6: Training Performance =====

// TrainingPerformanceRequest represents the request for training performance metrics.
type TrainingPerformanceRequest struct {
	UID   string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	Start string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix milliseconds),required"`
	End   string `json:"end" query:"end" mcp:"end,description=End timestamp (unix milliseconds),required"`
}

// TrainingPerformanceResponse is a list of training performance data points.
type TrainingPerformanceResponse []GrafanaMetricsPoint

// ===== Phase 6: Data Sources =====

// WorkloadDataSourcesRequest represents the request for workload data sources.
type WorkloadDataSourcesRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
}

// WorkloadDataSourcesResponse represents data sources available for a workload.
type WorkloadDataSourcesResponse = DataSourcesResponse

// ===== Phase 6: Available Metrics =====

// WorkloadAvailableMetricsRequest represents the request for available metrics.
type WorkloadAvailableMetricsRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID        string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter by data source (log/wandb/tensorflow)"`
}

// WorkloadAvailableMetricsResponse represents available metrics for a workload.
type WorkloadAvailableMetricsResponse = AvailableMetricsResponse

// ===== Phase 6: Metrics Data =====

// WorkloadMetricsDataRequest represents the request for metrics data.
type WorkloadMetricsDataRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID        string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter by data source (log/wandb/tensorflow)"`
	Metrics    string `json:"metrics" query:"metrics" mcp:"metrics,description=Comma-separated metric names or 'all' (default: all)"`
	Start      string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix milliseconds)"`
	End        string `json:"end" query:"end" mcp:"end,description=End timestamp (unix milliseconds)"`
}

// WorkloadMetricsDataResponse represents metrics data for a workload.
type WorkloadMetricsDataResponse = MetricsDataResponse

// ===== Phase 6: Iteration Times =====

// WorkloadIterationTimesRequest represents the request for iteration times.
type WorkloadIterationTimesRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	UID        string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter by data source (log/wandb/tensorflow)"`
	Start      string `json:"start" query:"start" mcp:"start,description=Start timestamp (unix milliseconds)"`
	End        string `json:"end" query:"end" mcp:"end,description=End timestamp (unix milliseconds)"`
}

// WorkloadIterationTimesResponse represents iteration time data.
type WorkloadIterationTimesResponse = MetricsDataResponse

// ===== Phase 6: Workload Metadata =====

// WorkloadMetadataRequest represents the request for workload metadata.
type WorkloadMetadataRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// WorkloadMetadataResponse represents workload metadata (namespaces and kinds).
type WorkloadMetadataResponse struct {
	Namespaces []string `json:"namespaces"`
	Kinds      []string `json:"kinds"`
}

// ===== Register Workload Endpoints =====

func init() {
	// Register workload list endpoint - replaces listWorkloads
	unified.Register(&unified.EndpointDef[WorkloadListRequest, WorkloadListResponse]{
		Name:        "workload_list",
		Description: "List GPU workloads in the cluster with filtering by name, kind, namespace, status. Returns workload metadata, GPU allocation, and utilization statistics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads",
		MCPToolName: "lens_workload_list",
		Handler:     handleWorkloadList,
	})

	// Register workload detail endpoint - replaces getWorkloadInfo
	unified.Register(&unified.EndpointDef[WorkloadDetailRequest, WorkloadDetailResponse]{
		Name:        "workload_detail",
		Description: "Get detailed information about a specific workload by UID including pods, GPU allocation per node, start/end times, and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid",
		MCPToolName: "lens_workload_detail",
		Handler:     handleWorkloadDetail,
	})

	// Phase 5: Register workload statistics endpoint - replaces getWorkloadsStatistic
	unified.Register(&unified.EndpointDef[WorkloadStatisticsRequest, WorkloadStatisticsResponse]{
		Name:        "workload_statistics",
		Description: "Get statistics about running workloads including count, average GPU allocation, average GPU utilization, and number of low utilization workloads.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/statistic",
		MCPToolName: "lens_workload_statistics",
		Handler:     handleWorkloadStatistics,
	})

	// Phase 5: Register workload hierarchy query endpoint - replaces getWorkloadHierarchyByKindName
	unified.Register(&unified.EndpointDef[WorkloadHierarchyQueryRequest, WorkloadHierarchyResponse]{
		Name:        "workload_hierarchy_query",
		Description: "Get the workload hierarchy tree by kind and name. Shows parent-child relationships between workload resources.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/hierarchy",
		MCPToolName: "lens_workload_hierarchy_query",
		Handler:     handleWorkloadHierarchyQuery,
	})

	// Phase 5: Register workload GPU history endpoint - replaces getWorkloadGpuUtilizationHistoryByKindName
	unified.Register(&unified.EndpointDef[WorkloadGPUHistoryRequest, WorkloadGPUHistoryResponse]{
		Name:        "workload_gpu_history",
		Description: "Get GPU utilization history for a workload by kind and name. Returns time series data for GPU utilization over the specified time range.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/gpuUtilizationHistory",
		MCPToolName: "lens_workload_gpu_history",
		Handler:     handleWorkloadGPUHistory,
	})

	// Phase 6: Register workload hierarchy by UID endpoint - replaces getWorkloadHierarchy
	unified.Register(&unified.EndpointDef[WorkloadHierarchyByUIDRequest, WorkloadHierarchyResponse]{
		Name:        "workload_hierarchy",
		Description: "Get the workload hierarchy tree by UID. Shows parent-child relationships between workload resources.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/hierarchy",
		MCPToolName: "lens_workload_hierarchy",
		Handler:     handleWorkloadHierarchyByUID,
	})

	// Phase 6: Register workload metrics endpoint - replaces getWorkloadMetrics
	unified.Register(&unified.EndpointDef[WorkloadMetricsRequest, WorkloadMetricsResponse]{
		Name:        "workload_metrics",
		Description: "Get GPU utilization, memory utilization, power metrics, and training performance for a workload over time.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics",
		MCPToolName: "lens_workload_metrics",
		Handler:     handleWorkloadMetrics,
	})

	// Phase 6: Register training performance endpoint - replaces GetWorkloadTrainingPerformance
	unified.Register(&unified.EndpointDef[TrainingPerformanceRequest, TrainingPerformanceResponse]{
		Name:        "training_performance",
		Description: "Get training performance metrics (TFLOPS, loss, etc.) for a workload. Returns time series data suitable for Grafana visualization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/trainingPerformance",
		MCPToolName: "lens_training_performance",
		Handler:     handleTrainingPerformance,
	})

	// Phase 6: Register data sources endpoint - replaces GetDataSources
	unified.Register(&unified.EndpointDef[WorkloadDataSourcesRequest, WorkloadDataSourcesResponse]{
		Name:        "workload_data_sources",
		Description: "Get all available data sources (log, wandb, tensorflow) for a workload's training metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/sources",
		MCPToolName: "lens_workload_data_sources",
		Handler:     handleWorkloadDataSources,
	})

	// Phase 6: Register available metrics endpoint - replaces GetAvailableMetrics
	unified.Register(&unified.EndpointDef[WorkloadAvailableMetricsRequest, WorkloadAvailableMetricsResponse]{
		Name:        "workload_available_metrics",
		Description: "Get all available training metrics for a workload, optionally filtered by data source.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/available",
		MCPToolName: "lens_workload_available_metrics",
		Handler:     handleWorkloadAvailableMetrics,
	})

	// Phase 6: Register metrics data endpoint - replaces GetMetricsData
	unified.Register(&unified.EndpointDef[WorkloadMetricsDataRequest, WorkloadMetricsDataResponse]{
		Name:        "workload_metrics_data",
		Description: "Get training metrics data for a workload with optional filtering by data source, metrics, and time range.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/data",
		MCPToolName: "lens_workload_metrics_data",
		Handler:     handleWorkloadMetricsData,
	})

	// Phase 6: Register iteration times endpoint - replaces GetIterationTimes
	unified.Register(&unified.EndpointDef[WorkloadIterationTimesRequest, WorkloadIterationTimesResponse]{
		Name:        "workload_iteration_times",
		Description: "Get iteration/step timing data for a workload's training run.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloads/:uid/metrics/iteration-times",
		MCPToolName: "lens_workload_iteration_times",
		Handler:     handleWorkloadIterationTimes,
	})

	// Phase 6: Register workload metadata endpoint - replaces getWorkloadsMetadata
	unified.Register(&unified.EndpointDef[WorkloadMetadataRequest, WorkloadMetadataResponse]{
		Name:        "workload_metadata",
		Description: "Get workload metadata including available namespaces and workload kinds in the cluster.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workloadMetadata",
		MCPToolName: "lens_workload_metadata",
		Handler:     handleWorkloadMetadata,
	})
}

// ===== Handler Implementations =====

// handleWorkloadList handles workload list requests.
// Reuses: database.GetWorkload().QueryWorkload, cvtDBWorkloadListItem
func handleWorkloadList(ctx context.Context, req *WorkloadListRequest) (*WorkloadListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	// Build filter - reusing existing filter structure
	emptyParentUid := ""
	f := &filter.WorkloadFilter{
		Limit:     pageSize,
		Offset:    (pageNum - 1) * pageSize,
		ParentUid: &emptyParentUid,
	}
	if req.Name != "" {
		f.Name = &req.Name
	}
	if req.Kind != "" {
		f.Kind = &req.Kind
	}
	if req.Namespace != "" {
		f.Namespace = &req.Namespace
	}
	if req.Status != "" {
		f.Status = &req.Status
	}
	if req.OrderBy != "" {
		switch req.OrderBy {
		case "start_at":
			f.OrderBy = "created_at"
		case "end_at":
			f.OrderBy = "end_at"
		}
	}

	// Reuse existing database query
	workloads, count, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		return nil, err
	}

	// Reuse existing conversion function
	result := []model.WorkloadListItem{}
	for _, w := range workloads {
		item, _ := cvtDBWorkloadListItem(ctx, clients.ClusterName, w)
		result = append(result, item)
	}

	return &WorkloadListResponse{
		Data:  result,
		Total: count,
	}, nil
}

// handleWorkloadDetail handles workload detail requests.
// Reuses: database.GetWorkload().GetGpuWorkloadByUid, workload.GetWorkloadPods, workload.GetWorkloadResource
func handleWorkloadDetail(ctx context.Context, req *WorkloadDetailRequest) (*WorkloadDetailResponse, error) {
	// DEBUG: Log request parameters
	log.Infof("[DEBUG-WorkloadDetail] Request received: UID=%s, Cluster=%s", req.UID, req.Cluster)

	cm := clientsets.GetClusterManager()

	// DEBUG: Log ClusterManager state
	clusterNames := cm.GetClusterNames()
	log.Infof("[DEBUG-WorkloadDetail] ClusterManager state: available_clusters=%v, requested_cluster=%s", clusterNames, req.Cluster)

	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		log.Errorf("[DEBUG-WorkloadDetail] GetClusterClientsOrDefault failed: cluster=%s, error=%v", req.Cluster, err)
		return nil, err
	}

	// DEBUG: Log resolved cluster
	log.Infof("[DEBUG-WorkloadDetail] Resolved cluster: requested=%s, resolved=%s, has_storage=%v",
		req.Cluster, clients.ClusterName, clients.StorageClientSet != nil)

	// Reuse existing database query
	log.Infof("[DEBUG-WorkloadDetail] Querying database for workload: uid=%s, cluster=%s", req.UID, clients.ClusterName)
	dbWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, req.UID)
	if err != nil {
		log.Errorf("[DEBUG-WorkloadDetail] Database query error: uid=%s, cluster=%s, error=%v", req.UID, clients.ClusterName, err)
		return nil, err
	}
	if dbWorkload == nil {
		log.Warnf("[DEBUG-WorkloadDetail] Workload not found in database: uid=%s, cluster=%s", req.UID, clients.ClusterName)
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

	// DEBUG: Log found workload
	log.Infof("[DEBUG-WorkloadDetail] Workload found: uid=%s, name=%s, kind=%s, namespace=%s",
		dbWorkload.UID, dbWorkload.Name, dbWorkload.Kind, dbWorkload.Namespace)

	workloadInfo := &model.WorkloadInfo{
		ApiVersion:    dbWorkload.GroupVersion,
		Kind:          dbWorkload.Kind,
		Name:          dbWorkload.Name,
		Namespace:     dbWorkload.Namespace,
		Uid:           dbWorkload.UID,
		GpuAllocation: nil,
		Pods:          []model.WorkloadInfoPod{},
		ActivePods:    []model.WorkloadInfoPod{},
		StartTime:     dbWorkload.CreatedAt.Unix(),
		EndTime:       dbWorkload.EndAt.Unix(),
		Source:        getSource(dbWorkload),
	}
	if dbWorkload.EndAt.Unix() < int64(8*time.Hour) {
		workloadInfo.EndTime = 0
	}

	// Reuse existing helper to get pods
	pods, err := workload.GetWorkloadPods(ctx, clients.ClusterName, dbWorkload.UID)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		podInfo := model.WorkloadInfoPod{
			PodUID:       pod.UID,
			PodNamespace: pod.Namespace,
			PodName:      pod.Name,
			NodeName:     pod.NodeName,
			Phase:        pod.Phase,
			Running:      pod.Running,
			IP:           pod.IP,
			GpuAllocated: int(pod.GpuAllocated),
			CreatedAt:    pod.CreatedAt.Unix(),
			UpdatedAt:    pod.UpdatedAt.Unix(),
		}
		workloadInfo.Pods = append(workloadInfo.Pods, podInfo)
		if pod.Running && !pod.Deleted {
			workloadInfo.ActivePods = append(workloadInfo.ActivePods, podInfo)
		}
	}

	// Reuse existing helper to get GPU allocation
	gpuAllocation, err := workload.GetWorkloadResource(ctx, clients.ClusterName, dbWorkload.UID)
	if err == nil {
		workloadInfo.GpuAllocation = gpuAllocation
	}

	return workloadInfo, nil
}

// ===== Phase 5 Handler Implementations =====

// handleWorkloadStatistics handles workload statistics requests.
// Reuses: database.GetWorkload().GetWorkloadNotEnd, database.GetWorkloadStatistic().GetByUID
func handleWorkloadStatistics(ctx context.Context, req *WorkloadStatisticsRequest) (*WorkloadStatisticsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get all workloads that have not ended
	workloadFacade := database.GetFacadeForCluster(clients.ClusterName).GetWorkload()
	allWorkloads, err := workloadFacade.GetWorkloadNotEnd(ctx)
	if err != nil {
		return nil, err
	}

	// Filter out top-level (ParentUID is empty) and Running status workloads
	runningWorkloads := make([]*dbModel.GpuWorkload, 0)
	for _, w := range allWorkloads {
		if w.ParentUID == "" && w.Status == metadata.WorkloadStatusRunning {
			runningWorkloads = append(runningWorkloads, w)
		}
	}

	resp := &model.WorkloadStatisticResp{
		RunningWorkloadsCount:        len(runningWorkloads),
		AvgGpuAllocated:              0,
		AvgGpuUtilization:            0,
		LowUtilizationWorkloadsCount: 0,
	}

	if len(runningWorkloads) == 0 {
		return resp, nil
	}

	// Calculate average gpu_allocated
	totalGpuAllocated := int64(0)
	for _, w := range runningWorkloads {
		totalGpuAllocated += int64(w.GpuRequest)
	}
	resp.AvgGpuAllocated = float64(totalGpuAllocated) / float64(len(runningWorkloads))

	// Get utilization data from workload_statistic table
	statisticFacade := database.GetFacadeForCluster(clients.ClusterName).GetWorkloadStatistic()
	totalUtilization := 0.0
	utilizationCount := 0
	lowUtilizationCount := 0

	for _, w := range runningWorkloads {
		statistic, err := statisticFacade.GetByUID(ctx, w.UID)
		if err != nil {
			log.Warnf("Failed to get statistic for workload %s: %v", w.UID, err)
			continue
		}
		if statistic == nil {
			continue
		}

		totalUtilization += statistic.InstantGpuUtilization
		utilizationCount++

		if statistic.AvgGpuUtilization < 30.0 {
			lowUtilizationCount++
		}
	}

	if utilizationCount > 0 {
		resp.AvgGpuUtilization = totalUtilization / float64(utilizationCount)
	}
	resp.LowUtilizationWorkloadsCount = lowUtilizationCount

	return resp, nil
}

// handleWorkloadHierarchyQuery handles workload hierarchy query requests.
// Reuses: database.GetWorkload().QueryWorkload, buildHierarchy
func handleWorkloadHierarchyQuery(ctx context.Context, req *WorkloadHierarchyQueryRequest) (*WorkloadHierarchyResponse, error) {
	// DEBUG: Log request parameters
	log.Infof("[DEBUG-WorkloadHierarchy] Request received: Kind=%s, Name=%s, Namespace=%s, Cluster=%s",
		req.Kind, req.Name, req.Namespace, req.Cluster)

	cm := clientsets.GetClusterManager()

	// DEBUG: Log ClusterManager state
	clusterNames := cm.GetClusterNames()
	log.Infof("[DEBUG-WorkloadHierarchy] ClusterManager state: available_clusters=%v, requested_cluster=%s", clusterNames, req.Cluster)

	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		log.Errorf("[DEBUG-WorkloadHierarchy] GetClusterClientsOrDefault failed: cluster=%s, error=%v", req.Cluster, err)
		return nil, err
	}

	// DEBUG: Log resolved cluster
	log.Infof("[DEBUG-WorkloadHierarchy] Resolved cluster: requested=%s, resolved=%s, has_storage=%v",
		req.Cluster, clients.ClusterName, clients.StorageClientSet != nil)

	if req.Kind == "" || req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("kind and name are required")
	}

	// Build filter to find workload by kind and name
	f := &filter.WorkloadFilter{
		Kind: &req.Kind,
		Name: &req.Name,
	}
	if req.Namespace != "" {
		f.Namespace = &req.Namespace
	}

	// Query workload by kind and name
	log.Infof("[DEBUG-WorkloadHierarchy] Querying database: kind=%s, name=%s, namespace=%s, cluster=%s",
		req.Kind, req.Name, req.Namespace, clients.ClusterName)
	workloads, _, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		log.Errorf("[DEBUG-WorkloadHierarchy] Database query error: kind=%s, name=%s, cluster=%s, error=%v",
			req.Kind, req.Name, clients.ClusterName, err)
		return nil, err
	}

	if len(workloads) == 0 {
		log.Warnf("[DEBUG-WorkloadHierarchy] Workload not found: kind=%s, name=%s, cluster=%s", req.Kind, req.Name, clients.ClusterName)
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

	// DEBUG: Log found workloads
	log.Infof("[DEBUG-WorkloadHierarchy] Found %d workloads, using first: uid=%s, name=%s",
		len(workloads), workloads[0].UID, workloads[0].Name)

	// Use the first matched workload
	rootWorkload := workloads[0]

	// Build hierarchy tree - reuse existing helper
	tree, err := buildHierarchy(ctx, clients.ClusterName, rootWorkload.UID)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// handleWorkloadGPUHistory handles workload GPU utilization history requests.
// Reuses: database.GetWorkload().QueryWorkload, workload.GetWorkloadGpuUtilizationHistory
func handleWorkloadGPUHistory(ctx context.Context, req *WorkloadGPUHistoryRequest) (*WorkloadGPUHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.Kind == "" || req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("kind and name are required")
	}

	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	// Parse timestamps
	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value, must be positive integer")
		}
	}

	// Build filter to find workload by kind and name
	f := &filter.WorkloadFilter{
		Kind: &req.Kind,
		Name: &req.Name,
	}
	if req.Namespace != "" {
		f.Namespace = &req.Namespace
	}

	// Query workload by kind and name
	workloads, _, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		return nil, err
	}

	if len(workloads) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

	dbWorkload := workloads[0]

	// Query GPU utilization history - reuse existing helper
	storageClient := clients.StorageClientSet
	// Check if storage client is available (needed for Prometheus queries)
	if storageClient == nil {
		log.Warnf("[WorkloadGPUHistory] Cluster '%s' has no storage configuration, falling back to current cluster",
			clients.ClusterName)
		currentClients := cm.GetCurrentClusterClients()
		if currentClients == nil || currentClients.StorageClientSet == nil {
			return nil, errors.NewError().WithCode(errors.CodeInternalError).
				WithMessage("No storage configuration available for metrics query")
		}
		storageClient = currentClients.StorageClientSet
	}

	gpuUtilHistory, err := workload.GetWorkloadGpuUtilizationHistory(ctx, dbWorkload.UID, startTime, endTime, step, storageClient)
	if err != nil {
		return nil, err
	}

	return gpuUtilHistory, nil
}

// ===== Phase 6 Handler Implementations =====

// handleWorkloadHierarchyByUID handles workload hierarchy by UID requests.
// Reuses: database.GetWorkload().GetGpuWorkloadByUid, buildHierarchy
func handleWorkloadHierarchyByUID(ctx context.Context, req *WorkloadHierarchyByUIDRequest) (*WorkloadHierarchyResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Verify workload exists
	rootWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, req.UID)
	if err != nil {
		return nil, err
	}
	if rootWorkload == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

	// Reuse existing buildHierarchy function
	tree, err := buildHierarchy(ctx, clients.ClusterName, req.UID)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// handleWorkloadMetrics handles workload metrics requests (GPU util, memory, power, TFLOPS).
// Reuses: workload.GetWorkloadGpuUtilMetrics, workload.GetWorkloadGpuMemoryUtilMetrics,
//
//	workload.GetWorkloadGpuPowerMetrics, workload.GetTFLOPSMetrics
func handleWorkloadMetrics(ctx context.Context, req *WorkloadMetricsRequest) (*WorkloadMetricsResponse, error) {
	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value")
		}
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}
	storageClient := clients.StorageClientSet

	// Check if storage client is available (needed for Prometheus queries)
	if storageClient == nil {
		log.Warnf("[WorkloadMetrics] Cluster '%s' has no storage configuration, falling back to current cluster",
			clients.ClusterName)
		currentClients := cm.GetCurrentClusterClients()
		if currentClients == nil || currentClients.StorageClientSet == nil {
			return nil, errors.NewError().WithCode(errors.CodeInternalError).
				WithMessage("No storage configuration available for metrics query")
		}
		storageClient = currentClients.StorageClientSet
	}

	result := make(WorkloadMetricsResponse)

	// GPU Utilization
	gpuUtil, err := workload.GetWorkloadGpuUtilMetrics(ctx, req.UID, startTime, endTime, step, storageClient)
	if err != nil {
		return nil, err
	}
	gpuUtil.Serial = 1
	result["GPU Utilization"] = *gpuUtil

	// GPU Memory Utilization
	gpuMemUtil, err := workload.GetWorkloadGpuMemoryUtilMetrics(ctx, req.UID, startTime, endTime, step, storageClient)
	if err != nil {
		return nil, err
	}
	gpuMemUtil.Serial = 2
	result["GPU Memory Utilization"] = *gpuMemUtil

	// GPU Power
	powerUtil, err := workload.GetWorkloadGpuPowerMetrics(ctx, req.UID, startTime, endTime, step, storageClient)
	if err != nil {
		return nil, err
	}
	powerUtil.Serial = 3
	result["GPU Power"] = *powerUtil

	// Training Performance (TFLOPS)
	tflopsMetrics, err := workload.GetTFLOPSMetrics(ctx, req.UID, startTime, endTime, step, storageClient)
	if err != nil {
		return nil, err
	}
	if tflopsMetrics != nil {
		tflopsMetrics.Serial = 4
		result["TrainingPerformance"] = *tflopsMetrics
	}

	return &result, nil
}

// handleTrainingPerformance handles training performance requests.
// Reuses: database.GetTraining().ListTrainingPerformanceByWorkloadIdsAndTimeRange
func handleTrainingPerformance(ctx context.Context, req *TrainingPerformanceRequest) (*TrainingPerformanceResponse, error) {
	if req.Start == "" || req.End == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("start and end timestamps are required")
	}

	startMs, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start time format")
	}
	endMs, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end time format")
	}

	startTime := time.UnixMilli(startMs)
	endTime := time.UnixMilli(endMs)

	performances, err := database.GetFacade().GetTraining().ListTrainingPerformanceByWorkloadIdsAndTimeRange(
		ctx, []string{req.UID}, startTime, endTime,
	)
	if err != nil {
		return nil, err
	}

	series := map[string]*model.GrafanaMetricsSeries{}
	for _, p := range performances {
		for key, value := range p.Performance {
			valueFloat := convertToFloat(value)
			if isNaN(valueFloat) {
				continue
			}
			if _, ok := series[key]; !ok {
				series[key] = &model.GrafanaMetricsSeries{
					Name:   key,
					Points: [][2]float64{},
				}
			}
			series[key].Points = append(series[key].Points,
				[2]float64{valueFloat, float64(p.CreatedAt.UnixMilli())})
		}
	}

	// Flatten to rows for Infinity
	result := make(TrainingPerformanceResponse, 0)
	for name, s := range series {
		for _, pt := range s.Points {
			result = append(result, GrafanaMetricsPoint{
				Metric:    name,
				Value:     pt[0],
				Timestamp: int64(pt[1]),
			})
		}
	}

	return &result, nil
}

// handleWorkloadDataSources handles data sources requests.
// Reuses: database.GetTraining().ListTrainingPerformanceByWorkloadUID
func handleWorkloadDataSources(ctx context.Context, req *WorkloadDataSourcesRequest) (*WorkloadDataSourcesResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	performances, err := database.GetFacadeForCluster(clients.ClusterName).GetTraining().
		ListTrainingPerformanceByWorkloadUID(ctx, req.UID)
	if err != nil {
		return nil, err
	}

	// Count data sources
	sourceMap := make(map[string]int)
	for _, p := range performances {
		sourceMap[p.DataSource]++
	}

	dataSources := make([]DataSourceInfo, 0, len(sourceMap))
	for source, count := range sourceMap {
		dataSources = append(dataSources, DataSourceInfo{
			Name:  source,
			Count: count,
		})
	}

	return &DataSourcesResponse{
		WorkloadUID: req.UID,
		DataSources: dataSources,
		TotalCount:  len(dataSources),
	}, nil
}

// handleWorkloadAvailableMetrics handles available metrics requests.
// Reuses: database.GetTraining().ListTrainingPerformanceByWorkloadUID/ByWorkloadUIDAndDataSource
func handleWorkloadAvailableMetrics(ctx context.Context, req *WorkloadAvailableMetricsRequest) (*WorkloadAvailableMetricsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	var performances []*dbModel.TrainingPerformance
	if req.DataSource != "" {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, req.UID, req.DataSource)
	} else {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUID(ctx, req.UID)
	}
	if err != nil {
		return nil, err
	}

	// Count all available metrics
	metricMap := make(map[string]map[string]int) // metric_name -> {data_source -> count}
	for _, p := range performances {
		for metricName := range p.Performance {
			if commonMetadataFields[metricName] {
				continue
			}
			if !isMetricField(metricName, p.DataSource) {
				continue
			}
			if _, exists := metricMap[metricName]; !exists {
				metricMap[metricName] = make(map[string]int)
			}
			metricMap[metricName][p.DataSource]++
		}
	}

	metrics := make([]MetricInfo, 0, len(metricMap))
	for metricName, sources := range metricMap {
		sourceList := make([]string, 0, len(sources))
		totalCount := 0
		for source, count := range sources {
			sourceList = append(sourceList, source)
			totalCount += count
		}
		metrics = append(metrics, MetricInfo{
			Name:       metricName,
			DataSource: sourceList,
			Count:      totalCount,
		})
	}

	return &AvailableMetricsResponse{
		WorkloadUID: req.UID,
		Metrics:     metrics,
		TotalCount:  len(metrics),
	}, nil
}

// handleWorkloadMetricsData handles metrics data requests.
// Reuses: database.GetTraining().ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange
func handleWorkloadMetricsData(ctx context.Context, req *WorkloadMetricsDataRequest) (*WorkloadMetricsDataResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Parse time range
	var startTime, endTime time.Time
	var hasTimeRange bool
	if req.Start != "" && req.End != "" {
		startMs, err := strconv.ParseInt(req.Start, 10, 64)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start time format")
		}
		endMs, err := strconv.ParseInt(req.End, 10, 64)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end time format")
		}
		startTime = time.UnixMilli(startMs)
		endTime = time.UnixMilli(endMs)
		hasTimeRange = true
	}

	var performances []*dbModel.TrainingPerformance
	if hasTimeRange {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(ctx, req.UID, req.DataSource, startTime, endTime)
	} else {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, req.UID, req.DataSource)
	}
	if err != nil {
		return nil, err
	}

	// Parse metrics filter
	returnAllMetrics := true
	metricsSet := make(map[string]bool)
	if req.Metrics != "" && req.Metrics != "all" {
		metricsStr := req.Metrics
		if len(metricsStr) > 2 && metricsStr[0] == '{' && metricsStr[len(metricsStr)-1] == '}' {
			metricsStr = metricsStr[1 : len(metricsStr)-1]
		}
		for _, m := range splitMetricsForUnified(metricsStr) {
			metricsSet[m] = true
		}
		returnAllMetrics = false
	}

	// Build data points
	dataPoints := make([]MetricDataPoint, 0)
	for _, p := range performances {
		for metricName, value := range p.Performance {
			if commonMetadataFields[metricName] {
				continue
			}
			if !isMetricField(metricName, p.DataSource) {
				continue
			}
			if !returnAllMetrics && len(metricsSet) > 0 && !metricsSet[metricName] {
				continue
			}
			valueFloat := convertToFloat(value)
			if isNaN(valueFloat) {
				continue
			}
			dataPoints = append(dataPoints, MetricDataPoint{
				MetricName: metricName,
				Value:      valueFloat,
				Timestamp:  p.CreatedAt.UnixMilli(),
				Iteration:  p.Iteration,
				DataSource: p.DataSource,
			})
		}
	}

	// Deduplicate tensorflow data
	if req.DataSource == "tensorflow" || (req.DataSource == "" && len(dataPoints) > 0 && dataPoints[0].DataSource == "tensorflow") {
		dataPoints = deduplicateTensorflowDataPoints(dataPoints)
	}

	return &MetricsDataResponse{
		WorkloadUID: req.UID,
		DataSource:  req.DataSource,
		Data:        dataPoints,
		TotalCount:  len(dataPoints),
	}, nil
}

// handleWorkloadIterationTimes handles iteration times requests.
// Reuses: database.GetTraining().ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange
func handleWorkloadIterationTimes(ctx context.Context, req *WorkloadIterationTimesRequest) (*WorkloadIterationTimesResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Parse time range
	var startTime, endTime time.Time
	var hasTimeRange bool
	if req.Start != "" && req.End != "" {
		startMs, err := strconv.ParseInt(req.Start, 10, 64)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start time format")
		}
		endMs, err := strconv.ParseInt(req.End, 10, 64)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end time format")
		}
		startTime = time.UnixMilli(startMs)
		endTime = time.UnixMilli(endMs)
		hasTimeRange = true
	}

	var performances []*dbModel.TrainingPerformance
	if hasTimeRange {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(ctx, req.UID, req.DataSource, startTime, endTime)
	} else {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, req.UID, req.DataSource)
	}
	if err != nil {
		return nil, err
	}

	// Build iteration map for deduplication
	iterationMap := make(map[int32]*IterationInfo)
	for _, p := range performances {
		timestamp := p.CreatedAt.UnixMilli()
		var targetIteration *float64
		if targetIterValue, exists := p.Performance["target_iteration"]; exists {
			targetIterFloat := convertToFloat(targetIterValue)
			if !isNaN(targetIterFloat) {
				targetIteration = &targetIterFloat
			}
		}
		if existing, exists := iterationMap[p.Iteration]; !exists || timestamp < existing.Timestamp {
			iterationMap[p.Iteration] = &IterationInfo{
				Timestamp:       timestamp,
				TargetIteration: targetIteration,
				DataSource:      p.DataSource,
			}
		}
	}

	// Filter anomalous iterations for tensorflow
	if req.DataSource == "tensorflow" || (req.DataSource == "" && hasTensorflowData(iterationMap)) {
		iterationMap = filterAnomalousIterations(iterationMap)
	}

	// Convert to data points
	dataPoints := make([]MetricDataPoint, 0, len(iterationMap)*2)
	for iteration, info := range iterationMap {
		dataPoints = append(dataPoints, MetricDataPoint{
			MetricName: "iteration",
			Value:      float64(iteration),
			Timestamp:  info.Timestamp,
			Iteration:  iteration,
			DataSource: info.DataSource,
		})
		if info.TargetIteration != nil {
			dataPoints = append(dataPoints, MetricDataPoint{
				MetricName: "target_iteration",
				Value:      *info.TargetIteration,
				Timestamp:  info.Timestamp,
				Iteration:  iteration,
				DataSource: info.DataSource,
			})
		}
	}

	// Deduplicate tensorflow data
	if req.DataSource == "tensorflow" || (req.DataSource == "" && len(dataPoints) > 0 && dataPoints[0].DataSource == "tensorflow") {
		dataPoints = deduplicateTensorflowDataPoints(dataPoints)
	}

	return &MetricsDataResponse{
		WorkloadUID: req.UID,
		DataSource:  req.DataSource,
		Data:        dataPoints,
		TotalCount:  len(dataPoints),
	}, nil
}

// handleWorkloadMetadata handles workload metadata requests.
// Reuses: database.GetWorkload().GetWorkloadsNamespaceList, GetWorkloadKindList
func handleWorkloadMetadata(ctx context.Context, req *WorkloadMetadataRequest) (*WorkloadMetadataResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	namespaces, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetWorkloadsNamespaceList(ctx)
	if err != nil {
		return nil, err
	}

	kinds, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetWorkloadKindList(ctx)
	if err != nil {
		return nil, err
	}

	return &WorkloadMetadataResponse{
		Namespaces: namespaces,
		Kinds:      kinds,
	}, nil
}

// Helper functions

// splitMetricsForUnified splits a comma-separated metrics string and trims spaces.
func splitMetricsForUnified(s string) []string {
	parts := make([]string, 0)
	for _, p := range splitStringForUnified(s, ",") {
		trimmed := trimSpaceForUnified(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitStringForUnified(s string, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpaceForUnified(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func isNaN(f float64) bool {
	return f != f
}
