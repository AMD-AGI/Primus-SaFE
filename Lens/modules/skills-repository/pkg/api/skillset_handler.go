// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/gin-gonic/gin"
)

// RegisterSkillsetRoutes registers skillset API routes
func RegisterSkillsetRoutes(router *gin.Engine, h *Handler) {
	v1 := router.Group("/api/v1")
	{
		// Skillset CRUD
		v1.GET("/skillsets", h.ListSkillsets)
		v1.GET("/skillsets/:name", h.GetSkillset)
		v1.POST("/skillsets", h.CreateSkillset)
		v1.PUT("/skillsets/:name", h.UpdateSkillset)
		v1.DELETE("/skillsets/:name", h.DeleteSkillset)

		// Skillset-Skill management
		v1.GET("/skillsets/:name/skills", h.ListSkillsetSkills)
		v1.POST("/skillsets/:name/skills", h.AddSkillsToSkillset)
		v1.DELETE("/skillsets/:name/skills", h.RemoveSkillsFromSkillset)

		// Skillset-scoped skill operations
		v1.POST("/skillsets/:name/skills/search", h.SearchSkillsInSkillset)
	}
}

// ======================== Skillset CRUD Handlers ========================

// ListSkillsets lists all skillsets with pagination
func (h *Handler) ListSkillsets(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	owner := c.Query("owner")

	var skillsets []*model.Skillset
	var total int64
	var err error

	if owner != "" {
		skillsets, total, err = h.registry.ListSkillsetsByOwner(c.Request.Context(), owner, offset, limit)
	} else {
		skillsets, total, err = h.registry.ListSkillsets(c.Request.Context(), offset, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"skillsets": skillsets,
		"total":     total,
		"offset":    offset,
		"limit":     limit,
	})
}

// GetSkillset retrieves a skillset by name
func (h *Handler) GetSkillset(c *gin.Context) {
	name := c.Param("name")

	skillset, err := h.registry.GetSkillset(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skillset not found"})
		return
	}

	c.JSON(http.StatusOK, skillset)
}

// CreateSkillsetRequest represents a request to create a skillset
type CreateSkillsetRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Owner       string                 `json:"owner"`
	IsDefault   bool                   `json:"is_default"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CreateSkillset creates a new skillset
func (h *Handler) CreateSkillset(c *gin.Context) {
	var req CreateSkillsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	skillset := &model.Skillset{
		Name:        req.Name,
		Description: req.Description,
		Owner:       req.Owner,
		IsDefault:   req.IsDefault,
	}

	if req.Metadata != nil {
		skillset.Metadata = model.SkillsetMetadata(req.Metadata)
	}

	if err := h.registry.CreateSkillset(c.Request.Context(), skillset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, skillset)
}

// UpdateSkillsetRequest represents a request to update a skillset
type UpdateSkillsetRequest struct {
	Description string                 `json:"description"`
	Owner       string                 `json:"owner"`
	IsDefault   bool                   `json:"is_default"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// UpdateSkillset updates an existing skillset
func (h *Handler) UpdateSkillset(c *gin.Context) {
	name := c.Param("name")

	// Get existing skillset
	skillset, err := h.registry.GetSkillset(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skillset not found"})
		return
	}

	var req UpdateSkillsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields if provided
	if req.Description != "" {
		skillset.Description = req.Description
	}
	if req.Owner != "" {
		skillset.Owner = req.Owner
	}
	skillset.IsDefault = req.IsDefault
	if req.Metadata != nil {
		skillset.Metadata = model.SkillsetMetadata(req.Metadata)
	}

	if err := h.registry.UpdateSkillset(c.Request.Context(), skillset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, skillset)
}

// DeleteSkillset deletes a skillset by name
func (h *Handler) DeleteSkillset(c *gin.Context) {
	name := c.Param("name")

	if err := h.registry.DeleteSkillset(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "skillset deleted successfully"})
}

// ======================== Skillset-Skill Management Handlers ========================

// ListSkillsetSkills lists skills in a skillset
func (h *Handler) ListSkillsetSkills(c *gin.Context) {
	name := c.Param("name")
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	skills, total, err := h.registry.ListSkillsBySkillset(c.Request.Context(), name, offset, limit)
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

// SkillsetSkillsRequest represents a request to add/remove skills
type SkillsetSkillsRequest struct {
	Skills []string `json:"skills" binding:"required"`
}

// AddSkillsToSkillset adds skills to a skillset
func (h *Handler) AddSkillsToSkillset(c *gin.Context) {
	name := c.Param("name")

	var req SkillsetSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.registry.AddSkillsToSkillset(c.Request.Context(), name, req.Skills); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "skills added to skillset"})
}

// RemoveSkillsFromSkillset removes skills from a skillset
func (h *Handler) RemoveSkillsFromSkillset(c *gin.Context) {
	name := c.Param("name")

	var req SkillsetSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.registry.RemoveSkillsFromSkillset(c.Request.Context(), name, req.Skills); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "skills removed from skillset"})
}

// SkillsetSearchRequest represents a search request within a skillset
type SkillsetSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

// SearchSkillsInSkillset performs semantic search within a skillset
func (h *Handler) SearchSkillsInSkillset(c *gin.Context) {
	name := c.Param("name")

	var req SkillsetSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	results, err := h.registry.SearchInSkillset(c.Request.Context(), name, req.Query, req.Limit)
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
		"skills":   summaries,
		"total":    len(summaries),
		"skillset": name,
		"hint":     "Use GET /api/v1/skills/{name}/content to retrieve full skill content",
	})
}
