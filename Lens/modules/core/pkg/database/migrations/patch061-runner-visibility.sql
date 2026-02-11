-- Migration: Add runner visibility fields
-- Version: 2026020501
-- Description: Add fields for EphemeralRunner visibility including runner type, pod phase, condition, and placeholder summaries

BEGIN;

-- 1. github_workflow_runs table: add runner type and pod status fields
ALTER TABLE github_workflow_runs 
    ADD COLUMN IF NOT EXISTS runner_type VARCHAR(20) DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS pod_phase VARCHAR(20),
    ADD COLUMN IF NOT EXISTS pod_condition VARCHAR(50),
    ADD COLUMN IF NOT EXISTS pod_message TEXT;

COMMENT ON COLUMN github_workflow_runs.runner_type IS 'Runner type: launcher, worker, or unknown';
COMMENT ON COLUMN github_workflow_runs.pod_phase IS 'Pod phase: Pending, Running, Succeeded, Failed, Unknown';
COMMENT ON COLUMN github_workflow_runs.pod_condition IS 'Pod condition: ImagePullBackOff, CrashLoopBackOff, ContainerCreating, Ready, etc.';
COMMENT ON COLUMN github_workflow_runs.pod_message IS 'Detailed error message from pod status';

-- 2. github_workflow_run_summaries table: make github_run_id nullable and add placeholder fields
ALTER TABLE github_workflow_run_summaries 
    ALTER COLUMN github_run_id DROP NOT NULL,
    ALTER COLUMN github_run_id SET DEFAULT 0;

ALTER TABLE github_workflow_run_summaries
    ADD COLUMN IF NOT EXISTS is_placeholder BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS active_launchers INT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS active_workers INT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_count INT DEFAULT 0;

COMMENT ON COLUMN github_workflow_run_summaries.is_placeholder IS 'True if this is a placeholder record waiting for GitHub info';
COMMENT ON COLUMN github_workflow_run_summaries.active_launchers IS 'Number of active launcher pods';
COMMENT ON COLUMN github_workflow_run_summaries.active_workers IS 'Number of active worker pods';
COMMENT ON COLUMN github_workflow_run_summaries.error_count IS 'Number of pods with errors';

-- 3. Create indexes for new fields
CREATE INDEX IF NOT EXISTS idx_runs_runner_type ON github_workflow_runs(runner_type);
CREATE INDEX IF NOT EXISTS idx_runs_pod_condition ON github_workflow_runs(pod_condition) WHERE pod_condition IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_summaries_placeholder ON github_workflow_run_summaries(is_placeholder) WHERE is_placeholder = true;
CREATE INDEX IF NOT EXISTS idx_summaries_runner_set_placeholder ON github_workflow_run_summaries(primary_runner_set_id, is_placeholder) WHERE is_placeholder = true;

COMMIT;
