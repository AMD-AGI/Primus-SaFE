-- Tools Tables Migration
-- Unified design for MCP + Skills tools

-- Enable pgvector extension for semantic search
CREATE EXTENSION IF NOT EXISTS vector;

-- Unified tools table (skills + mcp)
CREATE TABLE IF NOT EXISTS tools (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(50) NOT NULL,                    -- 'skill', 'mcp'
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    description TEXT NOT NULL,
    tags JSONB DEFAULT '[]'::jsonb,
    icon_url TEXT,
    author VARCHAR(255),
    
    -- Unified config field
    -- skill: {"s3_key": "skills/xxx/", "is_prefix": true}
    -- mcp: {"command": "npx", "args": [...], "env": {...}}
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    
    -- Source tracking (for skill only)
    skill_source VARCHAR(50) DEFAULT 'upload',    -- upload, github, zip
    skill_source_url TEXT,
    
    -- Access control
    owner_user_id VARCHAR(255),
    is_public BOOLEAN DEFAULT true,               -- true: public, false: private
    status VARCHAR(50) DEFAULT 'active',
    
    -- Statistics
    run_count INTEGER DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    
    -- Semantic search (1024 for BGE-M3)
    embedding vector(1024),
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(type, name)
);

CREATE INDEX IF NOT EXISTS idx_tools_type ON tools(type);
CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);
CREATE INDEX IF NOT EXISTS idx_tools_owner ON tools(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_tools_status ON tools(status);
