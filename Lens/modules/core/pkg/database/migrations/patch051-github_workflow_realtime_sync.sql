-- Migration: Add real-time sync fields to github_workflow_runs
-- Date: 2026-01-26
-- Description: Add fields for real-time workflow state synchronization from GitHub

-- Add workflow status field (GitHub execution status)
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(50);
-- Add workflow conclusion field (GitHub execution result)
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS workflow_conclusion VARCHAR(50);
-- Add collection status field (internal collection status)
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS collection_status VARCHAR(50);

-- Add fields for real-time sync progress tracking
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS current_job_name VARCHAR(255);
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS current_step_name VARCHAR(255);
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS progress_percent INT DEFAULT 0;
ALTER TABLE github_workflow_runs ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMP;

-- Add index for finding runs that need sync
CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_sync_status 
ON github_workflow_runs (workflow_status, last_synced_at) 
WHERE workflow_status IN ('queued', 'in_progress');

-- Add index for collection status queries
CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_collection_status
ON github_workflow_runs (collection_status)
WHERE collection_status IN ('pending', 'collecting');

-- Add comments
COMMENT ON COLUMN github_workflow_runs.workflow_status IS 'GitHub workflow execution status: queued, in_progress, completed, waiting, pending, requested';
COMMENT ON COLUMN github_workflow_runs.workflow_conclusion IS 'GitHub workflow conclusion: success, failure, cancelled, skipped, neutral, timed_out, action_required';
COMMENT ON COLUMN github_workflow_runs.collection_status IS 'Internal metrics collection status: pending, collecting, completed, failed, skipped';
COMMENT ON COLUMN github_workflow_runs.current_job_name IS 'Currently running job name from GitHub';
COMMENT ON COLUMN github_workflow_runs.current_step_name IS 'Currently running step name from GitHub';
COMMENT ON COLUMN github_workflow_runs.progress_percent IS 'Overall workflow progress percentage (0-100)';
COMMENT ON COLUMN github_workflow_runs.last_synced_at IS 'Last time state was synced from GitHub API';
