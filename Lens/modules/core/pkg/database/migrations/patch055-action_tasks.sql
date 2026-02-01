-- Migration: action_tasks
-- Description: Task queue for cross-cluster action execution
-- Database: PostgreSQL
--
-- This table implements a task queue for:
-- 1. Cross-cluster action execution (e.g., pyspy profiling)
-- 2. Async task status tracking
-- 3. Result storage and retrieval

-- ============================================================================
-- Table: action_tasks
-- Task queue for cross-cluster action execution
-- ============================================================================

CREATE TABLE IF NOT EXISTS action_tasks (
    id BIGSERIAL PRIMARY KEY,
    cluster_name VARCHAR(128) NOT NULL,
    action_type VARCHAR(64) NOT NULL,
    target_type VARCHAR(64),
    target_id VARCHAR(256),
    target_node VARCHAR(256),
    parameters JSONB DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    timeout_seconds INT NOT NULL DEFAULT 30,
    created_by VARCHAR(256)
);

-- Add comments
COMMENT ON TABLE action_tasks IS 'Task queue for cross-cluster action execution';
COMMENT ON COLUMN action_tasks.id IS 'Unique task identifier (auto-increment)';
COMMENT ON COLUMN action_tasks.cluster_name IS 'Target cluster for task execution';
COMMENT ON COLUMN action_tasks.action_type IS 'Type of action to execute (e.g., pyspy_profile)';
COMMENT ON COLUMN action_tasks.target_type IS 'Type of target (e.g., pod, node)';
COMMENT ON COLUMN action_tasks.target_id IS 'Target identifier (e.g., pod name)';
COMMENT ON COLUMN action_tasks.target_node IS 'Node where the target is running';
COMMENT ON COLUMN action_tasks.parameters IS 'JSON parameters for the action';
COMMENT ON COLUMN action_tasks.status IS 'Task status: pending, running, completed, failed';
COMMENT ON COLUMN action_tasks.result IS 'JSON result from task execution';
COMMENT ON COLUMN action_tasks.error_message IS 'Error message if task failed';
COMMENT ON COLUMN action_tasks.timeout_seconds IS 'Task timeout in seconds';
COMMENT ON COLUMN action_tasks.created_by IS 'User who created the task';

-- ============================================================================
-- Indexes
-- ============================================================================

-- Primary query pattern: find pending tasks by cluster
CREATE INDEX IF NOT EXISTS idx_action_tasks_cluster_status 
    ON action_tasks(cluster_name, status, created_at ASC)
    WHERE status = 'pending';

-- Find tasks by status
CREATE INDEX IF NOT EXISTS idx_action_tasks_status 
    ON action_tasks(status, created_at ASC);

-- Created at for general queries and cleanup
CREATE INDEX IF NOT EXISTS idx_action_tasks_created_at 
    ON action_tasks(created_at DESC);

-- Find running tasks for timeout checking
CREATE INDEX IF NOT EXISTS idx_action_tasks_running 
    ON action_tasks(started_at) 
    WHERE status = 'running';

