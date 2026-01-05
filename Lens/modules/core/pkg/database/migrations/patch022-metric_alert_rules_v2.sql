-- Migration for Metric Alert Rules V2 - Add resource mapping and alert enrichment features
-- Run this migration to add new columns to existing metric_alert_rules table

-- Add resource mapping configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS resource_mapping JSONB;

-- Add alert enrichment configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS alert_enrichment JSONB;

-- Add alert grouping configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS alert_grouping JSONB;

-- Add alert routing configuration column
ALTER TABLE metric_alert_rules
ADD COLUMN IF NOT EXISTS alert_routing JSONB;

-- Add comments
COMMENT ON COLUMN metric_alert_rules.resource_mapping IS 'Configuration for mapping Prometheus labels to Primus-Lens resources';
COMMENT ON COLUMN metric_alert_rules.alert_enrichment IS 'Configuration for enriching alerts with resource information';
COMMENT ON COLUMN metric_alert_rules.alert_grouping IS 'Configuration for grouping related alerts';
COMMENT ON COLUMN metric_alert_rules.alert_routing IS 'Configuration for routing alerts to different receivers';

