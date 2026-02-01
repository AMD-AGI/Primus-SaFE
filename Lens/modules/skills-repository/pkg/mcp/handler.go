// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package mcp

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
	"github.com/gin-gonic/gin"
)

// Handler handles MCP requests for skills repository
type Handler struct {
	mcpServer    *server.Server
	sseTransport *server.SSETransport
}

// NewHandler creates a new MCP handler
func NewHandler(reg *registry.SkillsRegistry) *Handler {
	// Create MCP server
	mcpServer := server.New()

	// Set server instructions
	mcpServer.SetInstructions(`Skills Repository MCP Server

This MCP server provides access to the Skills Repository, which contains reusable skills
that can help you accomplish various tasks.

Available tools:
- skills_search: Search for skills by natural language query (USE THIS FIRST)
- skills_list: List all available skills with optional filters
- skills_get: Get detailed information about a skill
- skills_get_content: Get the full SKILL.md content with instructions

Typical workflow:
1. Use skills_search to find relevant skills for your task
2. Use skills_get_content to load the skill instructions
3. Follow the skill instructions to complete the task

Example: If user asks to "create a database migration", use skills_search with 
query "database migration" to find relevant skills.`)

	// Register skills tools
	tools := CreateSkillsTools(reg)
	mcpServer.RegisterTools(tools)

	// Create SSE transport
	sseTransport := server.NewSSETransport(mcpServer)

	return &Handler{
		mcpServer:    mcpServer,
		sseTransport: sseTransport,
	}
}

// RegisterRoutes registers MCP routes to the router
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	// Mount SSE transport at /mcp path using Any to handle all sub-routes
	router.Any("/mcp/*path", func(c *gin.Context) {
		// Strip /mcp prefix and let the transport handler route internally
		c.Request.URL.Path = c.Param("path")
		if c.Request.URL.Path == "" {
			c.Request.URL.Path = "/"
		}
		h.sseTransport.Handler().ServeHTTP(c.Writer, c.Request)
	})
}

// GetServer returns the underlying MCP server
func (h *Handler) GetServer() *server.Server {
	return h.mcpServer
}

// GetSSETransport returns the SSE transport
func (h *Handler) GetSSETransport() *server.SSETransport {
	return h.sseTransport
}
