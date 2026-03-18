// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ReleaseVersionFacadeInterface defines the interface for release version operations
type ReleaseVersionFacadeInterface interface {
	Create(ctx context.Context, version *model.ReleaseVersion) error
	GetByID(ctx context.Context, id int32) (*model.ReleaseVersion, error)
	GetByName(ctx context.Context, name string) (*model.ReleaseVersion, error)
	List(ctx context.Context, channel, status string, limit int) ([]*model.ReleaseVersion, error)
	ListActive(ctx context.Context, channel string) ([]*model.ReleaseVersion, error)
	Update(ctx context.Context, version *model.ReleaseVersion) error
	UpdateStatus(ctx context.Context, id int32, status string) error
	Delete(ctx context.Context, id int32) error
	GetLatestByChannel(ctx context.Context, channel string) (*model.ReleaseVersion, error)
}

// ReleaseVersionFacade implements ReleaseVersionFacadeInterface
type ReleaseVersionFacade struct {
	db *gorm.DB
}

// NewReleaseVersionFacade creates a new ReleaseVersionFacade
func NewReleaseVersionFacade(db *gorm.DB) *ReleaseVersionFacade {
	return &ReleaseVersionFacade{db: db}
}

// Create creates a new release version
func (f *ReleaseVersionFacade) Create(ctx context.Context, version *model.ReleaseVersion) error {
	return f.db.WithContext(ctx).Create(version).Error
}

// GetByID gets a release version by ID
func (f *ReleaseVersionFacade) GetByID(ctx context.Context, id int32) (*model.ReleaseVersion, error) {
	var version model.ReleaseVersion
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// GetByName gets a release version by name
func (f *ReleaseVersionFacade) GetByName(ctx context.Context, name string) (*model.ReleaseVersion, error) {
	var version model.ReleaseVersion
	err := f.db.WithContext(ctx).Where("version_name = ?", name).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// List lists release versions with optional filters
func (f *ReleaseVersionFacade) List(ctx context.Context, channel, status string, limit int) ([]*model.ReleaseVersion, error) {
	var versions []*model.ReleaseVersion
	query := f.db.WithContext(ctx).Order("created_at DESC")

	if channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&versions).Error
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// ListActive lists active release versions
func (f *ReleaseVersionFacade) ListActive(ctx context.Context, channel string) ([]*model.ReleaseVersion, error) {
	return f.List(ctx, channel, model.ReleaseStatusActive, 0)
}

// Update updates a release version
func (f *ReleaseVersionFacade) Update(ctx context.Context, version *model.ReleaseVersion) error {
	version.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).Save(version).Error
}

// UpdateStatus updates the status of a release version
func (f *ReleaseVersionFacade) UpdateStatus(ctx context.Context, id int32, status string) error {
	return f.db.WithContext(ctx).
		Model(&model.ReleaseVersion{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// Delete deletes a release version
func (f *ReleaseVersionFacade) Delete(ctx context.Context, id int32) error {
	return f.db.WithContext(ctx).Delete(&model.ReleaseVersion{}, id).Error
}

// GetLatestByChannel gets the latest active version for a channel
func (f *ReleaseVersionFacade) GetLatestByChannel(ctx context.Context, channel string) (*model.ReleaseVersion, error) {
	var version model.ReleaseVersion
	err := f.db.WithContext(ctx).
		Where("channel = ? AND status = ?", channel, model.ReleaseStatusActive).
		Order("created_at DESC").
		First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}
