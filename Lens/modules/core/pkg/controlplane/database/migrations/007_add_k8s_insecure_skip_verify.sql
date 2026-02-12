-- Migration: Add k8s_insecure_skip_verify field to cluster_config
-- This allows skipping TLS certificate verification for K8S API connections

ALTER TABLE cluster_config ADD COLUMN IF NOT EXISTS k8s_insecure_skip_verify BOOLEAN DEFAULT FALSE;

-- Add index for potential filtering
CREATE INDEX IF NOT EXISTS idx_cluster_config_k8s_insecure ON cluster_config(k8s_insecure_skip_verify);
