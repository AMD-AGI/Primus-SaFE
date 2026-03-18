-- patch063: Add safe_workload_id for SaFE UnifiedJob association
-- Links EphemeralRunner (GitHub info) with its corresponding UnifiedJob (platform statistics)

ALTER TABLE github_ephemeral_runner_states
    ADD COLUMN IF NOT EXISTS safe_workload_id VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE github_workflow_runs
    ADD COLUMN IF NOT EXISTS safe_workload_id VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_safe_workload_id
    ON github_workflow_runs (safe_workload_id)
    WHERE safe_workload_id != '';
