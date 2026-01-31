// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// SkillFacadeInterface defines the interface for Skill operations
type SkillFacadeInterface interface {
	// Skill CRUD
	GetByID(ctx context.Context, id int64) (*model.Skill, error)
	GetByName(ctx context.Context, name string) (*model.Skill, error)
	List(ctx context.Context, offset, limit int) ([]*model.Skill, int64, error)
	ListByCategory(ctx context.Context, category string, offset, limit int) ([]*model.Skill, int64, error)
	ListBySource(ctx context.Context, source string, offset, limit int) ([]*model.Skill, int64, error)
	Create(ctx context.Context, skill *model.Skill) error
	Update(ctx context.Context, skill *model.Skill) error
	Delete(ctx context.Context, id int64) error
	Upsert(ctx context.Context, skill *model.Skill) error
}

// SkillFacade implements SkillFacadeInterface
type SkillFacade struct {
	db *gorm.DB
}

// NewSkillFacade creates a new SkillFacade
func NewSkillFacade(db *gorm.DB) *SkillFacade {
	return &SkillFacade{db: db}
}

// GetByID retrieves a skill by ID
func (f *SkillFacade) GetByID(ctx context.Context, id int64) (*model.Skill, error) {
	var skill model.Skill
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&skill).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

// GetByName retrieves a skill by name
func (f *SkillFacade) GetByName(ctx context.Context, name string) (*model.Skill, error) {
	var skill model.Skill
	err := f.db.WithContext(ctx).Where("name = ?", name).First(&skill).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

// List retrieves paginated skills
func (f *SkillFacade) List(ctx context.Context, offset, limit int) ([]*model.Skill, int64, error) {
	var skills []*model.Skill
	var total int64

	err := f.db.WithContext(ctx).Model(&model.Skill{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = f.db.WithContext(ctx).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&skills).Error
	if err != nil {
		return nil, 0, err
	}

	return skills, total, nil
}

// ListByCategory retrieves skills by category
func (f *SkillFacade) ListByCategory(ctx context.Context, category string, offset, limit int) ([]*model.Skill, int64, error) {
	var skills []*model.Skill
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Skill{}).Where("category = ?", category)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&skills).Error
	if err != nil {
		return nil, 0, err
	}

	return skills, total, nil
}

// ListBySource retrieves skills by source
func (f *SkillFacade) ListBySource(ctx context.Context, source string, offset, limit int) ([]*model.Skill, int64, error) {
	var skills []*model.Skill
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Skill{}).Where("source = ?", source)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&skills).Error
	if err != nil {
		return nil, 0, err
	}

	return skills, total, nil
}

// Create creates a new skill
func (f *SkillFacade) Create(ctx context.Context, skill *model.Skill) error {
	return f.db.WithContext(ctx).Create(skill).Error
}

// Update updates an existing skill
func (f *SkillFacade) Update(ctx context.Context, skill *model.Skill) error {
	skill.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(skill).Error
}

// Delete deletes a skill by ID
func (f *SkillFacade) Delete(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Delete(&model.Skill{}, id).Error
}

// Upsert creates or updates a skill based on name
func (f *SkillFacade) Upsert(ctx context.Context, skill *model.Skill) error {
	existing, err := f.GetByName(ctx, skill.Name)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return f.Create(ctx, skill)
		}
		return err
	}
	skill.ID = existing.ID
	skill.CreatedAt = existing.CreatedAt
	return f.Update(ctx, skill)
}
