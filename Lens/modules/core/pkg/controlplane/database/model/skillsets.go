// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameSkillsets = "skillsets"

// Skillset represents a skill set for agent execution context
type Skillset struct {
	ID          int64            `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name        string           `gorm:"column:name;not null;uniqueIndex" json:"name"`
	Description string           `gorm:"column:description" json:"description"`
	Owner       string           `gorm:"column:owner" json:"owner"`
	IsDefault   bool             `gorm:"column:is_default;not null;default:false" json:"is_default"`
	Metadata    SkillsetMetadata `gorm:"column:metadata;default:{}" json:"metadata"`
	CreatedAt   time.Time        `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time        `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*Skillset) TableName() string {
	return TableNameSkillsets
}

// SkillsetMetadata is a custom type for JSONB metadata field
type SkillsetMetadata map[string]interface{}

// Value implements driver.Valuer interface
func (m SkillsetMetadata) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (m *SkillsetMetadata) Scan(value interface{}) error {
	if value == nil {
		*m = make(SkillsetMetadata)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}
