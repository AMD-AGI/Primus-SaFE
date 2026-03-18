-- Migration: Add github_workflow_run_summaries table
-- Date: 2026-02-03
-- Description: Create Run-level summary table for GitHub workflows
-- This aggregates data from jobs to provide Run-level view

-- Create github_workflow_run_summaries table
CREATE TABLE IF NOT EXISTS github_workflow_run_summaries (
    id BIGSERIAL PRIMARY KEY,
    
    -- GitHub Run identifiers
    github_run_id BIGINT NOT NULL,
    github_run_number INT NOT NULL,
    github_run_attempt INT DEFAULT 1,
    
    -- Repository info
    owner VARCHAR(255) NOT NULL,
    repo VARCHAR(255) NOT NULL,
    
    -- Workflow info
    workflow_name VARCHAR(500),
    workflow_path VARCHAR(500),
    workflow_id BIGINT,
    
    -- Git info
    head_sha VARCHAR(64),
    head_branch VARCHAR(255),
    base_branch VARCHAR(255),
    event_name VARCHAR(100),
    
    -- Actor info
    actor VARCHAR(255),
    triggering_actor VARCHAR(255),
    
    -- Run status (from GitHub)
    status VARCHAR(50),
    conclusion VARCHAR(50),
    
    -- Timing
    run_started_at TIMESTAMP,
    run_completed_at TIMESTAMP,
    
    -- Job aggregation stats
    total_jobs INT DEFAULT 0,
    completed_jobs INT DEFAULT 0,
    successful_jobs INT DEFAULT 0,
    failed_jobs INT DEFAULT 0,
    cancelled_jobs INT DEFAULT 0,
    skipped_jobs INT DEFAULT 0,
    in_progress_jobs INT DEFAULT 0,
    queued_jobs INT DEFAULT 0,
    
    -- Progress tracking
    current_job_name VARCHAR(255),
    current_step_name VARCHAR(255),
    progress_percent INT DEFAULT 0,
    
    -- Collection aggregation (from all jobs)
    total_files_processed INT DEFAULT 0,
    total_metrics_count INT DEFAULT 0,
    collection_status VARCHAR(50),
    
    -- Runner Set association
    primary_runner_set_id BIGINT,
    
    -- Config association
    config_id BIGINT,
    
    -- Sync metadata
    last_synced_at TIMESTAMP,
    sync_error_message TEXT,
    
    -- Graph fetched flag
    graph_fetched BOOLEAN DEFAULT FALSE,
    graph_fetched_at TIMESTAMP,
    
    -- Analysis task tracking (Run level)
    code_analysis_triggered BOOLEAN DEFAULT FALSE,
    code_analysis_triggered_at TIMESTAMP,
    failure_analysis_triggered BOOLEAN DEFAULT FALSE,
    failure_analysis_triggered_at TIMESTAMP,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Unique index for one record per run attempt
CREATE UNIQUE INDEX IF NOT EXISTS uq_run_summaries_run_attempt 
    ON github_workflow_run_summaries(github_run_id, github_run_attempt);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_run_summaries_github_run_id 
    ON github_workflow_run_summaries(github_run_id);

CREATE INDEX IF NOT EXISTS idx_run_summaries_repo 
    ON github_workflow_run_summaries(owner, repo);

CREATE INDEX IF NOT EXISTS idx_run_summaries_status 
    ON github_workflow_run_summaries(status);

CREATE INDEX IF NOT EXISTS idx_run_summaries_created 
    ON github_workflow_run_summaries(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_run_summaries_runner_set 
    ON github_workflow_run_summaries(primary_runner_set_id);

CREATE INDEX IF NOT EXISTS idx_run_summaries_config 
    ON github_workflow_run_summaries(config_id);

CREATE INDEX IF NOT EXISTS idx_run_summaries_workflow 
    ON github_workflow_run_summaries(owner, repo, workflow_path);

-- Add comments
COMMENT ON TABLE github_workflow_run_summaries IS 'Workflow Run level aggregation, one record per run attempt';
COMMENT ON COLUMN github_workflow_run_summaries.github_run_id IS 'GitHub workflow run ID';
COMMENT ON COLUMN github_workflow_run_summaries.github_run_attempt IS 'Run attempt number (for re-runs)';
COMMENT ON COLUMN github_workflow_run_summaries.workflow_path IS 'Path to workflow file, e.g., .github/workflows/ci.yml';
COMMENT ON COLUMN github_workflow_run_summaries.event_name IS 'Event that triggered the run: push, pull_request, schedule, etc.';
COMMENT ON COLUMN github_workflow_run_summaries.status IS 'GitHub run status: queued, in_progress, completed, waiting, requested';
COMMENT ON COLUMN github_workflow_run_summaries.conclusion IS 'GitHub run conclusion: success, failure, cancelled, skipped, timed_out, action_required, neutral';
COMMENT ON COLUMN github_workflow_run_summaries.collection_status IS 'Metrics collection status: pending, partial, completed, failed';
COMMENT ON COLUMN github_workflow_run_summaries.graph_fetched IS 'Whether workflow job graph has been fetched from GitHub';
COMMENT ON COLUMN github_workflow_run_summaries.code_analysis_triggered IS 'Whether code analysis has been triggered for this run';
COMMENT ON COLUMN github_workflow_run_summaries.failure_analysis_triggered IS 'Whether failure analysis has been triggered for this run';
