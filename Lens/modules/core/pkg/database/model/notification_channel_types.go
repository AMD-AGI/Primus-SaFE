// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

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
