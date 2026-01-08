// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// SystemConfigFacadeInterface defines the SystemConfig Facade interface
type SystemConfigFacadeInterface interface {
	// GetDB returns the underlying GORM database connection
	GetDB() *gorm.DB
	// GetByKey retrieves a configuration by key
	GetByKey(ctx context.Context, key string) (*model.SystemConfig, error)
	// List lists all configurations with optional filters
	List(ctx context.Context, query *gorm.DB) ([]*model.SystemConfig, error)
	// Create creates a new configuration
	Create(ctx context.Context, config *model.SystemConfig) error
	// Update updates an existing configuration
	Update(ctx context.Context, config *model.SystemConfig, updates map[string]interface{}) error
	// Delete deletes a configuration by key
	Delete(ctx context.Context, key string) error
	// BatchGet retrieves multiple configurations by keys
	BatchGet(ctx context.Context, keys []string) ([]*model.SystemConfig, error)
	// CreateHistory creates a history record
	CreateHistory(ctx context.Context, history *model.SystemConfigHistory) error
	// GetHistory retrieves history records for a key
	GetHistory(ctx context.Context, key string, limit int) ([]*model.SystemConfigHistory, error)
	// Exists checks if a configuration key exists
	Exists(ctx context.Context, key string) (bool, error)
	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) SystemConfigFacadeInterface
}

// SystemConfigFacade implements SystemConfigFacadeInterface
type SystemConfigFacade struct {
	BaseFacade
}

// NewSystemConfigFacade creates a new SystemConfig Facade
func NewSystemConfigFacade() *SystemConfigFacade {
	return &SystemConfigFacade{}
}

// GetDB returns the underlying GORM database connection
func (f *SystemConfigFacade) GetDB() *gorm.DB {
	return f.getDB()
}

// GetByKey retrieves a configuration by key
func (f *SystemConfigFacade) GetByKey(ctx context.Context, key string) (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := f.getDB().WithContext(ctx).Where("key = ?", key).First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	if config.ID == 0 {
		return nil, nil
	}
	return &config, nil
}

// List lists all configurations
func (f *SystemConfigFacade) List(ctx context.Context, query *gorm.DB) ([]*model.SystemConfig, error) {
	var configs []*model.SystemConfig
	err := query.WithContext(ctx).Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// Create creates a new configuration
func (f *SystemConfigFacade) Create(ctx context.Context, config *model.SystemConfig) error {
	return f.getDB().WithContext(ctx).Create(config).Error
}

// Update updates an existing configuration
func (f *SystemConfigFacade) Update(ctx context.Context, config *model.SystemConfig, updates map[string]interface{}) error {
	return f.getDB().WithContext(ctx).Model(config).Updates(updates).Error
}

// Delete deletes a configuration by key
func (f *SystemConfigFacade) Delete(ctx context.Context, key string) error {
	return f.getDB().WithContext(ctx).Where("key = ?", key).Delete(&model.SystemConfig{}).Error
}

// BatchGet retrieves multiple configurations by keys
func (f *SystemConfigFacade) BatchGet(ctx context.Context, keys []string) ([]*model.SystemConfig, error) {
	var configs []*model.SystemConfig
	err := f.getDB().WithContext(ctx).Where("key IN ?", keys).Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// CreateHistory creates a history record
func (f *SystemConfigFacade) CreateHistory(ctx context.Context, history *model.SystemConfigHistory) error {
	return f.getDB().WithContext(ctx).Create(history).Error
}

// GetHistory retrieves history records for a key
func (f *SystemConfigFacade) GetHistory(ctx context.Context, key string, limit int) ([]*model.SystemConfigHistory, error) {
	var history []*model.SystemConfigHistory
	query := f.getDB().WithContext(ctx).Where("key = ?", key).Order("changed_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&history).Error
	if err != nil {
		return nil, err
	}
	return history, nil
}

// Exists checks if a configuration key exists
func (f *SystemConfigFacade) Exists(ctx context.Context, key string) (bool, error) {
	var count int64
	err := f.getDB().WithContext(ctx).Model(&model.SystemConfig{}).Where("key = ?", key).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// WithCluster returns a new facade instance for the specified cluster
func (f *SystemConfigFacade) WithCluster(clusterName string) SystemConfigFacadeInterface {
	return &SystemConfigFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}
