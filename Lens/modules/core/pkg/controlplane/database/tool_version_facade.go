// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ToolVersionFacadeInterface defines the interface for ToolVersion operations
type ToolVersionFacadeInterface interface {
	GetByID(ctx context.Context, id int64) (*model.ToolVersion, error)
	GetByToolID(ctx context.Context, toolID int64) ([]*model.ToolVersion, error)
	GetByToolIDAndVersion(ctx context.Context, toolID int64, version string) (*model.ToolVersion, error)
	GetLatestByToolID(ctx context.Context, toolID int64) (*model.ToolVersion, error)
	Create(ctx context.Context, version *model.ToolVersion) error
	DeleteByToolID(ctx context.Context, toolID int64) error
}

// ToolVersionFacade implements ToolVersionFacadeInterface
type ToolVersionFacade struct {
	db *gorm.DB
}

// NewToolVersionFacade creates a new ToolVersionFacade
func NewToolVersionFacade(db *gorm.DB) *ToolVersionFacade {
	return &ToolVersionFacade{db: db}
}

// GetByID retrieves a tool version by ID
func (f *ToolVersionFacade) GetByID(ctx context.Context, id int64) (*model.ToolVersion, error) {
	var version model.ToolVersion
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// GetByToolID retrieves all versions for a tool
func (f *ToolVersionFacade) GetByToolID(ctx context.Context, toolID int64) ([]*model.ToolVersion, error) {
	var versions []*model.ToolVersion
	err := f.db.WithContext(ctx).
		Where("tool_id = ?", toolID).
		Order("created_at DESC").
		Find(&versions).Error
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetByToolIDAndVersion retrieves a specific version for a tool
func (f *ToolVersionFacade) GetByToolIDAndVersion(ctx context.Context, toolID int64, version string) (*model.ToolVersion, error) {
	var ver model.ToolVersion
	err := f.db.WithContext(ctx).
		Where("tool_id = ? AND version = ?", toolID, version).
		First(&ver).Error
	if err != nil {
		return nil, err
	}
	return &ver, nil
}

// GetLatestByToolID retrieves the latest version for a tool
func (f *ToolVersionFacade) GetLatestByToolID(ctx context.Context, toolID int64) (*model.ToolVersion, error) {
	var version model.ToolVersion
	err := f.db.WithContext(ctx).
		Where("tool_id = ?", toolID).
		Order("created_at DESC").
		First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// Create creates a new tool version
func (f *ToolVersionFacade) Create(ctx context.Context, version *model.ToolVersion) error {
	return f.db.WithContext(ctx).Create(version).Error
}

// DeleteByToolID deletes all versions for a tool
func (f *ToolVersionFacade) DeleteByToolID(ctx context.Context, toolID int64) error {
	return f.db.WithContext(ctx).Where("tool_id = ?", toolID).Delete(&model.ToolVersion{}).Error
}
