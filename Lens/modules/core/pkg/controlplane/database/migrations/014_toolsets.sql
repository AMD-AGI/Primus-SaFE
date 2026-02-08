-- Toolsets Migration
-- Collections of tools (skills + mcp servers)

-- Toolsets table
CREATE TABLE IF NOT EXISTS toolsets (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    description TEXT NOT NULL DEFAULT '',
    tags JSONB DEFAULT '[]'::jsonb,
    icon_url TEXT,

    -- Access control
    owner_user_id VARCHAR(255),
    owner_user_name VARCHAR(255),
    is_public BOOLEAN DEFAULT true,

    -- Statistics (denormalized)
    tool_count INTEGER DEFAULT 0,

    -- Semantic search (1024 for BGE-M3)
    embedding vector(1024),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    UNIQUE(owner_user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_toolsets_owner ON toolsets(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_toolsets_deleted_at ON toolsets(deleted_at);

-- Toolset-Tools join table
CREATE TABLE IF NOT EXISTS toolset_tools (
    id BIGSERIAL PRIMARY KEY,
    toolset_id BIGINT NOT NULL REFERENCES toolsets(id) ON DELETE CASCADE,
    tool_id BIGINT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    sort_order INTEGER DEFAULT 0,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(toolset_id, tool_id)
);

CREATE INDEX IF NOT EXISTS idx_toolset_tools_toolset_id ON toolset_tools(toolset_id);
CREATE INDEX IF NOT EXISTS idx_toolset_tools_tool_id ON toolset_tools(tool_id);
