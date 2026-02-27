-- patch069: Add container_image column to gpu_pods
-- Purpose: Store the primary container image reference for each GPU pod,
-- enabling the intent analysis pipeline to determine which image to analyze.

ALTER TABLE gpu_pods ADD COLUMN IF NOT EXISTS container_image VARCHAR(500) DEFAULT NULL;

-- Index for querying pods by owner_uid (workload UID) with image populated
-- This is used by SpecCollector to look up images for a given workload.
CREATE INDEX IF NOT EXISTS idx_gpu_pods_owner_uid_image
    ON gpu_pods(owner_uid)
    WHERE container_image IS NOT NULL AND owner_uid != '';
