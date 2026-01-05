-- Migration: github_workflow_display_settings
-- Description: Add display_settings column to github_workflow_configs for chart display customization
-- Database: PostgreSQL

-- Add display_settings JSONB column to github_workflow_configs
ALTER TABLE github_workflow_configs
ADD COLUMN IF NOT EXISTS display_settings JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Comment
COMMENT ON COLUMN github_workflow_configs.display_settings IS 'JSON object for display customization: defaultChartGroupBy, showRawDataByDefault, defaultChartType';

-- Example display_settings structure:
-- {
--   "defaultChartGroupBy": "model_name",     -- Default dimension for chart grouping (empty = no grouping)
--   "showRawDataByDefault": false,           -- Whether to expand raw data table by default
--   "defaultChartType": "line"               -- Default chart type: "line" or "bar"
-- }

