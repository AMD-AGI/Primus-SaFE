-- patch068-snapshot_storage_ref.sql
-- Add storage_key column to workload_code_snapshot for external file storage.
-- When storage_key is set, file contents are stored in S3/local filesystem
-- instead of in the JSONB columns (entry_script, local_modules, config_files).
-- The JSONB columns retain only metadata (path, hash, size) without the "content" field.

ALTER TABLE workload_code_snapshot
    ADD COLUMN IF NOT EXISTS storage_key VARCHAR(512) DEFAULT NULL;

ALTER TABLE workload_code_snapshot
    ADD COLUMN IF NOT EXISTS storage_type VARCHAR(20) DEFAULT NULL;

-- Index for looking up by storage key
CREATE INDEX IF NOT EXISTS idx_wcs_storage_key ON workload_code_snapshot (storage_key)
    WHERE storage_key IS NOT NULL;

COMMENT ON COLUMN workload_code_snapshot.storage_key IS 'External storage reference key (e.g. {workload_uid}/{fingerprint}). NULL means content is inline in JSONB columns.';
COMMENT ON COLUMN workload_code_snapshot.storage_type IS 'Storage backend type: s3 or local. NULL means inline.';
