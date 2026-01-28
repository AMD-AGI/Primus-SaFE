// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package release

import (
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// getFacade returns the control plane facade
func getFacade() *cpdb.ControlPlaneFacade {
	cpClient := clientsets.GetControlPlaneClientSet()
	if cpClient == nil {
		return nil
	}
	return cpClient.Facade
}

// ===== Release Version Handlers =====

// ListReleaseVersions lists all release versions
func ListReleaseVersions(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	channel := c.Query("channel")
	status := c.Query("status")
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)

	versions, err := facade.GetReleaseVersion().List(c, channel, status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, versions))
}

// GetReleaseVersion gets a release version by ID
func GetReleaseVersion(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	version, err := facade.GetReleaseVersion().GetByID(c, int32(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, version))
}

// CreateReleaseVersionRequest is the request for creating a release version
type CreateReleaseVersionRequest struct {
	VersionName   string           `json:"version_name" binding:"required"`
	Channel       string           `json:"channel"`
	ChartRepo     string           `json:"chart_repo"`
	ChartVersion  string           `json:"chart_version" binding:"required"`
	ImageRegistry string           `json:"image_registry"`
	ImageTag      string           `json:"image_tag" binding:"required"`
	DefaultValues model.ValuesJSON `json:"default_values"`
	ValuesSchema  model.ValuesJSON `json:"values_schema"`
	ReleaseNotes  string           `json:"release_notes"`
}

// CreateReleaseVersion creates a new release version
func CreateReleaseVersion(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	var req CreateReleaseVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from context
	createdBy := "system"
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			createdBy = u
		}
	}

	version := &model.ReleaseVersion{
		VersionName:   req.VersionName,
		Channel:       req.Channel,
		ChartRepo:     req.ChartRepo,
		ChartVersion:  req.ChartVersion,
		ImageRegistry: req.ImageRegistry,
		ImageTag:      req.ImageTag,
		DefaultValues: req.DefaultValues,
		ValuesSchema:  req.ValuesSchema,
		ReleaseNotes:  req.ReleaseNotes,
		CreatedBy:     createdBy,
		Status:        model.ReleaseStatusDraft,
	}

	// Set defaults
	if version.Channel == "" {
		version.Channel = model.ChannelStable
	}
	if version.ChartRepo == "" {
		version.ChartRepo = "oci://docker.io/primussafe"
	}
	if version.ImageRegistry == "" {
		version.ImageRegistry = "docker.io/primussafe"
	}
	if version.DefaultValues == nil {
		version.DefaultValues = model.ValuesJSON{}
	}

	if err := facade.GetReleaseVersion().Create(c, version); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rest.SuccessResp(c, version))
}

// UpdateReleaseVersion updates a release version
func UpdateReleaseVersion(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	version, err := facade.GetReleaseVersion().GetByID(c, int32(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req CreateReleaseVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields
	version.VersionName = req.VersionName
	version.Channel = req.Channel
	version.ChartRepo = req.ChartRepo
	version.ChartVersion = req.ChartVersion
	version.ImageRegistry = req.ImageRegistry
	version.ImageTag = req.ImageTag
	version.DefaultValues = req.DefaultValues
	version.ValuesSchema = req.ValuesSchema
	version.ReleaseNotes = req.ReleaseNotes

	if err := facade.GetReleaseVersion().Update(c, version); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, version))
}

// UpdateReleaseVersionStatusRequest is the request for updating status
type UpdateReleaseVersionStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateReleaseVersionStatus updates the status of a release version
func UpdateReleaseVersionStatus(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateReleaseVersionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := facade.GetReleaseVersion().UpdateStatus(c, int32(id), req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"status": "updated"}))
}

// DeleteReleaseVersion deletes a release version
func DeleteReleaseVersion(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := facade.GetReleaseVersion().Delete(c, int32(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"status": "deleted"}))
}

// ===== Cluster Release Config Handlers =====

// ListClusterReleaseConfigs lists all cluster release configs
func ListClusterReleaseConfigs(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	configs, err := facade.GetClusterReleaseConfig().ListWithVersions(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, configs))
}

// GetClusterReleaseConfig gets a cluster release config
func GetClusterReleaseConfig(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	clusterName := c.Param("cluster_name")
	config, err := facade.GetClusterReleaseConfig().GetByClusterNameWithVersion(c, clusterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, config))
}

// UpdateClusterVersionRequest is the request for updating cluster version
type UpdateClusterVersionRequest struct {
	ReleaseVersionID int32            `json:"release_version_id" binding:"required"`
	ValuesOverride   model.ValuesJSON `json:"values_override"`
}

// UpdateClusterVersion updates the target version for a cluster
func UpdateClusterVersion(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	clusterName := c.Param("cluster_name")

	var req UpdateClusterVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if config exists, create if not
	_, err := facade.GetClusterReleaseConfig().GetByClusterName(c, clusterName)
	if err != nil {
		// Create new config
		config := &model.ClusterReleaseConfig{
			ClusterName:      clusterName,
			ReleaseVersionID: &req.ReleaseVersionID,
			ValuesOverride:   req.ValuesOverride,
			SyncStatus:       model.SyncStatusOutOfSync,
		}
		if err := facade.GetClusterReleaseConfig().Create(c, config); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		// Update existing config
		if err := facade.GetClusterReleaseConfig().UpdateVersion(c, clusterName, req.ReleaseVersionID, req.ValuesOverride); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"status": "updated"}))
}

// UpdateClusterValuesOverride updates only the values override for a cluster
func UpdateClusterValuesOverride(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	clusterName := c.Param("cluster_name")

	var valuesOverride model.ValuesJSON
	if err := c.ShouldBindJSON(&valuesOverride); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, err := facade.GetClusterReleaseConfig().GetByClusterName(c, clusterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cluster config not found"})
		return
	}

	config.ValuesOverride = valuesOverride
	config.SyncStatus = model.SyncStatusOutOfSync

	if err := facade.GetClusterReleaseConfig().Update(c, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, config))
}

// ===== Release History Handlers =====

// ListReleaseHistory lists release history for a cluster
func ListReleaseHistory(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	clusterName := c.Param("cluster_name")
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	histories, err := facade.GetReleaseHistory().ListByClusterWithVersions(c, clusterName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, histories))
}

// GetReleaseHistoryByID gets a release history by ID
func GetReleaseHistoryByID(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	history, err := facade.GetReleaseHistory().GetByID(c, int32(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, history))
}

// ===== Deployment Actions =====

// TriggerDeployRequest is the request for triggering a deployment
type TriggerDeployRequest struct {
	Action string `json:"action"` // install, upgrade, rollback, sync
}

// TriggerDeploy triggers a deployment for a cluster
func TriggerDeploy(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	clusterName := c.Param("cluster_name")

	var req TriggerDeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Action = model.ReleaseActionSync // Default to sync
	}

	// Get cluster release config
	config, err := facade.GetClusterReleaseConfig().GetByClusterNameWithVersion(c, clusterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cluster config not found"})
		return
	}

	if config.ReleaseVersionID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no version configured for cluster"})
		return
	}

	// Determine action
	action := req.Action
	if action == "" {
		if config.DeployedVersionID == nil {
			action = model.ReleaseActionInstall
		} else if *config.ReleaseVersionID != *config.DeployedVersionID {
			action = model.ReleaseActionUpgrade
		} else {
			action = model.ReleaseActionSync
		}
	}

	// Get user from context
	triggeredBy := "system"
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			triggeredBy = u
		}
	}

	// Merge values
	version := config.ReleaseVersion
	if version == nil {
		version, err = facade.GetReleaseVersion().GetByID(c, *config.ReleaseVersionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get version"})
			return
		}
	}
	mergedValues := model.MergeValues(version.DefaultValues, config.ValuesOverride)

	// Create release history
	history := &model.ReleaseHistory{
		ClusterName:      clusterName,
		ReleaseVersionID: *config.ReleaseVersionID,
		Action:           action,
		TriggeredBy:      triggeredBy,
		ValuesSnapshot:   mergedValues,
		Status:           model.ReleaseHistoryStatusPending,
	}
	if config.DeployedVersionID != nil {
		history.PreviousVersionID = config.DeployedVersionID
	}

	if err := facade.GetReleaseHistory().Create(c, history); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update cluster sync status
	if err := facade.GetClusterReleaseConfig().UpdateSyncStatus(c, clusterName, model.SyncStatusUpgrading, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create install task (will be picked up by the scheduler)
	task := &model.DataplaneInstallTask{
		ClusterName:  clusterName,
		TaskType:     action,
		CurrentStage: model.StagePending,
		StorageMode:  model.StorageModeExternal, // Will be determined from values
		Status:       model.TaskStatusPending,
	}

	// Build install config from merged values
	installConfig := model.InstallConfigJSON{
		Namespace:     getStringFromValues(mergedValues, "global", "namespace"),
		StorageClass:  getStringFromValues(mergedValues, "global", "storageClass"),
		ImageRegistry: version.ImageRegistry,
	}
	task.InstallConfig = installConfig

	if err := facade.GetDataplaneInstallTask().Create(c, task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Link task to history
	if err := facade.GetReleaseHistory().SetTaskID(c, history.ID, task.ID); err != nil {
		// Non-critical error, just log
	}

	c.JSON(http.StatusAccepted, rest.SuccessResp(c, gin.H{
		"message":    "deployment triggered",
		"history_id": history.ID,
		"task_id":    task.ID,
	}))
}

// TriggerRollback triggers a rollback for a cluster
func TriggerRollback(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	clusterName := c.Param("cluster_name")

	// Get the last successful deployment
	lastSuccess, err := facade.GetReleaseHistory().GetLatestSuccessfulByCluster(c, clusterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no successful deployment to rollback to"})
		return
	}

	// Get user from context
	triggeredBy := "system"
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			triggeredBy = u
		}
	}

	// Create rollback history
	history := &model.ReleaseHistory{
		ClusterName:      clusterName,
		ReleaseVersionID: lastSuccess.ReleaseVersionID,
		Action:           model.ReleaseActionRollback,
		TriggeredBy:      triggeredBy,
		ValuesSnapshot:   lastSuccess.ValuesSnapshot, // Use the exact values from last success
		Status:           model.ReleaseHistoryStatusPending,
	}

	// Get current deployed version as previous
	config, err := facade.GetClusterReleaseConfig().GetByClusterName(c, clusterName)
	if err == nil && config.DeployedVersionID != nil {
		history.PreviousVersionID = config.DeployedVersionID
	}

	if err := facade.GetReleaseHistory().Create(c, history); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update cluster to use the rollback version
	if err := facade.GetClusterReleaseConfig().UpdateVersion(c, clusterName, lastSuccess.ReleaseVersionID, lastSuccess.ValuesSnapshot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update sync status
	if err := facade.GetClusterReleaseConfig().UpdateSyncStatus(c, clusterName, model.SyncStatusUpgrading, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, rest.SuccessResp(c, gin.H{
		"message":            "rollback triggered",
		"history_id":         history.ID,
		"rollback_to_version": lastSuccess.ReleaseVersionID,
	}))
}

// ===== Default Cluster Handlers =====

// GetDefaultCluster gets the current default cluster
func GetDefaultCluster(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	cluster, err := facade.ClusterConfig.GetDefaultCluster(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if cluster == nil {
		c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"default_cluster": nil}))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"default_cluster": cluster.ClusterName,
		"cluster_id":      cluster.ID,
	}))
}

// SetDefaultClusterRequest is the request for setting default cluster
type SetDefaultClusterRequest struct {
	ClusterName string `json:"cluster_name" binding:"required"`
}

// SetDefaultCluster sets a cluster as the default
func SetDefaultCluster(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	var req SetDefaultClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := facade.ClusterConfig.SetDefaultCluster(c, req.ClusterName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"message":         "default cluster set",
		"default_cluster": req.ClusterName,
	}))
}

// ClearDefaultCluster clears the default cluster setting
func ClearDefaultCluster(c *gin.Context) {
	facade := getFacade()
	if facade == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control plane not available"})
		return
	}

	// Clear by setting is_default = false for all
	if err := facade.ClusterConfig.SetDefaultCluster(c, ""); err != nil {
		// If no cluster found, that's fine - it means no default was set
		if err.Error() == "record not found" {
			c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "no default cluster to clear"}))
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "default cluster cleared"}))
}

// Helper function to extract string from nested values
func getStringFromValues(values model.ValuesJSON, keys ...string) string {
	current := interface{}(map[string]interface{}(values))
	for _, key := range keys {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[key]
		} else {
			return ""
		}
	}
	if s, ok := current.(string); ok {
		return s
	}
	return ""
}
