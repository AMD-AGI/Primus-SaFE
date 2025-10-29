/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

// UpsertImageDigest performs the UpsertImageDigest operation.
func (c *Client) UpsertImageDigest(ctx context.Context, d *model.ImageDigest) error {
	if d == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}

	exist, err := c.GetImageDigestById(ctx, d.ID)
	if err != nil {
		return err
	}
	if exist == nil {
		// insert
		if err := dal.Use(c.gorm).ImageDigest.WithContext(ctx).Create(d); err != nil {
			klog.ErrorS(err, "failed to insert image_digest", "image_digest", d)
			return err
		}
	} else {
		// update
		d.ID = exist.ID
		if err := dal.Use(c.gorm).ImageDigest.WithContext(ctx).Save(d); err != nil {
			klog.ErrorS(err, "failed to update image_digest", "image_digest", d)
			return err
		}
	}
	return nil
}

// GetImageDigestById returns the ImageDigestById value.
func (c *Client) GetImageDigestById(ctx context.Context, id int32) (*model.ImageDigest, error) {
	q := dal.Use(c.gorm).ImageDigest
	item, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		klog.ErrorS(err, "failed to get image_digest by id", "id", id)
		return nil, err
	}
	return item, nil
}

// DeleteImageDigest performs a soft delete of an image digest.
func (c *Client) DeleteImageDigest(ctx context.Context, id int32) error {
	q := dal.Use(c.gorm).ImageDigest
	_, err := q.WithContext(ctx).Where(q.ID.Eq(int32(id))).Delete()
	if err != nil {
		klog.ErrorS(err, "failed to delete image_digest by id",
			"id", id)
		return err
	}
	return nil
}
