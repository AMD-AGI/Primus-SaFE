// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ToolFacadeInterface defines the interface for Tool operations
type ToolFacadeInterface interface {
	// Tool CRUD
	GetByID(ctx context.Context, id int64) (*model.Tool, error)
	GetByName(ctx context.Context, name string) (*model.Tool, error)
	List(ctx context.Context, offset, limit int) ([]*model.Tool, int64, error)
	ListByCategory(ctx context.Context, category string, offset, limit int) ([]*model.Tool, int64, error)
	ListByDomain(ctx context.Context, domain string, offset, limit int) ([]*model.Tool, int64, error)
	ListByScope(ctx context.Context, scope string, offset, limit int) ([]*model.Tool, int64, error)
	ListByStatus(ctx context.Context, status string, offset, limit int) ([]*model.Tool, int64, error)
	Create(ctx context.Context, tool *model.Tool) error
	Update(ctx context.Context, tool *model.Tool) error
	Delete(ctx context.Context, id int64) error
	Upsert(ctx context.Context, tool *model.Tool) error
}

// ToolFacade implements ToolFacadeInterface
type ToolFacade struct {
	db *gorm.DB
}

// NewToolFacade creates a new ToolFacade
func NewToolFacade(db *gorm.DB) *ToolFacade {
	return &ToolFacade{db: db}
}

// GetByID retrieves a tool by ID
func (f *ToolFacade) GetByID(ctx context.Context, id int64) (*model.Tool, error) {
	var tool model.Tool
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&tool).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// GetByName retrieves a tool by name
func (f *ToolFacade) GetByName(ctx context.Context, name string) (*model.Tool, error) {
	var tool model.Tool
	err := f.db.WithContext(ctx).Where("name = ?", name).First(&tool).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// List retrieves paginated tools
func (f *ToolFacade) List(ctx context.Context, offset, limit int) ([]*model.Tool, int64, error) {
	var tools []*model.Tool
	var total int64

	err := f.db.WithContext(ctx).Model(&model.Tool{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = f.db.WithContext(ctx).
		Order("registered_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tools).Error
	if err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// ListByCategory retrieves tools by category
func (f *ToolFacade) ListByCategory(ctx context.Context, category string, offset, limit int) ([]*model.Tool, int64, error) {
	var tools []*model.Tool
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Tool{}).Where("category = ?", category)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("registered_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tools).Error
	if err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// ListByDomain retrieves tools by domain
func (f *ToolFacade) ListByDomain(ctx context.Context, domain string, offset, limit int) ([]*model.Tool, int64, error) {
	var tools []*model.Tool
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Tool{}).Where("domain = ?", domain)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("registered_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tools).Error
	if err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// ListByScope retrieves tools by access scope
func (f *ToolFacade) ListByScope(ctx context.Context, scope string, offset, limit int) ([]*model.Tool, int64, error) {
	var tools []*model.Tool
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Tool{}).Where("access_scope = ?", scope)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("registered_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tools).Error
	if err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// ListByStatus retrieves tools by status
func (f *ToolFacade) ListByStatus(ctx context.Context, status string, offset, limit int) ([]*model.Tool, int64, error) {
	var tools []*model.Tool
	var total int64

	query := f.db.WithContext(ctx).Model(&model.Tool{}).Where("status = ?", status)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("registered_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tools).Error
	if err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// Create creates a new tool
func (f *ToolFacade) Create(ctx context.Context, tool *model.Tool) error {
	return f.db.WithContext(ctx).Create(tool).Error
}

// Update updates an existing tool
func (f *ToolFacade) Update(ctx context.Context, tool *model.Tool) error {
	tool.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(tool).Error
}

// Delete deletes a tool by ID
func (f *ToolFacade) Delete(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Delete(&model.Tool{}, id).Error
}

// Upsert creates or updates a tool based on name
func (f *ToolFacade) Upsert(ctx context.Context, tool *model.Tool) error {
	existing, err := f.GetByName(ctx, tool.Name)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return f.Create(ctx, tool)
		}
		return err
	}
	tool.ID = existing.ID
	tool.RegisteredAt = existing.RegisteredAt
	return f.Update(ctx, tool)
}
