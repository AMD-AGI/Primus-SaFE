// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// SkillFeedbackFacadeInterface defines the interface for SkillFeedback operations
type SkillFeedbackFacadeInterface interface {
	GetByID(ctx context.Context, id int64) (*model.SkillFeedback, error)
	ListBySkillName(ctx context.Context, skillName string, offset, limit int) ([]*model.SkillFeedback, int64, error)
	ListByExecutionID(ctx context.Context, executionID int64) ([]*model.SkillFeedback, error)
	GetAverageRating(ctx context.Context, skillName string) (float64, int64, error)
	Create(ctx context.Context, feedback *model.SkillFeedback) error
}

// SkillFeedbackFacade implements SkillFeedbackFacadeInterface
type SkillFeedbackFacade struct {
	db *gorm.DB
}

// NewSkillFeedbackFacade creates a new SkillFeedbackFacade
func NewSkillFeedbackFacade(db *gorm.DB) *SkillFeedbackFacade {
	return &SkillFeedbackFacade{db: db}
}

// GetByID retrieves feedback by ID
func (f *SkillFeedbackFacade) GetByID(ctx context.Context, id int64) (*model.SkillFeedback, error) {
	var feedback model.SkillFeedback
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&feedback).Error
	if err != nil {
		return nil, err
	}
	return &feedback, nil
}

// ListBySkillName retrieves feedback for a skill
func (f *SkillFeedbackFacade) ListBySkillName(ctx context.Context, skillName string, offset, limit int) ([]*model.SkillFeedback, int64, error) {
	var feedback []*model.SkillFeedback
	var total int64

	query := f.db.WithContext(ctx).Model(&model.SkillFeedback{}).Where("skill_name = ?", skillName)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&feedback).Error
	if err != nil {
		return nil, 0, err
	}

	return feedback, total, nil
}

// ListByExecutionID retrieves feedback for an execution
func (f *SkillFeedbackFacade) ListByExecutionID(ctx context.Context, executionID int64) ([]*model.SkillFeedback, error) {
	var feedback []*model.SkillFeedback
	err := f.db.WithContext(ctx).Where("execution_id = ?", executionID).Find(&feedback).Error
	if err != nil {
		return nil, err
	}
	return feedback, nil
}

// GetAverageRating returns the average rating and count for a skill
func (f *SkillFeedbackFacade) GetAverageRating(ctx context.Context, skillName string) (float64, int64, error) {
	var result struct {
		AvgRating float64
		Count     int64
	}

	err := f.db.WithContext(ctx).Model(&model.SkillFeedback{}).
		Select("AVG(rating) as avg_rating, COUNT(*) as count").
		Where("skill_name = ? AND rating IS NOT NULL", skillName).
		Scan(&result).Error
	if err != nil {
		return 0, 0, err
	}

	return result.AvgRating, result.Count, nil
}

// Create creates new feedback
func (f *SkillFeedbackFacade) Create(ctx context.Context, feedback *model.SkillFeedback) error {
	return f.db.WithContext(ctx).Create(feedback).Error
}
