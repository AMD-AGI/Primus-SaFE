create database "primus-lens";
-- Step 1: Grant privileges to user 'primus-lens' on database 'primus-lens'
GRANT CONNECT ON DATABASE "primus-lens" TO "primus-lens";
GRANT CREATE, TEMPORARY ON DATABASE "primus-lens" TO "primus-lens";

-- Optional: ensure ownership or schema-level permissions
-- You can also grant privileges on the public schema if needed
GRANT USAGE ON SCHEMA public TO "primus-lens";
GRANT CREATE ON SCHEMA public TO "primus-lens";

ALTER SYSTEM SET max_connections = 500;

-- Step 2: Connect to the database
\c "primus-lens"

-- Step 3: Create example tables (customize as needed)
-- Step 1: Grant privileges to user 'primus-lens' on database 'primus-lens'
GRANT CONNECT ON DATABASE "primus-lens" TO "primus-lens";
GRANT CREATE, TEMPORARY ON DATABASE "primus-lens" TO "primus-lens";

-- Optional: ensure ownership or schema-level permissions
-- You can also grant privileges on the public schema if needed
GRANT USAGE ON SCHEMA public TO "primus-lens";
GRANT CREATE ON SCHEMA public TO "primus-lens";

-- Step 2: Connect to the database
\c "primus-lens"

-- Step 3: Create example tables
create table node
(
    id                 serial
        constraint node_pk
            primary key,
    name               varchar(65),
    address            varchar(32),
    gpu_name           varchar(128),
    gpu_allocation     integer,
    gpu_count          integer,
    gpu_utilization    double precision,
    status             varchar(64),
    created_at         timestamp with time zone,
    updated_at         timestamp with time zone,
    cpu                varchar(64),
    cpu_count          integer,
    memory             varchar(64),
    k8s_version        varchar(64),
    k8s_status         varchar(64),
    kubelet_version    varchar(64),
    containerd_version varchar(64),
    os                 varchar(64),
    driver_version     varchar(64)
);

alter table node
    owner to "primus-lens";

create table gpu_device
(
    id               serial
        constraint gpu_device_pk
            primary key,
    node_id          integer,
    gpu_id           integer,
    gpu_model        varchar(64),
    memory           integer,
    utilization      double precision,
    temperature      double precision,
    power            double precision,
    serial           varchar(128),
    rdma_device_name varchar(64),
    rdma_guid        varchar(64),
    rdma_lid         varchar(64),
    created_at       timestamp with time zone,
    updated_at       timestamp with time zone,
    numa_node        integer,
    numa_affinity    integer
);

alter table gpu_device
    owner to "primus-lens";

create unique index idx_gpu_device_node_gpu_unique
    on gpu_device (node_id, gpu_id);

create index idx_gpu_device_node_id
    on gpu_device (node_id);

create table gpu_pods
(
    id            serial
        constraint gpu_pods_pk
            primary key,
    namespace     varchar(64),
    name          varchar(64),
    node_name     varchar(64),
    uid           varchar(64),
    gpu_allocated integer,
    phase         varchar(64),
    deleted       boolean,
    created_at    timestamp with time zone,
    updated_at    timestamp with time zone,
    owner_uid     varchar(64),
    running       boolean
);

alter table gpu_pods
    owner to "primus-lens";

create table gpu_pods_event
(
    id            serial
        constraint gpu_pods_event_pk
            primary key,
    pod_uuid      varchar(64),
    pod_phase     varchar(64),
    event_type    varchar(64),
    created_at    timestamp with time zone,
    restart_count integer
);

alter table gpu_pods_event
    owner to "primus-lens";

create table gpu_workload
(
    id            serial
        constraint gpu_workload_pk
            primary key,
    group_version varchar(128),
    kind          varchar(64),
    namespace     varchar(64),
    name          varchar(64),
    uid           varchar(128),
    parent_uid    varchar(128),
    gpu_request   integer,
    created_at    timestamp with time zone,
    updated_at    timestamp with time zone,
    deleted_at    timestamp with time zone,
    end_at        timestamp with time zone,
    status        varchar(64),
    source        varchar(64),
    labels        jsonb
);

alter table gpu_workload
    owner to "primus-lens";

alter table gpu_workload add column annotations jsonb;

create table pod_snapshot
(
    id               serial
        constraint pod_snapshot_pk
            primary key,
    pod_uid          varchar(64),
    pod_name         varchar(64),
    namespace        varchar(64),
    spec             jsonb,
    metadata         jsonb,
    status           jsonb,
    created_at       timestamp with time zone,
    resource_version integer
);

alter table pod_snapshot
    owner to "primus-lens";

create index pod_snapshot_pod_uid_resource_version_index
    on pod_snapshot (pod_uid asc, resource_version desc);

create table gpu_workload_snapshot
(
    id               serial
        constraint gpu_workload_snapshot_pk
            primary key,
    uid              varchar(64),
    group_version    varchar(128),
    kind             varchar(64),
    name             varchar(64),
    namespace        varchar(64),
    metadata         jsonb,
    detail           jsonb,
    resource_version integer,
    created_at       timestamp with time zone
);

alter table gpu_workload_snapshot
    owner to "primus-lens";

create table workload_pod_reference
(
    id           serial,
    workload_uid varchar(64),
    pod_uid      varchar(64),
    created_at   timestamp with time zone
);

alter table workload_pod_reference
    owner to "primus-lens";

create table fault
(
    id       serial
        constraint fault_pk
            primary key,
    code     varchar(64),
    kind     varchar(64),
    name     varchar(64),
    detail   jsonb,
    plugin   varchar(64),
    start_at timestamp with time zone,
    end_at   timestamp with time zone,
    level    integer
);

alter table fault
    owner to "primus-lens";

create table workload_resource
(
    id               serial
        constraint workload_resource_pk
            primary key,
    workload_uid     varchar(64),
    gpu_time_seconds double precision,
    gpu_request      integer,
    gpu_model        varchar(64),
    pod_count        integer,
    updated_at       timestamp with time zone
);

alter table workload_resource
    owner to "primus-lens";

create table pod_resource
(
    id            serial
        constraint pod_resource_pk
            primary key,
    uid           varchar(64),
    gpu_model     varchar(64),
    gpu_allocated integer,
    created_at    timestamp with time zone,
    end_at        timestamp with time zone
);

alter table pod_resource
    owner to "primus-lens";

create table node_container
(
    id             serial
        constraint node_container_pk
            primary key,
    container_id   varchar(128),
    container_name varchar(128),
    pod_uid        varchar(128),
    pod_name       varchar(128),
    pod_namespace  varchar(128),
    status         varchar(128),
    created_at     timestamp with time zone,
    updated_at     timestamp with time zone,
    node_name      varchar(128),
    source         varchar(128)
);

alter table node_container
    owner to "primus-lens";

create unique index idx_node_container_container_id_unique
    on node_container (container_id);

create index idx_node_container_pod_uid
    on node_container (pod_uid);

create table node_container_devices
(
    id            serial
        constraint node_container_devices_pk
            primary key,
    container_id  varchar(64),
    device_type   varchar(64),
    device_name   varchar(64),
    device_no     integer,
    device_uuid   varchar(64),
    created_at    timestamp with time zone,
    restart_count integer,
    updated_at    timestamp with time zone
);

alter table node_container_devices
    owner to "primus-lens";

create table node_container_event
(
    id           serial
        constraint node_container_event_pk
            primary key,
    container_id varchar(64),
    event_type   varchar(64),
    created_at   timestamp with time zone
);

alter table node_container_event
    owner to "primus-lens";

create index node_container_event_container_id_index
    on node_container_event (container_id);

create table training_performance
(
    id           serial
        constraint training_performance_pk
            primary key,
    pod_uuid     varchar(256),
    performance  jsonb,
    iteration    integer,
    created_at   timestamp with time zone,
    serial       integer,
    workload_uid varchar(256)
);

alter table training_performance
    owner to "primus-lens";

create table ai_workload_metadata
(
    id           serial
        constraint ai_workload_metadata_pk
            primary key,
    workload_uid varchar(256),
    type         varchar(256),
    framework    varchar(256),
    metadata     jsonb,
    created_at   timestamp with time zone
);

alter table ai_workload_metadata
    owner to "primus-lens";

create table workload_event
(
    id                   serial
        constraint workload_event_pk
            primary key,
    workload_uid         varchar(256),
    type                 varchar(256),
    run_serial           integer,
    created_at           timestamp with time zone,
    pod_uid              varchar(256),
    nearest_workload_uid varchar(256)
);

alter table workload_event
    owner to "primus-lens";

create table rdma_device
(
    id         serial
        constraint rdma_device_pk
            primary key,
    node_id    integer,
    ifname     varchar(512),
    node_guid  varchar(256),
    if_index   integer,
    fw         varchar(64),
    node_type  varchar(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

alter table rdma_device
    owner to "primus-lens";

create table node_device_changelog
(
    id          serial
        constraint node_device_changelog_pk
            primary key,
    node_id     integer,
    node_name   varchar(256),
    device_type varchar(32),
    device_name varchar(256),
    device_uuid varchar(256),
    op          varchar(32),
    created_at  timestamp with time zone
);

alter table node_device_changelog
    owner to "primus-lens";

create table storage
(
    id         serial
        constraint storage_pk
            primary key,
    name       varchar(64),
    kind       varchar(64),
    config     jsonb,
    source     varchar(64),
    status     varchar(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

alter table storage
    owner to "primus-lens";

alter table public.gpu_pods
    add ip varchar(32);

alter table public.node
    add taints jsonb;

-- Add data_source column to training_performance table
-- This column is used to distinguish the source of training performance data

-- Create enum type for data source if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'training_data_source') THEN
        CREATE TYPE training_data_source AS ENUM ('log', 'wandb', 'tensorflow');
    END IF;
END $$;

-- Add data_source column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'training_performance' 
        AND column_name = 'data_source'
    ) THEN
        ALTER TABLE training_performance 
        ADD COLUMN data_source training_data_source DEFAULT 'log';
    END IF;
END $$;

-- Add index for data_source queries (useful for filtering by source)
CREATE INDEX IF NOT EXISTS idx_training_performance_data_source
ON training_performance(data_source);

-- Add composite index for common queries (workload + data_source + iteration)
CREATE INDEX IF NOT EXISTS idx_training_performance_workload_source_iteration
ON training_performance(workload_uid, data_source, iteration DESC);

-- Add comments for documentation
COMMENT ON COLUMN training_performance.data_source IS 'Source of training performance data: log (parsed from logs), wandb (from W&B API), or tensorflow (from TensorFlow/TensorBoard)';
COMMENT ON INDEX idx_training_performance_data_source IS 'Index for filtering training performance data by source';
COMMENT ON INDEX idx_training_performance_workload_source_iteration IS 'Composite index for efficient queries by workload, data source, and iteration';

-- Note: Default value is 'log' for backward compatibility with existing data
-- Application layer should explicitly set data_source when inserting new records

-- Add image_prefix column to ai_workload_metadata table
-- This column is used for efficient candidate workload queries in metadata reuse

-- Add image_prefix column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ai_workload_metadata' 
        AND column_name = 'image_prefix'
    ) THEN
        ALTER TABLE ai_workload_metadata 
        ADD COLUMN image_prefix TEXT;
    END IF;
END $$;

-- Add index for image_prefix + created_at queries (used in reuse matching)
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_reuse_query
ON ai_workload_metadata(image_prefix, created_at DESC)
WHERE image_prefix IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN ai_workload_metadata.image_prefix IS 'Image repository address without tag, extracted from container image for efficient similarity matching';
COMMENT ON INDEX idx_ai_workload_metadata_reuse_query IS 'Composite index for efficient reuse candidate queries by image_prefix and time window';

-- Note: image_prefix should be populated by application layer when creating/updating workload metadata
-- Example: registry.example.com/primus:v1.2.3 -> registry.example.com/primus

-- AI Workload Metadata Indexes
-- This file adds indexes to the existing ai_workload_metadata table
-- for framework detection and metadata reuse optimization

-- Add GIN index for JSONB metadata queries
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_metadata_gin 
ON ai_workload_metadata USING GIN (metadata jsonb_path_ops);

-- Add index for time window queries (used in reuse matching)
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_created_at
ON ai_workload_metadata(created_at DESC);

-- Add index for workload_uid queries
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_workload_uid
ON ai_workload_metadata(workload_uid);

-- Comments for documentation
COMMENT ON INDEX idx_ai_workload_metadata_metadata_gin IS 'GIN index for efficient JSONB queries on metadata field';
COMMENT ON INDEX idx_ai_workload_metadata_created_at IS 'Index for time-based queries, used in workload reuse matching';
COMMENT ON INDEX idx_ai_workload_metadata_workload_uid IS 'Index for fast workload_uid lookups';

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

-- Create checkpoint_event table for tracking checkpoint operations
CREATE TABLE IF NOT EXISTS checkpoint_event (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(255) NOT NULL,
    pod_uuid VARCHAR(255),
    iteration INTEGER NOT NULL,
    checkpoint_path TEXT,
    event_type VARCHAR(50) NOT NULL,  -- start_saving, end_saving, loading
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    duration_ms BIGINT DEFAULT 0,
    size_bytes BIGINT DEFAULT 0,
    is_fast_ckpt BOOLEAN DEFAULT false,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    serial INTEGER DEFAULT 1,
    status VARCHAR(50) DEFAULT 'in_progress'  -- success, failed, in_progress
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_workload_uid ON checkpoint_event(workload_uid);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_iteration ON checkpoint_event(iteration);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_type ON checkpoint_event(event_type);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_status ON checkpoint_event(status);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_created_at ON checkpoint_event(created_at);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_workload_iteration ON checkpoint_event(workload_uid, iteration);

-- Add comment
COMMENT ON TABLE checkpoint_event IS 'Tracks checkpoint save/load events from training logs';
COMMENT ON COLUMN checkpoint_event.workload_uid IS 'Reference to the workload';
COMMENT ON COLUMN checkpoint_event.event_type IS 'Type of checkpoint event: start_saving, end_saving, loading';
COMMENT ON COLUMN checkpoint_event.duration_ms IS 'Duration of checkpoint operation in milliseconds';
COMMENT ON COLUMN checkpoint_event.is_fast_ckpt IS 'Whether this is a fast checkpoint';
COMMENT ON COLUMN checkpoint_event.metadata IS 'Additional metadata extracted from logs';

-- Cluster Overview Cache Database Schema
-- This file defines the database schema for caching cluster overview data
-- to improve API performance for large clusters

-- Table: cluster_overview_cache
-- Stores pre-computed cluster overview statistics
CREATE TABLE IF NOT EXISTS cluster_overview_cache (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    
    -- Node statistics
    total_nodes INT NOT NULL DEFAULT 0,
    healthy_nodes INT NOT NULL DEFAULT 0,
    faulty_nodes INT NOT NULL DEFAULT 0,
    fully_idle_nodes INT NOT NULL DEFAULT 0,
    partially_idle_nodes INT NOT NULL DEFAULT 0,
    busy_nodes INT NOT NULL DEFAULT 0,
    
    -- GPU statistics
    allocation_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Storage statistics
    storage_total_space DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_used_space DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_usage_percentage DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_total_inodes DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_used_inodes DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_inodes_usage_percentage DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_read_bandwidth DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_write_bandwidth DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- RDMA statistics
    rdma_total_tx DOUBLE PRECISION NOT NULL DEFAULT 0,
    rdma_total_rx DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for cluster_overview_cache
CREATE UNIQUE INDEX IF NOT EXISTS idx_cluster_overview_cache_cluster_name ON cluster_overview_cache(cluster_name);
CREATE INDEX IF NOT EXISTS idx_cluster_overview_cache_updated_at ON cluster_overview_cache(updated_at DESC);

-- Comment on table
COMMENT ON TABLE cluster_overview_cache IS 'Cached cluster overview statistics to improve API performance';
COMMENT ON COLUMN cluster_overview_cache.cluster_name IS 'Name of the cluster';
COMMENT ON COLUMN cluster_overview_cache.total_nodes IS 'Total number of GPU nodes';
COMMENT ON COLUMN cluster_overview_cache.allocation_rate IS 'GPU allocation rate (0-100)';
COMMENT ON COLUMN cluster_overview_cache.utilization IS 'GPU utilization rate (0-100)';

-- Detection Conflict Log Database Schema
-- This file defines the database schema for logging framework detection conflicts
-- used for analysis and optimization of detection strategies

-- Table: detection_conflict_log
-- Records conflicts when different detection sources disagree on framework
CREATE TABLE IF NOT EXISTS detection_conflict_log (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    source_1 VARCHAR(64) NOT NULL,
    source_2 VARCHAR(64) NOT NULL,
    framework_1 VARCHAR(64) NOT NULL,
    framework_2 VARCHAR(64) NOT NULL,
    confidence_1 DECIMAL(3,2) NOT NULL,
    confidence_2 DECIMAL(3,2) NOT NULL,
    resolution_strategy VARCHAR(64),  -- priority/confidence/time/vote/manual
    resolved_framework VARCHAR(64),
    resolved_confidence DECIMAL(3,2),
    resolved_at TIMESTAMP,
    evidence_1 JSONB,
    evidence_2 JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for workload-specific conflict queries
CREATE INDEX IF NOT EXISTS idx_detection_conflict_workload
ON detection_conflict_log(workload_uid, created_at DESC);

-- Index for source pair analysis
CREATE INDEX IF NOT EXISTS idx_detection_conflict_sources
ON detection_conflict_log(source_1, source_2, created_at DESC);

-- Index for framework pair analysis
CREATE INDEX IF NOT EXISTS idx_detection_conflict_frameworks
ON detection_conflict_log(framework_1, framework_2);

-- Index for time-based queries and cleanup
CREATE INDEX IF NOT EXISTS idx_detection_conflict_created_at
ON detection_conflict_log(created_at DESC);

-- Index for resolution strategy analysis
CREATE INDEX IF NOT EXISTS idx_detection_conflict_resolution
ON detection_conflict_log(resolution_strategy, created_at DESC);

-- GIN index for evidence JSONB queries
CREATE INDEX IF NOT EXISTS idx_detection_conflict_evidence_gin
ON detection_conflict_log USING GIN (evidence_1 jsonb_path_ops);

-- Comments for documentation
COMMENT ON TABLE detection_conflict_log IS 'Log of framework detection conflicts for analysis and debugging';
COMMENT ON COLUMN detection_conflict_log.workload_uid IS 'UID of the workload with conflicting detections';
COMMENT ON COLUMN detection_conflict_log.source_1 IS 'First detection source (e.g., component, log, image)';
COMMENT ON COLUMN detection_conflict_log.source_2 IS 'Second detection source that conflicts with first';
COMMENT ON COLUMN detection_conflict_log.framework_1 IS 'Framework detected by source_1';
COMMENT ON COLUMN detection_conflict_log.framework_2 IS 'Framework detected by source_2 (conflicts with framework_1)';
COMMENT ON COLUMN detection_conflict_log.confidence_1 IS 'Confidence level from source_1 (0.00-1.00)';
COMMENT ON COLUMN detection_conflict_log.confidence_2 IS 'Confidence level from source_2 (0.00-1.00)';
COMMENT ON COLUMN detection_conflict_log.resolution_strategy IS 'Strategy used to resolve the conflict';
COMMENT ON COLUMN detection_conflict_log.resolved_framework IS 'Final framework after conflict resolution';
COMMENT ON COLUMN detection_conflict_log.resolved_confidence IS 'Final confidence after conflict resolution';
COMMENT ON COLUMN detection_conflict_log.resolved_at IS 'Timestamp when the conflict was resolved';
COMMENT ON COLUMN detection_conflict_log.evidence_1 IS 'Evidence/details from source_1';
COMMENT ON COLUMN detection_conflict_log.evidence_2 IS 'Evidence/details from source_2';
COMMENT ON COLUMN detection_conflict_log.created_at IS 'Timestamp when the conflict was detected';

-- Note: Old conflict logs should be cleaned up periodically by application
-- Recommended retention: 90 days
-- Cleanup logic: DELETE FROM detection_conflict_log WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '90 days';

-- Detection Source Priority Database Schema
-- This file defines the database schema for storing detection source priority configuration
-- which determines how different detection sources are weighted and prioritized

-- Table: detection_source_priority
-- Stores priority and confidence configuration for different detection sources
CREATE TABLE IF NOT EXISTS detection_source_priority (
    id SERIAL PRIMARY KEY,
    source_name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128),
    priority INTEGER NOT NULL,  -- Priority level (higher is better)
    base_confidence DECIMAL(3,2) NOT NULL,  -- Base confidence level (0.00-1.00)
    confidence_decay DECIMAL(3,2) DEFAULT 1.0,  -- Decay rate for reused detections (0.00-1.00)
    enabled BOOLEAN NOT NULL DEFAULT true,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for detection_source_priority
CREATE INDEX IF NOT EXISTS idx_detection_source_priority_enabled
ON detection_source_priority(enabled, priority DESC);

CREATE INDEX IF NOT EXISTS idx_detection_source_priority_name
ON detection_source_priority(source_name);

-- Comments for documentation
COMMENT ON TABLE detection_source_priority IS 'Configuration for detection source priority and confidence levels';
COMMENT ON COLUMN detection_source_priority.source_name IS 'Unique source identifier (e.g., user, component, log, image)';
COMMENT ON COLUMN detection_source_priority.display_name IS 'Human-readable source name';
COMMENT ON COLUMN detection_source_priority.priority IS 'Priority level for conflict resolution (higher wins)';
COMMENT ON COLUMN detection_source_priority.base_confidence IS 'Base confidence level for this source (0.00-1.00)';
COMMENT ON COLUMN detection_source_priority.confidence_decay IS 'Decay multiplier for reused detections (e.g., 0.9 means 10% decay)';
COMMENT ON COLUMN detection_source_priority.enabled IS 'Whether this detection source is enabled';
COMMENT ON COLUMN detection_source_priority.description IS 'Description of the detection source';
COMMENT ON COLUMN detection_source_priority.created_at IS 'Timestamp when the configuration was created';
COMMENT ON COLUMN detection_source_priority.updated_at IS 'Timestamp when the configuration was last updated (managed by application)';

-- Insert default detection source priorities
INSERT INTO detection_source_priority (source_name, display_name, priority, base_confidence, confidence_decay, description) VALUES
('user', 'User Annotation', 100, 1.0, 1.0, 'Manual user annotation, highest priority'),
('component', 'Component Detection', 80, 0.85, 1.0, 'Detected by component through image, labels, etc.'),
('reuse', 'Reuse Detection', 75, 0.85, 0.9, 'Reused results from similar workload'),
('log', 'Log Analysis', 60, 0.70, 1.0, 'Identified through log pattern matching'),
('image', 'Image Inference', 40, 0.60, 1.0, 'Inferred from image name only'),
('default', 'Default Inference', 20, 0.30, 1.0, 'Default or unknown case')
ON CONFLICT (source_name) DO NOTHING;

-- Framework Configuration Database Schema
-- This file defines the database schema for storing framework configuration
-- including log patterns, image patterns, and priority settings

-- Table: framework_config
-- Stores framework configuration information for detection
CREATE TABLE IF NOT EXISTS framework_config (
    id SERIAL PRIMARY KEY,
    framework_name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128),
    description TEXT,
    priority INTEGER NOT NULL DEFAULT 50,  -- Priority for conflict resolution (higher is better)
    base_confidence DECIMAL(3,2) NOT NULL DEFAULT 0.70,  -- Base confidence level (0.00-1.00)
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL,  -- Framework-specific configuration (patterns, keys, etc.)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for framework_config
CREATE INDEX IF NOT EXISTS idx_framework_config_enabled 
ON framework_config(enabled, priority DESC);

CREATE INDEX IF NOT EXISTS idx_framework_config_name
ON framework_config(framework_name);

-- Comments for documentation
COMMENT ON TABLE framework_config IS 'Framework configuration for AI workload detection';
COMMENT ON COLUMN framework_config.framework_name IS 'Unique framework identifier (e.g., primus, deepspeed, megatron)';
COMMENT ON COLUMN framework_config.display_name IS 'Human-readable framework name';
COMMENT ON COLUMN framework_config.priority IS 'Priority for conflict resolution, higher value means higher priority';
COMMENT ON COLUMN framework_config.base_confidence IS 'Base confidence level for this framework (0.00-1.00)';
COMMENT ON COLUMN framework_config.enabled IS 'Whether this framework detection is enabled';
COMMENT ON COLUMN framework_config.config IS 'JSONB configuration containing log_patterns, image_patterns, env_keys, etc.';
COMMENT ON COLUMN framework_config.created_at IS 'Timestamp when the configuration was created';
COMMENT ON COLUMN framework_config.updated_at IS 'Timestamp when the configuration was last updated (managed by application)';

-- Insert default framework configurations
INSERT INTO framework_config (framework_name, display_name, priority, base_confidence, config) VALUES
('primus', 'Primus', 80, 0.75, '{
    "log_patterns": [
        {"pattern": "primus-rocm-memory", "weight": 1.0, "category": "memory", "description": "Primus ROCm memory allocation"},
        {"pattern": "PrimusTrainer", "weight": 0.9, "category": "framework", "description": "Primus trainer initialization"},
        {"pattern": "primus\\\\.distributed", "weight": 0.8, "category": "distributed", "description": "Primus distributed training"}
    ],
    "image_patterns": ["primus", "primus-training"],
    "env_keys": ["PRIMUS_CONFIG", "PRIMUS_MASTER_ADDR"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb),
('deepspeed', 'DeepSpeed', 75, 0.75, '{
    "log_patterns": [
        {"pattern": "DeepSpeed Engine", "weight": 1.0, "category": "framework", "description": "DeepSpeed engine initialization"},
        {"pattern": "deepspeed\\\\.runtime", "weight": 0.9, "category": "runtime", "description": "DeepSpeed runtime"}
    ],
    "image_patterns": ["deepspeed"],
    "env_keys": ["DEEPSPEED_CONFIG"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb),
('megatron', 'Megatron-LM', 75, 0.75, '{
    "log_patterns": [
        {"pattern": "Megatron-LM", "weight": 1.0, "category": "framework", "description": "Megatron-LM framework"},
        {"pattern": "megatron\\\\.model", "weight": 0.9, "category": "model", "description": "Megatron model initialization"}
    ],
    "image_patterns": ["megatron", "megatron-lm"],
    "env_keys": ["MEGATRON_CONFIG"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb)
ON CONFLICT (framework_name) DO NOTHING;

-- Framework Detection System - Complete Initialization Script
-- This file contains all tables and indexes for the multi-source framework detection system
-- Version: 1.0.0

-- Description:
-- This migration adds support for multi-source framework detection and metadata reuse
-- for AI workload training jobs. It includes:
--   1. Framework configuration management
--   2. Detection source priority configuration
--   3. Workload similarity caching for reuse optimization
--   4. Conflict detection and logging
--   5. Performance metrics tracking
--   6. Reuse effectiveness evaluation

BEGIN;

-- ============================================================================
-- Section 1: AI Workload Metadata Indexes
-- ============================================================================

-- Add GIN index for JSONB metadata queries
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_metadata_gin 
ON ai_workload_metadata USING GIN (metadata jsonb_path_ops);

-- Add index for time window queries (used in reuse matching)
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_created_at
ON ai_workload_metadata(created_at DESC);

-- Add index for workload_uid queries
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_workload_uid
ON ai_workload_metadata(workload_uid);

-- ============================================================================
-- Section 2: Framework Configuration
-- ============================================================================

CREATE TABLE IF NOT EXISTS framework_config (
    id SERIAL PRIMARY KEY,
    framework_name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128),
    description TEXT,
    priority INTEGER NOT NULL DEFAULT 50,
    base_confidence DECIMAL(3,2) NOT NULL DEFAULT 0.70,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_framework_config_enabled 
ON framework_config(enabled, priority DESC);

CREATE INDEX IF NOT EXISTS idx_framework_config_name
ON framework_config(framework_name);

COMMENT ON TABLE framework_config IS 'Framework configuration for AI workload detection';

-- ============================================================================
-- Section 3: Detection Source Priority
-- ============================================================================

CREATE TABLE IF NOT EXISTS detection_source_priority (
    id SERIAL PRIMARY KEY,
    source_name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128),
    priority INTEGER NOT NULL,
    base_confidence DECIMAL(3,2) NOT NULL,
    confidence_decay DECIMAL(3,2) DEFAULT 1.0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_detection_source_priority_enabled
ON detection_source_priority(enabled, priority DESC);

CREATE INDEX IF NOT EXISTS idx_detection_source_priority_name
ON detection_source_priority(source_name);

COMMENT ON TABLE detection_source_priority IS 'Configuration for detection source priority and confidence levels';

-- ============================================================================
-- Section 4: Workload Similarity Cache
-- ============================================================================

CREATE TABLE IF NOT EXISTS workload_similarity_cache (
    id BIGSERIAL PRIMARY KEY,
    workload_uid_1 VARCHAR(128) NOT NULL,
    workload_uid_2 VARCHAR(128) NOT NULL,
    similarity_score DECIMAL(4,3) NOT NULL,
    signature_hash_1 VARCHAR(64) NOT NULL,
    signature_hash_2 VARCHAR(64) NOT NULL,
    calculated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    CONSTRAINT unique_workload_pair UNIQUE (workload_uid_1, workload_uid_2)
);

CREATE INDEX IF NOT EXISTS idx_similarity_cache_uid1_score
ON workload_similarity_cache(workload_uid_1, similarity_score DESC)
WHERE similarity_score >= 0.85;

CREATE INDEX IF NOT EXISTS idx_similarity_cache_uid2_score
ON workload_similarity_cache(workload_uid_2, similarity_score DESC)
WHERE similarity_score >= 0.85;

CREATE INDEX IF NOT EXISTS idx_similarity_cache_expires
ON workload_similarity_cache(expires_at);

CREATE INDEX IF NOT EXISTS idx_similarity_cache_signatures
ON workload_similarity_cache(signature_hash_1, signature_hash_2);

COMMENT ON TABLE workload_similarity_cache IS 'Cache for workload similarity calculation results';

-- ============================================================================
-- Section 5: Detection Conflict Log
-- ============================================================================

CREATE TABLE IF NOT EXISTS detection_conflict_log (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    source_1 VARCHAR(64) NOT NULL,
    source_2 VARCHAR(64) NOT NULL,
    framework_1 VARCHAR(64) NOT NULL,
    framework_2 VARCHAR(64) NOT NULL,
    confidence_1 DECIMAL(3,2) NOT NULL,
    confidence_2 DECIMAL(3,2) NOT NULL,
    resolution_strategy VARCHAR(64),
    resolved_framework VARCHAR(64),
    resolved_confidence DECIMAL(3,2),
    resolved_at TIMESTAMP,
    evidence_1 JSONB,
    evidence_2 JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_detection_conflict_workload
ON detection_conflict_log(workload_uid, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_detection_conflict_sources
ON detection_conflict_log(source_1, source_2, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_detection_conflict_frameworks
ON detection_conflict_log(framework_1, framework_2);

CREATE INDEX IF NOT EXISTS idx_detection_conflict_created_at
ON detection_conflict_log(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_detection_conflict_resolution
ON detection_conflict_log(resolution_strategy, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_detection_conflict_evidence_gin
ON detection_conflict_log USING GIN (evidence_1 jsonb_path_ops);

COMMENT ON TABLE detection_conflict_log IS 'Log of framework detection conflicts for analysis and debugging';

-- ============================================================================
-- Section 6: Framework Detection Metrics
-- ============================================================================

CREATE TABLE IF NOT EXISTS framework_detection_metrics (
    id BIGSERIAL PRIMARY KEY,
    metric_time TIMESTAMP NOT NULL,
    source VARCHAR(64),
    framework VARCHAR(64),
    status VARCHAR(32),
    total_count INTEGER NOT NULL DEFAULT 0,
    conflict_count INTEGER DEFAULT 0,
    reuse_count INTEGER DEFAULT 0,
    avg_confidence DECIMAL(4,3),
    min_confidence DECIMAL(4,3),
    max_confidence DECIMAL(4,3),
    avg_detection_latency_ms INTEGER,
    p50_detection_latency_ms INTEGER,
    p95_detection_latency_ms INTEGER,
    p99_detection_latency_ms INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_detection_metrics_time
ON framework_detection_metrics(metric_time DESC);

CREATE INDEX IF NOT EXISTS idx_detection_metrics_source_framework
ON framework_detection_metrics(source, framework, metric_time DESC);

CREATE INDEX IF NOT EXISTS idx_detection_metrics_status
ON framework_detection_metrics(status, metric_time DESC);

CREATE INDEX IF NOT EXISTS idx_detection_metrics_framework
ON framework_detection_metrics(framework, metric_time DESC);

CREATE INDEX IF NOT EXISTS idx_detection_metrics_composite
ON framework_detection_metrics(metric_time DESC, framework, source, status);

COMMENT ON TABLE framework_detection_metrics IS 'Aggregated metrics for framework detection monitoring';

-- ============================================================================
-- Section 7: Reuse Effectiveness Log
-- ============================================================================

CREATE TABLE IF NOT EXISTS reuse_effectiveness_log (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    reused_from VARCHAR(128) NOT NULL,
    similarity_score DECIMAL(4,3) NOT NULL,
    reused_framework VARCHAR(64) NOT NULL,
    reused_confidence DECIMAL(3,2) NOT NULL,
    verified_by VARCHAR(64),
    verified_framework VARCHAR(64),
    verified_at TIMESTAMP,
    is_correct BOOLEAN,
    time_saved_ms INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_workload
ON reuse_effectiveness_log(workload_uid);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_from
ON reuse_effectiveness_log(reused_from);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_correct
ON reuse_effectiveness_log(is_correct, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_framework
ON reuse_effectiveness_log(reused_framework, is_correct);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_created_at
ON reuse_effectiveness_log(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_similarity
ON reuse_effectiveness_log(similarity_score DESC, is_correct);

CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_analytics
ON reuse_effectiveness_log(created_at DESC, reused_framework, is_correct);

COMMENT ON TABLE reuse_effectiveness_log IS 'Log of metadata reuse effectiveness for evaluation and tuning';

-- ============================================================================
-- Section 8: Default Configuration Data
-- ============================================================================

-- Insert default framework configurations
INSERT INTO framework_config (framework_name, display_name, priority, base_confidence, config) VALUES
('primus', 'Primus', 80, 0.75, '{
    "log_patterns": [
        {"pattern": "primus-rocm-memory", "weight": 1.0, "category": "memory", "description": "Primus ROCm memory allocation"},
        {"pattern": "PrimusTrainer", "weight": 0.9, "category": "framework", "description": "Primus trainer initialization"},
        {"pattern": "primus\\\\.distributed", "weight": 0.8, "category": "distributed", "description": "Primus distributed training"}
    ],
    "image_patterns": ["primus", "primus-training"],
    "env_keys": ["PRIMUS_CONFIG", "PRIMUS_MASTER_ADDR"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb),
('deepspeed', 'DeepSpeed', 75, 0.75, '{
    "log_patterns": [
        {"pattern": "DeepSpeed Engine", "weight": 1.0, "category": "framework", "description": "DeepSpeed engine initialization"},
        {"pattern": "deepspeed\\\\.runtime", "weight": 0.9, "category": "runtime", "description": "DeepSpeed runtime"}
    ],
    "image_patterns": ["deepspeed"],
    "env_keys": ["DEEPSPEED_CONFIG"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb),
('megatron', 'Megatron-LM', 75, 0.75, '{
    "log_patterns": [
        {"pattern": "Megatron-LM", "weight": 1.0, "category": "framework", "description": "Megatron-LM framework"},
        {"pattern": "megatron\\\\.model", "weight": 0.9, "category": "model", "description": "Megatron model initialization"}
    ],
    "image_patterns": ["megatron", "megatron-lm"],
    "env_keys": ["MEGATRON_CONFIG"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb)
ON CONFLICT (framework_name) DO NOTHING;

-- Insert default detection source priorities
INSERT INTO detection_source_priority (source_name, display_name, priority, base_confidence, confidence_decay, description) VALUES
('user', 'User Annotation', 100, 1.0, 1.0, 'Manual user annotation, highest priority'),
('component', 'Component Detection', 80, 0.85, 1.0, 'Detected by component through image, labels, etc.'),
('reuse', 'Reuse Detection', 75, 0.85, 0.9, 'Reused results from similar workload'),
('log', 'Log Analysis', 60, 0.70, 1.0, 'Identified through log pattern matching'),
('image', 'Image Inference', 40, 0.60, 1.0, 'Inferred from image name only'),
('default', 'Default Inference', 20, 0.30, 1.0, 'Default or unknown case')
ON CONFLICT (source_name) DO NOTHING;

COMMIT;

-- ============================================================================
-- Migration Notes
-- ============================================================================
-- 
-- 1. All updated_at fields are managed by application layer
-- 2. Periodic cleanup tasks are managed by application layer:
--    - workload_similarity_cache: cleanup expired entries
--    - detection_conflict_log: retain 90 days
-- 3. Data integrity constraints are enforced by application layer
-- 4. No foreign keys, triggers, or stored procedures for zero coupling
-- 5. JSONB fields use application-defined schemas
--
-- Performance Tips:
-- - Use GIN indexes for JSONB queries with @> operator
-- - Consider partitioning large tables (conflict_log, metrics) by time
-- - Monitor index usage with pg_stat_user_indexes
-- - Use connection pooling for high concurrency
--
-- For more details, see:
-- - docs/database-schema-design.md
-- - docs/multi-source-framework-detection-design.md

-- Framework Detection Metrics Database Schema
-- This file defines the database schema for storing framework detection metrics
-- used for monitoring and analysis of detection performance

-- Table: framework_detection_metrics
-- Stores aggregated metrics for framework detection
CREATE TABLE IF NOT EXISTS framework_detection_metrics (
    id BIGSERIAL PRIMARY KEY,
    metric_time TIMESTAMP NOT NULL,
    source VARCHAR(64),  -- Detection source (component, log, reuse, etc.)
    framework VARCHAR(64),  -- Framework name (primus, deepspeed, etc.)
    status VARCHAR(32),  -- Detection status (unknown, suspected, confirmed, verified, conflict, reused)
    
    -- Count metrics
    total_count INTEGER NOT NULL DEFAULT 0,
    conflict_count INTEGER DEFAULT 0,
    reuse_count INTEGER DEFAULT 0,
    
    -- Confidence metrics
    avg_confidence DECIMAL(4,3),
    min_confidence DECIMAL(4,3),
    max_confidence DECIMAL(4,3),
    
    -- Latency metrics (milliseconds)
    avg_detection_latency_ms INTEGER,
    p50_detection_latency_ms INTEGER,
    p95_detection_latency_ms INTEGER,
    p99_detection_latency_ms INTEGER,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for time-series queries
CREATE INDEX IF NOT EXISTS idx_detection_metrics_time
ON framework_detection_metrics(metric_time DESC);

-- Index for source and framework filtering
CREATE INDEX IF NOT EXISTS idx_detection_metrics_source_framework
ON framework_detection_metrics(source, framework, metric_time DESC);

-- Index for status filtering
CREATE INDEX IF NOT EXISTS idx_detection_metrics_status
ON framework_detection_metrics(status, metric_time DESC);

-- Index for framework-specific queries
CREATE INDEX IF NOT EXISTS idx_detection_metrics_framework
ON framework_detection_metrics(framework, metric_time DESC);

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_detection_metrics_composite
ON framework_detection_metrics(metric_time DESC, framework, source, status);

-- Comments for documentation
COMMENT ON TABLE framework_detection_metrics IS 'Aggregated metrics for framework detection monitoring';
COMMENT ON COLUMN framework_detection_metrics.metric_time IS 'Time bucket for the metrics (typically hourly)';
COMMENT ON COLUMN framework_detection_metrics.source IS 'Detection source (user, component, reuse, log, image)';
COMMENT ON COLUMN framework_detection_metrics.framework IS 'Framework name (primus, deepspeed, megatron, etc.)';
COMMENT ON COLUMN framework_detection_metrics.status IS 'Detection status (unknown, suspected, confirmed, verified, conflict, reused)';
COMMENT ON COLUMN framework_detection_metrics.total_count IS 'Total number of detections in this time bucket';
COMMENT ON COLUMN framework_detection_metrics.conflict_count IS 'Number of detections with conflicts';
COMMENT ON COLUMN framework_detection_metrics.reuse_count IS 'Number of detections from reuse';
COMMENT ON COLUMN framework_detection_metrics.avg_confidence IS 'Average confidence level (0.000-1.000)';
COMMENT ON COLUMN framework_detection_metrics.min_confidence IS 'Minimum confidence level in this bucket';
COMMENT ON COLUMN framework_detection_metrics.max_confidence IS 'Maximum confidence level in this bucket';
COMMENT ON COLUMN framework_detection_metrics.avg_detection_latency_ms IS 'Average detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.p50_detection_latency_ms IS 'P50 (median) detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.p95_detection_latency_ms IS 'P95 detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.p99_detection_latency_ms IS 'P99 detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.created_at IS 'Timestamp when the metric record was created';

-- Note: Metrics are aggregated and inserted by application layer
-- Recommended aggregation interval: 1 hour
-- Example aggregation logic:
--   SELECT DATE_TRUNC('hour', detected_at) as metric_time,
--          source, framework, status,
--          COUNT(*) as total_count,
--          AVG(confidence) as avg_confidence,
--          MIN(confidence) as min_confidence,
--          MAX(confidence) as max_confidence
--   FROM detection_sources
--   GROUP BY metric_time, source, framework, status;

-- For TimescaleDB users, consider creating a hypertable:
-- SELECT create_hypertable('framework_detection_metrics', 'metric_time', if_not_exists => TRUE);

-- Insert default framework log parser configurations

-- Primus framework configuration
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.primus',
    '{
        "name": "primus",
        "display_name": "Primus",
        "version": "1.0.0",
        "priority": 100,
        "enabled": true,
        "identify_patterns": [
            {
                "name": "primus-identifier",
                "pattern": "primus|Primus|PRIMUS",
                "description": "Identify Primus framework from log content",
                "enabled": true,
                "tags": ["identify"],
                "confidence": 0.7
            }
        ],
        "performance_patterns": [
            {
                "name": "primus-rocm-memory",
                "pattern": "\\\\.*iteration\\\\s+(?P<CurrentIteration>\\\\d+)\\\\s*/\\\\s*(?P<TargetIteration>\\\\d+)\\\\s*\\\\|\\\\s*consumed samples:\\\\s+(?P<ConsumedSamples>\\\\d+)\\\\s*\\\\|\\\\s*elapsed\\\\stime\\\\sper\\\\siteration\\\\s\\\\(ms\\\\):\\\\s+(?P<ElapsedTimePerIterationMS>\\\\d+(?:\\\\.\\\\d+)*)",
                "description": "Primus training performance log",
                "enabled": true,
                "tags": ["performance", "rocm"],
                "confidence": 0.8
            }
        ],
        "training_events": {
            "start_training": [
                {
                    "name": "primus-start-training",
                    "pattern": "training\\\\s*\\\\.\\\\.\\\\.",
                    "description": "Primus training start marker",
                    "enabled": true,
                    "tags": ["lifecycle"],
                    "confidence": 0.9
                }
            ]
        },
        "checkpoint_events": {
            "start_saving": [
                {
                    "name": "primus-checkpoint-start",
                    "pattern": "saving checkpoint at iteration (?P<Iteration>\\\\d+) to (?P<Path>\\\\S+)",
                    "description": "Primus checkpoint save start",
                    "enabled": true,
                    "tags": ["checkpoint"],
                    "confidence": 0.95
                }
            ],
            "end_saving": [
                {
                    "name": "primus-checkpoint-end",
                    "pattern": "successfully saved checkpoint at iteration (?P<Iteration>\\\\d+).*?took (?P<DurationMs>\\\\d+)\\\\s*ms",
                    "description": "Primus checkpoint save completion",
                    "enabled": true,
                    "tags": ["checkpoint"],
                    "confidence": 0.95
                }
            ]
        },
        "extensions": {},
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Log parsing patterns for Primus framework',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- DeepSpeed framework configuration (placeholder)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.deepspeed',
    '{
        "name": "deepspeed",
        "display_name": "DeepSpeed",
        "version": "1.0.0",
        "priority": 90,
        "enabled": true,
        "identify_patterns": [
            {
                "name": "deepspeed-identifier",
                "pattern": "deepspeed|DeepSpeed|DEEPSPEED",
                "description": "Identify DeepSpeed framework from log content",
                "enabled": true,
                "tags": ["identify"],
                "confidence": 0.7
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "checkpoint_events": {
            "start_saving": [],
            "end_saving": []
        },
        "extensions": {},
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Log parsing patterns for DeepSpeed framework',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- Megatron framework configuration (placeholder)
INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.megatron',
    '{
        "name": "megatron",
        "display_name": "Megatron-LM",
        "version": "1.0.0",
        "priority": 80,
        "enabled": true,
        "identify_patterns": [
            {
                "name": "megatron-identifier",
                "pattern": "megatron|Megatron|MEGATRON",
                "description": "Identify Megatron framework from log content",
                "enabled": true,
                "tags": ["identify"],
                "confidence": 0.7
            }
        ],
        "performance_patterns": [],
        "training_events": {
            "start_training": []
        },
        "checkpoint_events": {
            "start_saving": [],
            "end_saving": []
        },
        "extensions": {},
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Log parsing patterns for Megatron framework',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- Primus Framework Log Patterns Initialization
-- This script initializes the default Primus framework log patterns configuration
-- Version: 1.0.0

BEGIN;

-- Insert Primus framework patterns configuration into system_config
INSERT INTO system_config (key, value, description, created_at, updated_at) VALUES
('training.log.parser.framework.primus', '{
  "name": "primus",
  "display_name": "Primus",
  "version": "1.0.0",
  "priority": 80,
  "enabled": true,
  "identify_patterns": [
    {
      "name": "primus-iteration-log",
      "pattern": "iteration\\s+\\d+\\s*/\\s*\\d+.*throughput\\s+per\\s+GPU",
      "description": "Primus iteration performance log identifier",
      "enabled": true,
      "tags": ["performance", "iteration"],
      "confidence": 0.3
    },
    {
      "name": "primus-trainer",
      "pattern": "PRIMUS_PATH",
      "description": "Primus trainer initialization",
      "enabled": true,
      "tags": ["framework"],
      "confidence": 1.0
    }
  ],
  "performance_patterns": [
    {
      "name": "primus-rocm-memory",
      "pattern": ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)\\s*\\|\\s*consumed samples:\\s+(?P<ConsumedSamples>\\d+)\\s*\\|\\s*elapsed\\stime\\sper\\siteration\\s\\(ms\\):\\s+(?P<ElapsedTimePerIterationMS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+rocm\\s+mem\\s+usage/free/total/usage_ratio:\\s+(?P<MemUsage>\\d+\\.\\d+)GB/(?P<MemFree>\\d+\\.\\d+)GB/(?P<MemTotal>\\d+\\.\\d+)GB/(?P<MemUsageRatio>\\d+\\.\\d+)%\\s+\\|\\s+throughput\\s+per\\s+GPU\\s+\\(TFLOP/s/GPU\\):\\s+(?P<TFLOPS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+tokens\\s+per\\s+GPU\\s+\\(tokens/s/GPU\\):\\s+(?P<TokensPerGPU>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s*learning\\s+rate:\\s+(?P<LearningRate>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s*\\|\\s+global\\s+batch\\s+size:\\s+(?P<GlobalBatchSize>\\d+(?:\\.\\d+)*)\\s+\\|\\s+lm\\s+loss:\\s+(?P<LmLoss>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s+\\|\\s+loss\\s+scale:\\s+(?P<LossScale>\\d+(?:\\.\\d+)*)\\s+\\|\\s+grad\\s+norm:\\s+(?P<GradNorm>\\d+(?:\\.\\d+)*)\\s+\\|\\s+num\\s+zeros:\\s(?P<NumZeros>\\d+(?:\\.\\d+)*)\\s+\\|\\s+number\\s+of\\s+skipped\\s+iterations:\\s+(?P<SkippedIterationsNumber>\\d+)\\s+\\|\\s+number\\s+of\\s+nan\\s+iterations:\\s+(?P<NanIterationsNumber>\\d+)\\s*\\|.*",
      "description": "Primus performance log with ROCm memory metrics",
      "enabled": true,
      "tags": ["performance", "rocm", "memory"],
      "confidence": 1.0
    },
    {
      "name": "primus-hip-memory",
      "pattern": ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)\\s*\\|\\s*consumed samples:\\s+(?P<ConsumedSamples>\\d+)\\s*\\|\\s*elapsed\\stime\\sper\\siteration\\s\\(ms\\):\\s+(?P<ElapsedTimePerIterationMS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+hip\\s+mem\\s+usage/free/total/usage_ratio:\\s+(?P<MemUsage>\\d+\\.\\d+)GB/(?P<MemFree>\\d+\\.\\d+)GB/(?P<MemTotal>\\d+\\.\\d+)GB/(?P<MemUsageRatio>\\d+\\.\\d+)%\\s+\\|\\s+throughput\\s+per\\s+GPU\\s+\\(TFLOP/s/GPU\\):\\s+(?P<TFLOPS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+tokens\\s+per\\s+GPU\\s+\\(tokens/s/GPU\\):\\s+(?P<TokensPerGPU>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s*learning\\s+rate:\\s+(?P<LearningRate>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s*\\|\\s+global\\s+batch\\s+size:\\s+(?P<GlobalBatchSize>\\d+(?:\\.\\d+)*)\\s+\\|\\s+lm\\s+loss:\\s+(?P<LmLoss>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s+\\|\\s+loss\\s+scale:\\s+(?P<LossScale>\\d+(?:\\.\\d+)*)\\s+\\|\\s+grad\\s+norm:\\s+(?P<GradNorm>\\d+(?:\\.\\d+)*)\\s+\\|\\s+num\\s+zeros:\\s(?P<NumZeros>\\d+(?:\\.\\d+)*)\\s+\\|\\s+number\\s+of\\s+skipped\\s+iterations:\\s+(?P<SkippedIterationsNumber>\\d+)\\s+\\|\\s+number\\s+of\\s+nan\\s+iterations:\\s+(?P<NanIterationsNumber>\\d+)\\s*\\|.*",
      "description": "Primus performance log with HIP memory metrics",
      "enabled": true,
      "tags": ["performance", "hip", "memory"],
      "confidence": 1.0
    },
    {
      "name": "primus-hip-memory-v2",
      "pattern": ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)\\s*\\|\\s*consumed samples:\\s+(?P<ConsumedSamples>\\d+)\\s*\\|\\s*elapsed\\stime\\sper\\siteration\\s\\(ms\\):\\s+(?P<ElapsedTimePerIterationMS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+hip\\s+mem\\s+usage/free/total/usage_ratio:\\s+(?P<MemUsage>\\d+\\.\\d+)GB/(?P<MemFree>\\d+\\.\\d+)GB/(?P<MemTotal>\\d+\\.\\d+)GB/(?P<MemUsageRatio>\\d+\\.\\d+)%\\s+\\|\\s+throughput\\s+per\\s+GPU\\s+\\(TFLOP/s/GPU\\):\\s+(?P<TFLOPS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+tokens\\s+per\\s+GPU\\s+\\(tokens/s/GPU\\):\\s+(?P<TokensPerGPU>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s*learning\\s+rate:\\s+(?P<LearningRate>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s*\\|\\s+global\\s+batch\\s+size:\\s+(?P<GlobalBatchSize>\\d+(?:\\.\\d+)*)\\s+\\|\\s+lm\\s+loss:\\s+(?P<LmLoss>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s+\\|\\s+loss\\s+scale:\\s+(?P<LossScale>\\d+(?:\\.\\d+)*)\\s+\\|\\s+grad\\s+norm:\\s+(?P<GradNorm>\\d+(?:\\.\\d+)*)\\s+\\|\\s+number\\s+of\\s+skipped\\s+iterations:\\s+(?P<SkippedIterationsNumber>\\d+)\\s+\\|\\s+number\\s+of\\s+nan\\s+iterations:\\s+(?P<NanIterationsNumber>\\d+)\\s*\\|.*",
      "description": "Primus performance log with HIP memory metrics (v2 - without num zeros field)",
      "enabled": true,
      "tags": ["performance", "hip", "memory"],
      "confidence": 1.0
    },
    {
      "name": "primus-legacy",
      "pattern": ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)\\s*\\|\\s*consumed samples:\\s+(?P<ConsumedSamples>\\d+)\\s*\\|\\s*elapsed\\stime\\sper\\siteration\\s\\(ms\\):\\s+(?P<ElapsedTimePerIterationMS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+mem\\s+usages:\\s+(?P<MemUsages>\\d+\\.\\d+)\\s+\\|\\s+throughput\\s+per\\s+GPU\\s+\\(TFLOP/s/GPU\\):\\s+(?P<TFLOPS>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+tokens\\s+per\\s+GPU\\s+\\(tokens/s/GPU\\):\\s+(?P<TokensPerGPU>\\d+(?:\\.\\d+)*)/\\d+(?:\\.\\d+)*\\s+\\|\\s+learning\\s+rate:\\s+(?P<LearningRate>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s+\\|\\s+global\\s+batch\\s+size:\\s+(?P<GlobalBatchSize>\\d+(?:\\.\\d+)*)\\s+\\|\\s+lm\\s+loss:\\s+(?P<LmLoss>[+-]?\\d+(?:\\.\\d+)?(?:[Ee][+-]?\\d+)?)\\s+\\|\\s+loss\\s+scale:\\s+(?P<LossScale>\\d+(?:\\.\\d+)*)\\s+\\|\\s+grad\\s+norm:\\s+(?P<GradNorm>\\d+(?:\\.\\d+)*)\\s+\\|\\s+num\\s+zeros:\\s(?P<NumZeros>\\d+(?:\\.\\d+)*)\\s+\\|\\s+number\\s+of\\s+skipped\\s+iterations:\\s+(?P<SkippedIterationsNumber>\\d+)\\s+\\|\\s+number\\s+of\\s+nan\\s+iterations:\\s+(?P<NanIterationsNumber>\\d+)\\s*\\|.*",
      "description": "Primus legacy format performance log (without detailed memory metrics)",
      "enabled": true,
      "tags": ["performance", "legacy"],
      "confidence": 0.95
    }
  ],
  "training_events": {
    "start_training": [
      {
        "name": "training-start",
        "pattern": "training\\s+\\.\\.\\.",
        "description": "Training started marker",
        "enabled": true,
        "tags": ["lifecycle", "start"],
        "confidence": 1.0
      }
    ],
    "end_training": [],
    "pause_training": [],
    "resume_training": []
  },
  "checkpoint_events": {
    "start_saving": [],
    "end_saving": [],
    "loading": []
  },
  "extensions": {
    "supports_rocm": true,
    "supports_hip": true,
    "memory_tracking": true
  },
  "updated_at": "2024-01-01T00:00:00Z",
  "created_at": "2024-01-01T00:00:00Z"
}'::jsonb, 'Primus framework log patterns configuration', NOW(), NOW())
ON CONFLICT (key) 
DO UPDATE SET 
  value = EXCLUDED.value,
  updated_at = NOW(),
  description = EXCLUDED.description;

COMMIT;
-- Generic Cache Database Schema
-- This file defines the database schema for a generic key-value cache system
-- that can store arbitrary JSON data

-- Table: generic_cache
-- Stores key-value pairs with JSONB values for flexible data caching
CREATE TABLE IF NOT EXISTS generic_cache (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value JSONB NOT NULL,
    
    -- Expiration support
    expires_at TIMESTAMP,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for generic_cache
CREATE UNIQUE INDEX IF NOT EXISTS idx_generic_cache_key ON generic_cache(key);
CREATE INDEX IF NOT EXISTS idx_generic_cache_expires_at ON generic_cache(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_generic_cache_updated_at ON generic_cache(updated_at DESC);

-- GIN index for JSONB value queries (optional, for advanced JSON queries)
CREATE INDEX IF NOT EXISTS idx_generic_cache_value ON generic_cache USING GIN(value);

-- Comments for documentation
COMMENT ON TABLE generic_cache IS 'Generic key-value cache system with JSONB values for flexible data storage';
COMMENT ON COLUMN generic_cache.key IS 'Unique cache key identifier';
COMMENT ON COLUMN generic_cache.value IS 'JSONB value containing cached data';
COMMENT ON COLUMN generic_cache.expires_at IS 'Optional expiration timestamp, NULL means no expiration';
COMMENT ON COLUMN generic_cache.created_at IS 'Timestamp when the cache entry was first created';
COMMENT ON COLUMN generic_cache.updated_at IS 'Timestamp when the cache entry was last updated';

-- GPU Usage Aggregation Table Design
-- For storing hourly aggregated GPU allocation rate and utilization data

-- 1. Cluster-level hourly aggregation table
CREATE TABLE IF NOT EXISTS cluster_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    
    -- Time dimension (aggregated by hour)
    stat_hour TIMESTAMP NOT NULL,  -- Start time of the statistical hour, e.g.: 2025-11-05 14:00:00
    
    -- GPU allocation statistics
    total_gpu_capacity INT NOT NULL DEFAULT 0,           -- Total GPU capacity of the cluster for this hour (average)
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated for this hour (average)
    allocation_rate DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Allocation rate (0-100)
    
    -- GPU utilization statistics (based on actual GPU utilization metrics)
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Average utilization (0-100)
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Peak utilization
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Minimum utilization
    p50_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 50th percentile utilization
    p95_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 95th percentile utilization
    
    -- Sampling statistics
    sample_count INT NOT NULL DEFAULT 0,                      -- Number of samples taken during this hour
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 2. Namespace-level hourly aggregation table
CREATE TABLE IF NOT EXISTS namespace_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    namespace VARCHAR(253) NOT NULL,  -- Maximum length of K8s namespace
    
    -- Time dimension
    stat_hour TIMESTAMP NOT NULL,
    
    -- GPU capacity statistics
    total_gpu_capacity INT NOT NULL DEFAULT 0,                -- Total GPU capacity quota for this namespace during this hour (average)
    
    -- GPU allocation statistics
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated for this namespace (average)
    
    -- GPU utilization statistics
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Workload statistics
    active_workload_count INT NOT NULL DEFAULT 0,             -- Number of active workloads (average)
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 3. Label/Annotation aggregation table (grouped by custom label or annotation)
CREATE TABLE IF NOT EXISTS label_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    
    -- Label/Annotation dimension
    dimension_type VARCHAR(20) NOT NULL,     -- 'label' or 'annotation'
    dimension_key VARCHAR(255) NOT NULL,     -- Key of label/annotation
    dimension_value TEXT NOT NULL,           -- Value of label/annotation
    
    -- Time dimension
    stat_hour TIMESTAMP NOT NULL,
    
    -- GPU allocation statistics
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- GPU utilization statistics
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Workload statistics
    active_workload_count INT NOT NULL DEFAULT 0,
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 4. Snapshot table (for accurate calculation of dynamic resources, supporting multiple dimensions)
CREATE TABLE IF NOT EXISTS gpu_allocation_snapshots (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    snapshot_time TIMESTAMP NOT NULL,
    
    -- Dimension information
    dimension_type VARCHAR(20) NOT NULL,     -- 'cluster', 'namespace', 'label', 'annotation'
    dimension_key VARCHAR(255),              -- Key when dimension_type is label/annotation, otherwise NULL
    dimension_value TEXT,                    -- Namespace name or label/annotation value, NULL when dimension_type is cluster
    
    -- GPU capacity and allocation
    total_gpu_capacity INT NOT NULL DEFAULT 0,      -- Total GPU capacity of this dimension
    allocated_gpu_count INT NOT NULL DEFAULT 0,     -- Number of GPUs allocated in this dimension
    
    -- Detailed allocation information stored in JSON format
    allocation_details JSONB NOT NULL DEFAULT '{}',  -- Detailed workload information, etc.
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index design
-- Cluster level
CREATE UNIQUE INDEX IF NOT EXISTS idx_cluster_gpu_hourly_cluster_hour 
    ON cluster_gpu_hourly_stats(cluster_name, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_cluster_gpu_hourly_hour 
    ON cluster_gpu_hourly_stats(stat_hour DESC);

-- Namespace level
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespace_gpu_hourly_cluster_ns_hour 
    ON namespace_gpu_hourly_stats(cluster_name, namespace, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_namespace_gpu_hourly_cluster_hour 
    ON namespace_gpu_hourly_stats(cluster_name, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_namespace_gpu_hourly_namespace 
    ON namespace_gpu_hourly_stats(namespace, stat_hour DESC);

-- Label/Annotation level
CREATE UNIQUE INDEX IF NOT EXISTS idx_label_gpu_hourly_cluster_type_key_value_hour 
    ON label_gpu_hourly_stats(cluster_name, dimension_type, dimension_key, dimension_value, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_label_gpu_hourly_cluster_hour 
    ON label_gpu_hourly_stats(cluster_name, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_label_gpu_hourly_type_key 
    ON label_gpu_hourly_stats(dimension_type, dimension_key, stat_hour DESC);

-- Snapshot table indexes
CREATE INDEX IF NOT EXISTS idx_gpu_allocation_snapshots_cluster_time 
    ON gpu_allocation_snapshots(cluster_name, snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_gpu_allocation_snapshots_time 
    ON gpu_allocation_snapshots(snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_gpu_allocation_snapshots_dimension 
    ON gpu_allocation_snapshots(cluster_name, dimension_type, dimension_key, dimension_value, snapshot_time DESC);

-- Table comments
COMMENT ON TABLE cluster_gpu_hourly_stats IS 'Cluster-level GPU utilization hourly aggregation statistics table';
COMMENT ON TABLE namespace_gpu_hourly_stats IS 'Namespace-level GPU utilization hourly aggregation statistics table';
COMMENT ON TABLE label_gpu_hourly_stats IS 'Label/Annotation-level GPU utilization hourly aggregation statistics table';
COMMENT ON TABLE gpu_allocation_snapshots IS 'GPU allocation snapshot table, supporting multiple dimensions (cluster/namespace/label/annotation) for accurate tracking of dynamic resource changes';

-- Column comments
COMMENT ON COLUMN cluster_gpu_hourly_stats.stat_hour IS 'Statistical hour (rounded down to the hour), e.g. 2025-11-05 14:00:00';
COMMENT ON COLUMN cluster_gpu_hourly_stats.sample_count IS 'Number of samples collected from Prometheus during this hour';
COMMENT ON COLUMN namespace_gpu_hourly_stats.total_gpu_capacity IS 'Total GPU capacity quota for this namespace during this hour (average)';
COMMENT ON COLUMN label_gpu_hourly_stats.dimension_type IS 'Dimension type: label or annotation';
COMMENT ON COLUMN label_gpu_hourly_stats.dimension_key IS 'Key of label or annotation';
COMMENT ON COLUMN gpu_allocation_snapshots.dimension_type IS 'Dimension type: cluster/namespace/label/annotation';
COMMENT ON COLUMN gpu_allocation_snapshots.allocation_details IS 'Detailed allocation information stored in JSON format, including workload and other detailed data';

-- GPU Usage Weekly Reports Database Schema
-- This file defines the database schema for storing GPU usage weekly reports
-- Reports include HTML, PDF, and JSON formats stored as BLOB/JSONB

-- Table: gpu_usage_weekly_reports
-- Stores generated GPU usage weekly reports with multiple format outputs
CREATE TABLE IF NOT EXISTS gpu_usage_weekly_reports (
    id VARCHAR(64) PRIMARY KEY,
    cluster_name VARCHAR(128) NOT NULL,
    
    -- Report period
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    
    -- Report generation info
    generated_at TIMESTAMP NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    
    -- Report content in different formats
    -- HTML and PDF are stored as binary data (BYTEA in PostgreSQL)
    html_content BYTEA,
    pdf_content BYTEA,
    
    -- JSON content stored as JSONB for efficient querying
    json_content JSONB,
    
    -- Metadata contains summary information like avg_utilization, total_gpus, etc.
    metadata JSONB,
    
    -- Error tracking
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for gpu_usage_weekly_reports
-- Index on cluster_name for filtering by cluster
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_cluster_name 
    ON gpu_usage_weekly_reports(cluster_name);

-- Index on status for filtering pending/completed/failed reports
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_status 
    ON gpu_usage_weekly_reports(status);

-- Composite index on cluster_name and period_start for time-range queries
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_cluster_period 
    ON gpu_usage_weekly_reports(cluster_name, period_start DESC);

-- Index on period_start for date-based queries and cleanup
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_period_start 
    ON gpu_usage_weekly_reports(period_start DESC);

-- Index on generated_at for listing reports by generation time
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_generated_at 
    ON gpu_usage_weekly_reports(generated_at DESC);

-- Index on updated_at for tracking recent changes
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_updated_at 
    ON gpu_usage_weekly_reports(updated_at DESC);

-- GIN index for metadata JSONB queries (for filtering by summary stats)
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_metadata 
    ON gpu_usage_weekly_reports USING GIN(metadata);

-- GIN index for json_content JSONB queries (optional, for advanced queries)
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_json_content 
    ON gpu_usage_weekly_reports USING GIN(json_content);

-- Comments for documentation
COMMENT ON TABLE gpu_usage_weekly_reports IS 'Stores GPU usage weekly reports with HTML, PDF, and JSON formats';
COMMENT ON COLUMN gpu_usage_weekly_reports.id IS 'Unique report identifier (e.g., rpt_20251123_x_flannel_001)';
COMMENT ON COLUMN gpu_usage_weekly_reports.cluster_name IS 'Name of the cluster this report is for';
COMMENT ON COLUMN gpu_usage_weekly_reports.period_start IS 'Start timestamp of the report period';
COMMENT ON COLUMN gpu_usage_weekly_reports.period_end IS 'End timestamp of the report period';
COMMENT ON COLUMN gpu_usage_weekly_reports.generated_at IS 'Timestamp when the report was generated';
COMMENT ON COLUMN gpu_usage_weekly_reports.status IS 'Report status: pending, generated, sent, or failed';
COMMENT ON COLUMN gpu_usage_weekly_reports.html_content IS 'HTML report content stored as binary data';
COMMENT ON COLUMN gpu_usage_weekly_reports.pdf_content IS 'PDF report content stored as binary data';
COMMENT ON COLUMN gpu_usage_weekly_reports.json_content IS 'Structured JSON report data for programmatic access';
COMMENT ON COLUMN gpu_usage_weekly_reports.metadata IS 'Report metadata including summary statistics (avg_utilization, total_gpus, etc.)';
COMMENT ON COLUMN gpu_usage_weekly_reports.error_message IS 'Error message if report generation or sending failed';
COMMENT ON COLUMN gpu_usage_weekly_reports.created_at IS 'Timestamp when the report record was created';
COMMENT ON COLUMN gpu_usage_weekly_reports.updated_at IS 'Timestamp when the report record was last updated';

-- Job Execution History Database Schema
-- This file defines the database schema for tracking job execution history
-- Stores historical records of all job executions for debugging and monitoring

-- Table: job_execution_history
-- Stores execution history for all scheduled jobs
CREATE TABLE IF NOT EXISTS job_execution_history (
    id BIGSERIAL PRIMARY KEY,
    
    -- Job identification
    job_name VARCHAR(255) NOT NULL,
    job_type VARCHAR(100) NOT NULL,
    schedule VARCHAR(50),
    
    -- Execution status
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    
    -- Timing information
    started_at TIMESTAMP NOT NULL,
    ended_at TIMESTAMP,
    duration_seconds DOUBLE PRECISION,
    
    -- Error information
    error_message TEXT,
    error_stack TEXT,
    
    -- Execution context
    cluster_name VARCHAR(100),
    hostname VARCHAR(255),
    
    -- Metadata (for additional context)
    metadata JSONB,
    
    -- Statistics (optional metrics collected during execution)
    execution_stats JSONB,
    
    -- Record creation timestamp
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for job_execution_history
CREATE INDEX IF NOT EXISTS idx_job_execution_history_job_name ON job_execution_history(job_name);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_status ON job_execution_history(status);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_started_at ON job_execution_history(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_ended_at ON job_execution_history(ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_job_name_started_at ON job_execution_history(job_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_status_started_at ON job_execution_history(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_duration ON job_execution_history(duration_seconds) WHERE duration_seconds IS NOT NULL;

-- GIN index for metadata queries
CREATE INDEX IF NOT EXISTS idx_job_execution_history_metadata ON job_execution_history USING GIN(metadata);

-- Comments for documentation
COMMENT ON TABLE job_execution_history IS 'Stores execution history for all scheduled jobs with detailed timing and error information';

COMMENT ON COLUMN job_execution_history.job_name IS 'Name of the job (extracted from job type)';
COMMENT ON COLUMN job_execution_history.job_type IS 'Full type name of the job';
COMMENT ON COLUMN job_execution_history.status IS 'Execution status: running, success, failed, cancelled, timeout';
COMMENT ON COLUMN job_execution_history.started_at IS 'Timestamp when the job execution started';
COMMENT ON COLUMN job_execution_history.ended_at IS 'Timestamp when the job execution ended (NULL if still running)';
COMMENT ON COLUMN job_execution_history.duration_seconds IS 'Total execution duration in seconds';
COMMENT ON COLUMN job_execution_history.error_message IS 'Error message if execution failed';
COMMENT ON COLUMN job_execution_history.error_stack IS 'Full error stack trace for debugging';
COMMENT ON COLUMN job_execution_history.metadata IS 'Additional metadata about the execution context';
COMMENT ON COLUMN job_execution_history.execution_stats IS 'Metrics collected during job execution (e.g., records processed, bytes transferred)';
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
COMMENT ON COLUMN metric_alert_rules.sync_status IS 'Sync status: pending/synced/failed';
COMMENT ON COLUMN metric_alert_rules.groups IS 'Array of VMRule groups containing alert rules';
COMMENT ON COLUMN metric_alert_rules.vmrule_status IS 'Status of the VMRule in Kubernetes';
COMMENT ON COLUMN metric_alert_rules.vmrule_uid IS 'UID of the VMRule resource in Kubernetes';

-- Migration for Metric Alert Rules V2 - Add resource mapping and alert enrichment features
-- Run this migration to add new columns to existing metric_alert_rules table

-- Add resource mapping configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS resource_mapping JSONB;

-- Add alert enrichment configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS alert_enrichment JSONB;

-- Add alert grouping configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS alert_grouping JSONB;

-- Add alert routing configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS alert_routing JSONB;

-- Add comments
COMMENT ON COLUMN metric_alert_rules.resource_mapping IS 'Configuration for mapping Prometheus labels to Primus-Lens resources';
COMMENT ON COLUMN metric_alert_rules.alert_enrichment IS 'Configuration for enriching alerts with resource information';
COMMENT ON COLUMN metric_alert_rules.alert_grouping IS 'Configuration for grouping related alerts';
COMMENT ON COLUMN metric_alert_rules.alert_routing IS 'Configuration for routing alerts to different receivers';

-- Namespace Information Database Schema
-- This file defines the database schema for storing namespace GPU resource information

-- Table: namespace_info
-- Stores namespace GPU configuration and resource allocation information
CREATE TABLE IF NOT EXISTS namespace_info (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    gpu_model VARCHAR(100) NOT NULL,
    gpu_resource INT NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for namespace_info
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespace_info_name ON namespace_info(name);
CREATE INDEX IF NOT EXISTS idx_namespace_info_gpu_model ON namespace_info(gpu_model);
CREATE INDEX IF NOT EXISTS idx_namespace_info_updated_at ON namespace_info(updated_at DESC);

-- Comments for documentation
COMMENT ON TABLE namespace_info IS 'Namespace GPU resource information storage';
COMMENT ON COLUMN namespace_info.name IS 'Namespace name (unique identifier)';
COMMENT ON COLUMN namespace_info.gpu_model IS 'GPU model type allocated to this namespace';
COMMENT ON COLUMN namespace_info.gpu_resource IS 'Number of GPU cards allocated to this namespace';
COMMENT ON COLUMN namespace_info.created_at IS 'Timestamp when the namespace info was first created';
COMMENT ON COLUMN namespace_info.updated_at IS 'Timestamp when the namespace info was last updated';

-- Reuse Effectiveness Log Database Schema
-- This file defines the database schema for tracking metadata reuse effectiveness
-- used for evaluating and tuning the reuse mechanism

-- Table: reuse_effectiveness_log
-- Records effectiveness of metadata reuse for evaluation
CREATE TABLE IF NOT EXISTS reuse_effectiveness_log (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    reused_from VARCHAR(128) NOT NULL,
    similarity_score DECIMAL(4,3) NOT NULL,  -- Similarity score (0.000-1.000)
    reused_framework VARCHAR(64) NOT NULL,
    reused_confidence DECIMAL(3,2) NOT NULL,
    
    -- Verification results
    verified_by VARCHAR(64),  -- Verification source (component, log, user)
    verified_framework VARCHAR(64),
    verified_at TIMESTAMP,
    is_correct BOOLEAN,  -- Whether the reuse was correct
    
    -- Performance metrics
    time_saved_ms INTEGER,  -- Time saved by skipping detection (milliseconds)
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for workload-specific queries
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_workload
ON reuse_effectiveness_log(workload_uid);

-- Index for source tracking (which workload was reused from)
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_from
ON reuse_effectiveness_log(reused_from);

-- Index for correctness analysis
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_correct
ON reuse_effectiveness_log(is_correct, created_at DESC);

-- Index for framework analysis
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_framework
ON reuse_effectiveness_log(reused_framework, is_correct);

-- Index for time-based queries
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_created_at
ON reuse_effectiveness_log(created_at DESC);

-- Index for similarity score analysis
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_similarity
ON reuse_effectiveness_log(similarity_score DESC, is_correct);

-- Composite index for common analytics queries
CREATE INDEX IF NOT EXISTS idx_reuse_effectiveness_analytics
ON reuse_effectiveness_log(created_at DESC, reused_framework, is_correct);

-- Comments for documentation
COMMENT ON TABLE reuse_effectiveness_log IS 'Log of metadata reuse effectiveness for evaluation and tuning';
COMMENT ON COLUMN reuse_effectiveness_log.workload_uid IS 'UID of the workload that used reused metadata';
COMMENT ON COLUMN reuse_effectiveness_log.reused_from IS 'UID of the workload that metadata was reused from';
COMMENT ON COLUMN reuse_effectiveness_log.similarity_score IS 'Calculated similarity score (0.000-1.000)';
COMMENT ON COLUMN reuse_effectiveness_log.reused_framework IS 'Framework from reused metadata';
COMMENT ON COLUMN reuse_effectiveness_log.reused_confidence IS 'Confidence from reused metadata (0.00-1.00)';
COMMENT ON COLUMN reuse_effectiveness_log.verified_by IS 'Source that verified the reuse (component, log, user)';
COMMENT ON COLUMN reuse_effectiveness_log.verified_framework IS 'Framework detected by verification source';
COMMENT ON COLUMN reuse_effectiveness_log.verified_at IS 'Timestamp when the reuse was verified';
COMMENT ON COLUMN reuse_effectiveness_log.is_correct IS 'Whether the reused framework matched verification (true/false)';
COMMENT ON COLUMN reuse_effectiveness_log.time_saved_ms IS 'Time saved by skipping detection in milliseconds';
COMMENT ON COLUMN reuse_effectiveness_log.created_at IS 'Timestamp when the reuse occurred';

-- Note: This table is used for analytics and tuning
-- Key metrics to monitor:
--   - Reuse accuracy rate: COUNT(*) WHERE is_correct = true / COUNT(*)
--   - Average similarity score for correct vs incorrect reuse
--   - Average time saved per reuse
--   - Reuse rate by framework

-- Example analytics query:
-- SELECT reused_framework,
--        COUNT(*) as total_reuse,
--        COUNT(*) FILTER (WHERE is_correct = true) as correct_reuse,
--        ROUND(AVG(similarity_score), 3) as avg_similarity,
--        ROUND(AVG(time_saved_ms), 0) as avg_time_saved_ms
-- FROM reuse_effectiveness_log
-- WHERE created_at > CURRENT_TIMESTAMP - INTERVAL '24 hours'
--   AND is_correct IS NOT NULL
-- GROUP BY reused_framework;

-- System Configuration Database Schema
-- This file defines the database schema for system configuration storage
-- that stores key-value pairs with JSON values

-- Table: system_config
-- Stores system configuration key-value pairs with JSONB values
CREATE TABLE IF NOT EXISTS system_config (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value JSONB NOT NULL,
    
    -- Description and metadata
    description TEXT,
    category VARCHAR(100),
    is_encrypted BOOLEAN DEFAULT FALSE,
    
    -- Version control
    version INTEGER DEFAULT 1,
    
    -- Access control
    is_readonly BOOLEAN DEFAULT FALSE,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255),
    updated_by VARCHAR(255)
);

-- Indexes for system_config
CREATE UNIQUE INDEX IF NOT EXISTS idx_system_config_key ON system_config(key);
CREATE INDEX IF NOT EXISTS idx_system_config_category ON system_config(category);
CREATE INDEX IF NOT EXISTS idx_system_config_updated_at ON system_config(updated_at DESC);

-- GIN index for JSONB value queries (for advanced JSON queries)
CREATE INDEX IF NOT EXISTS idx_system_config_value ON system_config USING GIN(value);

-- Comments for documentation
COMMENT ON TABLE system_config IS 'System configuration storage with key-value pairs and JSONB values';
COMMENT ON COLUMN system_config.key IS 'Unique configuration key identifier';
COMMENT ON COLUMN system_config.value IS 'JSONB value containing configuration data';
COMMENT ON COLUMN system_config.description IS 'Human-readable description of the configuration';
COMMENT ON COLUMN system_config.category IS 'Configuration category for grouping related configs';
COMMENT ON COLUMN system_config.is_encrypted IS 'Flag indicating if the value is encrypted';
COMMENT ON COLUMN system_config.version IS 'Configuration version for tracking changes';
COMMENT ON COLUMN system_config.is_readonly IS 'Flag indicating if the configuration is read-only';
COMMENT ON COLUMN system_config.created_at IS 'Timestamp when the configuration was first created';
COMMENT ON COLUMN system_config.updated_at IS 'Timestamp when the configuration was last updated';
COMMENT ON COLUMN system_config.created_by IS 'User who created the configuration';
COMMENT ON COLUMN system_config.updated_by IS 'User who last updated the configuration';

-- Table: system_config_history
-- Stores historical versions of configuration changes
CREATE TABLE IF NOT EXISTS system_config_history (
    id BIGSERIAL PRIMARY KEY,
    config_id BIGINT NOT NULL,
    key VARCHAR(255) NOT NULL,
    old_value JSONB,
    new_value JSONB NOT NULL,
    version INTEGER NOT NULL,
    change_reason TEXT,
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    changed_by VARCHAR(255),
    
    CONSTRAINT fk_system_config_history_config FOREIGN KEY (config_id) REFERENCES system_config(id) ON DELETE CASCADE
);

-- Indexes for system_config_history
CREATE INDEX IF NOT EXISTS idx_system_config_history_config_id ON system_config_history(config_id);
CREATE INDEX IF NOT EXISTS idx_system_config_history_key ON system_config_history(key);
CREATE INDEX IF NOT EXISTS idx_system_config_history_changed_at ON system_config_history(changed_at DESC);

-- Comments for documentation
COMMENT ON TABLE system_config_history IS 'Historical versions of configuration changes for audit trail';
COMMENT ON COLUMN system_config_history.config_id IS 'Reference to the system_config table';
COMMENT ON COLUMN system_config_history.old_value IS 'Previous configuration value before the change';
COMMENT ON COLUMN system_config_history.new_value IS 'New configuration value after the change';
COMMENT ON COLUMN system_config_history.version IS 'Configuration version number';
COMMENT ON COLUMN system_config_history.change_reason IS 'Reason for the configuration change';
COMMENT ON COLUMN system_config_history.changed_at IS 'Timestamp when the configuration was changed';
COMMENT ON COLUMN system_config_history.changed_by IS 'User who made the configuration change';

-- Workload-level GPU Usage Statistics Table
-- For storing hourly aggregated GPU usage data at the workload level

-- Workload-level hourly aggregation table
CREATE TABLE IF NOT EXISTS workload_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    namespace VARCHAR(253) NOT NULL,  -- K8s namespace
    workload_name VARCHAR(253) NOT NULL,  -- Workload name
    workload_type VARCHAR(50) NOT NULL,  -- Workload type: Job/Deployment/StatefulSet/etc.
    
    -- Time dimension (aggregated by hour)
    stat_hour TIMESTAMP NOT NULL,  -- Start time of the statistical hour, e.g.: 2025-11-05 14:00:00
    
    -- GPU allocation statistics
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated to this workload (average)
    requested_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs requested by this workload (average)
    
    -- GPU utilization statistics (based on actual GPU metrics)
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Average utilization (0-100)
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Peak utilization
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Minimum utilization
    p50_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 50th percentile utilization
    p95_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 95th percentile utilization
    
    -- GPU memory statistics (optional, in GB)
    avg_gpu_memory_used DOUBLE PRECISION NOT NULL DEFAULT 0,   -- Average GPU memory used
    max_gpu_memory_used DOUBLE PRECISION NOT NULL DEFAULT 0,   -- Peak GPU memory used
    avg_gpu_memory_total DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Average GPU memory total
    
    -- Workload lifecycle statistics
    avg_replica_count DOUBLE PRECISION NOT NULL DEFAULT 0,     -- Average number of replicas during this hour
    max_replica_count INT NOT NULL DEFAULT 0,                  -- Maximum number of replicas
    min_replica_count INT NOT NULL DEFAULT 0,                  -- Minimum number of replicas
    
    -- Workload status
    workload_status VARCHAR(50),  -- Running/Completed/Failed/etc.
    
    -- Sampling statistics
    sample_count INT NOT NULL DEFAULT 0,  -- Number of samples taken during this hour
    
    -- Owner information (optional)
    owner_uid VARCHAR(255),  -- Owner UID from workload metadata
    owner_name VARCHAR(255), -- Owner name (user/team)
    
    -- Labels and annotations (stored as JSONB for flexible querying)
    labels JSONB NOT NULL DEFAULT '{}',      -- Workload labels
    annotations JSONB NOT NULL DEFAULT '{}', -- Workload annotations
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index design
-- Primary query index: by cluster, namespace, workload, and time
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_gpu_hourly_cluster_ns_wl_hour 
    ON workload_gpu_hourly_stats(cluster_name, namespace, workload_name, stat_hour DESC);

-- Query by cluster and time
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_cluster_hour 
    ON workload_gpu_hourly_stats(cluster_name, stat_hour DESC);

-- Query by namespace and time
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_namespace_hour 
    ON workload_gpu_hourly_stats(namespace, stat_hour DESC);

-- Query by workload name and time (across clusters/namespaces)
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_workload_hour 
    ON workload_gpu_hourly_stats(workload_name, stat_hour DESC);

-- Query by workload type and time
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_type_hour 
    ON workload_gpu_hourly_stats(workload_type, stat_hour DESC);

-- Query by owner
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_owner 
    ON workload_gpu_hourly_stats(owner_uid, stat_hour DESC) WHERE owner_uid IS NOT NULL;

-- Query by time only (for cleanup or full scans)
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_hour 
    ON workload_gpu_hourly_stats(stat_hour DESC);

-- JSONB index for label queries
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_labels 
    ON workload_gpu_hourly_stats USING GIN(labels);

-- JSONB index for annotation queries
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_annotations 
    ON workload_gpu_hourly_stats USING GIN(annotations);

-- Table comments
COMMENT ON TABLE workload_gpu_hourly_stats IS 'Workload-level GPU utilization hourly aggregation statistics table, recording historical GPU usage for each workload';

-- Column comments
COMMENT ON COLUMN workload_gpu_hourly_stats.stat_hour IS 'Statistical hour (rounded down to the hour), e.g. 2025-11-05 14:00:00';
COMMENT ON COLUMN workload_gpu_hourly_stats.workload_type IS 'Type of Kubernetes workload: Job, Deployment, StatefulSet, DaemonSet, etc.';
COMMENT ON COLUMN workload_gpu_hourly_stats.allocated_gpu_count IS 'Average number of GPUs actually allocated to this workload during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.requested_gpu_count IS 'Average number of GPUs requested by this workload during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.sample_count IS 'Number of samples collected from Prometheus during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.avg_replica_count IS 'Average number of pod replicas running during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.owner_uid IS 'UID of the workload owner from metadata';
COMMENT ON COLUMN workload_gpu_hourly_stats.labels IS 'Workload labels stored in JSONB format for flexible querying';
COMMENT ON COLUMN workload_gpu_hourly_stats.annotations IS 'Workload annotations stored in JSONB format for flexible querying';
COMMENT ON COLUMN workload_gpu_hourly_stats.avg_gpu_memory_used IS 'Average GPU memory used in GB';
COMMENT ON COLUMN workload_gpu_hourly_stats.workload_status IS 'Status of the workload during this hour: Running, Completed, Failed, Pending, etc.';
-- Workload Similarity Cache Database Schema
-- This file defines the database schema for caching workload similarity calculation results
-- to improve performance of metadata reuse matching

-- Table: workload_similarity_cache
-- Caches similarity scores between workload pairs
CREATE TABLE IF NOT EXISTS workload_similarity_cache (
    id BIGSERIAL PRIMARY KEY,
    workload_uid_1 VARCHAR(128) NOT NULL,
    workload_uid_2 VARCHAR(128) NOT NULL,
    similarity_score DECIMAL(4,3) NOT NULL,  -- Similarity score (0.000-1.000)
    signature_hash_1 VARCHAR(64) NOT NULL,  -- Hash of workload 1 signature
    signature_hash_2 VARCHAR(64) NOT NULL,  -- Hash of workload 2 signature
    calculated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,  -- Cache expiration time
    
    -- Ensure unique pair (prevent duplicate calculations)
    CONSTRAINT unique_workload_pair UNIQUE (workload_uid_1, workload_uid_2)
);

-- Index for finding similar workloads (forward direction)
CREATE INDEX IF NOT EXISTS idx_similarity_cache_uid1_score
ON workload_similarity_cache(workload_uid_1, similarity_score DESC)
WHERE similarity_score >= 0.85;

-- Index for finding similar workloads (reverse direction)
CREATE INDEX IF NOT EXISTS idx_similarity_cache_uid2_score
ON workload_similarity_cache(workload_uid_2, similarity_score DESC)
WHERE similarity_score >= 0.85;

-- Index for cache cleanup (by expiration time)
CREATE INDEX IF NOT EXISTS idx_similarity_cache_expires
ON workload_similarity_cache(expires_at);

-- Index for signature-based lookups
CREATE INDEX IF NOT EXISTS idx_similarity_cache_signatures
ON workload_similarity_cache(signature_hash_1, signature_hash_2);

-- Comments for documentation
COMMENT ON TABLE workload_similarity_cache IS 'Cache for workload similarity calculation results';
COMMENT ON COLUMN workload_similarity_cache.workload_uid_1 IS 'First workload UID in the comparison pair';
COMMENT ON COLUMN workload_similarity_cache.workload_uid_2 IS 'Second workload UID in the comparison pair';
COMMENT ON COLUMN workload_similarity_cache.similarity_score IS 'Calculated similarity score (0.000-1.000)';
COMMENT ON COLUMN workload_similarity_cache.signature_hash_1 IS 'Hash of first workload signature for cache invalidation';
COMMENT ON COLUMN workload_similarity_cache.signature_hash_2 IS 'Hash of second workload signature for cache invalidation';
COMMENT ON COLUMN workload_similarity_cache.calculated_at IS 'Timestamp when the similarity was calculated';
COMMENT ON COLUMN workload_similarity_cache.expires_at IS 'Cache expiration time (managed by application cleanup task)';

-- Note: Cache cleanup is managed by application layer
-- Cleanup logic: DELETE FROM workload_similarity_cache WHERE expires_at < CURRENT_TIMESTAMP;

-- Add soft delete support to namespace_info table
-- This migration adds deleted_at column for soft delete functionality

-- Add deleted_at column (nullable for soft delete)
ALTER TABLE namespace_info 
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

-- Create index for soft delete queries (filter out deleted records efficiently)
CREATE INDEX IF NOT EXISTS idx_namespace_info_deleted_at ON namespace_info(deleted_at);

-- Composite index for common queries: find active namespaces by name
CREATE INDEX IF NOT EXISTS idx_namespace_info_name_deleted_at ON namespace_info(name, deleted_at);

-- Comment for documentation
COMMENT ON COLUMN namespace_info.deleted_at IS 'Timestamp when the namespace info was soft deleted (NULL means not deleted)';

-- Add labels and annotations columns to node table
-- These columns store Kubernetes node labels and annotations as JSONB

-- Add labels column if it doesn't exist
ALTER TABLE node 
ADD COLUMN IF NOT EXISTS labels JSONB DEFAULT '{}';

-- Add annotations column if it doesn't exist
ALTER TABLE node 
ADD COLUMN IF NOT EXISTS annotations JSONB DEFAULT '{}';

-- Create GIN index for labels to support efficient JSONB queries
CREATE INDEX IF NOT EXISTS idx_node_labels ON node USING GIN (labels);

-- Create GIN index for annotations to support efficient JSONB queries
CREATE INDEX IF NOT EXISTS idx_node_annotations ON node USING GIN (annotations);

-- Add comments for documentation
COMMENT ON COLUMN node.labels IS 'Kubernetes node labels stored as JSONB key-value pairs';
COMMENT ON COLUMN node.annotations IS 'Kubernetes node annotations stored as JSONB key-value pairs';

-- Node and Namespace mapping table
-- This table stores the relationship between nodes and namespaces

-- Create node_namespace_mapping table
CREATE TABLE IF NOT EXISTS node_namespace_mapping (
    id SERIAL PRIMARY KEY,
    node_id INTEGER NOT NULL,
    node_name VARCHAR(255) NOT NULL,
    namespace_id BIGINT NOT NULL,
    namespace_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_node_namespace_mapping_node FOREIGN KEY (node_id) REFERENCES node(id) ON DELETE CASCADE,
    CONSTRAINT fk_node_namespace_mapping_namespace FOREIGN KEY (namespace_id) REFERENCES namespace_info(id) ON DELETE CASCADE
);

-- Add deleted_at column for soft delete support
ALTER TABLE node_namespace_mapping 
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

-- Create index for querying by node
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_node_id 
ON node_namespace_mapping(node_id);

-- Create index for querying by namespace
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_namespace_id 
ON node_namespace_mapping(namespace_id);

-- Create index for querying by node_name (useful when node_id is not available)
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_node_name 
ON node_namespace_mapping(node_name);

-- Create index for querying by namespace_name (useful when namespace_id is not available)
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_namespace_name 
ON node_namespace_mapping(namespace_name);

-- Create index for soft delete queries
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_deleted_at 
ON node_namespace_mapping(deleted_at);

-- Add comments for documentation
COMMENT ON TABLE node_namespace_mapping IS 'Mapping table that stores relationships between nodes and namespaces';
COMMENT ON COLUMN node_namespace_mapping.id IS 'Primary key';
COMMENT ON COLUMN node_namespace_mapping.node_id IS 'Foreign key reference to node table';
COMMENT ON COLUMN node_namespace_mapping.node_name IS 'Node name for quick reference without join';
COMMENT ON COLUMN node_namespace_mapping.namespace_id IS 'Foreign key reference to namespace_info table';
COMMENT ON COLUMN node_namespace_mapping.namespace_name IS 'Namespace name for quick reference without join';
COMMENT ON COLUMN node_namespace_mapping.created_at IS 'Timestamp when the mapping was created';
COMMENT ON COLUMN node_namespace_mapping.updated_at IS 'Timestamp when the mapping was last updated';
COMMENT ON COLUMN node_namespace_mapping.deleted_at IS 'Timestamp when the mapping was soft deleted (NULL means active)';

-- ============================================================================
-- Node Namespace Mapping History Table
-- Records historical changes of node-namespace relationships for time-based queries
-- Uses record_start and record_end for time-range based queries
-- ============================================================================

CREATE TABLE IF NOT EXISTS node_namespace_mapping_history (
    id SERIAL PRIMARY KEY,
    mapping_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    node_name VARCHAR(255) NOT NULL,
    namespace_id BIGINT NOT NULL,
    namespace_name VARCHAR(255) NOT NULL,
    action VARCHAR(20) NOT NULL,  -- 'added' or 'removed'
    record_start TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    record_end TIMESTAMP DEFAULT NULL
);

-- ============================================================================
-- Migration: Change snapshot_time/recorded_at to record_start/record_end
-- Run these ALTER statements if table already exists with old schema
-- ============================================================================

-- Drop old columns if they exist
ALTER TABLE node_namespace_mapping_history 
DROP COLUMN IF EXISTS snapshot_time;

ALTER TABLE node_namespace_mapping_history 
DROP COLUMN IF EXISTS recorded_at;

-- Add new columns if they don't exist
ALTER TABLE node_namespace_mapping_history 
ADD COLUMN IF NOT EXISTS record_start TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE node_namespace_mapping_history 
ADD COLUMN IF NOT EXISTS record_end TIMESTAMP DEFAULT NULL;

-- Drop old indexes if they exist
DROP INDEX IF EXISTS idx_node_namespace_history_snapshot_time;
DROP INDEX IF EXISTS idx_node_namespace_history_ns_time;
DROP INDEX IF EXISTS idx_node_namespace_history_ns_name_time;

-- Create index for record_start (find records that started before a time)
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_record_start 
ON node_namespace_mapping_history(record_start);

-- Create index for record_end (find active records where record_end is NULL)
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_record_end 
ON node_namespace_mapping_history(record_end);

-- Create index for querying history by namespace
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_namespace_id 
ON node_namespace_mapping_history(namespace_id);

-- Create index for querying history by namespace_name
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_namespace_name 
ON node_namespace_mapping_history(namespace_name);

-- Create index for querying history by node
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_node_id 
ON node_namespace_mapping_history(node_id);

-- Create composite index for efficient time-range queries per namespace
-- Query: Find nodes in namespace at time T: WHERE namespace_id = X AND record_start <= T AND (record_end IS NULL OR record_end > T)
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_ns_time_range 
ON node_namespace_mapping_history(namespace_id, record_start, record_end);

-- Create composite index for efficient time-range queries per namespace_name
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_ns_name_time_range 
ON node_namespace_mapping_history(namespace_name, record_start, record_end);

-- Add comments for documentation
COMMENT ON TABLE node_namespace_mapping_history IS 'History table recording all changes to node-namespace relationships. Use record_start and record_end for time-range queries';
COMMENT ON COLUMN node_namespace_mapping_history.id IS 'Primary key';
COMMENT ON COLUMN node_namespace_mapping_history.mapping_id IS 'Reference to the original mapping record';
COMMENT ON COLUMN node_namespace_mapping_history.node_id IS 'Node ID at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.node_name IS 'Node name at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.namespace_id IS 'Namespace ID at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.namespace_name IS 'Namespace name at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.action IS 'Type of change: added (node joined namespace) or removed (node left namespace)';
COMMENT ON COLUMN node_namespace_mapping_history.record_start IS 'Start time when this node-namespace relationship became effective';
COMMENT ON COLUMN node_namespace_mapping_history.record_end IS 'End time when this relationship ended (NULL means currently active)';
-- profiler_files.sql
-- PyTorch Profiler files metadata and binary storage tables
-- Created: 2024-12-15

CREATE TABLE IF NOT EXISTS profiler_files (
    id SERIAL PRIMARY KEY,
    workload_uid VARCHAR(255) NOT NULL,
    pod_uid VARCHAR(255),
    pod_name VARCHAR(255),
    pod_namespace VARCHAR(255),
    file_name VARCHAR(512) NOT NULL,
    file_path TEXT,
    file_type VARCHAR(50) NOT NULL,
    file_size BIGINT,
    storage_type VARCHAR(20) NOT NULL,
    storage_path TEXT,
    storage_bucket VARCHAR(255),
    download_url TEXT,
    confidence VARCHAR(20),
    source_pid INT,
    detected_at TIMESTAMP,
    collected_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_profiler_files_workload_uid ON profiler_files(workload_uid);
CREATE INDEX IF NOT EXISTS idx_profiler_files_pod_uid ON profiler_files(pod_uid);
CREATE INDEX IF NOT EXISTS idx_profiler_files_file_type ON profiler_files(file_type);
CREATE INDEX IF NOT EXISTS idx_profiler_files_storage_type ON profiler_files(storage_type);
CREATE INDEX IF NOT EXISTS idx_profiler_files_collected_at ON profiler_files(collected_at);
CREATE INDEX IF NOT EXISTS idx_profiler_files_created_at ON profiler_files(created_at);

CREATE TABLE IF NOT EXISTS profiler_file_content (
    id SERIAL PRIMARY KEY,
    profiler_file_id INT NOT NULL,
    content BYTEA NOT NULL,
    content_encoding VARCHAR(20),
    chunk_index INT NOT NULL DEFAULT 0,
    total_chunks INT NOT NULL DEFAULT 1,
    chunk_size BIGINT,
    md5_hash VARCHAR(32),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_profiler_file_content_file_id ON profiler_file_content(profiler_file_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiler_file_content_file_chunk ON profiler_file_content(profiler_file_id, chunk_index);

CREATE TABLE IF NOT EXISTS profiler_analysis (
    id SERIAL PRIMARY KEY,
    profiler_file_id INT,
    workload_uid VARCHAR(255) NOT NULL,
    total_duration_ms FLOAT,
    gpu_utilization_avg FLOAT,
    cpu_utilization_avg FLOAT,
    memory_peak_mb FLOAT,
    memory_allocated_mb FLOAT,
    top_operations JSONB,
    bottlenecks JSONB,
    total_operators INT,
    unique_operators INT,
    analyzed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_profiler_analysis_file_id ON profiler_analysis(profiler_file_id);
CREATE INDEX IF NOT EXISTS idx_profiler_analysis_workload_uid ON profiler_analysis(workload_uid);

-- ============================================================
-- TraceLens Sessions Table
-- Manages on-demand TraceLens analysis pods for profiler files
-- ============================================================

-- Create tracelens_sessions table
-- Note: Business logic constraints (status values, resource profiles) are enforced in application code
CREATE TABLE IF NOT EXISTS tracelens_sessions (
    -- Primary key
    id SERIAL PRIMARY KEY,
    
    -- Session identification (unique identifier for this analysis session)
    session_id VARCHAR(64) UNIQUE NOT NULL,
    
    -- Association with profiler data
    workload_uid VARCHAR(64) NOT NULL,
    profiler_file_id INTEGER,  -- References profiler_files(id), enforced in application
    
    -- User tracking
    user_id VARCHAR(64),
    user_email VARCHAR(256),
    
    -- Kubernetes resources
    pod_name VARCHAR(128),
    pod_namespace VARCHAR(64) DEFAULT 'primus-lens',
    pod_ip VARCHAR(64),
    pod_port INTEGER DEFAULT 8501,
    
    -- Session status (validated in application code)
    -- Valid values: pending, creating, initializing, ready, failed, expired, deleted
    status VARCHAR(32) DEFAULT 'pending' NOT NULL,
    status_message TEXT,
    
    -- Lifecycle management
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    ready_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    last_accessed_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Resource profile (validated in application code)
    -- Valid values: small, medium, large
    resource_profile VARCHAR(16) DEFAULT 'medium',
    
    -- Configuration and metadata (JSON)
    config JSONB DEFAULT '{}'
);

-- ============================================================
-- Indexes for efficient queries
-- ============================================================

-- Index for status-based queries (find all ready/expired sessions)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_status 
    ON tracelens_sessions(status);

-- Index for workload-based queries (find sessions for a workload)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_workload 
    ON tracelens_sessions(workload_uid);

-- Index for finding sessions by profiler file
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_profiler_file 
    ON tracelens_sessions(profiler_file_id);

-- Index for user-based queries (find user's sessions)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_user 
    ON tracelens_sessions(user_id);

-- Index for cleanup queries (find expired sessions that need cleanup)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_expires 
    ON tracelens_sessions(expires_at) 
    WHERE status NOT IN ('deleted', 'expired');

-- Index for finding active sessions (for resource management)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_active 
    ON tracelens_sessions(status, created_at) 
    WHERE status IN ('pending', 'creating', 'initializing', 'ready');

-- Composite index for session lookup by workload and status
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_workload_status 
    ON tracelens_sessions(workload_uid, status);

-- ============================================================
-- Comments for documentation
-- ============================================================

COMMENT ON TABLE tracelens_sessions IS 'Manages on-demand TraceLens analysis pods for profiler files';

COMMENT ON COLUMN tracelens_sessions.session_id IS 'Unique session identifier, format: tls-{workload_uid_prefix}-{file_id}-{timestamp}';
COMMENT ON COLUMN tracelens_sessions.workload_uid IS 'Associated GPU workload UID';
COMMENT ON COLUMN tracelens_sessions.profiler_file_id IS 'Associated profiler file ID (logical reference to profiler_files.id)';
COMMENT ON COLUMN tracelens_sessions.pod_name IS 'Kubernetes pod name, format: tracelens-session-{session_id}';
COMMENT ON COLUMN tracelens_sessions.pod_ip IS 'Pod cluster IP for proxy routing';
COMMENT ON COLUMN tracelens_sessions.status IS 'Session lifecycle status: pending -> creating -> initializing -> ready -> expired -> deleted';
COMMENT ON COLUMN tracelens_sessions.expires_at IS 'Automatic expiration time, default TTL is 1 hour';
COMMENT ON COLUMN tracelens_sessions.last_accessed_at IS 'Last proxy request time, used for idle detection';
COMMENT ON COLUMN tracelens_sessions.resource_profile IS 'Resource allocation profile: small (2GB), medium (4GB), large (8GB)';
COMMENT ON COLUMN tracelens_sessions.config IS 'Additional configuration as JSON (e.g., analysis options)';

-- Migration: workload_task_state
-- Description: Simplified task state management with distributed lock support
-- Purpose: Support service restart recovery with multi-instance coordination
-- Database: PostgreSQL
-- Note: All logic (functions, triggers, views) is implemented in application layer

-- ============================================================================
-- Table: workload_task_state
-- ============================================================================

CREATE TABLE IF NOT EXISTS workload_task_state (
    id BIGSERIAL PRIMARY KEY,
    
    -- Core fields
    workload_uid VARCHAR(128) NOT NULL,
    task_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Distributed lock fields for multi-instance coordination
    lock_owner VARCHAR(128),
    lock_acquired_at TIMESTAMPTZ,
    lock_expires_at TIMESTAMPTZ,
    lock_version BIGINT NOT NULL DEFAULT 0,
    
    -- Extension field for all other data
    ext JSONB DEFAULT '{}'::jsonb,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT uq_workload_task UNIQUE (workload_uid, task_type)
);

-- ============================================================================
-- Indexes
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_workload_uid ON workload_task_state(workload_uid);
CREATE INDEX IF NOT EXISTS idx_task_type ON workload_task_state(task_type);
CREATE INDEX IF NOT EXISTS idx_status ON workload_task_state(status);
CREATE INDEX IF NOT EXISTS idx_lock_owner ON workload_task_state(lock_owner);
CREATE INDEX IF NOT EXISTS idx_lock_expires_at ON workload_task_state(lock_expires_at);
CREATE INDEX IF NOT EXISTS idx_created_at ON workload_task_state(created_at);
CREATE INDEX IF NOT EXISTS idx_updated_at ON workload_task_state(updated_at);
CREATE INDEX IF NOT EXISTS idx_ext_gin ON workload_task_state USING GIN(ext);

-- ============================================================================
-- Comments
-- ============================================================================

COMMENT ON TABLE workload_task_state IS 'Task state with distributed lock support - all logic in application layer';
COMMENT ON COLUMN workload_task_state.workload_uid IS 'Workload unique identifier';
COMMENT ON COLUMN workload_task_state.task_type IS 'Task type: detection, metadata_collection, tensorboard_stream, etc';
COMMENT ON COLUMN workload_task_state.status IS 'Task status: pending, running, completed, failed, cancelled';
COMMENT ON COLUMN workload_task_state.lock_owner IS 'Instance ID that currently owns this task';
COMMENT ON COLUMN workload_task_state.lock_acquired_at IS 'Timestamp when lock was acquired';
COMMENT ON COLUMN workload_task_state.lock_expires_at IS 'Timestamp when lock expires (for stale lock detection)';
COMMENT ON COLUMN workload_task_state.lock_version IS 'Version number for optimistic locking';
COMMENT ON COLUMN workload_task_state.ext IS 'Extension field (JSONB) for all other task data';

-- ============================================================================
-- Application layer responsibilities
-- ============================================================================
-- 1. Distributed lock operations (acquire, extend, release)
-- 2. Task state management (create, update, delete)
-- 3. Checkpoint management (store/restore from ext field)
-- 4. Stale lock cleanup (release expired locks)
-- 5. Task recovery (find recoverable tasks on startup)
-- 6. Statistics and monitoring (query tasks by status, etc)
--
-- See: Lens/modules/core/pkg/database/workload_task_facade.go
-- Docs: Lens/docs/workload-task-persistence-simplified.md

-- Workload Statistics Table
-- For storing workload-level GPU utilization statistics including instant, average, and percentile metrics

-- Workload statistics table
CREATE TABLE IF NOT EXISTS workload_statistic (
    id SERIAL PRIMARY KEY,
    
    -- User/Owner identification
    uid VARCHAR(255) NOT NULL,  -- User/Owner UID
    
    -- Workload identification
    cluster_name VARCHAR(100) NOT NULL,
    namespace VARCHAR(253) NOT NULL,  -- K8s namespace
    workload_name VARCHAR(253) NOT NULL,  -- Workload name
    workload_type VARCHAR(50) NOT NULL,  -- Workload type: Job/Deployment/StatefulSet/etc.
    
    -- Instant GPU utilization (real-time snapshot)
    instant_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Current/instant GPU utilization (0-100)
    
    -- Historical average GPU utilization
    avg_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Historical average GPU utilization (0-100)
    
    -- Percentile GPU utilization metrics
    p50_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- 50th percentile (median) GPU utilization
    p90_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- 90th percentile GPU utilization
    p95_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- 95th percentile GPU utilization
    
    -- Additional useful metrics
    max_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Maximum GPU utilization
    min_gpu_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Minimum GPU utilization
    
    -- Time window for statistics
    stat_start_time TIMESTAMP NOT NULL,  -- Start time of the statistical period
    stat_end_time TIMESTAMP NOT NULL,    -- End time of the statistical period
    
    -- Sample count
    sample_count INT NOT NULL DEFAULT 0,  -- Number of samples collected during this period
    
    -- GPU allocation information
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated to this workload
    
    -- Workload status
    workload_status VARCHAR(50),  -- Running/Completed/Failed/etc.
    
    -- Labels and annotations (stored as JSONB for flexible querying)
    labels JSONB NOT NULL DEFAULT '{}',      -- Workload labels
    annotations JSONB NOT NULL DEFAULT '{}', -- Workload annotations
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index design
-- Single uid index: for fast lookups by uid without ORDER BY id overhead
-- This prevents full table scans when querying by uid with LIMIT 1
CREATE INDEX IF NOT EXISTS idx_workload_statistic_uid 
    ON workload_statistic(uid);

-- Primary query index: by uid and time
CREATE INDEX IF NOT EXISTS idx_workload_statistic_uid_time 
    ON workload_statistic(uid, stat_end_time DESC);

-- Query by cluster, namespace, workload, and time
CREATE INDEX IF NOT EXISTS idx_workload_statistic_cluster_ns_wl_time 
    ON workload_statistic(cluster_name, namespace, workload_name, stat_end_time DESC);

-- Query by uid, cluster, and time
CREATE INDEX IF NOT EXISTS idx_workload_statistic_uid_cluster_time 
    ON workload_statistic(uid, cluster_name, stat_end_time DESC);

-- Query by workload name and time (across clusters/namespaces)
CREATE INDEX IF NOT EXISTS idx_workload_statistic_workload_time 
    ON workload_statistic(workload_name, stat_end_time DESC);

-- Query by workload type and time
CREATE INDEX IF NOT EXISTS idx_workload_statistic_type_time 
    ON workload_statistic(workload_type, stat_end_time DESC);

-- Query by time only (for cleanup or full scans)
CREATE INDEX IF NOT EXISTS idx_workload_statistic_time 
    ON workload_statistic(stat_end_time DESC);

-- JSONB index for label queries
CREATE INDEX IF NOT EXISTS idx_workload_statistic_labels 
    ON workload_statistic USING GIN(labels);

-- JSONB index for annotation queries
CREATE INDEX IF NOT EXISTS idx_workload_statistic_annotations 
    ON workload_statistic USING GIN(annotations);

-- Composite index for uid and cluster
CREATE INDEX IF NOT EXISTS idx_workload_statistic_uid_cluster 
    ON workload_statistic(uid, cluster_name);

-- Table comments
COMMENT ON TABLE workload_statistic IS 'Workload-level GPU utilization statistics table, storing instant, average, and percentile metrics for each workload by user';

-- Column comments
COMMENT ON COLUMN workload_statistic.uid IS 'User/Owner UID from workload metadata';
COMMENT ON COLUMN workload_statistic.instant_gpu_utilization IS 'Current/instant GPU utilization snapshot (0-100)';
COMMENT ON COLUMN workload_statistic.avg_gpu_utilization IS 'Historical average GPU utilization (0-100)';
COMMENT ON COLUMN workload_statistic.p50_gpu_utilization IS '50th percentile (median) GPU utilization (0-100)';
COMMENT ON COLUMN workload_statistic.p90_gpu_utilization IS '90th percentile GPU utilization (0-100)';
COMMENT ON COLUMN workload_statistic.p95_gpu_utilization IS '95th percentile GPU utilization (0-100)';
COMMENT ON COLUMN workload_statistic.stat_start_time IS 'Start time of the statistical period';
COMMENT ON COLUMN workload_statistic.stat_end_time IS 'End time of the statistical period';
COMMENT ON COLUMN workload_statistic.sample_count IS 'Number of samples collected during the statistical period';
COMMENT ON COLUMN workload_statistic.allocated_gpu_count IS 'Average number of GPUs allocated to this workload';
COMMENT ON COLUMN workload_statistic.workload_status IS 'Status of the workload: Running, Completed, Failed, Pending, etc.';
COMMENT ON COLUMN workload_statistic.labels IS 'Workload labels stored in JSONB format for flexible querying';
COMMENT ON COLUMN workload_statistic.annotations IS 'Workload annotations stored in JSONB format for flexible querying';

-- ============================================
-- Incremental Update Enhancement
-- Fields for incremental update optimization (to reduce Prometheus query pressure)
-- ============================================

-- Add incremental update fields
ALTER TABLE workload_statistic 
    ADD COLUMN IF NOT EXISTS last_query_time TIMESTAMP,
    ADD COLUMN IF NOT EXISTS total_sum DOUBLE PRECISION DEFAULT 0,
    ADD COLUMN IF NOT EXISTS histogram JSONB DEFAULT '{"buckets": []}';

-- Create unique index for active workloads (one record per workload)
-- Ensures only one record per active workload
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_statistic_unique_active 
    ON workload_statistic(cluster_name, namespace, workload_name, uid)
    WHERE workload_status IN ('Running', 'Pending');

-- Index for completed workloads (for historical queries)
-- Optimizes queries for completed workloads
CREATE INDEX IF NOT EXISTS idx_workload_statistic_completed 
    ON workload_statistic(stat_end_time DESC)
    WHERE workload_status IN ('Completed', 'Failed', 'Deleted');

-- Index on last_query_time for incremental updates
CREATE INDEX IF NOT EXISTS idx_workload_statistic_last_query 
    ON workload_statistic(last_query_time DESC)
    WHERE last_query_time IS NOT NULL;

-- JSONB index for histogram queries
CREATE INDEX IF NOT EXISTS idx_workload_statistic_histogram 
    ON workload_statistic USING GIN(histogram);

-- Column comments for new fields
COMMENT ON COLUMN workload_statistic.last_query_time IS 'Last time Prometheus was queried for this workload (used for incremental updates)';
COMMENT ON COLUMN workload_statistic.total_sum IS 'Sum of all GPU utilization samples (used for incremental average calculation)';
COMMENT ON COLUMN workload_statistic.histogram IS 'Histogram data in JSONB format for percentile calculation, structure: {"buckets": [{"lower": 0, "upper": 10, "count": 5}, ...]}';

-- K8s Services Database Schema
-- This file defines the database schema for storing Kubernetes Service information
-- Used by gateway-exporter for traffic metrics enrichment

-- Table: k8s_services
-- Stores Kubernetes Service information for gateway traffic correlation
CREATE TABLE IF NOT EXISTS k8s_services (
    id BIGSERIAL PRIMARY KEY,
    uid VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(253) NOT NULL,
    cluster_ip VARCHAR(45),
    service_type VARCHAR(32),
    selector JSONB DEFAULT '{}',
    ports JSONB DEFAULT '[]',
    labels JSONB DEFAULT '{}',
    annotations JSONB DEFAULT '{}',
    deleted BOOLEAN DEFAULT false,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for k8s_services
CREATE UNIQUE INDEX IF NOT EXISTS idx_k8s_services_uid ON k8s_services(uid);
CREATE INDEX IF NOT EXISTS idx_k8s_services_name ON k8s_services(name);
CREATE INDEX IF NOT EXISTS idx_k8s_services_namespace ON k8s_services(namespace);
CREATE INDEX IF NOT EXISTS idx_k8s_services_namespace_name ON k8s_services(namespace, name);
CREATE INDEX IF NOT EXISTS idx_k8s_services_deleted ON k8s_services(deleted);
CREATE INDEX IF NOT EXISTS idx_k8s_services_updated_at ON k8s_services(updated_at DESC);

-- Comments for documentation
COMMENT ON TABLE k8s_services IS 'Kubernetes Service information storage for gateway traffic correlation';
COMMENT ON COLUMN k8s_services.uid IS 'Kubernetes Service UID (unique identifier)';
COMMENT ON COLUMN k8s_services.name IS 'Service name';
COMMENT ON COLUMN k8s_services.namespace IS 'Service namespace';
COMMENT ON COLUMN k8s_services.cluster_ip IS 'Service ClusterIP address';
COMMENT ON COLUMN k8s_services.service_type IS 'Service type (ClusterIP, NodePort, LoadBalancer, ExternalName)';
COMMENT ON COLUMN k8s_services.selector IS 'Service label selector as JSON';
COMMENT ON COLUMN k8s_services.ports IS 'Service ports configuration as JSON array';
COMMENT ON COLUMN k8s_services.labels IS 'Service labels as JSON';
COMMENT ON COLUMN k8s_services.annotations IS 'Service annotations as JSON';
COMMENT ON COLUMN k8s_services.deleted IS 'Whether the service has been deleted from Kubernetes';
COMMENT ON COLUMN k8s_services.created_at IS 'Timestamp when the service was first created';
COMMENT ON COLUMN k8s_services.updated_at IS 'Timestamp when the service was last updated';

-- Table: service_pod_references
-- Stores the relationship between Services and their backend Pods
CREATE TABLE IF NOT EXISTS service_pod_references (
    id BIGSERIAL PRIMARY KEY,
    service_uid VARCHAR(64) NOT NULL,
    service_name VARCHAR(253) NOT NULL,
    service_namespace VARCHAR(253) NOT NULL,
    pod_uid VARCHAR(64) NOT NULL,
    pod_name VARCHAR(253) NOT NULL,
    pod_ip VARCHAR(45),
    pod_labels JSONB DEFAULT '{}',
    workload_id VARCHAR(253),
    workload_owner VARCHAR(253),
    workload_type VARCHAR(64),
    node_name VARCHAR(253),
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for service_pod_references
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_service_uid ON service_pod_references(service_uid);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_service_name ON service_pod_references(service_name);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_service_namespace ON service_pod_references(service_namespace);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_namespace_name ON service_pod_references(service_namespace, service_name);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_pod_uid ON service_pod_references(pod_uid);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_workload_id ON service_pod_references(workload_id);
CREATE INDEX IF NOT EXISTS idx_service_pod_refs_updated_at ON service_pod_references(updated_at DESC);

-- Composite unique index to prevent duplicate service-pod relationships
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_pod_refs_unique ON service_pod_references(service_uid, pod_uid);

-- Comments for documentation
COMMENT ON TABLE service_pod_references IS 'Service to Pod relationship mapping for gateway traffic correlation';
COMMENT ON COLUMN service_pod_references.service_uid IS 'Reference to the Service UID';
COMMENT ON COLUMN service_pod_references.service_name IS 'Service name (denormalized for faster queries)';
COMMENT ON COLUMN service_pod_references.service_namespace IS 'Service namespace (denormalized for faster queries)';
COMMENT ON COLUMN service_pod_references.pod_uid IS 'Pod UID';
COMMENT ON COLUMN service_pod_references.pod_name IS 'Pod name';
COMMENT ON COLUMN service_pod_references.pod_ip IS 'Pod IP address';
COMMENT ON COLUMN service_pod_references.pod_labels IS 'Pod labels as JSON';
COMMENT ON COLUMN service_pod_references.workload_id IS 'Primus-SaFE workload identifier';
COMMENT ON COLUMN service_pod_references.workload_owner IS 'Workload owner (user name)';
COMMENT ON COLUMN service_pod_references.workload_type IS 'Workload type (deployment, statefulset, job, etc.)';
COMMENT ON COLUMN service_pod_references.node_name IS 'Node where the Pod is running';
COMMENT ON COLUMN service_pod_references.created_at IS 'Timestamp when the reference was first created';
COMMENT ON COLUMN service_pod_references.updated_at IS 'Timestamp when the reference was last updated';

-- Inference Framework Configurations
-- This migration adds configuration for inference frameworks (vLLM, TGI, Triton, etc.)
-- Version: 1.0.0

-- ============================================================================
-- vLLM Inference Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.vllm',
    '{
        "name": "vllm",
        "display_name": "vLLM",
        "version": "1.0.0",
        "priority": 90,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "vllm-import",
                "pattern": "vllm|from vllm import|import vllm",
                "description": "vLLM import statement",
                "enabled": true,
                "tags": ["import", "inference"],
                "confidence": 0.9
            },
            {
                "name": "vllm-server-log",
                "pattern": "vLLM|vllm\\.entrypoints|AsyncLLMEngine|LLMEngine",
                "description": "vLLM server log patterns",
                "enabled": true,
                "tags": ["server", "inference"],
                "confidence": 0.85
            }
        ],
        "performance_patterns": [
            {
                "name": "vllm-throughput",
                "pattern": "Throughput:\\s+(?P<throughput>[\\d.]+)\\s+requests/s",
                "description": "vLLM throughput metrics",
                "enabled": true,
                "tags": ["performance", "throughput"],
                "confidence": 0.9
            },
            {
                "name": "vllm-latency",
                "pattern": "Avg latency:\\s+(?P<latency>[\\d.]+)\\s*(ms|s)",
                "description": "vLLM latency metrics",
                "enabled": true,
                "tags": ["performance", "latency"],
                "confidence": 0.9
            }
        ],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "vllm-server-process",
                    "pattern": "vllm\\.entrypoints|python.*-m\\s+vllm|vllm\\.engine",
                    "description": "vLLM server process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.95
                }
            ],
            "ports": [8000],
            "env_patterns": [
                {
                    "name": "vllm-env-vars",
                    "pattern": "^VLLM_.*",
                    "description": "vLLM environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.8
                }
            ],
            "image_patterns": [
                {
                    "name": "vllm-official-image",
                    "pattern": "vllm/vllm-openai|vllm/vllm",
                    "description": "vLLM official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.95
                },
                {
                    "name": "vllm-custom-image",
                    "pattern": ".*vllm.*",
                    "description": "Custom vLLM container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.7
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "vllm-api-server",
                    "pattern": "vllm\\.entrypoints\\.openai\\.api_server|--served-model-name",
                    "description": "vLLM API server command line",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.9
                }
            ],
            "health_endpoint": "/health"
        },
        "extensions": {
            "model_loading_mode": "eager",
            "supports_openai_api": true,
            "default_port": 8000
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'vLLM inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- TGI (Text Generation Inference) Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.tgi',
    '{
        "name": "tgi",
        "display_name": "Text Generation Inference (TGI)",
        "version": "1.0.0",
        "priority": 85,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "tgi-server-log",
                "pattern": "text-generation-inference|text_generation_server|TGI",
                "description": "TGI server log patterns",
                "enabled": true,
                "tags": ["server", "inference"],
                "confidence": 0.9
            }
        ],
        "performance_patterns": [
            {
                "name": "tgi-request-metrics",
                "pattern": "request_count|batch_size|queue_size",
                "description": "TGI request metrics",
                "enabled": true,
                "tags": ["performance"],
                "confidence": 0.8
            }
        ],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "tgi-launcher",
                    "pattern": "text-generation-launcher|text-generation-router",
                    "description": "TGI launcher process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.95
                },
                {
                    "name": "tgi-server",
                    "pattern": "text_generation_server",
                    "description": "TGI server process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.9
                }
            ],
            "ports": [80, 8080, 3000],
            "env_patterns": [
                {
                    "name": "tgi-model-env",
                    "pattern": "^(MODEL_ID|HF_TOKEN|HUGGING_FACE_HUB_TOKEN)",
                    "description": "TGI model environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.7
                },
                {
                    "name": "tgi-config-env",
                    "pattern": "^(MAX_BATCH_PREFILL_TOKENS|MAX_INPUT_LENGTH|MAX_TOTAL_TOKENS)",
                    "description": "TGI configuration environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.85
                }
            ],
            "image_patterns": [
                {
                    "name": "tgi-official-image",
                    "pattern": "ghcr\\.io/huggingface/text-generation-inference",
                    "description": "TGI official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.95
                },
                {
                    "name": "tgi-custom-image",
                    "pattern": ".*text-generation-inference.*|.*tgi.*",
                    "description": "Custom TGI container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.7
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "tgi-server-cmd",
                    "pattern": "--model-id|--max-batch-prefill-tokens|--quantize",
                    "description": "TGI server command line arguments",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.85
                }
            ],
            "health_endpoint": "/health"
        },
        "extensions": {
            "supports_flash_attention": true,
            "supports_quantization": true,
            "default_port": 80
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Text Generation Inference (TGI) framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- Triton Inference Server Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.triton',
    '{
        "name": "triton",
        "display_name": "Triton Inference Server",
        "version": "1.0.0",
        "priority": 85,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "triton-server-log",
                "pattern": "tritonserver|Triton Inference Server|TRITON",
                "description": "Triton server log patterns",
                "enabled": true,
                "tags": ["server", "inference"],
                "confidence": 0.9
            }
        ],
        "performance_patterns": [
            {
                "name": "triton-inference-stats",
                "pattern": "Inference count|execution count|cumulative time",
                "description": "Triton inference statistics",
                "enabled": true,
                "tags": ["performance"],
                "confidence": 0.85
            }
        ],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "triton-server-process",
                    "pattern": "tritonserver",
                    "description": "Triton server process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.95
                }
            ],
            "ports": [8000, 8001, 8002],
            "env_patterns": [
                {
                    "name": "triton-env-vars",
                    "pattern": "^TRITON_.*",
                    "description": "Triton environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.85
                }
            ],
            "image_patterns": [
                {
                    "name": "triton-official-image",
                    "pattern": "nvcr\\.io/nvidia/tritonserver",
                    "description": "Triton official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.95
                },
                {
                    "name": "triton-custom-image",
                    "pattern": ".*tritonserver.*|.*triton.*inference.*",
                    "description": "Custom Triton container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.7
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "triton-server-cmd",
                    "pattern": "--model-repository|--model-control-mode|--strict-model-config",
                    "description": "Triton server command line arguments",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.9
                }
            ],
            "health_endpoint": "/v2/health/ready"
        },
        "extensions": {
            "supports_grpc": true,
            "supports_http": true,
            "http_port": 8000,
            "grpc_port": 8001,
            "metrics_port": 8002
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Triton Inference Server framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- TensorRT-LLM Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.tensorrt-llm',
    '{
        "name": "tensorrt-llm",
        "display_name": "TensorRT-LLM",
        "version": "1.0.0",
        "priority": 80,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "trtllm-import",
                "pattern": "tensorrt_llm|from tensorrt_llm import",
                "description": "TensorRT-LLM import statement",
                "enabled": true,
                "tags": ["import", "inference"],
                "confidence": 0.9
            }
        ],
        "performance_patterns": [],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "trtllm-process",
                    "pattern": "tensorrt_llm|trtllm",
                    "description": "TensorRT-LLM process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.85
                }
            ],
            "ports": [8000],
            "env_patterns": [
                {
                    "name": "trtllm-env-vars",
                    "pattern": "^TRTLLM_.*",
                    "description": "TensorRT-LLM environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.8
                }
            ],
            "image_patterns": [
                {
                    "name": "trtllm-official-image",
                    "pattern": "nvcr\\.io/nvidia/tensorrt",
                    "description": "TensorRT official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.9
                }
            ],
            "cmdline_patterns": [],
            "health_endpoint": "/health"
        },
        "extensions": {
            "supports_nvidia_gpu": true
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'TensorRT-LLM inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- ============================================================================
-- Ray Serve Framework Configuration
-- ============================================================================

INSERT INTO system_config (key, value, description, created_at, updated_at)
VALUES (
    'training.log.parser.framework.ray-serve',
    '{
        "name": "ray-serve",
        "display_name": "Ray Serve",
        "version": "1.0.0",
        "priority": 75,
        "enabled": true,
        "type": "inference",
        "identify_patterns": [
            {
                "name": "ray-serve-import",
                "pattern": "ray\\.serve|from ray import serve",
                "description": "Ray Serve import statement",
                "enabled": true,
                "tags": ["import", "inference"],
                "confidence": 0.85
            }
        ],
        "performance_patterns": [],
        "inference_patterns": {
            "process_patterns": [
                {
                    "name": "ray-serve-process",
                    "pattern": "ray::SERVE|serve\\.run|serve\\.deployment",
                    "description": "Ray Serve process",
                    "enabled": true,
                    "tags": ["process"],
                    "confidence": 0.8
                }
            ],
            "ports": [8000],
            "env_patterns": [
                {
                    "name": "ray-env-vars",
                    "pattern": "^RAY_.*",
                    "description": "Ray environment variables",
                    "enabled": true,
                    "tags": ["env"],
                    "confidence": 0.7
                }
            ],
            "image_patterns": [
                {
                    "name": "ray-official-image",
                    "pattern": "rayproject/ray",
                    "description": "Ray official container image",
                    "enabled": true,
                    "tags": ["image"],
                    "confidence": 0.85
                }
            ],
            "cmdline_patterns": [
                {
                    "name": "ray-serve-cmd",
                    "pattern": "serve\\.run|serve\\.deployment|@serve\\.deployment",
                    "description": "Ray Serve command line patterns",
                    "enabled": true,
                    "tags": ["cmdline"],
                    "confidence": 0.8
                }
            ],
            "health_endpoint": "/-/healthz"
        },
        "extensions": {
            "supports_autoscaling": true,
            "supports_batching": true
        },
        "updated_at": "2024-01-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z"
    }',
    'Ray Serve inference framework configuration',
    NOW(),
    NOW()
)
ON CONFLICT (key) DO UPDATE SET
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = NOW();

-- Migration: workload_detection_tables
-- Description: Add tables for evidence-based active framework detection
-- Purpose: Support active detection with evidence storage and aggregation
-- Database: PostgreSQL
-- Note: All logic (aggregation, fusion, conflict detection) is implemented in application layer

-- ============================================================================
-- Table 1: workload_detection_evidence
-- Purpose: Store ALL detection evidence from ALL sources (passive and active)
-- ============================================================================

CREATE TABLE IF NOT EXISTS workload_detection_evidence (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    
    -- Evidence source information
    source VARCHAR(100) NOT NULL,                    -- 'wandb', 'process', 'env', 'image', 'log', 'label', 'active_detection', etc.
    source_type VARCHAR(50) DEFAULT 'passive',       -- 'passive' or 'active'
    
    -- Detection result from this evidence
    framework VARCHAR(100),                          -- Primary detected framework
    frameworks JSONB DEFAULT '[]'::jsonb,            -- All detected frameworks ["primus", "megatron"]
    workload_type VARCHAR(50),                       -- 'training' or 'inference'
    confidence DECIMAL(4,3) NOT NULL DEFAULT 0.0,    -- Confidence score [0.000-1.000]
    
    -- Dual-layer framework support
    framework_layer VARCHAR(20),                     -- 'wrapper' or 'base'
    wrapper_framework VARCHAR(100),                  -- Wrapper framework (e.g., primus, lightning)
    base_framework VARCHAR(100),                     -- Base framework (e.g., megatron, deepspeed)
    
    -- Raw evidence data
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,     -- Source-specific evidence details
    
    -- Processing status
    processed BOOLEAN NOT NULL DEFAULT FALSE,        -- Whether this evidence has been processed by aggregator
    processed_at TIMESTAMPTZ,                        -- When this evidence was processed
    
    -- Timestamps
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- When this evidence was detected
    expires_at TIMESTAMPTZ,                          -- Optional: evidence expiration time
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- Indexes for workload_detection_evidence
-- ============================================================================

-- Primary query: get evidence for a workload
CREATE INDEX IF NOT EXISTS idx_wde_workload_uid 
ON workload_detection_evidence(workload_uid);

-- Query unprocessed evidence for aggregation
CREATE INDEX IF NOT EXISTS idx_wde_workload_processed 
ON workload_detection_evidence(workload_uid, processed) 
WHERE processed = FALSE;

-- Query by source
CREATE INDEX IF NOT EXISTS idx_wde_source 
ON workload_detection_evidence(source);

-- Query by framework
CREATE INDEX IF NOT EXISTS idx_wde_framework 
ON workload_detection_evidence(framework);

-- Query by confidence (for high-confidence evidence)
CREATE INDEX IF NOT EXISTS idx_wde_confidence 
ON workload_detection_evidence(confidence DESC);

-- Time-based queries
CREATE INDEX IF NOT EXISTS idx_wde_detected_at 
ON workload_detection_evidence(detected_at DESC);

-- Expiration cleanup
CREATE INDEX IF NOT EXISTS idx_wde_expires_at 
ON workload_detection_evidence(expires_at) 
WHERE expires_at IS NOT NULL;

-- Composite index for common query pattern
CREATE INDEX IF NOT EXISTS idx_wde_workload_source_time 
ON workload_detection_evidence(workload_uid, source, detected_at DESC);

-- GIN index for JSONB evidence field
CREATE INDEX IF NOT EXISTS idx_wde_evidence_gin 
ON workload_detection_evidence USING GIN(evidence);

-- GIN index for JSONB frameworks field
CREATE INDEX IF NOT EXISTS idx_wde_frameworks_gin 
ON workload_detection_evidence USING GIN(frameworks);

-- ============================================================================
-- Table 2: workload_detection
-- Purpose: Track detection state and aggregated results for active detection
-- ============================================================================

CREATE TABLE IF NOT EXISTS workload_detection (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL UNIQUE,
    
    -- Aggregated detection result
    status VARCHAR(50) NOT NULL DEFAULT 'unknown',   -- 'unknown', 'suspected', 'confirmed', 'verified', 'conflict'
    framework VARCHAR(100),                          -- Primary framework
    frameworks JSONB DEFAULT '[]'::jsonb,            -- All frameworks ["primus", "megatron"]
    workload_type VARCHAR(50),                       -- 'training' or 'inference'
    confidence DECIMAL(4,3) NOT NULL DEFAULT 0.0,    -- Aggregated confidence [0.000-1.000]
    
    -- Dual-layer framework support
    framework_layer VARCHAR(20),                     -- 'wrapper' or 'base'
    wrapper_framework VARCHAR(100),                  -- Wrapper framework
    base_framework VARCHAR(100),                     -- Base framework
    
    -- Active detection task state
    detection_state VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending', 'in_progress', 'completed', 'failed'
    attempt_count INTEGER NOT NULL DEFAULT 0,        -- Number of detection attempts
    max_attempts INTEGER NOT NULL DEFAULT 5,         -- Maximum attempts before giving up
    last_attempt_at TIMESTAMPTZ,                     -- Last attempt timestamp
    next_attempt_at TIMESTAMPTZ,                     -- Scheduled next attempt time
    
    -- Detection context and configuration
    context JSONB DEFAULT '{}'::jsonb,               -- Detection context (retry config, probe settings, etc.)
    
    -- Evidence summary
    evidence_count INTEGER NOT NULL DEFAULT 0,       -- Total number of evidence records
    evidence_sources JSONB DEFAULT '[]'::jsonb,      -- List of sources that contributed ["wandb", "process", "env"]
    conflicts JSONB DEFAULT '[]'::jsonb,             -- Conflict records if any
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ                         -- When detection was confirmed/verified
);

-- ============================================================================
-- Indexes for workload_detection
-- ============================================================================

-- Primary query: get detection by workload_uid (covered by UNIQUE constraint)
-- Additional index for partial queries
CREATE INDEX IF NOT EXISTS idx_wd_workload_uid 
ON workload_detection(workload_uid);

-- Query by detection status
CREATE INDEX IF NOT EXISTS idx_wd_status 
ON workload_detection(status);

-- Query by detection state (for task scheduler)
CREATE INDEX IF NOT EXISTS idx_wd_detection_state 
ON workload_detection(detection_state);

-- Query pending/in_progress detections
CREATE INDEX IF NOT EXISTS idx_wd_active_detection 
ON workload_detection(detection_state, next_attempt_at) 
WHERE detection_state IN ('pending', 'in_progress');

-- Query by framework
CREATE INDEX IF NOT EXISTS idx_wd_framework 
ON workload_detection(framework);

-- Query by workload type
CREATE INDEX IF NOT EXISTS idx_wd_workload_type 
ON workload_detection(workload_type);

-- Query by confidence (for analytics)
CREATE INDEX IF NOT EXISTS idx_wd_confidence 
ON workload_detection(confidence DESC);

-- Time-based queries
CREATE INDEX IF NOT EXISTS idx_wd_created_at 
ON workload_detection(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wd_updated_at 
ON workload_detection(updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_wd_confirmed_at 
ON workload_detection(confirmed_at DESC) 
WHERE confirmed_at IS NOT NULL;

-- Query detections needing retry
CREATE INDEX IF NOT EXISTS idx_wd_next_attempt 
ON workload_detection(next_attempt_at) 
WHERE next_attempt_at IS NOT NULL AND detection_state = 'pending';

-- GIN index for JSONB context field
CREATE INDEX IF NOT EXISTS idx_wd_context_gin 
ON workload_detection USING GIN(context);

-- GIN index for JSONB evidence_sources field
CREATE INDEX IF NOT EXISTS idx_wd_evidence_sources_gin 
ON workload_detection USING GIN(evidence_sources);

-- ============================================================================
-- Comments for workload_detection_evidence
-- ============================================================================

COMMENT ON TABLE workload_detection_evidence IS 'Stores all detection evidence from passive and active sources';
COMMENT ON COLUMN workload_detection_evidence.workload_uid IS 'Workload unique identifier';
COMMENT ON COLUMN workload_detection_evidence.source IS 'Evidence source: wandb, process, env, image, log, label, active_detection';
COMMENT ON COLUMN workload_detection_evidence.source_type IS 'Source type: passive (external trigger) or active (probing)';
COMMENT ON COLUMN workload_detection_evidence.framework IS 'Primary detected framework from this evidence';
COMMENT ON COLUMN workload_detection_evidence.frameworks IS 'All detected frameworks as JSON array';
COMMENT ON COLUMN workload_detection_evidence.workload_type IS 'Workload type: training or inference';
COMMENT ON COLUMN workload_detection_evidence.confidence IS 'Detection confidence from this evidence [0.000-1.000]';
COMMENT ON COLUMN workload_detection_evidence.framework_layer IS 'Framework layer: wrapper or base';
COMMENT ON COLUMN workload_detection_evidence.wrapper_framework IS 'Wrapper framework name (e.g., primus, lightning)';
COMMENT ON COLUMN workload_detection_evidence.base_framework IS 'Base framework name (e.g., megatron, deepspeed)';
COMMENT ON COLUMN workload_detection_evidence.evidence IS 'Raw evidence data specific to the source type (JSONB)';
COMMENT ON COLUMN workload_detection_evidence.processed IS 'Whether this evidence has been processed by the aggregator';
COMMENT ON COLUMN workload_detection_evidence.processed_at IS 'Timestamp when this evidence was processed';
COMMENT ON COLUMN workload_detection_evidence.detected_at IS 'Timestamp when this evidence was detected';
COMMENT ON COLUMN workload_detection_evidence.expires_at IS 'Optional expiration time for this evidence';

-- ============================================================================
-- Comments for workload_detection
-- ============================================================================

COMMENT ON TABLE workload_detection IS 'Tracks detection state and aggregated results for each workload';
COMMENT ON COLUMN workload_detection.workload_uid IS 'Workload unique identifier (unique)';
COMMENT ON COLUMN workload_detection.status IS 'Detection status: unknown, suspected, confirmed, verified, conflict';
COMMENT ON COLUMN workload_detection.framework IS 'Primary framework (aggregated result)';
COMMENT ON COLUMN workload_detection.frameworks IS 'All frameworks as JSON array (aggregated result)';
COMMENT ON COLUMN workload_detection.workload_type IS 'Workload type: training or inference';
COMMENT ON COLUMN workload_detection.confidence IS 'Aggregated confidence score [0.000-1.000]';
COMMENT ON COLUMN workload_detection.framework_layer IS 'Framework layer: wrapper or base';
COMMENT ON COLUMN workload_detection.wrapper_framework IS 'Wrapper framework name';
COMMENT ON COLUMN workload_detection.base_framework IS 'Base framework name';
COMMENT ON COLUMN workload_detection.detection_state IS 'Active detection task state: pending, in_progress, completed, failed';
COMMENT ON COLUMN workload_detection.attempt_count IS 'Number of detection attempts made';
COMMENT ON COLUMN workload_detection.max_attempts IS 'Maximum detection attempts before giving up';
COMMENT ON COLUMN workload_detection.last_attempt_at IS 'Timestamp of last detection attempt';
COMMENT ON COLUMN workload_detection.next_attempt_at IS 'Scheduled time for next detection attempt';
COMMENT ON COLUMN workload_detection.context IS 'Detection context: retry config, probe settings, etc. (JSONB)';
COMMENT ON COLUMN workload_detection.evidence_count IS 'Total number of evidence records for this workload';
COMMENT ON COLUMN workload_detection.evidence_sources IS 'List of sources that contributed evidence (JSON array)';
COMMENT ON COLUMN workload_detection.conflicts IS 'Detection conflicts if any (JSON array)';
COMMENT ON COLUMN workload_detection.confirmed_at IS 'Timestamp when detection was confirmed or verified';

-- ============================================================================
-- Application layer responsibilities
-- ============================================================================
-- 1. Evidence management
--    - Store evidence from various sources (wandb, process, env, image, log, etc.)
--    - Query unprocessed evidence for aggregation
--    - Mark evidence as processed after aggregation
--    - Cleanup expired evidence periodically
--
-- 2. Evidence aggregation
--    - Aggregate evidence from multiple sources
--    - Calculate weighted confidence based on source priority
--    - Apply multi-source bonus to confidence
--    - Detect conflicts between different sources
--
-- 3. Detection state management
--    - Create detection record when workload is discovered
--    - Update detection state based on aggregation results
--    - Manage retry logic (attempt_count, next_attempt_at)
--    - Track evidence sources and conflicts
--
-- 4. Active detection task
--    - Query pending detections for task scheduling
--    - Execute detection probes (process, env, image, labels)
--    - Store collected evidence
--    - Re-aggregate after new evidence
--
-- See: ai-advisor/pkg/detection/evidence_aggregator.go (to be implemented)
-- See: ai-advisor/pkg/task/active_detection_executor.go (to be implemented)
-- Docs: ai-advisor/docs/active-detection-task-design.md

-- Migration: migrate_detection_data
-- Description: Migrate existing detection data from ai_workload_metadata to new tables
-- Database: PostgreSQL
-- 
-- IMPORTANT: Run this AFTER patch037-workload_detection_tables.sql
-- 
-- This migration:
-- 1. Creates workload_detection records from ai_workload_metadata
-- 2. Creates workload_detection_evidence records for each detection
-- 3. Does NOT modify or delete data from ai_workload_metadata

-- ============================================================================
-- Step 1: Migrate to workload_detection table
-- Creates aggregated detection records from existing metadata
-- ============================================================================

INSERT INTO workload_detection (
    workload_uid,
    status,
    framework,
    frameworks,
    workload_type,
    confidence,
    framework_layer,
    wrapper_framework,
    base_framework,
    detection_state,
    attempt_count,
    max_attempts,
    evidence_count,
    evidence_sources,
    created_at,
    updated_at,
    confirmed_at
)
SELECT 
    m.workload_uid,
    COALESCE(
        m.metadata->>'detection_status',
        CASE 
            WHEN m.framework IS NOT NULL AND m.framework != '' THEN 'confirmed'
            ELSE 'unknown'
        END
    ) as status,
    m.framework,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' 
        THEN jsonb_build_array(m.framework)
        ELSE '[]'::jsonb
    END as frameworks,
    m.type as workload_type,
    COALESCE(
        (m.metadata->>'confidence')::numeric,
        CASE 
            WHEN m.framework IS NOT NULL AND m.framework != '' THEN 0.8
            ELSE 0.0
        END
    ) as confidence,
    m.metadata->>'framework_layer' as framework_layer,
    m.metadata->>'wrapper_framework' as wrapper_framework,
    m.metadata->>'base_framework' as base_framework,
    'completed' as detection_state,  -- Migrated records are already completed
    0 as attempt_count,
    5 as max_attempts,
    1 as evidence_count,  -- At least one evidence from migration
    jsonb_build_array('migration') as evidence_sources,
    m.created_at,
    NOW() as updated_at,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' THEN m.created_at
        ELSE NULL
    END as confirmed_at
FROM ai_workload_metadata m
WHERE NOT EXISTS (
    SELECT 1 FROM workload_detection wd 
    WHERE wd.workload_uid = m.workload_uid
)
ON CONFLICT (workload_uid) DO NOTHING;

-- ============================================================================
-- Step 2: Migrate to workload_detection_evidence table
-- Creates evidence records from existing metadata
-- ============================================================================

-- Create evidence records for each metadata entry with a framework
INSERT INTO workload_detection_evidence (
    workload_uid,
    source,
    source_type,
    framework,
    frameworks,
    workload_type,
    confidence,
    framework_layer,
    wrapper_framework,
    base_framework,
    evidence,
    processed,
    processed_at,
    detected_at,
    created_at
)
SELECT 
    m.workload_uid,
    'migration' as source,
    'passive' as source_type,
    m.framework,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' 
        THEN jsonb_build_array(m.framework)
        ELSE '[]'::jsonb
    END as frameworks,
    m.type as workload_type,
    COALESCE(
        (m.metadata->>'confidence')::numeric,
        CASE 
            WHEN m.framework IS NOT NULL AND m.framework != '' THEN 0.8
            ELSE 0.0
        END
    ) as confidence,
    m.metadata->>'framework_layer' as framework_layer,
    m.metadata->>'wrapper_framework' as wrapper_framework,
    m.metadata->>'base_framework' as base_framework,
    jsonb_build_object(
        'migrated_from', 'ai_workload_metadata',
        'migrated_at', NOW()::text,
        'original_id', m.id,
        'original_metadata', m.metadata
    ) as evidence,
    true as processed,
    NOW() as processed_at,
    m.created_at as detected_at,
    NOW() as created_at
FROM ai_workload_metadata m
WHERE m.framework IS NOT NULL 
  AND m.framework != ''
  AND NOT EXISTS (
    SELECT 1 FROM workload_detection_evidence wde 
    WHERE wde.workload_uid = m.workload_uid 
      AND wde.source = 'migration'
);

-- ============================================================================
-- Step 3: Create evidence records for WandB detections (if wandb data exists)
-- ============================================================================

INSERT INTO workload_detection_evidence (
    workload_uid,
    source,
    source_type,
    framework,
    frameworks,
    workload_type,
    confidence,
    evidence,
    processed,
    processed_at,
    detected_at,
    created_at
)
SELECT 
    m.workload_uid,
    'wandb' as source,
    'passive' as source_type,
    m.framework,
    CASE 
        WHEN m.framework IS NOT NULL AND m.framework != '' 
        THEN jsonb_build_array(m.framework)
        ELSE '[]'::jsonb
    END as frameworks,
    m.type as workload_type,
    1.0 as confidence,  -- WandB is high confidence
    jsonb_build_object(
        'wandb_data', m.metadata->'wandb',
        'migrated_from', 'ai_workload_metadata',
        'migrated_at', NOW()::text
    ) as evidence,
    true as processed,
    NOW() as processed_at,
    m.created_at as detected_at,
    NOW() as created_at
FROM ai_workload_metadata m
WHERE m.metadata ? 'wandb'
  AND m.metadata->'wandb' IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM workload_detection_evidence wde 
    WHERE wde.workload_uid = m.workload_uid 
      AND wde.source = 'wandb'
);

-- ============================================================================
-- Step 4: Update evidence counts in workload_detection
-- ============================================================================

UPDATE workload_detection wd
SET 
    evidence_count = (
        SELECT COUNT(*) 
        FROM workload_detection_evidence wde 
        WHERE wde.workload_uid = wd.workload_uid
    ),
    evidence_sources = (
        SELECT jsonb_agg(DISTINCT wde.source)
        FROM workload_detection_evidence wde
        WHERE wde.workload_uid = wd.workload_uid
    ),
    updated_at = NOW()
WHERE EXISTS (
    SELECT 1 FROM workload_detection_evidence wde 
    WHERE wde.workload_uid = wd.workload_uid
);

-- ============================================================================
-- Verification queries (run these to check migration results)
-- ============================================================================

-- Check migration counts:
-- SELECT 'ai_workload_metadata' as table_name, COUNT(*) as count FROM ai_workload_metadata
-- UNION ALL
-- SELECT 'workload_detection', COUNT(*) FROM workload_detection
-- UNION ALL
-- SELECT 'workload_detection_evidence', COUNT(*) FROM workload_detection_evidence;

-- Check detection status distribution:
-- SELECT status, COUNT(*) as count 
-- FROM workload_detection 
-- GROUP BY status 
-- ORDER BY count DESC;

-- Check evidence source distribution:
-- SELECT source, COUNT(*) as count 
-- FROM workload_detection_evidence 
-- GROUP BY source 
-- ORDER BY count DESC;

-- Migration: detection_coverage
-- Description: Track evidence collection coverage for each workload and source
-- Database: PostgreSQL
--
-- This table tracks:
-- 1. Which evidence sources have been collected for each workload
-- 2. Collection status and attempt history
-- 3. Time window coverage for sources like logs (to detect gaps)
-- 4. Log availability timestamps from telemetry-processor

-- ============================================================================
-- Table: detection_coverage
-- Tracks evidence collection coverage per workload and source
-- ============================================================================

CREATE TABLE IF NOT EXISTS detection_coverage (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    source VARCHAR(50) NOT NULL,
    -- Evidence sources:
    -- 'process': cmdline, env, cwd from process tree
    -- 'log': log pattern matching
    -- 'image': container image name/tag
    -- 'label': pod labels/annotations
    -- 'wandb': WandB metadata
    -- 'import': Python import detection

    -- Coverage status
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- Status values:
    -- 'pending': waiting to be collected
    -- 'collecting': collection in progress
    -- 'collected': successfully collected
    -- 'failed': collection failed
    -- 'not_applicable': source not applicable for this workload

    -- Collection attempt tracking
    attempt_count INT NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_error TEXT,

    -- Time window coverage (for sources with time ranges, e.g., logs)
    covered_from TIMESTAMPTZ,     -- Start of covered time range
    covered_to TIMESTAMPTZ,       -- End of covered time range
    pending_from TIMESTAMPTZ,     -- Start of pending backfill range
    pending_to TIMESTAMPTZ,       -- End of pending backfill range

    -- Log source specific fields (updated by telemetry-processor)
    log_available_from TIMESTAMPTZ,  -- Earliest log timestamp reported
    log_available_to TIMESTAMPTZ,    -- Latest log timestamp reported

    -- Evidence statistics
    evidence_count INT NOT NULL DEFAULT 0,

    -- Extensible metadata
    ext JSONB DEFAULT '{}'::jsonb,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Unique constraint: one record per workload per source
    UNIQUE(workload_uid, source)
);

-- Add comments
COMMENT ON TABLE detection_coverage IS 'Tracks evidence collection coverage for each workload and source';
COMMENT ON COLUMN detection_coverage.workload_uid IS 'Workload unique identifier';
COMMENT ON COLUMN detection_coverage.source IS 'Evidence source: process, log, image, label, wandb, import';
COMMENT ON COLUMN detection_coverage.status IS 'Collection status: pending, collecting, collected, failed, not_applicable';
COMMENT ON COLUMN detection_coverage.attempt_count IS 'Number of collection attempts';
COMMENT ON COLUMN detection_coverage.covered_from IS 'Start of time range that has been scanned (for log source)';
COMMENT ON COLUMN detection_coverage.covered_to IS 'End of time range that has been scanned (for log source)';
COMMENT ON COLUMN detection_coverage.pending_from IS 'Start of time range pending backfill (for log source)';
COMMENT ON COLUMN detection_coverage.pending_to IS 'End of time range pending backfill (for log source)';
COMMENT ON COLUMN detection_coverage.log_available_from IS 'Earliest log timestamp from telemetry-processor';
COMMENT ON COLUMN detection_coverage.log_available_to IS 'Latest log timestamp from telemetry-processor';
COMMENT ON COLUMN detection_coverage.evidence_count IS 'Number of evidence records collected from this source';

-- ============================================================================
-- Indexes
-- ============================================================================

-- Primary lookup by workload
CREATE INDEX IF NOT EXISTS idx_dc_workload_uid ON detection_coverage(workload_uid);

-- Filter by status (for finding pending collections)
CREATE INDEX IF NOT EXISTS idx_dc_status ON detection_coverage(status);

-- Filter by source
CREATE INDEX IF NOT EXISTS idx_dc_source ON detection_coverage(source);

-- Composite index for common query pattern
CREATE INDEX IF NOT EXISTS idx_dc_workload_source ON detection_coverage(workload_uid, source);

-- Partial index for pending items (efficient for coordinator polling)
CREATE INDEX IF NOT EXISTS idx_dc_pending ON detection_coverage(workload_uid) 
    WHERE status = 'pending';

-- Partial index for log source with available data
CREATE INDEX IF NOT EXISTS idx_dc_log_available ON detection_coverage(workload_uid, log_available_to) 
    WHERE source = 'log' AND log_available_to IS NOT NULL;

-- Index for finding coverage gaps (log source)
CREATE INDEX IF NOT EXISTS idx_dc_log_coverage_gap ON detection_coverage(workload_uid)
    WHERE source = 'log' 
    AND log_available_to IS NOT NULL 
    AND (covered_to IS NULL OR covered_to < log_available_to);



DO
$$
    DECLARE
        r RECORD;
    BEGIN
        FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public'
            LOOP
                EXECUTE format('ALTER TABLE public.%I OWNER TO "primus-lens";', r.tablename);
            END LOOP;
    END
$$;