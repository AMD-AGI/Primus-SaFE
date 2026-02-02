-- Tools Repository Database Schema
-- Version: 1.0.0

-- Tools registry table
CREATE TABLE IF NOT EXISTS tools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    version VARCHAR(50) NOT NULL DEFAULT '1.0.0',
    description TEXT NOT NULL,
    provider_type VARCHAR(50) NOT NULL, -- 'mcp', 'http', 'a2a'
    provider_config JSONB NOT NULL,     -- Provider-specific configuration
    input_schema JSONB,                 -- JSON Schema for input
    output_schema JSONB,                -- JSON Schema for output
    category VARCHAR(100),
    tags TEXT[],
    scope VARCHAR(50) DEFAULT 'platform', -- 'platform', 'team', 'user'
    scope_id VARCHAR(200),              -- Team ID or User ID for scoped tools
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(200),
    UNIQUE(name, version, scope, scope_id)
);

CREATE INDEX idx_tools_name ON tools(name);
CREATE INDEX idx_tools_category ON tools(category);
CREATE INDEX idx_tools_provider ON tools(provider_type);
CREATE INDEX idx_tools_scope ON tools(scope, scope_id);
CREATE INDEX idx_tools_enabled ON tools(enabled);
CREATE INDEX idx_tools_tags ON tools USING GIN(tags);

-- Tool embeddings for semantic search (requires pgvector extension)
CREATE TABLE IF NOT EXISTS tool_embeddings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_id UUID NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    embedding_type VARCHAR(50) NOT NULL, -- 'description', 'combined'
    embedding vector(1536),              -- Embedding vector
    text_content TEXT,
    model_version VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(tool_id, embedding_type)
);

CREATE INDEX idx_tool_embeddings_tool ON tool_embeddings(tool_id);
CREATE INDEX idx_tool_embeddings_hnsw ON tool_embeddings 
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Tool execution logs
CREATE TABLE IF NOT EXISTS tool_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_id UUID REFERENCES tools(id),
    tool_name VARCHAR(200) NOT NULL,
    user_id VARCHAR(200),
    session_id UUID,
    input JSONB,
    output JSONB,
    status VARCHAR(20) NOT NULL, -- 'success', 'error', 'timeout'
    error_message TEXT,
    duration_ms INTEGER,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_tool_executions_tool ON tool_executions(tool_id);
CREATE INDEX idx_tool_executions_tool_name ON tool_executions(tool_name);
CREATE INDEX idx_tool_executions_user ON tool_executions(user_id);
CREATE INDEX idx_tool_executions_status ON tool_executions(status);
CREATE INDEX idx_tool_executions_time ON tool_executions(created_at);

-- Tool usage statistics (aggregated)
CREATE TABLE IF NOT EXISTS tool_stats (
    tool_name VARCHAR(200) PRIMARY KEY,
    total_executions BIGINT DEFAULT 0,
    success_count BIGINT DEFAULT 0,
    error_count BIGINT DEFAULT 0,
    avg_duration_ms INTEGER,
    p95_duration_ms INTEGER,
    last_used_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Tool access control
CREATE TABLE IF NOT EXISTS tool_access (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_id UUID NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    principal_type VARCHAR(50) NOT NULL, -- 'user', 'team', 'role'
    principal_id VARCHAR(200) NOT NULL,
    permission VARCHAR(50) NOT NULL,     -- 'execute', 'admin'
    created_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(200),
    UNIQUE(tool_id, principal_type, principal_id, permission)
);

CREATE INDEX idx_tool_access_tool ON tool_access(tool_id);
CREATE INDEX idx_tool_access_principal ON tool_access(principal_type, principal_id);
