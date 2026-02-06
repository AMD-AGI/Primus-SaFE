// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/importer"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/runner"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
	"github.com/gin-gonic/gin"
)

// Handler handles API requests for tools
type Handler struct {
	facade    *database.ToolFacade
	runner    *runner.Runner
	storage   storage.Storage
	importer  *importer.Importer
	embedding *embedding.Service
}

// NewHandler creates a new Handler
func NewHandler(
	facade *database.ToolFacade,
	runner *runner.Runner,
	storage storage.Storage,
	embeddingSvc *embedding.Service,
) *Handler {
	var imp *importer.Importer
	if storage != nil {
		imp = importer.NewImporter(facade, storage)
	}
	return &Handler{
		facade:    facade,
		runner:    runner,
		storage:   storage,
		importer:  imp,
		embedding: embeddingSvc,
	}
}

// RegisterRoutes registers API routes
func RegisterRoutes(router *gin.Engine, h *Handler) {
	v1 := router.Group("/api/v1")
	{
		// Tools list and get
		v1.GET("/tools", h.ListTools)
		v1.GET("/tools/:id", h.GetTool)
		v1.PUT("/tools/:id", h.UpdateTool)
		v1.DELETE("/tools/:id", h.DeleteTool)

		// Create MCP (JSON) - Skills are created via import/discover + import/commit
		v1.POST("/tools/mcp", h.CreateMCP)

		// Search
		v1.GET("/tools/search", h.SearchTools)

		// Run tools
		v1.POST("/tools/run", h.RunTools)

		// Download
		v1.GET("/tools/:id/download", h.DownloadTool)

		// Import (batch)
		v1.POST("/tools/import/discover", h.ImportDiscover)
		v1.POST("/tools/import/commit", h.ImportCommit)

		// Health check
		v1.GET("/tools/health", h.Health)
	}
}

// ListTools lists all tools with pagination and sorting
func (h *Handler) ListTools(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	toolType := c.Query("type") // skill, mcp
	status := c.DefaultQuery("status", "active")
	sortField := c.DefaultQuery("sort", "created_at")
	sortOrder := c.DefaultQuery("order", "desc")

	// Validate sort field
	validSortFields := map[string]bool{
		"created_at":     true,
		"updated_at":     true,
		"run_count":      true,
		"download_count": true,
	}
	if !validSortFields[sortField] {
		sortField = "created_at"
	}

	// Validate sort order
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	tools, total, err := h.facade.List(toolType, status, sortField, sortOrder, offset, limit)
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

// GetTool retrieves a tool by ID
func (h *Handler) GetTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	tool, err := h.facade.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}

	c.JSON(http.StatusOK, tool)
}

// CreateMCPRequest represents a request to create an MCP server
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

// CreateMCP creates a new MCP server
func (h *Handler) CreateMCP(c *gin.Context) {
	var req CreateMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	// Store config as-is (full mcpServers JSON format)
	config := model.AppConfig(req.Config)

	tool := &model.Tool{
		Type:        model.AppTypeMCP,
		Name:        req.Name,
		DisplayName: displayName,
		Description: req.Description,
		Tags:        req.Tags,
		IconURL:     req.IconURL,
		Author:      req.Author,
		Config:      config,
		OwnerUserID: "", // Optional, empty by default
		IsPublic:    isPublic,
		Status:      model.AppStatusActive,
	}

	if err := h.facade.Create(tool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Generate embedding asynchronously
	h.generateEmbeddingAsync(tool)

	c.JSON(http.StatusCreated, tool)
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

// UpdateTool updates an existing tool
func (h *Handler) UpdateTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	tool, err := h.facade.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}

	var req UpdateToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields if provided
	if req.DisplayName != "" {
		tool.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		tool.Description = req.Description
	}
	if req.Tags != nil {
		tool.Tags = req.Tags
	}
	if req.IconURL != "" {
		tool.IconURL = req.IconURL
	}
	if req.Author != "" {
		tool.Author = req.Author
	}
	if req.IsPublic != nil {
		tool.IsPublic = *req.IsPublic
	}
	if req.Status != "" {
		tool.Status = req.Status
	}

	// Handle config update for skill (content update)
	if req.Config != nil {
		if tool.Type == model.AppTypeSkill {
			if content, ok := req.Config["content"].(string); ok && content != "" {
				s3Key := tool.GetSkillS3Key()
				if s3Key == "" {
					// Generate new S3 key using timestamp
					s3Key = fmt.Sprintf("skills/%d/SKILL.md", time.Now().UnixNano())
					tool.Config = map[string]interface{}{
						"s3_key":    s3Key,
						"is_prefix": false,
					}
				}
				if h.storage != nil {
					if err := h.storage.UploadBytes(c.Request.Context(), s3Key, []byte(content)); err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload skill content"})
						return
					}
				}
				// Keep existing s3_key, just update content in S3
			} else {
				tool.Config = req.Config
			}
		} else {
			tool.Config = req.Config
		}
	}

	if err := h.facade.Update(tool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tool)
}

// DeleteTool deletes a tool by ID
func (h *Handler) DeleteTool(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	tool, err := h.facade.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}

	// Delete S3 content for skill
	if tool.Type == model.AppTypeSkill {
		if s3Key := tool.GetSkillS3Key(); s3Key != "" {
			_ = h.storage.Delete(c.Request.Context(), s3Key)
		}
	}

	if err := h.facade.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tool deleted successfully"})
}

// SearchTools searches tools by query with different modes
func (h *Handler) SearchTools(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	toolType := c.Query("type")
	mode := c.DefaultQuery("mode", "semantic") // semantic, keyword, hybrid
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	switch mode {
	case "semantic":
		if h.embedding == nil || !h.embedding.IsEnabled() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "semantic search is not enabled"})
			return
		}
		emb, err := h.embedding.Generate(c.Request.Context(), query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate embedding"})
			return
		}
		results, err := h.facade.SemanticSearch(emb, toolType, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tools": results,
			"total": len(results),
			"mode":  "semantic",
		})

	case "hybrid":
		if h.embedding == nil || !h.embedding.IsEnabled() {
			// Fallback to keyword search
			tools, err := h.facade.Search(query, toolType, limit)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"tools": tools,
				"total": len(tools),
				"mode":  "keyword",
			})
			return
		}
		emb, err := h.embedding.Generate(c.Request.Context(), query)
		if err != nil {
			// Fallback to keyword search
			tools, err := h.facade.Search(query, toolType, limit)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"tools": tools,
				"total": len(tools),
				"mode":  "keyword",
			})
			return
		}
		results, err := h.facade.HybridSearch(query, emb, toolType, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tools": results,
			"total": len(results),
			"mode":  "hybrid",
		})

	default: // keyword
		tools, err := h.facade.Search(query, toolType, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tools": tools,
			"total": len(tools),
			"mode":  "keyword",
		})
	}
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

// RunTools runs multiple tools via the execution backend
func (h *Handler) RunTools(c *gin.Context) {
	var req RunToolsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "runner not configured"})
		return
	}

	// Load tools by ID or type+name
	var tools []*model.Tool
	for _, ref := range req.Tools {
		var tool *model.Tool
		var err error

		if ref.ID != nil {
			// Lookup by ID
			tool, err = h.facade.GetByID(*ref.ID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("tool not found: id=%d", *ref.ID)})
				return
			}
		} else if ref.Type != "" && ref.Name != "" {
			// Lookup by type + name
			tool, err = h.facade.GetByTypeAndName(ref.Type, ref.Name)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("tool not found: %s/%s", ref.Type, ref.Name)})
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each tool must have either 'id' or 'type'+'name'"})
			return
		}

		tools = append(tools, tool)
	}

	// Get redirect URL from runner
	result, err := h.runner.GetRunURL(c.Request.Context(), tools)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update run counts
	for _, tool := range tools {
		_ = h.facade.IncrementRunCount(tool.ID)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	tool, err := h.facade.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}

	if tool.Type == model.AppTypeSkill {
		// For skill, download as ZIP
		h.downloadSkillAsZip(c, tool)
	} else {
		// For MCP, download setup guide as markdown
		content := generateMCPSetupGuide(tool)
		filename := tool.Name + "-setup.md"

		_ = h.facade.IncrementDownloadCount(id)

		c.Header("Content-Disposition", "attachment; filename="+filename)
		c.Header("Content-Type", "text/markdown")
		c.String(http.StatusOK, content)
	}
}

// downloadSkillAsZip downloads skill files as a ZIP archive
func (h *Handler) downloadSkillAsZip(c *gin.Context, tool *model.Tool) {
	if h.storage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "storage not configured"})
		return
	}

	s3Key := tool.GetSkillS3Key()
	if s3Key == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill content not found"})
		return
	}

	// Check if it's a prefix (directory) or single file
	isPrefix := false
	if v, ok := tool.Config["is_prefix"].(bool); ok {
		isPrefix = v
	}

	// Create ZIP buffer
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	if isPrefix {
		// List and download all files in the directory
		objects, err := h.storage.ListObjects(c.Request.Context(), s3Key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list skill files"})
			return
		}

		for _, obj := range objects {
			data, err := h.storage.DownloadBytes(c.Request.Context(), obj.Key)
			if err != nil {
				continue
			}
			// Use relative path in ZIP
			relPath := strings.TrimPrefix(obj.Key, s3Key)
			relPath = strings.TrimPrefix(relPath, "/")
			if relPath == "" {
				relPath = filepath.Base(obj.Key)
			}

			w, err := zipWriter.Create(relPath)
			if err != nil {
				continue
			}
			w.Write(data)
		}
	} else {
		// Download single file
		data, err := h.storage.DownloadBytes(c.Request.Context(), s3Key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download skill content"})
			return
		}

		w, err := zipWriter.Create("SKILL.md")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create zip entry"})
			return
		}
		w.Write(data)
	}

	if err := zipWriter.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create zip file"})
		return
	}

	_ = h.facade.IncrementDownloadCount(tool.ID)

	filename := tool.Name + ".zip"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/zip")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// generateMCPSetupGuide generates a setup guide for MCP server
func generateMCPSetupGuide(tool *model.Tool) string {
	command, args, env := tool.GetMCPServerConfig()

	content := "# " + tool.Name + " - MCP Server Setup Guide\n\n"
	content += "## Description\n\n" + tool.Description + "\n\n"
	content += "## Cursor Configuration\n\n"
	content += "Add the following to your Cursor MCP settings:\n\n"
	content += "```json\n"
	content += "{\n"
	content += "  \"mcpServers\": {\n"
	content += "    \"" + tool.Name + "\": {\n"
	content += "      \"command\": \"" + command + "\",\n"
	content += "      \"args\": ["

	for i, arg := range args {
		if i > 0 {
			content += ", "
		}
		content += "\"" + arg + "\""
	}
	content += "]"

	if len(env) > 0 {
		content += ",\n      \"env\": {\n"
		first := true
		for k, v := range env {
			if !first {
				content += ",\n"
			}
			content += "        \"" + k + "\": \"" + v + "\""
			first = false
		}
		content += "\n      }"
	}

	content += "\n    }\n"
	content += "  }\n"
	content += "}\n"
	content += "```\n"

	return content
}

// Health returns service health status
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// ImportDiscoverRequest represents a request to discover skills from ZIP or GitHub
type ImportDiscoverRequest struct {
	GitHubURL string `form:"github_url"`
}

// ImportDiscover handles skill discovery from ZIP file or GitHub URL
func (h *Handler) ImportDiscover(c *gin.Context) {
	if h.importer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "import service not configured"})
		return
	}

	// Check for file upload
	file, header, fileErr := c.Request.FormFile("file")
	githubURL := c.PostForm("github_url")

	if fileErr != nil && githubURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either file or github_url must be provided"})
		return
	}

	if fileErr == nil && githubURL != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only one of file or github_url can be provided"})
		return
	}

	req := &importer.DiscoverRequest{
		UserID:    "", // Optional, empty by default
		GitHubURL: githubURL,
	}

	if fileErr == nil {
		defer file.Close()
		req.File = file
		req.FileName = header.Filename
	}

	result, err := h.importer.Discover(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ImportCommitRequest represents a request to commit selected skills
type ImportCommitRequest struct {
	ArchiveKey string               `json:"archive_key" binding:"required"`
	Selections []importer.Selection `json:"selections" binding:"required"`
}

// ImportCommit handles committing selected skills from a discovered archive
func (h *Handler) ImportCommit(c *gin.Context) {
	if h.importer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "import service not configured"})
		return
	}

	var req ImportCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Selections) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selections cannot be empty"})
		return
	}

	result, err := h.importer.Commit(c.Request.Context(), &importer.CommitRequest{
		UserID:     "", // Optional, empty by default
		ArchiveKey: req.ArchiveKey,
		Selections: req.Selections,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// generateEmbeddingAsync generates embedding for a tool asynchronously
func (h *Handler) generateEmbeddingAsync(tool *model.Tool) {
	if h.embedding == nil || !h.embedding.IsEnabled() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		emb, err := h.embedding.GenerateForTool(ctx, tool.Name, tool.Description)
		if err != nil {
			// Log error but don't fail the request
			fmt.Printf("Failed to generate embedding for tool %d: %v\n", tool.ID, err)
			return
		}

		if err := h.facade.UpdateEmbedding(tool.ID, emb); err != nil {
			fmt.Printf("Failed to update embedding for tool %d: %v\n", tool.ID, err)
		}
	}()
}
