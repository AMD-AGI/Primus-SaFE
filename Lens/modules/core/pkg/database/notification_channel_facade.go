// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// NotificationChannelFacade provides database operations for notification channels
type NotificationChannelFacade struct {
	db *gorm.DB
}

// NewNotificationChannelFacade creates a new NotificationChannelFacade
func NewNotificationChannelFacade(db *gorm.DB) *NotificationChannelFacade {
	return &NotificationChannelFacade{db: db}
}

// NotificationChannelFilter defines filter options for listing channels
type NotificationChannelFilter struct {
	Type    *string
	Enabled *bool
	Name    *string
	Offset  int
	Limit   int
}

// CreateNotificationChannel creates a new notification channel
func (f *NotificationChannelFacade) CreateNotificationChannel(ctx context.Context, channel *model.NotificationChannels) error {
	return f.db.WithContext(ctx).Create(channel).Error
}

// GetNotificationChannelByID retrieves a channel by ID
func (f *NotificationChannelFacade) GetNotificationChannelByID(ctx context.Context, id int64) (*model.NotificationChannels, error) {
	var channel model.NotificationChannels
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&channel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &channel, nil
}

// GetNotificationChannelByName retrieves a channel by name
func (f *NotificationChannelFacade) GetNotificationChannelByName(ctx context.Context, name string) (*model.NotificationChannels, error) {
	var channel model.NotificationChannels
	err := f.db.WithContext(ctx).Where("name = ?", name).First(&channel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &channel, nil
}

// ListNotificationChannels lists channels with optional filters
func (f *NotificationChannelFacade) ListNotificationChannels(ctx context.Context, filter *NotificationChannelFilter) ([]*model.NotificationChannels, int64, error) {
	var channels []*model.NotificationChannels
	var total int64

	query := f.db.WithContext(ctx).Model(&model.NotificationChannels{})

	if filter != nil {
		if filter.Type != nil {
			query = query.Where("type = ?", *filter.Type)
		}
		if filter.Enabled != nil {
			query = query.Where("enabled = ?", *filter.Enabled)
		}
		if filter.Name != nil && *filter.Name != "" {
			query = query.Where("name ILIKE ?", "%"+*filter.Name+"%")
		}
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if filter != nil {
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Order by created_at desc
	query = query.Order("created_at DESC")

	if err := query.Find(&channels).Error; err != nil {
		return nil, 0, err
	}

	return channels, total, nil
}

// UpdateNotificationChannel updates a notification channel
func (f *NotificationChannelFacade) UpdateNotificationChannel(ctx context.Context, channel *model.NotificationChannels) error {
	return f.db.WithContext(ctx).Save(channel).Error
}

// DeleteNotificationChannel deletes a notification channel by ID
func (f *NotificationChannelFacade) DeleteNotificationChannel(ctx context.Context, id int64) error {
	return f.db.WithContext(ctx).Delete(&model.NotificationChannels{}, id).Error
}

// GetEnabledChannelsByType retrieves all enabled channels of a specific type
func (f *NotificationChannelFacade) GetEnabledChannelsByType(ctx context.Context, channelType string) ([]*model.NotificationChannels, error) {
	var channels []*model.NotificationChannels
	err := f.db.WithContext(ctx).
		Where("type = ? AND enabled = true", channelType).
		Find(&channels).Error
	return channels, err
}

// GetChannelsByIDs retrieves channels by their IDs
func (f *NotificationChannelFacade) GetChannelsByIDs(ctx context.Context, ids []int64) ([]*model.NotificationChannels, error) {
	var channels []*model.NotificationChannels
	err := f.db.WithContext(ctx).
		Where("id IN ? AND enabled = true", ids).
		Find(&channels).Error
	return channels, err
}
