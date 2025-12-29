/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// UpsertModel creates or updates a Model record.
func (c *Client) UpsertModel(ctx context.Context, model *Model) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}

	// Use GORM's Clauses to handle Upsert (On Conflict Update)
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(model).Error
}

// GetModelByID retrieves a Model by its ID.
func (c *Client) GetModelByID(ctx context.Context, id string) (*Model, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}

	var model Model
	result := db.WithContext(ctx).Where("id = ? AND is_deleted = ?", id, false).First(&model)
	if result.Error != nil {
		return nil, result.Error
	}
	return &model, nil
}

// ListModels retrieves all models matching the filter criteria.
// If accessMode is empty, all access modes are returned.
// If workspace is empty, all workspaces are returned.
func (c *Client) ListModels(ctx context.Context, accessMode string, workspace string, isDeleted bool) ([]*Model, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}

	query := db.WithContext(ctx).Where("is_deleted = ?", isDeleted)

	if accessMode != "" {
		query = query.Where("access_mode = ?", accessMode)
	}

	if workspace != "" {
		// Filter by specific workspace OR public models (empty workspace)
		query = query.Where("workspace = ? OR workspace = ''", workspace)
	}

	var models []*Model
	result := query.Order("created_at DESC").Find(&models)
	if result.Error != nil {
		return nil, result.Error
	}
	return models, nil
}

// DeleteModel soft deletes a Model by setting is_deleted to true.
func (c *Client) DeleteModel(ctx context.Context, id string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}

	now := time.Now()
	return db.WithContext(ctx).Model(&Model{}).Where("id = ?", id).Updates(map[string]interface{}{
		"is_deleted":    true,
		"deletion_time": now,
	}).Error
}
