package config

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// Manager provides read/write management for system configurations
type Manager struct {
	db *gorm.DB
}

// NewManager creates a new configuration manager
func NewManager(db *gorm.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// NewManagerForCluster creates a configuration manager for a specific cluster
func NewManagerForCluster(clusterName string) *Manager {
	facade := database.GetFacadeForCluster(clusterName)
	return &Manager{
		db: facade.GetSystemConfig().GetDB(),
	}
}

// Get retrieves configuration by key and parses it into the specified struct
// key: configuration key
// dest: destination struct pointer where the configuration value will be parsed into
func (m *Manager) Get(ctx context.Context, key string, dest interface{}) error {
	if dest == nil {
		return fmt.Errorf("destination pointer cannot be nil")
	}

	var config model.SystemConfig
	err := m.db.WithContext(ctx).
		Where("key = ?", key).
		First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("config key '%s' not found", key)
		}
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Convert ExtType (map[string]interface{}) to target structure
	return unmarshalExtType(config.Value, dest)
}

// GetRaw retrieves the raw configuration object by key
func (m *Manager) GetRaw(ctx context.Context, key string) (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := m.db.WithContext(ctx).
		Where("key = ?", key).
		First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("config key '%s' not found", key)
		}
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	return &config, nil
}

// Set sets a configuration value
// key: configuration key
// value: configuration value (will be serialized to JSON)
// opts: optional parameters such as description, category, updatedBy, etc.
func (m *Manager) Set(ctx context.Context, key string, value interface{}, opts ...SetOption) error {
	options := &setOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Convert value to ExtType
	extValue, err := marshalToExtType(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Look for existing configuration
	var existing model.SystemConfig
	err = m.db.WithContext(ctx).
		Where("key = ?", key).
		First(&existing).Error

	now := time.Now()

	if err == gorm.ErrRecordNotFound {
		// Create new configuration
		config := model.SystemConfig{
			Key:         key,
			Value:       extValue,
			Description: options.description,
			Category:    options.category,
			IsEncrypted: options.isEncrypted,
			Version:     1,
			IsReadonly:  options.isReadonly,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   options.createdBy,
			UpdatedBy:   options.updatedBy,
		}
		return m.db.WithContext(ctx).Create(&config).Error
	} else if err != nil {
		return fmt.Errorf("failed to check existing config: %w", err)
	}

	// Check if configuration is readonly
	if existing.IsReadonly {
		return fmt.Errorf("config key '%s' is readonly and cannot be modified", key)
	}

	// Record history version
	if options.recordHistory {
		history := model.SystemConfigHistory{
			ConfigID:     existing.ID,
			Key:          key,
			OldValue:     existing.Value,
			NewValue:     extValue,
			Version:      existing.Version,
			ChangeReason: options.changeReason,
			ChangedAt:    now,
			ChangedBy:    options.updatedBy,
		}
		if err := m.db.WithContext(ctx).Create(&history).Error; err != nil {
			return fmt.Errorf("failed to create history record: %w", err)
		}
	}

	// Update configuration
	updates := map[string]interface{}{
		"value":      extValue,
		"version":    existing.Version + 1,
		"updated_at": now,
	}

	if options.description != "" {
		updates["description"] = options.description
	}
	if options.category != "" {
		updates["category"] = options.category
	}
	if options.updatedBy != "" {
		updates["updated_by"] = options.updatedBy
	}

	return m.db.WithContext(ctx).
		Model(&model.SystemConfig{}).
		Where("key = ?", key).
		Updates(updates).Error
}

// Delete deletes a configuration
func (m *Manager) Delete(ctx context.Context, key string) error {
	// Check if configuration exists and is readonly
	var config model.SystemConfig
	err := m.db.WithContext(ctx).
		Where("key = ?", key).
		First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("config key '%s' not found", key)
		}
		return fmt.Errorf("failed to get config: %w", err)
	}

	if config.IsReadonly {
		return fmt.Errorf("config key '%s' is readonly and cannot be deleted", key)
	}

	return m.db.WithContext(ctx).
		Where("key = ?", key).
		Delete(&model.SystemConfig{}).Error
}

// List lists all configurations
func (m *Manager) List(ctx context.Context, filters ...ListFilter) ([]model.SystemConfig, error) {
	query := m.db.WithContext(ctx).Model(&model.SystemConfig{})

	for _, filter := range filters {
		query = filter(query)
	}

	var configs []model.SystemConfig
	err := query.Find(&configs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	return configs, nil
}

// ListByCategory lists configurations by category
func (m *Manager) ListByCategory(ctx context.Context, category string) ([]model.SystemConfig, error) {
	return m.List(ctx, WithCategoryFilter(category))
}

// GetHistory retrieves the history records of a configuration
func (m *Manager) GetHistory(ctx context.Context, key string, limit int) ([]model.SystemConfigHistory, error) {
	var history []model.SystemConfigHistory
	query := m.db.WithContext(ctx).
		Where("key = ?", key).
		Order("changed_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&history).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get config history: %w", err)
	}

	return history, nil
}

// Exists checks if a configuration key exists
func (m *Manager) Exists(ctx context.Context, key string) (bool, error) {
	var count int64
	err := m.db.WithContext(ctx).
		Model(&model.SystemConfig{}).
		Where("key = ?", key).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check config existence: %w", err)
	}
	return count > 0, nil
}

// GetOrDefault retrieves configuration, or returns default value if not found
func (m *Manager) GetOrDefault(ctx context.Context, key string, dest interface{}, defaultValue interface{}) error {
	err := m.Get(ctx, key, dest)
	if err != nil {
		if err == gorm.ErrRecordNotFound || (err != nil && err.Error() == fmt.Sprintf("config key '%s' not found", key)) {
			// Use default value
			return unmarshalExtType(defaultValue, dest)
		}
		return err
	}
	return nil
}

// Rollback rolls back to a specified version
func (m *Manager) Rollback(ctx context.Context, key string, version int32, updatedBy string) error {
	// Retrieve history version
	var history model.SystemConfigHistory
	err := m.db.WithContext(ctx).
		Where("key = ? AND version = ?", key, version).
		First(&history).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("history version %d not found for key '%s'", version, key)
		}
		return fmt.Errorf("failed to get history: %w", err)
	}

	// Set rollback value
	return m.Set(ctx, key, history.OldValue,
		WithUpdatedBy(updatedBy),
		WithChangeReason(fmt.Sprintf("Rollback to version %d", version)),
		WithRecordHistory(true),
	)
}

// Helper functions

// marshalToExtType converts any type to ExtType
func marshalToExtType(value interface{}) (model.ExtType, error) {
	// If already ExtType, return directly
	if extType, ok := value.(model.ExtType); ok {
		return extType, nil
	}

	// If map[string]interface{}, convert directly
	if m, ok := value.(map[string]interface{}); ok {
		return model.ExtType(m), nil
	}

	// Convert other types through JSON serialization
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var extType model.ExtType
	err = json.Unmarshal(jsonBytes, &extType)
	if err != nil {
		return nil, err
	}

	return extType, nil
}

// unmarshalExtType parses ExtType or other types to target struct
func unmarshalExtType(value interface{}, dest interface{}) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	err = json.Unmarshal(jsonBytes, dest)
	if err != nil {
		return fmt.Errorf("failed to unmarshal to destination: %w", err)
	}

	return nil
}
