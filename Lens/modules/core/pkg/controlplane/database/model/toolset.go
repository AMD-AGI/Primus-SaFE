// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

const TableNameToolsets = "toolsets"
const TableNameToolsetTools = "toolset_tools"

// Toolset represents a collection of tools
type Toolset struct {
	ID          int64   `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name        string  `gorm:"column:name;not null" json:"name"`
	DisplayName string  `gorm:"column:display_name" json:"display_name"`
	Description string  `gorm:"column:description;not null;default:''" json:"description"`
	Tags        AppTags `gorm:"column:tags;default:[]" json:"tags"`
	IconURL     string  `gorm:"column:icon_url" json:"icon_url"`

	// Access control
	OwnerUserID   string `gorm:"column:owner_user_id" json:"owner_user_id"`
	OwnerUserName string `gorm:"column:owner_user_name" json:"owner_user_name"`
	IsPublic      bool   `gorm:"column:is_public;default:true" json:"is_public"`

	// Statistics (denormalized for performance)
	ToolCount int `gorm:"column:tool_count;default:0" json:"tool_count"`

	// Semantic search (not exposed in JSON response)
	Embedding pgvector.Vector `gorm:"type:vector(1024)" json:"-"`

	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"` // Soft delete
}

// TableName returns the table name
func (*Toolset) TableName() string {
	return TableNameToolsets
}

// ToolsetTool represents the many-to-many relationship between toolsets and tools
type ToolsetTool struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ToolsetID int64     `gorm:"column:toolset_id;not null" json:"toolset_id"`
	ToolID    int64     `gorm:"column:tool_id;not null" json:"tool_id"`
	SortOrder int       `gorm:"column:sort_order;default:0" json:"sort_order"`
	AddedAt   time.Time `gorm:"column:added_at;autoCreateTime" json:"added_at"`
}

// TableName returns the table name
func (*ToolsetTool) TableName() string {
	return TableNameToolsetTools
}
