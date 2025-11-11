-- Cluster Overview Cache Database Schema
-- This file defines the database schema for caching cluster overview data
-- to improve API performance for large clusters

-- Table: cluster_overview_cache
-- Stores pre-computed cluster overview statistics
CREATE TABLE IF NOT EXISTS cluster_overview_cache (
    id SERIAL PRIMARY KEY,
    cluster_name VARCHAR(100) NOT NULL,
    
    -- Node statistics
    total_nodes INT NOT NULL DEFAULT 0,
    healthy_nodes INT NOT NULL DEFAULT 0,
    faulty_nodes INT NOT NULL DEFAULT 0,
    fully_idle_nodes INT NOT NULL DEFAULT 0,
    partially_idle_nodes INT NOT NULL DEFAULT 0,
    busy_nodes INT NOT NULL DEFAULT 0,
    
    -- GPU statistics
    allocation_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    utilization DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Storage statistics
    storage_total_space DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_used_space DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_usage_percentage DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_total_inodes DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_used_inodes DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_inodes_usage_percentage DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_read_bandwidth DOUBLE PRECISION NOT NULL DEFAULT 0,
    storage_write_bandwidth DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- RDMA statistics
    rdma_total_tx DOUBLE PRECISION NOT NULL DEFAULT 0,
    rdma_total_rx DOUBLE PRECISION NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for cluster_overview_cache
CREATE UNIQUE INDEX IF NOT EXISTS idx_cluster_overview_cache_cluster_name ON cluster_overview_cache(cluster_name);
CREATE INDEX IF NOT EXISTS idx_cluster_overview_cache_updated_at ON cluster_overview_cache(updated_at DESC);

-- Comment on table
COMMENT ON TABLE cluster_overview_cache IS 'Cached cluster overview statistics to improve API performance';
COMMENT ON COLUMN cluster_overview_cache.cluster_name IS 'Name of the cluster';
COMMENT ON COLUMN cluster_overview_cache.total_nodes IS 'Total number of GPU nodes';
COMMENT ON COLUMN cluster_overview_cache.allocation_rate IS 'GPU allocation rate (0-100)';
COMMENT ON COLUMN cluster_overview_cache.utilization IS 'GPU utilization rate (0-100)';

