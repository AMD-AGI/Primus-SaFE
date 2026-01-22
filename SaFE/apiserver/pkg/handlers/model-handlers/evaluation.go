/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	DefaultEvalTimeoutSecond = 7200 // 2 hours
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

// CreateEvaluationTask creates a new evaluation task.
// POST /api/v1/evaluations/tasks
func (h *Handler) CreateEvaluationTask(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		var req CreateEvaluationTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request: %v", err))
		}

		// Validate service type
		if req.ServiceType != EvalServiceTypeRemoteAPI && req.ServiceType != EvalServiceTypeLocalWorkload {
			return nil, commonerrors.NewBadRequest("serviceType must be 'remote_api' or 'local_workload'")
		}

		// Validate benchmarks (now from dataset table)
		if err := h.validateBenchmarks(c.Request.Context(), req.Benchmarks); err != nil {
			return nil, err
		}

		// Get user info from context
		userId := c.GetString(common.UserId)
		userName := c.GetString(common.UserName)
		if userId == "" {
			return nil, commonerrors.NewUnauthorized("user not authenticated")
		}

		// Get service name
		serviceName := ""
		if req.ServiceType == EvalServiceTypeRemoteAPI {
			model, err := h.dbClient.GetModelByID(c.Request.Context(), req.ServiceId)
			if err != nil {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model not found: %s", req.ServiceId))
			}
			serviceName = model.DisplayName
		} else {
			workload, err := h.dbClient.GetWorkload(c.Request.Context(), req.ServiceId)
			if err != nil {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("workload not found: %s", req.ServiceId))
			}
			serviceName = workload.DisplayName
		}

		// Generate task ID
		taskId := fmt.Sprintf("eval-task-%s", uuid.New().String()[:8])

		// Set default values
		if req.TimeoutSecond <= 0 {
			req.TimeoutSecond = DefaultEvalTimeoutSecond
		}
		if req.EvalParams == nil {
			req.EvalParams = &EvalParams{}
		}

		// Serialize benchmarks and params to JSON
		benchmarksJSON, err := json.Marshal(req.Benchmarks)
		if err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to serialize benchmarks: %v", err))
		}
		evalParamsJSON, err := json.Marshal(req.EvalParams)
		if err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to serialize evalParams: %v", err))
		}

		// Create evaluation task in database
		task := &dbclient.EvaluationTask{
			TaskId:      taskId,
			TaskName:    req.Name,
			Description: req.Description,
			ServiceId:   req.ServiceId,
			ServiceType: string(req.ServiceType),
			ServiceName: serviceName,
			Benchmarks:  string(benchmarksJSON),
			EvalParams:  string(evalParamsJSON),
			Status:      dbclient.EvaluationTaskStatusPending,
			Progress:    0,
			Workspace:   req.WorkspaceId,
			UserId:      userId,
			UserName:    userName,
		}

		if err := h.dbClient.UpsertEvaluationTask(c.Request.Context(), task); err != nil {
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create evaluation task: %v", err))
		}

		// Create OpsJob for evaluation
		opsJobId, err := h.createEvaluationOpsJob(c.Request.Context(), task, req, userId)
		if err != nil {
			// Mark task as failed
			_ = h.dbClient.SetEvaluationTaskFailed(c.Request.Context(), taskId, err.Error())
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to create evaluation job: %v", err))
		}

		// Update task with OpsJob ID
		if err := h.dbClient.UpdateEvaluationTaskOpsJobId(c.Request.Context(), taskId, opsJobId); err != nil {
			klog.ErrorS(err, "failed to update task with ops_job_id", "taskId", taskId, "opsJobId", opsJobId)
		}

		return &CreateEvaluationTaskResponse{
			TaskId:   taskId,
			OpsJobId: opsJobId,
		}, nil
	})
}

// validateBenchmarks validates the benchmark configurations by checking dataset existence
// All benchmarks are now stored in the dataset table (system benchmarks have userId = primus-safe-system)
func (h *Handler) validateBenchmarks(ctx context.Context, benchmarks []BenchmarkConfig) error {
	for _, b := range benchmarks {
		if b.DatasetId == "" {
			return commonerrors.NewBadRequest("datasetId is required")
		}

		// Verify dataset exists and is evaluation type
		dataset, err := h.dbClient.GetDataset(ctx, b.DatasetId)
		if err != nil {
			return commonerrors.NewBadRequest(fmt.Sprintf("dataset not found: %s", b.DatasetId))
		}
		if dataset.DatasetType != "evaluation" {
			return commonerrors.NewBadRequest(fmt.Sprintf("dataset %s is not an evaluation type dataset", b.DatasetId))
		}

		// For user-uploaded datasets, evalType is required
		// System benchmark datasets (userId = primus-safe-system) don't need evalType
		if dataset.UserId != common.UserSystem && b.EvalType != "" {
			if !IsValidCustomEvalType(b.EvalType) {
				return commonerrors.NewBadRequest(fmt.Sprintf("invalid evalType: %s, must be 'general_qa' or 'general_mcq'", b.EvalType))
			}
		}
	}
	return nil
}

// createEvaluationOpsJob creates an OpsJob for the evaluation task
func (h *Handler) createEvaluationOpsJob(ctx context.Context, task *dbclient.EvaluationTask, req CreateEvaluationTaskRequest, userId string) (string, error) {
	opsJobName := fmt.Sprintf("evaluation-%s", task.TaskId)

	// Build OpsJob inputs
	inputs := []v1.Parameter{
		{Name: v1.ParameterEvalTaskId, Value: task.TaskId},
		{Name: v1.ParameterEvalServiceType, Value: task.ServiceType},
		{Name: v1.ParameterEvalBenchmarks, Value: task.Benchmarks},
		{Name: v1.ParameterEvalParams, Value: task.EvalParams},
	}

	// Add model-specific parameters
	if req.ServiceType == EvalServiceTypeRemoteAPI {
		model, err := h.dbClient.GetModelByID(ctx, req.ServiceId)
		if err != nil {
			return "", fmt.Errorf("failed to get model: %v", err)
		}
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterModelEndpoint, Value: model.SourceURL})
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterModelName, Value: model.ModelName})
		if model.SourceToken != "" {
			inputs = append(inputs, v1.Parameter{Name: v1.ParameterModelApiKey, Value: model.SourceToken})
		}
	} else {
		// For local workload, get the service endpoint
		workload, err := h.dbClient.GetWorkload(ctx, req.ServiceId)
		if err != nil {
			return "", fmt.Errorf("failed to get workload: %v", err)
		}
		// Construct endpoint from workload service info
		endpoint := fmt.Sprintf("http://%s:8080/v1", workload.WorkloadId)
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterModelEndpoint, Value: endpoint})
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterModelName, Value: workload.DisplayName})
		// Use workload's cluster and workspace
		if workload.Cluster != "" {
			inputs = append(inputs, v1.Parameter{Name: v1.ParameterCluster, Value: workload.Cluster})
		}
		if req.WorkspaceId == "" && workload.Workspace != "" {
			req.WorkspaceId = workload.Workspace
		}
	}

	// Add workspace parameter
	if req.WorkspaceId != "" {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterWorkspace, Value: req.WorkspaceId})
	}

	// Get cluster ID from input parameters
	var clusterId string
	for _, input := range inputs {
		if input.Name == v1.ParameterCluster {
			clusterId = input.Value
			break
		}
	}

	// Create OpsJob CR
	opsJob := &v1.OpsJob{}
	opsJob.Name = opsJobName
	opsJob.Labels = map[string]string{
		v1.DisplayNameLabel:            req.Name,
		dbclient.EvaluationTaskIdLabel: task.TaskId,
	}
	if clusterId != "" {
		opsJob.Labels[v1.ClusterIdLabel] = clusterId
	}
	if userId != "" {
		opsJob.Labels[v1.UserIdLabel] = userId
	}
	opsJob.Spec = v1.OpsJobSpec{
		Type:                    v1.OpsJobEvaluationType,
		Inputs:                  inputs,
		TimeoutSecond:           req.TimeoutSecond,
		TTLSecondsAfterFinished: 3600, // Keep for 1 hour after completion
		IsTolerateAll:           true,
	}

	// Create the OpsJob in Kubernetes
	if err := h.k8sClient.Create(ctx, opsJob); err != nil {
		return "", fmt.Errorf("failed to create OpsJob: %v", err)
	}

	klog.InfoS("created evaluation OpsJob", "opsJobName", opsJobName, "taskId", task.TaskId)
	return opsJobName, nil
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
			query = append(query, sqrl.Eq{dbclient.GetFieldTag(dbTags, "ServiceId"): req.ServiceId})
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
		orderBy := []string{"creation_time DESC"}
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

		// If task is running, try to cancel the OpsJob
		if task.Status == dbclient.EvaluationTaskStatusRunning && task.OpsJobId.Valid {
			opsJob := &v1.OpsJob{}
			opsJob.Name = task.OpsJobId.String
			if err := h.k8sClient.Delete(c.Request.Context(), opsJob); err != nil {
				klog.ErrorS(err, "failed to delete OpsJob", "opsJobId", task.OpsJobId.String)
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

		// Parse result summary
		var results map[string]interface{}
		if task.ResultSummary.Valid && task.ResultSummary.String != "" {
			if err := json.Unmarshal([]byte(task.ResultSummary.String), &results); err != nil {
				klog.ErrorS(err, "failed to parse result summary", "taskId", taskId)
			}
		}

		response := &EvaluationReportResponse{
			TaskId:      task.TaskId,
			TaskName:    task.TaskName,
			ServiceName: task.ServiceName,
			Status:      string(task.Status),
			Results:     results,
		}

		if task.ReportS3Path.Valid {
			response.ReportS3Path = task.ReportS3Path.String
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
		Progress:    task.Progress,
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

	// Parse eval params
	if task.EvalParams != "" {
		var params EvalParams
		if err := json.Unmarshal([]byte(task.EvalParams), &params); err == nil {
			view.EvalParams = &params
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
