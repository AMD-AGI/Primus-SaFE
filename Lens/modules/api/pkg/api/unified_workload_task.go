// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Workload Task endpoints: list detection/analysis tasks from workload_task_state =====

// WorkloadTaskListRequest is the request for listing workload tasks
type WorkloadTaskListRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Status   string `json:"status" query:"status" mcp:"status,description=Filter by task status: pending/running/completed/failed/cancelled (optional)"`
	TaskType string `json:"task_type" query:"task_type" mcp:"task_type,description=Filter by task type: detection_coordinator/analysis_pipeline/detection_process_probe/detection_image_probe/detection_log_scan/detection_label_probe (optional)"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 50)"`
}

// WorkloadTaskItem represents a single task in the response
type WorkloadTaskItem struct {
	ID             int64                  `json:"id"`
	WorkloadUID    string                 `json:"workload_uid"`
	TaskType       string                 `json:"task_type"`
	Status         string                 `json:"status"`
	LockOwner      string                 `json:"lock_owner,omitempty"`
	LockAcquiredAt string                 `json:"lock_acquired_at,omitempty"`
	LockExpiresAt  string                 `json:"lock_expires_at,omitempty"`
	Ext            map[string]interface{} `json:"ext,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// WorkloadTaskListResponse returns a list of workload tasks
type WorkloadTaskListResponse struct {
	Data  []WorkloadTaskItem `json:"data"`
	Total int                `json:"total"`
}

func init() {
	unified.Register(&unified.EndpointDef[WorkloadTaskListRequest, WorkloadTaskListResponse]{
		Name:        "workload_task_list",
		Description: "List workload detection and analysis tasks from workload_task_state table. Shows detection_coordinator, analysis_pipeline, probe tasks, etc.",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-task",
		MCPToolName: "lens_workload_task_list",
		Handler:     handleWorkloadTaskList,
	})
}

func handleWorkloadTaskList(ctx context.Context, req *WorkloadTaskListRequest) (*WorkloadTaskListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}
	clusterName := clients.ClusterName

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	taskFacade := database.NewWorkloadTaskFacadeForCluster(clusterName)

	// Intent-related task types
	intentTaskTypes := []string{
		"detection_coordinator",
		"analysis_pipeline",
		"detection_process_probe",
		"detection_image_probe",
		"detection_log_scan",
		"detection_label_probe",
		"active_detection",
	}

	// Query based on filters
	var allTasks []*dbModel.WorkloadTaskState
	if req.TaskType != "" {
		// Filter by specific task type
		if req.Status != "" {
			allTasks, err = taskFacade.ListTasksByTypeAndStatus(ctx, req.TaskType, req.Status)
		} else {
			// Get all statuses for this type
			allTasks, err = taskFacade.ListTasksByTypeAndStatuses(ctx, req.TaskType,
				[]string{"pending", "running", "completed", "failed", "cancelled"})
		}
	} else if req.Status != "" {
		// Filter by status, scoped to intent-related task types
		allTasks, err = taskFacade.ListTasksByStatusAndTypes(ctx, req.Status, intentTaskTypes)
	} else {
		// Default: get all intent-related tasks (all statuses)
		for _, status := range []string{"running", "pending", "completed", "failed", "cancelled"} {
			tasks, e := taskFacade.ListTasksByStatusAndTypes(ctx, status, intentTaskTypes)
			if e != nil {
				err = e
				break
			}
			allTasks = append(allTasks, tasks...)
		}
	}
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to list workload tasks: " + err.Error())
	}

	total := len(allTasks)

	// Sort by updated_at desc (most recent first)
	// allTasks comes ordered by created_at ASC from the facade, reverse it
	for i, j := 0, len(allTasks)-1; i < j; i, j = i+1, j-1 {
		allTasks[i], allTasks[j] = allTasks[j], allTasks[i]
	}

	// Paginate
	offset := (pageNum - 1) * pageSize
	if offset >= len(allTasks) {
		return &WorkloadTaskListResponse{Data: []WorkloadTaskItem{}, Total: total}, nil
	}
	end := offset + pageSize
	if end > len(allTasks) {
		end = len(allTasks)
	}
	paged := allTasks[offset:end]

	// Convert to response items
	data := make([]WorkloadTaskItem, 0, len(paged))
	for _, t := range paged {
		item := WorkloadTaskItem{
			ID:          t.ID,
			WorkloadUID: t.WorkloadUID,
			TaskType:    t.TaskType,
			Status:      t.Status,
			LockOwner:   t.LockOwner,
			CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if !t.LockAcquiredAt.IsZero() {
			item.LockAcquiredAt = t.LockAcquiredAt.Format("2006-01-02T15:04:05Z")
		}
		if !t.LockExpiresAt.IsZero() {
			item.LockExpiresAt = t.LockExpiresAt.Format("2006-01-02T15:04:05Z")
		}
		if t.Ext != nil {
			item.Ext = t.Ext
		}
		data = append(data, item)
	}

	return &WorkloadTaskListResponse{Data: data, Total: total}, nil
}
