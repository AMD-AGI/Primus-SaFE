// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameControlPlaneConfig = "control_plane_config"

// ControlPlaneConfig stores control plane level configuration
type ControlPlaneConfig struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Key         string    `gorm:"column:key;not null;uniqueIndex" json:"key"`
	Value       ConfigVal `gorm:"column:value;not null" json:"value"`
	Description string    `gorm:"column:description" json:"description"`
	Category    string    `gorm:"column:category" json:"category"`
	Version     int32     `gorm:"column:version;default:1" json:"version"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	UpdatedBy   string    `gorm:"column:updated_by" json:"updated_by"`
}

// TableName returns the table name
func (*ControlPlaneConfig) TableName() string {
	return TableNameControlPlaneConfig
}

// ConfigVal is a custom type for JSONB config value
type ConfigVal map[string]interface{}

// Value implements driver.Valuer interface
func (v ConfigVal) Value() (driver.Value, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (v *ConfigVal) Scan(value interface{}) error {
	if value == nil {
		*v = make(ConfigVal)
		return nil
	}
	switch val := value.(type) {
	case []byte:
		return json.Unmarshal(val, v)
	case string:
		return json.Unmarshal([]byte(val), v)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

// GetString returns the value as string if it exists
func (v ConfigVal) GetString(key string) string {
	if val, ok := v[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// Config key constants
const (
	ConfigKeyInstallerImage = "installer.image"
	ConfigKeyInstallerTag   = "installer.tag"
	ConfigKeyDefaultRegistry = "default.registry"
)

// Config category constants
const (
	ConfigCategoryInstaller = "installer"
	ConfigCategoryRegistry  = "registry"
)
