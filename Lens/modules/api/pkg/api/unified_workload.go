// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for workload operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
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
