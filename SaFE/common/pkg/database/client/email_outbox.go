/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func (c *Client) CreateEmailOutbox(ctx context.Context, outbox *model.EmailOutbox) error {
	return c.gorm.WithContext(ctx).Create(outbox).Error
}

func (c *Client) ListPendingEmailOutbox(ctx context.Context, limit int) ([]*model.EmailOutbox, error) {
	var results []*model.EmailOutbox
	query := c.gorm.WithContext(ctx).
		Where("status = ?", model.EmailOutboxStatusPending).
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&results).Error
	return results, err
}

func (c *Client) ListPendingEmailOutboxAfter(ctx context.Context, afterID int32, limit int) ([]*model.EmailOutbox, error) {
	var results []*model.EmailOutbox
	query := c.gorm.WithContext(ctx).
		Where("status = ? AND id > ?", model.EmailOutboxStatusPending, afterID).
		Order("id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&results).Error
	return results, err
}

func (c *Client) AckEmailOutbox(ctx context.Context, id int32) error {
	now := time.Now()
	return c.gorm.WithContext(ctx).
		Model(&model.EmailOutbox{}).
		Where("id = ? AND status = ?", id, model.EmailOutboxStatusPending).
		Updates(map[string]interface{}{
			"status":  model.EmailOutboxStatusSent,
			"sent_at": now,
		}).Error
}

func (c *Client) FailEmailOutbox(ctx context.Context, id int32, errMsg string) error {
	return c.gorm.WithContext(ctx).
		Model(&model.EmailOutbox{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        model.EmailOutboxStatusFailed,
			"error_message": errMsg,
		}).Error
}

func (c *Client) GetEmailOutbox(ctx context.Context, id int32) (*model.EmailOutbox, error) {
	var outbox model.EmailOutbox
	err := c.gorm.WithContext(ctx).Where("id = ?", id).First(&outbox).Error
	if err != nil {
		return nil, err
	}
	return &outbox, nil
}
