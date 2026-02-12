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

	// GetByImageRef retrieves a cache entry by full image reference
	GetByImageRef(ctx context.Context, imageRef string) (*model.ImageRegistryCache, error)

	// Delete deletes a cache entry by ID
	Delete(ctx context.Context, id int64) error

	// GetExpired retrieves cache entries with expired tag-based lookups
	GetExpired(ctx context.Context, limit int) ([]*model.ImageRegistryCache, error)

	// UpdateExpiresAt updates the expiration time for a cache entry
	UpdateExpiresAt(ctx context.Context, id int64, expiresAt time.Time) error

	// DeleteOrphaned deletes cache entries older than the given time that have no workload references
	DeleteOrphaned(ctx context.Context, olderThan time.Time) (int64, error)

	// List retrieves cached images with optional registry/repository filtering and pagination
	List(ctx context.Context, registry, repository string, limit, offset int) ([]*model.ImageRegistryCache, int64, error)

	// GetPending retrieves pending analysis requests with FOR UPDATE SKIP LOCKED
	GetPending(ctx context.Context, limit int) ([]*model.ImageRegistryCache, error)

	// UpdateStatus updates the analysis status and optional error message
	UpdateStatus(ctx context.Context, id int64, status string, errorMessage string) error

	// UpsertPending creates a pending analysis request or returns existing entry
	UpsertPending(ctx context.Context, imageRef, namespace string) (*model.ImageRegistryCache, error)

	// UpdateAnalysisResult updates an existing record by ID with analysis results
	UpdateAnalysisResult(ctx context.Context, entry *model.ImageRegistryCache) error

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

func (f *ImageRegistryCacheFacade) List(ctx context.Context, registry, repository string, limit, offset int) ([]*model.ImageRegistryCache, int64, error) {
	db := f.getDB().WithContext(ctx).Model(&model.ImageRegistryCache{})

	if registry != "" {
		db = db.Where("registry = ?", registry)
	}
	if repository != "" {
		db = db.Where("repository LIKE ?", "%"+repository+"%")
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var results []*model.ImageRegistryCache
	err := db.Order("cached_at DESC").Limit(limit).Offset(offset).Find(&results).Error
	return results, total, err
}

func (f *ImageRegistryCacheFacade) DeleteOrphaned(ctx context.Context, olderThan time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("cached_at < ?", olderThan).
		Delete(&model.ImageRegistryCache{})
	return result.RowsAffected, result.Error
}

func (f *ImageRegistryCacheFacade) GetByImageRef(ctx context.Context, imageRef string) (*model.ImageRegistryCache, error) {
	var result model.ImageRegistryCache
	err := f.getDB().WithContext(ctx).
		Where("image_ref = ?", imageRef).
		Order("cached_at DESC").
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (f *ImageRegistryCacheFacade) GetPending(ctx context.Context, limit int) ([]*model.ImageRegistryCache, error) {
	var results []*model.ImageRegistryCache
	err := f.getDB().WithContext(ctx).
		Raw(`SELECT * FROM image_registry_cache
			WHERE status = 'pending'
			ORDER BY cached_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED`, limit).
		Scan(&results).Error
	return results, err
}

func (f *ImageRegistryCacheFacade) UpdateStatus(ctx context.Context, id int64, status string, errorMessage string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if errorMessage != "" {
		updates["error_message"] = errorMessage
	}
	if status == "completed" || status == "failed" {
		now := time.Now()
		updates["analyzed_at"] = now
	}
	return f.getDB().WithContext(ctx).
		Table(model.TableNameImageRegistryCache).
		Where("id = ?", id).
		Updates(updates).Error
}

func (f *ImageRegistryCacheFacade) UpsertPending(ctx context.Context, imageRef, namespace string) (*model.ImageRegistryCache, error) {
	// Check if there is already a non-failed entry for this image
	existing, err := f.GetByImageRef(ctx, imageRef)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status != "failed" {
		return existing, nil
	}

	now := time.Now()
	entry := &model.ImageRegistryCache{
		ImageRef:  imageRef,
		Digest:    "",
		Status:    "pending",
		Namespace: namespace,
		CachedAt:  now,
	}

	// Parse image ref into components
	parts := splitImageRef(imageRef)
	entry.Registry = parts[0]
	entry.Repository = parts[1]
	entry.Tag = parts[2]

	if err := f.getDB().WithContext(ctx).Create(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

func (f *ImageRegistryCacheFacade) UpdateAnalysisResult(ctx context.Context, entry *model.ImageRegistryCache) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Table(model.TableNameImageRegistryCache).
		Where("id = ?", entry.ID).
		Updates(map[string]interface{}{
			"digest":             entry.Digest,
			"base_image":         entry.BaseImage,
			"layer_count":        entry.LayerCount,
			"layer_history":      entry.LayerHistory,
			"image_labels":       entry.ImageLabels,
			"image_env":          entry.ImageEnv,
			"image_entrypoint":   entry.ImageEntrypoint,
			"installed_packages": entry.InstalledPackages,
			"framework_hints":    entry.FrameworkHints,
			"total_size":         entry.TotalSize,
			"image_created_at":   entry.ImageCreatedAt,
			"expires_at":         entry.ExpiresAt,
			"status":             "completed",
			"analyzed_at":        now,
		}).Error
}

// splitImageRef splits "registry.example.com/repo/name:tag" into [registry, repository, tag]
func splitImageRef(imageRef string) [3]string {
	var result [3]string
	result[2] = "latest"

	ref := imageRef

	// Split off tag
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == ':' {
			afterColon := ref[i+1:]
			// Make sure it is not a port
			if len(afterColon) > 0 && !containsSlash(afterColon) {
				result[2] = afterColon
				ref = ref[:i]
			}
			break
		}
		if ref[i] == '/' {
			break
		}
	}

	// Split registry from repository
	slashIdx := -1
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' {
			slashIdx = i
			break
		}
	}
	if slashIdx > 0 && (containsDot(ref[:slashIdx]) || containsColon(ref[:slashIdx])) {
		result[0] = ref[:slashIdx]
		result[1] = ref[slashIdx+1:]
	} else {
		result[1] = ref
	}
	return result
}

func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

func containsDot(s string) bool {
	for _, c := range s {
		if c == '.' {
			return true
		}
	}
	return false
}

func containsColon(s string) bool {
	for _, c := range s {
		if c == ':' {
			return true
		}
	}
	return false
}
