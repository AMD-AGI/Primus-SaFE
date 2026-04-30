/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package router wires the MCP server into the apiserver's Gin engine.
// It exposes SSE, Streamable HTTP, health and index endpoints under the
// configured base path (defaults to /api/v1/mcp).
package router

import (
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

// MountRoutes mounts SSE / streamable HTTP / health / index endpoints for the
// given MCP server under basePath. Exported so tests can verify the routing
// layer without bringing up a full apiserver.
func MountRoutes(engine *gin.Engine, srv *mcpserver.Server, basePath string) {
	if basePath == "" {
		basePath = defaultBasePath
	}
	cleanBase := strings.TrimRight(basePath, "/")

	sseTransport := mcpserver.NewSSETransport(srv)
	sseTransport.MessageEndpointPath = cleanBase + "/message"
	streamableTransport := mcpserver.NewStreamableHTTPTransport(srv)

	mcpGroup := engine.Group(basePath)
	{
		mcpGroup.GET("/sse", func(c *gin.Context) { sseTransport.HandleSSE(c.Writer, c.Request) })
		mcpGroup.POST("/message", func(c *gin.Context) { sseTransport.HandleMessage(c.Writer, c.Request) })
		mcpGroup.POST("/rpc", func(c *gin.Context) { streamableTransport.HandleRPC(c.Writer, c.Request) })

		mcpGroup.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":      "ok",
				"server":      "SaFE MCP Server",
				"version":     mcpserver.MCPVersion,
				"total_tools": srv.ToolCount(),
			})
		})

		mcpGroup.GET("/", func(c *gin.Context) {
			toolNames := srv.GetToolNames()
			c.JSON(200, gin.H{
				"server":       "SaFE MCP Server",
				"version":      mcpserver.MCPVersion,
				"sse_endpoint": cleanBase + "/sse",
				"rpc_endpoint": cleanBase + "/rpc",
				"total_tools":  len(toolNames),
				"tools":        toolNames,
			})
		})
	}

	klog.Infof("MCP Server: Routes registered under %s", basePath)
	klog.Infof("MCP Server: SSE endpoint: %s/sse", cleanBase)
	klog.Infof("MCP Server: RPC endpoint: %s/rpc (for testing)", cleanBase)
}
