// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

const TableNameToolEmbeddings = "tool_embeddings"

// ToolEmbedding represents a vector embedding for semantic search
type ToolEmbedding struct {
	ID            int64           `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ToolName      string          `gorm:"column:tool_name;not null;index" json:"tool_name"`
	EmbeddingType string          `gorm:"column:embedding_type;not null" json:"embedding_type"` // name, description, combined
	Embedding     pgvector.Vector `gorm:"column:embedding;type:vector(1024)" json:"-"`
	TextContent   string          `gorm:"column:text_content" json:"text_content"`
	ModelVersion  string          `gorm:"column:model_version" json:"model_version"`
	CreatedAt     time.Time       `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*ToolEmbedding) TableName() string {
	return TableNameToolEmbeddings
}

// Tool embedding type constants
const (
	ToolEmbeddingTypeName        = "name"
	ToolEmbeddingTypeDescription = "description"
	ToolEmbeddingTypeCombined    = "combined"
)
