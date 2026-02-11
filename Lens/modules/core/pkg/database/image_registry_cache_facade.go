// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImageRegistryCacheFacadeInterface defines the database operation interface for image registry cache
type ImageRegistryCacheFacadeInterface interface {
	// Create creates a new image registry cache entry
	Create(ctx context.Context, entry *model.ImageRegistryCache) error

	// Upsert creates or updates a cache entry by digest
	Upsert(ctx context.Context, entry *model.ImageRegistryCache) error

	// GetByDigest retrieves a cache entry by image digest
	GetByDigest(ctx context.Context, digest string) (*model.ImageRegistryCache, error)

	// GetByTagRef retrieves a cache entry by registry, repository, and tag
	GetByTagRef(ctx context.Context, registry, repository, tag string) (*model.ImageRegistryCache, error)

	// Delete deletes a cache entry by ID
	Delete(ctx context.Context, id int64) error

	// GetExpired retrieves cache entries with expired tag-based lookups
	GetExpired(ctx context.Context, limit int) ([]*model.ImageRegistryCache, error)

	// UpdateExpiresAt updates the expiration time for a cache entry
	UpdateExpiresAt(ctx context.Context, id int64, expiresAt time.Time) error

	// DeleteOrphaned deletes cache entries older than the given time that have no workload references
	DeleteOrphaned(ctx context.Context, olderThan time.Time) (int64, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) ImageRegistryCacheFacadeInterface
}

// ImageRegistryCacheFacade implements ImageRegistryCacheFacadeInterface
type ImageRegistryCacheFacade struct {
	BaseFacade
}

// NewImageRegistryCacheFacade creates a new ImageRegistryCacheFacade instance
func NewImageRegistryCacheFacade() ImageRegistryCacheFacadeInterface {
	return &ImageRegistryCacheFacade{}
}

func (f *ImageRegistryCacheFacade) WithCluster(clusterName string) ImageRegistryCacheFacadeInterface {
	return &ImageRegistryCacheFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

func (f *ImageRegistryCacheFacade) Create(ctx context.Context, entry *model.ImageRegistryCache) error {
	now := time.Now()
	if entry.CachedAt.IsZero() {
		entry.CachedAt = now
	}
	return f.getDB().WithContext(ctx).Create(entry).Error
}

func (f *ImageRegistryCacheFacade) Upsert(ctx context.Context, entry *model.ImageRegistryCache) error {
	now := time.Now()
	if entry.CachedAt.IsZero() {
		entry.CachedAt = now
	}

	return f.getDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "digest"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"image_ref", "registry", "repository", "tag",
				"base_image", "layer_count", "layer_history",
				"image_labels", "image_env", "image_entrypoint",
				"installed_packages", "framework_hints",
				"total_size", "image_created_at", "expires_at",
			}),
		}).
		Create(entry).Error
}

func (f *ImageRegistryCacheFacade) GetByDigest(ctx context.Context, digest string) (*model.ImageRegistryCache, error) {
	var result model.ImageRegistryCache
	err := f.getDB().WithContext(ctx).
		Where("digest = ?", digest).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (f *ImageRegistryCacheFacade) GetByTagRef(ctx context.Context, registry, repository, tag string) (*model.ImageRegistryCache, error) {
	var result model.ImageRegistryCache
	err := f.getDB().WithContext(ctx).
		Where("registry = ? AND repository = ? AND tag = ?", registry, repository, tag).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (f *ImageRegistryCacheFacade) Delete(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.ImageRegistryCache{}).Error
}

func (f *ImageRegistryCacheFacade) GetExpired(ctx context.Context, limit int) ([]*model.ImageRegistryCache, error) {
	var results []*model.ImageRegistryCache
	err := f.getDB().WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Limit(limit).
		Find(&results).Error
	return results, err
}

func (f *ImageRegistryCacheFacade) UpdateExpiresAt(ctx context.Context, id int64, expiresAt time.Time) error {
	return f.getDB().WithContext(ctx).
		Table(model.TableNameImageRegistryCache).
		Where("id = ?", id).
		UpdateColumn("expires_at", expiresAt).Error
}

func (f *ImageRegistryCacheFacade) DeleteOrphaned(ctx context.Context, olderThan time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("cached_at < ?", olderThan).
		Delete(&model.ImageRegistryCache{})
	return result.RowsAffected, result.Error
}
