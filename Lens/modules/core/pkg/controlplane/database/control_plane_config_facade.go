// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ControlPlaneConfigFacadeInterface defines the interface for control plane config operations
type ControlPlaneConfigFacadeInterface interface {
	// Get gets a config by key
	Get(ctx context.Context, key string) (*model.ControlPlaneConfig, error)
	// GetValue gets the value of a config by key
	GetValue(ctx context.Context, key string) (model.ConfigVal, error)
	// GetString gets the string value of a config by key and field
	GetString(ctx context.Context, key, field string) (string, error)
	// Set sets a config value
	Set(ctx context.Context, key string, value model.ConfigVal, opts ...SetConfigOption) error
	// Delete deletes a config
	Delete(ctx context.Context, key string) error
	// List lists all configs
	List(ctx context.Context) ([]*model.ControlPlaneConfig, error)
	// ListByCategory lists configs by category
	ListByCategory(ctx context.Context, category string) ([]*model.ControlPlaneConfig, error)
}

// SetConfigOption is an option for setting config
type SetConfigOption func(*setConfigOptions)

type setConfigOptions struct {
	description string
	category    string
	updatedBy   string
}

// WithConfigDescription sets the description
func WithConfigDescription(desc string) SetConfigOption {
	return func(o *setConfigOptions) {
		o.description = desc
	}
}

// WithConfigCategory sets the category
func WithConfigCategory(cat string) SetConfigOption {
	return func(o *setConfigOptions) {
		o.category = cat
	}
}

// WithConfigUpdatedBy sets the updatedBy
func WithConfigUpdatedBy(user string) SetConfigOption {
	return func(o *setConfigOptions) {
		o.updatedBy = user
	}
}

// ControlPlaneConfigFacade implements ControlPlaneConfigFacadeInterface
type ControlPlaneConfigFacade struct {
	db *gorm.DB
}

// NewControlPlaneConfigFacade creates a new ControlPlaneConfigFacade
func NewControlPlaneConfigFacade(db *gorm.DB) *ControlPlaneConfigFacade {
	return &ControlPlaneConfigFacade{db: db}
}

// Get gets a config by key
func (f *ControlPlaneConfigFacade) Get(ctx context.Context, key string) (*model.ControlPlaneConfig, error) {
	var config model.ControlPlaneConfig
	err := f.db.WithContext(ctx).Where("key = ?", key).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetValue gets the value of a config by key
func (f *ControlPlaneConfigFacade) GetValue(ctx context.Context, key string) (model.ConfigVal, error) {
	config, err := f.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return config.Value, nil
}

// GetString gets the string value of a config by key and field
func (f *ControlPlaneConfigFacade) GetString(ctx context.Context, key, field string) (string, error) {
	value, err := f.GetValue(ctx, key)
	if err != nil {
		return "", err
	}
	return value.GetString(field), nil
}

// Set sets a config value (upsert)
func (f *ControlPlaneConfigFacade) Set(ctx context.Context, key string, value model.ConfigVal, opts ...SetConfigOption) error {
	options := &setConfigOptions{}
	for _, opt := range opts {
		opt(options)
	}

	config := &model.ControlPlaneConfig{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
	}

	if options.description != "" {
		config.Description = options.description
	}
	if options.category != "" {
		config.Category = options.category
	}
	if options.updatedBy != "" {
		config.UpdatedBy = options.updatedBy
	}

	// Upsert: create or update
	return f.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"value", "description", "category", "updated_by", "updated_at", "version",
		}),
	}).Create(config).Error
}

// Delete deletes a config
func (f *ControlPlaneConfigFacade) Delete(ctx context.Context, key string) error {
	return f.db.WithContext(ctx).Where("key = ?", key).Delete(&model.ControlPlaneConfig{}).Error
}

// List lists all configs
func (f *ControlPlaneConfigFacade) List(ctx context.Context) ([]*model.ControlPlaneConfig, error) {
	var configs []*model.ControlPlaneConfig
	err := f.db.WithContext(ctx).Order("category, key").Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListByCategory lists configs by category
func (f *ControlPlaneConfigFacade) ListByCategory(ctx context.Context, category string) ([]*model.ControlPlaneConfig, error) {
	var configs []*model.ControlPlaneConfig
	err := f.db.WithContext(ctx).Where("category = ?", category).Order("key").Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}
