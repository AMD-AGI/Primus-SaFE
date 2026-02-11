// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SkillsetSkillFacadeInterface defines the interface for SkillsetSkill operations
type SkillsetSkillFacadeInterface interface {
	// SkillsetSkill operations
	AddSkillToSkillset(ctx context.Context, skillsetID, skillID int64) error
	RemoveSkillFromSkillset(ctx context.Context, skillsetID, skillID int64) error
	GetSkillIDsBySkillsetID(ctx context.Context, skillsetID int64) ([]int64, error)
	GetSkillsetIDsBySkillID(ctx context.Context, skillID int64) ([]int64, error)
	ListBySkillsetID(ctx context.Context, skillsetID int64, offset, limit int) ([]*model.SkillsetSkill, int64, error)
	BatchAddSkillsToSkillset(ctx context.Context, skillsetID int64, skillIDs []int64) error
	BatchRemoveSkillsFromSkillset(ctx context.Context, skillsetID int64, skillIDs []int64) error
	DeleteBySkillsetID(ctx context.Context, skillsetID int64) error
	DeleteBySkillID(ctx context.Context, skillID int64) error
	IsSkillInSkillset(ctx context.Context, skillsetID, skillID int64) (bool, error)
}

// SkillsetSkillFacade implements SkillsetSkillFacadeInterface
type SkillsetSkillFacade struct {
	db *gorm.DB
}

// NewSkillsetSkillFacade creates a new SkillsetSkillFacade
func NewSkillsetSkillFacade(db *gorm.DB) *SkillsetSkillFacade {
	return &SkillsetSkillFacade{db: db}
}

// AddSkillToSkillset adds a skill to a skillset
func (f *SkillsetSkillFacade) AddSkillToSkillset(ctx context.Context, skillsetID, skillID int64) error {
	ss := &model.SkillsetSkill{
		SkillsetID: skillsetID,
		SkillID:    skillID,
	}
	return f.db.WithContext(ctx).Create(ss).Error
}

// RemoveSkillFromSkillset removes a skill from a skillset
func (f *SkillsetSkillFacade) RemoveSkillFromSkillset(ctx context.Context, skillsetID, skillID int64) error {
	return f.db.WithContext(ctx).
		Where("skillset_id = ? AND skill_id = ?", skillsetID, skillID).
		Delete(&model.SkillsetSkill{}).Error
}

// GetSkillIDsBySkillsetID retrieves all skill IDs in a skillset
func (f *SkillsetSkillFacade) GetSkillIDsBySkillsetID(ctx context.Context, skillsetID int64) ([]int64, error) {
	var skillIDs []int64
	err := f.db.WithContext(ctx).
		Model(&model.SkillsetSkill{}).
		Where("skillset_id = ?", skillsetID).
		Pluck("skill_id", &skillIDs).Error
	return skillIDs, err
}

// GetSkillsetIDsBySkillID retrieves all skillset IDs that contain a skill
func (f *SkillsetSkillFacade) GetSkillsetIDsBySkillID(ctx context.Context, skillID int64) ([]int64, error) {
	var skillsetIDs []int64
	err := f.db.WithContext(ctx).
		Model(&model.SkillsetSkill{}).
		Where("skill_id = ?", skillID).
		Pluck("skillset_id", &skillsetIDs).Error
	return skillsetIDs, err
}

// ListBySkillsetID retrieves paginated skillset-skill relationships
func (f *SkillsetSkillFacade) ListBySkillsetID(ctx context.Context, skillsetID int64, offset, limit int) ([]*model.SkillsetSkill, int64, error) {
	var items []*model.SkillsetSkill
	var total int64

	query := f.db.WithContext(ctx).Model(&model.SkillsetSkill{}).Where("skillset_id = ?", skillsetID)

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

// BatchAddSkillsToSkillset adds multiple skills to a skillset
func (f *SkillsetSkillFacade) BatchAddSkillsToSkillset(ctx context.Context, skillsetID int64, skillIDs []int64) error {
	if len(skillIDs) == 0 {
		return nil
	}

	items := make([]*model.SkillsetSkill, len(skillIDs))
	for i, skillID := range skillIDs {
		items[i] = &model.SkillsetSkill{
			SkillsetID: skillsetID,
			SkillID:    skillID,
		}
	}

	// Use ON CONFLICT DO NOTHING to ignore duplicates
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&items).Error
}

// BatchRemoveSkillsFromSkillset removes multiple skills from a skillset
func (f *SkillsetSkillFacade) BatchRemoveSkillsFromSkillset(ctx context.Context, skillsetID int64, skillIDs []int64) error {
	if len(skillIDs) == 0 {
		return nil
	}

	return f.db.WithContext(ctx).
		Where("skillset_id = ? AND skill_id IN ?", skillsetID, skillIDs).
		Delete(&model.SkillsetSkill{}).Error
}

// DeleteBySkillsetID deletes all skills associations for a skillset
func (f *SkillsetSkillFacade) DeleteBySkillsetID(ctx context.Context, skillsetID int64) error {
	return f.db.WithContext(ctx).
		Where("skillset_id = ?", skillsetID).
		Delete(&model.SkillsetSkill{}).Error
}

// DeleteBySkillID deletes all skillset associations for a skill
func (f *SkillsetSkillFacade) DeleteBySkillID(ctx context.Context, skillID int64) error {
	return f.db.WithContext(ctx).
		Where("skill_id = ?", skillID).
		Delete(&model.SkillsetSkill{}).Error
}

// IsSkillInSkillset checks if a skill is in a skillset
func (f *SkillsetSkillFacade) IsSkillInSkillset(ctx context.Context, skillsetID, skillID int64) (bool, error) {
	var count int64
	err := f.db.WithContext(ctx).
		Model(&model.SkillsetSkill{}).
		Where("skillset_id = ? AND skill_id = ?", skillsetID, skillID).
		Count(&count).Error
	return count > 0, err
}
