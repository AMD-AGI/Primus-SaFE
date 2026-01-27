// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package server implements the MCP (Model Context Protocol) server.
// It supports both SSE (Server-Sent Events) and STDIO transports.
package server

import (
	"encoding/json"
)

// JSON-RPC 2.0 Protocol Constants
const (
	JSONRPCVersion = "2.0"
)

// MCP Protocol Version
const (
	MCPProtocolVersion = "2024-11-05"
	MCPVersion         = "1.0.0" // Server version for display
)

// MCP Method Names
const (
	MethodInitialize      = "initialize"
	MethodInitialized     = "notifications/initialized"
	MethodToolsList       = "tools/list"
	MethodToolsCall       = "tools/call"
	MethodPing            = "ping"
	MethodResourcesList   = "resources/list"
	MethodResourcesRead   = "resources/read"
	MethodPromptsList     = "prompts/list"
	MethodPromptsGet      = "prompts/get"
	MethodLoggingSetLevel = "logging/setLevel"
)

// MCP Error Codes (JSON-RPC 2.0 + MCP specific)
const (
	// Standard JSON-RPC 2.0 errors
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603

	// MCP specific errors
	ErrorCodeToolNotFound      = -32001
	ErrorCodeToolExecutionFail = -32002
	ErrorCodeResourceNotFound  = -32003
)

// JSONRPCRequest represents a JSON-RPC 2.0 request message.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // Can be string, number, or null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response message.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification (no id, no response expected).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewResponse creates a successful JSON-RPC response.
func NewResponse(id json.RawMessage, result any) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates an error JSON-RPC response.
func NewErrorResponse(id json.RawMessage, code int, message string, data any) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// ===== MCP Initialize Messages =====

// InitializeParams represents the parameters for the initialize request.
type InitializeParams struct {
	ProtocolVersion string           `json:"protocolVersion"`
	Capabilities    ClientCapability `json:"capabilities"`
	ClientInfo      Implementation   `json:"clientInfo"`
}

// InitializeResult represents the result of the initialize request.
type InitializeResult struct {
	ProtocolVersion string           `json:"protocolVersion"`
	Capabilities    ServerCapability `json:"capabilities"`
	ServerInfo      Implementation   `json:"serverInfo"`
	Instructions    string           `json:"instructions,omitempty"`
}

// Implementation describes a client or server implementation.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapability represents client capabilities.
type ClientCapability struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

// ServerCapability represents server capabilities.
type ServerCapability struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Logging   *LoggingCapability   `json:"logging,omitempty"`
}

// RootsCapability indicates the client supports roots.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability indicates the client supports sampling.
type SamplingCapability struct{}

// ToolsCapability indicates the server supports tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates the server supports resources.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates the server supports prompts.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability indicates the server supports logging.
type LoggingCapability struct{}

// ===== MCP Tools Messages =====

// ToolsListResult represents the result of tools/list.
type ToolsListResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// ToolDefinition represents a tool definition in MCP.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolsCallParams represents the parameters for tools/call.
type ToolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolsCallResult represents the result of tools/call.
type ToolsCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents a content block in MCP responses.
type ContentBlock struct {
	Type     string `json:"type"` // "text", "image", "resource"
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // Base64 encoded for images
	URI      string `json:"uri,omitempty"`  // For resource references
}

// NewTextContent creates a text content block.
func NewTextContent(text string) ContentBlock {
	return ContentBlock{
		Type: "text",
		Text: text,
	}
}

// NewJSONContent creates a text content block with JSON-formatted data.
func NewJSONContent(data any) (ContentBlock, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return ContentBlock{}, err
	}
	return ContentBlock{
		Type: "text",
		Text: string(jsonBytes),
	}, nil
}

// ===== MCP Ping =====

// PingResult represents the result of a ping request.
type PingResult struct{}

// ===== MCP Resources Messages =====

// ResourcesListResult represents the result of resources/list.
type ResourcesListResult struct {
	Resources []ResourceDefinition `json:"resources"`
}

// ResourceDefinition represents a resource definition in MCP.
type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourcesReadParams represents the parameters for resources/read.
type ResourcesReadParams struct {
	URI string `json:"uri"`
}

// ResourcesReadResult represents the result of resources/read.
type ResourcesReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent represents the content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // Base64 encoded binary data
}

// ===== MCP Prompts Messages =====

// PromptsListResult represents the result of prompts/list.
type PromptsListResult struct {
	Prompts []PromptDefinition `json:"prompts"`
}

// PromptDefinition represents a prompt definition in MCP.
type PromptDefinition struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Arguments   []PromptArgument  `json:"arguments,omitempty"`
}

// PromptArgument represents an argument for a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptsGetParams represents the parameters for prompts/get.
type PromptsGetParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// PromptsGetResult represents the result of prompts/get.
type PromptsGetResult struct {
	Description string         `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage represents a message in a prompt result.
type PromptMessage struct {
	Role    string       `json:"role"` // "user" or "assistant"
	Content ContentBlock `json:"content"`
}

// ===== Helper Functions =====

// ParseRequest parses a JSON-RPC request from bytes.
func ParseRequest(data []byte) (*JSONRPCRequest, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// IsNotification returns true if the request is a notification (no ID).
func (r *JSONRPCRequest) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// GetIDString returns the ID as a string for logging purposes.
func (r *JSONRPCRequest) GetIDString() string {
	if r.ID == nil {
		return "<nil>"
	}
	return string(r.ID)
}
