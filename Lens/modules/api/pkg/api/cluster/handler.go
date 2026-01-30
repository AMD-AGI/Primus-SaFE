// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package cluster

import (
	"io"
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	OpensearchScheme     string `json:"opensearch_scheme"`
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
		OpensearchScheme:      c.OpensearchScheme,
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
			"cluster_name":      clusterName,
			"k8s_connected":     false,
			"storage_connected": false,
			"error":             err.Error(),
		})
		return
	}

	result := gin.H{
		"cluster_name":      clusterName,
		"k8s_connected":     clientSet.K8SClientSet != nil,
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

// ============================================================================
// Infrastructure Initialization APIs
// ============================================================================

// InitializeInfrastructureRequest represents the request to initialize infrastructure
type InitializeInfrastructureRequest struct {
	StorageMode   string                    `json:"storage_mode"`    // external or lens-managed
	StorageClass  string                    `json:"storage_class"`   // for lens-managed mode
	ManagedStorage *model.ManagedStorageJSON `json:"managed_storage"` // for lens-managed mode
}

// InitializeInfrastructure initializes cluster infrastructure
// @Summary Initialize cluster infrastructure
// @Tags Clusters
// @Accept json
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Param request body InitializeInfrastructureRequest true "Initialize Request"
// @Success 200 {object} map[string]interface{}
// @Router /management/clusters/{cluster_name}/initialize [post]
func InitializeInfrastructure(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name is required"})
		return
	}

	var req InitializeInfrastructureRequest
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

	// Get cluster config
	cluster, err := cpClientSet.Facade.ClusterConfig.GetByName(ctx, clusterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return
	}

	// Check if already initialized or initializing
	if cluster.InfrastructureStatus == model.InfrastructureStatusReady {
		c.JSON(http.StatusConflict, gin.H{"error": "Infrastructure already initialized"})
		return
	}
	if cluster.InfrastructureStatus == model.InfrastructureStatusInitializing {
		c.JSON(http.StatusConflict, gin.H{"error": "Infrastructure initialization already in progress"})
		return
	}

	// Check for active infrastructure task
	activeTask, _ := cpClientSet.Facade.GetDataplaneInstallTask().GetActiveTaskByScope(ctx, clusterName, model.InstallScopeInfrastructure)
	if activeTask != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "Infrastructure initialization task already exists",
			"task_id": activeTask.ID,
		})
		return
	}

	// Determine storage mode
	storageMode := req.StorageMode
	if storageMode == "" {
		storageMode = cluster.StorageMode
	}
	if storageMode == "" {
		storageMode = model.StorageModeExternal
	}

	// Create install config
	installConfig := model.InstallConfigJSON{
		Namespace:    "primus-lens",
		StorageClass: req.StorageClass,
	}
	if req.ManagedStorage != nil {
		installConfig.ManagedStorage = &model.ManagedStorageConfig{
			StorageClass:           req.ManagedStorage.StorageClass,
			PostgresEnabled:        req.ManagedStorage.PostgresEnabled,
			PostgresSize:           req.ManagedStorage.PostgresSize,
			OpensearchEnabled:      req.ManagedStorage.OpensearchEnabled,
			OpensearchSize:         req.ManagedStorage.OpensearchSize,
			OpensearchReplicas:     req.ManagedStorage.OpensearchReplicas,
			VictoriametricsEnabled: req.ManagedStorage.VictoriametricsEnabled,
			VictoriametricsSize:    req.ManagedStorage.VictoriametricsSize,
		}
	}

	// Create install task with infrastructure scope
	task := &model.DataplaneInstallTask{
		ClusterName:   clusterName,
		TaskType:      model.TaskTypeInstall,
		InstallScope:  model.InstallScopeInfrastructure,
		CurrentStage:  model.StagePending,
		StorageMode:   storageMode,
		InstallConfig: installConfig,
		Status:        model.TaskStatusPending,
		MaxRetries:    3,
	}

	if err := cpClientSet.Facade.GetDataplaneInstallTask().Create(ctx, task); err != nil {
		log.Errorf("Failed to create infrastructure install task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update cluster infrastructure status
	if err := cpClientSet.Facade.ClusterConfig.UpdateInfrastructureStatus(ctx, clusterName, model.InfrastructureStatusInitializing, ""); err != nil {
		log.Warnf("Failed to update infrastructure status: %v", err)
	}

	log.Infof("Created infrastructure initialization task %d for cluster %s", task.ID, clusterName)
	c.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"status":  "pending",
		"message": "Infrastructure initialization task created",
	})
}

// GetInfrastructureStatus gets the infrastructure status for a cluster
// @Summary Get infrastructure status
// @Tags Clusters
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Success 200 {object} map[string]interface{}
// @Router /management/clusters/{cluster_name}/infrastructure/status [get]
func GetInfrastructureStatus(c *gin.Context) {
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

	ctx := c.Request.Context()

	// Get cluster config
	cluster, err := cpClientSet.Facade.ClusterConfig.GetByName(ctx, clusterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return
	}

	response := gin.H{
		"cluster_name":  clusterName,
		"initialized":   cluster.InfrastructureStatus == model.InfrastructureStatusReady,
		"storage_mode":  cluster.StorageMode,
		"status":        cluster.InfrastructureStatus,
		"message":       cluster.InfrastructureMessage,
	}

	if cluster.InfrastructureTime != nil {
		response["infrastructure_time"] = cluster.InfrastructureTime.Format("2006-01-02T15:04:05Z07:00")
	}

	// Get latest infrastructure task
	latestTask, err := cpClientSet.Facade.GetDataplaneInstallTask().GetLatestByScope(ctx, clusterName, model.InstallScopeInfrastructure)
	if err == nil && latestTask != nil {
		response["last_task"] = gin.H{
			"id":            latestTask.ID,
			"scope":         latestTask.InstallScope,
			"status":        latestTask.Status,
			"current_stage": latestTask.CurrentStage,
			"error_message": latestTask.ErrorMessage,
			"created_at":    latestTask.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if latestTask.CompletedAt != nil {
			response["last_task"].(gin.H)["completed_at"] = latestTask.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		}
	}

	c.JSON(http.StatusOK, response)
}

// ============================================================================
// Task Management APIs
// ============================================================================

// TaskResponse represents a task in API response
type TaskResponse struct {
	ID            int32  `json:"id"`
	ClusterName   string `json:"cluster_name"`
	TaskType      string `json:"task_type"`
	InstallScope  string `json:"install_scope"`
	CurrentStage  string `json:"current_stage"`
	StorageMode   string `json:"storage_mode"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message"`
	RetryCount    int    `json:"retry_count"`
	MaxRetries    int    `json:"max_retries"`
	JobName       string `json:"job_name"`
	JobNamespace  string `json:"job_namespace"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	StartedAt     string `json:"started_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
}

func taskToResponse(task *model.DataplaneInstallTask) *TaskResponse {
	resp := &TaskResponse{
		ID:           task.ID,
		ClusterName:  task.ClusterName,
		TaskType:     task.TaskType,
		InstallScope: task.InstallScope,
		CurrentStage: task.CurrentStage,
		StorageMode:  task.StorageMode,
		Status:       task.Status,
		ErrorMessage: task.ErrorMessage,
		RetryCount:   task.RetryCount,
		MaxRetries:   task.MaxRetries,
		JobName:      task.JobName,
		JobNamespace: task.JobNamespace,
		CreatedAt:    task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    task.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if task.StartedAt != nil {
		resp.StartedAt = task.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if task.CompletedAt != nil {
		resp.CompletedAt = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

// ListTasks lists installation tasks for a cluster
// @Summary List installation tasks
// @Tags Clusters
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Param scope query string false "Filter by scope (infrastructure, apps, full)"
// @Param status query string false "Filter by status (pending, running, completed, failed)"
// @Param limit query int false "Max results (default: 20)"
// @Success 200 {object} map[string]interface{}
// @Router /management/clusters/{cluster_name}/tasks [get]
func ListTasks(c *gin.Context) {
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

	ctx := c.Request.Context()
	scope := c.Query("scope")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var tasks []*model.DataplaneInstallTask
	var err error

	if scope != "" {
		tasks, err = cpClientSet.Facade.GetDataplaneInstallTask().ListByClusterAndScope(ctx, clusterName, scope, limit)
	} else {
		tasks, err = cpClientSet.Facade.GetDataplaneInstallTask().ListByCluster(ctx, clusterName, limit)
	}

	if err != nil {
		log.Errorf("Failed to list tasks: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]*TaskResponse, len(tasks))
	for i, task := range tasks {
		response[i] = taskToResponse(task)
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// GetTask gets a specific installation task
// @Summary Get installation task
// @Tags Clusters
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Param task_id path int true "Task ID"
// @Success 200 {object} TaskResponse
// @Router /management/clusters/{cluster_name}/tasks/{task_id} [get]
func GetTask(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	taskIDStr := c.Param("task_id")

	if clusterName == "" || taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name and task_id are required"})
		return
	}

	taskID, err := parseInt(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task_id"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()
	task, err := cpClientSet.Facade.GetDataplaneInstallTask().GetByID(ctx, int32(taskID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	if task.ClusterName != clusterName {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found for this cluster"})
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// GetActiveTask gets the active installation task for a cluster
// @Summary Get active installation task
// @Tags Clusters
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Param scope query string false "Filter by scope"
// @Success 200 {object} TaskResponse
// @Router /management/clusters/{cluster_name}/tasks/active [get]
func GetActiveTask(c *gin.Context) {
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

	ctx := c.Request.Context()
	scope := c.Query("scope")

	var task *model.DataplaneInstallTask
	var err error

	if scope != "" {
		task, err = cpClientSet.Facade.GetDataplaneInstallTask().GetActiveTaskByScope(ctx, clusterName, scope)
	} else {
		task, err = cpClientSet.Facade.GetDataplaneInstallTask().GetActiveTask(ctx, clusterName)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active task found"})
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// GetTaskLogs gets the logs for an installation task
// @Summary Get task logs
// @Tags Clusters
// @Produce json
// @Param cluster_name path string true "Cluster Name"
// @Param task_id path int true "Task ID"
// @Param tail query int false "Number of lines from end (default: 500)"
// @Success 200 {object} map[string]interface{}
// @Router /management/clusters/{cluster_name}/tasks/{task_id}/logs [get]
func GetTaskLogs(c *gin.Context) {
	clusterName := c.Param("cluster_name")
	taskIDStr := c.Param("task_id")

	if clusterName == "" || taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_name and task_id are required"})
		return
	}

	taskID, err := parseInt(taskIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task_id"})
		return
	}

	cpClientSet := clientsets.GetControlPlaneClientSet()
	if cpClientSet == nil || cpClientSet.Facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Control plane not initialized"})
		return
	}

	ctx := c.Request.Context()

	// Get task
	task, err := cpClientSet.Facade.GetDataplaneInstallTask().GetByID(ctx, int32(taskID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	if task.ClusterName != clusterName {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found for this cluster"})
		return
	}

	// Check if job exists
	if task.JobName == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "No job associated with task",
			"task_id": task.ID,
			"status":  task.Status,
		})
		return
	}

	// Get K8S client for control plane cluster
	cm := clientsets.GetClusterManager()
	if cm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cluster manager not available"})
		return
	}
	currentCluster := cm.GetCurrentClusterClients()
	if currentCluster == nil || currentCluster.K8SClientSet == nil || currentCluster.K8SClientSet.Clientsets == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "K8S client not available"})
		return
	}
	k8sClient := currentCluster.K8SClientSet

	// Get tail lines
	tailLines := int64(500)
	if t := c.Query("tail"); t != "" {
		if parsed, err := parseInt(t); err == nil && parsed > 0 {
			tailLines = int64(parsed)
		}
	}

	// Get pods for this job
	pods, err := k8sClient.Clientsets.CoreV1().Pods(task.JobNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + task.JobName,
	})
	if err != nil || len(pods.Items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "No pods found for job",
			"job_name": task.JobName,
		})
		return
	}

	// Get logs from the first pod
	pod := pods.Items[0]
	logOptions := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}

	req := k8sClient.Clientsets.CoreV1().Pods(task.JobNamespace).GetLogs(pod.Name, logOptions)
	logStream, err := req.Stream(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":    "Failed to get logs",
			"job_name": task.JobName,
			"pod_name": pod.Name,
		})
		return
	}
	defer logStream.Close()

	// Read logs
	buf, err := io.ReadAll(logStream)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read logs"})
		return
	}
	logs := string(buf)

	c.JSON(http.StatusOK, gin.H{
		"task_id":   task.ID,
		"job_name":  task.JobName,
		"pod_name":  pod.Name,
		"logs":      logs,
		"log_lines": len(logs),
	})
}

// parseInt parses a string to int
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
