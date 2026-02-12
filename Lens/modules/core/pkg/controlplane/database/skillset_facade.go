// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// SkillsetFacadeInterface defines the interface for Skillset operations
type SkillsetFacadeInterface interface {
	// Skillset CRUD
	GetByID(ctx context.Context, id int64) (*model.Skillset, error)
	GetByName(ctx context.Context, name string) (*model.Skillset, error)
	GetDefault(ctx context.Context) (*model.Skillset, error)
	List(ctx context.Context, offset, limit int) ([]*model.Skillset, int64, error)
	ListByOwner(ctx context.Context, owner string, offset, limit int) ([]*model.Skillset, int64, error)
	Create(ctx context.Context, skillset *model.Skillset) error
	Update(ctx context.Context, skillset *model.Skillset) error
	Delete(ctx context.Context, id int64) error
	SetDefault(ctx context.Context, id int64) error
}

// SkillsetFacade implements SkillsetFacadeInterface
type SkillsetFacade struct {
	db *gorm.DB
}

// NewSkillsetFacade creates a new SkillsetFacade
func NewSkillsetFacade(db *gorm.DB) *SkillsetFacade {
	return &SkillsetFacade{db: db}
}

// GetByID retrieves a skillset by ID
func (f *SkillsetFacade) GetByID(ctx context.Context, id int64) (*model.Skillset, error) {
	var skillset model.Skillset
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&skillset).Error
	if err != nil {
		return nil, err
	}
	return &skillset, nil
}

// GetByName retrieves a skillset by name
func (f *SkillsetFacade) GetByName(ctx context.Context, name string) (*model.Skillset, error) {
	var skillset model.Skillset
	err := f.db.WithContext(ctx).Where("name = ?", name).First(&skillset).Error
	if err != nil {
		return nil, err
	}
	return &skillset, nil
}

// GetDefault retrieves the default skillset
func (f *SkillsetFacade) GetDefault(ctx context.Context) (*model.Skillset, error) {
	var skillset model.Skillset
	err := f.db.WithContext(ctx).Where("is_default = ?", true).First(&skillset).Error
	if err != nil {
		return nil, err
	}
	return &skillset, nil
}

// List retrieves paginated skillsets
func (f *SkillsetFacade) List(ctx context.Context, offset, limit int) ([]*model.Skillset, int64, error) {
	var skillsets []*model.Skillset
	var total int64

	err := f.db.WithContext(ctx).Model(&model.Skillset{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = f.db.WithContext(ctx).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&skillsets).Error
	if err != nil {
		return nil, 0, err
	}

	return skillsets, total, nil
}

// ListByOwner retrieves skillsets by owner
func (f *SkillsetFacade) ListByOwner(ctx context.Context, owner string, offset, limit int) ([]*model.Skillset, int64, error) {
	var skillsets []*model.Skillset
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Skillset{}).Where("owner = ?", owner)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&skillsets).Error
	if err != nil {
		return nil, 0, err
	}

	return skillsets, total, nil
}

// Create creates a new skillset
func (f *SkillsetFacade) Create(ctx context.Context, skillset *model.Skillset) error {
	return f.db.WithContext(ctx).Create(skillset).Error
}

// Update updates an existing skillset
func (f *SkillsetFacade) Update(ctx context.Context, skillset *model.Skillset) error {
	skillset.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(skillset).Error
}

// Delete deletes a skillset by ID
func (f *SkillsetFacade) Delete(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Delete(&model.Skillset{}, id).Error
}

// SetDefault sets a skillset as the default and unsets others
func (f *SkillsetFacade) SetDefault(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Unset all existing defaults
		if err := tx.Model(&model.Skillset{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		// Set the new default
		return tx.Model(&model.Skillset{}).Where("id = ?", id).Update("is_default", true).Error
	})
}
