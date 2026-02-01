-- Tools Repository Tables Migration
-- This migration creates tables for the Tools Repository service

-- Tools registry table
CREATE TABLE IF NOT EXISTS tools (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL UNIQUE,
    version VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(100),
    domain VARCHAR(100),
    tags JSONB DEFAULT '[]'::jsonb,
    
    -- Provider information
    provider_type VARCHAR(50) NOT NULL,
    provider_endpoint VARCHAR(500) NOT NULL,
    provider_timeout_ms INT DEFAULT 30000,
    
    -- Schema information
    input_schema JSONB DEFAULT '{}'::jsonb,
    output_schema JSONB DEFAULT '{}'::jsonb,
    
    -- Annotations (hints about tool behavior)
    read_only_hint BOOLEAN DEFAULT true,
    destructive_hint BOOLEAN DEFAULT false,
    idempotent_hint BOOLEAN DEFAULT true,
    open_world_hint BOOLEAN DEFAULT false,
    
    -- Access control
    access_scope VARCHAR(50) DEFAULT 'platform',
    access_roles JSONB DEFAULT '[]'::jsonb,
    access_teams JSONB DEFAULT '[]'::jsonb,
    access_users JSONB DEFAULT '[]'::jsonb,
    
    -- Examples
    examples JSONB DEFAULT '[]'::jsonb,
    
    -- Owner information
    owner_type VARCHAR(50),
    owner_id VARCHAR(200),
    
    -- Status
    status VARCHAR(20) DEFAULT 'active',
    
    -- Timestamps
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);
CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_domain ON tools(domain);
CREATE INDEX IF NOT EXISTS idx_tools_status ON tools(status);
CREATE INDEX IF NOT EXISTS idx_tools_access_scope ON tools(access_scope);
CREATE INDEX IF NOT EXISTS idx_tools_registered_at ON tools(registered_at);

-- Tool versions history
CREATE TABLE IF NOT EXISTS tool_versions (
    id BIGSERIAL PRIMARY KEY,
    tool_id BIGINT NOT NULL,
    version VARCHAR(50) NOT NULL,
    input_schema JSONB,
    output_schema JSONB,
    change_log TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(200)
);

CREATE INDEX IF NOT EXISTS idx_tool_versions_tool_id ON tool_versions(tool_id);
CREATE INDEX IF NOT EXISTS idx_tool_versions_version ON tool_versions(version);
CREATE INDEX IF NOT EXISTS idx_tool_versions_created_at ON tool_versions(created_at);

-- Tool invocations log
CREATE TABLE IF NOT EXISTS tool_invocations (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(200) NOT NULL,
    user_id VARCHAR(200),
    input JSONB,
    output JSONB,
    error TEXT,
    duration_ms INT,
    success BOOLEAN,
    invoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_tool_name ON tool_invocations(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_user_id ON tool_invocations(user_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_invoked_at ON tool_invocations(invoked_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_success ON tool_invocations(success);

-- Tool quality stats (aggregated)
CREATE TABLE IF NOT EXISTS tool_quality_stats (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(200) NOT NULL UNIQUE,
    total_invocations BIGINT DEFAULT 0,
    success_count BIGINT DEFAULT 0,
    failure_count BIGINT DEFAULT 0,
    avg_duration_ms INT,
    p50_duration_ms INT,
    p99_duration_ms INT,
    error_rate NUMERIC(5,4),
    last_invoked_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_quality_stats_tool_name ON tool_quality_stats(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_quality_stats_updated_at ON tool_quality_stats(updated_at);

-- Tool embeddings table for semantic search
-- Using 1024 dimensions for BGE-M3 model compatibility
CREATE TABLE IF NOT EXISTS tool_embeddings (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(200) NOT NULL,
    embedding_type VARCHAR(50) NOT NULL,
    embedding vector(1024),
    text_content TEXT,
    model_version VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tool_name, embedding_type)
);

CREATE INDEX IF NOT EXISTS idx_tool_embeddings_tool_name ON tool_embeddings(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_embeddings_type ON tool_embeddings(embedding_type);

-- Tool domains table
CREATE TABLE IF NOT EXISTS tool_domains (
    id BIGSERIAL PRIMARY KEY,
    domain VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    tool_names JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_domains_domain ON tool_domains(domain);
