// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aiclient

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// Client is the main interface for invoking AI agents
type Client interface {
	// InvokeSync invokes an AI agent synchronously and waits for result
	InvokeSync(ctx context.Context, topic string, payload interface{}) (*aitopics.Response, error)

	// InvokeAsync submits a task to the queue and returns immediately
	InvokeAsync(ctx context.Context, topic string, payload interface{}) (taskID string, err error)

	// GetResult retrieves the result of an async task
	GetResult(ctx context.Context, taskID string) (*aitopics.Response, error)

	// GetResultWithTimeout retrieves the result with polling until timeout
	GetResultWithTimeout(ctx context.Context, taskID string, timeout time.Duration) (*aitopics.Response, error)

	// IsAvailable checks if any agent is available for the topic
	IsAvailable(ctx context.Context, topic string) bool
}

// ClientConfig contains configuration for the AI client
type ClientConfig struct {
	// Default timeout for sync invocations
	DefaultTimeout time.Duration `json:"default_timeout" yaml:"default_timeout"`

	// Retry configuration
	RetryCount int           `json:"retry_count" yaml:"retry_count"`
	RetryDelay time.Duration `json:"retry_delay" yaml:"retry_delay"`

	// Circuit breaker configuration
	CircuitBreakerEnabled   bool `json:"circuit_breaker_enabled" yaml:"circuit_breaker_enabled"`
	CircuitBreakerThreshold int  `json:"circuit_breaker_threshold" yaml:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   time.Duration `json:"circuit_breaker_timeout" yaml:"circuit_breaker_timeout"`

	// Graceful degradation
	DegradationEnabled bool `json:"degradation_enabled" yaml:"degradation_enabled"`

	// Tool endpoint (passed to agent in context)
	ToolEndpoint string `json:"tool_endpoint" yaml:"tool_endpoint"`
}

// DefaultClientConfig returns the default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		DefaultTimeout:          30 * time.Second,
		RetryCount:              2,
		RetryDelay:              1 * time.Second,
		CircuitBreakerEnabled:   true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   60 * time.Second,
		DegradationEnabled:      true,
	}
}

// AIClient implements the Client interface
type AIClient struct {
	config         *ClientConfig
	registry       airegistry.Registry
	router         *airegistry.Router
	taskQueue      TaskQueue
	invoker        *Invoker
	circuitBreaker *CircuitBreaker
	degradation    *DegradationHandler
}

// TaskQueue is the interface for async task operations
// This is implemented by aitaskqueue package
type TaskQueue interface {
	// Publish submits a task to the queue
	Publish(ctx context.Context, topic string, payload json.RawMessage, context aitopics.RequestContext) (taskID string, err error)

	// GetTask retrieves a task by ID
	GetTask(ctx context.Context, taskID string) (*Task, error)

	// GetResult retrieves the result of a completed task
	GetResult(ctx context.Context, taskID string) (*aitopics.Response, error)
}

// Task represents an async task
type Task struct {
	ID            string              `json:"id"`
	Topic         string              `json:"topic"`
	Status        TaskStatus          `json:"status"`
	InputPayload  json.RawMessage     `json:"input_payload"`
	OutputPayload json.RawMessage     `json:"output_payload,omitempty"`
	ErrorMessage  string              `json:"error_message,omitempty"`
	ErrorCode     int                 `json:"error_code,omitempty"`
	Context       aitopics.RequestContext `json:"context"`
	CreatedAt     time.Time           `json:"created_at"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	CompletedAt   *time.Time          `json:"completed_at,omitempty"`
}

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// New creates a new AI client
func New(cfg *ClientConfig, registry airegistry.Registry, taskQueue TaskQueue) *AIClient {
	if cfg == nil {
		cfg = DefaultClientConfig()
	}

	client := &AIClient{
		config:    cfg,
		registry:  registry,
		router:    airegistry.NewRouter(registry),
		taskQueue: taskQueue,
		invoker:   NewInvoker(cfg.DefaultTimeout),
	}

	if cfg.CircuitBreakerEnabled {
		client.circuitBreaker = NewCircuitBreaker(
			cfg.CircuitBreakerThreshold,
			cfg.CircuitBreakerTimeout,
		)
	}

	if cfg.DegradationEnabled {
		client.degradation = NewDegradationHandler()
	}

	return client
}

// NewWithDefaults creates a new AI client with default configuration
func NewWithDefaults(registry airegistry.Registry) *AIClient {
	return New(DefaultClientConfig(), registry, nil)
}

// InvokeSync invokes an AI agent synchronously
func (c *AIClient) InvokeSync(ctx context.Context, topic string, payload interface{}) (*aitopics.Response, error) {
	return c.invokeWithRetry(ctx, topic, payload)
}

// InvokeAsync submits a task to the queue
func (c *AIClient) InvokeAsync(ctx context.Context, topic string, payload interface{}) (string, error) {
	if c.taskQueue == nil {
		return "", ErrTaskQueueNotConfigured
	}

	// Serialize payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Create context
	reqCtx := c.buildRequestContext(ctx)

	// Submit to queue
	return c.taskQueue.Publish(ctx, topic, payloadBytes, reqCtx)
}

// GetResult retrieves the result of an async task
func (c *AIClient) GetResult(ctx context.Context, taskID string) (*aitopics.Response, error) {
	if c.taskQueue == nil {
		return nil, ErrTaskQueueNotConfigured
	}

	return c.taskQueue.GetResult(ctx, taskID)
}

// GetResultWithTimeout polls for result until timeout
func (c *AIClient) GetResultWithTimeout(ctx context.Context, taskID string, timeout time.Duration) (*aitopics.Response, error) {
	if c.taskQueue == nil {
		return nil, ErrTaskQueueNotConfigured
	}

	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		task, err := c.taskQueue.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		switch task.Status {
		case TaskStatusCompleted:
			return c.taskQueue.GetResult(ctx, taskID)
		case TaskStatusFailed:
			return aitopics.NewErrorResponse(taskID, task.ErrorCode, task.ErrorMessage), nil
		case TaskStatusCancelled:
			return aitopics.NewErrorResponse(taskID, aitopics.CodeInternalError, "task cancelled"), nil
		}

		// Still pending or processing, wait and retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
			// Increase poll interval for longer waits
			if pollInterval < 5*time.Second {
				pollInterval = pollInterval * 2
			}
		}
	}

	return nil, ErrTimeout
}

// IsAvailable checks if any agent is available for the topic
func (c *AIClient) IsAvailable(ctx context.Context, topic string) bool {
	agent, err := c.router.Route(ctx, topic)
	return err == nil && agent != nil
}

// invokeWithRetry invokes with retry logic
func (c *AIClient) invokeWithRetry(ctx context.Context, topic string, payload interface{}) (*aitopics.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay):
			}
		}

		resp, err := c.doInvoke(ctx, topic, payload)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			break
		}
	}

	// Handle degradation if enabled
	if c.degradation != nil && c.config.DegradationEnabled {
		return c.degradation.Handle(ctx, topic, lastErr)
	}

	return nil, lastErr
}

// doInvoke performs the actual invocation
func (c *AIClient) doInvoke(ctx context.Context, topic string, payload interface{}) (*aitopics.Response, error) {
	// Check circuit breaker
	if c.circuitBreaker != nil && c.circuitBreaker.IsOpen(topic) {
		return nil, ErrCircuitBreakerOpen
	}

	// Route to agent
	agent, err := c.router.Route(ctx, topic)
	if err != nil {
		return nil, err
	}

	// Build request
	reqCtx := c.buildRequestContext(ctx)
	req, err := aitopics.NewRequest(topic, reqCtx, payload)
	if err != nil {
		return nil, err
	}

	// Determine timeout
	timeout := c.config.DefaultTimeout
	if def, ok := aitopics.GetTopicDefinition(topic); ok {
		timeout = def.Timeout
	}

	// Invoke
	resp, err := c.invoker.Invoke(ctx, agent, req, timeout)
	if err != nil {
		// Record failure
		if c.circuitBreaker != nil {
			c.circuitBreaker.RecordFailure(topic)
		}
		return nil, err
	}

	// Record success
	if c.circuitBreaker != nil {
		c.circuitBreaker.RecordSuccess(topic)
	}

	return resp, nil
}

// buildRequestContext builds the request context
func (c *AIClient) buildRequestContext(ctx context.Context) aitopics.RequestContext {
	reqCtx := aitopics.RequestContext{
		ToolEndpoint: c.config.ToolEndpoint,
	}

	// Extract cluster ID from context if available
	if clusterID, ok := ctx.Value(contextKeyClusterID).(string); ok {
		reqCtx.ClusterID = clusterID
	}

	// Extract tenant ID from context if available
	if tenantID, ok := ctx.Value(contextKeyTenantID).(string); ok {
		reqCtx.TenantID = tenantID
	}

	// Extract trace ID from context if available
	if traceID, ok := ctx.Value(contextKeyTraceID).(string); ok {
		reqCtx.TraceID = traceID
	}

	return reqCtx
}

// Context keys
type contextKey string

const (
	contextKeyClusterID contextKey = "cluster_id"
	contextKeyTenantID  contextKey = "tenant_id"
	contextKeyTraceID   contextKey = "trace_id"
)

// WithClusterID adds cluster ID to context
func WithClusterID(ctx context.Context, clusterID string) context.Context {
	return context.WithValue(ctx, contextKeyClusterID, clusterID)
}

// WithTenantID adds tenant ID to context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, contextKeyTenantID, tenantID)
}

// WithTraceID adds trace ID to context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, contextKeyTraceID, traceID)
}

// isRetryable checks if an error is retryable
func isRetryable(err error) bool {
	switch err {
	case ErrAgentUnavailable, ErrTimeout, ErrConnectionFailed:
		return true
	default:
		return false
	}
}

// Global client instance
var globalClient *AIClient

// SetGlobalClient sets the global AI client instance
func SetGlobalClient(client *AIClient) {
	globalClient = client
}

// GetGlobalClient returns the global AI client instance
// Returns nil if not initialized
func GetGlobalClient() *AIClient {
	return globalClient
}

