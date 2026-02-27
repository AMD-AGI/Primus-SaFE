// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aigateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors
var (
	ErrTaskNotFound = errors.New("task not found")
)

// APIError represents an error response from the AI Gateway.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ai-gateway: HTTP %d: %s", e.StatusCode, e.Message)
}

// --- Producer request/response types ---

// PublishRequest is the body for POST /ai/tasks
type PublishRequest struct {
	Topic      string          `json:"topic"`
	Payload    json.RawMessage `json:"payload"`
	Priority   int             `json:"priority,omitempty"`
	MaxRetries int             `json:"max_retries,omitempty"`
	TimeoutSec int             `json:"timeout_sec,omitempty"`
}

// TaskInfo represents a task returned by the API.
type TaskInfo struct {
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

// TaskStatusInfo is the lightweight status response.
type TaskStatusInfo struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	AgentID      string `json:"agent_id,omitempty"`
	RetryCount   int    `json:"retry_count"`
	MaxRetries   int    `json:"max_retries"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TaskResultInfo is the response from GET /ai/tasks/:id/result.
type TaskResultInfo struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Code   int             `json:"code"`
	Result json.RawMessage `json:"result"`
}

// --- Agent request types ---

// ClaimRequest is the body for POST /ai/tasks/claim
type ClaimRequest struct {
	AgentID string   `json:"agent_id"`
	Topics  []string `json:"topics"`
}

// CompleteRequest is the body for POST /ai/tasks/:id/complete
type CompleteRequest struct {
	Result json.RawMessage `json:"result"`
}

// FailRequest is the body for POST /ai/tasks/:id/fail
type FailRequest struct {
	ErrorCode    int    `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message"`
}

// AgentRegistration is the body for POST /ai/agents/register
type AgentRegistration struct {
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Endpoint    string   `json:"endpoint,omitempty"`
	Topics      []string `json:"topics"`
	Tags        []string `json:"tags,omitempty"`
}
