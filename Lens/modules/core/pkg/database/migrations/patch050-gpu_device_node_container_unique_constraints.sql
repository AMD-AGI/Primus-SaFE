-- patch050: Add unique constraints and indexes for gpu_device and node_container tables
-- This patch fixes race condition duplicate inserts and improves query performance

-- 1. gpu_device: add unique constraint on (node_id, gpu_id)
-- First, clean up existing duplicates (keep the one with the latest updated_at)
DELETE FROM gpu_device
WHERE id NOT IN (
    SELECT DISTINCT ON (node_id, gpu_id) id
    FROM gpu_device
    ORDER BY node_id, gpu_id, updated_at DESC
);

-- Add unique constraint
CREATE UNIQUE INDEX IF NOT EXISTS idx_gpu_device_node_gpu_unique
    ON gpu_device (node_id, gpu_id);

-- Add index on node_id for frequent lookups (SELECT * FROM gpu_device WHERE node_id = $1)
CREATE INDEX IF NOT EXISTS idx_gpu_device_node_id
    ON gpu_device (node_id);

-- 2. node_container: add unique constraint on (container_id)
-- First, clean up existing duplicates (keep the one with the latest updated_at)
DELETE FROM node_container
WHERE id NOT IN (
    SELECT DISTINCT ON (container_id) id
    FROM node_container
    ORDER BY container_id, updated_at DESC
);

-- Add unique constraint on container_id
CREATE UNIQUE INDEX IF NOT EXISTS idx_node_container_container_id_unique
    ON node_container (container_id);

-- Add index on pod_uid for frequent lookups (SELECT * FROM node_container WHERE pod_uid = $1)
CREATE INDEX IF NOT EXISTS idx_node_container_pod_uid
    ON node_container (pod_uid);
