// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package cpconfig

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// ConfigResponse represents a config in API response
type ConfigResponse struct {
	ID          int64                  `json:"id"`
	Key         string                 `json:"key"`
	Value       map[string]interface{} `json:"value"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Version     int32                  `json:"version"`
	UpdatedAt   string                 `json:"updated_at"`
	UpdatedBy   string                 `json:"updated_by"`
}

func toConfigResponse(cfg *model.ControlPlaneConfig) *ConfigResponse {
	return &ConfigResponse{
		ID:          cfg.ID,
		Key:         cfg.Key,
		Value:       cfg.Value,
		Description: cfg.Description,
		Category:    cfg.Category,
		Version:     cfg.Version,
		UpdatedAt:   cfg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedBy:   cfg.UpdatedBy,
	}
}

// ListConfigs lists all control plane configurations
// @Summary List control plane configurations
// @Tags ControlPlaneConfig
// @Produce json
// @Param category query string false "Filter by category"
// @Success 200 {array} ConfigResponse
// @Router /management/config [get]
func ListConfigs(c *gin.Context) {
	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()
	category := c.Query("category")

	var configs []*model.ControlPlaneConfig
	var err error

	if category != "" {
		configs, err = cpClientSet.Facade.GetControlPlaneConfig().ListByCategory(ctx, category)
	} else {
		configs, err = cpClientSet.Facade.GetControlPlaneConfig().List(ctx)
	}

	if err != nil {
		log.Errorf("Failed to list configs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]*ConfigResponse, len(configs))
	for i, cfg := range configs {
		response[i] = toConfigResponse(cfg)
	}

	c.JSON(http.StatusOK, response)
}

// GetConfig gets a specific configuration by key
// @Summary Get configuration by key
// @Tags ControlPlaneConfig
// @Produce json
// @Param key path string true "Config Key"
// @Success 200 {object} ConfigResponse
// @Router /management/config/{key} [get]
func GetConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()
	cfg, err := cpClientSet.Facade.GetControlPlaneConfig().Get(ctx, key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found"})
		return
	}

	c.JSON(http.StatusOK, toConfigResponse(cfg))
}

// SetConfigRequest is the request for setting a config
type SetConfigRequest struct {
	Value       map[string]interface{} `json:"value" binding:"required"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
}

// SetConfig creates or updates a configuration
// @Summary Set configuration
// @Tags ControlPlaneConfig
// @Accept json
// @Produce json
// @Param key path string true "Config Key"
// @Param config body SetConfigRequest true "Config"
// @Success 200 {object} map[string]string
// @Router /management/config/{key} [put]
func SetConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	var req SetConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()

	// Get user from context
	updatedBy := "system"
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			updatedBy = u
		}
	}

	opts := []cpdb.SetConfigOption{
		cpdb.WithConfigUpdatedBy(updatedBy),
	}
	if req.Description != "" {
		opts = append(opts, cpdb.WithConfigDescription(req.Description))
	}
	if req.Category != "" {
		opts = append(opts, cpdb.WithConfigCategory(req.Category))
	}

	err := cpClientSet.Facade.GetControlPlaneConfig().Set(ctx, key, req.Value, opts...)
	if err != nil {
		log.Errorf("Failed to set config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Config %s updated by %s", key, updatedBy)
	c.JSON(http.StatusOK, gin.H{"message": "Configuration saved", "key": key})
}

// DeleteConfig deletes a configuration
// @Summary Delete configuration
// @Tags ControlPlaneConfig
// @Param key path string true "Config Key"
// @Success 204 "No Content"
// @Router /management/config/{key} [delete]
func DeleteConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()
	err := cpClientSet.Facade.GetControlPlaneConfig().Delete(ctx, key)
	if err != nil {
		log.Errorf("Failed to delete config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Config %s deleted", key)
	c.Status(http.StatusNoContent)
}

// GetInstallerConfig gets the installer configuration (convenience endpoint)
// @Summary Get installer configuration
// @Tags ControlPlaneConfig
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /management/config/installer [get]
func GetInstallerConfig(c *gin.Context) {
	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()
	cfg, err := cpClientSet.Facade.GetControlPlaneConfig().Get(ctx, model.ConfigKeyInstallerImage)
	if err != nil {
		// Return default values if not configured
		c.JSON(http.StatusOK, gin.H{
			"repository": "primussafe/primus-lens-installer",
			"tag":        "latest",
			"full_image": "primussafe/primus-lens-installer:latest",
		})
		return
	}

	repo := cfg.Value.GetString("repository")
	tag := cfg.Value.GetString("tag")
	if repo == "" {
		repo = "primussafe/primus-lens-installer"
	}
	if tag == "" {
		tag = "latest"
	}

	c.JSON(http.StatusOK, gin.H{
		"repository": repo,
		"tag":        tag,
		"full_image": repo + ":" + tag,
	})
}
