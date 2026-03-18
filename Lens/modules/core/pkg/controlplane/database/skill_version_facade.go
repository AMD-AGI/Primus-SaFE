// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// SkillVersionFacadeInterface defines the interface for SkillVersion operations
type SkillVersionFacadeInterface interface {
	GetByID(ctx context.Context, id int64) (*model.SkillVersion, error)
	ListBySkillID(ctx context.Context, skillID int64, offset, limit int) ([]*model.SkillVersion, int64, error)
	GetLatestBySkillID(ctx context.Context, skillID int64) (*model.SkillVersion, error)
	Create(ctx context.Context, version *model.SkillVersion) error
}

// SkillVersionFacade implements SkillVersionFacadeInterface
type SkillVersionFacade struct {
	db *gorm.DB
}

// NewSkillVersionFacade creates a new SkillVersionFacade
func NewSkillVersionFacade(db *gorm.DB) *SkillVersionFacade {
	return &SkillVersionFacade{db: db}
}

// GetByID retrieves a skill version by ID
func (f *SkillVersionFacade) GetByID(ctx context.Context, id int64) (*model.SkillVersion, error) {
	var version model.SkillVersion
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// ListBySkillID retrieves versions for a skill
func (f *SkillVersionFacade) ListBySkillID(ctx context.Context, skillID int64, offset, limit int) ([]*model.SkillVersion, int64, error) {
	var versions []*model.SkillVersion
	var total int64

	query := f.db.WithContext(ctx).Model(&model.SkillVersion{}).Where("skill_id = ?", skillID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&versions).Error
	if err != nil {
		return nil, 0, err
	}

	return versions, total, nil
}

// GetLatestBySkillID retrieves the latest version for a skill
func (f *SkillVersionFacade) GetLatestBySkillID(ctx context.Context, skillID int64) (*model.SkillVersion, error) {
	var version model.SkillVersion
	err := f.db.WithContext(ctx).
		Where("skill_id = ?", skillID).
		Order("created_at DESC").
		First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// Create creates a new skill version
func (f *SkillVersionFacade) Create(ctx context.Context, version *model.SkillVersion) error {
	return f.db.WithContext(ctx).Create(version).Error
}
