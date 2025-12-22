-- Node and Namespace mapping table
-- This table stores the relationship between nodes and namespaces

-- Create node_namespace_mapping table
CREATE TABLE IF NOT EXISTS node_namespace_mapping (
    id SERIAL PRIMARY KEY,
    node_id INTEGER NOT NULL,
    node_name VARCHAR(255) NOT NULL,
    namespace_id BIGINT NOT NULL,
    namespace_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_node_namespace_mapping_node FOREIGN KEY (node_id) REFERENCES node(id) ON DELETE CASCADE,
    CONSTRAINT fk_node_namespace_mapping_namespace FOREIGN KEY (namespace_id) REFERENCES namespace_info(id) ON DELETE CASCADE
);

-- Add deleted_at column for soft delete support
ALTER TABLE node_namespace_mapping 
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

-- Create index for querying by node
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_node_id 
ON node_namespace_mapping(node_id);

-- Create index for querying by namespace
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_namespace_id 
ON node_namespace_mapping(namespace_id);

-- Create index for querying by node_name (useful when node_id is not available)
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_node_name 
ON node_namespace_mapping(node_name);

-- Create index for querying by namespace_name (useful when namespace_id is not available)
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_namespace_name 
ON node_namespace_mapping(namespace_name);

-- Create index for soft delete queries
CREATE INDEX IF NOT EXISTS idx_node_namespace_mapping_deleted_at 
ON node_namespace_mapping(deleted_at);

-- Add comments for documentation
COMMENT ON TABLE node_namespace_mapping IS 'Mapping table that stores relationships between nodes and namespaces';
COMMENT ON COLUMN node_namespace_mapping.id IS 'Primary key';
COMMENT ON COLUMN node_namespace_mapping.node_id IS 'Foreign key reference to node table';
COMMENT ON COLUMN node_namespace_mapping.node_name IS 'Node name for quick reference without join';
COMMENT ON COLUMN node_namespace_mapping.namespace_id IS 'Foreign key reference to namespace_info table';
COMMENT ON COLUMN node_namespace_mapping.namespace_name IS 'Namespace name for quick reference without join';
COMMENT ON COLUMN node_namespace_mapping.created_at IS 'Timestamp when the mapping was created';
COMMENT ON COLUMN node_namespace_mapping.updated_at IS 'Timestamp when the mapping was last updated';
COMMENT ON COLUMN node_namespace_mapping.deleted_at IS 'Timestamp when the mapping was soft deleted (NULL means active)';

-- ============================================================================
-- Node Namespace Mapping History Table
-- Records historical changes of node-namespace relationships for time-based queries
-- Uses record_start and record_end for time-range based queries
-- ============================================================================

CREATE TABLE IF NOT EXISTS node_namespace_mapping_history (
    id SERIAL PRIMARY KEY,
    mapping_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    node_name VARCHAR(255) NOT NULL,
    namespace_id BIGINT NOT NULL,
    namespace_name VARCHAR(255) NOT NULL,
    action VARCHAR(20) NOT NULL,  -- 'added' or 'removed'
    record_start TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    record_end TIMESTAMP DEFAULT NULL
);

-- ============================================================================
-- Migration: Change snapshot_time/recorded_at to record_start/record_end
-- Run these ALTER statements if table already exists with old schema
-- ============================================================================

-- Drop old columns if they exist
ALTER TABLE node_namespace_mapping_history 
DROP COLUMN IF EXISTS snapshot_time;

ALTER TABLE node_namespace_mapping_history 
DROP COLUMN IF EXISTS recorded_at;

-- Add new columns if they don't exist
ALTER TABLE node_namespace_mapping_history 
ADD COLUMN IF NOT EXISTS record_start TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE node_namespace_mapping_history 
ADD COLUMN IF NOT EXISTS record_end TIMESTAMP DEFAULT NULL;

-- Drop old indexes if they exist
DROP INDEX IF EXISTS idx_node_namespace_history_snapshot_time;
DROP INDEX IF EXISTS idx_node_namespace_history_ns_time;
DROP INDEX IF EXISTS idx_node_namespace_history_ns_name_time;

-- Create index for record_start (find records that started before a time)
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_record_start 
ON node_namespace_mapping_history(record_start);

-- Create index for record_end (find active records where record_end is NULL)
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_record_end 
ON node_namespace_mapping_history(record_end);

-- Create index for querying history by namespace
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_namespace_id 
ON node_namespace_mapping_history(namespace_id);

-- Create index for querying history by namespace_name
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_namespace_name 
ON node_namespace_mapping_history(namespace_name);

-- Create index for querying history by node
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_node_id 
ON node_namespace_mapping_history(node_id);

-- Create composite index for efficient time-range queries per namespace
-- Query: Find nodes in namespace at time T: WHERE namespace_id = X AND record_start <= T AND (record_end IS NULL OR record_end > T)
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_ns_time_range 
ON node_namespace_mapping_history(namespace_id, record_start, record_end);

-- Create composite index for efficient time-range queries per namespace_name
CREATE INDEX IF NOT EXISTS idx_node_namespace_history_ns_name_time_range 
ON node_namespace_mapping_history(namespace_name, record_start, record_end);

-- Add comments for documentation
COMMENT ON TABLE node_namespace_mapping_history IS 'History table recording all changes to node-namespace relationships. Use record_start and record_end for time-range queries';
COMMENT ON COLUMN node_namespace_mapping_history.id IS 'Primary key';
COMMENT ON COLUMN node_namespace_mapping_history.mapping_id IS 'Reference to the original mapping record';
COMMENT ON COLUMN node_namespace_mapping_history.node_id IS 'Node ID at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.node_name IS 'Node name at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.namespace_id IS 'Namespace ID at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.namespace_name IS 'Namespace name at the time of the change';
COMMENT ON COLUMN node_namespace_mapping_history.action IS 'Type of change: added (node joined namespace) or removed (node left namespace)';
COMMENT ON COLUMN node_namespace_mapping_history.record_start IS 'Start time when this node-namespace relationship became effective';
COMMENT ON COLUMN node_namespace_mapping_history.record_end IS 'End time when this relationship ended (NULL means currently active)';
