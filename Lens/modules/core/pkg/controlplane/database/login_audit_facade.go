// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// LoginAuditFacadeInterface defines LoginAudit database operations
type LoginAuditFacadeInterface interface {
	Create(ctx context.Context, audit *model.LensLoginAudit) error
	ListByUsername(ctx context.Context, username string, offset, limit int) ([]*model.LensLoginAudit, int64, error)
	ListByUserID(ctx context.Context, userID string, offset, limit int) ([]*model.LensLoginAudit, int64, error)
	ListRecent(ctx context.Context, limit int) ([]*model.LensLoginAudit, error)
	ListByTimeRange(ctx context.Context, start, end time.Time, offset, limit int) ([]*model.LensLoginAudit, int64, error)
	CleanupOld(ctx context.Context, before time.Time) (int64, error)
}

// LoginAuditFacade implements LoginAuditFacadeInterface
type LoginAuditFacade struct {
	BaseFacade
}

// NewLoginAuditFacade creates a new LoginAuditFacade
func NewLoginAuditFacade() *LoginAuditFacade {
	return &LoginAuditFacade{}
}

// Create creates a new login audit record
func (f *LoginAuditFacade) Create(ctx context.Context, audit *model.LensLoginAudit) error {
	return f.getDB().WithContext(ctx).Create(audit).Error
}

// ListByUsername lists login audit records by username with pagination
func (f *LoginAuditFacade) ListByUsername(ctx context.Context, username string, offset, limit int) ([]*model.LensLoginAudit, int64, error) {
	var audits []*model.LensLoginAudit
	var count int64

	db := f.getDB().WithContext(ctx).Model(&model.LensLoginAudit{}).Where("username = ?", username)

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	if err := db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&audits).Error; err != nil {
		return nil, 0, err
	}

	return audits, count, nil
}

// ListByUserID lists login audit records by user ID with pagination
func (f *LoginAuditFacade) ListByUserID(ctx context.Context, userID string, offset, limit int) ([]*model.LensLoginAudit, int64, error) {
	var audits []*model.LensLoginAudit
	var count int64

	db := f.getDB().WithContext(ctx).Model(&model.LensLoginAudit{}).Where("user_id = ?", userID)

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	if err := db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&audits).Error; err != nil {
		return nil, 0, err
	}

	return audits, count, nil
}

// ListRecent lists the most recent login audit records
func (f *LoginAuditFacade) ListRecent(ctx context.Context, limit int) ([]*model.LensLoginAudit, error) {
	var audits []*model.LensLoginAudit
	err := f.getDB().WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&audits).Error
	if err != nil {
		return nil, err
	}
	return audits, nil
}

// ListByTimeRange lists login audit records within a time range with pagination
func (f *LoginAuditFacade) ListByTimeRange(ctx context.Context, start, end time.Time, offset, limit int) ([]*model.LensLoginAudit, int64, error) {
	var audits []*model.LensLoginAudit
	var count int64

	db := f.getDB().WithContext(ctx).Model(&model.LensLoginAudit{}).
		Where("created_at >= ? AND created_at <= ?", start, end)

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	if err := db.Offset(offset).Limit(limit).Order("created_at DESC").Find(&audits).Error; err != nil {
		return nil, 0, err
	}

	return audits, count, nil
}

// CleanupOld removes old login audit records
func (f *LoginAuditFacade) CleanupOld(ctx context.Context, before time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("created_at < ?", before).
		Delete(&model.LensLoginAudit{})
	return result.RowsAffected, result.Error
}

// Ensure LoginAuditFacade implements LoginAuditFacadeInterface
var _ LoginAuditFacadeInterface = (*LoginAuditFacade)(nil)
