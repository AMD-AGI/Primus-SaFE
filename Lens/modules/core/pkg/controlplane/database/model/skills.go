// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

const TableNameSkills = "skills"

// Skill represents a registered skill in the skills repository
type Skill struct {
	ID          int64          `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name        string         `gorm:"column:name;not null;uniqueIndex" json:"name"`
	Description string         `gorm:"column:description;not null" json:"description"`
	Category    string         `gorm:"column:category" json:"category"`
	Version     string         `gorm:"column:version" json:"version"`
	Source      string         `gorm:"column:source;not null" json:"source"` // platform, team, user
	License     string         `gorm:"column:license" json:"license"`
	Content     string         `gorm:"column:content" json:"content"`   // Full SKILL.md content
	FilePath    string         `gorm:"column:file_path" json:"file_path"`
	Metadata    SkillsMetadata `gorm:"column:metadata;default:{}" json:"metadata"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*Skill) TableName() string {
	return TableNameSkills
}

// SkillsMetadata is a custom type for JSONB metadata field
type SkillsMetadata map[string]interface{}

// Value implements driver.Valuer interface
func (m SkillsMetadata) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// Scan implements sql.Scanner interface
func (m *SkillsMetadata) Scan(value interface{}) error {
	if value == nil {
		*m = make(SkillsMetadata)
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

// Skill source constants
const (
	SkillSourcePlatform = "platform"
	SkillSourceTeam     = "team"
	SkillSourceUser     = "user"
)
