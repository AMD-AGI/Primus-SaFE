-- Migration: Add unique index to workload_pod_reference
-- Purpose: Prevent duplicate workload-pod references that were causing inflated GPU allocation calculations

-- First, clean up any remaining duplicates (keep the oldest record)
DELETE FROM workload_pod_reference a
USING workload_pod_reference b
WHERE a.id > b.id
  AND a.workload_uid = b.workload_uid
  AND a.pod_uid = b.pod_uid;

-- Add unique index to prevent future duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_pod_reference_unique 
ON workload_pod_reference(workload_uid, pod_uid);

-- Add individual indexes for query performance
CREATE INDEX IF NOT EXISTS idx_workload_pod_reference_workload_uid 
ON workload_pod_reference(workload_uid);

CREATE INDEX IF NOT EXISTS idx_workload_pod_reference_pod_uid 
ON workload_pod_reference(pod_uid);

COMMENT ON INDEX idx_workload_pod_reference_unique IS 'Prevents duplicate workload-pod reference entries';
