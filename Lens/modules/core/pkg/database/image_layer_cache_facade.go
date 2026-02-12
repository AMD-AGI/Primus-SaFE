// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// ImageLayerCacheFacadeInterface defines the database operation interface for image layer cache
type ImageLayerCacheFacadeInterface interface {
	// Create creates a new image layer cache entry
	Create(ctx context.Context, entry *model.ImageLayerCache) error

	// GetByDigest retrieves a layer cache entry by digest
	GetByDigest(ctx context.Context, digest string) (*model.ImageLayerCache, error)

	// GetByDigests retrieves multiple layer cache entries by digests
	GetByDigests(ctx context.Context, digests []string) ([]*model.ImageLayerCache, error)

	// Upsert creates or updates a layer cache entry by digest
	Upsert(ctx context.Context, entry *model.ImageLayerCache) error

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) ImageLayerCacheFacadeInterface
}

// ImageLayerCacheFacade implements ImageLayerCacheFacadeInterface
type ImageLayerCacheFacade struct {
	BaseFacade
}

// NewImageLayerCacheFacade creates a new ImageLayerCacheFacade instance
func NewImageLayerCacheFacade() ImageLayerCacheFacadeInterface {
	return &ImageLayerCacheFacade{}
}

func (f *ImageLayerCacheFacade) WithCluster(clusterName string) ImageLayerCacheFacadeInterface {
	return &ImageLayerCacheFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

func (f *ImageLayerCacheFacade) Create(ctx context.Context, entry *model.ImageLayerCache) error {
	return f.getDB().WithContext(ctx).Create(entry).Error
}

func (f *ImageLayerCacheFacade) GetByDigest(ctx context.Context, digest string) (*model.ImageLayerCache, error) {
	var result model.ImageLayerCache
	err := f.getDB().WithContext(ctx).
		Where("layer_digest = ?", digest).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (f *ImageLayerCacheFacade) GetByDigests(ctx context.Context, digests []string) ([]*model.ImageLayerCache, error) {
	if len(digests) == 0 {
		return nil, nil
	}
	var results []*model.ImageLayerCache
	err := f.getDB().WithContext(ctx).
		Where("layer_digest IN ?", digests).
		Find(&results).Error
	return results, err
}

func (f *ImageLayerCacheFacade) Upsert(ctx context.Context, entry *model.ImageLayerCache) error {
	// Try to find existing entry
	existing, err := f.GetByDigest(ctx, entry.LayerDigest)
	if err != nil {
		return err
	}
	if existing != nil {
		// Update existing
		return f.getDB().WithContext(ctx).
			Model(&model.ImageLayerCache{}).
			Where("layer_digest = ?", entry.LayerDigest).
			Updates(map[string]interface{}{
				"compressed_size":  entry.CompressedSize,
				"media_type":       entry.MediaType,
				"file_count":       entry.FileCount,
				"packages":         entry.Packages,
				"framework_hints":  entry.FrameworkHints,
				"notable_paths":    entry.NotablePaths,
				"analyzed_at":      entry.AnalyzedAt,
			}).Error
	}
	return f.Create(ctx, entry)
}
