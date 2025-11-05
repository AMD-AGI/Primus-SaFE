package config

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// Manager 提供系统配置的读写管理
type Manager struct {
	db *gorm.DB
}

// NewManager 创建一个新的配置管理器
func NewManager(db *gorm.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// NewManagerForCluster 根据集群名称创建配置管理器
func NewManagerForCluster(clusterName string) *Manager {
	facade := database.GetFacadeForCluster(clusterName)
	return &Manager{
		db: facade.GetSystemConfig().GetDB(),
	}
}

// Get 根据 key 获取配置并解析到指定的 struct 中
// key: 配置键
// dest: 目标 struct 指针，配置值将解析到此结构中
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

	// 将 ExtType (map[string]interface{}) 转换为目标结构
	return unmarshalExtType(config.Value, dest)
}

// GetRaw 根据 key 获取原始配置对象
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

// Set 设置配置值
// key: 配置键
// value: 配置值（将被序列化为 JSON）
// opts: 可选参数，如 description, category, updatedBy 等
func (m *Manager) Set(ctx context.Context, key string, value interface{}, opts ...SetOption) error {
	options := &setOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// 将 value 转换为 ExtType
	extValue, err := marshalToExtType(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// 查找现有配置
	var existing model.SystemConfig
	err = m.db.WithContext(ctx).
		Where("key = ?", key).
		First(&existing).Error

	now := time.Now()

	if err == gorm.ErrRecordNotFound {
		// 创建新配置
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

	// 检查是否为只读配置
	if existing.IsReadonly {
		return fmt.Errorf("config key '%s' is readonly and cannot be modified", key)
	}

	// 记录历史版本
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

	// 更新配置
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

// Delete 删除配置
func (m *Manager) Delete(ctx context.Context, key string) error {
	// 检查配置是否存在且是否为只读
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

// List 列出所有配置
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

// ListByCategory 根据类别列出配置
func (m *Manager) ListByCategory(ctx context.Context, category string) ([]model.SystemConfig, error) {
	return m.List(ctx, WithCategoryFilter(category))
}

// GetHistory 获取配置的历史记录
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

// Exists 检查配置键是否存在
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

// GetOrDefault 获取配置，如果不存在则返回默认值
func (m *Manager) GetOrDefault(ctx context.Context, key string, dest interface{}, defaultValue interface{}) error {
	err := m.Get(ctx, key, dest)
	if err != nil {
		if err == gorm.ErrRecordNotFound || (err != nil && err.Error() == fmt.Sprintf("config key '%s' not found", key)) {
			// 使用默认值
			return unmarshalExtType(defaultValue, dest)
		}
		return err
	}
	return nil
}

// Rollback 回滚到指定版本
func (m *Manager) Rollback(ctx context.Context, key string, version int32, updatedBy string) error {
	// 获取历史版本
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

	// 设置回滚值
	return m.Set(ctx, key, history.OldValue,
		WithUpdatedBy(updatedBy),
		WithChangeReason(fmt.Sprintf("Rollback to version %d", version)),
		WithRecordHistory(true),
	)
}

// Helper functions

// marshalToExtType 将任意类型转换为 ExtType
func marshalToExtType(value interface{}) (model.ExtType, error) {
	// 如果已经是 ExtType，直接返回
	if extType, ok := value.(model.ExtType); ok {
		return extType, nil
	}

	// 如果是 map[string]interface{}，直接转换
	if m, ok := value.(map[string]interface{}); ok {
		return model.ExtType(m), nil
	}

	// 其他类型通过 JSON 序列化转换
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

// unmarshalExtType 将 ExtType 或其他类型解析到目标 struct
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
