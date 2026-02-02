// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// WorkspaceFacadeInterface defines the interface for Workspace operations
type WorkspaceFacadeInterface interface {
	// Workspace CRUD
	GetByID(ctx context.Context, id int64) (*model.Workspace, error)
	GetByName(ctx context.Context, name string) (*model.Workspace, error)
	GetDefault(ctx context.Context) (*model.Workspace, error)
	List(ctx context.Context, offset, limit int) ([]*model.Workspace, int64, error)
	ListByOwner(ctx context.Context, owner string, offset, limit int) ([]*model.Workspace, int64, error)
	Create(ctx context.Context, workspace *model.Workspace) error
	Update(ctx context.Context, workspace *model.Workspace) error
	Delete(ctx context.Context, id int64) error
	SetDefault(ctx context.Context, id int64) error
}

// WorkspaceFacade implements WorkspaceFacadeInterface
type WorkspaceFacade struct {
	db *gorm.DB
}

// NewWorkspaceFacade creates a new WorkspaceFacade
func NewWorkspaceFacade(db *gorm.DB) *WorkspaceFacade {
	return &WorkspaceFacade{db: db}
}

// GetByID retrieves a workspace by ID
func (f *WorkspaceFacade) GetByID(ctx context.Context, id int64) (*model.Workspace, error) {
	var workspace model.Workspace
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// GetByName retrieves a workspace by name
func (f *WorkspaceFacade) GetByName(ctx context.Context, name string) (*model.Workspace, error) {
	var workspace model.Workspace
	err := f.db.WithContext(ctx).Where("name = ?", name).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// GetDefault retrieves the default workspace
func (f *WorkspaceFacade) GetDefault(ctx context.Context) (*model.Workspace, error) {
	var workspace model.Workspace
	err := f.db.WithContext(ctx).Where("is_default = ?", true).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// List retrieves paginated workspaces
func (f *WorkspaceFacade) List(ctx context.Context, offset, limit int) ([]*model.Workspace, int64, error) {
	var workspaces []*model.Workspace
	var total int64

	err := f.db.WithContext(ctx).Model(&model.Workspace{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = f.db.WithContext(ctx).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&workspaces).Error
	if err != nil {
		return nil, 0, err
	}

	return workspaces, total, nil
}

// ListByOwner retrieves workspaces by owner
func (f *WorkspaceFacade) ListByOwner(ctx context.Context, owner string, offset, limit int) ([]*model.Workspace, int64, error) {
	var workspaces []*model.Workspace
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Workspace{}).Where("owner = ?", owner)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&workspaces).Error
	if err != nil {
		return nil, 0, err
	}

	return workspaces, total, nil
}

// Create creates a new workspace
func (f *WorkspaceFacade) Create(ctx context.Context, workspace *model.Workspace) error {
	return f.db.WithContext(ctx).Create(workspace).Error
}

// Update updates an existing workspace
func (f *WorkspaceFacade) Update(ctx context.Context, workspace *model.Workspace) error {
	workspace.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(workspace).Error
}

// Delete deletes a workspace by ID
func (f *WorkspaceFacade) Delete(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Delete(&model.Workspace{}, id).Error
}

// SetDefault sets a workspace as the default and unsets others
func (f *WorkspaceFacade) SetDefault(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Unset all existing defaults
		if err := tx.Model(&model.Workspace{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		// Set the new default
		return tx.Model(&model.Workspace{}).Where("id = ?", id).Update("is_default", true).Error
	})
}
