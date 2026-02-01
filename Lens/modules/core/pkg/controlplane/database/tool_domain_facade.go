// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ToolDomainFacadeInterface defines the interface for ToolDomain operations
type ToolDomainFacadeInterface interface {
	GetByDomain(ctx context.Context, domain string) (*model.ToolDomain, error)
	List(ctx context.Context) ([]*model.ToolDomain, error)
	Create(ctx context.Context, toolDomain *model.ToolDomain) error
	Update(ctx context.Context, toolDomain *model.ToolDomain) error
	Delete(ctx context.Context, domain string) error
	Upsert(ctx context.Context, toolDomain *model.ToolDomain) error
	AddToolToDomain(ctx context.Context, domain, toolName string) error
	RemoveToolFromDomain(ctx context.Context, domain, toolName string) error
}

// ToolDomainFacade implements ToolDomainFacadeInterface
type ToolDomainFacade struct {
	db *gorm.DB
}

// NewToolDomainFacade creates a new ToolDomainFacade
func NewToolDomainFacade(db *gorm.DB) *ToolDomainFacade {
	return &ToolDomainFacade{db: db}
}

// GetByDomain retrieves a tool domain by domain name
func (f *ToolDomainFacade) GetByDomain(ctx context.Context, domain string) (*model.ToolDomain, error) {
	var toolDomain model.ToolDomain
	err := f.db.WithContext(ctx).Where("domain = ?", domain).First(&toolDomain).Error
	if err != nil {
		return nil, err
	}
	return &toolDomain, nil
}

// List retrieves all tool domains
func (f *ToolDomainFacade) List(ctx context.Context) ([]*model.ToolDomain, error) {
	var domains []*model.ToolDomain
	err := f.db.WithContext(ctx).Order("domain ASC").Find(&domains).Error
	if err != nil {
		return nil, err
	}
	return domains, nil
}

// Create creates a new tool domain
func (f *ToolDomainFacade) Create(ctx context.Context, toolDomain *model.ToolDomain) error {
	return f.db.WithContext(ctx).Create(toolDomain).Error
}

// Update updates an existing tool domain
func (f *ToolDomainFacade) Update(ctx context.Context, toolDomain *model.ToolDomain) error {
	toolDomain.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(toolDomain).Error
}

// Delete deletes a tool domain by domain name
func (f *ToolDomainFacade) Delete(ctx context.Context, domain string) error {
	return f.db.WithContext(ctx).Where("domain = ?", domain).Delete(&model.ToolDomain{}).Error
}

// Upsert creates or updates a tool domain
func (f *ToolDomainFacade) Upsert(ctx context.Context, toolDomain *model.ToolDomain) error {
	toolDomain.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain"}},
			DoUpdates: clause.AssignmentColumns([]string{"description", "tool_names", "updated_at"}),
		}).
		Create(toolDomain).Error
}

// AddToolToDomain adds a tool to a domain
func (f *ToolDomainFacade) AddToolToDomain(ctx context.Context, domain, toolName string) error {
	toolDomain, err := f.GetByDomain(ctx, domain)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new domain with the tool
			toolDomain = &model.ToolDomain{
				Domain:    domain,
				ToolNames: model.Strings{toolName},
			}
			return f.Create(ctx, toolDomain)
		}
		return err
	}

	// Check if tool already exists in domain
	for _, name := range toolDomain.ToolNames {
		if name == toolName {
			return nil // Already exists
		}
	}

	// Add tool to domain
	toolDomain.ToolNames = append(toolDomain.ToolNames, toolName)
	return f.Update(ctx, toolDomain)
}

// RemoveToolFromDomain removes a tool from a domain
func (f *ToolDomainFacade) RemoveToolFromDomain(ctx context.Context, domain, toolName string) error {
	toolDomain, err := f.GetByDomain(ctx, domain)
	if err != nil {
		return err
	}

	// Find and remove the tool
	newToolNames := make(model.Strings, 0, len(toolDomain.ToolNames))
	for _, name := range toolDomain.ToolNames {
		if name != toolName {
			newToolNames = append(newToolNames, name)
		}
	}

	toolDomain.ToolNames = newToolNames
	return f.Update(ctx, toolDomain)
}
