// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// SkillExecutionFacadeInterface defines the interface for SkillExecution operations
type SkillExecutionFacadeInterface interface {
	GetByID(ctx context.Context, id int64) (*model.SkillExecution, error)
	ListBySkillName(ctx context.Context, skillName string, offset, limit int) ([]*model.SkillExecution, int64, error)
	ListByUserID(ctx context.Context, userID string, offset, limit int) ([]*model.SkillExecution, int64, error)
	Create(ctx context.Context, execution *model.SkillExecution) error
	Update(ctx context.Context, execution *model.SkillExecution) error
	Complete(ctx context.Context, id int64, status string, durationMs int) error
}

// SkillExecutionFacade implements SkillExecutionFacadeInterface
type SkillExecutionFacade struct {
	db *gorm.DB
}

// NewSkillExecutionFacade creates a new SkillExecutionFacade
func NewSkillExecutionFacade(db *gorm.DB) *SkillExecutionFacade {
	return &SkillExecutionFacade{db: db}
}

// GetByID retrieves an execution by ID
func (f *SkillExecutionFacade) GetByID(ctx context.Context, id int64) (*model.SkillExecution, error) {
	var execution model.SkillExecution
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&execution).Error
	if err != nil {
		return nil, err
	}
	return &execution, nil
}

// ListBySkillName retrieves executions for a skill
func (f *SkillExecutionFacade) ListBySkillName(ctx context.Context, skillName string, offset, limit int) ([]*model.SkillExecution, int64, error) {
	var executions []*model.SkillExecution
	var total int64

	query := f.db.WithContext(ctx).Model(&model.SkillExecution{}).Where("skill_name = ?", skillName)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&executions).Error
	if err != nil {
		return nil, 0, err
	}

	return executions, total, nil
}

// ListByUserID retrieves executions for a user
func (f *SkillExecutionFacade) ListByUserID(ctx context.Context, userID string, offset, limit int) ([]*model.SkillExecution, int64, error) {
	var executions []*model.SkillExecution
	var total int64

	query := f.db.WithContext(ctx).Model(&model.SkillExecution{}).Where("user_id = ?", userID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&executions).Error
	if err != nil {
		return nil, 0, err
	}

	return executions, total, nil
}

// Create creates a new execution
func (f *SkillExecutionFacade) Create(ctx context.Context, execution *model.SkillExecution) error {
	return f.db.WithContext(ctx).Create(execution).Error
}

// Update updates an execution
func (f *SkillExecutionFacade) Update(ctx context.Context, execution *model.SkillExecution) error {
	return f.db.WithContext(ctx).Save(execution).Error
}

// Complete marks an execution as completed
func (f *SkillExecutionFacade) Complete(ctx context.Context, id int64, status string, durationMs int) error {
	now := time.Now()
	return f.db.WithContext(ctx).Model(&model.SkillExecution{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"completed_at": now,
			"status":       status,
			"duration_ms":  durationMs,
		}).Error
}
