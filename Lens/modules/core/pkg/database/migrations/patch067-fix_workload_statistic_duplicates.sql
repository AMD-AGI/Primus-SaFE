-- patch067-fix_workload_statistic_duplicates.sql
-- Fix: workload_statistic had millions of duplicate rows because GetOrCreate filtered
-- by workload_status IN ('Running','Pending'). When status changed to Done/Failed,
-- the old record was not found and a new row was inserted every cycle (~30-60s).
--
-- This migration:
-- 1. Deletes all duplicate rows, keeping only the one with the highest ID per (uid, cluster_name).
-- 2. Drops the old partial unique index that only covered Running/Pending statuses.
-- 3. Creates a new unique index on (uid, cluster_name) covering ALL statuses.

-- Step 1: Delete duplicate rows, keeping only the latest record per (uid, cluster_name).
DELETE FROM workload_statistic
WHERE id NOT IN (
    SELECT MAX(id)
    FROM workload_statistic
    GROUP BY uid, cluster_name
);

-- Step 2: Drop the old partial unique index (only covered Running/Pending).
DROP INDEX IF EXISTS idx_workload_statistic_unique_active;

-- Step 3: Create a proper unique index on (uid, cluster_name) covering ALL statuses.
-- This prevents duplicate rows regardless of workload_status transitions.
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_statistic_uid_cluster_unique
ON workload_statistic (uid, cluster_name);
