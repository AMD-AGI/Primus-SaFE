-- Notification Channels Database Schema
-- This file defines the database schema for reusable notification channel configurations

-- Table: notification_channels
-- Stores notification channel configurations that can be referenced by alert rules
CREATE TABLE IF NOT EXISTS notification_channels (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    type VARCHAR(50) NOT NULL,  -- email, webhook, dingtalk, wechat, slack, alertmanager
    enabled BOOLEAN DEFAULT TRUE,
    config JSONB NOT NULL,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255),
    updated_by VARCHAR(255)
);

-- Indexes for notification_channels
CREATE INDEX IF NOT EXISTS idx_notification_channels_type ON notification_channels(type);
CREATE INDEX IF NOT EXISTS idx_notification_channels_enabled ON notification_channels(enabled);
CREATE INDEX IF NOT EXISTS idx_notification_channels_name ON notification_channels(name);

-- Comments for documentation
COMMENT ON TABLE notification_channels IS 'Stores reusable notification channel configurations';
COMMENT ON COLUMN notification_channels.type IS 'Channel type: email, webhook, dingtalk, wechat, slack, alertmanager';
COMMENT ON COLUMN notification_channels.config IS 'Channel-specific configuration (SMTP settings for email, URL for webhook, etc.)';

-- Example configurations:
--
-- Email channel config:
-- {
--   "smtp_host": "smtp.example.com",
--   "smtp_port": 587,
--   "username": "alerts@example.com",
--   "password": "your-password",
--   "from": "alerts@example.com",
--   "from_name": "Primus-Lens Alerts",
--   "use_starttls": true,
--   "skip_verify": false
-- }
--
-- Webhook channel config:
-- {
--   "url": "https://hooks.example.com/webhook",
--   "method": "POST",
--   "headers": {"Authorization": "Bearer xxx"},
--   "timeout": 30
-- }
--
-- DingTalk channel config:
-- {
--   "webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=xxx",
--   "secret": "SECxxx"
-- }
--
-- Slack channel config:
-- {
--   "webhook_url": "https://hooks.slack.com/services/xxx",
--   "channel": "#alerts",
--   "username": "Primus-Lens"
-- }
