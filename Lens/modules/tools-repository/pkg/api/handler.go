// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/tools-repository/pkg/provider"
	"github.com/AMD-AGI/Primus-SaFE/Lens/tools-repository/pkg/registry"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP API requests
type Handler struct {
	registry        *registry.ToolsRegistry
	providerFactory *provider.ProviderFactory
}

// NewHandler creates a new API handler
func NewHandler(reg *registry.ToolsRegistry) *Handler {
	return &Handler{
		registry:        reg,
		providerFactory: provider.NewProviderFactory(),
	}
}

// RegisterRoutes registers API routes
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	{
		tools := api.Group("/tools")
		{
			tools.GET("", h.ListTools)
			tools.GET("/:name", h.GetTool)
			tools.POST("", h.RegisterTool)
			tools.DELETE("/:name", h.DeleteTool)
			tools.POST("/search", h.SearchTools)
			tools.POST("/execute", h.ExecuteTool)
			tools.GET("/:name/stats", h.GetToolStats)
			tools.GET("/:name/definition", h.GetToolDefinition)
		}
	}
}

// ListTools lists all tools
// @Summary List all tools
// @Tags tools
// @Param category query string false "Filter by category"
// @Param provider_type query string false "Filter by provider type"
// @Param scope query string false "Filter by scope"
// @Param offset query int false "Offset"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/tools [get]
func (h *Handler) ListTools(c *gin.Context) {
	category := c.Query("category")
	providerType := c.Query("provider_type")
	scope := registry.Scope(c.Query("scope"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	tools, total, err := h.registry.List(c.Request.Context(), category, providerType, scope, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tools":  tools,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

// GetTool gets a tool by name
// @Summary Get tool by name
// @Tags tools
// @Param name path string true "Tool name"
// @Success 200 {object} registry.Tool
// @Router /api/v1/tools/{name} [get]
func (h *Handler) GetTool(c *gin.Context) {
	name := c.Param("name")

	tool, err := h.registry.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tool)
}

// RegisterTool registers a new tool
// @Summary Register a new tool
// @Tags tools
// @Param tool body registry.RegisterToolRequest true "Tool registration request"
// @Success 201 {object} registry.Tool
// @Router /api/v1/tools [post]
func (h *Handler) RegisterTool(c *gin.Context) {
	var req registry.RegisterToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from context (set by auth middleware)
	createdBy := c.GetString("user_id")
	if createdBy == "" {
		createdBy = "system"
	}

	tool, err := h.registry.Register(c.Request.Context(), &req, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tool)
}

// DeleteTool deletes a tool
// @Summary Delete a tool
// @Tags tools
// @Param name path string true "Tool name"
// @Success 204 "No Content"
// @Router /api/v1/tools/{name} [delete]
func (h *Handler) DeleteTool(c *gin.Context) {
	name := c.Param("name")

	if err := h.registry.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// SearchTools searches for tools
// @Summary Search for tools
// @Tags tools
// @Param request body map[string]interface{} true "Search request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/tools/search [post]
func (h *Handler) SearchTools(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	results, err := h.registry.Search(c.Request.Context(), req.Query, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
	})
}

// ExecuteTool executes a tool
// @Summary Execute a tool
// @Tags tools
// @Param request body registry.ExecuteToolRequest true "Execution request"
// @Success 200 {object} registry.ExecuteToolResponse
// @Router /api/v1/tools/execute [post]
func (h *Handler) ExecuteTool(c *gin.Context) {
	var req registry.ExecuteToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get tool
	tool, err := h.registry.Get(c.Request.Context(), req.ToolName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Get provider
	prov, err := h.providerFactory.GetProvider(tool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create execution context
	ctx := c.Request.Context()
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
		defer cancel()
	}

	// Execute
	startTime := time.Now()
	resp, err := prov.Execute(ctx, tool, req.Arguments)
	if err != nil {
		resp = &registry.ExecuteToolResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}
	}

	// Record execution
	exec := &registry.ToolExecution{
		ToolID:     tool.ID,
		ToolName:   tool.Name,
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		Input:      req.Arguments,
		StartedAt:  startTime,
		DurationMs: resp.DurationMs,
	}

	if resp.Success {
		exec.Status = "success"
		exec.Output = resp.Output
	} else {
		exec.Status = "error"
		exec.ErrorMessage = resp.Error
	}

	completedAt := time.Now()
	exec.CompletedAt = &completedAt

	h.registry.RecordExecution(context.Background(), exec)

	c.JSON(http.StatusOK, resp)
}

// GetToolStats gets statistics for a tool
// @Summary Get tool statistics
// @Tags tools
// @Param name path string true "Tool name"
// @Success 200 {object} registry.ToolStats
// @Router /api/v1/tools/{name}/stats [get]
func (h *Handler) GetToolStats(c *gin.Context) {
	name := c.Param("name")

	stats, err := h.registry.GetStats(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetToolDefinition gets tool definition in MCP format
// @Summary Get tool definition
// @Tags tools
// @Param name path string true "Tool name"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/tools/{name}/definition [get]
func (h *Handler) GetToolDefinition(c *gin.Context) {
	name := c.Param("name")

	definition, err := h.registry.GetToolDefinition(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, definition)
}

// MCP Tool for tools discovery
type ToolsDiscoveryMCPHandler struct {
	registry *registry.ToolsRegistry
}

// NewToolsDiscoveryMCPHandler creates MCP handler for tools discovery
func NewToolsDiscoveryMCPHandler(reg *registry.ToolsRegistry) *ToolsDiscoveryMCPHandler {
	return &ToolsDiscoveryMCPHandler{registry: reg}
}

// HandleToolsSearch handles tools_search MCP tool call
func (h *ToolsDiscoveryMCPHandler) HandleToolsSearch(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	if params.Limit == 0 {
		params.Limit = 5
	}

	results, err := h.registry.Search(ctx, params.Query, params.Limit)
	if err != nil {
		return nil, err
	}

	// Format for MCP response
	type ToolSummary struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Category    string  `json:"category"`
		Score       float64 `json:"relevance_score"`
	}

	summaries := make([]ToolSummary, len(results))
	for i, r := range results {
		summaries[i] = ToolSummary{
			Name:        r.Tool.Name,
			Description: r.Tool.Description,
			Category:    r.Tool.Category,
			Score:       r.Score,
		}
	}

	return map[string]interface{}{
		"tools": summaries,
		"total": len(summaries),
		"hint":  "Use tools_execute to call a tool",
	}, nil
}

// HandleToolsList handles tools_list MCP tool call
func (h *ToolsDiscoveryMCPHandler) HandleToolsList(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Category string `json:"category"`
		Limit    int    `json:"limit"`
	}
	json.Unmarshal(args, &params)

	if params.Limit == 0 {
		params.Limit = 50
	}

	tools, total, err := h.registry.List(ctx, params.Category, "", "", 0, params.Limit)
	if err != nil {
		return nil, err
	}

	type ToolSummary struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Category     string `json:"category"`
		ProviderType string `json:"provider_type"`
	}

	summaries := make([]ToolSummary, len(tools))
	for i, t := range tools {
		summaries[i] = ToolSummary{
			Name:         t.Name,
			Description:  t.Description,
			Category:     t.Category,
			ProviderType: string(t.ProviderType),
		}
	}

	return map[string]interface{}{
		"tools": summaries,
		"total": total,
	}, nil
}
