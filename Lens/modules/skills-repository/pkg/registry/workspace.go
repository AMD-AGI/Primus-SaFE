// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ======================== Workspace Operations ========================

// CreateWorkspace creates a new workspace
func (r *SkillsRegistry) CreateWorkspace(ctx context.Context, workspace *model.Workspace) error {
	if err := r.facade.Workspace.Create(ctx, workspace); err != nil {
		return err
	}
	log.Infof("Created workspace: %s", workspace.Name)
	return nil
}

// GetWorkspace retrieves a workspace by name
func (r *SkillsRegistry) GetWorkspace(ctx context.Context, name string) (*model.Workspace, error) {
	return r.facade.Workspace.GetByName(ctx, name)
}

// GetWorkspaceByID retrieves a workspace by ID
func (r *SkillsRegistry) GetWorkspaceByID(ctx context.Context, id int64) (*model.Workspace, error) {
	return r.facade.Workspace.GetByID(ctx, id)
}

// GetDefaultWorkspace retrieves the default workspace
func (r *SkillsRegistry) GetDefaultWorkspace(ctx context.Context) (*model.Workspace, error) {
	return r.facade.Workspace.GetDefault(ctx)
}

// ListWorkspaces retrieves all workspaces with pagination
func (r *SkillsRegistry) ListWorkspaces(ctx context.Context, offset, limit int) ([]*model.Workspace, int64, error) {
	return r.facade.Workspace.List(ctx, offset, limit)
}

// ListWorkspacesByOwner retrieves workspaces by owner
func (r *SkillsRegistry) ListWorkspacesByOwner(ctx context.Context, owner string, offset, limit int) ([]*model.Workspace, int64, error) {
	return r.facade.Workspace.ListByOwner(ctx, owner, offset, limit)
}

// UpdateWorkspace updates an existing workspace
func (r *SkillsRegistry) UpdateWorkspace(ctx context.Context, workspace *model.Workspace) error {
	if err := r.facade.Workspace.Update(ctx, workspace); err != nil {
		return err
	}
	log.Infof("Updated workspace: %s", workspace.Name)
	return nil
}

// DeleteWorkspace deletes a workspace and its skill associations
func (r *SkillsRegistry) DeleteWorkspace(ctx context.Context, name string) error {
	workspace, err := r.GetWorkspace(ctx, name)
	if err != nil {
		return err
	}

	// Delete all skill associations first
	if err := r.facade.WorkspaceSkill.DeleteByWorkspaceID(ctx, workspace.ID); err != nil {
		log.Warnf("Failed to delete skill associations for workspace %s: %v", name, err)
	}

	// Delete the workspace
	if err := r.facade.Workspace.Delete(ctx, workspace.ID); err != nil {
		return err
	}

	log.Infof("Deleted workspace: %s", name)
	return nil
}

// SetDefaultWorkspace sets a workspace as the default
func (r *SkillsRegistry) SetDefaultWorkspace(ctx context.Context, name string) error {
	workspace, err := r.GetWorkspace(ctx, name)
	if err != nil {
		return err
	}
	return r.facade.Workspace.SetDefault(ctx, workspace.ID)
}

// ======================== Workspace-Skill Operations ========================

// AddSkillsToWorkspace adds skills to a workspace
func (r *SkillsRegistry) AddSkillsToWorkspace(ctx context.Context, workspaceName string, skillNames []string) error {
	workspace, err := r.GetWorkspace(ctx, workspaceName)
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

	return r.facade.WorkspaceSkill.BatchAddSkillsToWorkspace(ctx, workspace.ID, skillIDs)
}

// RemoveSkillsFromWorkspace removes skills from a workspace
func (r *SkillsRegistry) RemoveSkillsFromWorkspace(ctx context.Context, workspaceName string, skillNames []string) error {
	workspace, err := r.GetWorkspace(ctx, workspaceName)
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

	return r.facade.WorkspaceSkill.BatchRemoveSkillsFromWorkspace(ctx, workspace.ID, skillIDs)
}

// ListSkillsByWorkspace retrieves skills in a workspace with pagination
func (r *SkillsRegistry) ListSkillsByWorkspace(ctx context.Context, workspaceName string, offset, limit int) ([]*model.Skill, int64, error) {
	workspace, err := r.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, 0, err
	}

	// Get all skill IDs in the workspace
	skillIDs, err := r.facade.WorkspaceSkill.GetSkillIDsByWorkspaceID(ctx, workspace.ID)
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

// SearchInWorkspace performs semantic search within a workspace
func (r *SkillsRegistry) SearchInWorkspace(ctx context.Context, workspaceName, query string, limit int) ([]*SearchResult, error) {
	if r.embedder == nil {
		return nil, nil
	}

	workspace, err := r.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	// Get all skill IDs in the workspace
	skillIDs, err := r.facade.WorkspaceSkill.GetSkillIDsByWorkspaceID(ctx, workspace.ID)
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
		// Skip if not in workspace
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

// GetWorkspaceSkillNames returns the names of all skills in a workspace
func (r *SkillsRegistry) GetWorkspaceSkillNames(ctx context.Context, workspaceName string) ([]string, error) {
	workspace, err := r.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}

	skillIDs, err := r.facade.WorkspaceSkill.GetSkillIDsByWorkspaceID(ctx, workspace.ID)
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
