// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// Job Execution Histories
	unified.Register(&unified.EndpointDef[JobHistoriesListRequest, JobHistoriesListResponse]{
		Name:        "job_histories",
		Description: "List job execution histories with filtering and pagination",
		HTTPMethod:  "GET",
		HTTPPath:    "/job-execution-histories",
		MCPToolName: "lens_job_histories",
		Handler:     handleJobHistoriesList,
	})

	unified.Register(&unified.EndpointDef[RecentFailuresRequest, []*dbmodel.JobExecutionHistory]{
		Name:        "recent_failures",
		Description: "Get recent job failure records",
		HTTPMethod:  "GET",
		HTTPPath:    "/job-execution-histories/recent-failures",
		MCPToolName: "lens_recent_failures",
		Handler:     handleRecentFailures,
	})

	unified.Register(&unified.EndpointDef[JobStatisticsRequest, database.JobStatistics]{
		Name:        "job_statistics",
		Description: "Get statistics for a specific job",
		HTTPMethod:  "GET",
		HTTPPath:    "/job-execution-histories/statistics/:job_name",
		MCPToolName: "lens_job_statistics",
		Handler:     handleJobStatistics,
	})

	unified.Register(&unified.EndpointDef[JobHistoryDetailRequest, dbmodel.JobExecutionHistory]{
		Name:        "job_history_detail",
		Description: "Get job execution history details by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/job-execution-histories/:id",
		MCPToolName: "lens_job_history_detail",
		Handler:     handleJobHistoryDetail,
	})
}

// ======================== Request Types ========================

type JobHistoriesListRequest struct {
	Cluster       string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	JobName       string `json:"job_name" form:"job_name" mcp:"description=Filter by job name (supports fuzzy matching)"`
	JobType       string `json:"job_type" form:"job_type" mcp:"description=Filter by job type"`
	Status        string `json:"status" form:"status" mcp:"description=Filter by status (running/success/failed/cancelled/timeout)"`
	ClusterName   string `json:"cluster_name" form:"cluster_name" mcp:"description=Filter by cluster name"`
	Hostname      string `json:"hostname" form:"hostname" mcp:"description=Filter by hostname"`
	StartTimeFrom string `json:"start_time_from" form:"start_time_from" mcp:"description=Start time range (RFC3339 format)"`
	StartTimeTo   string `json:"start_time_to" form:"start_time_to" mcp:"description=End time range (RFC3339 format)"`
	MinDuration   string `json:"min_duration" form:"min_duration" mcp:"description=Minimum execution duration (seconds)"`
	MaxDuration   string `json:"max_duration" form:"max_duration" mcp:"description=Maximum execution duration (seconds)"`
	PageNum       int    `json:"page_num" form:"page_num" mcp:"description=Page number (default 1)"`
	PageSize      int    `json:"page_size" form:"page_size" mcp:"description=Page size (default 20 max 100)"`
	OrderBy       string `json:"order_by" form:"order_by" mcp:"description=Sort field (default: started_at DESC)"`
}

type RecentFailuresRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Number of records to return (default 10 max 100)"`
}

type JobStatisticsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	JobName string `json:"job_name" form:"job_name" uri:"job_name" binding:"required" mcp:"description=Job name,required"`
}

type JobHistoryDetailRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Job execution history ID,required"`
}

// ======================== Response Types ========================

type JobHistoriesListResponse struct {
	Data     []*dbmodel.JobExecutionHistory `json:"data"`
	Total    int64                          `json:"total"`
	PageNum  int                            `json:"pageNum"`
	PageSize int                            `json:"pageSize"`
}

// ======================== Handler Implementations ========================

func handleJobHistoriesList(ctx context.Context, req *JobHistoriesListRequest) (*JobHistoriesListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	if pageNum < 1 {
		pageNum = 1
	}

	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	orderBy := req.OrderBy
	if orderBy == "" {
		orderBy = "started_at DESC"
	}

	filter := &database.JobExecutionHistoryFilter{
		Offset:  (pageNum - 1) * pageSize,
		Limit:   pageSize,
		OrderBy: orderBy,
	}

	// Apply filters
	if req.JobName != "" {
		filter.JobName = &req.JobName
	}
	if req.JobType != "" {
		filter.JobType = &req.JobType
	}
	if req.Status != "" {
		filter.Status = &req.Status
	}
	if req.ClusterName != "" {
		filter.ClusterName = &req.ClusterName
	}
	if req.Hostname != "" {
		filter.Hostname = &req.Hostname
	}

	// Parse time range
	if req.StartTimeFrom != "" {
		if t, err := time.Parse(time.RFC3339, req.StartTimeFrom); err == nil {
			filter.StartTimeFrom = &t
		} else {
			return nil, errors.WrapError(err, "invalid start_time_from format, use RFC3339", errors.RequestParameterInvalid)
		}
	}
	if req.StartTimeTo != "" {
		if t, err := time.Parse(time.RFC3339, req.StartTimeTo); err == nil {
			filter.StartTimeTo = &t
		} else {
			return nil, errors.WrapError(err, "invalid start_time_to format, use RFC3339", errors.RequestParameterInvalid)
		}
	}

	// Parse duration range
	if req.MinDuration != "" {
		if d, err := strconv.ParseFloat(req.MinDuration, 64); err == nil {
			filter.MinDuration = &d
		}
	}
	if req.MaxDuration != "" {
		if d, err := strconv.ParseFloat(req.MaxDuration, 64); err == nil {
			filter.MaxDuration = &d
		}
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	histories, total, err := facade.ListJobExecutionHistories(context.Background(), filter)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to list job execution histories", errors.CodeDatabaseError)
	}

	return &JobHistoriesListResponse{
		Data:     histories,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

func handleRecentFailures(ctx context.Context, req *RecentFailuresRequest) (*[]*dbmodel.JobExecutionHistory, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	histories, err := facade.GetRecentFailures(context.Background(), limit)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get recent failures", errors.CodeDatabaseError)
	}

	return &histories, nil
}

func handleJobStatistics(ctx context.Context, req *JobStatisticsRequest) (*database.JobStatistics, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	if req.JobName == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("job_name is required")
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	stats, err := facade.GetJobStatistics(context.Background(), req.JobName)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get job statistics", errors.CodeDatabaseError)
	}

	return stats, nil
}

func handleJobHistoryDetail(ctx context.Context, req *JobHistoryDetailRequest) (*dbmodel.JobExecutionHistory, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid history ID")
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	history, err := facade.GetJobExecutionHistoryByID(context.Background(), id)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to get job execution history", errors.CodeDatabaseError)
	}

	if history == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("history record not found")
	}

	return history, nil
}
