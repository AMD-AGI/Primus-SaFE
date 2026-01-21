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
type WorkloadGPUHistoryResponse = model.GpuUtilizationHistory

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
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Reuse existing database query
	dbWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, req.UID)
	if err != nil {
		return nil, err
	}
	if dbWorkload == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

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
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

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
	workloads, _, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		return nil, err
	}

	if len(workloads) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

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
	gpuUtilHistory, err := workload.GetWorkloadGpuUtilizationHistory(ctx, dbWorkload.UID, startTime, endTime, step, storageClient)
	if err != nil {
		return nil, err
	}

	return gpuUtilHistory, nil
}
