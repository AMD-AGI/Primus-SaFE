package aiclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 30*time.Second, cfg.DefaultTimeout)
	assert.Equal(t, 2, cfg.RetryCount)
	assert.Equal(t, 1*time.Second, cfg.RetryDelay)
	assert.True(t, cfg.CircuitBreakerEnabled)
	assert.Equal(t, 5, cfg.CircuitBreakerThreshold)
	assert.Equal(t, 60*time.Second, cfg.CircuitBreakerTimeout)
	assert.True(t, cfg.DegradationEnabled)
}

func TestNew(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		registry := airegistry.NewMemoryStore()
		client := New(nil, registry, nil)
		assert.NotNil(t, client)
		assert.NotNil(t, client.config)
		assert.NotNil(t, client.circuitBreaker)
		assert.NotNil(t, client.degradation)
	})

	t.Run("with custom config", func(t *testing.T) {
		registry := airegistry.NewMemoryStore()
		cfg := &ClientConfig{
			DefaultTimeout:          10 * time.Second,
			RetryCount:              5,
			CircuitBreakerEnabled:   false,
			DegradationEnabled:      false,
		}
		client := New(cfg, registry, nil)
		assert.NotNil(t, client)
		assert.Nil(t, client.circuitBreaker)
		assert.Nil(t, client.degradation)
	})
}

func TestNewWithDefaults(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	client := NewWithDefaults(registry)
	assert.NotNil(t, client)
	assert.NotNil(t, client.config)
	assert.NotNil(t, client.circuitBreaker)
}

func TestAIClient_IsAvailable(t *testing.T) {
	ctx := context.Background()
	registry := airegistry.NewMemoryStore()
	client := NewWithDefaults(registry)

	// No agent registered
	assert.False(t, client.IsAvailable(ctx, "test.topic"))

	// Register an agent
	agent := &airegistry.AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   airegistry.AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	// Now should be available
	assert.True(t, client.IsAvailable(ctx, "test.topic"))
	assert.False(t, client.IsAvailable(ctx, "unknown.topic"))
}

func TestAIClient_InvokeAsync_NoTaskQueue(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	client := New(DefaultClientConfig(), registry, nil)

	_, err := client.InvokeAsync(context.Background(), "test.topic", map[string]string{"key": "value"})
	assert.Equal(t, ErrTaskQueueNotConfigured, err)
}

func TestAIClient_GetResult_NoTaskQueue(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	client := New(DefaultClientConfig(), registry, nil)

	_, err := client.GetResult(context.Background(), "task-id")
	assert.Equal(t, ErrTaskQueueNotConfigured, err)
}

func TestAIClient_GetResultWithTimeout_NoTaskQueue(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	client := New(DefaultClientConfig(), registry, nil)

	_, err := client.GetResultWithTimeout(context.Background(), "task-id", 5*time.Second)
	assert.Equal(t, ErrTaskQueueNotConfigured, err)
}

// MockTaskQueue for testing async operations
type MockTaskQueue struct {
	publishFunc   func(ctx context.Context, topic string, payload json.RawMessage, context aitopics.RequestContext) (string, error)
	getTaskFunc   func(ctx context.Context, taskID string) (*Task, error)
	getResultFunc func(ctx context.Context, taskID string) (*aitopics.Response, error)
}

func (m *MockTaskQueue) Publish(ctx context.Context, topic string, payload json.RawMessage, context aitopics.RequestContext) (string, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, topic, payload, context)
	}
	return "mock-task-id", nil
}

func (m *MockTaskQueue) GetTask(ctx context.Context, taskID string) (*Task, error) {
	if m.getTaskFunc != nil {
		return m.getTaskFunc(ctx, taskID)
	}
	return &Task{ID: taskID, Status: TaskStatusCompleted}, nil
}

func (m *MockTaskQueue) GetResult(ctx context.Context, taskID string) (*aitopics.Response, error) {
	if m.getResultFunc != nil {
		return m.getResultFunc(ctx, taskID)
	}
	return &aitopics.Response{
		RequestID: taskID,
		Status:    aitopics.StatusSuccess,
		Code:      aitopics.CodeSuccess,
	}, nil
}

func TestAIClient_InvokeAsync_Success(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	taskQueue := &MockTaskQueue{}
	client := New(DefaultClientConfig(), registry, taskQueue)

	taskID, err := client.InvokeAsync(context.Background(), "test.topic", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.Equal(t, "mock-task-id", taskID)
}

func TestAIClient_GetResult_Success(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	taskQueue := &MockTaskQueue{}
	client := New(DefaultClientConfig(), registry, taskQueue)

	resp, err := client.GetResult(context.Background(), "task-123")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "task-123", resp.RequestID)
}

func TestAIClient_GetResultWithTimeout_Success(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	callCount := 0
	taskQueue := &MockTaskQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			callCount++
			if callCount < 2 {
				return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
			}
			return &Task{ID: taskID, Status: TaskStatusCompleted}, nil
		},
	}
	client := New(DefaultClientConfig(), registry, taskQueue)

	resp, err := client.GetResultWithTimeout(context.Background(), "task-123", 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, callCount >= 2)
}

func TestAIClient_GetResultWithTimeout_Failed(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	taskQueue := &MockTaskQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{
				ID:           taskID,
				Status:       TaskStatusFailed,
				ErrorCode:    aitopics.CodeInternalError,
				ErrorMessage: "task failed",
			}, nil
		},
	}
	client := New(DefaultClientConfig(), registry, taskQueue)

	resp, err := client.GetResultWithTimeout(context.Background(), "task-123", 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, aitopics.StatusError, resp.Status)
}

func TestAIClient_GetResultWithTimeout_Cancelled(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	taskQueue := &MockTaskQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{ID: taskID, Status: TaskStatusCancelled}, nil
		},
	}
	client := New(DefaultClientConfig(), registry, taskQueue)

	resp, err := client.GetResultWithTimeout(context.Background(), "task-123", 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, aitopics.StatusError, resp.Status)
	assert.Contains(t, resp.Message, "cancelled")
}

func TestAIClient_GetResultWithTimeout_Timeout(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	taskQueue := &MockTaskQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
		},
	}
	client := New(DefaultClientConfig(), registry, taskQueue)

	_, err := client.GetResultWithTimeout(context.Background(), "task-123", 100*time.Millisecond)
	assert.Equal(t, ErrTimeout, err)
}

func TestAIClient_GetResultWithTimeout_ContextCancelled(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	taskQueue := &MockTaskQueue{
		getTaskFunc: func(ctx context.Context, taskID string) (*Task, error) {
			return &Task{ID: taskID, Status: TaskStatusProcessing}, nil
		},
	}
	client := New(DefaultClientConfig(), registry, taskQueue)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := client.GetResultWithTimeout(ctx, "task-123", 5*time.Second)
	assert.Equal(t, context.Canceled, err)
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

func TestAIClient_buildRequestContext(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	cfg := &ClientConfig{
		ToolEndpoint: "http://localhost:9090/tools",
	}
	client := New(cfg, registry, nil)

	ctx := context.Background()
	ctx = WithClusterID(ctx, "cluster-123")
	ctx = WithTenantID(ctx, "tenant-456")
	ctx = WithTraceID(ctx, "trace-789")

	reqCtx := client.buildRequestContext(ctx)

	assert.Equal(t, "http://localhost:9090/tools", reqCtx.ToolEndpoint)
	assert.Equal(t, "cluster-123", reqCtx.ClusterID)
	assert.Equal(t, "tenant-456", reqCtx.TenantID)
	assert.Equal(t, "trace-789", reqCtx.TraceID)
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{ErrAgentUnavailable, true},
		{ErrTimeout, true},
		{ErrConnectionFailed, true},
		{ErrInvalidRequest, false},
		{ErrCircuitBreakerOpen, false},
		{nil, false},
	}

	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			got := isRetryable(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	assert.Equal(t, TaskStatus("pending"), TaskStatusPending)
	assert.Equal(t, TaskStatus("processing"), TaskStatusProcessing)
	assert.Equal(t, TaskStatus("completed"), TaskStatusCompleted)
	assert.Equal(t, TaskStatus("failed"), TaskStatusFailed)
	assert.Equal(t, TaskStatus("cancelled"), TaskStatusCancelled)
}

func TestTask_Fields(t *testing.T) {
	now := time.Now()
	task := &Task{
		ID:            "task-123",
		Topic:         "test.topic",
		Status:        TaskStatusPending,
		InputPayload:  json.RawMessage(`{"key":"value"}`),
		OutputPayload: json.RawMessage(`{"result":"ok"}`),
		ErrorMessage:  "",
		ErrorCode:     0,
		Context: aitopics.RequestContext{
			ClusterID: "cluster-1",
		},
		CreatedAt: now,
	}

	assert.Equal(t, "task-123", task.ID)
	assert.Equal(t, "test.topic", task.Topic)
	assert.Equal(t, TaskStatusPending, task.Status)
	assert.Equal(t, "cluster-1", task.Context.ClusterID)
}

