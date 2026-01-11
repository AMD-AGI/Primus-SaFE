// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// BatchConfig represents a batch configuration item
type BatchConfig struct {
	Key         string
	Value       interface{}
	Description string
	Category    string
	IsEncrypted bool
	IsReadonly  bool
	CreatedBy   string
}

// BatchSet sets configurations in batch
func (m *Manager) BatchSet(ctx context.Context, configs []BatchConfig) error {
	for _, cfg := range configs {
		opts := []SetOption{
			WithDescription(cfg.Description),
			WithCategory(cfg.Category),
			WithEncrypted(cfg.IsEncrypted),
			WithReadonly(cfg.IsReadonly),
			WithCreatedBy(cfg.CreatedBy),
		}

		err := m.Set(ctx, cfg.Key, cfg.Value, opts...)
		if err != nil {
			return fmt.Errorf("failed to set config '%s': %w", cfg.Key, err)
		}
	}
	return nil
}

// BatchGet retrieves configurations in batch
// keys: list of configuration keys
// Returns: map[key]SystemConfig
func (m *Manager) BatchGet(ctx context.Context, keys []string) (map[string]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := m.db.WithContext(ctx).
		Where("key IN ?", keys).
		Find(&configs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to batch get configs: %w", err)
	}

	result := make(map[string]model.SystemConfig, len(configs))
	for _, cfg := range configs {
		result[cfg.Key] = cfg
	}

	return result, nil
}

// BatchGetParsed retrieves and parses configurations in batch
// configMap: map[key]destPointer, parses configuration values into corresponding pointers
func (m *Manager) BatchGetParsed(ctx context.Context, configMap map[string]interface{}) error {
	keys := make([]string, 0, len(configMap))
	for key := range configMap {
		keys = append(keys, key)
	}

	configs, err := m.BatchGet(ctx, keys)
	if err != nil {
		return err
	}

	for key, dest := range configMap {
		cfg, exists := configs[key]
		if !exists {
			return fmt.Errorf("config key '%s' not found", key)
		}

		if err := unmarshalExtType(cfg.Value, dest); err != nil {
			return fmt.Errorf("failed to unmarshal config '%s': %w", key, err)
		}
	}

	return nil
}

// BatchDelete deletes configurations in batch
func (m *Manager) BatchDelete(ctx context.Context, keys []string) error {
	// Check if there are readonly configurations
	var readonlyConfigs []string
	err := m.db.WithContext(ctx).
		Model(&model.SystemConfig{}).
		Where("key IN ? AND is_readonly = ?", keys, true).
		Pluck("key", &readonlyConfigs).Error
	if err != nil {
		return fmt.Errorf("failed to check readonly configs: %w", err)
	}

	if len(readonlyConfigs) > 0 {
		return fmt.Errorf("cannot delete readonly configs: %v", readonlyConfigs)
	}

	return m.db.WithContext(ctx).
		Where("key IN ?", keys).
		Delete(&model.SystemConfig{}).Error
}

// CopyConfig copies configuration to a new key
func (m *Manager) CopyConfig(ctx context.Context, sourceKey, targetKey string, updatedBy string) error {
	// Get source configuration
	source, err := m.GetRaw(ctx, sourceKey)
	if err != nil {
		return fmt.Errorf("failed to get source config: %w", err)
	}

	// Check if target key already exists
	exists, err := m.Exists(ctx, targetKey)
	if err != nil {
		return fmt.Errorf("failed to check target key existence: %w", err)
	}
	if exists {
		return fmt.Errorf("target key '%s' already exists", targetKey)
	}

	// Create new configuration
	return m.Set(ctx, targetKey, source.Value,
		WithDescription(source.Description),
		WithCategory(source.Category),
		WithEncrypted(source.IsEncrypted),
		WithReadonly(false), // Copied configuration is not readonly by default
		WithCreatedBy(updatedBy),
		WithUpdatedBy(updatedBy),
	)
}
