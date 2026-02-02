// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/importer"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
	"github.com/gin-gonic/gin"
)

// Handler handles API requests
type Handler struct {
	registry *registry.SkillsRegistry
	embedder embedding.Embedder
	importer *importer.SkillImporter
}

// NewHandler creates a new Handler
func NewHandler(reg *registry.SkillsRegistry, embedder embedding.Embedder) *Handler {
	return &Handler{
		registry: reg,
		embedder: embedder,
		importer: importer.NewSkillImporter(reg, ""),
	}
}

// SetGitHubToken sets the GitHub token for the importer
func (h *Handler) SetGitHubToken(token string) {
	h.importer = importer.NewSkillImporter(h.registry, token)
}

// RegisterRoutes registers API routes
func RegisterRoutes(router *gin.Engine, h *Handler) {
	v1 := router.Group("/api/v1")
	{
		// Skills endpoints - Read
		v1.GET("/skills", h.ListSkills)
		v1.GET("/skills/:name", h.GetSkill)
		v1.GET("/skills/:name/content", h.GetSkillContent)
		
		// Skills endpoints - Search (must be before generic POST)
		v1.POST("/skills/search", h.SearchSkills)
		
		// Skills endpoints - Create/Update/Delete
		v1.POST("/skills", h.CreateSkill)
		v1.PUT("/skills/:name", h.UpdateSkill)
		v1.DELETE("/skills/:name", h.DeleteSkill)

		// Skills import endpoints
		v1.POST("/skills/import/github", h.ImportFromGitHub)
		v1.POST("/skills/import/file", h.ImportFromFile)

		// Health check
		v1.GET("/health", h.Health)
	}

	// Register skillset routes
	RegisterSkillsetRoutes(router, h)
}

// ListSkills lists all skills with pagination
func (h *Handler) ListSkills(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	category := c.Query("category")
	source := c.Query("source")

	var skills interface{}
	var total int64
	var err error

	if category != "" {
		skills, total, err = h.registry.ListByCategory(c.Request.Context(), category, offset, limit)
	} else if source != "" {
		skills, total, err = h.registry.ListBySource(c.Request.Context(), source, offset, limit)
	} else {
		skills, total, err = h.registry.List(c.Request.Context(), offset, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"skills": skills,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

// GetSkill retrieves a skill by name
func (h *Handler) GetSkill(c *gin.Context) {
	name := c.Param("name")

	skill, err := h.registry.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	c.JSON(http.StatusOK, skill)
}

// GetSkillContent retrieves the full SKILL.md content
func (h *Handler) GetSkillContent(c *gin.Context) {
	name := c.Param("name")

	content, err := h.registry.GetContent(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	c.Header("Content-Type", "text/markdown")
	c.String(http.StatusOK, content)
}

// SearchRequest represents a search request
type SearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

// SearchSkills performs semantic search for skills
func (h *Handler) SearchSkills(c *gin.Context) {
	var req SearchRequest
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

	// Format results
	type SkillSummary struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Category    string  `json:"category"`
		Score       float64 `json:"relevance_score"`
	}

	summaries := make([]SkillSummary, len(results))
	for i, r := range results {
		summaries[i] = SkillSummary{
			Name:        r.Skill.Name,
			Description: r.Skill.Description,
			Category:    r.Skill.Category,
			Score:       r.Score,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"skills": summaries,
		"total":  len(summaries),
		"hint":   "Use GET /api/v1/skills/{name}/content to retrieve full skill content",
	})
}

// Health returns service health status
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// CreateSkillRequest represents a request to create a skill
type CreateSkillRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description" binding:"required"`
	Category    string                 `json:"category"`
	Version     string                 `json:"version"`
	Source      string                 `json:"source"`
	License     string                 `json:"license"`
	Content     string                 `json:"content"`
	FilePath    string                 `json:"file_path"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CreateSkill creates a new skill
func (h *Handler) CreateSkill(c *gin.Context) {
	var req CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default source if not provided
	if req.Source == "" {
		req.Source = "manual"
	}

	skill := &model.Skill{
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Version:     req.Version,
		Source:      req.Source,
		License:     req.License,
		Content:     req.Content,
		FilePath:    req.FilePath,
	}

	if req.Metadata != nil {
		skill.Metadata = model.SkillsMetadata(req.Metadata)
	}

	if err := h.registry.Register(c.Request.Context(), skill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, skill)
}

// UpdateSkillRequest represents a request to update a skill
type UpdateSkillRequest struct {
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Version     string                 `json:"version"`
	License     string                 `json:"license"`
	Content     string                 `json:"content"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// UpdateSkill updates an existing skill
func (h *Handler) UpdateSkill(c *gin.Context) {
	name := c.Param("name")

	// Get existing skill
	skill, err := h.registry.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	var req UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields if provided
	if req.Description != "" {
		skill.Description = req.Description
	}
	if req.Category != "" {
		skill.Category = req.Category
	}
	if req.Version != "" {
		skill.Version = req.Version
	}
	if req.License != "" {
		skill.License = req.License
	}
	if req.Content != "" {
		skill.Content = req.Content
	}
	if req.Metadata != nil {
		skill.Metadata = model.SkillsMetadata(req.Metadata)
	}

	if err := h.registry.Register(c.Request.Context(), skill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, skill)
}

// DeleteSkill deletes a skill by name
func (h *Handler) DeleteSkill(c *gin.Context) {
	name := c.Param("name")

	if err := h.registry.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "skill deleted successfully"})
}

// ImportGitHubRequest represents a request to import from GitHub
type ImportGitHubRequest struct {
	URL         string `json:"url" binding:"required"`
	GitHubToken string `json:"github_token"`
}

// ImportFromGitHub imports skills from a GitHub repository
func (h *Handler) ImportFromGitHub(c *gin.Context) {
	var req ImportGitHubRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use provided token or environment token
	imp := h.importer
	if req.GitHubToken != "" {
		imp = importer.NewSkillImporter(h.registry, req.GitHubToken)
	}

	result, err := imp.ImportFromGitHub(c.Request.Context(), req.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Import completed",
		"imported": result.Imported,
		"skipped":  result.Skipped,
		"errors":   result.Errors,
	})
}

// ImportFromFile imports skills from an uploaded file
func (h *Handler) ImportFromFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	result, err := h.importer.ImportFromFile(c.Request.Context(), header.Filename, content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Import completed",
		"imported": result.Imported,
		"skipped":  result.Skipped,
		"errors":   result.Errors,
	})
}
