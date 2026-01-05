-- Migration: github_workflow_metrics
-- Description: Tables for GitHub Workflow Metrics Collector feature
-- Database: PostgreSQL
--
-- This migration creates tables for:
-- 1. github_workflow_configs - Configuration for binding AutoscalingRunnerSet to metric collection
-- 2. github_workflow_runs - Run records for each EphemeralRunner processed
-- 3. github_workflow_metric_schemas - Schema definitions for extracted metrics
-- 4. github_workflow_metrics - Actual metric data extracted from workflow results

-- ============================================================================
-- Table: github_workflow_configs
-- Configuration for binding AutoscalingRunnerSet to metric collection
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_configs (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- AutoscalingRunnerSet Binding
    runner_set_namespace VARCHAR(255) NOT NULL,
    runner_set_name VARCHAR(255) NOT NULL,
    runner_set_uid VARCHAR(255),
    
    -- GitHub Configuration
    github_owner VARCHAR(255) NOT NULL,
    github_repo VARCHAR(255) NOT NULL,
    workflow_filter VARCHAR(255),
    branch_filter VARCHAR(255),
    
    -- File Collection Configuration
    file_patterns JSONB NOT NULL DEFAULT '[]'::jsonb,
    
    -- AI Extraction Configuration
    metric_schema_id BIGINT,
    
    -- Status
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_processed_workload_uid VARCHAR(255),
    last_checked_at TIMESTAMPTZ,
    
    -- Multi-cluster support
    cluster_name VARCHAR(255) NOT NULL DEFAULT '',
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Comments
COMMENT ON TABLE github_workflow_configs IS 'Configuration for binding AutoscalingRunnerSet to metric collection';
COMMENT ON COLUMN github_workflow_configs.runner_set_namespace IS 'Namespace of the AutoscalingRunnerSet';
COMMENT ON COLUMN github_workflow_configs.runner_set_name IS 'Name of the AutoscalingRunnerSet';
COMMENT ON COLUMN github_workflow_configs.runner_set_uid IS 'UID of the AutoscalingRunnerSet (optional, for precise matching)';
COMMENT ON COLUMN github_workflow_configs.github_owner IS 'GitHub repository owner';
COMMENT ON COLUMN github_workflow_configs.github_repo IS 'GitHub repository name';
COMMENT ON COLUMN github_workflow_configs.workflow_filter IS 'Optional workflow filename filter (e.g., benchmark.yml)';
COMMENT ON COLUMN github_workflow_configs.branch_filter IS 'Optional branch filter (e.g., main)';
COMMENT ON COLUMN github_workflow_configs.file_patterns IS 'JSON array of file path patterns (glob) to collect';
COMMENT ON COLUMN github_workflow_configs.metric_schema_id IS 'Associated schema ID (populated after AI generates)';
COMMENT ON COLUMN github_workflow_configs.last_processed_workload_uid IS 'UID of last processed EphemeralRunner';

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_configs_unique
    ON github_workflow_configs(runner_set_namespace, runner_set_name, cluster_name);

CREATE INDEX IF NOT EXISTS idx_github_workflow_configs_enabled
    ON github_workflow_configs(enabled) WHERE enabled = TRUE;

CREATE INDEX IF NOT EXISTS idx_github_workflow_configs_cluster
    ON github_workflow_configs(cluster_name);


-- ============================================================================
-- Table: github_workflow_runs
-- Run records for each EphemeralRunner processed
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_runs (
    id BIGSERIAL PRIMARY KEY,
    config_id BIGINT NOT NULL,
    
    -- Workload Reference (from Lens gpu_workloads)
    workload_uid VARCHAR(255) NOT NULL,
    workload_name VARCHAR(255),
    workload_namespace VARCHAR(255),
    
    -- GitHub Reference (extracted from EphemeralRunner annotations)
    github_run_id BIGINT,
    github_run_number INT,
    github_job_id BIGINT,
    head_sha VARCHAR(64),
    head_branch VARCHAR(255),
    workflow_name VARCHAR(255),
    
    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    -- Status values: pending, collecting, extracting, completed, failed, skipped
    trigger_source VARCHAR(50) NOT NULL DEFAULT 'realtime',
    -- Trigger source: realtime, backfill, manual
    
    -- Collection Info
    files_found INT NOT NULL DEFAULT 0,
    files_processed INT NOT NULL DEFAULT 0,
    metrics_count INT NOT NULL DEFAULT 0,
    
    -- Timestamps
    workload_started_at TIMESTAMPTZ,
    workload_completed_at TIMESTAMPTZ,
    collection_started_at TIMESTAMPTZ,
    collection_completed_at TIMESTAMPTZ,
    
    -- Error Handling
    error_message TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Comments
COMMENT ON TABLE github_workflow_runs IS 'Run records for each EphemeralRunner processed';
COMMENT ON COLUMN github_workflow_runs.config_id IS 'Associated configuration ID';
COMMENT ON COLUMN github_workflow_runs.workload_uid IS 'Lens workload UID (from gpu_workloads table)';
COMMENT ON COLUMN github_workflow_runs.workload_name IS 'Workload name';
COMMENT ON COLUMN github_workflow_runs.workload_namespace IS 'Workload namespace';
COMMENT ON COLUMN github_workflow_runs.github_run_id IS 'GitHub workflow run ID (from annotations)';
COMMENT ON COLUMN github_workflow_runs.github_job_id IS 'GitHub job ID (from annotations)';
COMMENT ON COLUMN github_workflow_runs.status IS 'Processing status: pending, collecting, extracting, completed, failed, skipped';
COMMENT ON COLUMN github_workflow_runs.trigger_source IS 'How this run was triggered: realtime, backfill, manual';
COMMENT ON COLUMN github_workflow_runs.files_found IS 'Number of files matching patterns';
COMMENT ON COLUMN github_workflow_runs.files_processed IS 'Number of files successfully processed';
COMMENT ON COLUMN github_workflow_runs.metrics_count IS 'Number of metric records extracted';

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_runs_config_workload
    ON github_workflow_runs(config_id, workload_uid);

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_config_status
    ON github_workflow_runs(config_id, status);

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_workload_uid
    ON github_workflow_runs(workload_uid);

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_workload_completed
    ON github_workflow_runs(workload_completed_at DESC);

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_pending
    ON github_workflow_runs(config_id, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_github_run
    ON github_workflow_runs(github_run_id)
    WHERE github_run_id IS NOT NULL;


-- ============================================================================
-- Table: github_workflow_metric_schemas
-- Schema definitions for extracted metrics (AI-generated or user-defined)
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_metric_schemas (
    id BIGSERIAL PRIMARY KEY,
    config_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    version INT NOT NULL DEFAULT 1,
    
    -- Schema Definition
    fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- fields example:
    -- [
    --   {"name": "model_name", "type": "string", "description": "Model name"},
    --   {"name": "throughput", "type": "float", "unit": "tokens/s", "description": "Throughput"},
    --   {"name": "latency_p50", "type": "float", "unit": "ms", "description": "P50 latency"}
    -- ]
    
    -- Field Classification
    dimension_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- dimension_fields example: ["model_name", "batch_size", "precision"]
    
    metric_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- metric_fields example: ["throughput", "latency_p50", "latency_p99", "gpu_memory_used"]
    
    -- Status
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    
    -- AI Generation Info
    generated_by VARCHAR(50),
    -- generated_by: 'ai', 'user', 'system'
    generation_sample_file TEXT,
    -- Sample file content used for AI generation
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Comments
COMMENT ON TABLE github_workflow_metric_schemas IS 'Schema definitions for extracted metrics';
COMMENT ON COLUMN github_workflow_metric_schemas.config_id IS 'Associated configuration ID';
COMMENT ON COLUMN github_workflow_metric_schemas.name IS 'Schema name';
COMMENT ON COLUMN github_workflow_metric_schemas.version IS 'Schema version number';
COMMENT ON COLUMN github_workflow_metric_schemas.fields IS 'JSON array of field definitions with name, type, unit, description';
COMMENT ON COLUMN github_workflow_metric_schemas.dimension_fields IS 'JSON array of field names used as dimensions (for grouping)';
COMMENT ON COLUMN github_workflow_metric_schemas.metric_fields IS 'JSON array of field names used as metrics (numeric values)';
COMMENT ON COLUMN github_workflow_metric_schemas.is_active IS 'Whether this schema version is currently active';
COMMENT ON COLUMN github_workflow_metric_schemas.generated_by IS 'How schema was generated: ai, user, system';

-- Indexes
CREATE INDEX IF NOT EXISTS idx_github_workflow_metric_schemas_config
    ON github_workflow_metric_schemas(config_id);

CREATE INDEX IF NOT EXISTS idx_github_workflow_metric_schemas_active
    ON github_workflow_metric_schemas(config_id, is_active)
    WHERE is_active = TRUE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_metric_schemas_config_version
    ON github_workflow_metric_schemas(config_id, version);


-- ============================================================================
-- Table: github_workflow_metrics
-- Actual metric data extracted from workflow results
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_metrics (
    id BIGSERIAL PRIMARY KEY,
    config_id BIGINT NOT NULL,
    run_id BIGINT NOT NULL,
    schema_id BIGINT NOT NULL,
    
    -- Time dimension
    timestamp TIMESTAMPTZ NOT NULL,
    
    -- Source info
    source_file VARCHAR(1024),
    
    -- Dimension values (flexible JSONB)
    dimensions JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- dimensions example: {"model_name": "llama-70b", "batch_size": 32, "precision": "fp16"}
    
    -- Metric values (flexible JSONB)
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- metrics example: {"throughput": 1234.5, "latency_p50": 10.2, "latency_p99": 25.8}
    
    -- Raw data for debugging (optional)
    raw_data JSONB,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Comments
COMMENT ON TABLE github_workflow_metrics IS 'Metric data extracted from GitHub workflow results';
COMMENT ON COLUMN github_workflow_metrics.config_id IS 'Configuration ID';
COMMENT ON COLUMN github_workflow_metrics.run_id IS 'Associated workflow run ID';
COMMENT ON COLUMN github_workflow_metrics.schema_id IS 'Schema used for extraction';
COMMENT ON COLUMN github_workflow_metrics.timestamp IS 'Metric timestamp (workflow completion time or extracted from data)';
COMMENT ON COLUMN github_workflow_metrics.source_file IS 'Source file path from which metrics were extracted';
COMMENT ON COLUMN github_workflow_metrics.dimensions IS 'JSONB object of dimension key-value pairs';
COMMENT ON COLUMN github_workflow_metrics.metrics IS 'JSONB object of metric key-value pairs (numeric)';
COMMENT ON COLUMN github_workflow_metrics.raw_data IS 'Original raw data for debugging (optional)';

-- Indexes for time-series queries
CREATE INDEX IF NOT EXISTS idx_github_workflow_metrics_config_time
    ON github_workflow_metrics(config_id, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_github_workflow_metrics_run
    ON github_workflow_metrics(run_id);

CREATE INDEX IF NOT EXISTS idx_github_workflow_metrics_schema
    ON github_workflow_metrics(schema_id);

-- GIN index for dimension filtering
CREATE INDEX IF NOT EXISTS idx_github_workflow_metrics_dimensions
    ON github_workflow_metrics USING GIN(dimensions);

-- Composite index for common query pattern
CREATE INDEX IF NOT EXISTS idx_github_workflow_metrics_config_schema_time
    ON github_workflow_metrics(config_id, schema_id, timestamp DESC);

