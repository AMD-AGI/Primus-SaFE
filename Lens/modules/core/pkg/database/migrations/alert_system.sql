-- Alert System Database Schema
-- This file defines the database schema for the unified alert system

-- Table: alert_events
-- Stores all alert events from different sources (metric/log/trace)
CREATE TABLE IF NOT EXISTS alert_events (
    id VARCHAR(64) PRIMARY KEY,
    source VARCHAR(20) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'firing',
    
    -- Timestamps
    starts_at TIMESTAMP NOT NULL,
    ends_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Labels and annotations (JSONB for PostgreSQL, JSON for MySQL)
    labels JSONB NOT NULL,
    annotations JSONB,
    
    -- Context information
    workload_id VARCHAR(255),
    pod_name VARCHAR(255),
    pod_id VARCHAR(255),
    node_name VARCHAR(255),
    cluster_name VARCHAR(100),
    
    -- Raw and enriched data
    raw_data JSONB,
    enriched_data JSONB,
    
    -- Routing and notification
    route_key VARCHAR(255),
    notified_at TIMESTAMP,
    notification_status VARCHAR(50)
);

-- Indexes for alert_events
CREATE INDEX IF NOT EXISTS idx_alert_events_source_status ON alert_events(source, status);
CREATE INDEX IF NOT EXISTS idx_alert_events_starts_at ON alert_events(starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_alert_name ON alert_events(alert_name);
CREATE INDEX IF NOT EXISTS idx_alert_events_workload ON alert_events(workload_id);
CREATE INDEX IF NOT EXISTS idx_alert_events_pod ON alert_events(pod_name);
CREATE INDEX IF NOT EXISTS idx_alert_events_node ON alert_events(node_name);
CREATE INDEX IF NOT EXISTS idx_alert_events_labels ON alert_events USING GIN(labels);

-- Table: alert_correlations
-- Stores relationships between multiple alerts
CREATE TABLE IF NOT EXISTS alert_correlations (
    id BIGSERIAL PRIMARY KEY,
    correlation_id VARCHAR(64) NOT NULL,
    alert_id VARCHAR(64) NOT NULL,
    correlation_type VARCHAR(50) NOT NULL,
    correlation_score DOUBLE PRECISION,
    correlation_reason TEXT,
    correlation_metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_alert_correlations_alert FOREIGN KEY (alert_id) REFERENCES alert_events(id) ON DELETE CASCADE
);

-- Indexes for alert_correlations
CREATE INDEX IF NOT EXISTS idx_alert_correlations_correlation_id ON alert_correlations(correlation_id);
CREATE INDEX IF NOT EXISTS idx_alert_correlations_alert_id ON alert_correlations(alert_id);

-- Table: alert_statistics
-- Stores aggregated alert statistics for fast querying
CREATE TABLE IF NOT EXISTS alert_statistics (
    id BIGSERIAL PRIMARY KEY,
    date DATE NOT NULL,
    hour INTEGER,
    alert_name VARCHAR(255) NOT NULL,
    source VARCHAR(20) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    workload_id VARCHAR(255),
    cluster_name VARCHAR(100),
    
    -- Statistics
    firing_count INTEGER DEFAULT 0,
    resolved_count INTEGER DEFAULT 0,
    total_duration_seconds BIGINT DEFAULT 0,
    avg_duration_seconds DOUBLE PRECISION,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Unique constraint for statistics grouping
    CONSTRAINT uk_alert_statistics UNIQUE (date, hour, alert_name, source, workload_id, cluster_name)
);

-- Indexes for alert_statistics
CREATE INDEX IF NOT EXISTS idx_alert_statistics_date ON alert_statistics(date DESC);

-- Table: alert_rules
-- Stores dynamic alert rule configurations
CREATE TABLE IF NOT EXISTS alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    source VARCHAR(20) NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    
    -- Rule definition
    rule_type VARCHAR(50) NOT NULL,
    rule_config JSONB NOT NULL,
    
    -- Alert configuration
    severity VARCHAR(20) DEFAULT 'warning',
    labels JSONB,
    annotations JSONB,
    
    -- Routing configuration
    route_config JSONB,
    
    -- Inhibition rules
    inhibit_rules JSONB,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255)
);

-- Indexes for alert_rules
CREATE INDEX IF NOT EXISTS idx_alert_rules_source_enabled ON alert_rules(source, enabled);
CREATE INDEX IF NOT EXISTS idx_alert_rules_name ON alert_rules(name);

-- Table: alert_silences
-- Stores silence configurations for suppressing alerts
CREATE TABLE IF NOT EXISTS alert_silences (
    id VARCHAR(64) PRIMARY KEY,
    matchers JSONB NOT NULL,
    starts_at TIMESTAMP NOT NULL,
    ends_at TIMESTAMP NOT NULL,
    comment TEXT,
    created_by VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for alert_silences
CREATE INDEX IF NOT EXISTS idx_alert_silences_time_range ON alert_silences(starts_at, ends_at);

-- Table: alert_notifications
-- Stores notification history and status
CREATE TABLE IF NOT EXISTS alert_notifications (
    id BIGSERIAL PRIMARY KEY,
    alert_id VARCHAR(64) NOT NULL,
    
    -- Channel configuration
    channel VARCHAR(50) NOT NULL,
    channel_config JSONB,
    
    -- Notification status
    status VARCHAR(20) NOT NULL,
    sent_at TIMESTAMP,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    
    -- Notification payload
    notification_payload JSONB,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_alert_notifications_alert FOREIGN KEY (alert_id) REFERENCES alert_events(id) ON DELETE CASCADE
);

-- Indexes for alert_notifications
CREATE INDEX IF NOT EXISTS idx_alert_notifications_alert_id ON alert_notifications(alert_id);
CREATE INDEX IF NOT EXISTS idx_alert_notifications_status ON alert_notifications(status);
CREATE INDEX IF NOT EXISTS idx_alert_notifications_created_at ON alert_notifications(created_at DESC);

-- Comments for documentation
COMMENT ON TABLE alert_events IS 'Stores all alert events from different sources (metric/log/trace)';
COMMENT ON TABLE alert_correlations IS 'Stores relationships between multiple alerts';
COMMENT ON TABLE alert_statistics IS 'Aggregated alert statistics for fast querying';
COMMENT ON TABLE alert_rules IS 'Dynamic alert rule configurations';
COMMENT ON TABLE alert_silences IS 'Silence configurations for suppressing alerts';
COMMENT ON TABLE alert_notifications IS 'Notification history and status';

COMMENT ON COLUMN alert_events.source IS 'Alert source: metric, log, or trace';
COMMENT ON COLUMN alert_events.status IS 'Alert status: firing, resolved, or silenced';
COMMENT ON COLUMN alert_events.severity IS 'Alert severity: critical, high, warning, or info';
COMMENT ON COLUMN alert_correlations.correlation_type IS 'Type of correlation: time, entity, causal, or cross_source';
COMMENT ON COLUMN alert_statistics.hour IS 'Hour of day (0-23), NULL for daily aggregates';

