-- GPU Usage Weekly Reports Database Schema
-- This file defines the database schema for storing GPU usage weekly reports
-- Reports include HTML, PDF, and JSON formats stored as BLOB/JSONB

-- Table: gpu_usage_weekly_reports
-- Stores generated GPU usage weekly reports with multiple format outputs
CREATE TABLE IF NOT EXISTS gpu_usage_weekly_reports (
    id VARCHAR(64) PRIMARY KEY,
    cluster_name VARCHAR(128) NOT NULL,
    
    -- Report period
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    
    -- Report generation info
    generated_at TIMESTAMP NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    
    -- Report content in different formats
    -- HTML and PDF are stored as binary data (BYTEA in PostgreSQL)
    html_content BYTEA,
    pdf_content BYTEA,
    
    -- JSON content stored as JSONB for efficient querying
    json_content JSONB,
    
    -- Metadata contains summary information like avg_utilization, total_gpus, etc.
    metadata JSONB,
    
    -- Error tracking
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for gpu_usage_weekly_reports
-- Index on cluster_name for filtering by cluster
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_cluster_name 
    ON gpu_usage_weekly_reports(cluster_name);

-- Index on status for filtering pending/completed/failed reports
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_status 
    ON gpu_usage_weekly_reports(status);

-- Composite index on cluster_name and period_start for time-range queries
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_cluster_period 
    ON gpu_usage_weekly_reports(cluster_name, period_start DESC);

-- Index on period_start for date-based queries and cleanup
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_period_start 
    ON gpu_usage_weekly_reports(period_start DESC);

-- Index on generated_at for listing reports by generation time
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_generated_at 
    ON gpu_usage_weekly_reports(generated_at DESC);

-- Index on updated_at for tracking recent changes
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_updated_at 
    ON gpu_usage_weekly_reports(updated_at DESC);

-- GIN index for metadata JSONB queries (for filtering by summary stats)
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_metadata 
    ON gpu_usage_weekly_reports USING GIN(metadata);

-- GIN index for json_content JSONB queries (optional, for advanced queries)
CREATE INDEX IF NOT EXISTS idx_gpu_weekly_reports_json_content 
    ON gpu_usage_weekly_reports USING GIN(json_content);

-- Comments for documentation
COMMENT ON TABLE gpu_usage_weekly_reports IS 'Stores GPU usage weekly reports with HTML, PDF, and JSON formats';
COMMENT ON COLUMN gpu_usage_weekly_reports.id IS 'Unique report identifier (e.g., rpt_20251123_x_flannel_001)';
COMMENT ON COLUMN gpu_usage_weekly_reports.cluster_name IS 'Name of the cluster this report is for';
COMMENT ON COLUMN gpu_usage_weekly_reports.period_start IS 'Start timestamp of the report period';
COMMENT ON COLUMN gpu_usage_weekly_reports.period_end IS 'End timestamp of the report period';
COMMENT ON COLUMN gpu_usage_weekly_reports.generated_at IS 'Timestamp when the report was generated';
COMMENT ON COLUMN gpu_usage_weekly_reports.status IS 'Report status: pending, generated, sent, or failed';
COMMENT ON COLUMN gpu_usage_weekly_reports.html_content IS 'HTML report content stored as binary data';
COMMENT ON COLUMN gpu_usage_weekly_reports.pdf_content IS 'PDF report content stored as binary data';
COMMENT ON COLUMN gpu_usage_weekly_reports.json_content IS 'Structured JSON report data for programmatic access';
COMMENT ON COLUMN gpu_usage_weekly_reports.metadata IS 'Report metadata including summary statistics (avg_utilization, total_gpus, etc.)';
COMMENT ON COLUMN gpu_usage_weekly_reports.error_message IS 'Error message if report generation or sending failed';
COMMENT ON COLUMN gpu_usage_weekly_reports.created_at IS 'Timestamp when the report record was created';
COMMENT ON COLUMN gpu_usage_weekly_reports.updated_at IS 'Timestamp when the report record was last updated';

