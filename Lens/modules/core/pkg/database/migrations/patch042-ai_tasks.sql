-- Migration: ai_tasks
-- Description: Task queue for async AI agent invocations
-- Database: PostgreSQL
--
-- This table implements a PostgreSQL-based task queue for:
-- 1. Async AI agent invocations
-- 2. Task status tracking and retry handling
-- 3. Result storage and retrieval

-- ============================================================================
-- Table: ai_tasks
-- Task queue for async AI agent invocations
-- ============================================================================

CREATE TABLE IF NOT EXISTS ai_tasks (
    id VARCHAR(64) PRIMARY KEY,
    topic VARCHAR(128) NOT NULL,
    
    -- Task status
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    -- Status values:
    -- 'pending': waiting to be processed
    -- 'processing': currently being processed by an agent
    -- 'completed': successfully completed
    -- 'failed': failed after all retries
    -- 'cancelled': cancelled by user

    priority INT DEFAULT 0,
    -- Higher priority tasks are processed first

    -- Payloads
    input_payload JSONB NOT NULL,
    output_payload JSONB,
    
    -- Error information
    error_message VARCHAR(1024),
    error_code INT,
    
    -- Retry tracking
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,
    
    -- Agent assignment
    agent_id VARCHAR(128),
    -- The agent currently processing or that processed this task

    -- Request context (cluster, tenant, trace info)
    context JSONB DEFAULT '{}'::jsonb,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    timeout_at TIMESTAMPTZ NOT NULL
);

-- Add comments
COMMENT ON TABLE ai_tasks IS 'Task queue for async AI agent invocations';
COMMENT ON COLUMN ai_tasks.id IS 'Unique task identifier (UUID)';
COMMENT ON COLUMN ai_tasks.topic IS 'Topic for routing to the appropriate agent';
COMMENT ON COLUMN ai_tasks.status IS 'Task status: pending, processing, completed, failed, cancelled';
COMMENT ON COLUMN ai_tasks.priority IS 'Task priority (higher = more urgent)';
COMMENT ON COLUMN ai_tasks.input_payload IS 'Input payload for the agent';
COMMENT ON COLUMN ai_tasks.output_payload IS 'Output payload from the agent';
COMMENT ON COLUMN ai_tasks.agent_id IS 'Agent that is processing or processed this task';
COMMENT ON COLUMN ai_tasks.timeout_at IS 'When the task should be considered timed out if still processing';

-- ============================================================================
-- Indexes
-- ============================================================================

-- Primary query pattern: find pending tasks by priority and creation time
CREATE INDEX IF NOT EXISTS idx_ai_tasks_pending_priority 
    ON ai_tasks(status, priority DESC, created_at ASC)
    WHERE status = 'pending';

-- Filter by topic and status
CREATE INDEX IF NOT EXISTS idx_ai_tasks_topic_status 
    ON ai_tasks(topic, status);

-- Find timed-out processing tasks
CREATE INDEX IF NOT EXISTS idx_ai_tasks_timeout 
    ON ai_tasks(timeout_at) 
    WHERE status = 'processing';

-- Cleanup: find old completed tasks
CREATE INDEX IF NOT EXISTS idx_ai_tasks_completed 
    ON ai_tasks(completed_at) 
    WHERE status IN ('completed', 'failed', 'cancelled');

-- Created at for general queries
CREATE INDEX IF NOT EXISTS idx_ai_tasks_created_at 
    ON ai_tasks(created_at DESC);

-- Agent assignment tracking
CREATE INDEX IF NOT EXISTS idx_ai_tasks_agent 
    ON ai_tasks(agent_id) 
    WHERE agent_id IS NOT NULL;

