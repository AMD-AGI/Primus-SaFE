// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package unified provides a unified endpoint abstraction that allows defining
// handlers once and exposing them via both HTTP (Gin) and MCP (Model Context Protocol).
package unified

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// Handler is a generic handler function that takes a request and returns a response.
// Use this for standard request/response APIs that can be exposed via both HTTP and MCP.
type Handler[Req, Resp any] func(ctx context.Context, req *Req) (*Resp, error)

// RawHTTPHandler is for special cases that need direct HTTP access:
//   - Long-lived connections (WebSocket, SSE streaming)
//   - File downloads (need Content-Disposition header)
//   - Proxy requests (need full request/response control)
//   - Custom headers manipulation
//
// Note: RawHTTPHandler can only be registered for HTTP, not MCP.
type RawHTTPHandler func(c *gin.Context)

// RawMCPHandler is for MCP-specific complex operations that don't fit the simple Req/Resp model:
//   - Multi-step interactions
//   - Resource streaming
type RawMCPHandler func(ctx context.Context, params map[string]any) (any, error)

// EndpointDef defines a unified endpoint that can be registered as both HTTP and MCP.
type EndpointDef[Req, Resp any] struct {
	// Common fields
	Name        string // Unique identifier for the endpoint
	Description string // Human-readable description for documentation
	Group       string // Tool group for MCP server isolation (e.g. "diagnostic")

	// Handler options (use ONE of these, priority: RawHTTPHandler > Handler for HTTP)
	Handler        Handler[Req, Resp] // Standard handler - works for both HTTP and MCP
	RawHTTPHandler RawHTTPHandler     // Raw HTTP handler - HTTP only, for special cases
	RawMCPHandler  RawMCPHandler      // Raw MCP handler - MCP only, for complex operations

	// HTTP specific configuration
	HTTPMethod string // GET, POST, PUT, DELETE, Any
	HTTPPath   string // URL path, e.g., "/clusters/overview"

	// MCP specific configuration
	MCPToolName string // Tool name for MCP (optional, defaults to Name)
	MCPOnly     bool   // If true, only register as MCP tool, not HTTP
	HTTPOnly    bool   // If true, only register as HTTP endpoint, not MCP
}

// MCPTool represents an MCP tool definition that can be registered with an MCP server.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     MCPToolHandler `json:"-"`
}

// MCPToolHandler is the function signature for MCP tool handlers.
type MCPToolHandler func(ctx context.Context, params json.RawMessage) (any, error)

// EndpointRegistration is a non-generic interface for storing endpoint definitions in a registry.
type EndpointRegistration interface {
	// GetName returns the unique name of the endpoint.
	GetName() string
	// GetDescription returns the human-readable description.
	GetDescription() string
	// GetHTTPMethod returns the HTTP method (GET, POST, etc.).
	GetHTTPMethod() string
	// GetHTTPPath returns the URL path.
	GetHTTPPath() string
	// GetMCPToolName returns the MCP tool name.
	GetMCPToolName() string
	// IsHTTPOnly returns true if the endpoint should only be registered for HTTP.
	IsHTTPOnly() bool
	// IsMCPOnly returns true if the endpoint should only be registered for MCP.
	IsMCPOnly() bool
	// GetGinHandler returns a gin.HandlerFunc for HTTP registration.
	GetGinHandler() gin.HandlerFunc
	// GetMCPTool returns an MCPTool for MCP registration, or nil if HTTP-only.
	GetMCPTool() *MCPTool
}

// Implement EndpointRegistration interface for EndpointDef

// GetName returns the unique name of the endpoint.
func (def *EndpointDef[Req, Resp]) GetName() string {
	return def.Name
}

// GetDescription returns the human-readable description.
func (def *EndpointDef[Req, Resp]) GetDescription() string {
	return def.Description
}

// GetHTTPMethod returns the HTTP method.
func (def *EndpointDef[Req, Resp]) GetHTTPMethod() string {
	return def.HTTPMethod
}

// GetHTTPPath returns the URL path.
func (def *EndpointDef[Req, Resp]) GetHTTPPath() string {
	return def.HTTPPath
}

// GetMCPToolName returns the MCP tool name, defaulting to Name if not set.
func (def *EndpointDef[Req, Resp]) GetMCPToolName() string {
	if def.MCPToolName != "" {
		return def.MCPToolName
	}
	return def.Name
}

// IsHTTPOnly returns true if the endpoint should only be registered for HTTP.
func (def *EndpointDef[Req, Resp]) IsHTTPOnly() bool {
	return def.HTTPOnly
}

// IsMCPOnly returns true if the endpoint should only be registered for MCP.
func (def *EndpointDef[Req, Resp]) IsMCPOnly() bool {
	return def.MCPOnly
}

// GetGroup returns the tool group name.
func (def *EndpointDef[Req, Resp]) GetGroup() string {
	return def.Group
}
