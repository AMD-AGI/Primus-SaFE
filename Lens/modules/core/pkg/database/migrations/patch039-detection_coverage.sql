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


