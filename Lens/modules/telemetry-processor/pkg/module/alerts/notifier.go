// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
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
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background-color: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .header { padding: 20px; color: white; }
        .header.critical { background: linear-gradient(135deg, #dc3545 0%, #c82333 100%); }
        .header.high { background: linear-gradient(135deg, #fd7e14 0%, #e55a00 100%); }
        .header.warning { background: linear-gradient(135deg, #ffc107 0%, #d39e00 100%); color: #212529; }
        .header.info { background: linear-gradient(135deg, #17a2b8 0%, #138496 100%); }
        .header h1 { margin: 0 0 5px 0; font-size: 20px; }
        .header .status { font-size: 14px; opacity: 0.9; }
        .content { padding: 20px; }
        .field { margin-bottom: 15px; }
        .field-label { font-size: 12px; color: #6c757d; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px; }
        .field-value { font-size: 14px; color: #212529; }
        .labels { display: flex; flex-wrap: wrap; gap: 5px; }
        .label { background: #e9ecef; padding: 3px 8px; border-radius: 4px; font-size: 12px; }
        .footer { padding: 15px 20px; background: #f8f9fa; border-top: 1px solid #e9ecef; font-size: 12px; color: #6c757d; }
        .description { background: #f8f9fa; padding: 12px; border-radius: 4px; margin-top: 10px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header {{.Severity}}">
            <h1>{{.AlertName}}</h1>
            <div class="status">Status: {{.Status}} | Severity: {{.Severity | upper}}</div>
        </div>
        <div class="content">
            <div class="field">
                <div class="field-label">Source</div>
                <div class="field-value">{{.Source}}</div>
            </div>
            <div class="field">
                <div class="field-label">Time</div>
                <div class="field-value">{{.StartsAt}}</div>
            </div>
            {{if .WorkloadID}}
            <div class="field">
                <div class="field-label">Workload</div>
                <div class="field-value">{{.WorkloadID}}</div>
            </div>
            {{end}}
            {{if .PodName}}
            <div class="field">
                <div class="field-label">Pod</div>
                <div class="field-value">{{.PodName}}</div>
            </div>
            {{end}}
            {{if .NodeName}}
            <div class="field">
                <div class="field-label">Node</div>
                <div class="field-value">{{.NodeName}}</div>
            </div>
            {{end}}
            {{if .ClusterName}}
            <div class="field">
                <div class="field-label">Cluster</div>
                <div class="field-value">{{.ClusterName}}</div>
            </div>
            {{end}}
            {{if .Description}}
            <div class="field">
                <div class="field-label">Description</div>
                <div class="description">{{.Description}}</div>
            </div>
            {{end}}
            {{if .Labels}}
            <div class="field">
                <div class="field-label">Labels</div>
                <div class="labels">
                    {{range $key, $value := .Labels}}
                    <span class="label">{{$key}}={{$value}}</span>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>
        <div class="footer">
            Alert ID: {{.ID}} | Generated by Primus Lens Alert System
        </div>
    </div>
</body>
</html>`

	// Prepare template data
	data := map[string]interface{}{
		"ID":          alert.ID,
		"AlertName":   alert.AlertName,
		"Severity":    alert.Severity,
		"Status":      alert.Status,
		"Source":      alert.Source,
		"StartsAt":    alert.StartsAt.Format(time.RFC3339),
		"WorkloadID":  alert.WorkloadID,
		"PodName":     alert.PodName,
		"NodeName":    alert.NodeName,
		"ClusterName": alert.ClusterName,
		"Labels":      alert.Labels,
	}

	// Add description from annotations
	if desc, ok := alert.Annotations["description"]; ok {
		data["Description"] = desc
	} else if summary, ok := alert.Annotations["summary"]; ok {
		data["Description"] = summary
	}

	// Parse and execute template
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
	}

	t, err := template.New("email").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		// Fallback to plain text on template error
		return formatAlertMessage(alert)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return formatAlertMessage(alert)
	}

	return buf.String()
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
