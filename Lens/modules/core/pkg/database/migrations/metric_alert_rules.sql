-- Metric Alert Rules Database Schema
-- This file defines the database schema for metric alert rules that sync with VMRule

-- Table: metric_alert_rules
-- Stores metric-based alert rules that are synchronized to Kubernetes VMRule CRDs
CREATE TABLE IF NOT EXISTS metric_alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    cluster_name VARCHAR(100) NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    
    -- VMRule specification (array of groups)
    groups JSONB NOT NULL,
    
    -- Metadata
    description TEXT,
    labels JSONB,
    
    -- Sync status with Kubernetes
    sync_status VARCHAR(50) DEFAULT 'pending',
    sync_message TEXT,
    last_sync_at TIMESTAMP,
    vmrule_uid VARCHAR(255),
    
    -- VMRule status from Kubernetes
    vmrule_status JSONB,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    
    -- Unique constraint: one rule name per cluster
    CONSTRAINT uk_metric_alert_rule_name_cluster UNIQUE (name, cluster_name)
);

-- Indexes for metric_alert_rules
CREATE INDEX IF NOT EXISTS idx_metric_alert_rules_cluster ON metric_alert_rules(cluster_name);
CREATE INDEX IF NOT EXISTS idx_metric_alert_rules_enabled ON metric_alert_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_metric_alert_rules_sync_status ON metric_alert_rules(sync_status);
CREATE INDEX IF NOT EXISTS idx_metric_alert_rules_created_at ON metric_alert_rules(created_at DESC);

-- Comments for documentation
COMMENT ON TABLE metric_alert_rules IS 'Metric-based alert rules that sync to Kubernetes VMRule CRDs';
COMMENT ON COLUMN metric_alert_rules.source IS 'Always "metric" for metric alert rules, used to distinguish from log alert rules';
COMMENT ON COLUMN metric_alert_rules.sync_status IS 'Sync status: pending/synced/failed';
COMMENT ON COLUMN metric_alert_rules.groups IS 'Array of VMRule groups containing alert rules';
COMMENT ON COLUMN metric_alert_rules.vmrule_status IS 'Status of the VMRule in Kubernetes';
COMMENT ON COLUMN metric_alert_rules.vmrule_uid IS 'UID of the VMRule resource in Kubernetes';

