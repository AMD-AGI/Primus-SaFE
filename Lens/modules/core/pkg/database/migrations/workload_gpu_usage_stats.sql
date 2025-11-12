-- Workload-level GPU Usage Statistics Table
-- For storing hourly aggregated GPU usage data at the workload level

-- Workload-level hourly aggregation table
CREATE TABLE IF NOT EXISTS workload_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    namespace VARCHAR(253) NOT NULL,  -- K8s namespace
    workload_name VARCHAR(253) NOT NULL,  -- Workload name
    workload_type VARCHAR(50) NOT NULL,  -- Workload type: Job/Deployment/StatefulSet/etc.
    
    -- Time dimension (aggregated by hour)
    stat_hour TIMESTAMP NOT NULL,  -- Start time of the statistical hour, e.g.: 2025-11-05 14:00:00
    
    -- GPU allocation statistics
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated to this workload (average)
    requested_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs requested by this workload (average)
    
    -- GPU utilization statistics (based on actual GPU metrics)
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Average utilization (0-100)
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Peak utilization
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Minimum utilization
    p50_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 50th percentile utilization
    p95_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 95th percentile utilization
    
    -- GPU memory statistics (optional, in GB)
    avg_gpu_memory_used DOUBLE PRECISION NOT NULL DEFAULT 0,   -- Average GPU memory used
    max_gpu_memory_used DOUBLE PRECISION NOT NULL DEFAULT 0,   -- Peak GPU memory used
    avg_gpu_memory_total DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Average GPU memory total
    
    -- Workload lifecycle statistics
    avg_replica_count DOUBLE PRECISION NOT NULL DEFAULT 0,     -- Average number of replicas during this hour
    max_replica_count INT NOT NULL DEFAULT 0,                  -- Maximum number of replicas
    min_replica_count INT NOT NULL DEFAULT 0,                  -- Minimum number of replicas
    
    -- Workload status
    workload_status VARCHAR(50),  -- Running/Completed/Failed/etc.
    
    -- Sampling statistics
    sample_count INT NOT NULL DEFAULT 0,  -- Number of samples taken during this hour
    
    -- Owner information (optional)
    owner_uid VARCHAR(255),  -- Owner UID from workload metadata
    owner_name VARCHAR(255), -- Owner name (user/team)
    
    -- Labels and annotations (stored as JSONB for flexible querying)
    labels JSONB NOT NULL DEFAULT '{}',      -- Workload labels
    annotations JSONB NOT NULL DEFAULT '{}', -- Workload annotations
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index design
-- Primary query index: by cluster, namespace, workload, and time
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_gpu_hourly_cluster_ns_wl_hour 
    ON workload_gpu_hourly_stats(cluster_name, namespace, workload_name, stat_hour DESC);

-- Query by cluster and time
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_cluster_hour 
    ON workload_gpu_hourly_stats(cluster_name, stat_hour DESC);

-- Query by namespace and time
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_namespace_hour 
    ON workload_gpu_hourly_stats(namespace, stat_hour DESC);

-- Query by workload name and time (across clusters/namespaces)
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_workload_hour 
    ON workload_gpu_hourly_stats(workload_name, stat_hour DESC);

-- Query by workload type and time
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_type_hour 
    ON workload_gpu_hourly_stats(workload_type, stat_hour DESC);

-- Query by owner
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_owner 
    ON workload_gpu_hourly_stats(owner_uid, stat_hour DESC) WHERE owner_uid IS NOT NULL;

-- Query by time only (for cleanup or full scans)
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_hour 
    ON workload_gpu_hourly_stats(stat_hour DESC);

-- JSONB index for label queries
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_labels 
    ON workload_gpu_hourly_stats USING GIN(labels);

-- JSONB index for annotation queries
CREATE INDEX IF NOT EXISTS idx_workload_gpu_hourly_annotations 
    ON workload_gpu_hourly_stats USING GIN(annotations);

-- Table comments
COMMENT ON TABLE workload_gpu_hourly_stats IS 'Workload-level GPU utilization hourly aggregation statistics table, recording historical GPU usage for each workload';

-- Column comments
COMMENT ON COLUMN workload_gpu_hourly_stats.stat_hour IS 'Statistical hour (rounded down to the hour), e.g. 2025-11-05 14:00:00';
COMMENT ON COLUMN workload_gpu_hourly_stats.workload_type IS 'Type of Kubernetes workload: Job, Deployment, StatefulSet, DaemonSet, etc.';
COMMENT ON COLUMN workload_gpu_hourly_stats.allocated_gpu_count IS 'Average number of GPUs actually allocated to this workload during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.requested_gpu_count IS 'Average number of GPUs requested by this workload during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.sample_count IS 'Number of samples collected from Prometheus during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.avg_replica_count IS 'Average number of pod replicas running during this hour';
COMMENT ON COLUMN workload_gpu_hourly_stats.owner_uid IS 'UID of the workload owner from metadata';
COMMENT ON COLUMN workload_gpu_hourly_stats.labels IS 'Workload labels stored in JSONB format for flexible querying';
COMMENT ON COLUMN workload_gpu_hourly_stats.annotations IS 'Workload annotations stored in JSONB format for flexible querying';
COMMENT ON COLUMN workload_gpu_hourly_stats.avg_gpu_memory_used IS 'Average GPU memory used in GB';
COMMENT ON COLUMN workload_gpu_hourly_stats.workload_status IS 'Status of the workload during this hour: Running, Completed, Failed, Pending, etc.';
