// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ========== Response Types ==========

// AnalysisTasksResponse represents the response for listing analysis tasks
type AnalysisTasksResponse struct {
	RunID        int64                         `json:"run_id"`
	GithubRunID  int64                         `json:"github_run_id,omitempty"`
	WorkflowName string                        `json:"workflow_name,omitempty"`
	RepoName     string                        `json:"repo_name,omitempty"`
	Tasks        []*database.AnalysisTask      `json:"tasks"`
	Summary      *database.AnalysisTaskSummary `json:"summary"`
}

// AnalysisTaskListResponse represents the response for listing all analysis tasks
type AnalysisTaskListResponse struct {
	Tasks []*database.AnalysisTask `json:"tasks"`
	Total int64                    `json:"total"`
	Limit int                      `json:"limit"`
	Offset int                     `json:"offset"`
}

// ========== Handlers ==========

// GetAnalysisTasksByRunID handles GET /v1/github-workflow-metrics/runs/:id/analysis-tasks
// Returns all analysis tasks associated with a specific workflow run
func GetAnalysisTasksByRunID(ctx *gin.Context) {
	runIDStr := ctx.Param("id")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid run ID: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid run ID", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid cluster: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	facade := database.NewAnalysisTaskFacadeForCluster(clusterName)

	// Get tasks
	tasks, err := facade.GetTasksByRunID(ctx.Request.Context(), runID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis tasks: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Get summary
	summary, err := facade.GetSummaryByRunID(ctx.Request.Context(), runID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis task summary: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Get workflow info from the first task if available
	response := &AnalysisTasksResponse{
		RunID:   runID,
		Tasks:   tasks,
		Summary: summary,
	}

	if len(tasks) > 0 {
		response.GithubRunID = tasks[0].GithubRunID
		response.WorkflowName = tasks[0].WorkflowName
		response.RepoName = tasks[0].RepoName
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// GetAnalysisTasksByRunSummaryID handles GET /v1/github-runners/run-summaries/:id/analysis-tasks
// Returns all analysis tasks associated with a specific run summary
func GetAnalysisTasksByRunSummaryID(ctx *gin.Context) {
	summaryIDStr := ctx.Param("id")
	summaryID, err := strconv.ParseInt(summaryIDStr, 10, 64)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid summary ID: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid summary ID", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid cluster: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	facade := database.NewAnalysisTaskFacadeForCluster(clusterName)

	tasks, err := facade.GetTasksByRunSummaryID(ctx.Request.Context(), summaryID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis tasks for summary %d: %v", summaryID, err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	summary, err := facade.GetSummaryByRunSummaryID(ctx.Request.Context(), summaryID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis task summary for summary %d: %v", summaryID, err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	response := &AnalysisTasksResponse{
		RunID:   summaryID,
		Tasks:   tasks,
		Summary: summary,
	}

	if len(tasks) > 0 {
		response.GithubRunID = tasks[0].GithubRunID
		response.WorkflowName = tasks[0].WorkflowName
		response.RepoName = tasks[0].RepoName
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// GetAnalysisTaskByID handles GET /v1/github-workflow-metrics/analysis-tasks/:task_id
// Returns a single analysis task by ID
func GetAnalysisTaskByID(ctx *gin.Context) {
	taskIDStr := ctx.Param("task_id")
	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid task ID: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid task ID", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid cluster: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	facade := database.NewAnalysisTaskFacadeForCluster(clusterName)

	task, err := facade.GetTaskByID(ctx.Request.Context(), taskID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis task: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if task == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "analysis task not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), task))
}

// ListAnalysisTasks handles GET /v1/github-workflow-metrics/analysis-tasks
// Returns all analysis tasks with optional filters
func ListAnalysisTasks(ctx *gin.Context) {
	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid cluster: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Parse query parameters
	opts := database.ListAnalysisTasksOptions{
		TaskType: ctx.Query("type"),
		Status:   ctx.Query("status"),
		RepoName: ctx.Query("repo_name"),
	}

	// Parse time filters
	if startTimeStr := ctx.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			opts.StartTime = t
		}
	}
	if endTimeStr := ctx.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			opts.EndTime = t
		}
	}

	// Parse pagination
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			opts.Limit = limit
		}
	} else {
		opts.Limit = 20 // default limit
	}
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			opts.Offset = offset
		}
	}

	facade := database.NewAnalysisTaskFacadeForCluster(clusterName)

	tasks, total, err := facade.ListTasks(ctx.Request.Context(), opts)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list analysis tasks: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	response := &AnalysisTaskListResponse{
		Tasks:  tasks,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// UpdateAnalysisTaskRequest represents the request body for updating an analysis task
type UpdateAnalysisTaskRequest struct {
	Status string                 `json:"status"`
	Ext    map[string]interface{} `json:"ext"`
}

// UpdateAnalysisTask handles PUT /v1/github-workflow-metrics/analysis-tasks/:task_id
// Updates an analysis task status and ext fields
func UpdateAnalysisTask(ctx *gin.Context) {
	taskIDStr := ctx.Param("task_id")
	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid task ID: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid task ID", nil))
		return
	}

	var req UpdateAnalysisTaskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid request body", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid cluster: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	facade := database.NewAnalysisTaskFacadeForCluster(clusterName)

	// Update the task
	err = facade.UpdateTask(ctx.Request.Context(), taskID, req.Status, req.Ext)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update analysis task: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Get the updated task
	task, err := facade.GetTaskByID(ctx.Request.Context(), taskID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis task after update: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), task))
}

// RetryAnalysisTask handles POST /v1/github-workflow-metrics/analysis-tasks/:task_id/retry
// Retries a failed analysis task
func RetryAnalysisTask(ctx *gin.Context) {
	taskIDStr := ctx.Param("task_id")
	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid task ID: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid task ID", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Invalid cluster: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	facade := database.NewAnalysisTaskFacadeForCluster(clusterName)

	err = facade.RetryTask(ctx.Request.Context(), taskID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to retry analysis task: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Get the updated task
	task, err := facade.GetTaskByID(ctx.Request.Context(), taskID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get analysis task after retry: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), task))
}
