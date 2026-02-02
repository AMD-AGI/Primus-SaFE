// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkspaceSkillFacadeInterface defines the interface for WorkspaceSkill operations
type WorkspaceSkillFacadeInterface interface {
	// WorkspaceSkill operations
	AddSkillToWorkspace(ctx context.Context, workspaceID, skillID int64) error
	RemoveSkillFromWorkspace(ctx context.Context, workspaceID, skillID int64) error
	GetSkillIDsByWorkspaceID(ctx context.Context, workspaceID int64) ([]int64, error)
	GetWorkspaceIDsBySkillID(ctx context.Context, skillID int64) ([]int64, error)
	ListByWorkspaceID(ctx context.Context, workspaceID int64, offset, limit int) ([]*model.WorkspaceSkill, int64, error)
	BatchAddSkillsToWorkspace(ctx context.Context, workspaceID int64, skillIDs []int64) error
	BatchRemoveSkillsFromWorkspace(ctx context.Context, workspaceID int64, skillIDs []int64) error
	DeleteByWorkspaceID(ctx context.Context, workspaceID int64) error
	DeleteBySkillID(ctx context.Context, skillID int64) error
	IsSkillInWorkspace(ctx context.Context, workspaceID, skillID int64) (bool, error)
}

// WorkspaceSkillFacade implements WorkspaceSkillFacadeInterface
type WorkspaceSkillFacade struct {
	db *gorm.DB
}

// NewWorkspaceSkillFacade creates a new WorkspaceSkillFacade
func NewWorkspaceSkillFacade(db *gorm.DB) *WorkspaceSkillFacade {
	return &WorkspaceSkillFacade{db: db}
}

// AddSkillToWorkspace adds a skill to a workspace
func (f *WorkspaceSkillFacade) AddSkillToWorkspace(ctx context.Context, workspaceID, skillID int64) error {
	ws := &model.WorkspaceSkill{
		WorkspaceID: workspaceID,
		SkillID:     skillID,
	}
	return f.db.WithContext(ctx).Create(ws).Error
}

// RemoveSkillFromWorkspace removes a skill from a workspace
func (f *WorkspaceSkillFacade) RemoveSkillFromWorkspace(ctx context.Context, workspaceID, skillID int64) error {
	return f.db.WithContext(ctx).
		Where("workspace_id = ? AND skill_id = ?", workspaceID, skillID).
		Delete(&model.WorkspaceSkill{}).Error
}

// GetSkillIDsByWorkspaceID retrieves all skill IDs in a workspace
func (f *WorkspaceSkillFacade) GetSkillIDsByWorkspaceID(ctx context.Context, workspaceID int64) ([]int64, error) {
	var skillIDs []int64
	err := f.db.WithContext(ctx).
		Model(&model.WorkspaceSkill{}).
		Where("workspace_id = ?", workspaceID).
		Pluck("skill_id", &skillIDs).Error
	return skillIDs, err
}

// GetWorkspaceIDsBySkillID retrieves all workspace IDs that contain a skill
func (f *WorkspaceSkillFacade) GetWorkspaceIDsBySkillID(ctx context.Context, skillID int64) ([]int64, error) {
	var workspaceIDs []int64
	err := f.db.WithContext(ctx).
		Model(&model.WorkspaceSkill{}).
		Where("skill_id = ?", skillID).
		Pluck("workspace_id", &workspaceIDs).Error
	return workspaceIDs, err
}

// ListByWorkspaceID retrieves paginated workspace-skill relationships
func (f *WorkspaceSkillFacade) ListByWorkspaceID(ctx context.Context, workspaceID int64, offset, limit int) ([]*model.WorkspaceSkill, int64, error) {
	var items []*model.WorkspaceSkill
	var total int64

	query := f.db.WithContext(ctx).Model(&model.WorkspaceSkill{}).Where("workspace_id = ?", workspaceID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// BatchAddSkillsToWorkspace adds multiple skills to a workspace
func (f *WorkspaceSkillFacade) BatchAddSkillsToWorkspace(ctx context.Context, workspaceID int64, skillIDs []int64) error {
	if len(skillIDs) == 0 {
		return nil
	}

	items := make([]*model.WorkspaceSkill, len(skillIDs))
	for i, skillID := range skillIDs {
		items[i] = &model.WorkspaceSkill{
			WorkspaceID: workspaceID,
			SkillID:     skillID,
		}
	}

	// Use ON CONFLICT DO NOTHING to ignore duplicates
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&items).Error
}

// BatchRemoveSkillsFromWorkspace removes multiple skills from a workspace
func (f *WorkspaceSkillFacade) BatchRemoveSkillsFromWorkspace(ctx context.Context, workspaceID int64, skillIDs []int64) error {
	if len(skillIDs) == 0 {
		return nil
	}

	return f.db.WithContext(ctx).
		Where("workspace_id = ? AND skill_id IN ?", workspaceID, skillIDs).
		Delete(&model.WorkspaceSkill{}).Error
}

// DeleteByWorkspaceID deletes all skills associations for a workspace
func (f *WorkspaceSkillFacade) DeleteByWorkspaceID(ctx context.Context, workspaceID int64) error {
	return f.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Delete(&model.WorkspaceSkill{}).Error
}

// DeleteBySkillID deletes all workspace associations for a skill
func (f *WorkspaceSkillFacade) DeleteBySkillID(ctx context.Context, skillID int64) error {
	return f.db.WithContext(ctx).
		Where("skill_id = ?", skillID).
		Delete(&model.WorkspaceSkill{}).Error
}

// IsSkillInWorkspace checks if a skill is in a workspace
func (f *WorkspaceSkillFacade) IsSkillInWorkspace(ctx context.Context, workspaceID, skillID int64) (bool, error) {
	var count int64
	err := f.db.WithContext(ctx).
		Model(&model.WorkspaceSkill{}).
		Where("workspace_id = ? AND skill_id = ?", workspaceID, skillID).
		Count(&count).Error
	return count > 0, err
}
