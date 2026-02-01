-- Skills Repository Tables Migration
-- This migration creates tables for the Skills Repository service

-- Enable pgvector extension for semantic search
CREATE EXTENSION IF NOT EXISTS vector;

-- Skills registry table
CREATE TABLE IF NOT EXISTS skills (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL UNIQUE,
    description TEXT NOT NULL,
    category VARCHAR(100),
    version VARCHAR(50),
    source VARCHAR(50) NOT NULL,
    license VARCHAR(200),
    content TEXT,
    file_path VARCHAR(500),
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_skills_name ON skills(name);
CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category);
CREATE INDEX IF NOT EXISTS idx_skills_source ON skills(source);
CREATE INDEX IF NOT EXISTS idx_skills_created_at ON skills(created_at);

-- Skill versions history
CREATE TABLE IF NOT EXISTS skill_versions (
    id BIGSERIAL PRIMARY KEY,
    skill_id BIGINT NOT NULL,
    version VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    change_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(200)
);

CREATE INDEX IF NOT EXISTS idx_skill_versions_skill_id ON skill_versions(skill_id);
CREATE INDEX IF NOT EXISTS idx_skill_versions_version ON skill_versions(version);
CREATE INDEX IF NOT EXISTS idx_skill_versions_created_at ON skill_versions(created_at);

-- Skill embeddings table for semantic search
-- Using 1024 dimensions for BGE-M3 model compatibility
CREATE TABLE IF NOT EXISTS skill_embeddings (
    id BIGSERIAL PRIMARY KEY,
    skill_name VARCHAR(200) NOT NULL,
    embedding_type VARCHAR(50) NOT NULL,
    embedding vector(1024),
    text_content TEXT,
    model_version VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(skill_name, embedding_type)
);

CREATE INDEX IF NOT EXISTS idx_skill_embeddings_skill_name ON skill_embeddings(skill_name);
CREATE INDEX IF NOT EXISTS idx_skill_embeddings_type ON skill_embeddings(embedding_type);

-- Skill executions log
CREATE TABLE IF NOT EXISTS skill_executions (
    id BIGSERIAL PRIMARY KEY,
    skill_name VARCHAR(200) NOT NULL,
    user_id VARCHAR(200),
    session_id VARCHAR(200),
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,
    status VARCHAR(20),
    variables JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_skill_executions_skill_name ON skill_executions(skill_name);
CREATE INDEX IF NOT EXISTS idx_skill_executions_user_id ON skill_executions(user_id);
CREATE INDEX IF NOT EXISTS idx_skill_executions_created_at ON skill_executions(created_at);
CREATE INDEX IF NOT EXISTS idx_skill_executions_status ON skill_executions(status);

-- Skill feedback table
CREATE TABLE IF NOT EXISTS skill_feedback (
    id BIGSERIAL PRIMARY KEY,
    execution_id BIGINT,
    skill_name VARCHAR(200) NOT NULL,
    user_id VARCHAR(200),
    rating INT CHECK (rating >= 1 AND rating <= 5),
    comment TEXT,
    tags JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_skill_feedback_execution_id ON skill_feedback(execution_id);
CREATE INDEX IF NOT EXISTS idx_skill_feedback_skill_name ON skill_feedback(skill_name);
CREATE INDEX IF NOT EXISTS idx_skill_feedback_rating ON skill_feedback(rating);
CREATE INDEX IF NOT EXISTS idx_skill_feedback_created_at ON skill_feedback(created_at);

-- Skill quality stats (aggregated)
CREATE TABLE IF NOT EXISTS skill_quality_stats (
    id BIGSERIAL PRIMARY KEY,
    skill_name VARCHAR(200) NOT NULL UNIQUE,
    total_executions BIGINT DEFAULT 0,
    success_count BIGINT DEFAULT 0,
    failure_count BIGINT DEFAULT 0,
    avg_duration_ms INT,
    avg_rating NUMERIC(3,2),
    last_used_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_skill_quality_stats_skill_name ON skill_quality_stats(skill_name);
CREATE INDEX IF NOT EXISTS idx_skill_quality_stats_updated_at ON skill_quality_stats(updated_at);
