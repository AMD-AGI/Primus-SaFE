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

