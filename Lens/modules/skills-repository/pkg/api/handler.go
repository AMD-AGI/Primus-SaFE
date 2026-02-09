// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/service"
	"github.com/gin-gonic/gin"
)

// Handler handles API requests for tools and toolsets
type Handler struct {
	toolService    *service.ToolService
	searchService  *service.SearchService
	importService  *service.ImportService
	runService     *service.RunService
	toolsetService *service.ToolsetService
}

// NewHandler creates a new Handler
func NewHandler(
	toolSvc *service.ToolService,
	searchSvc *service.SearchService,
	importSvc *service.ImportService,
	runSvc *service.RunService,
	toolsetSvc *service.ToolsetService,
) *Handler {
	return &Handler{
		toolService:    toolSvc,
		searchService:  searchSvc,
		importService:  importSvc,
		runService:     runSvc,
		toolsetService: toolsetSvc,
	}
}

// RegisterRoutes registers API routes
func RegisterRoutes(router *gin.Engine, h *Handler) {
	v1 := router.Group("/api/v1")
	{
		// Health check (no auth required)
		v1.GET("/tools/health", h.Health)
	}

	// Routes that require authentication
	auth := v1.Group("")
	auth.Use(AuthMiddleware(true))
	{
		// Tools list and get
		auth.GET("/tools", h.ListTools)
		auth.GET("/tools/:id", h.GetTool)
		auth.PUT("/tools/:id", h.UpdateTool)
		auth.DELETE("/tools/:id", h.DeleteTool)

		// Create MCP (JSON) - Skills are created via import/discover + import/commit
		auth.POST("/tools/mcp", h.CreateMCP)

		// Search
		auth.GET("/tools/search", h.SearchTools)

		// Run tools
		auth.POST("/tools/run", h.RunTools)

		// Download
		auth.GET("/tools/:id/download", h.DownloadTool)

		// Import (batch)
		auth.POST("/tools/import/discover", h.ImportDiscover)
		auth.POST("/tools/import/commit", h.ImportCommit)

		// Like/Unlike
		auth.POST("/tools/:id/like", h.LikeTool)
		auth.DELETE("/tools/:id/like", h.UnlikeTool)

		// Icon upload
		auth.POST("/tools/icon", h.UploadIcon)

		// Get tool content (SKILL.md for skills)
		auth.GET("/tools/:id/content", h.GetToolContent)

		// Toolsets
		auth.GET("/toolsets", h.ListToolsets)
		auth.POST("/toolsets", h.CreateToolset)
		auth.GET("/toolsets/search", h.SearchToolsets)
		auth.GET("/toolsets/:id", h.GetToolset)
		auth.PUT("/toolsets/:id", h.UpdateToolset)
		auth.DELETE("/toolsets/:id", h.DeleteToolset)
		auth.POST("/toolsets/:id/tools", h.AddToolsToToolset)
		auth.DELETE("/toolsets/:id/tools/:toolId", h.RemoveToolFromToolset)
	}
}

// --- Request/Response Types ---

// CreateMCPRequest represents a request to create an MCP server
type CreateMCPRequest struct {
	Name        string                 `json:"name" binding:"required"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description" binding:"required"`
	Tags        []string               `json:"tags"`
	IconURL     string                 `json:"icon_url"`
	Author      string                 `json:"author"`
	Config      map[string]interface{} `json:"config" binding:"required"` // Full mcpServers JSON
	IsPublic    *bool                  `json:"is_public"`
}

// UpdateToolRequest represents a request to update a tool
type UpdateToolRequest struct {
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	Tags        []string               `json:"tags"`
	IconURL     string                 `json:"icon_url"`
	Author      string                 `json:"author"`
	Config      map[string]interface{} `json:"config"`
	IsPublic    *bool                  `json:"is_public"`
	Status      string                 `json:"status"`
}

// ToolRef represents a reference to a tool by ID or type+name
type ToolRef struct {
	ID   *int64 `json:"id"`   // Option 1: by ID
	Type string `json:"type"` // Option 2: by type + name
	Name string `json:"name"`
}

// RunToolsRequest represents a request to run multiple tools
type RunToolsRequest struct {
	Tools []ToolRef `json:"tools" binding:"required"`
}

// RunToolsResponse represents the response for running tools
type RunToolsResponse struct {
	RedirectURL string `json:"redirect_url"`
	SessionID   string `json:"session_id,omitempty"`
}

// ImportCommitRequest represents a request to commit selected skills
type ImportCommitRequest struct {
	ArchiveKey string              `json:"archive_key" binding:"required"`
	Selections []service.Selection `json:"selections" binding:"required"`
}

// --- Error Handling ---
// (Moved to error_response.go for centralized error handling)

// --- Handler Methods ---

// Health returns service health status
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// ListTools lists all tools with pagination and sorting
func (h *Handler) ListTools(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	userInfo := GetUserInfo(c)

	result, err := h.toolService.List(c.Request.Context(), service.ListInput{
		Type:      c.Query("type"),
		Status:    c.Query("status"),
		Owner:     c.Query("owner"),
		SortField: c.DefaultQuery("sort", "created_at"),
		SortOrder: c.DefaultQuery("order", "desc"),
		Offset:    offset,
		Limit:     limit,
		UserID:    userInfo.UserID,
	})
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tools":  result.Tools,
		"total":  result.Total,
		"offset": result.Offset,
		"limit":  result.Limit,
	})
}

// GetTool retrieves a tool by ID
func (h *Handler) GetTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "ID must be a valid integer")
		return
	}

	userInfo := GetUserInfo(c)
	result, err := h.toolService.GetTool(c.Request.Context(), id, userInfo.UserID)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// CreateMCP creates a new MCP server
func (h *Handler) CreateMCP(c *gin.Context) {
	var req CreateMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, "Invalid request body", err.Error())
		return
	}

	userInfo := GetUserInfo(c)
	log.Printf("[CreateMCP] user=%s name=%s", userInfo.UserID, req.Name)

	tool, err := h.toolService.CreateMCP(c.Request.Context(), service.CreateMCPInput{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Tags:        req.Tags,
		IconURL:     req.IconURL,
		Author:      req.Author,
		Config:      req.Config,
		IsPublic:    req.IsPublic,
		UserID:      userInfo.UserID,
		Username:    userInfo.Username,
	})
	if err != nil {
		log.Printf("[CreateMCP] user=%s name=%s error=%v", userInfo.UserID, req.Name, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[CreateMCP] user=%s name=%s tool_id=%d success", userInfo.UserID, req.Name, tool.ID)
	c.JSON(http.StatusCreated, tool)
}

// UpdateTool updates an existing tool
func (h *Handler) UpdateTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "ID must be a valid integer")
		return
	}

	var req UpdateToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, "Invalid request body", err.Error())
		return
	}

	userInfo := GetUserInfo(c)
	log.Printf("[UpdateTool] user=%s tool_id=%d", userInfo.UserID, id)

	tool, err := h.toolService.UpdateTool(c.Request.Context(), id, service.UpdateToolInput{
		DisplayName: req.DisplayName,
		Description: req.Description,
		Tags:        req.Tags,
		IconURL:     req.IconURL,
		Author:      req.Author,
		Config:      req.Config,
		IsPublic:    req.IsPublic,
		Status:      req.Status,
	}, userInfo.UserID)
	if err != nil {
		log.Printf("[UpdateTool] user=%s tool_id=%d error=%v", userInfo.UserID, id, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[UpdateTool] user=%s tool_id=%d success", userInfo.UserID, id)
	c.JSON(http.StatusOK, tool)
}

// DeleteTool deletes a tool by ID
func (h *Handler) DeleteTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "ID must be a valid integer")
		return
	}

	userInfo := GetUserInfo(c)
	log.Printf("[DeleteTool] user=%s tool_id=%d", userInfo.UserID, id)

	if err := h.toolService.DeleteTool(c.Request.Context(), id, userInfo.UserID); err != nil {
		log.Printf("[DeleteTool] user=%s tool_id=%d error=%v", userInfo.UserID, id, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[DeleteTool] user=%s tool_id=%d success", userInfo.UserID, id)
	c.JSON(http.StatusOK, gin.H{"message": "tool deleted successfully"})
}

// SearchTools searches tools by query with different modes
func (h *Handler) SearchTools(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		respondInvalidParameter(c, "q", "Query parameter 'q' is required")
		return
	}

	toolType := c.Query("type")
	mode := c.DefaultQuery("mode", "semantic")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.searchService.Search(c.Request.Context(), query, toolType, mode, limit)
	if err != nil {
		// "semantic search not configured" should be 400 (client requested unsupported mode)
		if errors.Is(err, service.ErrNotConfigured) {
			respondBadRequest(c, "Search mode not available", err.Error())
		} else {
			respondServiceError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": result.Tools,
		"total": result.Total,
		"mode":  result.Mode,
	})
}

// RunTools runs multiple tools via the execution backend
func (h *Handler) RunTools(c *gin.Context) {
	var req RunToolsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, "Invalid request body", err.Error())
		return
	}

	// Convert request ToolRefs to service ToolRefs
	refs := make([]service.ToolRef, len(req.Tools))
	for i, t := range req.Tools {
		refs[i] = service.ToolRef{ID: t.ID, Type: t.Type, Name: t.Name}
	}

	result, err := h.runService.RunTools(c.Request.Context(), refs)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, RunToolsResponse{
		RedirectURL: result.RedirectURL,
		SessionID:   result.SessionID,
	})
}

// DownloadTool generates and returns a downloadable file for local use
func (h *Handler) DownloadTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "ID must be a valid integer")
		return
	}

	userInfo := GetUserInfo(c)
	result, err := h.runService.DownloadTool(c.Request.Context(), id, userInfo.UserID)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+result.Filename)
	c.Header("Content-Type", result.ContentType)
	c.Data(http.StatusOK, result.ContentType, result.Data)
}

// ImportDiscover handles skill discovery from ZIP file or GitHub URL
func (h *Handler) ImportDiscover(c *gin.Context) {
	// Check for file upload
	file, header, fileErr := c.Request.FormFile("file")
	githubURL := c.PostForm("github_url")

	if fileErr != nil && githubURL == "" {
		respondBadRequest(c, "Either file or github_url must be provided")
		return
	}

	if fileErr == nil && githubURL != "" {
		respondBadRequest(c, "Only one of file or github_url can be provided")
		return
	}

	// Parse pagination parameters (support both query and form-data)
	offsetStr := c.Query("offset")
	if offsetStr == "" {
		offsetStr = c.PostForm("offset")
	}
	limitStr := c.Query("limit")
	if limitStr == "" {
		limitStr = c.PostForm("limit")
	}
	offset, _ := strconv.Atoi(offsetStr)
	limit, _ := strconv.Atoi(limitStr)

	userInfo := GetUserInfo(c)

	input := &service.DiscoverInput{
		UserID:    userInfo.UserID,
		GitHubURL: githubURL,
		Offset:    offset,
		Limit:     limit,
	}

	if fileErr == nil {
		defer file.Close()
		input.File = file
		input.FileName = header.Filename
	}

	log.Printf("[ImportDiscover] user=%s github_url=%s file=%s offset=%d limit=%d", userInfo.UserID, githubURL, input.FileName, offset, limit)

	result, err := h.importService.Discover(c.Request.Context(), input)
	if err != nil {
		log.Printf("[ImportDiscover] user=%s error=%v", userInfo.UserID, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[ImportDiscover] user=%s total=%d success", userInfo.UserID, result.Total)
	c.JSON(http.StatusOK, result)
}

// ImportCommit handles committing selected skills from a discovered archive
func (h *Handler) ImportCommit(c *gin.Context) {
	var req ImportCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, "Invalid request body", err.Error())
		return
	}

	if len(req.Selections) == 0 {
		respondBadRequest(c, "Selections cannot be empty")
		return
	}

	userInfo := GetUserInfo(c)
	log.Printf("[ImportCommit] user=%s archive_key=%s selections=%d", userInfo.UserID, req.ArchiveKey, len(req.Selections))

	result, err := h.importService.Commit(c.Request.Context(), &service.CommitInput{
		UserID:     userInfo.UserID,
		Username:   userInfo.Username,
		ArchiveKey: req.ArchiveKey,
		Selections: req.Selections,
	})
	if err != nil {
		log.Printf("[ImportCommit] user=%s error=%v", userInfo.UserID, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[ImportCommit] user=%s success", userInfo.UserID)
	c.JSON(http.StatusOK, result)
}

// LikeTool handles POST /tools/:id/like
func (h *Handler) LikeTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "Tool ID must be a valid integer")
		return
	}

	userInfo := GetUserInfo(c)
	if userInfo.UserID == "" {
		respondUnauthorized(c, "User ID required for liking")
		return
	}

	log.Printf("[LikeTool] user=%s tool_id=%d", userInfo.UserID, id)

	likeCount, err := h.toolService.LikeTool(c.Request.Context(), id, userInfo.UserID)
	if err != nil {
		log.Printf("[LikeTool] user=%s tool_id=%d error=%v", userInfo.UserID, id, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[LikeTool] user=%s tool_id=%d like_count=%d success", userInfo.UserID, id, likeCount)
	c.JSON(http.StatusOK, gin.H{
		"message":    "liked",
		"like_count": likeCount,
	})
}

// UnlikeTool handles DELETE /tools/:id/like
func (h *Handler) UnlikeTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "Tool ID must be a valid integer")
		return
	}

	userInfo := GetUserInfo(c)
	if userInfo.UserID == "" {
		respondUnauthorized(c, "User ID required for unliking")
		return
	}

	log.Printf("[UnlikeTool] user=%s tool_id=%d", userInfo.UserID, id)

	likeCount, err := h.toolService.UnlikeTool(c.Request.Context(), id, userInfo.UserID)
	if err != nil {
		log.Printf("[UnlikeTool] user=%s tool_id=%d error=%v", userInfo.UserID, id, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[UnlikeTool] user=%s tool_id=%d like_count=%d success", userInfo.UserID, id, likeCount)
	c.JSON(http.StatusOK, gin.H{
		"message":    "unliked",
		"like_count": likeCount,
	})
}

// UploadIcon handles POST /tools/icon - uploads icon to S3 and returns URL
func (h *Handler) UploadIcon(c *gin.Context) {
	userInfo := GetUserInfo(c)
	if userInfo.UserID == "" {
		respondUnauthorized(c, "User ID required")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondInvalidParameter(c, "file", "File is required")
		return
	}
	defer file.Close()

	log.Printf("[UploadIcon] user=%s filename=%s size=%d", userInfo.UserID, header.Filename, header.Size)

	// Validate file size (max 2MB)
	const maxSize = 2 * 1024 * 1024
	if header.Size > maxSize {
		respondWithError(c, http.StatusBadRequest, ErrCodeFileTooLarge, "File size exceeds 2MB limit")
		return
	}

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	allowedTypes := map[string]bool{
		"image/png":     true,
		"image/jpeg":    true,
		"image/jpg":     true,
		"image/svg+xml": true,
		"image/webp":    true,
	}
	if !allowedTypes[contentType] {
		respondWithError(c, http.StatusBadRequest, ErrCodeInvalidFileType, "Invalid file type, only png/jpg/svg/webp allowed")
		return
	}

	iconURL, err := h.toolService.UploadIcon(c.Request.Context(), userInfo.UserID, header.Filename, file)
	if err != nil {
		log.Printf("[UploadIcon] user=%s error=%v", userInfo.UserID, err)
		respondServiceError(c, err)
		return
	}

	log.Printf("[UploadIcon] user=%s icon_url=%s success", userInfo.UserID, iconURL)
	c.JSON(http.StatusOK, gin.H{"icon_url": iconURL})
}

// GetToolContent handles GET /tools/:id/content - returns raw SKILL.md content
func (h *Handler) GetToolContent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondInvalidParameter(c, "id", "ID must be a valid integer")
		return
	}

	userInfo := GetUserInfo(c)
	content, err := h.toolService.GetToolContent(c.Request.Context(), id, userInfo.UserID)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	// Return as plain text
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(content))
}
