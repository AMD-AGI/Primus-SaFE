-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- Migration: github_workflow_runs_workload_name_index
-- Description: Add index for runner_set_id + workload_name queries used by exporter
-- Database: PostgreSQL
--
-- Background:
-- The exporter uses GetByRunnerSetAndWorkloadName() which queries by (runner_set_id, workload_name).
-- Patch046 created an index on (runner_set_id, workload_uid), but the exporter prefers workload_name
-- as it's more stable across runner restarts.
--
-- This migration adds the missing index to optimize the exporter's most frequent query pattern.

-- ============================================================================
-- Add index for (runner_set_id, workload_name) - Used by exporter for uniqueness check
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_runner_set_workload_name
    ON github_workflow_runs(runner_set_id, workload_name);

COMMENT ON INDEX idx_github_workflow_runs_runner_set_workload_name IS 
    'Optimizes GetByRunnerSetAndWorkloadName() used by exporter reconciler';


-- ============================================================================
-- Optional: Add partial index for pending runs (frequently queried by collector)
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_status_pending
    ON github_workflow_runs(status, runner_set_id, created_at)
    WHERE status = 'pending';

COMMENT ON INDEX idx_github_workflow_runs_status_pending IS 
    'Optimizes ListPending() queries by collector job';
