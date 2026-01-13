// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// SystemConfigFacadeInterface defines SystemConfig database operations
type SystemConfigFacadeInterface interface {
	Get(ctx context.Context, key string) (*model.LensSystemConfigs, error)
	Set(ctx context.Context, config *model.LensSystemConfigs) error
	Delete(ctx context.Context, key string) error
	ListByCategory(ctx context.Context, category string) ([]*model.LensSystemConfigs, error)
	ListAll(ctx context.Context) ([]*model.LensSystemConfigs, error)
	GetValue(ctx context.Context, key string) (string, error)
	SetValue(ctx context.Context, key string, value string, category string) error
}

// SystemConfigFacade implements SystemConfigFacadeInterface
type SystemConfigFacade struct {
	BaseFacade
}

// NewSystemConfigFacade creates a new SystemConfigFacade
func NewSystemConfigFacade() *SystemConfigFacade {
	return &SystemConfigFacade{}
}

// Get gets a system config by key
func (f *SystemConfigFacade) Get(ctx context.Context, key string) (*model.LensSystemConfigs, error) {
	var config model.LensSystemConfigs
	err := f.getDB().WithContext(ctx).Where("key = ?", key).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// Set creates or updates a system config
func (f *SystemConfigFacade) Set(ctx context.Context, config *model.LensSystemConfigs) error {
	config.UpdatedAt = time.Now()

	// Use upsert: insert on conflict update
	return f.getDB().WithContext(ctx).
		Where("key = ?", config.Key).
		Assign(map[string]interface{}{
			"value":       config.Value,
			"description": config.Description,
			"category":    config.Category,
			"is_secret":   config.IsSecret,
			"updated_at":  config.UpdatedAt,
		}).
		FirstOrCreate(config).Error
}

// Delete deletes a system config by key
func (f *SystemConfigFacade) Delete(ctx context.Context, key string) error {
	return f.getDB().WithContext(ctx).Where("key = ?", key).Delete(&model.LensSystemConfigs{}).Error
}

// ListByCategory lists system configs by category
func (f *SystemConfigFacade) ListByCategory(ctx context.Context, category string) ([]*model.LensSystemConfigs, error) {
	var configs []*model.LensSystemConfigs
	err := f.getDB().WithContext(ctx).
		Where("category = ?", category).
		Order("key ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ListAll lists all system configs
func (f *SystemConfigFacade) ListAll(ctx context.Context) ([]*model.LensSystemConfigs, error) {
	var configs []*model.LensSystemConfigs
	err := f.getDB().WithContext(ctx).
		Order("category ASC, key ASC").
		Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// GetValue gets a system config value by key (returns JSON string)
func (f *SystemConfigFacade) GetValue(ctx context.Context, key string) (string, error) {
	config, err := f.Get(ctx, key)
	if err != nil {
		return "", err
	}
	// Value is stored as ExtType (map[string]interface{})
	if v, ok := config.Value["value"]; ok {
		if str, ok := v.(string); ok {
			return str, nil
		}
	}
	return "", nil
}

// SetValue sets a system config value by key
func (f *SystemConfigFacade) SetValue(ctx context.Context, key string, value string, category string) error {
	now := time.Now()
	valueMap := model.ExtType{"value": value}
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     valueMap,
		Category:  category,
		UpdatedAt: now,
	}

	return f.getDB().WithContext(ctx).
		Where("key = ?", key).
		Assign(map[string]interface{}{
			"value":      valueMap,
			"category":   category,
			"updated_at": now,
		}).
		FirstOrCreate(config).Error
}

// Ensure SystemConfigFacade implements SystemConfigFacadeInterface
var _ SystemConfigFacadeInterface = (*SystemConfigFacade)(nil)
