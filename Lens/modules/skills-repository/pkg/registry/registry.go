// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/pgvector/pgvector-go"
)

// SkillsRegistry manages skill registration and discovery
type SkillsRegistry struct {
	facade   *database.ControlPlaneFacade
	embedder embedding.Embedder
	cache    map[string]*model.Skill
	mu       sync.RWMutex
}

// NewSkillsRegistry creates a new SkillsRegistry
func NewSkillsRegistry(facade *database.ControlPlaneFacade, embedder embedding.Embedder) *SkillsRegistry {
	return &SkillsRegistry{
		facade:   facade,
		embedder: embedder,
		cache:    make(map[string]*model.Skill),
	}
}

// Register adds or updates a skill in the registry
func (r *SkillsRegistry) Register(ctx context.Context, skill *model.Skill) error {
	// Save to database
	if err := r.facade.Skill.Upsert(ctx, skill); err != nil {
		return err
	}

	// Update cache
	r.mu.Lock()
	r.cache[skill.Name] = skill
	r.mu.Unlock()

	// Generate and store embedding
	if r.embedder != nil {
		go r.indexSkill(ctx, skill)
	}

	log.Infof("Registered skill: %s (source: %s)", skill.Name, skill.Source)
	return nil
}

// Get retrieves a skill by name
func (r *SkillsRegistry) Get(ctx context.Context, name string) (*model.Skill, error) {
	// Check cache first
	r.mu.RLock()
	if skill, ok := r.cache[name]; ok {
		r.mu.RUnlock()
		return skill, nil
	}
	r.mu.RUnlock()

	// Fetch from database
	skill, err := r.facade.Skill.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}

	// Update cache
	r.mu.Lock()
	r.cache[skill.Name] = skill
	r.mu.Unlock()

	return skill, nil
}

// GetContent retrieves the full SKILL.md content for a skill
func (r *SkillsRegistry) GetContent(ctx context.Context, name string) (string, error) {
	skill, err := r.Get(ctx, name)
	if err != nil {
		return "", err
	}
	return skill.Content, nil
}

// List retrieves all skills with pagination
func (r *SkillsRegistry) List(ctx context.Context, offset, limit int) ([]*model.Skill, int64, error) {
	return r.facade.Skill.List(ctx, offset, limit)
}

// ListByCategory retrieves skills by category
func (r *SkillsRegistry) ListByCategory(ctx context.Context, category string, offset, limit int) ([]*model.Skill, int64, error) {
	return r.facade.Skill.ListByCategory(ctx, category, offset, limit)
}

// ListBySource retrieves skills by source
func (r *SkillsRegistry) ListBySource(ctx context.Context, source string, offset, limit int) ([]*model.Skill, int64, error) {
	return r.facade.Skill.ListBySource(ctx, source, offset, limit)
}

// Search performs semantic search for skills
func (r *SkillsRegistry) Search(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	if r.embedder == nil {
		return nil, nil
	}

	// Generate query embedding
	queryEmbedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	// Search in database
	dbResults, err := r.facade.SkillEmbedding.SemanticSearch(ctx, queryEmbedding, limit*2)
	if err != nil {
		return nil, err
	}

	// Deduplicate and build results
	seen := make(map[string]bool)
	var results []*SearchResult

	for _, dbResult := range dbResults {
		if seen[dbResult.SkillName] {
			continue
		}
		seen[dbResult.SkillName] = true

		skill, err := r.Get(ctx, dbResult.SkillName)
		if err != nil {
			continue
		}

		results = append(results, &SearchResult{
			Skill:     skill,
			Score:     dbResult.Similarity,
			MatchType: dbResult.EmbeddingType,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// Delete removes a skill from the registry
func (r *SkillsRegistry) Delete(ctx context.Context, name string) error {
	skill, err := r.Get(ctx, name)
	if err != nil {
		return err
	}

	// Delete from database
	if err := r.facade.Skill.Delete(ctx, skill.ID); err != nil {
		return err
	}

	// Delete embeddings
	if err := r.facade.SkillEmbedding.DeleteBySkillName(ctx, name); err != nil {
		log.Warnf("Failed to delete embeddings for skill %s: %v", name, err)
	}

	// Remove from cache
	r.mu.Lock()
	delete(r.cache, name)
	r.mu.Unlock()

	log.Infof("Deleted skill: %s", name)
	return nil
}

// indexSkill generates and stores embeddings for a skill
func (r *SkillsRegistry) indexSkill(ctx context.Context, skill *model.Skill) {
	embeddings := []struct {
		Type    string
		Content string
	}{
		{model.EmbeddingTypeDescription, skill.Description},
		{model.EmbeddingTypeCombined, skill.Name + ": " + skill.Description},
	}

	for _, e := range embeddings {
		vector, err := r.embedder.Embed(ctx, e.Content)
		if err != nil {
			log.Warnf("Failed to generate embedding for skill %s (%s): %v", skill.Name, e.Type, err)
			continue
		}

		embedding := &model.SkillEmbedding{
			SkillName:     skill.Name,
			EmbeddingType: e.Type,
			Embedding:     pgvector.NewVector(vector),
			TextContent:   e.Content,
			ModelVersion:  r.embedder.ModelName(),
		}

		if err := r.facade.SkillEmbedding.Upsert(ctx, embedding); err != nil {
			log.Warnf("Failed to store embedding for skill %s (%s): %v", skill.Name, e.Type, err)
		}
	}

	log.Debugf("Indexed skill: %s", skill.Name)
}

// SearchResult represents a semantic search result
type SearchResult struct {
	Skill     *model.Skill
	Score     float64
	MatchType string
}
