-- Migration: Remove Run-level fields from github_workflow_runs
-- Date: 2026-02-03
-- Description: Remove Run-level fields that are now stored in run_summaries
-- Note: We keep the table name as github_workflow_runs since github_workflow_jobs already exists
-- IMPORTANT: patch058 must run before this to migrate data

-- Step 1: Remove Run-level fields (now stored in run_summaries)
-- These fields are redundant as they belong to Run level, not per-runner-pod level
ALTER TABLE github_workflow_runs 
    DROP COLUMN IF EXISTS head_sha,
    DROP COLUMN IF EXISTS head_branch,
    DROP COLUMN IF EXISTS workflow_name,
    DROP COLUMN IF EXISTS github_run_number;

-- Step 2: Update comments
COMMENT ON TABLE github_workflow_runs IS 'Workflow runner execution records, one record per runner pod';
COMMENT ON COLUMN github_workflow_runs.run_summary_id IS 'Logical reference to parent run summary (github_workflow_run_summaries.id)';
