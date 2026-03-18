// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package aigateway provides a Go client for the AI Gateway HTTP API.
//
// The client supports two roles:
//   - Producer: publish tasks and poll for results
//   - Agent: claim pending tasks, report completion or failure
//
// This package is designed to be self-contained with no framework-specific
// dependencies, making it suitable for extraction as a standalone SDK.
package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for the AI Gateway API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures the Client.
type Option func(*Client)

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(client *Client) { client.httpClient = c }
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(client *Client) { client.httpClient.Timeout = d }
}

// NewClient creates a new AI Gateway client.
// baseURL should be the full URL prefix including version path, e.g.
// "http://ai-gateway:8080/api/v1".
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Producer API ---

// Publish creates a new task in the queue.
func (c *Client) Publish(ctx context.Context, req *PublishRequest) (*TaskInfo, error) {
	var resp TaskInfo
	if err := c.post(ctx, "/ai/tasks", req, &resp); err != nil {
		return nil, fmt.Errorf("publish task: %w", err)
	}
	return &resp, nil
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, taskID string) (*TaskInfo, error) {
	var resp TaskInfo
	if err := c.get(ctx, "/ai/tasks/"+taskID, &resp); err != nil {
		return nil, fmt.Errorf("get task %s: %w", taskID, err)
	}
	return &resp, nil
}

// GetTaskStatus retrieves the lightweight status of a task.
func (c *Client) GetTaskStatus(ctx context.Context, taskID string) (*TaskStatusInfo, error) {
	var resp TaskStatusInfo
	if err := c.get(ctx, "/ai/tasks/"+taskID+"/status", &resp); err != nil {
		return nil, fmt.Errorf("get task status %s: %w", taskID, err)
	}
	return &resp, nil
}

// GetResult retrieves the result of a completed task.
// Returns (nil, nil) if the task is still in progress (HTTP 202).
func (c *Client) GetResult(ctx context.Context, taskID string) (*TaskResultInfo, error) {
	path := "/ai/tasks/" + taskID + "/result"
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get result %s: %w", taskID, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var result TaskResultInfo
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode result: %w", err)
		}
		return &result, nil
	case http.StatusAccepted:
		// Task still in progress
		return nil, nil
	case http.StatusNotFound:
		return nil, ErrTaskNotFound
	default:
		return nil, c.readError(resp)
	}
}

// CancelTask cancels a pending task.
func (c *Client) CancelTask(ctx context.Context, taskID string) error {
	return c.post(ctx, "/ai/tasks/"+taskID+"/cancel", nil, nil)
}

// --- Agent API ---

// Claim attempts to claim the next pending task matching the given topics.
// Returns (nil, nil) when no tasks are available (HTTP 204).
func (c *Client) Claim(ctx context.Context, agentID string, topics []string) (*TaskInfo, error) {
	req := &ClaimRequest{AgentID: agentID, Topics: topics}

	path := "/ai/tasks/claim"
	url := c.baseURL + path

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var task TaskInfo
		if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
			return nil, fmt.Errorf("decode claimed task: %w", err)
		}
		return &task, nil
	case http.StatusNoContent:
		return nil, nil
	default:
		return nil, c.readError(resp)
	}
}

// Complete reports that a task has been successfully completed.
func (c *Client) Complete(ctx context.Context, taskID string, result json.RawMessage) error {
	req := &CompleteRequest{Result: result}
	return c.post(ctx, "/ai/tasks/"+taskID+"/complete", req, nil)
}

// Fail reports that a task has failed.
func (c *Client) Fail(ctx context.Context, taskID string, errorCode int, errorMessage string) error {
	req := &FailRequest{ErrorCode: errorCode, ErrorMessage: errorMessage}
	return c.post(ctx, "/ai/tasks/"+taskID+"/fail", req, nil)
}

// --- Agent Registration API ---

// RegisterAgent registers an agent with the gateway.
func (c *Client) RegisterAgent(ctx context.Context, reg *AgentRegistration) error {
	return c.post(ctx, "/ai/agents/register", reg, nil)
}

// Heartbeat sends a keep-alive signal (re-register with same data).
func (c *Client) Heartbeat(ctx context.Context, reg *AgentRegistration) error {
	return c.RegisterAgent(ctx, reg)
}

// --- Internal HTTP helpers ---

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrTaskNotFound
	}
	if resp.StatusCode >= 400 {
		return c.readError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, in, out interface{}) error {
	url := c.baseURL + path

	var bodyReader io.Reader
	if in != nil {
		body, err := json.Marshal(in)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrTaskNotFound
	}
	if resp.StatusCode >= 400 {
		return c.readError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return &APIError{StatusCode: resp.StatusCode, Message: errResp.Error}
	}
	return &APIError{StatusCode: resp.StatusCode, Message: string(body)}
}
