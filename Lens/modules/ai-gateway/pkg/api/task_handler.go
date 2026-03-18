// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/gin-gonic/gin"
)

// TaskHandler handles task API requests.
// It exposes two sets of endpoints:
//   - Producer endpoints: Publish, GetTask, GetTaskStatus, GetTaskResult, CancelTask, ListTasks
//   - Agent endpoints: ClaimTask, CompleteTask, FailTask
type TaskHandler struct {
	queue *aitaskqueue.PGStore
}

// NewTaskHandler creates a new TaskHandler
func NewTaskHandler(queue *aitaskqueue.PGStore) *TaskHandler {
	return &TaskHandler{queue: queue}
}

// --- Request / Response types ---

// TaskResponse represents a task in API responses
type TaskResponse struct {
	ID            string          `json:"id"`
	Topic         string          `json:"topic"`
	Status        string          `json:"status"`
	Priority      int             `json:"priority"`
	InputPayload  json.RawMessage `json:"input_payload,omitempty"`
	OutputPayload json.RawMessage `json:"output_payload,omitempty"`
	RetryCount    int             `json:"retry_count"`
	MaxRetries    int             `json:"max_retries"`
	AgentID       string          `json:"agent_id,omitempty"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	ErrorCode     int             `json:"error_code,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	TimeoutAt     time.Time       `json:"timeout_at"`
}

// PublishRequest is the body for POST /tasks
type PublishRequest struct {
	Topic      string          `json:"topic" binding:"required"`
	Payload    json.RawMessage `json:"payload" binding:"required"`
	Priority   int             `json:"priority,omitempty"`
	MaxRetries int             `json:"max_retries,omitempty"`
	TimeoutSec int             `json:"timeout_sec,omitempty"`
}

// ClaimRequest is the body for POST /tasks/claim
type ClaimRequest struct {
	AgentID string   `json:"agent_id" binding:"required"`
	Topics  []string `json:"topics" binding:"required,min=1"`
}

// CompleteRequest is the body for POST /tasks/:id/complete
type CompleteRequest struct {
	Result json.RawMessage `json:"result" binding:"required"`
}

// FailRequest is the body for POST /tasks/:id/fail
type FailRequest struct {
	ErrorCode    int    `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message" binding:"required"`
}

// --- Producer endpoints ---

// Publish handles POST /tasks - create a new task for agents to consume
func (h *TaskHandler) Publish(c *gin.Context) {
	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	opts := &aitaskqueue.PublishOptions{
		Topic:   req.Topic,
		Payload: req.Payload,
	}
	if req.Priority > 0 {
		opts.Priority = req.Priority
	}
	if req.MaxRetries > 0 {
		opts.MaxRetries = req.MaxRetries
	}
	if req.TimeoutSec > 0 {
		opts.Timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	taskID, err := h.queue.PublishWithOptions(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch the full task to return consistent response
	task, err := h.queue.GetTask(c.Request.Context(), taskID)
	if err != nil {
		// Task was created, return minimal info
		c.JSON(http.StatusCreated, gin.H{"id": taskID, "topic": req.Topic, "status": "pending"})
		return
	}

	c.JSON(http.StatusCreated, toTaskResponse(task))
}

// GetTask handles GET /tasks/:id
func (h *TaskHandler) GetTask(c *gin.Context) {
	task, err := h.queue.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleTaskError(c, err)
		return
	}
	c.JSON(http.StatusOK, toTaskResponse(task))
}

// GetTaskStatus handles GET /tasks/:id/status
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	task, err := h.queue.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleTaskError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":            task.ID,
		"status":        task.Status,
		"agent_id":      task.AgentID,
		"retry_count":   task.RetryCount,
		"max_retries":   task.MaxRetries,
		"error_message": task.ErrorMessage,
	})
}

// GetTaskResult handles GET /tasks/:id/result
func (h *TaskHandler) GetTaskResult(c *gin.Context) {
	taskID := c.Param("id")
	resp, err := h.queue.GetResult(c.Request.Context(), taskID)
	if err != nil {
		if err == aitaskqueue.ErrTaskNotCompleted {
			c.JSON(http.StatusAccepted, gin.H{"id": taskID, "status": "processing"})
			return
		}
		h.handleTaskError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":     taskID,
		"status": string(resp.Status),
		"code":   resp.Code,
		"result": resp.Payload,
	})
}

// CancelTask handles POST /tasks/:id/cancel
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("id")
	if err := h.queue.CancelTask(c.Request.Context(), taskID); err != nil {
		h.handleTaskError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": taskID, "message": "Task cancelled"})
}

// ListTasks handles GET /tasks
func (h *TaskHandler) ListTasks(c *gin.Context) {
	filter := &aitaskqueue.TaskFilter{}
	if status := c.Query("status"); status != "" {
		s := aitaskqueue.TaskStatus(status)
		filter.Status = &s
	}
	if topic := c.Query("topic"); topic != "" {
		filter.Topic = topic
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	} else {
		filter.Limit = 50
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	tasks, err := h.queue.ListTasks(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]TaskResponse, len(tasks))
	for i, t := range tasks {
		response[i] = toTaskResponse(t)
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":  response,
		"total":  len(response),
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// --- Agent endpoints (pull-model) ---

// ClaimTask handles POST /tasks/claim - agent pulls the next available task
func (h *TaskHandler) ClaimTask(c *gin.Context) {
	var req ClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.queue.ClaimTask(c.Request.Context(), req.Topics, req.AgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		// No pending tasks - normal, agent should retry later
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, toTaskResponse(task))
}

// CompleteTask handles POST /tasks/:id/complete - agent reports success
func (h *TaskHandler) CompleteTask(c *gin.Context) {
	taskID := c.Param("id")

	var req CompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := &aitopics.Response{
		RequestID: taskID,
		Status:    aitopics.StatusSuccess,
		Code:      aitopics.CodeSuccess,
		Message:   "success",
		Timestamp: time.Now(),
		Payload:   req.Result,
	}

	if err := h.queue.CompleteTask(c.Request.Context(), taskID, resp); err != nil {
		h.handleTaskError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": taskID, "status": "completed"})
}

// FailTask handles POST /tasks/:id/fail - agent reports failure
func (h *TaskHandler) FailTask(c *gin.Context) {
	taskID := c.Param("id")

	var req FailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.queue.FailTask(c.Request.Context(), taskID, req.ErrorCode, req.ErrorMessage); err != nil {
		h.handleTaskError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": taskID, "status": "failed"})
}

// --- Helpers ---

func (h *TaskHandler) handleTaskError(c *gin.Context, err error) {
	if err == aitaskqueue.ErrTaskNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func toTaskResponse(task *aitaskqueue.Task) TaskResponse {
	return TaskResponse{
		ID:            task.ID,
		Topic:         task.Topic,
		Status:        string(task.Status),
		Priority:      task.Priority,
		InputPayload:  task.InputPayload,
		OutputPayload: task.OutputPayload,
		RetryCount:    task.RetryCount,
		MaxRetries:    task.MaxRetries,
		AgentID:       task.AgentID,
		ErrorMessage:  task.ErrorMessage,
		ErrorCode:     task.ErrorCode,
		CreatedAt:     task.CreatedAt,
		StartedAt:     task.StartedAt,
		CompletedAt:   task.CompletedAt,
		TimeoutAt:     task.TimeoutAt,
	}
}
