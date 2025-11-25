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

