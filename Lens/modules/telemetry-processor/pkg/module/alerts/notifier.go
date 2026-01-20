// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
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

// EmailConfig represents SMTP email configuration
type EmailConfig struct {
	SMTPHost     string   // SMTP server host
	SMTPPort     int      // SMTP server port (25, 465, 587)
	Username     string   // SMTP authentication username
	Password     string   // SMTP authentication password
	From         string   // Sender email address
	FromName     string   // Sender display name
	To           []string // Recipient email addresses
	CC           []string // CC recipients
	UseTLS       bool     // Use TLS/SSL connection
	UseSTARTTLS  bool     // Use STARTTLS upgrade
	SkipVerify   bool     // Skip TLS certificate verification
	TemplateName string   // Custom template name (optional)
}

// parseEmailConfig parses email configuration from map
func parseEmailConfig(config map[string]interface{}) (*EmailConfig, error) {
	ec := &EmailConfig{
		SMTPPort:    587,     // Default to submission port
		UseSTARTTLS: true,    // Default to STARTTLS
		SkipVerify:  false,
	}

	// Required fields
	if host, ok := config["smtp_host"].(string); ok && host != "" {
		ec.SMTPHost = host
	} else {
		return nil, fmt.Errorf("smtp_host is required")
	}

	if from, ok := config["from"].(string); ok && from != "" {
		ec.From = from
	} else {
		return nil, fmt.Errorf("from email address is required")
	}

	// Parse recipients
	if to, ok := config["to"].([]interface{}); ok {
		for _, addr := range to {
			if s, ok := addr.(string); ok && s != "" {
				ec.To = append(ec.To, s)
			}
		}
	} else if to, ok := config["to"].(string); ok && to != "" {
		ec.To = strings.Split(to, ",")
	}
	if len(ec.To) == 0 {
		return nil, fmt.Errorf("at least one recipient (to) is required")
	}

	// Optional fields
	if port, ok := config["smtp_port"].(float64); ok {
		ec.SMTPPort = int(port)
	} else if port, ok := config["smtp_port"].(int); ok {
		ec.SMTPPort = port
	}

	if username, ok := config["username"].(string); ok {
		ec.Username = username
	}
	if password, ok := config["password"].(string); ok {
		ec.Password = password
	}
	if fromName, ok := config["from_name"].(string); ok {
		ec.FromName = fromName
	}

	// Parse CC recipients
	if cc, ok := config["cc"].([]interface{}); ok {
		for _, addr := range cc {
			if s, ok := addr.(string); ok && s != "" {
				ec.CC = append(ec.CC, s)
			}
		}
	} else if cc, ok := config["cc"].(string); ok && cc != "" {
		ec.CC = strings.Split(cc, ",")
	}

	// TLS options
	if useTLS, ok := config["use_tls"].(bool); ok {
		ec.UseTLS = useTLS
	}
	if useSTARTTLS, ok := config["use_starttls"].(bool); ok {
		ec.UseSTARTTLS = useSTARTTLS
	}
	if skipVerify, ok := config["skip_verify"].(bool); ok {
		ec.SkipVerify = skipVerify
	}
	if templateName, ok := config["template_name"].(string); ok {
		ec.TemplateName = templateName
	}

	return ec, nil
}

// sendEmailNotification sends a notification via email
func sendEmailNotification(ctx context.Context, alert *UnifiedAlert, config map[string]interface{}) error {
	emailConfig, err := parseEmailConfig(config)
	if err != nil {
		return fmt.Errorf("invalid email configuration: %w", err)
	}

	// Build email content
	subject := fmt.Sprintf("[%s] Alert: %s - %s", strings.ToUpper(alert.Severity), alert.AlertName, alert.Status)
	htmlBody := buildEmailHTMLBody(alert)
	textBody := formatAlertMessage(alert)

	// Build email message
	msg := buildEmailMessage(emailConfig, subject, textBody, htmlBody)

	// Send email
	if err := sendSMTPEmail(ctx, emailConfig, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Email notification sent successfully for alert %s to %v", alert.ID, emailConfig.To)
	return nil
}

// buildEmailMessage builds a MIME multipart email message
func buildEmailMessage(config *EmailConfig, subject, textBody, htmlBody string) []byte {
	var buf bytes.Buffer

	// From header
	if config.FromName != "" {
		buf.WriteString(fmt.Sprintf("From: %s <%s>\r\n", config.FromName, config.From))
	} else {
		buf.WriteString(fmt.Sprintf("From: %s\r\n", config.From))
	}

	// To header
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(config.To, ", ")))

	// CC header
	if len(config.CC) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(config.CC, ", ")))
	}

	// Subject
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))

	// Date
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))

	// MIME headers
	boundary := "----=_Part_Alert_Notification"
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	buf.WriteString("\r\n")

	// Plain text part
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(textBody)
	buf.WriteString("\r\n")

	// HTML part
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(htmlBody)
	buf.WriteString("\r\n")

	// End boundary
	buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return buf.Bytes()
}

// buildEmailHTMLBody builds an HTML email body for the alert
func buildEmailHTMLBody(alert *UnifiedAlert) string {
	// Determine header color based on severity
	severityColor := "#17a2b8" // default info blue
	severityBg := "#17a2b8"
	textColor := "white"
	switch strings.ToLower(alert.Severity) {
	case "critical":
		severityColor = "#dc3545"
		severityBg = "#dc3545"
	case "high":
		severityColor = "#fd7e14"
		severityBg = "#fd7e14"
	case "warning":
		severityColor = "#ffc107"
		severityBg = "#ffc107"
		textColor = "#212529"
	}

	// Status badge color
	statusColor := "#28a745" // green for resolved
	if strings.ToLower(alert.Status) == "firing" {
		statusColor = "#dc3545" // red for firing
	}

	// Build labels HTML
	var labelsHTML string
	for k, v := range alert.Labels {
		labelsHTML += fmt.Sprintf(`<span style="display:inline-block;background:#e9ecef;padding:4px 10px;border-radius:4px;font-size:12px;margin:2px;">%s=%s</span>`, k, v)
	}

	// Get description
	description := ""
	if desc, ok := alert.Annotations["description"]; ok {
		description = desc
	} else if summary, ok := alert.Annotations["summary"]; ok {
		description = summary
	}

	// Build HTML directly without template to avoid any parsing issues
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0;padding:0;background-color:#f4f4f4;font-family:Arial,Helvetica,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f4;padding:20px;">
<tr>
<td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.1);">
<!-- Header -->
<tr>
<td style="background-color:%s;padding:25px 30px;">
<h1 style="margin:0;color:%s;font-size:22px;font-weight:600;">🔔 %s</h1>
<p style="margin:10px 0 0 0;color:%s;font-size:14px;opacity:0.9;">
<span style="display:inline-block;background-color:%s;color:white;padding:3px 10px;border-radius:12px;font-size:12px;font-weight:bold;">%s</span>
<span style="margin-left:10px;">Severity: <strong>%s</strong></span>
</p>
</td>
</tr>
<!-- Content -->
<tr>
<td style="padding:25px 30px;">
<table width="100%%" cellpadding="0" cellspacing="0">
<!-- Source -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Source</p>
<p style="margin:5px 0 0 0;font-size:15px;color:#212529;">%s</p>
</td>
</tr>
<!-- Time -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Time</p>
<p style="margin:5px 0 0 0;font-size:15px;color:#212529;">%s</p>
</td>
</tr>`,
		severityBg, textColor, alert.AlertName, textColor,
		statusColor, strings.ToUpper(alert.Status), strings.ToUpper(alert.Severity),
		alert.Source, alert.StartsAt.Format("2006-01-02 15:04:05 MST"))

	// Add optional fields
	if alert.ClusterName != "" {
		html += fmt.Sprintf(`
<!-- Cluster -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Cluster</p>
<p style="margin:5px 0 0 0;font-size:15px;color:#212529;">%s</p>
</td>
</tr>`, alert.ClusterName)
	}

	if alert.NodeName != "" {
		html += fmt.Sprintf(`
<!-- Node -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Node</p>
<p style="margin:5px 0 0 0;font-size:15px;color:#212529;">%s</p>
</td>
</tr>`, alert.NodeName)
	}

	if alert.PodName != "" {
		html += fmt.Sprintf(`
<!-- Pod -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Pod</p>
<p style="margin:5px 0 0 0;font-size:15px;color:#212529;">%s</p>
</td>
</tr>`, alert.PodName)
	}

	if alert.WorkloadID != "" {
		html += fmt.Sprintf(`
<!-- Workload -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Workload</p>
<p style="margin:5px 0 0 0;font-size:15px;color:#212529;">%s</p>
</td>
</tr>`, alert.WorkloadID)
	}

	if description != "" {
		html += fmt.Sprintf(`
<!-- Description -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Description</p>
<div style="margin:8px 0 0 0;padding:12px;background-color:#f8f9fa;border-radius:6px;border-left:4px solid %s;">
<p style="margin:0;font-size:14px;color:#212529;line-height:1.5;">%s</p>
</div>
</td>
</tr>`, severityColor, description)
	}

	if labelsHTML != "" {
		html += fmt.Sprintf(`
<!-- Labels -->
<tr>
<td style="padding-bottom:15px;">
<p style="margin:0;font-size:11px;color:#6c757d;text-transform:uppercase;letter-spacing:1px;">Labels</p>
<div style="margin:8px 0 0 0;">%s</div>
</td>
</tr>`, labelsHTML)
	}

	// Close content and add footer
	html += fmt.Sprintf(`
</table>
</td>
</tr>
<!-- Footer -->
<tr>
<td style="background-color:#f8f9fa;padding:20px 30px;border-top:1px solid #e9ecef;">
<p style="margin:0;font-size:12px;color:#6c757d;">
Alert ID: <code style="background:#e9ecef;padding:2px 6px;border-radius:3px;">%s</code>
</p>
<p style="margin:8px 0 0 0;font-size:11px;color:#adb5bd;">
Generated by Primus Lens Alert System
</p>
</td>
</tr>
</table>
</td>
</tr>
</table>
</body>
</html>`, alert.ID)

	return html
}

// sendSMTPEmail sends email via SMTP
func sendSMTPEmail(ctx context.Context, config *EmailConfig, msg []byte) error {
	addr := config.SMTPHost + ":" + strconv.Itoa(config.SMTPPort)

	// Collect all recipients
	recipients := append([]string{}, config.To...)
	recipients = append(recipients, config.CC...)

	// TLS configuration
	tlsConfig := &tls.Config{
		ServerName:         config.SMTPHost,
		InsecureSkipVerify: config.SkipVerify,
	}

	var conn *tls.Conn
	var client *smtp.Client
	var err error

	if config.UseTLS {
		// Direct TLS connection (port 465)
		conn, err = tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to connect with TLS: %w", err)
		}
		defer conn.Close()

		client, err = smtp.NewClient(conn, config.SMTPHost)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
	} else {
		// Plain connection with optional STARTTLS
		client, err = smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}

		// Try STARTTLS if configured
		if config.UseSTARTTLS {
			if ok, _ := client.Extension("STARTTLS"); ok {
				if err = client.StartTLS(tlsConfig); err != nil {
					return fmt.Errorf("failed to start TLS: %w", err)
				}
			}
		}
	}
	defer client.Close()

	// Authenticate if credentials provided
	if config.Username != "" && config.Password != "" {
		auth := smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err = client.Mail(config.From); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, rcpt := range recipients {
		rcpt = strings.TrimSpace(rcpt)
		if rcpt == "" {
			continue
		}
		if err = client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("failed to add recipient %s: %w", rcpt, err)
		}
	}

	// Send message body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data writer: %w", err)
	}

	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
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
