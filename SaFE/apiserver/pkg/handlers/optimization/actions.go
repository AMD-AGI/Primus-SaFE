/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/klog/v2"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// BatchCreateTasks creates multiple optimization tasks sequentially. Each item
// is attempted independently — failures are recorded per-item rather than
// aborting the whole batch. Callers should inspect the Error field on each
// item to distinguish successes from failures.
func (h *Handler) BatchCreateTasks(c *gin.Context) {
	var req BatchCreateTasksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	if len(req.Items) == 0 {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("items must not be empty"))
		return
	}
	if len(req.Items) > 100 {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("batch size must not exceed 100 items per request"))
		return
	}
	userID := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	bearer := clawBearerForGin(c)
	// Use a detached context so that individual item failures or a slow Claw
	// call on item N do not cancel remaining items via request timeout.
	batchCtx := context.Background()
	items := make([]BatchCreateTaskResponseItem, len(req.Items))
	for i := range req.Items {
		resp, err := h.submitTask(batchCtx, &req.Items[i], userID, userName, "", bearer)
		if err != nil {
			klog.ErrorS(err, "batch create task: item failed", "index", i, "model_id", req.Items[i].ModelID)
			items[i] = BatchCreateTaskResponseItem{Error: err.Error()}
		} else {
			items[i] = BatchCreateTaskResponseItem{
				ID:            resp.ID,
				ClawSessionID: resp.ClawSessionID,
			}
		}
	}
	c.JSON(http.StatusMultiStatus, BatchCreateTasksResponse{Items: items})
}

// ListArtifacts returns the session artifacts Claw has stored for this task.
func (h *Handler) ListArtifacts(c *gin.Context) {
	task, err := h.getTaskForAction(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if task.ClawSessionID == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("task has no Claw session"))
		return
	}
	clawCtx := WithClawBearer(c.Request.Context(), clawBearerForGin(c))
	items, err := h.clawClient.ListSessionFiles(clawCtx, task.ClawSessionID)
	if err != nil {
		klog.ErrorS(err, "list optimization artifacts", "task_id", task.ID)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list artifacts"))
		return
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})

	respItems := make([]ArtifactInfo, 0, len(items))
	reportPath := ""
	for _, item := range items {
		respItems = append(respItems, ArtifactInfo{
			Path:         item.Path,
			Run:          item.Run,
			Size:         item.Size,
			LastModified: item.LastModified,
			DownloadPath: common.PrimusRouterCustomRootPath + "/optimization/tasks/" + task.ID + "/artifacts/download?path=" + url.QueryEscape(item.Path),
		})
		if reportPath == "" && looksLikeOptimizationReport(item.Path) {
			reportPath = item.Path
		}
	}
	if reportPath != "" && task.ReportPath == "" {
		_ = h.dbClient.UpdateOptimizationTaskResult(context.Background(), task.ID, task.FinalMetrics, reportPath)
	}
	c.JSON(http.StatusOK, ListArtifactsResponse{Items: respItems})
}

// DownloadArtifact proxies the bytes of a Claw session artifact back to the client.
func (h *Handler) DownloadArtifact(c *gin.Context) {
	task, err := h.getTaskForAction(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("path query parameter is required"))
		return
	}
	data, err := h.clawClient.ReadSessionFile(WithClawBearer(c.Request.Context(), clawBearerForGin(c)), task.ClawSessionID, path)
	if err != nil {
		klog.ErrorS(err, "download optimization artifact", "task_id", task.ID, "path", path)
		if strings.Contains(err.Error(), "HTTP 404") {
			apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("artifact not found: "+path))
		} else {
			apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to download artifact"))
		}
		return
	}
	filename := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		filename = path[idx+1:]
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/octet-stream", data)
}

// InterruptTask requests cancellation from Claw and marks the task interrupted.
func (h *Handler) InterruptTask(c *gin.Context) {
	task, err := h.getTaskForAction(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if task.ClawSessionID == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("task has no active Claw session"))
		return
	}
	if err := h.clawClient.InterruptSession(WithClawBearer(c.Request.Context(), clawBearerForGin(c)), task.ClawSessionID); err != nil {
		klog.ErrorS(err, "interrupt optimization task", "task_id", task.ID)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to interrupt task"))
		return
	}
	_ = h.dbClient.UpdateOptimizationTaskStatus(c.Request.Context(), task.ID,
		dbclient.OptimizationTaskStatusInterrupted, task.CurrentPhase, "interrupt requested")
	c.JSON(http.StatusOK, StatusEventPayload{
		Status:  StatusInterrupted,
		Message: "interrupt requested",
	})
}

// RetryTask clones a failed/interrupted task into a fresh task with the same
// parameters, preserving the original task history intact.
func (h *Handler) RetryTask(c *gin.Context) {
	task, err := h.getTaskForAction(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if task.Status != dbclient.OptimizationTaskStatusFailed &&
		task.Status != dbclient.OptimizationTaskStatusInterrupted {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("only failed or interrupted tasks can be retried"))
		return
	}
	req := taskToCreateRequest(task)
	userID := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	resp, err := h.submitTask(c.Request.Context(), req, userID, userName, "", clawBearerForGin(c))
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.JSON(http.StatusCreated, RetryTaskResponse{
		ID:            resp.ID,
		ClawSessionID: resp.ClawSessionID,
	})
}

func (h *Handler) getTaskForAction(c *gin.Context) (*dbclient.OptimizationTask, error) {
	id := c.Param("id")
	task, err := h.dbClient.GetOptimizationTask(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, commonerrors.NewNotFoundWithMessage("task not found")
		}
		return nil, commonerrors.NewInternalError("failed to load task")
	}
	if task == nil {
		return nil, commonerrors.NewNotFoundWithMessage("task not found")
	}
	return task, nil
}

func taskToCreateRequest(task *dbclient.OptimizationTask) *CreateTaskRequest {
	var kernelBackends []string
	if task.KernelBackends != "" {
		_ = json.Unmarshal([]byte(task.KernelBackends), &kernelBackends)
	}
	return &CreateTaskRequest{
		DisplayName:    task.DisplayName,
		ModelID:        task.ModelID,
		Workspace:      task.Workspace,
		Mode:           task.Mode,
		Framework:      task.Framework,
		Precision:      task.Precision,
		TP:             task.TP,
		EP:             task.EP,
		GPUType:        task.GPUType,
		ISL:            task.ISL,
		OSL:            task.OSL,
		Concurrency:    task.Concurrency,
		KernelBackends: kernelBackends,
		GeakStepLimit:  task.GeakStepLimit,
		Image:          task.Image,
		ResultsPath:    task.ResultsPath,
	}
}

func looksLikeOptimizationReport(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, "optimization_report.md") ||
		strings.HasSuffix(lower, "optimization-report.md") ||
		strings.HasSuffix(lower, "optimization_report_qwen3_30b_vllm.md") ||
		strings.HasSuffix(lower, "kimi-k25-vllm-optimization-report.md") ||
		strings.HasSuffix(lower, "gpt-oss-120b_optimization_report.md") ||
		strings.Contains(lower, "optimization_report")
}
