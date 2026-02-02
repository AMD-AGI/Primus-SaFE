// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/gin-gonic/gin"
)

// RegisterWorkspaceRoutes registers workspace API routes
func RegisterWorkspaceRoutes(router *gin.Engine, h *Handler) {
	v1 := router.Group("/api/v1")
	{
		// Workspace CRUD
		v1.GET("/workspaces", h.ListWorkspaces)
		v1.GET("/workspaces/:name", h.GetWorkspace)
		v1.POST("/workspaces", h.CreateWorkspace)
		v1.PUT("/workspaces/:name", h.UpdateWorkspace)
		v1.DELETE("/workspaces/:name", h.DeleteWorkspace)

		// Workspace-Skill management
		v1.GET("/workspaces/:name/skills", h.ListWorkspaceSkills)
		v1.POST("/workspaces/:name/skills", h.AddSkillsToWorkspace)
		v1.DELETE("/workspaces/:name/skills", h.RemoveSkillsFromWorkspace)

		// Workspace-scoped skill operations
		v1.POST("/workspaces/:name/skills/search", h.SearchSkillsInWorkspace)
	}
}

// ======================== Workspace CRUD Handlers ========================

// ListWorkspaces lists all workspaces with pagination
func (h *Handler) ListWorkspaces(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	owner := c.Query("owner")

	var workspaces []*model.Workspace
	var total int64
	var err error

	if owner != "" {
		workspaces, total, err = h.registry.ListWorkspacesByOwner(c.Request.Context(), owner, offset, limit)
	} else {
		workspaces, total, err = h.registry.ListWorkspaces(c.Request.Context(), offset, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workspaces": workspaces,
		"total":      total,
		"offset":     offset,
		"limit":      limit,
	})
}

// GetWorkspace retrieves a workspace by name
func (h *Handler) GetWorkspace(c *gin.Context) {
	name := c.Param("name")

	workspace, err := h.registry.GetWorkspace(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	c.JSON(http.StatusOK, workspace)
}

// CreateWorkspaceRequest represents a request to create a workspace
type CreateWorkspaceRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Owner       string                 `json:"owner"`
	IsDefault   bool                   `json:"is_default"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CreateWorkspace creates a new workspace
func (h *Handler) CreateWorkspace(c *gin.Context) {
	var req CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	workspace := &model.Workspace{
		Name:        req.Name,
		Description: req.Description,
		Owner:       req.Owner,
		IsDefault:   req.IsDefault,
	}

	if req.Metadata != nil {
		workspace.Metadata = model.WorkspaceMetadata(req.Metadata)
	}

	if err := h.registry.CreateWorkspace(c.Request.Context(), workspace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, workspace)
}

// UpdateWorkspaceRequest represents a request to update a workspace
type UpdateWorkspaceRequest struct {
	Description string                 `json:"description"`
	Owner       string                 `json:"owner"`
	IsDefault   bool                   `json:"is_default"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// UpdateWorkspace updates an existing workspace
func (h *Handler) UpdateWorkspace(c *gin.Context) {
	name := c.Param("name")

	// Get existing workspace
	workspace, err := h.registry.GetWorkspace(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	var req UpdateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields if provided
	if req.Description != "" {
		workspace.Description = req.Description
	}
	if req.Owner != "" {
		workspace.Owner = req.Owner
	}
	workspace.IsDefault = req.IsDefault
	if req.Metadata != nil {
		workspace.Metadata = model.WorkspaceMetadata(req.Metadata)
	}

	if err := h.registry.UpdateWorkspace(c.Request.Context(), workspace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, workspace)
}

// DeleteWorkspace deletes a workspace by name
func (h *Handler) DeleteWorkspace(c *gin.Context) {
	name := c.Param("name")

	if err := h.registry.DeleteWorkspace(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workspace deleted successfully"})
}

// ======================== Workspace-Skill Management Handlers ========================

// ListWorkspaceSkills lists skills in a workspace
func (h *Handler) ListWorkspaceSkills(c *gin.Context) {
	name := c.Param("name")
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	skills, total, err := h.registry.ListSkillsByWorkspace(c.Request.Context(), name, offset, limit)
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

// WorkspaceSkillsRequest represents a request to add/remove skills
type WorkspaceSkillsRequest struct {
	Skills []string `json:"skills" binding:"required"`
}

// AddSkillsToWorkspace adds skills to a workspace
func (h *Handler) AddSkillsToWorkspace(c *gin.Context) {
	name := c.Param("name")

	var req WorkspaceSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.registry.AddSkillsToWorkspace(c.Request.Context(), name, req.Skills); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "skills added to workspace"})
}

// RemoveSkillsFromWorkspace removes skills from a workspace
func (h *Handler) RemoveSkillsFromWorkspace(c *gin.Context) {
	name := c.Param("name")

	var req WorkspaceSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.registry.RemoveSkillsFromWorkspace(c.Request.Context(), name, req.Skills); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "skills removed from workspace"})
}

// WorkspaceSearchRequest represents a search request within a workspace
type WorkspaceSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

// SearchSkillsInWorkspace performs semantic search within a workspace
func (h *Handler) SearchSkillsInWorkspace(c *gin.Context) {
	name := c.Param("name")

	var req WorkspaceSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit == 0 {
		req.Limit = 10
	}

	results, err := h.registry.SearchInWorkspace(c.Request.Context(), name, req.Query, req.Limit)
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
		"skills":    summaries,
		"total":     len(summaries),
		"workspace": name,
		"hint":      "Use GET /api/v1/skills/{name}/content to retrieve full skill content",
	})
}
