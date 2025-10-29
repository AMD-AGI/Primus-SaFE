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
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

func (c *Client) UpsertRegistryInfo(ctx context.Context, registryInfo *model.RegistryInfo) error {
	if registryInfo == nil {
		return errors.New("the input is empty")
	}

	exist, err := c.GetRegistryInfoById(ctx, registryInfo.ID)
	if err != nil {
		return err
	}
	if exist == nil {
		// insert
		if err := dal.Use(c.gorm).RegistryInfo.WithContext(ctx).Create(registryInfo); err != nil {
			klog.Errorf("UpsertRegistryInfo insert error: %+v", err)
			return err
		}
	} else {
		// update
		registryInfo.ID = exist.ID
		if err := dal.Use(c.gorm).RegistryInfo.WithContext(ctx).Save(registryInfo); err != nil {
			klog.Errorf("UpsertRegistryInfo update error: %+v", err)
			return err
		}
	}
	return nil
}

func (c *Client) GetRegistryInfoById(ctx context.Context, id int32) (*model.RegistryInfo, error) {
	q := dal.Use(c.gorm).RegistryInfo
	item, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		klog.Errorf("GetRegistryInfoById error: %+v", err)
		return nil, err
	}
	return item, nil
}

func (c *Client) GetDefaultRegistryInfo(ctx context.Context) (*model.RegistryInfo, error) {
	q := dal.Use(c.gorm).RegistryInfo
	item, err := q.WithContext(ctx).Where(q.Default.Is(true)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		klog.Errorf("GetDefaultRegistryInfo error: %+v", err)
		return nil, err
	}
	return item, nil
}

func (c *Client) GetRegistryInfoByUrl(ctx context.Context, url string) (*model.RegistryInfo, error) {
	q := dal.Use(c.gorm).RegistryInfo
	item, err := q.WithContext(ctx).Where(q.URL.Eq(url)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		klog.Errorf("GetRegistryInfoByUrl error: %+v", err)
	}
	return item, nil
}

func (c *Client) DeleteRegistryInfo(ctx context.Context, id int32) error {
	q := dal.Use(c.gorm).RegistryInfo
	_, err := q.WithContext(ctx).Where(q.ID.Eq(int32(id))).Delete()
	if err != nil {
		klog.Errorf("DeleteRegistryInfo error: %+v", err)
		return err
	}
	return nil
}

func (c *Client) ListRegistryInfos(ctx context.Context, pageNum, pageSize int) ([]*model.RegistryInfo, error) {
	q := dal.Use(c.gorm).RegistryInfo
	query := q.WithContext(ctx).Order(q.ID.Desc())
	if pageNum > 0 && pageSize > 0 {
		query = query.Offset((pageNum - 1) * pageSize).Limit(pageSize)
	}
	items, err := query.Find()
	if err != nil {
		klog.Errorf("ListRegistryInfos error: %+v", err)
		return nil, err
	}
	return items, nil
}
