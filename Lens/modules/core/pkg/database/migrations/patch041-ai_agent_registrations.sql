-- Migration: ai_agent_registrations
-- Description: Store AI Agent registrations for the webhook system
-- Database: PostgreSQL
--
-- This table stores:
-- 1. Registered AI agents and their endpoints
-- 2. Topics each agent can handle
-- 3. Health status and check history
-- 4. Agent metadata

-- ============================================================================
-- Table: ai_agent_registrations
-- Stores registered AI agents for the webhook system
-- ============================================================================

CREATE TABLE IF NOT EXISTS ai_agent_registrations (
    name VARCHAR(128) PRIMARY KEY,
    endpoint VARCHAR(512) NOT NULL,
    topics JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Topics is a JSON array of topic patterns the agent handles
    -- e.g., ["alert.advisor.*", "scan.identify-component"]

    health_check_path VARCHAR(256) DEFAULT '/health',
    timeout_secs INT DEFAULT 60,

    -- Health status
    status VARCHAR(32) DEFAULT 'unknown',
    -- Status values:
    -- 'healthy': agent is responding normally
    -- 'unhealthy': agent is not responding or returning errors
    -- 'unknown': health has not been checked yet

    last_health_check TIMESTAMPTZ,
    failure_count INT DEFAULT 0,

    -- Extensible metadata
    metadata JSONB DEFAULT '{}'::jsonb,

    -- Timestamps
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add comments
COMMENT ON TABLE ai_agent_registrations IS 'Registered AI agents for the webhook system';
COMMENT ON COLUMN ai_agent_registrations.name IS 'Unique agent name/identifier';
COMMENT ON COLUMN ai_agent_registrations.endpoint IS 'HTTP endpoint URL for the agent';
COMMENT ON COLUMN ai_agent_registrations.topics IS 'JSON array of topic patterns this agent handles';
COMMENT ON COLUMN ai_agent_registrations.status IS 'Health status: healthy, unhealthy, unknown';
COMMENT ON COLUMN ai_agent_registrations.failure_count IS 'Consecutive health check failures';

-- ============================================================================
-- Indexes
-- ============================================================================

-- Filter by status (for health checks and routing)
CREATE INDEX IF NOT EXISTS idx_ai_agent_reg_status 
    ON ai_agent_registrations(status);

-- GIN index for topic pattern matching (JSON array contains)
CREATE INDEX IF NOT EXISTS idx_ai_agent_reg_topics 
    ON ai_agent_registrations USING GIN(topics);

-- Partial index for healthy agents
CREATE INDEX IF NOT EXISTS idx_ai_agent_reg_healthy 
    ON ai_agent_registrations(name) 
    WHERE status = 'healthy';

