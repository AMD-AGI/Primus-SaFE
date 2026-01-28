-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- Add is_default column to cluster_config table
-- Only one cluster can be marked as default at a time

ALTER TABLE cluster_config ADD COLUMN IF NOT EXISTS is_default BOOLEAN DEFAULT FALSE;

-- Create partial unique index to ensure only one default cluster
CREATE UNIQUE INDEX IF NOT EXISTS idx_cluster_config_is_default_unique 
    ON cluster_config (is_default) WHERE is_default = TRUE;

-- +goose Down
DROP INDEX IF EXISTS idx_cluster_config_is_default_unique;
ALTER TABLE cluster_config DROP COLUMN IF EXISTS is_default;
