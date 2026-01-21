-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- Add schema versioning support fields to github_workflow_metric_schemas table

-- Add schema_hash for quick schema matching
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS schema_hash VARCHAR(64) NOT NULL DEFAULT '';

-- Add is_wide_table flag for wide table detection
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS is_wide_table BOOLEAN NOT NULL DEFAULT false;

-- Add date_columns for wide table date column tracking
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS date_columns JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Add record_count for tracking metrics count per schema
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS record_count BIGINT NOT NULL DEFAULT 0;

-- Add first_seen_at for schema discovery time
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS first_seen_at TIMESTAMP;

-- Add last_seen_at for schema last usage time
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP;

-- Create unique index for schema hash matching (config_id + schema_hash should be unique)
CREATE UNIQUE INDEX IF NOT EXISTS idx_schema_config_hash
ON github_workflow_metric_schemas(config_id, schema_hash)
WHERE schema_hash != '';

-- Create index for listing schemas by config with version ordering
CREATE INDEX IF NOT EXISTS idx_schema_config_version
ON github_workflow_metric_schemas(config_id, version DESC);

-- Create index for metrics schema_id lookup optimization
CREATE INDEX IF NOT EXISTS idx_metrics_config_schema
ON github_workflow_metrics(config_id, schema_id, timestamp);

-- Update existing records with default timestamps
UPDATE github_workflow_metric_schemas
SET first_seen_at = created_at,
    last_seen_at = updated_at
WHERE first_seen_at IS NULL;
