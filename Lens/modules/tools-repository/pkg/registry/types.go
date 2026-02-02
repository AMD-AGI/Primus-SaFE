// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"encoding/json"
	"time"
)

// ProviderType represents the type of tool provider
type ProviderType string

const (
	ProviderMCP  ProviderType = "mcp"
	ProviderHTTP ProviderType = "http"
	ProviderA2A  ProviderType = "a2a"
)

// Scope represents the visibility scope of a tool
type Scope string

const (
	ScopePlatform Scope = "platform"
	ScopeTeam     Scope = "team"
	ScopeUser     Scope = "user"
)

// Tool represents a registered tool in the repository
type Tool struct {
	ID             string            `json:"id" db:"id"`
	Name           string            `json:"name" db:"name"`
	Version        string            `json:"version" db:"version"`
	Description    string            `json:"description" db:"description"`
	ProviderType   ProviderType      `json:"provider_type" db:"provider_type"`
	ProviderConfig json.RawMessage   `json:"provider_config" db:"provider_config"`
	InputSchema    json.RawMessage   `json:"input_schema,omitempty" db:"input_schema"`
	OutputSchema   json.RawMessage   `json:"output_schema,omitempty" db:"output_schema"`
	Category       string            `json:"category,omitempty" db:"category"`
	Tags           []string          `json:"tags,omitempty" db:"tags"`
	Scope          Scope             `json:"scope" db:"scope"`
	ScopeID        string            `json:"scope_id,omitempty" db:"scope_id"`
	Enabled        bool              `json:"enabled" db:"enabled"`
	CreatedAt      time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at" db:"updated_at"`
	CreatedBy      string            `json:"created_by,omitempty" db:"created_by"`
}

// MCPProviderConfig configuration for MCP provider
type MCPProviderConfig struct {
	ServerURL   string            `json:"server_url"`
	Transport   string            `json:"transport"` // "sse", "stdio", "websocket"
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// HTTPProviderConfig configuration for HTTP provider
type HTTPProviderConfig struct {
	URL         string            `json:"url"`
	Method      string            `json:"method"` // "GET", "POST", etc.
	Headers     map[string]string `json:"headers,omitempty"`
	AuthType    string            `json:"auth_type,omitempty"` // "none", "bearer", "basic", "api_key"
	AuthConfig  json.RawMessage   `json:"auth_config,omitempty"`
	Timeout     int               `json:"timeout_seconds,omitempty"`
	RetryCount  int               `json:"retry_count,omitempty"`
}

// A2AProviderConfig configuration for Agent-to-Agent provider
type A2AProviderConfig struct {
	AgentURL     string            `json:"agent_url"`
	AgentCard    json.RawMessage   `json:"agent_card,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
}

// ToolExecution represents a tool execution record
type ToolExecution struct {
	ID           string          `json:"id" db:"id"`
	ToolID       string          `json:"tool_id,omitempty" db:"tool_id"`
	ToolName     string          `json:"tool_name" db:"tool_name"`
	UserID       string          `json:"user_id,omitempty" db:"user_id"`
	SessionID    string          `json:"session_id,omitempty" db:"session_id"`
	Input        json.RawMessage `json:"input,omitempty" db:"input"`
	Output       json.RawMessage `json:"output,omitempty" db:"output"`
	Status       string          `json:"status" db:"status"`
	ErrorMessage string          `json:"error_message,omitempty" db:"error_message"`
	DurationMs   int             `json:"duration_ms,omitempty" db:"duration_ms"`
	StartedAt    time.Time       `json:"started_at" db:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// ToolStats represents aggregated tool statistics
type ToolStats struct {
	ToolName        string     `json:"tool_name" db:"tool_name"`
	TotalExecutions int64      `json:"total_executions" db:"total_executions"`
	SuccessCount    int64      `json:"success_count" db:"success_count"`
	ErrorCount      int64      `json:"error_count" db:"error_count"`
	AvgDurationMs   int        `json:"avg_duration_ms,omitempty" db:"avg_duration_ms"`
	P95DurationMs   int        `json:"p95_duration_ms,omitempty" db:"p95_duration_ms"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// ToolAccess represents access control for a tool
type ToolAccess struct {
	ID            string    `json:"id" db:"id"`
	ToolID        string    `json:"tool_id" db:"tool_id"`
	PrincipalType string    `json:"principal_type" db:"principal_type"`
	PrincipalID   string    `json:"principal_id" db:"principal_id"`
	Permission    string    `json:"permission" db:"permission"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	CreatedBy     string    `json:"created_by,omitempty" db:"created_by"`
}

// SearchResult represents a tool search result
type SearchResult struct {
	Tool      *Tool   `json:"tool"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"` // "semantic", "keyword", "hybrid"
}

// RegisterToolRequest request to register a new tool
type RegisterToolRequest struct {
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Description    string          `json:"description"`
	ProviderType   ProviderType    `json:"provider_type"`
	ProviderConfig json.RawMessage `json:"provider_config"`
	InputSchema    json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema   json.RawMessage `json:"output_schema,omitempty"`
	Category       string          `json:"category,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	Scope          Scope           `json:"scope"`
	ScopeID        string          `json:"scope_id,omitempty"`
}

// ExecuteToolRequest request to execute a tool
type ExecuteToolRequest struct {
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
	UserID    string          `json:"user_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Timeout   int             `json:"timeout_seconds,omitempty"`
}

// ExecuteToolResponse response from tool execution
type ExecuteToolResponse struct {
	Success    bool            `json:"success"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMs int             `json:"duration_ms"`
}
