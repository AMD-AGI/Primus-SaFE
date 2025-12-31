package aitaskqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// Publisher provides a convenient interface for publishing tasks
type Publisher struct {
	queue  Queue
	config *PublisherConfig
}

// PublisherConfig contains configuration for the publisher
type PublisherConfig struct {
	// Default priority for tasks
	DefaultPriority int

	// Default max retries
	DefaultMaxRetries int

	// Default timeout
	DefaultTimeout time.Duration

	// Context builder
	ContextBuilder func(ctx context.Context) aitopics.RequestContext
}

// DefaultPublisherConfig returns default publisher configuration
func DefaultPublisherConfig() *PublisherConfig {
	return &PublisherConfig{
		DefaultPriority:   0,
		DefaultMaxRetries: 3,
		DefaultTimeout:    5 * time.Minute,
	}
}

// NewPublisher creates a new task publisher
func NewPublisher(queue Queue, config *PublisherConfig) *Publisher {
	if config == nil {
		config = DefaultPublisherConfig()
	}
	return &Publisher{
		queue:  queue,
		config: config,
	}
}

// Publish publishes a task with default settings
func (p *Publisher) Publish(ctx context.Context, topic string, payload interface{}) (string, error) {
	return p.PublishWithPriority(ctx, topic, payload, p.config.DefaultPriority)
}

// PublishWithPriority publishes a task with specified priority
func (p *Publisher) PublishWithPriority(ctx context.Context, topic string, payload interface{}, priority int) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	reqCtx := p.buildContext(ctx)

	return p.queue.PublishWithOptions(ctx, &PublishOptions{
		Topic:      topic,
		Payload:    payloadBytes,
		Context:    reqCtx,
		Priority:   priority,
		MaxRetries: p.config.DefaultMaxRetries,
		Timeout:    p.config.DefaultTimeout,
	})
}

// PublishWithTimeout publishes a task with specified timeout
func (p *Publisher) PublishWithTimeout(ctx context.Context, topic string, payload interface{}, timeout time.Duration) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	reqCtx := p.buildContext(ctx)

	return p.queue.PublishWithOptions(ctx, &PublishOptions{
		Topic:      topic,
		Payload:    payloadBytes,
		Context:    reqCtx,
		Priority:   p.config.DefaultPriority,
		MaxRetries: p.config.DefaultMaxRetries,
		Timeout:    timeout,
	})
}

// PublishBatch publishes multiple tasks
func (p *Publisher) PublishBatch(ctx context.Context, tasks []BatchPublishItem) ([]string, error) {
	ids := make([]string, 0, len(tasks))

	for _, item := range tasks {
		id, err := p.Publish(ctx, item.Topic, item.Payload)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// BatchPublishItem represents an item to publish in a batch
type BatchPublishItem struct {
	Topic   string
	Payload interface{}
}

// buildContext builds the request context
func (p *Publisher) buildContext(ctx context.Context) aitopics.RequestContext {
	if p.config.ContextBuilder != nil {
		return p.config.ContextBuilder(ctx)
	}

	reqCtx := aitopics.RequestContext{}

	// Extract common values from context
	if clusterID, ok := ctx.Value(contextKeyClusterID).(string); ok {
		reqCtx.ClusterID = clusterID
	}
	if tenantID, ok := ctx.Value(contextKeyTenantID).(string); ok {
		reqCtx.TenantID = tenantID
	}
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
