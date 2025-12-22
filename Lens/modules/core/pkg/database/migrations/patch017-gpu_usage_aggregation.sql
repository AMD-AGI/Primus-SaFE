-- GPU Usage Aggregation Table Design
-- For storing hourly aggregated GPU allocation rate and utilization data

-- 1. Cluster-level hourly aggregation table
CREATE TABLE IF NOT EXISTS cluster_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    
    -- Time dimension (aggregated by hour)
    stat_hour TIMESTAMP NOT NULL,  -- Start time of the statistical hour, e.g.: 2025-11-05 14:00:00
    
    -- GPU allocation statistics
    total_gpu_capacity INT NOT NULL DEFAULT 0,           -- Total GPU capacity of the cluster for this hour (average)
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated for this hour (average)
    allocation_rate DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Allocation rate (0-100)
    
    -- GPU utilization statistics (based on actual GPU utilization metrics)
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Average utilization (0-100)
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Peak utilization
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- Minimum utilization
    p50_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 50th percentile utilization
    p95_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 95th percentile utilization
    
    -- Sampling statistics
    sample_count INT NOT NULL DEFAULT 0,                      -- Number of samples taken during this hour
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 2. Namespace-level hourly aggregation table
CREATE TABLE IF NOT EXISTS namespace_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    namespace VARCHAR(253) NOT NULL,  -- Maximum length of K8s namespace
    
    -- Time dimension
    stat_hour TIMESTAMP NOT NULL,
    
    -- GPU capacity statistics
    total_gpu_capacity INT NOT NULL DEFAULT 0,                -- Total GPU capacity quota for this namespace during this hour (average)
    
    -- GPU allocation statistics
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Number of GPUs allocated for this namespace (average)
    
    -- GPU utilization statistics
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Workload statistics
    active_workload_count INT NOT NULL DEFAULT 0,             -- Number of active workloads (average)
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 3. Label/Annotation aggregation table (grouped by custom label or annotation)
CREATE TABLE IF NOT EXISTS label_gpu_hourly_stats (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    
    -- Label/Annotation dimension
    dimension_type VARCHAR(20) NOT NULL,     -- 'label' or 'annotation'
    dimension_key VARCHAR(255) NOT NULL,     -- Key of label/annotation
    dimension_value TEXT NOT NULL,           -- Value of label/annotation
    
    -- Time dimension
    stat_hour TIMESTAMP NOT NULL,
    
    -- GPU allocation statistics
    allocated_gpu_count DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- GPU utilization statistics
    avg_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Workload statistics
    active_workload_count INT NOT NULL DEFAULT 0,
    
    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 4. Snapshot table (for accurate calculation of dynamic resources, supporting multiple dimensions)
CREATE TABLE IF NOT EXISTS gpu_allocation_snapshots (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    snapshot_time TIMESTAMP NOT NULL,
    
    -- Dimension information
    dimension_type VARCHAR(20) NOT NULL,     -- 'cluster', 'namespace', 'label', 'annotation'
    dimension_key VARCHAR(255),              -- Key when dimension_type is label/annotation, otherwise NULL
    dimension_value TEXT,                    -- Namespace name or label/annotation value, NULL when dimension_type is cluster
    
    -- GPU capacity and allocation
    total_gpu_capacity INT NOT NULL DEFAULT 0,      -- Total GPU capacity of this dimension
    allocated_gpu_count INT NOT NULL DEFAULT 0,     -- Number of GPUs allocated in this dimension
    
    -- Detailed allocation information stored in JSON format
    allocation_details JSONB NOT NULL DEFAULT '{}',  -- Detailed workload information, etc.
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index design
-- Cluster level
CREATE UNIQUE INDEX IF NOT EXISTS idx_cluster_gpu_hourly_cluster_hour 
    ON cluster_gpu_hourly_stats(cluster_name, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_cluster_gpu_hourly_hour 
    ON cluster_gpu_hourly_stats(stat_hour DESC);

-- Namespace level
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespace_gpu_hourly_cluster_ns_hour 
    ON namespace_gpu_hourly_stats(cluster_name, namespace, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_namespace_gpu_hourly_cluster_hour 
    ON namespace_gpu_hourly_stats(cluster_name, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_namespace_gpu_hourly_namespace 
    ON namespace_gpu_hourly_stats(namespace, stat_hour DESC);

-- Label/Annotation level
CREATE UNIQUE INDEX IF NOT EXISTS idx_label_gpu_hourly_cluster_type_key_value_hour 
    ON label_gpu_hourly_stats(cluster_name, dimension_type, dimension_key, dimension_value, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_label_gpu_hourly_cluster_hour 
    ON label_gpu_hourly_stats(cluster_name, stat_hour DESC);
CREATE INDEX IF NOT EXISTS idx_label_gpu_hourly_type_key 
    ON label_gpu_hourly_stats(dimension_type, dimension_key, stat_hour DESC);

-- Snapshot table indexes
CREATE INDEX IF NOT EXISTS idx_gpu_allocation_snapshots_cluster_time 
    ON gpu_allocation_snapshots(cluster_name, snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_gpu_allocation_snapshots_time 
    ON gpu_allocation_snapshots(snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_gpu_allocation_snapshots_dimension 
    ON gpu_allocation_snapshots(cluster_name, dimension_type, dimension_key, dimension_value, snapshot_time DESC);

-- Table comments
COMMENT ON TABLE cluster_gpu_hourly_stats IS 'Cluster-level GPU utilization hourly aggregation statistics table';
COMMENT ON TABLE namespace_gpu_hourly_stats IS 'Namespace-level GPU utilization hourly aggregation statistics table';
COMMENT ON TABLE label_gpu_hourly_stats IS 'Label/Annotation-level GPU utilization hourly aggregation statistics table';
COMMENT ON TABLE gpu_allocation_snapshots IS 'GPU allocation snapshot table, supporting multiple dimensions (cluster/namespace/label/annotation) for accurate tracking of dynamic resource changes';

-- Column comments
COMMENT ON COLUMN cluster_gpu_hourly_stats.stat_hour IS 'Statistical hour (rounded down to the hour), e.g. 2025-11-05 14:00:00';
COMMENT ON COLUMN cluster_gpu_hourly_stats.sample_count IS 'Number of samples collected from Prometheus during this hour';
COMMENT ON COLUMN namespace_gpu_hourly_stats.total_gpu_capacity IS 'Total GPU capacity quota for this namespace during this hour (average)';
COMMENT ON COLUMN label_gpu_hourly_stats.dimension_type IS 'Dimension type: label or annotation';
COMMENT ON COLUMN label_gpu_hourly_stats.dimension_key IS 'Key of label or annotation';
COMMENT ON COLUMN gpu_allocation_snapshots.dimension_type IS 'Dimension type: cluster/namespace/label/annotation';
COMMENT ON COLUMN gpu_allocation_snapshots.allocation_details IS 'Detailed allocation information stored in JSON format, including workload and other detailed data';

