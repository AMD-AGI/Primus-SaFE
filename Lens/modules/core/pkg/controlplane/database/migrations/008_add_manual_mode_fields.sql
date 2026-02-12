-- Migration: Add manual mode fields to cluster_config
-- k8s_manual_mode: when true, K8S config won't be overwritten by primus-safe-adapter sync
-- storage_manual_mode: when true, storage config won't be overwritten by multi-cluster config sync job

ALTER TABLE cluster_config ADD COLUMN IF NOT EXISTS k8s_manual_mode BOOLEAN DEFAULT FALSE;
ALTER TABLE cluster_config ADD COLUMN IF NOT EXISTS storage_manual_mode BOOLEAN DEFAULT FALSE;

-- Add index for potential filtering
CREATE INDEX IF NOT EXISTS idx_cluster_config_k8s_manual ON cluster_config(k8s_manual_mode);
CREATE INDEX IF NOT EXISTS idx_cluster_config_storage_manual ON cluster_config(storage_manual_mode);

COMMENT ON COLUMN cluster_config.k8s_manual_mode IS 'When true, K8S configuration will not be overwritten by primus-safe-adapter sync';
COMMENT ON COLUMN cluster_config.storage_manual_mode IS 'When true, storage configuration will not be overwritten by multi-cluster config sync job';
