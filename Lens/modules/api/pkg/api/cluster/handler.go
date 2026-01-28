// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package cluster

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// ClusterConfigRequest represents a cluster configuration request
type ClusterConfigRequest struct {
	ClusterName   string `json:"cluster_name" binding:"required"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	Source        string `json:"source"` // manual or primus-safe

	// K8S Connection Config
	K8SEndpoint           string `json:"k8s_endpoint"`
	K8SCAData             string `json:"k8s_ca_data"`
	K8SCertData           string `json:"k8s_cert_data"`
	K8SKeyData            string `json:"k8s_key_data"`
	K8SToken              string `json:"k8s_token"`
	K8SInsecureSkipVerify *bool  `json:"k8s_insecure_skip_verify,omitempty"`
	K8SManualMode         *bool  `json:"k8s_manual_mode,omitempty"` // When true, K8S config won't be overwritten by adapter

	// Storage Config
	PostgresHost     string `json:"postgres_host"`
	PostgresPort     int    `json:"postgres_port"`
	PostgresUsername string `json:"postgres_username"`
	PostgresPassword string `json:"postgres_password"`
	PostgresDBName   string `json:"postgres_db_name"`
	PostgresSSLMode  string `json:"postgres_ssl_mode"`

	OpensearchHost     string `json:"opensearch_host"`
	OpensearchPort     int    `json:"opensearch_port"`
	OpensearchUsername string `json:"opensearch_username"`
	OpensearchPassword string `json:"opensearch_password"`
	OpensearchScheme   string `json:"opensearch_scheme"`

	PrometheusReadHost  string `json:"prometheus_read_host"`
	PrometheusReadPort  int    `json:"prometheus_read_port"`
	PrometheusWriteHost string `json:"prometheus_write_host"`
	PrometheusWritePort int    `json:"prometheus_write_port"`
	StorageManualMode   *bool  `json:"storage_manual_mode,omitempty"` // When true, storage config won't be overwritten by sync job

	// Storage Mode
	StorageMode          string                     `json:"storage_mode"` // external or lens-managed
	ManagedStorageConfig *model.ManagedStorageJSON  `json:"managed_storage_config"`

	// Labels
	Labels map[string]string `json:"labels"`
}

// ClusterConfigResponse represents a cluster configuration response
type ClusterConfigResponse struct {
	ID            int32             `json:"id"`
	ClusterName   string            `json:"cluster_name"`
	DisplayName   string            `json:"display_name"`
	Description   string            `json:"description"`
	Source        string            `json:"source"`

	// K8S Connection (sensitive data masked)
	K8SEndpoint           string `json:"k8s_endpoint"`
	K8SConfigured         bool   `json:"k8s_configured"`
	K8SInsecureSkipVerify bool   `json:"k8s_insecure_skip_verify"`
	K8SManualMode         bool   `json:"k8s_manual_mode"`

	// Storage Config (sensitive data masked)
	PostgresHost       string `json:"postgres_host"`
	PostgresPort       int    `json:"postgres_port"`
	PostgresConfigured bool   `json:"postgres_configured"`

	OpensearchHost       string `json:"opensearch_host"`
	OpensearchPort       int    `json:"opensearch_port"`
	OpensearchConfigured bool   `json:"opensearch_configured"`

	PrometheusReadHost   string `json:"prometheus_read_host"`
	PrometheusReadPort   int    `json:"prometheus_read_port"`
	PrometheusWriteHost  string `json:"prometheus_write_host"`
	PrometheusWritePort  int    `json:"prometheus_write_port"`
	PrometheusConfigured bool   `json:"prometheus_configured"`
	StorageManualMode    bool   `json:"storage_manual_mode"`

	// Status
	StorageMode      string                    `json:"storage_mode"`
	DataplaneStatus  string                    `json:"dataplane_status"`
	DataplaneVersion string                    `json:"dataplane_version"`
	DataplaneMessage string                    `json:"dataplane_message"`
	LastDeployTime   string                    `json:"last_deploy_time"`
	Status           string                    `json:"status"`
	IsDefault        bool                      `json:"is_default"`
	Labels           map[string]string         `json:"labels"`
	CreatedAt        string                    `json:"created_at"`
	UpdatedAt        string                    `json:"updated_at"`

	ManagedStorageConfig *model.ManagedStorageJSON `json:"managed_storage_config,omitempty"`
}

// toResponse converts model to response (masks sensitive data)
func toResponse(c *model.ClusterConfig) *ClusterConfigResponse {
	resp := &ClusterConfigResponse{
		ID:                    c.ID,
		ClusterName:           c.ClusterName,
		DisplayName:           c.DisplayName,
		Description:           c.Description,
		Source:                c.Source,
		K8SEndpoint:           c.K8SEndpoint,
		K8SConfigured:         c.K8SEndpoint != "" && (c.K8SToken != "" || (c.K8SCertData != "" && c.K8SKeyData != "")),
		K8SInsecureSkipVerify: c.K8SInsecureSkipVerify,
		K8SManualMode:         c.K8SManualMode,
		PostgresHost:          c.PostgresHost,
		PostgresPort:          c.PostgresPort,
		PostgresConfigured:    c.PostgresHost != "" && c.PostgresUsername != "",
		OpensearchHost:        c.OpensearchHost,
		OpensearchPort:        c.OpensearchPort,
		OpensearchConfigured:  c.OpensearchHost != "" && c.OpensearchUsername != "",
		PrometheusReadHost:    c.PrometheusReadHost,
		PrometheusReadPort:    c.PrometheusReadPort,
		PrometheusWriteHost:   c.PrometheusWriteHost,
		PrometheusWritePort:   c.PrometheusWritePort,
		PrometheusConfigured:  c.PrometheusReadHost != "" || c.PrometheusWriteHost != "",
		StorageManualMode:     c.StorageManualMode,
		StorageMode:           c.StorageMode,
		DataplaneStatus:       c.DataplaneStatus,
		DataplaneVersion:      c.DataplaneVersion,
		DataplaneMessage:      c.DataplaneMessage,
		Status:                c.Status,
		IsDefault:             c.IsDefault,
		Labels:                c.Labels,
		CreatedAt:             c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:             c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if c.LastDeployTime != nil {
		resp.LastDeployTime = c.LastDeployTime.Format("2006-01-02T15:04:05Z07:00")
	}

	if c.StorageMode == "lens-managed" {
		resp.ManagedStorageConfig = &c.ManagedStorageConfig
	}

	return resp
}

// ListClusters lists all cluster configurations
// @Summary List all clusters
// @Tags Clusters
// @Produce json
// @Success 200 {array} ClusterConfigResponse
// @Router /management/clusters [get]
func ListClusters(c *gin.Context) {
	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	clusters, err := cpClientSet.Facade.ClusterConfig.List(c.Request.Context())
	if err != nil {
		log.Errorf("Failed to list clusters: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]*ClusterConfigResponse, len(clusters))
	for i, cluster := range clusters {
		response[i] = toResponse(cluster)
	}

	c.JSON(http.StatusOK, response)
}

// GetCluster gets a cluster configuration by name
// @Summary Get cluster by name
// @Tags Clusters
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Success 200 {object} ClusterConfigResponse
// @Router /management/clusters/{cluster_name} [get]
func GetCluster(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name is required"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	cluster, err := cpClientSet.Facade.ClusterConfig.GetByName(c.Request.Context(), clusterName)
	if err != nil {
		log.Errorf("Failed to get cluster %s: %v", clusterName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return
	}

	c.JSON(http.StatusOK, toResponse(cluster))
}

// CreateCluster creates a new cluster configuration
// @Summary Create a new cluster
// @Tags Clusters
// @Accept json
// @Produce json
// @Param cluster body ClusterConfigRequest true "Cluster Config"
// @Success 201 {object} ClusterConfigResponse
// @Router /management/clusters [post]
func CreateCluster(c *gin.Context) {
	var req ClusterConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	// Check if cluster already exists
	exists, err := cpClientSet.Facade.ClusterConfig.Exists(c.Request.Context(), req.ClusterName)
	if err != nil {
		log.Errorf("Failed to check cluster existence: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "Cluster already exists"})
		return
	}

	// Set defaults
	source := req.Source
	if source == "" {
		source = model.ClusterSourceManual
	}
	storageMode := req.StorageMode
	if storageMode == "" {
		storageMode = "external"
	}
	postgresPort := req.PostgresPort
	if postgresPort == 0 {
		postgresPort = 5432
	}
	opensearchPort := req.OpensearchPort
	if opensearchPort == 0 {
		opensearchPort = 9200
	}

	cluster := &model.ClusterConfig{
		ClusterName:         req.ClusterName,
		DisplayName:         req.DisplayName,
		Description:         req.Description,
		Source:              source,
		K8SEndpoint:         req.K8SEndpoint,
		K8SCAData:           req.K8SCAData,
		K8SCertData:         req.K8SCertData,
		K8SKeyData:          req.K8SKeyData,
		K8SToken:            req.K8SToken,
		PostgresHost:        req.PostgresHost,
		PostgresPort:        postgresPort,
		PostgresUsername:    req.PostgresUsername,
		PostgresPassword:    req.PostgresPassword,
		PostgresDBName:      req.PostgresDBName,
		PostgresSSLMode:     req.PostgresSSLMode,
		OpensearchHost:      req.OpensearchHost,
		OpensearchPort:      opensearchPort,
		OpensearchUsername:  req.OpensearchUsername,
		OpensearchPassword:  req.OpensearchPassword,
		OpensearchScheme:    req.OpensearchScheme,
		PrometheusReadHost:  req.PrometheusReadHost,
		PrometheusReadPort:  req.PrometheusReadPort,
		PrometheusWriteHost: req.PrometheusWriteHost,
		PrometheusWritePort: req.PrometheusWritePort,
		StorageMode:         storageMode,
		DataplaneStatus:     model.DataplaneStatusPending,
		Status:              model.ClusterStatusActive,
		Labels:              req.Labels,
	}

	// Set manual mode flags
	if req.K8SInsecureSkipVerify != nil {
		cluster.K8SInsecureSkipVerify = *req.K8SInsecureSkipVerify
	}
	if req.K8SManualMode != nil {
		cluster.K8SManualMode = *req.K8SManualMode
	}
	if req.StorageManualMode != nil {
		cluster.StorageManualMode = *req.StorageManualMode
	}

	if req.ManagedStorageConfig != nil {
		cluster.ManagedStorageConfig = *req.ManagedStorageConfig
	}

	if err := cpClientSet.Facade.ClusterConfig.Create(c.Request.Context(), cluster); err != nil {
		log.Errorf("Failed to create cluster: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Created cluster: %s", req.ClusterName)
	c.JSON(http.StatusCreated, toResponse(cluster))
}

// UpdateCluster updates a cluster configuration
// @Summary Update a cluster
// @Tags Clusters
// @Accept json
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Param cluster body ClusterConfigRequest true "Cluster Config"
// @Success 200 {object} ClusterConfigResponse
// @Router /management/clusters/{cluster_name} [put]
func UpdateCluster(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name is required"})
		return
	}

	var req ClusterConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	// Get existing cluster
	cluster, err := cpClientSet.Facade.ClusterConfig.GetByName(c.Request.Context(), clusterName)
	if err != nil {
		log.Errorf("Failed to get cluster %s: %v", clusterName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return
	}

	// Update fields
	if req.DisplayName != "" {
		cluster.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		cluster.Description = req.Description
	}
	if req.K8SEndpoint != "" {
		cluster.K8SEndpoint = req.K8SEndpoint
	}
	if req.K8SCAData != "" {
		cluster.K8SCAData = req.K8SCAData
	}
	if req.K8SCertData != "" {
		cluster.K8SCertData = req.K8SCertData
	}
	if req.K8SKeyData != "" {
		cluster.K8SKeyData = req.K8SKeyData
	}
	if req.K8SToken != "" {
		cluster.K8SToken = req.K8SToken
	}
	if req.PostgresHost != "" {
		cluster.PostgresHost = req.PostgresHost
	}
	if req.PostgresPort > 0 {
		cluster.PostgresPort = req.PostgresPort
	}
	if req.PostgresUsername != "" {
		cluster.PostgresUsername = req.PostgresUsername
	}
	if req.PostgresPassword != "" {
		cluster.PostgresPassword = req.PostgresPassword
	}
	if req.PostgresDBName != "" {
		cluster.PostgresDBName = req.PostgresDBName
	}
	if req.PostgresSSLMode != "" {
		cluster.PostgresSSLMode = req.PostgresSSLMode
	}
	if req.OpensearchHost != "" {
		cluster.OpensearchHost = req.OpensearchHost
	}
	if req.OpensearchPort > 0 {
		cluster.OpensearchPort = req.OpensearchPort
	}
	if req.OpensearchUsername != "" {
		cluster.OpensearchUsername = req.OpensearchUsername
	}
	if req.OpensearchPassword != "" {
		cluster.OpensearchPassword = req.OpensearchPassword
	}
	if req.OpensearchScheme != "" {
		cluster.OpensearchScheme = req.OpensearchScheme
	}
	if req.PrometheusReadHost != "" {
		cluster.PrometheusReadHost = req.PrometheusReadHost
	}
	if req.PrometheusReadPort > 0 {
		cluster.PrometheusReadPort = req.PrometheusReadPort
	}
	if req.PrometheusWriteHost != "" {
		cluster.PrometheusWriteHost = req.PrometheusWriteHost
	}
	if req.PrometheusWritePort > 0 {
		cluster.PrometheusWritePort = req.PrometheusWritePort
	}
	if req.StorageMode != "" {
		cluster.StorageMode = req.StorageMode
	}
	if req.ManagedStorageConfig != nil {
		cluster.ManagedStorageConfig = *req.ManagedStorageConfig
	}
	if req.Labels != nil {
		cluster.Labels = req.Labels
	}
	// Handle manual mode flags
	if req.K8SInsecureSkipVerify != nil {
		cluster.K8SInsecureSkipVerify = *req.K8SInsecureSkipVerify
	}
	if req.K8SManualMode != nil {
		cluster.K8SManualMode = *req.K8SManualMode
	}
	if req.StorageManualMode != nil {
		cluster.StorageManualMode = *req.StorageManualMode
	}

	if err := cpClientSet.Facade.ClusterConfig.Update(c.Request.Context(), cluster); err != nil {
		log.Errorf("Failed to update cluster: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Updated cluster: %s", clusterName)
	c.JSON(http.StatusOK, toResponse(cluster))
}

// DeleteCluster deletes a cluster configuration
// @Summary Delete a cluster
// @Tags Clusters
// @Param cluster_name path string true "Cluster Name"
// @Success 204 "No Content"
// @Router /management/clusters/{cluster_name} [delete]
func DeleteCluster(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name is required"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	if err := cpClientSet.Facade.ClusterConfig.Delete(c.Request.Context(), clusterName); err != nil {
		log.Errorf("Failed to delete cluster %s: %v", clusterName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Deleted cluster: %s", clusterName)
	c.Status(http.StatusNoContent)
}

// SetDefaultCluster sets a cluster as the default
// @Summary Set default cluster
// @Tags Clusters
// @Param cluster_name path string true "Cluster Name"
// @Success 200 {object} map[string]string
// @Router /management/clusters/{cluster_name}/default [put]
func SetDefaultCluster(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name is required"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	if err := cpClientSet.Facade.ClusterConfig.SetDefaultCluster(c.Request.Context(), clusterName); err != nil {
		log.Errorf("Failed to set default cluster %s: %v", clusterName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Infof("Set default cluster: %s", clusterName)
	c.JSON(http.StatusOK, gin.H{"message": "Default cluster set successfully", "cluster_name": clusterName})
}

// TestClusterConnection tests the connection to a cluster
// @Summary Test cluster connection
// @Tags Clusters
// @Param cluster_name path string true "Cluster Name"
// @Success 200 {object} map[string]interface{}
// @Router /management/clusters/{cluster_name}/test [post]
func TestClusterConnection(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name is required"})
		return
	}

	cm := clientsets.GetClusterManager()
	if cm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cluster manager not initialized"})
		return
	}

	// Try to get the cluster's client set
	clientSet, err := cm.GetClientSetByClusterName(clusterName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cluster_name": clusterName,
			"k8s_connected": false,
			"storage_connected": false,
			"error": err.Error(),
		})
		return
	}

	result := gin.H{
		"cluster_name": clusterName,
		"k8s_connected": clientSet.K8SClientSet != nil,
		"storage_connected": clientSet.StorageClientSet != nil,
	}

	// Test K8S connection by getting server version
	if clientSet.K8SClientSet != nil && clientSet.K8SClientSet.Clientsets != nil {
		version, err := clientSet.K8SClientSet.Clientsets.Discovery().ServerVersion()
		if err != nil {
			result["k8s_error"] = err.Error()
			result["k8s_connected"] = false
		} else {
			result["k8s_version"] = version.String()
		}
	}

	c.JSON(http.StatusOK, result)
}
