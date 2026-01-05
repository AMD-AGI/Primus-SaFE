-- Migration: github_workflow_runner_set_relations
-- Description: Restructure github_workflow_runs and github_workflow_configs to directly reference github_runner_sets
-- Database: PostgreSQL
--
-- Changes:
-- 1. Add runner_set_id to github_workflow_configs (replaces namespace/name/uid lookup)
-- 2. Add runner_set_id to github_workflow_runs (replaces config_id dependency)
-- 3. Make config_id optional in github_workflow_runs
-- 4. Add runner_set_name/namespace to github_workflow_runs for denormalization

-- ============================================================================
-- Table: github_workflow_configs - Add runner_set_id for direct reference
-- ============================================================================

ALTER TABLE github_workflow_configs
ADD COLUMN IF NOT EXISTS runner_set_id BIGINT;

COMMENT ON COLUMN github_workflow_configs.runner_set_id IS 'Direct reference to github_runner_sets.id';

-- Create index for runner_set_id lookup
CREATE INDEX IF NOT EXISTS idx_github_workflow_configs_runner_set_id
    ON github_workflow_configs(runner_set_id);


-- ============================================================================
-- Table: github_workflow_runs - Add runner_set_id and make config_id optional
-- ============================================================================

-- Add runner_set_id column
ALTER TABLE github_workflow_runs
ADD COLUMN IF NOT EXISTS runner_set_id BIGINT;

-- Add runner_set_name and runner_set_namespace for denormalization
ALTER TABLE github_workflow_runs
ADD COLUMN IF NOT EXISTS runner_set_name VARCHAR(255);

ALTER TABLE github_workflow_runs
ADD COLUMN IF NOT EXISTS runner_set_namespace VARCHAR(255);

COMMENT ON COLUMN github_workflow_runs.runner_set_id IS 'Direct reference to github_runner_sets.id';
COMMENT ON COLUMN github_workflow_runs.runner_set_name IS 'Denormalized runner set name for efficient querying';
COMMENT ON COLUMN github_workflow_runs.runner_set_namespace IS 'Denormalized runner set namespace for efficient querying';

-- Make config_id nullable (it's now optional, runs are primarily linked via runner_set)
ALTER TABLE github_workflow_runs
ALTER COLUMN config_id DROP NOT NULL;

-- Create indexes for runner_set lookups
CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_runner_set_id
    ON github_workflow_runs(runner_set_id);

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_runner_set_name
    ON github_workflow_runs(runner_set_namespace, runner_set_name);

-- Composite index for runner_set_id + status queries
CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_runner_set_status
    ON github_workflow_runs(runner_set_id, status);

-- Unique constraint: one run per workload_uid per runner_set
-- (replacing the old config_id + workload_uid constraint)
DROP INDEX IF EXISTS idx_github_workflow_runs_config_workload;

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_workflow_runs_runner_set_workload
    ON github_workflow_runs(runner_set_id, workload_uid);


-- ============================================================================
-- Data Migration: Populate runner_set_id from existing config relationships
-- ============================================================================

-- For github_workflow_configs: populate runner_set_id from runner_set_namespace/name
UPDATE github_workflow_configs c
SET runner_set_id = rs.id
FROM github_runner_sets rs
WHERE c.runner_set_namespace = rs.namespace
  AND c.runner_set_name = rs.name
  AND c.runner_set_id IS NULL;

-- For github_workflow_runs: populate runner_set_id from config
UPDATE github_workflow_runs r
SET 
    runner_set_id = c.runner_set_id,
    runner_set_name = c.runner_set_name,
    runner_set_namespace = c.runner_set_namespace
FROM github_workflow_configs c
WHERE r.config_id = c.id
  AND r.runner_set_id IS NULL;


