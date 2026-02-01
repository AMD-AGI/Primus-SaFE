// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ToolInvocationFacadeInterface defines the interface for ToolInvocation operations
type ToolInvocationFacadeInterface interface {
	GetByID(ctx context.Context, id int64) (*model.ToolInvocation, error)
	GetByToolName(ctx context.Context, toolName string, offset, limit int) ([]*model.ToolInvocation, int64, error)
	GetByUserID(ctx context.Context, userID string, offset, limit int) ([]*model.ToolInvocation, int64, error)
	GetByToolNameAndTimeRange(ctx context.Context, toolName string, startTime, endTime time.Time, offset, limit int) ([]*model.ToolInvocation, int64, error)
	Create(ctx context.Context, invocation *model.ToolInvocation) error
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// ToolInvocationFacade implements ToolInvocationFacadeInterface
type ToolInvocationFacade struct {
	db *gorm.DB
}

// NewToolInvocationFacade creates a new ToolInvocationFacade
func NewToolInvocationFacade(db *gorm.DB) *ToolInvocationFacade {
	return &ToolInvocationFacade{db: db}
}

// GetByID retrieves a tool invocation by ID
func (f *ToolInvocationFacade) GetByID(ctx context.Context, id int64) (*model.ToolInvocation, error) {
	var invocation model.ToolInvocation
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&invocation).Error
	if err != nil {
		return nil, err
	}
	return &invocation, nil
}

// GetByToolName retrieves invocations for a tool
func (f *ToolInvocationFacade) GetByToolName(ctx context.Context, toolName string, offset, limit int) ([]*model.ToolInvocation, int64, error) {
	var invocations []*model.ToolInvocation
	var total int64

	query := f.db.WithContext(ctx).Model(&model.ToolInvocation{}).Where("tool_name = ?", toolName)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("invoked_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&invocations).Error
	if err != nil {
		return nil, 0, err
	}

	return invocations, total, nil
}

// GetByUserID retrieves invocations by user ID
func (f *ToolInvocationFacade) GetByUserID(ctx context.Context, userID string, offset, limit int) ([]*model.ToolInvocation, int64, error) {
	var invocations []*model.ToolInvocation
	var total int64

	query := f.db.WithContext(ctx).Model(&model.ToolInvocation{}).Where("user_id = ?", userID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("invoked_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&invocations).Error
	if err != nil {
		return nil, 0, err
	}

	return invocations, total, nil
}

// GetByToolNameAndTimeRange retrieves invocations for a tool within a time range
func (f *ToolInvocationFacade) GetByToolNameAndTimeRange(ctx context.Context, toolName string, startTime, endTime time.Time, offset, limit int) ([]*model.ToolInvocation, int64, error) {
	var invocations []*model.ToolInvocation
	var total int64

	query := f.db.WithContext(ctx).Model(&model.ToolInvocation{}).
		Where("tool_name = ? AND invoked_at >= ? AND invoked_at <= ?", toolName, startTime, endTime)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("invoked_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&invocations).Error
	if err != nil {
		return nil, 0, err
	}

	return invocations, total, nil
}

// Create creates a new tool invocation record
func (f *ToolInvocationFacade) Create(ctx context.Context, invocation *model.ToolInvocation) error {
	return f.db.WithContext(ctx).Create(invocation).Error
}

// DeleteOlderThan deletes invocation records older than the specified time
func (f *ToolInvocationFacade) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result := f.db.WithContext(ctx).Where("invoked_at < ?", before).Delete(&model.ToolInvocation{})
	return result.RowsAffected, result.Error
}
