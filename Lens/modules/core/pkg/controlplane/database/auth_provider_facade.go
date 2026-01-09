// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// AuthProviderFacadeInterface defines AuthProvider database operations
type AuthProviderFacadeInterface interface {
	Create(ctx context.Context, provider *model.LensAuthProviders) error
	GetByID(ctx context.Context, id string) (*model.LensAuthProviders, error)
	GetByName(ctx context.Context, name string) (*model.LensAuthProviders, error)
	GetByType(ctx context.Context, providerType string) ([]*model.LensAuthProviders, error)
	Update(ctx context.Context, provider *model.LensAuthProviders) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*model.LensAuthProviders, error)
	ListEnabled(ctx context.Context) ([]*model.LensAuthProviders, error)
	UpdateStatus(ctx context.Context, id string, status string, lastError string) error
}

// AuthProviderFacade implements AuthProviderFacadeInterface
type AuthProviderFacade struct {
	BaseFacade
}

// NewAuthProviderFacade creates a new AuthProviderFacade
func NewAuthProviderFacade() *AuthProviderFacade {
	return &AuthProviderFacade{}
}

// Create creates a new auth provider
func (f *AuthProviderFacade) Create(ctx context.Context, provider *model.LensAuthProviders) error {
	return f.getDB().WithContext(ctx).Create(provider).Error
}

// GetByID gets an auth provider by ID
func (f *AuthProviderFacade) GetByID(ctx context.Context, id string) (*model.LensAuthProviders, error) {
	var provider model.LensAuthProviders
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetByName gets an auth provider by name
func (f *AuthProviderFacade) GetByName(ctx context.Context, name string) (*model.LensAuthProviders, error) {
	var provider model.LensAuthProviders
	err := f.getDB().WithContext(ctx).Where("name = ?", name).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetByType gets auth providers by type
func (f *AuthProviderFacade) GetByType(ctx context.Context, providerType string) ([]*model.LensAuthProviders, error) {
	var providers []*model.LensAuthProviders
	err := f.getDB().WithContext(ctx).
		Where("type = ?", providerType).
		Order("priority ASC").
		Find(&providers).Error
	if err != nil {
		return nil, err
	}
	return providers, nil
}

// Update updates an auth provider
func (f *AuthProviderFacade) Update(ctx context.Context, provider *model.LensAuthProviders) error {
	provider.UpdatedAt = time.Now()
	return f.getDB().WithContext(ctx).Save(provider).Error
}

// Delete deletes an auth provider by ID
func (f *AuthProviderFacade) Delete(ctx context.Context, id string) error {
	return f.getDB().WithContext(ctx).Where("id = ?", id).Delete(&model.LensAuthProviders{}).Error
}

// List lists all auth providers
func (f *AuthProviderFacade) List(ctx context.Context) ([]*model.LensAuthProviders, error) {
	var providers []*model.LensAuthProviders
	err := f.getDB().WithContext(ctx).
		Order("priority ASC").
		Find(&providers).Error
	if err != nil {
		return nil, err
	}
	return providers, nil
}

// ListEnabled lists all enabled auth providers
func (f *AuthProviderFacade) ListEnabled(ctx context.Context) ([]*model.LensAuthProviders, error) {
	var providers []*model.LensAuthProviders
	err := f.getDB().WithContext(ctx).
		Where("enabled = true").
		Order("priority ASC").
		Find(&providers).Error
	if err != nil {
		return nil, err
	}
	return providers, nil
}

// UpdateStatus updates the status of an auth provider
func (f *AuthProviderFacade) UpdateStatus(ctx context.Context, id string, status string, lastError string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.LensAuthProviders{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"last_error":    lastError,
			"last_check_at": now,
			"updated_at":    now,
		}).Error
}

// Ensure AuthProviderFacade implements AuthProviderFacadeInterface
var _ AuthProviderFacadeInterface = (*AuthProviderFacade)(nil)
