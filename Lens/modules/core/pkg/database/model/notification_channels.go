// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameNotificationChannels = "notification_channels"

// NotificationChannels represents a reusable notification channel configuration
type NotificationChannels struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Name        string    `gorm:"column:name;uniqueIndex;not null" json:"name"`
	Type        string    `gorm:"column:type;not null" json:"type"` // email, webhook, dingtalk, wechat, slack, alertmanager
	Enabled     bool      `gorm:"column:enabled;default:true" json:"enabled"`
	Config      ExtType   `gorm:"column:config;not null" json:"config"`
	Description string    `gorm:"column:description" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	CreatedBy   string    `gorm:"column:created_by" json:"created_by"`
	UpdatedBy   string    `gorm:"column:updated_by" json:"updated_by"`
}

// TableName returns the table name
func (*NotificationChannels) TableName() string {
	return TableNameNotificationChannels
}

// Channel types
const (
	ChannelTypeEmail        = "email"
	ChannelTypeWebhook      = "webhook"
	ChannelTypeDingTalk     = "dingtalk"
	ChannelTypeWeChat       = "wechat"
	ChannelTypeSlack        = "slack"
	ChannelTypeAlertManager = "alertmanager"
)

// ValidChannelTypes returns all valid channel types
func ValidChannelTypes() []string {
	return []string{
		ChannelTypeEmail,
		ChannelTypeWebhook,
		ChannelTypeDingTalk,
		ChannelTypeWeChat,
		ChannelTypeSlack,
		ChannelTypeAlertManager,
	}
}
