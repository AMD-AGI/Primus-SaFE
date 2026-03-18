-- Migration: Migrate data to github_workflow_run_summaries
-- Date: 2026-02-03
-- Description: Populate run_summaries from existing job data
-- IMPORTANT: This must run BEFORE patch057 which removes columns from jobs table
-- Note: On fresh databases these columns may not exist; wrap in DO block to skip gracefully.

DO $$ BEGIN
  -- Only run migration if the source column exists (i.e. upgrading from older schema)
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='github_workflow_runs' AND column_name='github_run_number') THEN

    INSERT INTO github_workflow_run_summaries (
        github_run_id, github_run_number, github_run_attempt,
        owner, repo, workflow_name, head_sha, head_branch,
        status, conclusion, run_started_at, run_completed_at,
        total_jobs, completed_jobs, successful_jobs, failed_jobs,
        total_files_processed, total_metrics_count, collection_status,
        primary_runner_set_id, config_id, created_at, updated_at
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
        END,
        CASE 
            WHEN COUNT(*) FILTER (WHERE j.workflow_conclusion = 'failure') > 0 THEN 'failure'
            WHEN COUNT(*) FILTER (WHERE j.workflow_conclusion = 'cancelled') > 0 THEN 'cancelled'
            WHEN COUNT(*) = COUNT(*) FILTER (WHERE j.workflow_conclusion = 'success') THEN 'success'
            ELSE NULL
        END,
        MIN(j.workload_started_at),
        MAX(j.workload_completed_at),
        COUNT(*),
        COUNT(*) FILTER (WHERE j.workflow_status = 'completed'),
        COUNT(*) FILTER (WHERE j.workflow_conclusion = 'success'),
        COUNT(*) FILTER (WHERE j.workflow_conclusion = 'failure'),
        COALESCE(SUM(j.files_processed), 0)::INT,
        COALESCE(SUM(j.metrics_count), 0)::INT,
        CASE 
            WHEN COUNT(*) FILTER (WHERE j.collection_status = 'failed') > 0 THEN 'failed'
            WHEN COUNT(*) FILTER (WHERE j.collection_status = 'pending' OR j.collection_status = 'collecting') > 0 THEN 'partial'
            WHEN COUNT(*) = COUNT(*) FILTER (WHERE j.collection_status = 'completed' OR j.collection_status = 'skipped') THEN 'completed'
            ELSE 'pending'
        END,
        (array_agg(j.runner_set_id ORDER BY j.id))[1],
        (array_agg(j.config_id ORDER BY j.id) FILTER (WHERE j.config_id IS NOT NULL))[1],
        MIN(j.created_at),
        MAX(j.updated_at)
    FROM github_workflow_runs j
    LEFT JOIN github_runner_sets rs ON j.runner_set_id = rs.id
    WHERE j.github_run_id IS NOT NULL
    GROUP BY j.github_run_id, rs.github_owner, rs.github_repo
    ON CONFLICT DO NOTHING;

  END IF;
END $$;

-- Add run_summary_id column (safe on fresh DB too)
ALTER TABLE github_workflow_runs 
    ADD COLUMN IF NOT EXISTS run_summary_id BIGINT;

-- Update job records with run_summary_id (no-op if no data)
UPDATE github_workflow_runs j
SET run_summary_id = rs.id
FROM github_workflow_run_summaries rs
WHERE j.github_run_id = rs.github_run_id
  AND j.run_summary_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_run_summary_id 
    ON github_workflow_runs(run_summary_id);
