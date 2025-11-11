-- Migration script for Log Alert Rule System
-- Created: 2025-11-03
-- Description: Creates tables for log-based alert rule system

-- Table: log_alert_rules
-- Stores log alert rule configurations
CREATE TABLE IF NOT EXISTS log_alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    cluster_name VARCHAR(100) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    priority INT DEFAULT 5,
    
    -- Label selectors for matching target objects
    label_selectors JSONB NOT NULL,
    
    -- Match configuration
    match_type VARCHAR(50) NOT NULL, -- pattern/threshold/anomaly/composite
    match_config JSONB NOT NULL,
    
    -- Alert configuration
    severity VARCHAR(20) DEFAULT 'warning',
    alert_template JSONB,
    
    -- Grouping and suppression
    group_by TEXT[],
    group_wait BIGINT DEFAULT 30,
    repeat_interval BIGINT DEFAULT 3600,
    
    -- Routing configuration
    route_config JSONB,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    
    -- Status tracking
    last_triggered_at TIMESTAMP,
    trigger_count BIGINT DEFAULT 0,
    
    -- Constraints
    CONSTRAINT uq_log_alert_rule_name_cluster UNIQUE (name, cluster_name)
);

-- Indexes for log_alert_rules
CREATE INDEX IF NOT EXISTS idx_log_alert_rules_cluster_enabled 
    ON log_alert_rules(cluster_name, enabled);
CREATE INDEX IF NOT EXISTS idx_log_alert_rules_priority 
    ON log_alert_rules(priority DESC);
CREATE INDEX IF NOT EXISTS idx_log_alert_rules_label_selectors 
    ON log_alert_rules USING GIN(label_selectors);
CREATE INDEX IF NOT EXISTS idx_log_alert_rules_created_at 
    ON log_alert_rules(created_at DESC);

-- Table: log_alert_rule_versions
-- Stores version history of rules
CREATE TABLE IF NOT EXISTS log_alert_rule_versions (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL,
    version INT NOT NULL,
    config JSONB NOT NULL,
    status VARCHAR(20) DEFAULT 'draft',
    deployed_at TIMESTAMP,
    created_by VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    change_log TEXT,
    
    CONSTRAINT fk_log_alert_rule_version_rule 
        FOREIGN KEY (rule_id) 
        REFERENCES log_alert_rules(id) 
        ON DELETE CASCADE
);

-- Indexes for log_alert_rule_versions
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_versions_rule_id 
    ON log_alert_rule_versions(rule_id);
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_versions_rule_version 
    ON log_alert_rule_versions(rule_id, version DESC);

-- Table: log_alert_rule_statistics
-- Stores execution statistics for rules
CREATE TABLE IF NOT EXISTS log_alert_rule_statistics (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL,
    date DATE NOT NULL,
    hour INT, -- 0-23, NULL for daily stats
    cluster_name VARCHAR(100) NOT NULL,
    evaluated_count BIGINT DEFAULT 0,
    matched_count BIGINT DEFAULT 0,
    fired_count BIGINT DEFAULT 0,
    avg_eval_time_ms FLOAT,
    max_eval_time_ms FLOAT,
    error_count BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Unique constraint for aggregation
    CONSTRAINT uq_log_alert_rule_stat_unique 
        UNIQUE (rule_id, date, hour, cluster_name)
);

-- Indexes for log_alert_rule_statistics
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_statistics_date 
    ON log_alert_rule_statistics(date DESC);
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_statistics_rule_date 
    ON log_alert_rule_statistics(rule_id, date DESC);

-- Table: log_alert_rule_templates
-- Stores predefined rule templates
CREATE TABLE IF NOT EXISTS log_alert_rule_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    category VARCHAR(50) NOT NULL,
    description TEXT,
    template_config JSONB NOT NULL,
    tags TEXT[],
    is_builtin BOOLEAN DEFAULT false,
    usage_count BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(255)
);

-- Indexes for log_alert_rule_templates
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_templates_category 
    ON log_alert_rule_templates(category);
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_templates_usage_count 
    ON log_alert_rule_templates(usage_count DESC);
CREATE INDEX IF NOT EXISTS idx_log_alert_rule_templates_is_builtin 
    ON log_alert_rule_templates(is_builtin);

-- Comments for documentation
COMMENT ON TABLE log_alert_rules IS 'Stores log-based alert rule configurations';
COMMENT ON TABLE log_alert_rule_versions IS 'Stores version history of log alert rules';
COMMENT ON TABLE log_alert_rule_statistics IS 'Stores execution statistics for log alert rules';
COMMENT ON TABLE log_alert_rule_templates IS 'Stores predefined log alert rule templates';

COMMENT ON COLUMN log_alert_rules.label_selectors IS 'JSON array of label selectors for matching logs';
COMMENT ON COLUMN log_alert_rules.match_type IS 'Type of matching: pattern/threshold/anomaly/composite';
COMMENT ON COLUMN log_alert_rules.match_config IS 'Match configuration including patterns, thresholds, etc';
COMMENT ON COLUMN log_alert_rules.group_wait IS 'Group wait time in seconds';
COMMENT ON COLUMN log_alert_rules.repeat_interval IS 'Repeat notification interval in seconds';

-- Grant permissions (adjust as needed for your environment)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO lens_user;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO lens_user;

