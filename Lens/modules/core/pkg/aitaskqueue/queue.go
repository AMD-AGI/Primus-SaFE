// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitaskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// Common errors
var (
	ErrTaskNotFound     = errors.New("task not found")
	ErrTaskNotCompleted = errors.New("task not completed yet")
	ErrQueueFull        = errors.New("queue is full")
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// Task represents an async task in the queue
type Task struct {
	ID            string                  `json:"id" gorm:"primaryKey;size:64"`
	Topic         string                  `json:"topic" gorm:"size:128;index"`
	Status        TaskStatus              `json:"status" gorm:"size:32;index"`
	Priority      int                     `json:"priority" gorm:"default:0"`
	InputPayload  json.RawMessage         `json:"input_payload" gorm:"type:jsonb"`
	OutputPayload json.RawMessage         `json:"output_payload,omitempty" gorm:"type:jsonb"`
	ErrorMessage  string                  `json:"error_message,omitempty" gorm:"size:1024"`
	ErrorCode     int                     `json:"error_code,omitempty"`
	RetryCount    int                     `json:"retry_count" gorm:"default:0"`
	MaxRetries    int                     `json:"max_retries" gorm:"default:3"`
	AgentID       string                  `json:"agent_id,omitempty" gorm:"size:128"`
	ContextJSON   string                  `json:"-" gorm:"column:context;type:jsonb"`
	Context       aitopics.RequestContext `json:"context" gorm:"-"`
	CreatedAt     time.Time               `json:"created_at" gorm:"index"`
	StartedAt     *time.Time              `json:"started_at,omitempty"`
	CompletedAt   *time.Time              `json:"completed_at,omitempty"`
	TimeoutAt     time.Time               `json:"timeout_at" gorm:"index"`
}

// TableName returns the table name for GORM
func (Task) TableName() string {
	return "ai_tasks"
}

// BeforeSave serializes the context to JSON
func (t *Task) BeforeSave() error {
	if t.Context.ClusterID != "" || t.Context.TenantID != "" {
		contextBytes, err := json.Marshal(t.Context)
		if err != nil {
			return err
		}
		t.ContextJSON = string(contextBytes)
	}
	return nil
}

// AfterFind deserializes the context from JSON
func (t *Task) AfterFind() error {
	if t.ContextJSON != "" {
		return json.Unmarshal([]byte(t.ContextJSON), &t.Context)
	}
	return nil
}

// IsCompleted returns true if the task is in a terminal state
func (t *Task) IsCompleted() bool {
	return t.Status == TaskStatusCompleted ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusCancelled
}

// IsRetryable returns true if the task can be retried
func (t *Task) IsRetryable() bool {
	return t.Status == TaskStatusFailed && t.RetryCount < t.MaxRetries
}

// Queue defines the interface for task queue operations
type Queue interface {
	// Publish adds a new task to the queue
	Publish(ctx context.Context, topic string, payload json.RawMessage, reqCtx aitopics.RequestContext) (taskID string, err error)

	// PublishWithOptions adds a new task with options
	PublishWithOptions(ctx context.Context, opts *PublishOptions) (taskID string, err error)

	// GetTask retrieves a task by ID
	GetTask(ctx context.Context, taskID string) (*Task, error)

	// GetResult retrieves the result of a completed task
	GetResult(ctx context.Context, taskID string) (*aitopics.Response, error)

	// ClaimTask claims a pending task for processing
	ClaimTask(ctx context.Context, topics []string, agentID string) (*Task, error)

	// CompleteTask marks a task as completed with result
	CompleteTask(ctx context.Context, taskID string, result *aitopics.Response) error

	// FailTask marks a task as failed
	FailTask(ctx context.Context, taskID string, errorCode int, errorMsg string) error

	// CancelTask cancels a pending task
	CancelTask(ctx context.Context, taskID string) error

	// ListTasks lists tasks with optional filters
	ListTasks(ctx context.Context, filter *TaskFilter) ([]*Task, error)

	// CountTasks counts tasks by status
	CountTasks(ctx context.Context, filter *TaskFilter) (int64, error)

	// HandleTimeouts moves timed-out tasks back to pending
	HandleTimeouts(ctx context.Context) (count int, err error)

	// Cleanup removes old completed tasks
	Cleanup(ctx context.Context, olderThan time.Duration) (count int, err error)
}

// PublishOptions contains options for publishing a task
type PublishOptions struct {
	Topic       string
	Payload     json.RawMessage
	Context     aitopics.RequestContext
	Priority    int
	MaxRetries  int
	Timeout     time.Duration
}

// TaskFilter contains filters for listing tasks
type TaskFilter struct {
	Status    *TaskStatus
	Topic     string
	Topics    []string
	AgentID   string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit     int
	Offset    int
}

// QueueConfig contains configuration for the task queue
type QueueConfig struct {
	// Default timeout for tasks
	DefaultTimeout time.Duration `json:"default_timeout" yaml:"default_timeout"`

	// Default max retries
	DefaultMaxRetries int `json:"default_max_retries" yaml:"default_max_retries"`

	// Cleanup settings
	RetentionDays     int           `json:"retention_days" yaml:"retention_days"`
	CleanupInterval   time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`

	// Timeout check interval
	TimeoutCheckInterval time.Duration `json:"timeout_check_interval" yaml:"timeout_check_interval"`
}

// DefaultQueueConfig returns default queue configuration
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		DefaultTimeout:       5 * time.Minute,
		DefaultMaxRetries:    3,
		RetentionDays:        7,
		CleanupInterval:      1 * time.Hour,
		TimeoutCheckInterval: 1 * time.Minute,
	}
}

