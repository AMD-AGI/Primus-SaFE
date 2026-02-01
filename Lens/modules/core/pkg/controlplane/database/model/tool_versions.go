// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameToolVersions = "tool_versions"

// ToolVersion represents a version history entry for a tool
type ToolVersion struct {
	ID           int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ToolID       int64      `gorm:"column:tool_id;not null;index" json:"tool_id"`
	Version      string     `gorm:"column:version;not null" json:"version"`
	InputSchema  ToolSchema `gorm:"column:input_schema" json:"input_schema"`
	OutputSchema ToolSchema `gorm:"column:output_schema" json:"output_schema"`
	ChangeLog    string     `gorm:"column:change_log" json:"change_log"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	CreatedBy    string     `gorm:"column:created_by" json:"created_by"`
}

// TableName returns the table name
func (*ToolVersion) TableName() string {
	return TableNameToolVersions
}
