// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package unified provides a unified endpoint abstraction that allows defining
// handlers once and exposing them via both HTTP (Gin) and MCP (Model Context Protocol).
package unified

import (
	"context"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
	"github.com/gin-gonic/gin"
)

// Handler is a generic handler function for standard request/response APIs.
type Handler[Req, Resp any] func(ctx context.Context, req *Req) (*Resp, error)

// RawHTTPHandler is for special cases that need direct HTTP access
// (WebSocket, SSE streaming, file downloads, proxy requests).
type RawHTTPHandler func(c *gin.Context)

// RawMCPHandler is for MCP-specific complex operations.
type RawMCPHandler func(ctx context.Context, params map[string]any) (any, error)

// EndpointDef defines a unified endpoint registered as both HTTP and MCP.
type EndpointDef[Req, Resp any] struct {
	Name        string
	Description string
	Group       string // Tool group for MCP server isolation (e.g. "diagnostic")

	Handler        Handler[Req, Resp]
	RawHTTPHandler RawHTTPHandler
	RawMCPHandler  RawMCPHandler

	HTTPMethod string // GET, POST, PUT, DELETE, Any
	HTTPPath   string

	MCPToolName    string
	MCPInputSchema map[string]any // Custom input schema for RawMCPHandler (optional)
	MCPOnly        bool
	HTTPOnly       bool
}

// EndpointRegistration is a non-generic interface for the registry.
type EndpointRegistration interface {
	GetName() string
	GetDescription() string
	GetHTTPMethod() string
	GetHTTPPath() string
	GetMCPToolName() string
	IsHTTPOnly() bool
	IsMCPOnly() bool
	GetGinHandler() gin.HandlerFunc
	GetMCPTool() *mcpserver.MCPTool
	GetGroup() string
}

func (def *EndpointDef[Req, Resp]) GetName() string        { return def.Name }
func (def *EndpointDef[Req, Resp]) GetDescription() string { return def.Description }
func (def *EndpointDef[Req, Resp]) GetHTTPMethod() string  { return def.HTTPMethod }
func (def *EndpointDef[Req, Resp]) GetHTTPPath() string    { return def.HTTPPath }
func (def *EndpointDef[Req, Resp]) IsHTTPOnly() bool       { return def.HTTPOnly }
func (def *EndpointDef[Req, Resp]) IsMCPOnly() bool        { return def.MCPOnly }
func (def *EndpointDef[Req, Resp]) GetGroup() string       { return def.Group }

func (def *EndpointDef[Req, Resp]) GetMCPToolName() string {
	if def.MCPToolName != "" {
		return def.MCPToolName
	}
	return def.Name
}
