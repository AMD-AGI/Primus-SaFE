-- Migration: Add github_workflow_job_logs table
-- Date: 2026-01-27
-- Description: Create table for storing GitHub workflow job logs locally
-- This ensures logs are preserved even after GitHub's 90-day retention period

-- Create github_workflow_job_logs table
CREATE TABLE IF NOT EXISTS github_workflow_job_logs (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL,
    github_job_id BIGINT NOT NULL,
    job_name VARCHAR(255) NOT NULL,
    logs TEXT,
    logs_size_bytes INT DEFAULT 0,
    fetched_at TIMESTAMP,
    fetch_status VARCHAR(50) DEFAULT 'pending',
    fetch_error TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create unique constraint on run_id + github_job_id
CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_job_logs_run_github_job 
ON github_workflow_job_logs (run_id, github_job_id);

-- Create index for querying logs by run_id
CREATE INDEX IF NOT EXISTS idx_github_workflow_job_logs_run_id 
ON github_workflow_job_logs (run_id);

-- Create index for querying pending logs
CREATE INDEX IF NOT EXISTS idx_github_workflow_job_logs_fetch_status 
ON github_workflow_job_logs (fetch_status) 
WHERE fetch_status = 'pending';

-- Add comments
COMMENT ON TABLE github_workflow_job_logs IS 'Locally cached GitHub Actions job logs';
COMMENT ON COLUMN github_workflow_job_logs.run_id IS 'FK to github_workflow_runs.id';
COMMENT ON COLUMN github_workflow_job_logs.github_job_id IS 'GitHub job ID from API';
COMMENT ON COLUMN github_workflow_job_logs.logs IS 'Full job log content';
COMMENT ON COLUMN github_workflow_job_logs.logs_size_bytes IS 'Size of logs in bytes';
COMMENT ON COLUMN github_workflow_job_logs.fetch_status IS 'pending, fetched, failed, expired';
COMMENT ON COLUMN github_workflow_job_logs.fetch_error IS 'Error message if fetch failed';
