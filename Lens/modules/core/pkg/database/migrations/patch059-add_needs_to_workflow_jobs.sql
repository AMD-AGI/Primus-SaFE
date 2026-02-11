-- Migration: Add needs column to github_workflow_jobs for DAG dependencies
-- This column stores JSON array of job names that this job depends on

-- Add needs column to github_workflow_jobs
ALTER TABLE github_workflow_jobs ADD COLUMN IF NOT EXISTS needs TEXT;

-- Add comment
COMMENT ON COLUMN github_workflow_jobs.needs IS 'JSON array of job names this job depends on (for DAG visualization)';
