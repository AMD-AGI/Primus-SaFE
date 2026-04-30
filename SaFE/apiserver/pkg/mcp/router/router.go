/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package router wires the MCP server into the apiserver's Gin engine.
//
// Two transports are exposed under the configured base path
// (defaults to /api/v1/mcp), aligned with the MCP specification:
//
//   - SSE transport (2024-11-05 spec):
//       GET  {base}/sse       -> server-sent events stream
//       POST {base}/messages  -> client-to-server messages (session_id query)
//
//   - Streamable HTTP transport (2025-03-26 spec):
//       POST {base}           -> single request/response (or streamed) JSON-RPC
//
// Auxiliary endpoints:
//
//       GET  {base}/health    -> liveness check
//       GET  {base}/info      -> human-readable server info & tool list
package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	mcptools "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/mcp/tools"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/mcp/unified"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

// defaultBasePath is used when mcp.base_path is unset.
const defaultBasePath = "/api/v1/mcp"

// InitRoutes builds an MCP server pre-loaded with the unified registry tools
// plus the built-in tool set, then mounts it on engine under the configured
// base path. Caller should gate this on commonconfig.IsMCPEnable().
func InitRoutes(engine *gin.Engine) {
	srv := mcpserver.New()

	unifiedTools := unified.GetRegistry().GetMCPTools()
	srv.RegisterTools(unifiedTools)
	klog.Infof("MCP Server: Registered %d tools from unified registry", len(unifiedTools))

	builtinTools := mcptools.RegisterAllTools()
	srv.RegisterTools(builtinTools)
	klog.Infof("MCP Server: Registered %d built-in tools", len(builtinTools))

	if instructions := commonconfig.GetMCPInstructions(); instructions != "" {
		srv.SetInstructions(instructions)
	} else {
		srv.SetInstructions("SaFE API Server - GPU Cluster Management Tools via MCP")
	}

	MountRoutes(engine, srv, commonconfig.GetMCPBasePath())
}

// MountRoutes mounts the standard MCP transport endpoints (SSE + Streamable
// HTTP) plus health/info onto engine under basePath. Exported so tests can
// verify the routing layer without bringing up a full apiserver.
func MountRoutes(engine *gin.Engine, srv *mcpserver.Server, basePath string) {
	if basePath == "" {
		basePath = defaultBasePath
	}
	cleanBase := strings.TrimRight(basePath, "/")

	sseTransport := mcpserver.NewSSETransport(srv)
	sseTransport.MessageEndpointPath = cleanBase + "/messages"
	streamableTransport := mcpserver.NewStreamableHTTPTransport(srv)

	// Streamable HTTP transport: the base path itself is the endpoint per the
	// 2025-03-26 spec. POST is the JSON-RPC endpoint; GET is reserved for
	// optional server->client notifications (not implemented yet).
	engine.POST(cleanBase, func(c *gin.Context) { streamableTransport.HandleRPC(c.Writer, c.Request) })
	engine.GET(cleanBase, func(c *gin.Context) {
		c.Header("Allow", "POST")
		c.AbortWithStatus(http.StatusMethodNotAllowed)
	})

	mcpGroup := engine.Group(cleanBase)
	{
		// SSE transport (2024-11-05 spec): /sse opens the stream, /messages
		// receives client-to-server JSON-RPC messages.
		mcpGroup.GET("/sse", func(c *gin.Context) { sseTransport.HandleSSE(c.Writer, c.Request) })
		mcpGroup.POST("/messages", func(c *gin.Context) { sseTransport.HandleMessage(c.Writer, c.Request) })

		mcpGroup.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":      "ok",
				"server":      "SaFE MCP Server",
				"version":     mcpserver.MCPVersion,
				"total_tools": srv.ToolCount(),
			})
		})

		mcpGroup.GET("/info", func(c *gin.Context) {
			toolNames := srv.GetToolNames()
			c.JSON(http.StatusOK, gin.H{
				"server":                  "SaFE MCP Server",
				"version":                 mcpserver.MCPVersion,
				"sse_endpoint":            cleanBase + "/sse",
				"streamable_http_endpoint": cleanBase,
				"total_tools":             len(toolNames),
				"tools":                   toolNames,
			})
		})
	}

	klog.Infof("MCP Server: Routes registered under %s", cleanBase)
	klog.Infof("MCP Server: Streamable HTTP endpoint: %s", cleanBase)
	klog.Infof("MCP Server: SSE endpoint: %s/sse", cleanBase)
}
