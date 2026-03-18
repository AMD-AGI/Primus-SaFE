// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ClusterReleaseConfigFacadeInterface defines the interface for cluster release config operations
type ClusterReleaseConfigFacadeInterface interface {
	Create(ctx context.Context, config *model.ClusterReleaseConfig) error
	GetByClusterName(ctx context.Context, clusterName string) (*model.ClusterReleaseConfig, error)
	GetByClusterNameWithVersion(ctx context.Context, clusterName string) (*model.ClusterReleaseConfig, error)
	List(ctx context.Context) ([]*model.ClusterReleaseConfig, error)
	ListWithVersions(ctx context.Context) ([]*model.ClusterReleaseConfig, error)
	ListBySyncStatus(ctx context.Context, status string) ([]*model.ClusterReleaseConfig, error)
	ListOutOfSync(ctx context.Context) ([]*model.ClusterReleaseConfig, error)
	Update(ctx context.Context, config *model.ClusterReleaseConfig) error
	UpdateVersion(ctx context.Context, clusterName string, versionID int32, valuesOverride model.ValuesJSON) error
	UpdateSyncStatus(ctx context.Context, clusterName, status, errorMsg string) error
	MarkDeployed(ctx context.Context, clusterName string, versionID int32, deployedValues model.ValuesJSON) error
	Delete(ctx context.Context, clusterName string) error
}

// ClusterReleaseConfigFacade implements ClusterReleaseConfigFacadeInterface
type ClusterReleaseConfigFacade struct {
	db *gorm.DB
}

// NewClusterReleaseConfigFacade creates a new ClusterReleaseConfigFacade
func NewClusterReleaseConfigFacade(db *gorm.DB) *ClusterReleaseConfigFacade {
	return &ClusterReleaseConfigFacade{db: db}
}

// Create creates a new cluster release config
func (f *ClusterReleaseConfigFacade) Create(ctx context.Context, config *model.ClusterReleaseConfig) error {
	return f.db.WithContext(ctx).Create(config).Error
}

// GetByClusterName gets a cluster release config by cluster name
func (f *ClusterReleaseConfigFacade) GetByClusterName(ctx context.Context, clusterName string) (*model.ClusterReleaseConfig, error) {
	var config model.ClusterReleaseConfig
	err := f.db.WithContext(ctx).Where("cluster_name = ?", clusterName).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetByClusterNameWithVersion gets a cluster release config with version info
func (f *ClusterReleaseConfigFacade) GetByClusterNameWithVersion(ctx context.Context, clusterName string) (*model.ClusterReleaseConfig, error) {
	var config model.ClusterReleaseConfig
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Preload("DeployedVersion").
		Where("cluster_name = ?", clusterName).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// List lists all cluster release configs
func (f *ClusterReleaseConfigFacade) List(ctx context.Context) ([]*model.ClusterReleaseConfig, error) {
	var configs []*model.ClusterReleaseConfig
	err := f.db.WithContext(ctx).Order("cluster_name ASC").Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListWithVersions lists all configs with version info
func (f *ClusterReleaseConfigFacade) ListWithVersions(ctx context.Context) ([]*model.ClusterReleaseConfig, error) {
	var configs []*model.ClusterReleaseConfig
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Preload("DeployedVersion").
		Order("cluster_name ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListBySyncStatus lists configs by sync status
func (f *ClusterReleaseConfigFacade) ListBySyncStatus(ctx context.Context, status string) ([]*model.ClusterReleaseConfig, error) {
	var configs []*model.ClusterReleaseConfig
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Where("sync_status = ?", status).
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListOutOfSync lists clusters that need deployment
func (f *ClusterReleaseConfigFacade) ListOutOfSync(ctx context.Context) ([]*model.ClusterReleaseConfig, error) {
	var configs []*model.ClusterReleaseConfig
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Where("release_version_id IS NOT NULL AND (deployed_version_id IS NULL OR release_version_id != deployed_version_id)").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// Update updates a cluster release config
func (f *ClusterReleaseConfigFacade) Update(ctx context.Context, config *model.ClusterReleaseConfig) error {
	config.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(config).Error
}

// UpdateVersion updates the target version for a cluster
func (f *ClusterReleaseConfigFacade) UpdateVersion(ctx context.Context, clusterName string, versionID int32, valuesOverride model.ValuesJSON) error {
	updates := map[string]interface{}{
		"release_version_id": versionID,
		"sync_status":        model.SyncStatusOutOfSync,
		"updated_at":         time.Now(),
	}
	if valuesOverride != nil {
		updates["values_override"] = valuesOverride
	}
	return f.db.WithContext(ctx).
		Model(&model.ClusterReleaseConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(updates).Error
}

// UpdateSyncStatus updates the sync status
func (f *ClusterReleaseConfigFacade) UpdateSyncStatus(ctx context.Context, clusterName, status, errorMsg string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"sync_status":  status,
		"last_sync_at": now,
		"updated_at":   now,
	}
	if errorMsg != "" {
		updates["last_sync_error"] = errorMsg
	} else {
		updates["last_sync_error"] = ""
	}
	return f.db.WithContext(ctx).
		Model(&model.ClusterReleaseConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(updates).Error
}

// MarkDeployed marks a cluster as deployed with a specific version
func (f *ClusterReleaseConfigFacade) MarkDeployed(ctx context.Context, clusterName string, versionID int32, deployedValues model.ValuesJSON) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.ClusterReleaseConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(map[string]interface{}{
			"deployed_version_id": versionID,
			"deployed_values":     deployedValues,
			"deployed_at":         now,
			"sync_status":         model.SyncStatusSynced,
			"last_sync_at":        now,
			"last_sync_error":     "",
			"updated_at":          now,
		}).Error
}

// Delete deletes a cluster release config
func (f *ClusterReleaseConfigFacade) Delete(ctx context.Context, clusterName string) error {
	return f.db.WithContext(ctx).Where("cluster_name = ?", clusterName).Delete(&model.ClusterReleaseConfig{}).Error
}
