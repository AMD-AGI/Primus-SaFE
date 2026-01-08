// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitaskqueue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockQueue implements Queue interface for testing
type MockQueue struct {
	publishFunc             func(ctx context.Context, topic string, payload json.RawMessage, reqCtx aitopics.RequestContext) (string, error)
	publishWithOptionsFunc  func(ctx context.Context, opts *PublishOptions) (string, error)
	getTaskFunc             func(ctx context.Context, taskID string) (*Task, error)
	getResultFunc           func(ctx context.Context, taskID string) (*aitopics.Response, error)
	claimTaskFunc           func(ctx context.Context, topics []string, agentID string) (*Task, error)
	completeTaskFunc        func(ctx context.Context, taskID string, result *aitopics.Response) error
	failTaskFunc            func(ctx context.Context, taskID string, errorCode int, errorMsg string) error
	cancelTaskFunc          func(ctx context.Context, taskID string) error
	listTasksFunc           func(ctx context.Context, filter *TaskFilter) ([]*Task, error)
	countTasksFunc          func(ctx context.Context, filter *TaskFilter) (int64, error)
	handleTimeoutsFunc      func(ctx context.Context) (int, error)
	cleanupFunc             func(ctx context.Context, olderThan time.Duration) (int, error)
}

func (m *MockQueue) Publish(ctx context.Context, topic string, payload json.RawMessage, reqCtx aitopics.RequestContext) (string, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, topic, payload, reqCtx)
	}
	return "mock-task-id", nil
}

func (m *MockQueue) PublishWithOptions(ctx context.Context, opts *PublishOptions) (string, error) {
	if m.publishWithOptionsFunc != nil {
		return m.publishWithOptionsFunc(ctx, opts)
	}
	return "mock-task-id", nil
}

func (m *MockQueue) GetTask(ctx context.Context, taskID string) (*Task, error) {
	if m.getTaskFunc != nil {
		return m.getTaskFunc(ctx, taskID)
	}
	return &Task{ID: taskID, Status: TaskStatusCompleted}, nil
}

func (m *MockQueue) GetResult(ctx context.Context, taskID string) (*aitopics.Response, error) {
	if m.getResultFunc != nil {
		return m.getResultFunc(ctx, taskID)
	}
	return &aitopics.Response{RequestID: taskID, Status: aitopics.StatusSuccess}, nil
}

func (m *MockQueue) ClaimTask(ctx context.Context, topics []string, agentID string) (*Task, error) {
	if m.claimTaskFunc != nil {
		return m.claimTaskFunc(ctx, topics, agentID)
	}
	return nil, nil
}

func (m *MockQueue) CompleteTask(ctx context.Context, taskID string, result *aitopics.Response) error {
	if m.completeTaskFunc != nil {
		return m.completeTaskFunc(ctx, taskID, result)
	}
	return nil
}

func (m *MockQueue) FailTask(ctx context.Context, taskID string, errorCode int, errorMsg string) error {
	if m.failTaskFunc != nil {
		return m.failTaskFunc(ctx, taskID, errorCode, errorMsg)
	}
	return nil
}

func (m *MockQueue) CancelTask(ctx context.Context, taskID string) error {
	if m.cancelTaskFunc != nil {
		return m.cancelTaskFunc(ctx, taskID)
	}
	return nil
}

func (m *MockQueue) ListTasks(ctx context.Context, filter *TaskFilter) ([]*Task, error) {
	if m.listTasksFunc != nil {
		return m.listTasksFunc(ctx, filter)
	}
	return nil, nil
}

func (m *MockQueue) CountTasks(ctx context.Context, filter *TaskFilter) (int64, error) {
	if m.countTasksFunc != nil {
		return m.countTasksFunc(ctx, filter)
	}
	return 0, nil
}

func (m *MockQueue) HandleTimeouts(ctx context.Context) (int, error) {
	if m.handleTimeoutsFunc != nil {
		return m.handleTimeoutsFunc(ctx)
	}
	return 0, nil
}

func (m *MockQueue) Cleanup(ctx context.Context, olderThan time.Duration) (int, error) {
	if m.cleanupFunc != nil {
		return m.cleanupFunc(ctx, olderThan)
	}
	return 0, nil
}

func TestDefaultPublisherConfig(t *testing.T) {
	cfg := DefaultPublisherConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 0, cfg.DefaultPriority)
	assert.Equal(t, 3, cfg.DefaultMaxRetries)
	assert.Equal(t, 5*time.Minute, cfg.DefaultTimeout)
}

func TestNewPublisher(t *testing.T) {
	queue := &MockQueue{}

	t.Run("with nil config", func(t *testing.T) {
		pub := NewPublisher(queue, nil)
		assert.NotNil(t, pub)
		assert.NotNil(t, pub.config)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &PublisherConfig{
			DefaultPriority:   2,
			DefaultMaxRetries: 5,
			DefaultTimeout:    10 * time.Minute,
		}
		pub := NewPublisher(queue, cfg)
		assert.Equal(t, 2, pub.config.DefaultPriority)
	})
}

func TestPublisher_Publish(t *testing.T) {
	var capturedOpts *PublishOptions
	queue := &MockQueue{
		publishWithOptionsFunc: func(ctx context.Context, opts *PublishOptions) (string, error) {
			capturedOpts = opts
			return "task-123", nil
		},
	}

	pub := NewPublisher(queue, nil)
	taskID, err := pub.Publish(context.Background(), "test.topic", map[string]string{"key": "value"})

	require.NoError(t, err)
	assert.Equal(t, "task-123", taskID)
	assert.Equal(t, "test.topic", capturedOpts.Topic)
	assert.Equal(t, 0, capturedOpts.Priority)
	assert.Equal(t, 3, capturedOpts.MaxRetries)
}

func TestPublisher_PublishWithPriority(t *testing.T) {
	var capturedOpts *PublishOptions
	queue := &MockQueue{
		publishWithOptionsFunc: func(ctx context.Context, opts *PublishOptions) (string, error) {
			capturedOpts = opts
			return "task-123", nil
		},
	}

	pub := NewPublisher(queue, nil)
	taskID, err := pub.PublishWithPriority(context.Background(), "test.topic", map[string]string{"key": "value"}, 5)

	require.NoError(t, err)
	assert.Equal(t, "task-123", taskID)
	assert.Equal(t, 5, capturedOpts.Priority)
}

func TestPublisher_PublishWithTimeout(t *testing.T) {
	var capturedOpts *PublishOptions
	queue := &MockQueue{
		publishWithOptionsFunc: func(ctx context.Context, opts *PublishOptions) (string, error) {
			capturedOpts = opts
			return "task-123", nil
		},
	}

	pub := NewPublisher(queue, nil)
	taskID, err := pub.PublishWithTimeout(context.Background(), "test.topic", map[string]string{"key": "value"}, 15*time.Minute)

	require.NoError(t, err)
	assert.Equal(t, "task-123", taskID)
	assert.Equal(t, 15*time.Minute, capturedOpts.Timeout)
}

func TestPublisher_PublishBatch(t *testing.T) {
	callCount := 0
	queue := &MockQueue{
		publishWithOptionsFunc: func(ctx context.Context, opts *PublishOptions) (string, error) {
			callCount++
			return "task-" + string(rune('0'+callCount)), nil
		},
	}

	pub := NewPublisher(queue, nil)
	items := []BatchPublishItem{
		{Topic: "topic1", Payload: map[string]string{"key": "value1"}},
		{Topic: "topic2", Payload: map[string]string{"key": "value2"}},
		{Topic: "topic3", Payload: map[string]string{"key": "value3"}},
	}

	ids, err := pub.PublishBatch(context.Background(), items)
	require.NoError(t, err)
	assert.Len(t, ids, 3)
	assert.Equal(t, 3, callCount)
}

func TestPublisher_PublishBatch_PartialFailure(t *testing.T) {
	callCount := 0
	queue := &MockQueue{
		publishWithOptionsFunc: func(ctx context.Context, opts *PublishOptions) (string, error) {
			callCount++
			if callCount == 2 {
				return "", assert.AnError
			}
			return "task-" + string(rune('0'+callCount)), nil
		},
	}

	pub := NewPublisher(queue, nil)
	items := []BatchPublishItem{
		{Topic: "topic1", Payload: "data1"},
		{Topic: "topic2", Payload: "data2"},
		{Topic: "topic3", Payload: "data3"},
	}

	ids, err := pub.PublishBatch(context.Background(), items)
	assert.Error(t, err)
	assert.Len(t, ids, 1) // Only first task succeeded
}

func TestPublisher_PublishInvalidPayload(t *testing.T) {
	queue := &MockQueue{}
	pub := NewPublisher(queue, nil)

	// Channel cannot be serialized to JSON
	invalidPayload := make(chan int)
	_, err := pub.Publish(context.Background(), "test.topic", invalidPayload)
	assert.Error(t, err)
}

func TestPublisher_buildContext(t *testing.T) {
	queue := &MockQueue{}
	pub := NewPublisher(queue, nil)

	ctx := context.Background()
	ctx = WithClusterID(ctx, "cluster-1")
	ctx = WithTenantID(ctx, "tenant-1")
	ctx = WithTraceID(ctx, "trace-1")

	reqCtx := pub.buildContext(ctx)

	assert.Equal(t, "cluster-1", reqCtx.ClusterID)
	assert.Equal(t, "tenant-1", reqCtx.TenantID)
	assert.Equal(t, "trace-1", reqCtx.TraceID)
}

func TestPublisher_buildContext_CustomBuilder(t *testing.T) {
	queue := &MockQueue{}
	cfg := &PublisherConfig{
		ContextBuilder: func(ctx context.Context) aitopics.RequestContext {
			return aitopics.RequestContext{
				ClusterID: "custom-cluster",
				TenantID:  "custom-tenant",
			}
		},
	}
	pub := NewPublisher(queue, cfg)

	reqCtx := pub.buildContext(context.Background())

	assert.Equal(t, "custom-cluster", reqCtx.ClusterID)
	assert.Equal(t, "custom-tenant", reqCtx.TenantID)
}

func TestWithClusterID(t *testing.T) {
	ctx := context.Background()
	ctx = WithClusterID(ctx, "cluster-123")

	value := ctx.Value(contextKeyClusterID)
	assert.Equal(t, "cluster-123", value)
}

func TestWithTenantID(t *testing.T) {
	ctx := context.Background()
	ctx = WithTenantID(ctx, "tenant-456")

	value := ctx.Value(contextKeyTenantID)
	assert.Equal(t, "tenant-456", value)
}

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-789")

	value := ctx.Value(contextKeyTraceID)
	assert.Equal(t, "trace-789", value)
}

func TestBatchPublishItem(t *testing.T) {
	item := BatchPublishItem{
		Topic:   "test.topic",
		Payload: map[string]string{"key": "value"},
	}

	assert.Equal(t, "test.topic", item.Topic)
	assert.NotNil(t, item.Payload)
}

