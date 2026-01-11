// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/gin-gonic/gin"
)

// TaskHandler handles task API requests
type TaskHandler struct {
	queue *aitaskqueue.PGStore
}

// NewTaskHandler creates a new TaskHandler
func NewTaskHandler(queue *aitaskqueue.PGStore) *TaskHandler {
	return &TaskHandler{
		queue: queue,
	}
}

// TaskResponse represents a task in API responses
type TaskResponse struct {
	ID           string     `json:"id"`
	Topic        string     `json:"topic"`
	Status       string     `json:"status"`
	Priority     int        `json:"priority"`
	RetryCount   int        `json:"retry_count"`
	MaxRetries   int        `json:"max_retries"`
	AgentID      string     `json:"agent_id,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	ErrorCode    int        `json:"error_code,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	TimeoutAt    time.Time  `json:"timeout_at"`
}

// GetTask handles GET /api/v1/ai/tasks/:id
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.queue.GetTask(c.Request.Context(), taskID)
	if err != nil {
		if err == aitaskqueue.ErrTaskNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, toTaskResponse(task))
}

// GetTaskStatus handles GET /api/v1/ai/tasks/:id/status
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.queue.GetTask(c.Request.Context(), taskID)
	if err != nil {
		if err == aitaskqueue.ErrTaskNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           task.ID,
		"status":       task.Status,
		"retry_count":  task.RetryCount,
		"max_retries":  task.MaxRetries,
		"error_message": task.ErrorMessage,
	})
}

// CancelTask handles POST /api/v1/ai/tasks/:id/cancel
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.queue.CancelTask(c.Request.Context(), taskID); err != nil {
		if err == aitaskqueue.ErrTaskNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Task cancelled successfully",
		"id":      taskID,
	})
}

// ListTasks handles GET /api/v1/ai/tasks
func (h *TaskHandler) ListTasks(c *gin.Context) {
	filter := &aitaskqueue.TaskFilter{}

	// Parse query parameters
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
		filter.Limit = 50 // Default limit
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	tasks, err := h.queue.ListTasks(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	response := make([]TaskResponse, len(tasks))
	for i, task := range tasks {
		response[i] = toTaskResponse(task)
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":  response,
		"total":  len(response),
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// toTaskResponse converts a Task to TaskResponse
func toTaskResponse(task *aitaskqueue.Task) TaskResponse {
	return TaskResponse{
		ID:           task.ID,
		Topic:        task.Topic,
		Status:       string(task.Status),
		Priority:     task.Priority,
		RetryCount:   task.RetryCount,
		MaxRetries:   task.MaxRetries,
		AgentID:      task.AgentID,
		ErrorMessage: task.ErrorMessage,
		ErrorCode:    task.ErrorCode,
		CreatedAt:    task.CreatedAt,
		StartedAt:    task.StartedAt,
		CompletedAt:  task.CompletedAt,
		TimeoutAt:    task.TimeoutAt,
	}
}

