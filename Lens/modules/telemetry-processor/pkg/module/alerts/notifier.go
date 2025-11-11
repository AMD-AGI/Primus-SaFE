package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// sendNotification sends an alert notification through the specified channel
func sendNotification(ctx context.Context, alert *UnifiedAlert, channel ChannelConfig) error {
	log.GlobalLogger().WithContext(ctx).Infof("Sending notification for alert %s via %s", alert.ID, channel.Type)

	// Create notification record
	notification := createNotificationRecord(alert, channel)

	facade := database.GetFacade().GetAlert()
	if err := facade.CreateAlertNotifications(ctx, notification); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create notification record: %v", err)
		// Continue with sending even if record creation fails
	}

	// Send based on channel type
	var err error
	switch channel.Type {
	case ChannelWebhook:
		err = sendWebhookNotification(ctx, alert, channel.Config)
	case ChannelEmail:
		err = sendEmailNotification(ctx, alert, channel.Config)
	case ChannelDingTalk:
		err = sendDingTalkNotification(ctx, alert, channel.Config)
	case ChannelWeChat:
		err = sendWeChatNotification(ctx, alert, channel.Config)
	case ChannelSlack:
		err = sendSlackNotification(ctx, alert, channel.Config)
	case ChannelAlertManager:
		err = sendToAlertManager(ctx, alert, channel.Config)
	default:
		err = fmt.Errorf("unsupported notification channel: %s", channel.Type)
	}

	// Update notification status
	if err != nil {
		notification.Status = NotificationStatusFailed
		notification.ErrorMessage = err.Error()
	} else {
		notification.Status = NotificationStatusSent
		notification.SentAt = time.Now()
	}

	if updateErr := facade.UpdateAlertNotifications(ctx, notification); updateErr != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update notification status: %v", updateErr)
	}

	return err
}

// createNotificationRecord creates a notification record for the database
func createNotificationRecord(alert *UnifiedAlert, channel ChannelConfig) *model.AlertNotifications {
	channelConfigExt := model.ExtType(channel.Config)

	payloadExt := model.ExtType{}
	alertBytes, err := json.Marshal(alert)
	if err == nil {
		json.Unmarshal(alertBytes, &payloadExt)
	}

	return &model.AlertNotifications{
		AlertID:             alert.ID,
		Channel:             channel.Type,
		ChannelConfig:       channelConfigExt,
		Status:              NotificationStatusPending,
		NotificationPayload: payloadExt,
	}
}

// sendWebhookNotification sends a notification via webhook
func sendWebhookNotification(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	// Prepare webhook payload
	payload := map[string]interface{}{
		"alert_id":    alert.ID,
		"source":      alert.Source,
		"alert_name":  alert.AlertName,
		"severity":    alert.Severity,
		"status":      alert.Status,
		"starts_at":   alert.StartsAt,
		"labels":      alert.Labels,
		"annotations": alert.Annotations,
		"workload_id": alert.WorkloadID,
		"pod_name":    alert.PodName,
		"node_name":   alert.NodeName,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Send HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers if configured
	if headers, ok := config["headers"].(map[string]string); ok {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Webhook notification sent successfully for alert %s", alert.ID)
	return nil
}

// sendEmailNotification sends a notification via email
func sendEmailNotification(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	// TODO: Implement email notification
	// This would typically use SMTP or an email service API
	log.GlobalLogger().WithContext(ctx).Infof("Email notification not yet implemented for alert %s", alert.ID)
	return fmt.Errorf("email notification not implemented")
}

// sendDingTalkNotification sends a notification via DingTalk
func sendDingTalkNotification(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("DingTalk webhook URL not configured")
	}

	// Prepare DingTalk message
	message := formatAlertMessage(alert)
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"title": fmt.Sprintf("Alert: %s", alert.AlertName),
			"text":  message,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal DingTalk payload: %w", err)
	}

	// Send HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create DingTalk request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send DingTalk notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DingTalk returned status %d", resp.StatusCode)
	}

	log.GlobalLogger().WithContext(ctx).Infof("DingTalk notification sent successfully for alert %s", alert.ID)
	return nil
}

// sendWeChatNotification sends a notification via WeChat
func sendWeChatNotification(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	// TODO: Implement WeChat notification
	log.GlobalLogger().WithContext(ctx).Infof("WeChat notification not yet implemented for alert %s", alert.ID)
	return fmt.Errorf("WeChat notification not implemented")
}

// sendSlackNotification sends a notification via Slack
func sendSlackNotification(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("Slack webhook URL not configured")
	}

	// Prepare Slack message
	message := formatAlertMessage(alert)
	color := getSeverityColor(alert.Severity)

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":     color,
				"title":     fmt.Sprintf("Alert: %s", alert.AlertName),
				"text":      message,
				"timestamp": alert.StartsAt.Unix(),
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	// Send HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Slack returned status %d", resp.StatusCode)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Slack notification sent successfully for alert %s", alert.ID)
	return nil
}

// sendToAlertManager forwards the alert to AlertManager
func sendToAlertManager(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("AlertManager URL not configured")
	}

	// Convert unified alert to AlertManager format
	amAlert := []map[string]interface{}{
		{
			"labels":      alert.Labels,
			"annotations": alert.Annotations,
			"startsAt":    alert.StartsAt.Format(time.RFC3339),
		},
	}

	if alert.EndsAt != nil {
		amAlert[0]["endsAt"] = alert.EndsAt.Format(time.RFC3339)
	}

	jsonData, err := json.Marshal(amAlert)
	if err != nil {
		return fmt.Errorf("failed to marshal AlertManager payload: %w", err)
	}

	// Send HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url+"/api/v1/alerts", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create AlertManager request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send to AlertManager: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("AlertManager returned status %d", resp.StatusCode)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Alert forwarded to AlertManager successfully: %s", alert.ID)
	return nil
}

// formatAlertMessage formats an alert into a human-readable message
func formatAlertMessage(alert *UnifiedAlert) string {
	msg := fmt.Sprintf("**Severity**: %s\n", alert.Severity)
	msg += fmt.Sprintf("**Source**: %s\n", alert.Source)
	msg += fmt.Sprintf("**Status**: %s\n", alert.Status)
	msg += fmt.Sprintf("**Time**: %s\n", alert.StartsAt.Format(time.RFC3339))

	if alert.WorkloadID != "" {
		msg += fmt.Sprintf("**Workload**: %s\n", alert.WorkloadID)
	}
	if alert.PodName != "" {
		msg += fmt.Sprintf("**Pod**: %s\n", alert.PodName)
	}
	if alert.NodeName != "" {
		msg += fmt.Sprintf("**Node**: %s\n", alert.NodeName)
	}

	if description, ok := alert.Annotations["description"]; ok {
		msg += fmt.Sprintf("\n**Description**: %s\n", description)
	}
	if summary, ok := alert.Annotations["summary"]; ok {
		msg += fmt.Sprintf("\n**Summary**: %s\n", summary)
	}

	return msg
}

// getSeverityColor returns a color code for the given severity
func getSeverityColor(severity string) string {
	switch severity {
	case SeverityCritical:
		return "danger"
	case SeverityHigh:
		return "warning"
	case SeverityWarning:
		return "#FFA500"
	case SeverityInfo:
		return "good"
	default:
		return "#808080"
	}
}
