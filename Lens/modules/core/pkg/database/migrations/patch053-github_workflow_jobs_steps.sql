-- Migration: Add github_workflow_jobs and github_workflow_steps tables
-- Date: 2026-01-27
-- Description: Create tables for storing GitHub workflow job and step details

-- Create github_workflow_jobs table
CREATE TABLE IF NOT EXISTS github_workflow_jobs (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL,
    github_job_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50),
    conclusion VARCHAR(50),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_seconds INT DEFAULT 0,
    runner_id BIGINT DEFAULT 0,
    runner_name VARCHAR(255),
    runner_group_name VARCHAR(255),
    steps_count INT DEFAULT 0,
    steps_completed INT DEFAULT 0,
    steps_failed INT DEFAULT 0,
    html_url TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create unique constraint on run_id + github_job_id
CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_jobs_run_github_job 
ON github_workflow_jobs (run_id, github_job_id);

-- Create index for querying jobs by run_id
CREATE INDEX IF NOT EXISTS idx_github_workflow_jobs_run_id 
ON github_workflow_jobs (run_id);

-- Create index for querying in-progress jobs
CREATE INDEX IF NOT EXISTS idx_github_workflow_jobs_status 
ON github_workflow_jobs (status) 
WHERE status = 'in_progress';

-- Create github_workflow_steps table
CREATE TABLE IF NOT EXISTS github_workflow_steps (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    step_number INT NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50),
    conclusion VARCHAR(50),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_seconds INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create unique constraint on job_id + step_number
CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_steps_job_step 
ON github_workflow_steps (job_id, step_number);

-- Create index for querying steps by job_id
CREATE INDEX IF NOT EXISTS idx_github_workflow_steps_job_id 
ON github_workflow_steps (job_id);

-- Add comments
COMMENT ON TABLE github_workflow_jobs IS 'GitHub Actions workflow jobs synced from GitHub API';
COMMENT ON COLUMN github_workflow_jobs.run_id IS 'FK to github_workflow_runs.id';
COMMENT ON COLUMN github_workflow_jobs.github_job_id IS 'GitHub job ID from API';
COMMENT ON COLUMN github_workflow_jobs.status IS 'Job status: queued, in_progress, completed, waiting';
COMMENT ON COLUMN github_workflow_jobs.conclusion IS 'Job conclusion: success, failure, cancelled, skipped, neutral, timed_out, action_required';

COMMENT ON TABLE github_workflow_steps IS 'GitHub Actions workflow steps synced from GitHub API';
COMMENT ON COLUMN github_workflow_steps.job_id IS 'FK to github_workflow_jobs.id';
COMMENT ON COLUMN github_workflow_steps.step_number IS 'Step number within the job';
COMMENT ON COLUMN github_workflow_steps.status IS 'Step status: queued, in_progress, completed';
COMMENT ON COLUMN github_workflow_steps.conclusion IS 'Step conclusion: success, failure, cancelled, skipped';
