// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	regconf "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/registry"
	"github.com/gin-gonic/gin"
)

// GetRegistryConfigRequest is the request for getting registry config
type GetRegistryConfigRequest struct {
	Cluster string `form:"cluster"`
}

// SetRegistryConfigRequest is the request for setting registry config
type SetRegistryConfigRequest struct {
	Registry          string            `json:"registry" binding:"required"`
	Namespace         string            `json:"namespace"`
	HarborExternalURL string            `json:"harbor_external_url"`
	ImageVersions     map[string]string `json:"image_versions"`
}

// SyncFromHarborRequest is the request for syncing from Harbor secret
type SyncFromHarborRequest struct {
	HarborExternalURL string `json:"harbor_external_url" binding:"required"`
}

// GetRegistryConfig returns the current registry configuration
func GetRegistryConfig(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	cfg, err := regconf.GetConfig(c, clients.ClusterName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"config":   cfg,
		"defaults": gin.H{
			"registry":  regconf.DefaultRegistry,
			"namespace": regconf.DefaultNamespace,
		},
		"image_names": gin.H{
			"tracelens":       regconf.ImageTraceLens,
			"perfetto_viewer": regconf.ImagePerfettoViewer,
		},
	}))
}

// SetRegistryConfig sets the registry configuration
func SetRegistryConfig(c *gin.Context) {
	var req SetRegistryConfigRequest
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

	// Get user from context (from auth middleware)
	updatedBy := "system"
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			updatedBy = u
		}
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = regconf.DefaultNamespace
	}

	cfg := &regconf.Config{
		Registry:          req.Registry,
		Namespace:         namespace,
		HarborExternalURL: req.HarborExternalURL,
		ImageVersions:     req.ImageVersions,
	}

	if err := regconf.SetConfig(c, clients.ClusterName, cfg, updatedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"message": "Registry configuration updated",
		"config":  cfg,
	}))
}

// SyncFromHarbor syncs the registry configuration from a Harbor external URL
// This is a manual operation that extracts the hostname from the URL
func SyncFromHarbor(c *gin.Context) {
	var req SyncFromHarborRequest
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

	if err := regconf.SyncFromHarborSecret(c, clients.ClusterName, req.HarborExternalURL, updatedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return the new config
	cfg, _ := regconf.GetConfig(c, clients.ClusterName)

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"message": "Registry configuration synced from Harbor",
		"config":  cfg,
	}))
}

// GetImageURL returns the full image URL for a given image name
func GetImageURL(c *gin.Context) {
	imageName := c.Query("image")
	if imageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image parameter is required"})
		return
	}

	tag := c.Query("tag")
	if tag == "" {
		tag = "latest"
	}

	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	imageURL := regconf.GetImageURLForCluster(c, clients.ClusterName, imageName, tag)

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"image_name": imageName,
		"tag":        tag,
		"image_url":  imageURL,
	}))
}

