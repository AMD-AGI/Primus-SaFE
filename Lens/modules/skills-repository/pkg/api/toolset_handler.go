// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/service"
	"github.com/gin-gonic/gin"
)

// --- Toolset Request Types ---

// CreateToolsetRequest represents a request to create a toolset
type CreateToolsetRequest struct {
	Name        string   `json:"name" binding:"required"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	IconURL     string   `json:"icon_url"`
	IsPublic    *bool    `json:"is_public"`
	ToolIDs     []int64  `json:"tool_ids"` // Optional: add tools on creation
}

// UpdateToolsetRequest represents a request to update a toolset
type UpdateToolsetRequest struct {
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	IconURL     string   `json:"icon_url"`
	IsPublic    *bool    `json:"is_public"`
}

// AddToolsToToolsetRequest represents a request to add tools to a toolset
type AddToolsToToolsetRequest struct {
	ToolIDs []int64 `json:"tool_ids" binding:"required"`
}

// --- Toolset Handler Methods ---

// ListToolsets lists all toolsets with pagination and sorting
func (h *Handler) ListToolsets(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	userInfo := GetUserInfo(c)

	result, err := h.toolsetService.List(c.Request.Context(), service.ToolsetListInput{
		Owner:     c.Query("owner"),
		SortField: c.DefaultQuery("sort", "created_at"),
		SortOrder: c.DefaultQuery("order", "desc"),
		Offset:    offset,
		Limit:     limit,
		UserID:    userInfo.UserID,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"toolsets": result.Toolsets,
		"total":    result.Total,
		"offset":   result.Offset,
		"limit":    result.Limit,
	})
}

// GetToolset retrieves a toolset by ID with its tools
func (h *Handler) GetToolset(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userInfo := GetUserInfo(c)
	result, err := h.toolsetService.GetToolset(c.Request.Context(), id, userInfo.UserID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// CreateToolset creates a new toolset
func (h *Handler) CreateToolset(c *gin.Context) {
	var req CreateToolsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userInfo := GetUserInfo(c)
	toolset, err := h.toolsetService.Create(c.Request.Context(), service.CreateToolsetInput{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Tags:        req.Tags,
		IconURL:     req.IconURL,
		IsPublic:    req.IsPublic,
		ToolIDs:     req.ToolIDs,
		UserID:      userInfo.UserID,
		Username:    userInfo.Username,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, toolset)
}

// UpdateToolset updates an existing toolset
func (h *Handler) UpdateToolset(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateToolsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userInfo := GetUserInfo(c)
	toolset, err := h.toolsetService.UpdateToolset(c.Request.Context(), id, service.UpdateToolsetInput{
		DisplayName: req.DisplayName,
		Description: req.Description,
		Tags:        req.Tags,
		IconURL:     req.IconURL,
		IsPublic:    req.IsPublic,
	}, userInfo.UserID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, toolset)
}

// DeleteToolset deletes a toolset by ID
func (h *Handler) DeleteToolset(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	userInfo := GetUserInfo(c)
	if err := h.toolsetService.DeleteToolset(c.Request.Context(), id, userInfo.UserID); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "toolset deleted successfully"})
}

// AddToolsToToolset adds tools to a toolset
func (h *Handler) AddToolsToToolset(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req AddToolsToToolsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userInfo := GetUserInfo(c)
	added, err := h.toolsetService.AddTools(c.Request.Context(), id, service.AddToolsInput{
		ToolIDs: req.ToolIDs,
	}, userInfo.UserID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "tools added successfully",
		"added":   added,
	})
}

// RemoveToolFromToolset removes a tool from a toolset
func (h *Handler) RemoveToolFromToolset(c *gin.Context) {
	toolsetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid toolset id"})
		return
	}

	toolID, err := strconv.ParseInt(c.Param("toolId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool id"})
		return
	}

	userInfo := GetUserInfo(c)
	if err := h.toolsetService.RemoveTool(c.Request.Context(), toolsetID, toolID, userInfo.UserID); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tool removed from toolset"})
}

// SearchToolsets searches toolsets by query
func (h *Handler) SearchToolsets(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	mode := c.DefaultQuery("mode", "semantic")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	userInfo := GetUserInfo(c)

	result, err := h.toolsetService.Search(c.Request.Context(), query, mode, limit, userInfo.UserID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"toolsets": result.Toolsets,
		"total":    result.Total,
		"mode":     result.Mode,
	})
}
