// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
	"github.com/gin-gonic/gin"
)

// Handler handles API requests
type Handler struct {
	registry *registry.SkillsRegistry
	embedder embedding.Embedder
}

// NewHandler creates a new Handler
func NewHandler(reg *registry.SkillsRegistry, embedder embedding.Embedder) *Handler {
	return &Handler{
		registry: reg,
		embedder: embedder,
	}
}

// RegisterRoutes registers API routes
func RegisterRoutes(router *gin.Engine, h *Handler) {
	v1 := router.Group("/api/v1")
	{
		// Skills endpoints
		v1.GET("/skills", h.ListSkills)
		v1.GET("/skills/:name", h.GetSkill)
		v1.GET("/skills/:name/content", h.GetSkillContent)
		v1.POST("/skills/search", h.SearchSkills)

		// Health check
		v1.GET("/health", h.Health)
	}
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
