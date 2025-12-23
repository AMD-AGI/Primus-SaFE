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

