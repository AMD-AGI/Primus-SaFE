// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package sysconfig

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ListConfigs lists all system configurations
func ListConfigs(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	category := c.Query("category")
	mgr := config.NewManagerForCluster(clients.ClusterName)

	var filters []config.ListFilter
	if category != "" {
		filters = append(filters, config.WithCategoryFilter(category))
	}

	configs, err := mgr.List(c, filters...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, configs))
}

// GetConfig gets a specific configuration by key
func GetConfig(c *gin.Context) {
	key := c.Param("key")

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)
	cfg, err := mgr.GetRaw(c, key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, cfg))
}

// SetConfigRequest is the request for setting a config
type SetConfigRequest struct {
	Value       interface{} `json:"value" binding:"required"`
	Description string      `json:"description"`
	Category    string      `json:"category"`
}

// SetConfig creates or updates a configuration
func SetConfig(c *gin.Context) {
	key := c.Param("key")

	var req SetConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get user from context
	updatedBy := "system"
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			updatedBy = u
		}
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)

	opts := []config.SetOption{
		config.WithUpdatedBy(updatedBy),
		config.WithRecordHistory(true),
	}
	if req.Description != "" {
		opts = append(opts, config.WithDescription(req.Description))
	}
	if req.Category != "" {
		opts = append(opts, config.WithCategory(req.Category))
	}

	err = mgr.Set(c, key, req.Value, opts...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "Configuration saved", "key": key}))
}

// DeleteConfig deletes a configuration
func DeleteConfig(c *gin.Context) {
	key := c.Param("key")

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)
	err = mgr.Delete(c, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "Configuration deleted", "key": key}))
}

// GetConfigHistory gets the history of a configuration
func GetConfigHistory(c *gin.Context) {
	key := c.Param("key")

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	mgr := config.NewManagerForCluster(clients.ClusterName)
	history, err := mgr.GetHistory(c, key, 20) // Last 20 versions
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, history))
}

