-- Migration: sync_schema_columns
-- Description: Add missing columns to github_workflow_metric_schemas for wide table support
-- These columns were added manually to tw-proj2 but not recorded in migrations
-- Database: PostgreSQL

-- Add time_field column for time series data
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS time_field VARCHAR(100) DEFAULT NULL;

-- Add column_definitions for storing column metadata (renamed from 'columns' to avoid GORM gen conflict)
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS column_definitions JSONB;

-- Add date_column_pattern for wide table date column detection
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS date_column_pattern VARCHAR(255);

-- Add date_column_config for wide table date column configuration
ALTER TABLE github_workflow_metric_schemas
ADD COLUMN IF NOT EXISTS date_column_config JSONB;

-- Comments
COMMENT ON COLUMN github_workflow_metric_schemas.time_field IS 'Field name used as time dimension for time series data';
COMMENT ON COLUMN github_workflow_metric_schemas.column_definitions IS 'JSON object mapping column names to their configurations';
COMMENT ON COLUMN github_workflow_metric_schemas.date_column_pattern IS 'Regex pattern for detecting date columns in wide tables';
COMMENT ON COLUMN github_workflow_metric_schemas.date_column_config IS 'Configuration for date column parsing in wide tables';
