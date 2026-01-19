// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// NotificationChannelRequest represents a request to create or update a notification channel
type NotificationChannelRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        string                 `json:"type" binding:"required"` // email, webhook, dingtalk, wechat, slack, alertmanager
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config" binding:"required"`
	Description string                 `json:"description"`
}

// NotificationChannelResponse represents a notification channel response
type NotificationChannelResponse struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
	Description string                 `json:"description"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   string                 `json:"created_by"`
	UpdatedBy   string                 `json:"updated_by"`
}

// NotificationChannelListResponse represents a list response
type NotificationChannelListResponse struct {
	Total int64                          `json:"total"`
	Items []*NotificationChannelResponse `json:"items"`
}

// toResponse converts db model to API response
func toNotificationChannelResponse(channel *dbmodel.NotificationChannels) *NotificationChannelResponse {
	resp := &NotificationChannelResponse{
		ID:          channel.ID,
		Name:        channel.Name,
		Type:        channel.Type,
		Enabled:     channel.Enabled,
		Config:      channel.Config,
		Description: channel.Description,
		CreatedAt:   channel.CreatedAt,
		UpdatedAt:   channel.UpdatedAt,
		CreatedBy:   channel.CreatedBy,
		UpdatedBy:   channel.UpdatedBy,
	}
	return resp
}

// ListNotificationChannels handles GET /api/notification-channels
func ListNotificationChannels(c *gin.Context) {
	// Parse query parameters
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	channelType := c.Query("type")
	name := c.Query("search")
	enabledStr := c.Query("enabled")

	filter := &database.NotificationChannelFilter{
		Offset: offset,
		Limit:  limit,
	}

	if channelType != "" {
		filter.Type = &channelType
	}
	if name != "" {
		filter.Name = &name
	}
	if enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}

	facade := database.GetFacade().GetNotificationChannel()
	channels, total, err := facade.ListNotificationChannels(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to list notification channels: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Convert to response format
	items := make([]*NotificationChannelResponse, len(channels))
	for i, ch := range channels {
		items[i] = toNotificationChannelResponse(ch)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), NotificationChannelListResponse{
		Total: total,
		Items: items,
	}))
}

// GetNotificationChannel handles GET /api/notification-channels/:id
func GetNotificationChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid id", nil))
		return
	}

	facade := database.GetFacade().GetNotificationChannel()
	channel, err := facade.GetNotificationChannelByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if channel == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "notification channel not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), toNotificationChannelResponse(channel)))
}

// CreateNotificationChannel handles POST /api/notification-channels
func CreateNotificationChannel(c *gin.Context) {
	var req NotificationChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate channel type
	validTypes := dbmodel.ValidChannelTypes()
	validType := false
	for _, t := range validTypes {
		if req.Type == t {
			validType = true
			break
		}
	}
	if !validType {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, fmt.Sprintf("invalid channel type, must be one of: %v", validTypes), nil))
		return
	}

	// Validate channel config based on type
	if err := validateChannelConfig(req.Type, req.Config); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get user from context if available
	createdBy, _ := c.Get("user")
	createdByStr := ""
	if createdBy != nil {
		createdByStr = fmt.Sprintf("%v", createdBy)
	}

	// Convert config to ExtType
	var configExt dbmodel.ExtType
	configBytes, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid config format: "+err.Error(), nil))
		return
	}
	if err := json.Unmarshal(configBytes, &configExt); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid config format: "+err.Error(), nil))
		return
	}

	channel := &dbmodel.NotificationChannels{
		Name:        req.Name,
		Type:        req.Type,
		Enabled:     req.Enabled,
		Config:      configExt,
		Description: req.Description,
		CreatedBy:   createdByStr,
	}

	facade := database.GetFacade().GetNotificationChannel()

	// Check if name already exists
	existing, err := facade.GetNotificationChannelByName(c.Request.Context(), req.Name)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to check existing channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, rest.ErrorResp(c.Request.Context(), http.StatusConflict, "notification channel with this name already exists", nil))
		return
	}

	if err := facade.CreateNotificationChannel(c.Request.Context(), channel); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to create notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusCreated, rest.SuccessResp(c.Request.Context(), toNotificationChannelResponse(channel)))
}

// UpdateNotificationChannel handles PUT /api/notification-channels/:id
func UpdateNotificationChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid id", nil))
		return
	}

	var req NotificationChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to parse request body: %v", err)
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate channel type
	validTypes := dbmodel.ValidChannelTypes()
	validType := false
	for _, t := range validTypes {
		if req.Type == t {
			validType = true
			break
		}
	}
	if !validType {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, fmt.Sprintf("invalid channel type, must be one of: %v", validTypes), nil))
		return
	}

	// Validate channel config based on type
	if err := validateChannelConfig(req.Type, req.Config); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	facade := database.GetFacade().GetNotificationChannel()
	channel, err := facade.GetNotificationChannelByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if channel == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "notification channel not found", nil))
		return
	}

	// Check if name changed and if new name already exists
	if channel.Name != req.Name {
		existing, err := facade.GetNotificationChannelByName(c.Request.Context(), req.Name)
		if err != nil {
			log.GlobalLogger().WithContext(c).Errorf("Failed to check existing channel: %v", err)
			c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
			return
		}
		if existing != nil {
			c.JSON(http.StatusConflict, rest.ErrorResp(c.Request.Context(), http.StatusConflict, "notification channel with this name already exists", nil))
			return
		}
	}

	// Get user from context if available
	updatedBy, _ := c.Get("user")
	updatedByStr := ""
	if updatedBy != nil {
		updatedByStr = fmt.Sprintf("%v", updatedBy)
	}

	// Convert config to ExtType
	var configExt dbmodel.ExtType
	configBytes, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid config format: "+err.Error(), nil))
		return
	}
	if err := json.Unmarshal(configBytes, &configExt); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid config format: "+err.Error(), nil))
		return
	}

	// Update fields
	channel.Name = req.Name
	channel.Type = req.Type
	channel.Enabled = req.Enabled
	channel.Config = configExt
	channel.Description = req.Description
	channel.UpdatedAt = time.Now()
	channel.UpdatedBy = updatedByStr

	if err := facade.UpdateNotificationChannel(c.Request.Context(), channel); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to update notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), toNotificationChannelResponse(channel)))
}

// DeleteNotificationChannel handles DELETE /api/notification-channels/:id
func DeleteNotificationChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid id", nil))
		return
	}

	facade := database.GetFacade().GetNotificationChannel()
	channel, err := facade.GetNotificationChannelByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if channel == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "notification channel not found", nil))
		return
	}

	// TODO: Check if channel is referenced by any alert routing rules before deleting

	if err := facade.DeleteNotificationChannel(c.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to delete notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{"deleted": true}))
}

// TestNotificationChannel handles POST /api/notification-channels/:id/test
func TestNotificationChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid id", nil))
		return
	}

	facade := database.GetFacade().GetNotificationChannel()
	channel, err := facade.GetNotificationChannelByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get notification channel: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	if channel == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "notification channel not found", nil))
		return
	}

	// TODO: Implement actual channel testing logic based on channel type
	// For now, just return success
	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"success": true,
		"message": "Test notification sent (not yet implemented)",
	}))
}

// GetChannelTypes handles GET /api/notification-channels/types
func GetChannelTypes(c *gin.Context) {
	types := []map[string]interface{}{
		{
			"type":        dbmodel.ChannelTypeEmail,
			"name":        "Email",
			"description": "Send notifications via email using SMTP",
			"config_schema": map[string]interface{}{
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
			"type":        dbmodel.ChannelTypeWebhook,
			"name":        "Webhook",
			"description": "Send notifications to a webhook URL",
			"config_schema": map[string]interface{}{
				"url":     "string",
				"method":  "string",
				"headers": "object",
				"timeout": "number",
			},
		},
		{
			"type":        dbmodel.ChannelTypeDingTalk,
			"name":        "DingTalk",
			"description": "Send notifications to DingTalk robot",
			"config_schema": map[string]interface{}{
				"webhook_url": "string",
				"secret":      "string",
			},
		},
		{
			"type":        dbmodel.ChannelTypeWeChat,
			"name":        "WeChat Work",
			"description": "Send notifications via WeChat Work",
			"config_schema": map[string]interface{}{
				"corp_id":    "string",
				"agent_id":   "number",
				"secret":     "string",
				"to_user":    "string",
				"to_party":   "string",
				"to_tag":     "string",
				"message":    "string",
				"api_secret": "string",
			},
		},
		{
			"type":        dbmodel.ChannelTypeSlack,
			"name":        "Slack",
			"description": "Send notifications to Slack channel",
			"config_schema": map[string]interface{}{
				"webhook_url": "string",
				"channel":     "string",
				"username":    "string",
			},
		},
		{
			"type":        dbmodel.ChannelTypeAlertManager,
			"name":        "AlertManager",
			"description": "Forward alerts to AlertManager",
			"config_schema": map[string]interface{}{
				"url":      "string",
				"timeout":  "number",
				"api_path": "string",
			},
		},
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), types))
}

// validateChannelConfig validates the channel config based on type
func validateChannelConfig(channelType string, config map[string]interface{}) error {
	switch channelType {
	case dbmodel.ChannelTypeEmail:
		// Required fields for email
		requiredFields := []string{"smtp_host", "smtp_port", "from"}
		for _, field := range requiredFields {
			if _, ok := config[field]; !ok {
				return fmt.Errorf("missing required field for email channel: %s", field)
			}
		}
	case dbmodel.ChannelTypeWebhook:
		if _, ok := config["url"]; !ok {
			return fmt.Errorf("missing required field for webhook channel: url")
		}
	case dbmodel.ChannelTypeDingTalk:
		if _, ok := config["webhook_url"]; !ok {
			return fmt.Errorf("missing required field for dingtalk channel: webhook_url")
		}
	case dbmodel.ChannelTypeSlack:
		if _, ok := config["webhook_url"]; !ok {
			return fmt.Errorf("missing required field for slack channel: webhook_url")
		}
	case dbmodel.ChannelTypeWeChat:
		requiredFields := []string{"corp_id", "agent_id", "secret"}
		for _, field := range requiredFields {
			if _, ok := config[field]; !ok {
				return fmt.Errorf("missing required field for wechat channel: %s", field)
			}
		}
	case dbmodel.ChannelTypeAlertManager:
		if _, ok := config["url"]; !ok {
			return fmt.Errorf("missing required field for alertmanager channel: url")
		}
	}
	return nil
}
