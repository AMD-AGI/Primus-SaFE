package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// Invoker handles HTTP invocation to AI agents
type Invoker struct {
	client *http.Client
}

// NewInvoker creates a new invoker with the given default timeout
func NewInvoker(defaultTimeout time.Duration) *Invoker {
	if defaultTimeout == 0 {
		defaultTimeout = 30 * time.Second
	}

	return &Invoker{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// Invoke calls the AI agent with the given request
func (i *Invoker) Invoke(ctx context.Context, agent *airegistry.AgentRegistration, req *aitopics.Request, timeout time.Duration) (*aitopics.Response, error) {
	// Serialize request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL
	url := agent.Endpoint + "/invoke"

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Request-ID", req.RequestID)
	httpReq.Header.Set("X-Topic", req.Topic)

	// Create client with specific timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Execute request
	httpResp, err := client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrTimeout
		}
		return nil, ErrConnectionFailed
	}
	defer httpResp.Body.Close()

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if httpResp.StatusCode >= 400 {
		return nil, parseHTTPError(httpResp.StatusCode, body)
	}

	// Parse response
	var resp aitopics.Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// InvokeWithPayload is a convenience method that handles payload serialization
func (i *Invoker) InvokeWithPayload(ctx context.Context, agent *airegistry.AgentRegistration, topic string, reqCtx aitopics.RequestContext, payload interface{}, timeout time.Duration) (*aitopics.Response, error) {
	req, err := aitopics.NewRequest(topic, reqCtx, payload)
	if err != nil {
		return nil, err
	}

	return i.Invoke(ctx, agent, req, timeout)
}

// parseHTTPError converts HTTP error to appropriate error
func parseHTTPError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusBadRequest:
		return ErrInvalidRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrAgentUnavailable
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return ErrAgentUnavailable
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		// Try to parse error message from body
		var errResp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil {
			msg := errResp.Message
			if msg == "" {
				msg = errResp.Error
			}
			if msg != "" {
				return fmt.Errorf("agent error: %s", msg)
			}
		}
		return fmt.Errorf("agent returned status %d", statusCode)
	}
}

// BatchInvoker handles batch invocations
type BatchInvoker struct {
	invoker     *Invoker
	concurrency int
}

// NewBatchInvoker creates a new batch invoker
func NewBatchInvoker(invoker *Invoker, concurrency int) *BatchInvoker {
	if concurrency <= 0 {
		concurrency = 5
	}
	return &BatchInvoker{
		invoker:     invoker,
		concurrency: concurrency,
	}
}

// BatchRequest represents a single request in a batch
type BatchRequest struct {
	Agent   *airegistry.AgentRegistration
	Request *aitopics.Request
	Timeout time.Duration
}

// BatchResult represents the result of a batch request
type BatchResult struct {
	Request  *BatchRequest
	Response *aitopics.Response
	Error    error
}

// InvokeBatch invokes multiple requests concurrently
func (b *BatchInvoker) InvokeBatch(ctx context.Context, requests []*BatchRequest) []*BatchResult {
	results := make([]*BatchResult, len(requests))

	// Simple sequential execution if concurrency is 1
	if b.concurrency == 1 {
		for i, req := range requests {
			resp, err := b.invoker.Invoke(ctx, req.Agent, req.Request, req.Timeout)
			results[i] = &BatchResult{
				Request:  req,
				Response: resp,
				Error:    err,
			}
		}
		return results
	}

	// Concurrent execution with semaphore
	type indexedResult struct {
		index  int
		result *BatchResult
	}

	resultCh := make(chan indexedResult, len(requests))
	sem := make(chan struct{}, b.concurrency)

	for i, req := range requests {
		go func(idx int, r *BatchRequest) {
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := b.invoker.Invoke(ctx, r.Agent, r.Request, r.Timeout)
			resultCh <- indexedResult{
				index: idx,
				result: &BatchResult{
					Request:  r,
					Response: resp,
					Error:    err,
				},
			}
		}(i, req)
	}

	// Collect results
	for range requests {
		ir := <-resultCh
		results[ir.index] = ir.result
	}

	return results
}

