/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"

	"gorm.io/gorm/clause"
)

// UpsertModel creates or updates a Model record.
func (c *Client) UpsertModel(ctx context.Context, model *Model) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}

	// Use GORM's Save or Clauses to handle Upsert (On Conflict Update)
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(model).Error
}
