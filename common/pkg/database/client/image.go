/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

type ImageFilter struct {
	UserName string
	Tag      string
	OrderBy  string
	Order    string
	PageNum  int
	PageSize int
	Ready    bool
}

// UpsertImage 插入或更新镜像记录
func (c *Client) UpsertImage(ctx context.Context, img *model.Image) error {
	if img == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	q := dal.Use(c.gorm).Image
	exist, err := c.GetImageByTag(ctx, img.Tag)
	if err != nil {
		return err
	}
	if exist != nil {
		img.ID = exist.ID
		if err := q.WithContext(ctx).Save(img); err != nil {
			klog.ErrorS(err, "failed to upsert image", "image", img)
			return err
		}
	} else {
		if err := q.WithContext(ctx).Create(img); err != nil {
			klog.ErrorS(err, "failed to upsert image", "image", img)
			return err
		}
	}
	return nil
}

func (c *Client) GetImageByTag(ctx context.Context, tag string) (*model.Image, error) {
	q := dal.Use(c.gorm).Image
	img, err := q.WithContext(ctx).Where(q.Tag.Eq(tag), q.DeletedAt.IsNull()).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		klog.ErrorS(err, "failed to get image by tag", "tag", tag)
		return nil, err
	}
	return img, nil
}

func (c *Client) SelectImages(ctx context.Context, filter *ImageFilter) ([]*model.Image, int, error) {
	q := dal.Use(c.gorm).Image
	query := q.WithContext(ctx).Where(q.DeletedAt.IsNull())
	if filter.Tag != "" {
		query = query.Where(q.Tag.Like("%" + filter.Tag + "%"))
	}
	if filter.UserName != "" {
		query = query.Where(q.CreatedBy.Eq(filter.UserName))
	}
	if filter.Ready {
		query = query.Where(q.Status.Eq("Ready"))
	}
	count, err := query.Count()
	if err != nil {
		klog.ErrorS(err, "failed to count images")
		return nil, 0, err
	}
	gormDB := query.UnderlyingDB()
	if filter.OrderBy != "" {
		order := filter.Order
		if order == "" {
			order = "DESC"
		}
		gormDB = gormDB.Order(fmt.Sprintf("%s %s", filter.OrderBy, order))
	}
	if filter.PageSize > 0 {
		gormDB = gormDB.Limit(filter.PageSize)
	}
	if filter.PageNum > 0 {
		gormDB = gormDB.Offset((filter.PageNum - 1) * filter.PageSize)
	}
	var images []*model.Image
	err = gormDB.Find(&images).Error
	if err != nil {
		return nil, 0, err
	}
	return images, int(count), nil
}

func (c *Client) GetImage(ctx context.Context, imageId int32) (*model.Image, error) {
	q := dal.Use(c.gorm).Image
	img, err := q.WithContext(ctx).Where(q.ID.Eq(imageId), q.DeletedAt.IsNull()).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		klog.ErrorS(err, "failed to get image by id", "id", imageId)
		return nil, err
	}
	return img, nil
}

// DeleteImage 逻辑删除镜像
func (c *Client) DeleteImage(ctx context.Context, id int32, _ string) error {
	q := dal.Use(c.gorm).Image
	img, err := q.WithContext(ctx).Where(q.ID.Eq(id), q.DeletedAt.IsNull()).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		klog.ErrorS(err, "failed to get image by id", "id", id)
		return err
	}
	_, err = q.WithContext(ctx).Delete(img)
	if err != nil {
		klog.ErrorS(err, "failed to delete image by id", "id", id)
		return err
	}
	return nil
}
