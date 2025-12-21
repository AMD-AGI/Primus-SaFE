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

