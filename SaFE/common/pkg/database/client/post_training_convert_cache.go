/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPostTrainingConvertCache = "post_training_convert_cache"

	// ConvertCacheConverting, ConvertCacheReady and ConvertCacheInvalid are the states of a convert cache: a
	// conversion is running, the converted weights are complete, or they are gone and must be converted again.
	ConvertCacheConverting = "converting"
	ConvertCacheReady      = "ready"
	ConvertCacheInvalid    = "invalid"
)

// PostTrainingConvertCache is one conversion of a model's weights that the runs of a workspace share. The cache
// key is the framework, the spec, the model id and the framework commit.
type PostTrainingConvertCache struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	Workspace string `gorm:"column:workspace" json:"workspace" db:"workspace"`
	Framework string `gorm:"column:framework;default:slime" json:"framework" db:"framework"`
	CacheKey  string `gorm:"column:cache_key" json:"cacheKey" db:"cache_key"`
	// ModelID is the source model; the cache is reclaimed once it is deleted.
	ModelID string `gorm:"column:model_id" json:"modelId" db:"model_id"`
	// Path is the cache directory, which holds torch_dist/, fp8_hf/ and source.json.
	Path     string `gorm:"column:path" json:"path" db:"path"`
	Status   string `gorm:"column:status" json:"status" db:"status"`
	FP8Ready bool   `gorm:"column:fp8_ready" json:"fp8Ready" db:"fp8_ready"`
	// WorkloadID is the convert workload of a cache being converted in its own workload.
	WorkloadID string      `gorm:"column:workload_id" json:"workloadId,omitempty" db:"workload_id"`
	SizeBytes  int64       `gorm:"column:size_bytes" json:"sizeBytes,omitempty" db:"size_bytes"`
	CreatedAt  pq.NullTime `gorm:"column:created_at;autoCreateTime" json:"createdAt" db:"created_at"`
	UpdatedAt  pq.NullTime `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt" db:"updated_at"`
}

// TableName sets the SQL table name for PostTrainingConvertCache.
func (PostTrainingConvertCache) TableName() string {
	return TPostTrainingConvertCache
}

// PostTrainingConvertCacheFilter selects caches for ListPostTrainingConvertCaches. Zero-valued fields are ignored.
type PostTrainingConvertCacheFilter struct {
	Workspace string
	ModelID   string
	Status    string
}

// GetPostTrainingConvertCache fetches the cache of a key in a workspace. It returns gorm.ErrRecordNotFound
// when there is none.
func (c *Client) GetPostTrainingConvertCache(ctx context.Context, workspace, cacheKey string) (*PostTrainingConvertCache, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var cache PostTrainingConvertCache
	if err = db.WithContext(ctx).Where("workspace = ? AND cache_key = ?", workspace, cacheKey).First(&cache).Error; err != nil {
		return nil, err
	}
	return &cache, nil
}

// UpsertPostTrainingConvertCache records a cache; a cache already recorded for the same workspace and key is
// overwritten, so a repeated registration is harmless.
func (c *Client) UpsertPostTrainingConvertCache(ctx context.Context, cache *PostTrainingConvertCache) error {
	if cache == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace"}, {Name: "cache_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"framework", "model_id", "path", "status", "fp8_ready", "workload_id", "size_bytes", "updated_at"}),
	}).Create(cache).Error
}

// ListPostTrainingConvertCaches returns the caches matching filter, oldest first.
func (c *Client) ListPostTrainingConvertCaches(ctx context.Context, filter PostTrainingConvertCacheFilter) (
	[]*PostTrainingConvertCache, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	query := db.WithContext(ctx).Model(&PostTrainingConvertCache{})
	if filter.Workspace != "" {
		query = query.Where("workspace = ?", filter.Workspace)
	}
	if filter.ModelID != "" {
		query = query.Where("model_id = ?", filter.ModelID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var caches []*PostTrainingConvertCache
	if err = query.Order("id ASC").Find(&caches).Error; err != nil {
		return nil, err
	}
	return caches, nil
}

// UpdatePostTrainingConvertCache sets the given columns on a cache. It returns gorm.ErrRecordNotFound when no
// such cache exists.
func (c *Client) UpdatePostTrainingConvertCache(ctx context.Context, id int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	res := db.WithContext(ctx).Model(&PostTrainingConvertCache{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeletePostTrainingConvertCache removes a cache record.
func (c *Client) DeletePostTrainingConvertCache(ctx context.Context, id int64) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Where("id = ?", id).Delete(&PostTrainingConvertCache{}).Error
}
