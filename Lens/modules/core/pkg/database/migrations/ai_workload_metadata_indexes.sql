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

