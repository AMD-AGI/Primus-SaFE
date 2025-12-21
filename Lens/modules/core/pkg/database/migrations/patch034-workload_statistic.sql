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

