-- Migration: workload_task_state
-- Description: Simplified task state management with distributed lock support
-- Purpose: Support service restart recovery with multi-instance coordination
-- Database: PostgreSQL
-- Note: All logic (functions, triggers, views) is implemented in application layer

-- ============================================================================
-- Table: workload_task_state
-- ============================================================================

CREATE TABLE IF NOT EXISTS workload_task_state (
    id BIGSERIAL PRIMARY KEY,
    
    -- Core fields
    workload_uid VARCHAR(128) NOT NULL,
    task_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Distributed lock fields for multi-instance coordination
    lock_owner VARCHAR(128),
    lock_acquired_at TIMESTAMPTZ,
    lock_expires_at TIMESTAMPTZ,
    lock_version BIGINT NOT NULL DEFAULT 0,
    
    -- Extension field for all other data
    ext JSONB DEFAULT '{}'::jsonb,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT uq_workload_task UNIQUE (workload_uid, task_type)
);

-- ============================================================================
-- Indexes
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_workload_uid ON workload_task_state(workload_uid);
CREATE INDEX IF NOT EXISTS idx_task_type ON workload_task_state(task_type);
CREATE INDEX IF NOT EXISTS idx_status ON workload_task_state(status);
CREATE INDEX IF NOT EXISTS idx_lock_owner ON workload_task_state(lock_owner);
CREATE INDEX IF NOT EXISTS idx_lock_expires_at ON workload_task_state(lock_expires_at);
CREATE INDEX IF NOT EXISTS idx_created_at ON workload_task_state(created_at);
CREATE INDEX IF NOT EXISTS idx_updated_at ON workload_task_state(updated_at);
CREATE INDEX IF NOT EXISTS idx_ext_gin ON workload_task_state USING GIN(ext);

-- ============================================================================
-- Comments
-- ============================================================================

COMMENT ON TABLE workload_task_state IS 'Task state with distributed lock support - all logic in application layer';
COMMENT ON COLUMN workload_task_state.workload_uid IS 'Workload unique identifier';
COMMENT ON COLUMN workload_task_state.task_type IS 'Task type: detection, metadata_collection, tensorboard_stream, etc';
COMMENT ON COLUMN workload_task_state.status IS 'Task status: pending, running, completed, failed, cancelled';
COMMENT ON COLUMN workload_task_state.lock_owner IS 'Instance ID that currently owns this task';
COMMENT ON COLUMN workload_task_state.lock_acquired_at IS 'Timestamp when lock was acquired';
COMMENT ON COLUMN workload_task_state.lock_expires_at IS 'Timestamp when lock expires (for stale lock detection)';
COMMENT ON COLUMN workload_task_state.lock_version IS 'Version number for optimistic locking';
COMMENT ON COLUMN workload_task_state.ext IS 'Extension field (JSONB) for all other task data';

-- ============================================================================
-- Application layer responsibilities
-- ============================================================================
-- 1. Distributed lock operations (acquire, extend, release)
-- 2. Task state management (create, update, delete)
-- 3. Checkpoint management (store/restore from ext field)
-- 4. Stale lock cleanup (release expired locks)
-- 5. Task recovery (find recoverable tasks on startup)
-- 6. Statistics and monitoring (query tasks by status, etc)
--
-- See: Lens/modules/core/pkg/database/workload_task_facade.go
-- Docs: Lens/docs/workload-task-persistence-simplified.md

