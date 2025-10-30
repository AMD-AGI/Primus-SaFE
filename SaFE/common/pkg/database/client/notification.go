/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"gorm.io/gorm"
)

// SubmitNotification submits a notification to be processed and sent.
func (c *Client) SubmitNotification(ctx context.Context, data *model.Notification) error {
	q := dal.Use(c.gorm).Notification
	existNotification, err := q.WithContext(ctx).Where(q.UID.Eq(data.UID), q.Topic.Eq(data.Topic)).First()
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if existNotification != nil {
		// Notification already exists
		return nil
	}
	return q.WithContext(ctx).Create(data)
}

// UpdateNotification updates the specified resource.
func (c *Client) UpdateNotification(ctx context.Context, data *model.Notification) error {
	q := dal.Use(c.gorm).Notification
	err := q.WithContext(ctx).Where(q.ID.Eq(data.ID)).Save(data)
	return err
}

// ListUnprocessedNotifications retrieves a list of resources.
func (c *Client) ListUnprocessedNotifications(ctx context.Context) ([]*model.Notification, error) {
	q := dal.Use(c.gorm).Notification
	return q.WithContext(ctx).Where(q.SentAt.Eq(time.Time{})).Find()
}
