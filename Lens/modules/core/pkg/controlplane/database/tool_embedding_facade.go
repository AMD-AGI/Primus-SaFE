// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ToolEmbeddingFacadeInterface defines the interface for ToolEmbedding operations
type ToolEmbeddingFacadeInterface interface {
	GetByToolName(ctx context.Context, toolName string) ([]*model.ToolEmbedding, error)
	GetByToolNameAndType(ctx context.Context, toolName, embeddingType string) (*model.ToolEmbedding, error)
	Upsert(ctx context.Context, embedding *model.ToolEmbedding) error
	DeleteByToolName(ctx context.Context, toolName string) error
	SemanticSearch(ctx context.Context, queryEmbedding []float32, limit int) ([]*ToolSearchResult, error)
	SemanticSearchWithDomain(ctx context.Context, queryEmbedding []float32, domain string, limit int) ([]*ToolSearchResult, error)
}

// ToolSearchResult represents a semantic search result
type ToolSearchResult struct {
	ToolName      string  `json:"tool_name"`
	EmbeddingType string  `json:"embedding_type"`
	Similarity    float64 `json:"similarity"`
}

// ToolEmbeddingFacade implements ToolEmbeddingFacadeInterface
type ToolEmbeddingFacade struct {
	db *gorm.DB
}

// NewToolEmbeddingFacade creates a new ToolEmbeddingFacade
func NewToolEmbeddingFacade(db *gorm.DB) *ToolEmbeddingFacade {
	return &ToolEmbeddingFacade{db: db}
}

// GetByToolName retrieves all embeddings for a tool
func (f *ToolEmbeddingFacade) GetByToolName(ctx context.Context, toolName string) ([]*model.ToolEmbedding, error) {
	var embeddings []*model.ToolEmbedding
	err := f.db.WithContext(ctx).Where("tool_name = ?", toolName).Find(&embeddings).Error
	if err != nil {
		return nil, err
	}
	return embeddings, nil
}

// GetByToolNameAndType retrieves a specific embedding
func (f *ToolEmbeddingFacade) GetByToolNameAndType(ctx context.Context, toolName, embeddingType string) (*model.ToolEmbedding, error) {
	var embedding model.ToolEmbedding
	err := f.db.WithContext(ctx).
		Where("tool_name = ? AND embedding_type = ?", toolName, embeddingType).
		First(&embedding).Error
	if err != nil {
		return nil, err
	}
	return &embedding, nil
}

// Upsert creates or updates an embedding
func (f *ToolEmbeddingFacade) Upsert(ctx context.Context, embedding *model.ToolEmbedding) error {
	embedding.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tool_name"}, {Name: "embedding_type"}},
			DoUpdates: clause.AssignmentColumns([]string{"embedding", "text_content", "model_version", "updated_at"}),
		}).
		Create(embedding).Error
}

// DeleteByToolName deletes all embeddings for a tool
func (f *ToolEmbeddingFacade) DeleteByToolName(ctx context.Context, toolName string) error {
	return f.db.WithContext(ctx).Where("tool_name = ?", toolName).Delete(&model.ToolEmbedding{}).Error
}

// SemanticSearch performs vector similarity search
func (f *ToolEmbeddingFacade) SemanticSearch(ctx context.Context, queryEmbedding []float32, limit int) ([]*ToolSearchResult, error) {
	var results []*ToolSearchResult

	// Use pgvector cosine similarity: 1 - (embedding <=> query) gives similarity score
	// Use raw SQL for proper pgvector operator handling
	vectorStr := pgvector.NewVector(queryEmbedding).String()

	err := f.db.WithContext(ctx).
		Raw(`SELECT tool_name, embedding_type, 1 - (embedding <=> ?) as similarity 
			 FROM tool_embeddings 
			 WHERE embedding_type IN ('combined', 'description') 
			 ORDER BY embedding <=> ? 
			 LIMIT ?`, vectorStr, vectorStr, limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// SemanticSearchWithDomain performs vector similarity search filtered by domain
func (f *ToolEmbeddingFacade) SemanticSearchWithDomain(ctx context.Context, queryEmbedding []float32, domain string, limit int) ([]*ToolSearchResult, error) {
	var results []*ToolSearchResult

	vectorStr := pgvector.NewVector(queryEmbedding).String()

	err := f.db.WithContext(ctx).
		Raw(`SELECT te.tool_name, te.embedding_type, 1 - (te.embedding <=> ?) as similarity 
			 FROM tool_embeddings te
			 JOIN tools t ON te.tool_name = t.name
			 WHERE te.embedding_type IN ('combined', 'description') 
			   AND t.domain = ?
			 ORDER BY te.embedding <=> ? 
			 LIMIT ?`, vectorStr, domain, vectorStr, limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}
