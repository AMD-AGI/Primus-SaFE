-- Migration for Alert Silences V2 - Enhanced silence management with resource filters and time windows
-- This migration updates the alert_silences table to support advanced silence features

-- Drop existing alert_silences table if exists (backup data first if needed)
-- DROP TABLE IF EXISTS alert_silences CASCADE;

-- Create enhanced alert_silences table
CREATE TABLE IF NOT EXISTS alert_silences (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    cluster_name VARCHAR(100),
    enabled BOOLEAN DEFAULT TRUE,
    
    -- Silence type: resource/label/alert_name/expression
    silence_type VARCHAR(50) NOT NULL,
    
    -- Resource-based silence filters
    resource_filters JSONB,
    
    -- Label-based silence matchers
    label_matchers JSONB,
    
    -- Alert name-based silence
    alert_names JSONB,
    
    -- Expression-based silence (advanced)
    match_expression TEXT,
    
    -- Time configuration
    starts_at TIMESTAMP NOT NULL,
    ends_at TIMESTAMP, -- NULL means permanent silence
    time_windows JSONB, -- Recurring time windows
    
    -- Metadata
    reason TEXT,
    ticket_url VARCHAR(500),
    
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for alert_silences
CREATE INDEX IF NOT EXISTS idx_alert_silences_cluster ON alert_silences(cluster_name);
CREATE INDEX IF NOT EXISTS idx_alert_silences_enabled ON alert_silences(enabled);
CREATE INDEX IF NOT EXISTS idx_alert_silences_type ON alert_silences(silence_type);
CREATE INDEX IF NOT EXISTS idx_alert_silences_time_range ON alert_silences(starts_at, ends_at);
CREATE INDEX IF NOT EXISTS idx_alert_silences_active ON alert_silences(enabled, starts_at, ends_at) WHERE enabled = true;

-- Create silenced_alerts table for audit trail
CREATE TABLE IF NOT EXISTS silenced_alerts (
    id BIGSERIAL PRIMARY KEY,
    silence_id VARCHAR(64) NOT NULL,
    alert_id VARCHAR(64) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    cluster_name VARCHAR(100),
    silenced_at TIMESTAMP NOT NULL,
    reason TEXT,
    alert_data JSONB,
    
    CONSTRAINT fk_silence FOREIGN KEY (silence_id) REFERENCES alert_silences(id) ON DELETE CASCADE
);

-- Create indexes for silenced_alerts
CREATE INDEX IF NOT EXISTS idx_silenced_alerts_silence_id ON silenced_alerts(silence_id);
CREATE INDEX IF NOT EXISTS idx_silenced_alerts_alert_id ON silenced_alerts(alert_id);
CREATE INDEX IF NOT EXISTS idx_silenced_alerts_alert_name ON silenced_alerts(alert_name);
CREATE INDEX IF NOT EXISTS idx_silenced_alerts_silenced_at ON silenced_alerts(silenced_at DESC);

-- Add comments
COMMENT ON TABLE alert_silences IS 'Enhanced alert silence rules supporting resource filters, label matchers, and time windows';
COMMENT ON TABLE silenced_alerts IS 'Audit trail of alerts that were silenced';

COMMENT ON COLUMN alert_silences.silence_type IS 'Type of silence: resource/label/alert_name/expression';
COMMENT ON COLUMN alert_silences.resource_filters IS 'JSON array of resource filters (e.g., specific nodes, GPUs, workloads)';
COMMENT ON COLUMN alert_silences.label_matchers IS 'JSON array of label matchers for flexible filtering';
COMMENT ON COLUMN alert_silences.alert_names IS 'JSON array of alert names to silence';
COMMENT ON COLUMN alert_silences.time_windows IS 'JSON array of recurring time windows (e.g., nightly maintenance)';
COMMENT ON COLUMN alert_silences.ends_at IS 'End time of silence, NULL means permanent until manually disabled';

-- If migrating from old table, you may need to run data migration queries here
-- Example:
-- INSERT INTO alert_silences (id, name, starts_at, ends_at, ...)
-- SELECT ... FROM old_alert_silences;

