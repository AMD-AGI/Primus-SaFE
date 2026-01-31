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

// SkillEmbeddingFacadeInterface defines the interface for SkillEmbedding operations
type SkillEmbeddingFacadeInterface interface {
	GetBySkillName(ctx context.Context, skillName string) ([]*model.SkillEmbedding, error)
	GetBySkillNameAndType(ctx context.Context, skillName, embeddingType string) (*model.SkillEmbedding, error)
	Upsert(ctx context.Context, embedding *model.SkillEmbedding) error
	DeleteBySkillName(ctx context.Context, skillName string) error
	SemanticSearch(ctx context.Context, queryEmbedding []float32, limit int) ([]*SkillSearchResult, error)
}

// SkillSearchResult represents a semantic search result
type SkillSearchResult struct {
	SkillName     string  `json:"skill_name"`
	EmbeddingType string  `json:"embedding_type"`
	Similarity    float64 `json:"similarity"`
}

// SkillEmbeddingFacade implements SkillEmbeddingFacadeInterface
type SkillEmbeddingFacade struct {
	db *gorm.DB
}

// NewSkillEmbeddingFacade creates a new SkillEmbeddingFacade
func NewSkillEmbeddingFacade(db *gorm.DB) *SkillEmbeddingFacade {
	return &SkillEmbeddingFacade{db: db}
}

// GetBySkillName retrieves all embeddings for a skill
func (f *SkillEmbeddingFacade) GetBySkillName(ctx context.Context, skillName string) ([]*model.SkillEmbedding, error) {
	var embeddings []*model.SkillEmbedding
	err := f.db.WithContext(ctx).Where("skill_name = ?", skillName).Find(&embeddings).Error
	if err != nil {
		return nil, err
	}
	return embeddings, nil
}

// GetBySkillNameAndType retrieves a specific embedding
func (f *SkillEmbeddingFacade) GetBySkillNameAndType(ctx context.Context, skillName, embeddingType string) (*model.SkillEmbedding, error) {
	var embedding model.SkillEmbedding
	err := f.db.WithContext(ctx).
		Where("skill_name = ? AND embedding_type = ?", skillName, embeddingType).
		First(&embedding).Error
	if err != nil {
		return nil, err
	}
	return &embedding, nil
}

// Upsert creates or updates an embedding
func (f *SkillEmbeddingFacade) Upsert(ctx context.Context, embedding *model.SkillEmbedding) error {
	embedding.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "skill_name"}, {Name: "embedding_type"}},
			DoUpdates: clause.AssignmentColumns([]string{"embedding", "text_content", "model_version", "updated_at"}),
		}).
		Create(embedding).Error
}

// DeleteBySkillName deletes all embeddings for a skill
func (f *SkillEmbeddingFacade) DeleteBySkillName(ctx context.Context, skillName string) error {
	return f.db.WithContext(ctx).Where("skill_name = ?", skillName).Delete(&model.SkillEmbedding{}).Error
}

// SemanticSearch performs vector similarity search
func (f *SkillEmbeddingFacade) SemanticSearch(ctx context.Context, queryEmbedding []float32, limit int) ([]*SkillSearchResult, error) {
	var results []*SkillSearchResult

	// Use pgvector cosine similarity: 1 - (embedding <=> query) gives similarity score
	// Use raw SQL for proper pgvector operator handling
	vectorStr := pgvector.NewVector(queryEmbedding).String()
	
	err := f.db.WithContext(ctx).
		Raw(`SELECT skill_name, embedding_type, 1 - (embedding <=> ?) as similarity 
			 FROM skill_embeddings 
			 WHERE embedding_type IN ('combined', 'description') 
			 ORDER BY embedding <=> ? 
			 LIMIT ?`, vectorStr, vectorStr, limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}
