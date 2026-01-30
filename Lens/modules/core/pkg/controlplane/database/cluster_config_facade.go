// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ClusterConfigFacadeInterface defines the interface for cluster config operations
type ClusterConfigFacadeInterface interface {
	// Create creates a new cluster config
	Create(ctx context.Context, config *model.ClusterConfig) error
	// Update updates an existing cluster config
	Update(ctx context.Context, config *model.ClusterConfig) error
	// Delete soft deletes a cluster config
	Delete(ctx context.Context, clusterName string) error
	// GetByName gets a cluster config by name
	GetByName(ctx context.Context, clusterName string) (*model.ClusterConfig, error)
	// GetByPrimusSafeID gets a cluster config by primus-safe ID
	GetByPrimusSafeID(ctx context.Context, primusSafeID string) (*model.ClusterConfig, error)
	// List lists all active cluster configs
	List(ctx context.Context) ([]*model.ClusterConfig, error)
	// ListBySource lists cluster configs by source
	ListBySource(ctx context.Context, source string) ([]*model.ClusterConfig, error)
	// ListByDataplaneStatus lists cluster configs by dataplane status
	ListByDataplaneStatus(ctx context.Context, status string) ([]*model.ClusterConfig, error)
	// UpdateInfrastructureStatus updates the infrastructure status
	UpdateInfrastructureStatus(ctx context.Context, clusterName, status, message string) error
	// UpdateDataplaneStatus updates the dataplane status
	UpdateDataplaneStatus(ctx context.Context, clusterName, status, message string) error
	// UpdateStorageConfig updates the storage configuration
	UpdateStorageConfig(ctx context.Context, clusterName string, config *StorageConfigUpdate) error
	// Exists checks if a cluster config exists
	Exists(ctx context.Context, clusterName string) (bool, error)
	// GetDefaultCluster gets the default cluster config
	GetDefaultCluster(ctx context.Context) (*model.ClusterConfig, error)
	// SetDefaultCluster sets a cluster as the default (clears other defaults)
	SetDefaultCluster(ctx context.Context, clusterName string) error
}

// StorageConfigUpdate contains fields for updating storage configuration
type StorageConfigUpdate struct {
	PostgresHost     string
	PostgresPort     int
	PostgresUsername string
	PostgresPassword string
	PostgresDBName   string
	PostgresSSLMode  string

	OpensearchHost     string
	OpensearchPort     int
	OpensearchUsername string
	OpensearchPassword string
	OpensearchScheme   string

	PrometheusReadHost  string
	PrometheusReadPort  int
	PrometheusWriteHost string
	PrometheusWritePort int
}

// ClusterConfigFacade implements ClusterConfigFacadeInterface
type ClusterConfigFacade struct {
	db *gorm.DB
}

// NewClusterConfigFacade creates a new ClusterConfigFacade
func NewClusterConfigFacade(db *gorm.DB) *ClusterConfigFacade {
	return &ClusterConfigFacade{db: db}
}

// Create creates a new cluster config
func (f *ClusterConfigFacade) Create(ctx context.Context, config *model.ClusterConfig) error {
	return f.db.WithContext(ctx).Create(config).Error
}

// Update updates an existing cluster config
func (f *ClusterConfigFacade) Update(ctx context.Context, config *model.ClusterConfig) error {
	config.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(config).Error
}

// Delete soft deletes a cluster config
func (f *ClusterConfigFacade) Delete(ctx context.Context, clusterName string) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.ClusterConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(map[string]interface{}{
			"status":     model.ClusterStatusDeleted,
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

// GetByName gets a cluster config by name
func (f *ClusterConfigFacade) GetByName(ctx context.Context, clusterName string) (*model.ClusterConfig, error) {
	var config model.ClusterConfig
	err := f.db.WithContext(ctx).
		Where("cluster_name = ? AND status = ?", clusterName, model.ClusterStatusActive).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetByPrimusSafeID gets a cluster config by primus-safe ID
func (f *ClusterConfigFacade) GetByPrimusSafeID(ctx context.Context, primusSafeID string) (*model.ClusterConfig, error) {
	var config model.ClusterConfig
	err := f.db.WithContext(ctx).
		Where("primus_safe_id = ? AND status = ?", primusSafeID, model.ClusterStatusActive).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// List lists all active cluster configs
func (f *ClusterConfigFacade) List(ctx context.Context) ([]*model.ClusterConfig, error) {
	var configs []*model.ClusterConfig
	err := f.db.WithContext(ctx).
		Where("status = ?", model.ClusterStatusActive).
		Order("cluster_name ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListBySource lists cluster configs by source
func (f *ClusterConfigFacade) ListBySource(ctx context.Context, source string) ([]*model.ClusterConfig, error) {
	var configs []*model.ClusterConfig
	err := f.db.WithContext(ctx).
		Where("source = ? AND status = ?", source, model.ClusterStatusActive).
		Order("cluster_name ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListByDataplaneStatus lists cluster configs by dataplane status
func (f *ClusterConfigFacade) ListByDataplaneStatus(ctx context.Context, status string) ([]*model.ClusterConfig, error) {
	var configs []*model.ClusterConfig
	err := f.db.WithContext(ctx).
		Where("dataplane_status = ? AND status = ?", status, model.ClusterStatusActive).
		Order("cluster_name ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// UpdateInfrastructureStatus updates the infrastructure status
func (f *ClusterConfigFacade) UpdateInfrastructureStatus(ctx context.Context, clusterName, status, message string) error {
	updates := map[string]interface{}{
		"infrastructure_status":  status,
		"infrastructure_message": message,
		"updated_at":             time.Now(),
	}
	if status == model.InfrastructureStatusReady || status == model.InfrastructureStatusFailed {
		now := time.Now()
		updates["infrastructure_time"] = now
	}
	return f.db.WithContext(ctx).
		Model(&model.ClusterConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(updates).Error
}

// UpdateDataplaneStatus updates the dataplane status
func (f *ClusterConfigFacade) UpdateDataplaneStatus(ctx context.Context, clusterName, status, message string) error {
	updates := map[string]interface{}{
		"dataplane_status":  status,
		"dataplane_message": message,
		"updated_at":        time.Now(),
	}
	if status == model.DataplaneStatusDeployed || status == model.DataplaneStatusFailed {
		now := time.Now()
		updates["last_deploy_time"] = now
	}
	return f.db.WithContext(ctx).
		Model(&model.ClusterConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(updates).Error
}

// UpdateStorageConfig updates the storage configuration
func (f *ClusterConfigFacade) UpdateStorageConfig(ctx context.Context, clusterName string, config *StorageConfigUpdate) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if config.PostgresHost != "" {
		updates["postgres_host"] = config.PostgresHost
	}
	if config.PostgresPort != 0 {
		updates["postgres_port"] = config.PostgresPort
	}
	if config.PostgresUsername != "" {
		updates["postgres_username"] = config.PostgresUsername
	}
	if config.PostgresPassword != "" {
		updates["postgres_password"] = config.PostgresPassword
	}
	if config.PostgresDBName != "" {
		updates["postgres_db_name"] = config.PostgresDBName
	}
	if config.PostgresSSLMode != "" {
		updates["postgres_ssl_mode"] = config.PostgresSSLMode
	}

	if config.OpensearchHost != "" {
		updates["opensearch_host"] = config.OpensearchHost
	}
	if config.OpensearchPort != 0 {
		updates["opensearch_port"] = config.OpensearchPort
	}
	if config.OpensearchUsername != "" {
		updates["opensearch_username"] = config.OpensearchUsername
	}
	if config.OpensearchPassword != "" {
		updates["opensearch_password"] = config.OpensearchPassword
	}
	if config.OpensearchScheme != "" {
		updates["opensearch_scheme"] = config.OpensearchScheme
	}

	if config.PrometheusReadHost != "" {
		updates["prometheus_read_host"] = config.PrometheusReadHost
	}
	if config.PrometheusReadPort != 0 {
		updates["prometheus_read_port"] = config.PrometheusReadPort
	}
	if config.PrometheusWriteHost != "" {
		updates["prometheus_write_host"] = config.PrometheusWriteHost
	}
	if config.PrometheusWritePort != 0 {
		updates["prometheus_write_port"] = config.PrometheusWritePort
	}

	return f.db.WithContext(ctx).
		Model(&model.ClusterConfig{}).
		Where("cluster_name = ?", clusterName).
		Updates(updates).Error
}

// Exists checks if a cluster config exists
func (f *ClusterConfigFacade) Exists(ctx context.Context, clusterName string) (bool, error) {
	var count int64
	err := f.db.WithContext(ctx).
		Model(&model.ClusterConfig{}).
		Where("cluster_name = ? AND status = ?", clusterName, model.ClusterStatusActive).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetDefaultCluster gets the default cluster config
func (f *ClusterConfigFacade) GetDefaultCluster(ctx context.Context) (*model.ClusterConfig, error) {
	var config model.ClusterConfig
	err := f.db.WithContext(ctx).
		Where("is_default = ? AND status = ?", true, model.ClusterStatusActive).
		First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No default cluster configured
		}
		return nil, err
	}
	return &config, nil
}

// SetDefaultCluster sets a cluster as the default (clears other defaults)
func (f *ClusterConfigFacade) SetDefaultCluster(ctx context.Context, clusterName string) error {
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Clear existing default
		if err := tx.Model(&model.ClusterConfig{}).
			Where("is_default = ?", true).
			Update("is_default", false).Error; err != nil {
			return err
		}

		// Set new default
		result := tx.Model(&model.ClusterConfig{}).
			Where("cluster_name = ? AND status = ?", clusterName, model.ClusterStatusActive).
			Update("is_default", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
