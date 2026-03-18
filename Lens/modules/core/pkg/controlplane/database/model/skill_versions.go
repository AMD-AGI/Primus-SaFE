// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameSkillVersions = "skill_versions"

// SkillVersion represents a version history entry for a skill
type SkillVersion struct {
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SkillID       int64     `gorm:"column:skill_id;not null;index" json:"skill_id"`
	Version       string    `gorm:"column:version;not null" json:"version"`
	Content       string    `gorm:"column:content;not null" json:"content"`
	ChangeSummary string    `gorm:"column:change_summary" json:"change_summary"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	CreatedBy     string    `gorm:"column:created_by" json:"created_by"`
}

// TableName returns the table name
func (*SkillVersion) TableName() string {
	return TableNameSkillVersions
}
