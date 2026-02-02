// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ======================== Skillset Operations ========================

// CreateSkillset creates a new skillset
func (r *SkillsRegistry) CreateSkillset(ctx context.Context, skillset *model.Skillset) error {
	if err := r.facade.Skillset.Create(ctx, skillset); err != nil {
		return err
	}
	log.Infof("Created skillset: %s", skillset.Name)
	return nil
}

// GetSkillset retrieves a skillset by name
func (r *SkillsRegistry) GetSkillset(ctx context.Context, name string) (*model.Skillset, error) {
	return r.facade.Skillset.GetByName(ctx, name)
}

// GetSkillsetByID retrieves a skillset by ID
func (r *SkillsRegistry) GetSkillsetByID(ctx context.Context, id int64) (*model.Skillset, error) {
	return r.facade.Skillset.GetByID(ctx, id)
}

// GetDefaultSkillset retrieves the default skillset
func (r *SkillsRegistry) GetDefaultSkillset(ctx context.Context) (*model.Skillset, error) {
	return r.facade.Skillset.GetDefault(ctx)
}

// ListSkillsets retrieves all skillsets with pagination
func (r *SkillsRegistry) ListSkillsets(ctx context.Context, offset, limit int) ([]*model.Skillset, int64, error) {
	return r.facade.Skillset.List(ctx, offset, limit)
}

// ListSkillsetsByOwner retrieves skillsets by owner
func (r *SkillsRegistry) ListSkillsetsByOwner(ctx context.Context, owner string, offset, limit int) ([]*model.Skillset, int64, error) {
	return r.facade.Skillset.ListByOwner(ctx, owner, offset, limit)
}

// UpdateSkillset updates an existing skillset
func (r *SkillsRegistry) UpdateSkillset(ctx context.Context, skillset *model.Skillset) error {
	if err := r.facade.Skillset.Update(ctx, skillset); err != nil {
		return err
	}
	log.Infof("Updated skillset: %s", skillset.Name)
	return nil
}

// DeleteSkillset deletes a skillset and its skill associations
func (r *SkillsRegistry) DeleteSkillset(ctx context.Context, name string) error {
	skillset, err := r.GetSkillset(ctx, name)
	if err != nil {
		return err
	}

	// Delete all skill associations first
	if err := r.facade.SkillsetSkill.DeleteBySkillsetID(ctx, skillset.ID); err != nil {
		log.Warnf("Failed to delete skill associations for skillset %s: %v", name, err)
	}

	// Delete the skillset
	if err := r.facade.Skillset.Delete(ctx, skillset.ID); err != nil {
		return err
	}

	log.Infof("Deleted skillset: %s", name)
	return nil
}

// SetDefaultSkillset sets a skillset as the default
func (r *SkillsRegistry) SetDefaultSkillset(ctx context.Context, name string) error {
	skillset, err := r.GetSkillset(ctx, name)
	if err != nil {
		return err
	}
	return r.facade.Skillset.SetDefault(ctx, skillset.ID)
}

// ======================== Skillset-Skill Operations ========================

// AddSkillsToSkillset adds skills to a skillset
func (r *SkillsRegistry) AddSkillsToSkillset(ctx context.Context, skillsetName string, skillNames []string) error {
	skillset, err := r.GetSkillset(ctx, skillsetName)
	if err != nil {
		return err
	}

	var skillIDs []int64
	for _, skillName := range skillNames {
		skill, err := r.Get(ctx, skillName)
		if err != nil {
			log.Warnf("Skill not found: %s", skillName)
			continue
		}
		skillIDs = append(skillIDs, skill.ID)
	}

	if len(skillIDs) == 0 {
		return nil
	}

	return r.facade.SkillsetSkill.BatchAddSkillsToSkillset(ctx, skillset.ID, skillIDs)
}

// RemoveSkillsFromSkillset removes skills from a skillset
func (r *SkillsRegistry) RemoveSkillsFromSkillset(ctx context.Context, skillsetName string, skillNames []string) error {
	skillset, err := r.GetSkillset(ctx, skillsetName)
	if err != nil {
		return err
	}

	var skillIDs []int64
	for _, skillName := range skillNames {
		skill, err := r.Get(ctx, skillName)
		if err != nil {
			continue
		}
		skillIDs = append(skillIDs, skill.ID)
	}

	if len(skillIDs) == 0 {
		return nil
	}

	return r.facade.SkillsetSkill.BatchRemoveSkillsFromSkillset(ctx, skillset.ID, skillIDs)
}

// ListSkillsBySkillset retrieves skills in a skillset with pagination
func (r *SkillsRegistry) ListSkillsBySkillset(ctx context.Context, skillsetName string, offset, limit int) ([]*model.Skill, int64, error) {
	skillset, err := r.GetSkillset(ctx, skillsetName)
	if err != nil {
		return nil, 0, err
	}

	// Get all skill IDs in the skillset
	skillIDs, err := r.facade.SkillsetSkill.GetSkillIDsBySkillsetID(ctx, skillset.ID)
	if err != nil {
		return nil, 0, err
	}

	if len(skillIDs) == 0 {
		return []*model.Skill{}, 0, nil
	}

	// Get skills by IDs with pagination
	// For simplicity, we'll fetch all and paginate in memory
	// In production, you might want to add a ListByIDs method to SkillFacade
	var skills []*model.Skill
	for _, skillID := range skillIDs {
		skill, err := r.facade.Skill.GetByID(ctx, skillID)
		if err != nil {
			continue
		}
		skills = append(skills, skill)
	}

	total := int64(len(skills))

	// Apply pagination
	start := offset
	if start > len(skills) {
		start = len(skills)
	}
	end := offset + limit
	if end > len(skills) {
		end = len(skills)
	}

	return skills[start:end], total, nil
}

// SearchInSkillset performs semantic search within a skillset
func (r *SkillsRegistry) SearchInSkillset(ctx context.Context, skillsetName, query string, limit int) ([]*SearchResult, error) {
	if r.embedder == nil {
		return nil, nil
	}

	skillset, err := r.GetSkillset(ctx, skillsetName)
	if err != nil {
		return nil, err
	}

	// Get all skill IDs in the skillset
	skillIDs, err := r.facade.SkillsetSkill.GetSkillIDsBySkillsetID(ctx, skillset.ID)
	if err != nil {
		return nil, err
	}

	if len(skillIDs) == 0 {
		return []*SearchResult{}, nil
	}

	// Build a set of valid skill names
	validSkillNames := make(map[string]bool)
	for _, skillID := range skillIDs {
		skill, err := r.facade.Skill.GetByID(ctx, skillID)
		if err != nil {
			continue
		}
		validSkillNames[skill.Name] = true
	}

	// Generate query embedding
	queryEmbedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	// Search in database (get more results to filter)
	dbResults, err := r.facade.SkillEmbedding.SemanticSearch(ctx, queryEmbedding, limit*5)
	if err != nil {
		return nil, err
	}

	// Filter and deduplicate results
	seen := make(map[string]bool)
	var results []*SearchResult

	for _, dbResult := range dbResults {
		// Skip if not in skillset
		if !validSkillNames[dbResult.SkillName] {
			continue
		}

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

// GetSkillsetSkillNames returns the names of all skills in a skillset
func (r *SkillsRegistry) GetSkillsetSkillNames(ctx context.Context, skillsetName string) ([]string, error) {
	skillset, err := r.GetSkillset(ctx, skillsetName)
	if err != nil {
		return nil, err
	}

	skillIDs, err := r.facade.SkillsetSkill.GetSkillIDsBySkillsetID(ctx, skillset.ID)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, skillID := range skillIDs {
		skill, err := r.facade.Skill.GetByID(ctx, skillID)
		if err != nil {
			continue
		}
		names = append(names, skill.Name)
	}

	return names, nil
}
