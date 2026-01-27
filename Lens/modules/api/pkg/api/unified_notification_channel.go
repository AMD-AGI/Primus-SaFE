// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ============================================================================
// Notification Channel Endpoints
// ============================================================================

// --- List Channels ---

type ListNotificationChannelsRequest struct {
	Type    string `json:"type" mcp:"desc=Filter by channel type (email/webhook/slack/feishu/dingtalk)"`
	Search  string `json:"search" mcp:"desc=Search by channel name"`
	Enabled string `json:"enabled" mcp:"desc=Filter by enabled status (true/false)"`
	Offset  int    `json:"offset" mcp:"desc=Pagination offset"`
	Limit   int    `json:"limit" mcp:"desc=Pagination limit (default: 20)"`
}

type NotificationChannelItem struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

type ListNotificationChannelsResponse struct {
	Channels []NotificationChannelItem `json:"channels"`
	Total    int64                     `json:"total"`
	Offset   int                       `json:"offset"`
	Limit    int                       `json:"limit"`
}

func handleListNotificationChannels(ctx context.Context, req *ListNotificationChannelsRequest) (*ListNotificationChannelsResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	filter := &database.NotificationChannelFilter{
		Offset: req.Offset,
		Limit:  limit,
	}

	if req.Type != "" {
		filter.Type = &req.Type
	}
	if req.Search != "" {
		filter.Name = &req.Search
	}
	if req.Enabled != "" {
		enabled := req.Enabled == "true"
		filter.Enabled = &enabled
	}

	facade := database.GetFacade().GetNotificationChannel()
	channels, total, err := facade.ListNotificationChannels(ctx, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list channels", errors.CodeDatabaseError)
	}

	items := make([]NotificationChannelItem, 0, len(channels))
	for _, ch := range channels {
		item := NotificationChannelItem{
			ID:          ch.ID,
			Name:        ch.Name,
			Type:        ch.Type,
			Description: ch.Description,
			Enabled:     ch.Enabled,
			CreatedAt:   ch.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   ch.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if ch.Config != nil {
			item.Config = ch.Config
		}
		items = append(items, item)
	}

	return &ListNotificationChannelsResponse{
		Channels: items,
		Total:    total,
		Offset:   req.Offset,
		Limit:    limit,
	}, nil
}

// --- Get Channel ---

type GetNotificationChannelRequest struct {
	ID int64 `json:"id" mcp:"required,desc=Channel ID"`
}

type GetNotificationChannelResponse struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
	CreatedBy   string                 `json:"created_by"`
}

func handleGetNotificationChannel(ctx context.Context, req *GetNotificationChannelRequest) (*GetNotificationChannelResponse, error) {
	if req.ID <= 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid channel ID")
	}

	facade := database.GetFacade().GetNotificationChannel()
	channel, err := facade.GetNotificationChannelByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get channel", errors.CodeDatabaseError)
	}
	if channel == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("channel not found")
	}

	resp := &GetNotificationChannelResponse{
		ID:          channel.ID,
		Name:        channel.Name,
		Type:        channel.Type,
		Description: channel.Description,
		Enabled:     channel.Enabled,
		CreatedAt:   channel.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   channel.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		CreatedBy:   channel.CreatedBy,
	}
	if channel.Config != nil {
		resp.Config = channel.Config
	}

	return resp, nil
}

// --- Get Channel Types ---

type GetChannelTypesRequest struct{}

type ChannelTypeInfo struct {
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	ConfigSchema map[string]interface{} `json:"config_schema"`
}

type GetChannelTypesResponse struct {
	Types []ChannelTypeInfo `json:"types"`
}

func handleGetChannelTypes(ctx context.Context, req *GetChannelTypesRequest) (*GetChannelTypesResponse, error) {
	types := []ChannelTypeInfo{
		{
			Type:        dbmodel.ChannelTypeEmail,
			Name:        "Email",
			Description: "Send notifications via email using SMTP",
			ConfigSchema: map[string]interface{}{
				"smtp_host":    "string",
				"smtp_port":    "number",
				"username":     "string",
				"password":     "string",
				"from":         "string",
				"from_name":    "string",
				"use_starttls": "boolean",
				"skip_verify":  "boolean",
			},
		},
		{
			Type:        dbmodel.ChannelTypeWebhook,
			Name:        "Webhook",
			Description: "Send notifications to a webhook URL",
			ConfigSchema: map[string]interface{}{
				"url":     "string",
				"method":  "string",
				"headers": "object",
				"timeout": "number",
			},
		},
		{
			Type:        dbmodel.ChannelTypeDingTalk,
			Name:        "DingTalk",
			Description: "Send notifications to DingTalk groups",
			ConfigSchema: map[string]interface{}{
				"webhook_url": "string",
				"secret":      "string",
			},
		},
		{
			Type:        dbmodel.ChannelTypeWeChat,
			Name:        "WeChat Work",
			Description: "Send notifications via WeChat Work",
			ConfigSchema: map[string]interface{}{
				"corp_id":  "string",
				"agent_id": "string",
				"secret":   "string",
				"to_user":  "string",
			},
		},
		{
			Type:        dbmodel.ChannelTypeSlack,
			Name:        "Slack",
			Description: "Send notifications to Slack channels",
			ConfigSchema: map[string]interface{}{
				"webhook_url": "string",
				"channel":     "string",
				"username":    "string",
				"icon_emoji":  "string",
			},
		},
		{
			Type:        dbmodel.ChannelTypeAlertManager,
			Name:        "AlertManager",
			Description: "Forward alerts to AlertManager",
			ConfigSchema: map[string]interface{}{
				"url":        "string",
				"basic_auth": "object",
			},
		},
	}

	return &GetChannelTypesResponse{Types: types}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// Get Channel Types (must register before /:id)
	unified.Register(&unified.EndpointDef[GetChannelTypesRequest, GetChannelTypesResponse]{
		HTTPPath:    "/notification-channels/types",
		HTTPMethod:  "GET",
		MCPToolName: "lens_notification_channel_types",
		Description: "Get all available notification channel types with their configuration schemas",
		Handler:     handleGetChannelTypes,
	})

	// List Notification Channels
	unified.Register(&unified.EndpointDef[ListNotificationChannelsRequest, ListNotificationChannelsResponse]{
		HTTPPath:    "/notification-channels",
		HTTPMethod:  "GET",
		MCPToolName: "lens_notification_channels_list",
		Description: "List notification channels with filtering options",
		Handler:     handleListNotificationChannels,
	})

	// Get Notification Channel
	unified.Register(&unified.EndpointDef[GetNotificationChannelRequest, GetNotificationChannelResponse]{
		HTTPPath:    "/notification-channels/:id",
		HTTPMethod:  "GET",
		MCPToolName: "lens_notification_channel_detail",
		Description: "Get a specific notification channel by ID",
		Handler:     handleGetNotificationChannel,
	})
}
