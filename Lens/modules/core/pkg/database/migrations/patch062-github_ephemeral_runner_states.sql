-- patch062: Create github_ephemeral_runner_states table
-- This table stores raw K8s state of EphemeralRunner resources,
-- acting as a lightweight mirror decoupled from business logic.
-- The RunnerStateProcessor reads changes from this table to drive
-- workflow_run lifecycle and task creation.

CREATE TABLE IF NOT EXISTS github_ephemeral_runner_states (
    id BIGSERIAL PRIMARY KEY,
    namespace VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    uid VARCHAR(255) NOT NULL DEFAULT '',
    runner_set_name VARCHAR(255) NOT NULL DEFAULT '',
    runner_type VARCHAR(50) NOT NULL DEFAULT 'unknown',
    phase VARCHAR(50) NOT NULL DEFAULT '',
    github_run_id BIGINT NOT NULL DEFAULT 0,
    github_job_id BIGINT NOT NULL DEFAULT 0,
    github_run_number INT NOT NULL DEFAULT 0,
    workflow_name VARCHAR(500) NOT NULL DEFAULT '',
    head_sha VARCHAR(255) NOT NULL DEFAULT '',
    head_branch VARCHAR(255) NOT NULL DEFAULT '',
    repository VARCHAR(500) NOT NULL DEFAULT '',
    pod_phase VARCHAR(50) NOT NULL DEFAULT '',
    pod_condition VARCHAR(255) NOT NULL DEFAULT '',
    pod_message TEXT NOT NULL DEFAULT '',
    is_completed BOOLEAN NOT NULL DEFAULT FALSE,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    creation_timestamp TIMESTAMPTZ,
    completion_time TIMESTAMPTZ,
    deletion_time TIMESTAMPTZ,
    workflow_run_id BIGINT NOT NULL DEFAULT 0,
    run_summary_id BIGINT NOT NULL DEFAULT 0,
    last_status VARCHAR(50) NOT NULL DEFAULT '',
    last_processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint: one state record per (namespace, name) pair
CREATE UNIQUE INDEX IF NOT EXISTS idx_runner_states_ns_name
    ON github_ephemeral_runner_states(namespace, name);

-- Index for unprocessed states scan: processor queries WHERE updated_at > last_processed_at
CREATE INDEX IF NOT EXISTS idx_runner_states_unprocessed
    ON github_ephemeral_runner_states(updated_at, last_processed_at);

-- Index for runner set lookups
CREATE INDEX IF NOT EXISTS idx_runner_states_runner_set
    ON github_ephemeral_runner_states(runner_set_name, namespace);

-- Index for cleanup of old deleted states
CREATE INDEX IF NOT EXISTS idx_runner_states_deleted
    ON github_ephemeral_runner_states(is_deleted, updated_at)
    WHERE is_deleted = TRUE;
