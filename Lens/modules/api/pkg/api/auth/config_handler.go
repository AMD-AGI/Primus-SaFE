// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"net/http"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListSystemConfigs lists all system configurations
// GET /api/v1/admin/configs
func ListSystemConfigs(c *gin.Context) {
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	category := c.Query("category")

	var configs []*model.LensSystemConfigs
	var err error

	if category != "" {
		configs, err = facade.GetSystemConfig().ListByCategory(ctx, category)
	} else {
		configs, err = facade.GetSystemConfig().ListAll(ctx)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &ListSystemConfigsResponse{
		Configs: make([]*SystemConfigResponse, len(configs)),
	}

	for i, cfg := range configs {
		resp.Configs[i] = toSystemConfigResponse(cfg)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// GetSystemConfig gets a single system configuration by key
// GET /api/v1/admin/configs/:key
func GetSystemConfig(c *gin.Context) {
	key := c.Param("key")
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	config, err := facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := toSystemConfigResponse(config)
	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// UpdateSystemConfig updates a system configuration
// PUT /api/v1/admin/configs/:key
func UpdateSystemConfig(c *gin.Context) {
	key := c.Param("key")
	var req UpdateSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Get existing config to preserve category and other fields
	existing, err := facade.GetSystemConfig().Get(ctx, key)
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Prepare value map
	valueMap := model.ExtType{"value": req.Value}

	now := time.Now()
	config := &model.LensSystemConfigs{
		Key:         key,
		Value:       valueMap,
		UpdatedAt:   now,
	}

	// Preserve existing fields if updating
	if existing != nil {
		config.Category = existing.Category
		config.IsSecret = existing.IsSecret
		config.CreatedAt = existing.CreatedAt
		if req.Description != "" {
			config.Description = req.Description
		} else {
			config.Description = existing.Description
		}
	} else {
		// New config
		config.Category = "custom"
		config.CreatedAt = now
		config.Description = req.Description
	}

	if err := facade.GetSystemConfig().Set(ctx, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch updated config
	updated, err := facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := toSystemConfigResponse(updated)
	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// DeleteSystemConfig deletes a system configuration
// DELETE /api/v1/admin/configs/:key
func DeleteSystemConfig(c *gin.Context) {
	key := c.Param("key")
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Check if config exists
	_, err := facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := facade.GetSystemConfig().Delete(ctx, key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "config deleted successfully"}))
}

// Helper functions

func toSystemConfigResponse(cfg *model.LensSystemConfigs) *SystemConfigResponse {
	resp := &SystemConfigResponse{
		Key:         cfg.Key,
		Category:    cfg.Category,
		Description: cfg.Description,
		IsSecret:    cfg.IsSecret,
		UpdatedAt:   cfg.UpdatedAt,
	}

	// Extract value from ExtType
	if v, ok := cfg.Value["value"]; ok {
		if cfg.IsSecret {
			// Mask secret values
			resp.Value = "********"
		} else {
			resp.Value = v
		}
	} else {
		resp.Value = cfg.Value
	}

	return resp
}
