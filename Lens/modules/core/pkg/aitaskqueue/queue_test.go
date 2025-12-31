package aitaskqueue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStatus_Constants(t *testing.T) {
	assert.Equal(t, TaskStatus("pending"), TaskStatusPending)
	assert.Equal(t, TaskStatus("processing"), TaskStatusProcessing)
	assert.Equal(t, TaskStatus("completed"), TaskStatusCompleted)
	assert.Equal(t, TaskStatus("failed"), TaskStatusFailed)
	assert.Equal(t, TaskStatus("cancelled"), TaskStatusCancelled)
}

func TestTask_TableName(t *testing.T) {
	task := Task{}
	assert.Equal(t, "ai_tasks", task.TableName())
}

func TestTask_BeforeSave(t *testing.T) {
	t.Run("with context", func(t *testing.T) {
		task := &Task{
			Context: aitopics.RequestContext{
				ClusterID: "cluster-1",
				TenantID:  "tenant-1",
			},
		}

		err := task.BeforeSave()
		require.NoError(t, err)
		assert.NotEmpty(t, task.ContextJSON)
		assert.Contains(t, task.ContextJSON, "cluster-1")
		assert.Contains(t, task.ContextJSON, "tenant-1")
	})

	t.Run("without context", func(t *testing.T) {
		task := &Task{
			Context: aitopics.RequestContext{},
		}

		err := task.BeforeSave()
		require.NoError(t, err)
		assert.Empty(t, task.ContextJSON)
	})
}

func TestTask_AfterFind(t *testing.T) {
	t.Run("with context JSON", func(t *testing.T) {
		task := &Task{
			ContextJSON: `{"cluster_id":"cluster-1","tenant_id":"tenant-1"}`,
		}

		err := task.AfterFind()
		require.NoError(t, err)
		assert.Equal(t, "cluster-1", task.Context.ClusterID)
		assert.Equal(t, "tenant-1", task.Context.TenantID)
	})

	t.Run("without context JSON", func(t *testing.T) {
		task := &Task{
			ContextJSON: "",
		}

		err := task.AfterFind()
		require.NoError(t, err)
		assert.Empty(t, task.Context.ClusterID)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		task := &Task{
			ContextJSON: "invalid json",
		}

		err := task.AfterFind()
		assert.Error(t, err)
	})
}

func TestTask_IsCompleted(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   bool
	}{
		{TaskStatusPending, false},
		{TaskStatusProcessing, false},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			task := &Task{Status: tt.status}
			assert.Equal(t, tt.want, task.IsCompleted())
		})
	}
}

func TestTask_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		status     TaskStatus
		retryCount int
		maxRetries int
		want       bool
	}{
		{"failed with retries left", TaskStatusFailed, 1, 3, true},
		{"failed no retries left", TaskStatusFailed, 3, 3, false},
		{"failed exceeded retries", TaskStatusFailed, 5, 3, false},
		{"completed", TaskStatusCompleted, 0, 3, false},
		{"pending", TaskStatusPending, 0, 3, false},
		{"processing", TaskStatusProcessing, 0, 3, false},
		{"cancelled", TaskStatusCancelled, 0, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				Status:     tt.status,
				RetryCount: tt.retryCount,
				MaxRetries: tt.maxRetries,
			}
			assert.Equal(t, tt.want, task.IsRetryable())
		})
	}
}

func TestDefaultQueueConfig(t *testing.T) {
	cfg := DefaultQueueConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 5*time.Minute, cfg.DefaultTimeout)
	assert.Equal(t, 3, cfg.DefaultMaxRetries)
	assert.Equal(t, 7, cfg.RetentionDays)
	assert.Equal(t, 1*time.Hour, cfg.CleanupInterval)
	assert.Equal(t, 1*time.Minute, cfg.TimeoutCheckInterval)
}

func TestPublishOptions(t *testing.T) {
	opts := &PublishOptions{
		Topic:      "test.topic",
		Payload:    json.RawMessage(`{"key":"value"}`),
		Context:    aitopics.RequestContext{ClusterID: "cluster-1"},
		Priority:   1,
		MaxRetries: 5,
		Timeout:    10 * time.Minute,
	}

	assert.Equal(t, "test.topic", opts.Topic)
	assert.Equal(t, 1, opts.Priority)
	assert.Equal(t, 5, opts.MaxRetries)
	assert.Equal(t, 10*time.Minute, opts.Timeout)
}

func TestTaskFilter(t *testing.T) {
	status := TaskStatusPending
	now := time.Now()

	filter := &TaskFilter{
		Status:        &status,
		Topic:         "test.topic",
		Topics:        []string{"topic1", "topic2"},
		AgentID:       "agent-1",
		CreatedAfter:  &now,
		CreatedBefore: &now,
		Limit:         10,
		Offset:        5,
	}

	assert.Equal(t, TaskStatusPending, *filter.Status)
	assert.Equal(t, "test.topic", filter.Topic)
	assert.Len(t, filter.Topics, 2)
	assert.Equal(t, "agent-1", filter.AgentID)
	assert.Equal(t, 10, filter.Limit)
	assert.Equal(t, 5, filter.Offset)
}

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrTaskNotFound)
	assert.NotNil(t, ErrTaskNotCompleted)
	assert.NotNil(t, ErrQueueFull)

	assert.Contains(t, ErrTaskNotFound.Error(), "not found")
	assert.Contains(t, ErrTaskNotCompleted.Error(), "not completed")
	assert.Contains(t, ErrQueueFull.Error(), "full")
}

func TestTask_Fields(t *testing.T) {
	now := time.Now()
	startedAt := now.Add(-1 * time.Minute)
	completedAt := now

	task := &Task{
		ID:            "task-123",
		Topic:         "test.topic",
		Status:        TaskStatusCompleted,
		Priority:      1,
		InputPayload:  json.RawMessage(`{"input":"data"}`),
		OutputPayload: json.RawMessage(`{"output":"result"}`),
		ErrorMessage:  "",
		ErrorCode:     0,
		RetryCount:    2,
		MaxRetries:    3,
		AgentID:       "agent-1",
		Context: aitopics.RequestContext{
			ClusterID: "cluster-1",
			TenantID:  "tenant-1",
		},
		CreatedAt:   now.Add(-2 * time.Minute),
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
		TimeoutAt:   now.Add(5 * time.Minute),
	}

	assert.Equal(t, "task-123", task.ID)
	assert.Equal(t, "test.topic", task.Topic)
	assert.Equal(t, TaskStatusCompleted, task.Status)
	assert.Equal(t, 1, task.Priority)
	assert.Equal(t, 2, task.RetryCount)
	assert.Equal(t, 3, task.MaxRetries)
	assert.Equal(t, "agent-1", task.AgentID)
	assert.Equal(t, "cluster-1", task.Context.ClusterID)
	assert.NotNil(t, task.StartedAt)
	assert.NotNil(t, task.CompletedAt)
}

func TestQueueConfig_Fields(t *testing.T) {
	cfg := &QueueConfig{
		DefaultTimeout:       10 * time.Minute,
		DefaultMaxRetries:    5,
		RetentionDays:        14,
		CleanupInterval:      2 * time.Hour,
		TimeoutCheckInterval: 2 * time.Minute,
	}

	assert.Equal(t, 10*time.Minute, cfg.DefaultTimeout)
	assert.Equal(t, 5, cfg.DefaultMaxRetries)
	assert.Equal(t, 14, cfg.RetentionDays)
	assert.Equal(t, 2*time.Hour, cfg.CleanupInterval)
	assert.Equal(t, 2*time.Minute, cfg.TimeoutCheckInterval)
}
