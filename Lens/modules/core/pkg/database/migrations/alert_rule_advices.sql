-- Migration script for Alert Rule Advice System
-- Created: 2025-11-03
-- Description: Creates tables for alert rule advice system used in cluster inspection and recommendation

-- Table: alert_rule_advices
-- Stores recommended alert rules discovered during cluster inspection
CREATE TABLE IF NOT EXISTS alert_rule_advices (
    id BIGSERIAL PRIMARY KEY,
    
    -- Rule type and basic info
    rule_type VARCHAR(20) NOT NULL, -- log/metric
    name VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(50) NOT NULL, -- performance/error/resource/security/availability
    
    -- Target information
    cluster_name VARCHAR(100) NOT NULL,
    target_resource VARCHAR(50), -- workload/node/namespace/pod
    target_name VARCHAR(255),
    
    -- Rule configuration (flexible JSONB for different rule types)
    rule_config JSONB NOT NULL,
    
    -- Severity and priority
    severity VARCHAR(20) DEFAULT 'warning', -- critical/warning/info
    priority INT DEFAULT 5, -- 1-10
    
    -- Reasoning and evidence
    reason TEXT,
    evidence JSONB,
    
    -- Status tracking
    status VARCHAR(20) DEFAULT 'pending', -- pending/reviewed/accepted/rejected/applied
    reviewed_by VARCHAR(255),
    reviewed_at TIMESTAMP,
    review_notes TEXT,
    
    -- Applied rule tracking
    applied_rule_id BIGINT,
    applied_at TIMESTAMP,
    
    -- Inspection metadata
    inspection_id VARCHAR(100),
    inspection_time TIMESTAMP NOT NULL,
    
    -- Tags and categorization
    tags TEXT[],
    
    -- Confidence score (0.0-1.0)
    confidence_score FLOAT DEFAULT 0.5,
    
    -- Expiry
    expires_at TIMESTAMP,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(255)
);

-- Indexes for alert_rule_advices
CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_rule_type 
    ON alert_rule_advices(rule_type);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_category 
    ON alert_rule_advices(category);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_cluster_name 
    ON alert_rule_advices(cluster_name);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_status 
    ON alert_rule_advices(status);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_inspection_id 
    ON alert_rule_advices(inspection_id);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_created_at 
    ON alert_rule_advices(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_expires_at 
    ON alert_rule_advices(expires_at);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_rule_config 
    ON alert_rule_advices USING GIN(rule_config);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_evidence 
    ON alert_rule_advices USING GIN(evidence);

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_alert_rule_advices_cluster_status 
    ON alert_rule_advices(cluster_name, status, created_at DESC);

-- Table: alert_rule_advice_statistics
-- Stores aggregated statistics for alert rule advices
CREATE TABLE IF NOT EXISTS alert_rule_advice_statistics (
    id BIGSERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    date DATE NOT NULL,
    
    -- Counts by rule type
    log_rule_count BIGINT DEFAULT 0,
    metric_rule_count BIGINT DEFAULT 0,
    
    -- Counts by category
    performance_count BIGINT DEFAULT 0,
    error_count BIGINT DEFAULT 0,
    resource_count BIGINT DEFAULT 0,
    security_count BIGINT DEFAULT 0,
    availability_count BIGINT DEFAULT 0,
    
    -- Counts by status
    pending_count BIGINT DEFAULT 0,
    reviewed_count BIGINT DEFAULT 0,
    accepted_count BIGINT DEFAULT 0,
    rejected_count BIGINT DEFAULT 0,
    applied_count BIGINT DEFAULT 0,
    
    -- Average scores
    avg_confidence_score FLOAT,
    avg_priority FLOAT,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Unique constraint for daily aggregation
    CONSTRAINT uq_alert_rule_advice_stat_cluster_date 
        UNIQUE (cluster_name, date)
);

-- Indexes for alert_rule_advice_statistics
CREATE INDEX IF NOT EXISTS idx_alert_rule_advice_statistics_date 
    ON alert_rule_advice_statistics(date DESC);

CREATE INDEX IF NOT EXISTS idx_alert_rule_advice_statistics_cluster_date 
    ON alert_rule_advice_statistics(cluster_name, date DESC);

-- Comments for documentation
COMMENT ON TABLE alert_rule_advices IS 'Stores recommended alert rules discovered during cluster inspection';
COMMENT ON TABLE alert_rule_advice_statistics IS 'Stores aggregated statistics for alert rule advices';

COMMENT ON COLUMN alert_rule_advices.rule_type IS 'Type of alert rule: log or metric';
COMMENT ON COLUMN alert_rule_advices.category IS 'Advice category: performance/error/resource/security/availability';
COMMENT ON COLUMN alert_rule_advices.rule_config IS 'Complete rule configuration in JSON format';
COMMENT ON COLUMN alert_rule_advices.evidence IS 'Supporting data, logs, metrics that justify this recommendation';
COMMENT ON COLUMN alert_rule_advices.status IS 'Current status: pending/reviewed/accepted/rejected/applied';
COMMENT ON COLUMN alert_rule_advices.confidence_score IS 'Confidence score (0.0-1.0) indicating how certain the recommendation is';
COMMENT ON COLUMN alert_rule_advices.inspection_id IS 'ID of the inspection run that generated this advice';
COMMENT ON COLUMN alert_rule_advices.applied_rule_id IS 'ID of the actual alert rule if this advice was applied';

-- Grant permissions (adjust as needed for your environment)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO lens_user;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO lens_user;

