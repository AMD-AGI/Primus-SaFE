-- Migration: Add pod_running_periods table
-- Purpose: Track precise running time periods for pods to calculate accurate GPU allocation
-- 
-- This table records when pods enter and exit the Running phase,
-- enabling accurate time-weighted GPU allocation calculations.
-- 
-- Key benefits:
-- 1. Precise GPU allocation: Only count time when pods are actually Running
-- 2. Simple time-range queries: Directly filter pods that were running in a time window
-- 3. Support pod restarts: Multiple running periods per pod

-- Create pod_running_periods table
CREATE TABLE IF NOT EXISTS pod_running_periods (
    id BIGSERIAL PRIMARY KEY,
    
    -- Pod identification
    pod_uid VARCHAR(128) NOT NULL,
    namespace VARCHAR(64) NOT NULL,
    pod_name VARCHAR(253),           -- k8s pod name max 253 chars
    
    -- Running time range
    start_at TIMESTAMPTZ NOT NULL,   -- Time when pod entered Running phase
    end_at TIMESTAMPTZ,              -- Time when pod left Running phase (NULL = still running)
    
    -- GPU allocation (denormalized for efficient queries)
    gpu_allocated INT NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Primary index for time-range queries (most important for GPU aggregation)
-- Query pattern: WHERE start_at < :end_time AND (end_at IS NULL OR end_at > :start_time)
CREATE INDEX IF NOT EXISTS idx_pod_running_periods_time_range 
    ON pod_running_periods(start_at, end_at DESC NULLS FIRST);

-- Index for pod lookup
CREATE INDEX IF NOT EXISTS idx_pod_running_periods_pod_uid 
    ON pod_running_periods(pod_uid);

-- Composite index for namespace + time range queries
CREATE INDEX IF NOT EXISTS idx_pod_running_periods_ns_time 
    ON pod_running_periods(namespace, start_at, end_at DESC NULLS FIRST);

-- Index for finding currently running pods
CREATE INDEX IF NOT EXISTS idx_pod_running_periods_running 
    ON pod_running_periods(end_at) WHERE end_at IS NULL;

-- Add comment
COMMENT ON TABLE pod_running_periods IS 'Tracks precise running time periods for pods to enable accurate GPU allocation calculations';
COMMENT ON COLUMN pod_running_periods.start_at IS 'Timestamp when pod entered Running phase';
COMMENT ON COLUMN pod_running_periods.end_at IS 'Timestamp when pod left Running phase (NULL means still running)';
COMMENT ON COLUMN pod_running_periods.gpu_allocated IS 'Number of GPUs allocated to this pod (denormalized from gpu_pods)';
