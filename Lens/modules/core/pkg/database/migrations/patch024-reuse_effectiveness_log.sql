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

