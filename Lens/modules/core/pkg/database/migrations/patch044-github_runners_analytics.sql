-- Migration: github_runners_analytics
-- Description: Tables for GitHub Runners management and workflow analytics
-- Database: PostgreSQL
--
-- This migration creates tables for:
-- 1. github_runner_sets - Auto-discovered AutoScalingRunnerSets in the cluster
-- 2. github_workflow_commits - Commit details fetched from GitHub API
-- 3. github_workflow_run_details - Workflow run details from GitHub API

-- ============================================================================
-- Table: github_runner_sets
-- Auto-discovered AutoScalingRunnerSets in the cluster
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_runner_sets (
    id BIGSERIAL PRIMARY KEY,
    
    -- K8s Resource Info
    uid VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    
    -- GitHub Configuration (extracted from ARS spec)
    github_config_url TEXT,
    github_config_secret VARCHAR(255),
    runner_group VARCHAR(255),
    
    -- Repository Info (extracted from labels/annotations)
    github_owner VARCHAR(255),
    github_repo VARCHAR(255),
    
    -- Runner Pool Config
    min_runners INT NOT NULL DEFAULT 0,
    max_runners INT NOT NULL DEFAULT 0,
    
    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    current_runners INT NOT NULL DEFAULT 0,
    desired_runners INT NOT NULL DEFAULT 0,
    
    -- Timestamps
    last_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE github_runner_sets IS 'Auto-discovered AutoScalingRunnerSets in the cluster';
COMMENT ON COLUMN github_runner_sets.uid IS 'Kubernetes UID of the AutoScalingRunnerSet';
COMMENT ON COLUMN github_runner_sets.github_config_url IS 'GitHub config URL from ARS spec';
COMMENT ON COLUMN github_runner_sets.github_config_secret IS 'Name of the secret containing GitHub token';
COMMENT ON COLUMN github_runner_sets.status IS 'Status: active, inactive, deleted';

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_runner_sets_uid
    ON github_runner_sets(uid);

CREATE INDEX IF NOT EXISTS idx_github_runner_sets_namespace
    ON github_runner_sets(namespace);

CREATE INDEX IF NOT EXISTS idx_github_runner_sets_status
    ON github_runner_sets(status) WHERE status = 'active';


-- ============================================================================
-- Table: github_workflow_commits
-- Commit details fetched from GitHub API
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_commits (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL,
    
    -- Commit Info
    sha VARCHAR(64) NOT NULL,
    message TEXT,
    
    -- Author Info
    author_name VARCHAR(255),
    author_email VARCHAR(255),
    author_date TIMESTAMPTZ,
    
    -- Committer Info
    committer_name VARCHAR(255),
    committer_email VARCHAR(255),
    committer_date TIMESTAMPTZ,
    
    -- Change Stats
    additions INT NOT NULL DEFAULT 0,
    deletions INT NOT NULL DEFAULT 0,
    files_changed INT NOT NULL DEFAULT 0,
    
    -- Parent commits
    parent_shas JSONB NOT NULL DEFAULT '[]'::jsonb,
    
    -- Changed files (summary, not full diff)
    files JSONB,
    
    -- Raw commit URL
    html_url TEXT,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE github_workflow_commits IS 'Commit details fetched from GitHub API';
COMMENT ON COLUMN github_workflow_commits.run_id IS 'Reference to github_workflow_runs.id';
COMMENT ON COLUMN github_workflow_commits.sha IS 'Git commit SHA';
COMMENT ON COLUMN github_workflow_commits.files IS 'JSON array of changed files with status, additions, deletions';
COMMENT ON COLUMN github_workflow_commits.parent_shas IS 'JSON array of parent commit SHAs';

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_commits_run
    ON github_workflow_commits(run_id);

CREATE INDEX IF NOT EXISTS idx_github_workflow_commits_sha
    ON github_workflow_commits(sha);

CREATE INDEX IF NOT EXISTS idx_github_workflow_commits_author_date
    ON github_workflow_commits(author_date DESC);


-- ============================================================================
-- Table: github_workflow_run_details
-- Workflow run details from GitHub API
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_workflow_run_details (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL,
    
    -- GitHub Workflow Run Info
    github_run_id BIGINT NOT NULL,
    github_run_number INT,
    github_run_attempt INT DEFAULT 1,
    
    -- Workflow Info
    workflow_id BIGINT,
    workflow_name VARCHAR(255),
    workflow_path VARCHAR(512),
    
    -- Status
    status VARCHAR(50),
    conclusion VARCHAR(50),
    
    -- URLs
    html_url TEXT,
    jobs_url TEXT,
    logs_url TEXT,
    artifacts_url TEXT,
    
    -- Timestamps from GitHub
    created_at_github TIMESTAMPTZ,
    updated_at_github TIMESTAMPTZ,
    run_started_at TIMESTAMPTZ,
    run_completed_at TIMESTAMPTZ,
    
    -- Duration (calculated)
    duration_seconds INT,
    
    -- Trigger Info
    event VARCHAR(50),
    trigger_actor VARCHAR(255),
    trigger_actor_id BIGINT,
    
    -- Head Info
    head_sha VARCHAR(64),
    head_branch VARCHAR(255),
    head_repository_full_name VARCHAR(512),
    
    -- Base Info (for PRs)
    base_sha VARCHAR(64),
    base_branch VARCHAR(255),
    
    -- Pull Request Info
    pull_request_number INT,
    pull_request_title TEXT,
    pull_request_url TEXT,
    
    -- Jobs Summary
    jobs JSONB,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE github_workflow_run_details IS 'Workflow run details from GitHub API';
COMMENT ON COLUMN github_workflow_run_details.run_id IS 'Reference to github_workflow_runs.id';
COMMENT ON COLUMN github_workflow_run_details.status IS 'GitHub status: queued, in_progress, completed';
COMMENT ON COLUMN github_workflow_run_details.conclusion IS 'GitHub conclusion: success, failure, cancelled, skipped, timed_out, action_required';
COMMENT ON COLUMN github_workflow_run_details.event IS 'Trigger event: push, pull_request, schedule, workflow_dispatch';
COMMENT ON COLUMN github_workflow_run_details.jobs IS 'JSON array of job details with id, name, status, conclusion, started_at, completed_at';

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_run_details_run
    ON github_workflow_run_details(run_id);

CREATE INDEX IF NOT EXISTS idx_github_workflow_run_details_github_run
    ON github_workflow_run_details(github_run_id);

CREATE INDEX IF NOT EXISTS idx_github_workflow_run_details_status
    ON github_workflow_run_details(status, conclusion);

CREATE INDEX IF NOT EXISTS idx_github_workflow_run_details_event
    ON github_workflow_run_details(event);

CREATE INDEX IF NOT EXISTS idx_github_workflow_run_details_created
    ON github_workflow_run_details(created_at_github DESC);


