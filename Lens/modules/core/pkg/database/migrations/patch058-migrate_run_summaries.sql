-- Migration: Migrate data to github_workflow_run_summaries
-- Date: 2026-02-03
-- Description: Populate run_summaries from existing job data
-- IMPORTANT: This must run BEFORE patch057 which removes columns from jobs table

-- Step 1: Populate run_summaries from existing job data
-- Note: head_sha, head_branch, workflow_name are migrated from jobs before they're removed
INSERT INTO github_workflow_run_summaries (
    github_run_id,
    github_run_number,
    github_run_attempt,
    owner,
    repo,
    workflow_name,
    head_sha,
    head_branch,
    status,
    conclusion,
    run_started_at,
    run_completed_at,
    total_jobs,
    completed_jobs,
    successful_jobs,
    failed_jobs,
    total_files_processed,
    total_metrics_count,
    collection_status,
    primary_runner_set_id,
    config_id,
    created_at,
    updated_at
)
SELECT 
    j.github_run_id,
    COALESCE(j.github_run_number, 0),
    1 as github_run_attempt,
    rs.github_owner as owner,
    rs.github_repo as repo,
    (array_agg(j.workflow_name ORDER BY j.id) FILTER (WHERE j.workflow_name IS NOT NULL))[1],
    (array_agg(j.head_sha ORDER BY j.id) FILTER (WHERE j.head_sha IS NOT NULL))[1],
    (array_agg(j.head_branch ORDER BY j.id) FILTER (WHERE j.head_branch IS NOT NULL))[1],
    CASE 
        WHEN COUNT(*) FILTER (WHERE j.workflow_status = 'in_progress') > 0 THEN 'in_progress'
        WHEN COUNT(*) FILTER (WHERE j.workflow_status = 'queued') > 0 THEN 'queued'
        ELSE 'completed'
    END as status,
    CASE 
        WHEN COUNT(*) FILTER (WHERE j.workflow_conclusion = 'failure') > 0 THEN 'failure'
        WHEN COUNT(*) FILTER (WHERE j.workflow_conclusion = 'cancelled') > 0 THEN 'cancelled'
        WHEN COUNT(*) = COUNT(*) FILTER (WHERE j.workflow_conclusion = 'success') THEN 'success'
        ELSE NULL
    END as conclusion,
    MIN(j.workload_started_at) as run_started_at,
    MAX(j.workload_completed_at) as run_completed_at,
    COUNT(*) as total_jobs,
    COUNT(*) FILTER (WHERE j.workflow_status = 'completed') as completed_jobs,
    COUNT(*) FILTER (WHERE j.workflow_conclusion = 'success') as successful_jobs,
    COUNT(*) FILTER (WHERE j.workflow_conclusion = 'failure') as failed_jobs,
    COALESCE(SUM(j.files_processed), 0)::INT as total_files_processed,
    COALESCE(SUM(j.metrics_count), 0)::INT as total_metrics_count,
    CASE 
        WHEN COUNT(*) FILTER (WHERE j.collection_status = 'failed') > 0 THEN 'failed'
        WHEN COUNT(*) FILTER (WHERE j.collection_status = 'pending' OR j.collection_status = 'collecting') > 0 THEN 'partial'
        WHEN COUNT(*) = COUNT(*) FILTER (WHERE j.collection_status = 'completed' OR j.collection_status = 'skipped') THEN 'completed'
        ELSE 'pending'
    END as collection_status,
    (array_agg(j.runner_set_id ORDER BY j.id))[1] as primary_runner_set_id,
    (array_agg(j.config_id ORDER BY j.id) FILTER (WHERE j.config_id IS NOT NULL))[1] as config_id,
    MIN(j.created_at) as created_at,
    MAX(j.updated_at) as updated_at
FROM github_workflow_runs j
LEFT JOIN github_runner_sets rs ON j.runner_set_id = rs.id
WHERE j.github_run_id IS NOT NULL
GROUP BY j.github_run_id, rs.github_owner, rs.github_repo
ON CONFLICT DO NOTHING;

-- Step 2: Add run_summary_id column to jobs table (before rename)
ALTER TABLE github_workflow_runs 
    ADD COLUMN IF NOT EXISTS run_summary_id BIGINT;

-- Step 3: Update job records with run_summary_id
UPDATE github_workflow_runs j
SET run_summary_id = rs.id
FROM github_workflow_run_summaries rs
WHERE j.github_run_id = rs.github_run_id
  AND j.run_summary_id IS NULL;

-- Create index on new column
CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_run_summary_id 
    ON github_workflow_runs(run_summary_id);
