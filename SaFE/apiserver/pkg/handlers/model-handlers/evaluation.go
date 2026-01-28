/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// ==================== Evaluation API Handlers ====================

// ListAvailableEvalServices returns the list of models/services available for evaluation.
// GET /api/v1/evaluations/available-services
func (h *Handler) ListAvailableEvalServices(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		workspace := c.Query("workspace")
		ctx := c.Request.Context()

		var services []AvailableEvalService

		// Get remote_api models from Model table
		models, err := h.dbClient.ListModels(ctx, "remote_api", "", false)
		if err != nil {
			klog.ErrorS(err, "failed to list remote_api models")
		} else {
			for _, m := range models {
				services = append(services, AvailableEvalService{
					ServiceId:   m.ID,
					ServiceType: EvalServiceTypeRemoteAPI,
					DisplayName: m.DisplayName,
					ModelName:   m.ModelName,
					Status:      "Ready",
					Endpoint:    m.SourceURL,
				})
			}
		}

		// Get local workloads with inference (Deployment type with Running status)
		workloadTags := dbclient.GetWorkloadFieldTags()
		// GVK is stored as JSON in database, e.g. {"version":"v1","kind":"Deployment"}
		deploymentGVK := v1.GroupVersionKind{Kind: common.DeploymentKind, Version: common.DefaultVersion}
		gvkStr := string(jsonutils.MarshalSilently(deploymentGVK))
		workloadQuery := sqrl.And{
			sqrl.Eq{dbclient.GetFieldTag(workloadTags, "IsDeleted"): false},
			sqrl.Eq{dbclient.GetFieldTag(workloadTags, "GVK"): gvkStr},
			sqrl.Eq{dbclient.GetFieldTag(workloadTags, "Phase"): "Running"},
		}
		if workspace != "" {
			workloadQuery = append(workloadQuery, sqrl.Eq{dbclient.GetFieldTag(workloadTags, "Workspace"): workspace})
		}

		workloads, err := h.dbClient.SelectWorkloads(ctx, workloadQuery, nil, 100, 0)
		if err != nil {
			klog.ErrorS(err, "failed to list workloads")
		} else {
			for _, w := range workloads {
				services = append(services, AvailableEvalService{
					ServiceId:   w.WorkloadId,
					ServiceType: EvalServiceTypeLocalWorkload,
					DisplayName: w.DisplayName,
					Status:      w.Phase.String,
					Workspace:   w.Workspace,
				})
			}
		}

		return &ListAvailableServicesResponse{
			Items: services,
		}, nil
	})
}

// ListEvaluationTasks lists evaluation tasks with filtering.
// GET /api/v1/evaluations/tasks
func (h *Handler) ListEvaluationTasks(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		var req ListEvaluationTasksRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid query: %v", err))
		}

		// Build query
		dbTags := dbclient.GetEvaluationTaskFieldTags()
		query := sqrl.And{
			sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		}

		if req.Workspace != "" {
			query = append(query, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): req.Workspace})
		}
		if req.Status != "" {
			query = append(query, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Status"): req.Status})
		}
		if req.ServiceId != "" {
			// Use LIKE for fuzzy matching
			query = append(query, sqrl.Like{dbclient.GetFieldTag(dbTags, "ServiceId"): "%" + req.ServiceId + "%"})
		}

		// Set defaults
		if req.Limit <= 0 {
			req.Limit = 50
		}
		if req.Limit > 200 {
			req.Limit = 200
		}

		// Get total count
		totalCount, err := h.dbClient.CountEvaluationTasks(c.Request.Context(), query)
		if err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to count tasks: %v", err))
		}

		// Get tasks
		// Use NULLS LAST to ensure tasks with NULL creation_time are sorted after tasks with valid timestamps
		orderBy := []string{"creation_time DESC NULLS LAST", "id DESC"}
		tasks, err := h.dbClient.SelectEvaluationTasks(c.Request.Context(), query, orderBy, req.Limit, req.Offset)
		if err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to list tasks: %v", err))
		}

		// Convert to view
		items := make([]EvaluationTaskView, 0, len(tasks))
		for _, t := range tasks {
			items = append(items, h.convertToEvalTaskView(t))
		}

		return &ListEvaluationTasksResponse{
			Items:      items,
			TotalCount: totalCount,
		}, nil
	})
}

// GetEvaluationTask gets a specific evaluation task by ID.
// GET /api/v1/evaluations/tasks/:id
func (h *Handler) GetEvaluationTask(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		taskId := c.Param("id")
		if taskId == "" {
			return nil, commonerrors.NewBadRequest("task id is required")
		}

		task, err := h.dbClient.GetEvaluationTask(c.Request.Context(), taskId)
		if err != nil {
			return nil, err
		}

		return h.convertToEvalTaskView(task), nil
	})
}

// DeleteEvaluationTask deletes/cancels an evaluation task.
// DELETE /api/v1/evaluations/tasks/:id
func (h *Handler) DeleteEvaluationTask(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		taskId := c.Param("id")
		if taskId == "" {
			return nil, commonerrors.NewBadRequest("task id is required")
		}

		// Get task first
		task, err := h.dbClient.GetEvaluationTask(c.Request.Context(), taskId)
		if err != nil {
			return nil, err
		}

		// Delete associated OpsJob if exists (for any status, not just Running)
		// OpsJob might still exist if TTL hasn't expired yet
		if task.OpsJobId.Valid && task.OpsJobId.String != "" {
			opsJob := &v1.OpsJob{}
			opsJob.Name = task.OpsJobId.String
			if err := h.k8sClient.Delete(c.Request.Context(), opsJob); err != nil {
				// Ignore NotFound error (OpsJob may have been deleted by TTL)
				if !apierrors.IsNotFound(err) {
					klog.ErrorS(err, "failed to delete OpsJob", "opsJobId", task.OpsJobId.String)
				}
			} else {
				klog.InfoS("deleted OpsJob for evaluation task", "taskId", taskId, "opsJobId", task.OpsJobId.String)
			}
		}

		// Mark task as deleted
		if err := h.dbClient.SetEvaluationTaskDeleted(c.Request.Context(), taskId); err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to delete task: %v", err))
		}

		return gin.H{"message": "task deleted"}, nil
	})
}

// GetEvaluationReport gets the evaluation report for a task.
// GET /api/v1/evaluations/tasks/:id/report
func (h *Handler) GetEvaluationReport(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		taskId := c.Param("id")
		if taskId == "" {
			return nil, commonerrors.NewBadRequest("task id is required")
		}

		task, err := h.dbClient.GetEvaluationTask(c.Request.Context(), taskId)
		if err != nil {
			return nil, err
		}

		response := &EvaluationReportResponse{
			TaskId:      task.TaskId,
			TaskName:    task.TaskName,
			ServiceName: task.ServiceName,
			Status:      string(task.Status),
		}

		// Try to get report content from S3
		if task.ReportS3Path.Valid && task.ReportS3Path.String != "" && h.s3Client != nil {
			reportContent, err := h.s3Client.GetObject(c.Request.Context(), task.ReportS3Path.String, 60)
			if err != nil {
				klog.ErrorS(err, "failed to get report from S3", "taskId", taskId, "s3Key", task.ReportS3Path.String)
			} else {
				// Parse JSON content to results
				var reportData map[string]interface{}
				if err := json.Unmarshal([]byte(reportContent), &reportData); err != nil {
					klog.ErrorS(err, "failed to parse report JSON", "taskId", taskId)
				} else {
					response.Results = reportData
				}
			}
		}

		if task.StartTime.Valid {
			t := task.StartTime.Time
			response.StartTime = &t
		}
		if task.EndTime.Valid {
			t := task.EndTime.Time
			response.EndTime = &t
		}
		if task.StartTime.Valid && task.EndTime.Valid {
			duration := task.EndTime.Time.Sub(task.StartTime.Time)
			response.Duration = duration.String()
		}

		return response, nil
	})
}

// convertToEvalTaskView converts a database task to a view
func (h *Handler) convertToEvalTaskView(task *dbclient.EvaluationTask) EvaluationTaskView {
	view := EvaluationTaskView{
		TaskId:      task.TaskId,
		TaskName:    task.TaskName,
		Description: task.Description,
		ServiceId:   task.ServiceId,
		ServiceType: EvalServiceType(task.ServiceType),
		ServiceName: task.ServiceName,
		Status:      task.Status,
		Workspace:   task.Workspace,
		UserId:      task.UserId,
		UserName:    task.UserName,
	}

	// Parse benchmarks
	if task.Benchmarks != "" {
		var benchmarks []BenchmarkConfig
		if err := json.Unmarshal([]byte(task.Benchmarks), &benchmarks); err == nil {
			view.Benchmarks = benchmarks
		}
	}

	// Parse result summary
	if task.ResultSummary.Valid && task.ResultSummary.String != "" {
		var summary map[string]interface{}
		if err := json.Unmarshal([]byte(task.ResultSummary.String), &summary); err == nil {
			view.ResultSummary = summary
		}
	}

	if task.OpsJobId.Valid {
		view.OpsJobId = task.OpsJobId.String
	}
	if task.ReportS3Path.Valid {
		view.ReportS3Path = task.ReportS3Path.String
	}
	// Judge model configuration (for LLM-as-Judge mode)
	if task.JudgeServiceId.Valid {
		view.JudgeServiceId = task.JudgeServiceId.String
	}
	if task.JudgeServiceType.Valid {
		view.JudgeServiceType = task.JudgeServiceType.String
	}
	if task.JudgeServiceName.Valid {
		view.JudgeServiceName = task.JudgeServiceName.String
	}
	// Set evaluation type based on judge config
	if view.JudgeServiceId != "" {
		view.EvaluationType = "judge"
	} else {
		view.EvaluationType = "normal"
	}
	if task.CreationTime.Valid {
		t := task.CreationTime.Time
		view.CreationTime = &t
	}
	if task.StartTime.Valid {
		t := task.StartTime.Time
		view.StartTime = &t
	}
	if task.EndTime.Valid {
		t := task.EndTime.Time
		view.EndTime = &t
	}

	return view
}

// ==================== Report Utilities ====================

// parseS3Path parses an S3 path into bucket and key
// Supports formats: s3://bucket/key or bucket/key
func parseS3Path(s3Path string) (string, string, error) {
	path := strings.TrimPrefix(s3Path, "s3://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid S3 path format: %s", s3Path)
	}
	return parts[0], parts[1], nil
}

// GenerateS3ReportPath generates the S3 path for storing evaluation reports
func GenerateS3ReportPath(taskId string) string {
	return fmt.Sprintf("evaluations/%s/summary.json", taskId)
}

// CalculateOverallScore calculates an overall score from benchmark results
func CalculateOverallScore(results []BenchmarkResult) float64 {
	if len(results) == 0 {
		return 0
	}

	var totalScore float64
	var count int

	for _, r := range results {
		for key, value := range r.Metrics {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "accuracy") ||
				strings.Contains(lowerKey, "score") ||
				strings.Contains(lowerKey, "pass@1") {
				totalScore += value
				count++
				break
			}
		}
	}

	if count == 0 {
		return 0
	}

	return totalScore / float64(count)
}
