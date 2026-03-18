-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- GPU Usage Weekly Reports table for control plane
-- Stores generated GPU usage weekly reports for ALL clusters
-- Reports include HTML, PDF, and JSON formats

CREATE TABLE IF NOT EXISTS gpu_usage_weekly_reports (
    id              VARCHAR(64) PRIMARY KEY,
    cluster_name    VARCHAR(128) NOT NULL,
    
    -- Report period
    period_start    TIMESTAMP NOT NULL,
    period_end      TIMESTAMP NOT NULL,
    
    -- Report generation info
    generated_at    TIMESTAMP NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',  -- pending, generated, sent, failed
    
    -- Report content in different formats
    html_content    BYTEA,
    pdf_content     BYTEA,
    
    -- JSON content stored as JSONB for efficient querying
    json_content    JSONB,
    
    -- Metadata contains summary information like avg_utilization, total_gpus, etc.
    metadata        JSONB,
    
    -- Error tracking
    error_message   TEXT,
    
    -- Timestamps
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_cp_gpu_weekly_reports_cluster_name 
    ON gpu_usage_weekly_reports(cluster_name);

CREATE INDEX IF NOT EXISTS idx_cp_gpu_weekly_reports_status 
    ON gpu_usage_weekly_reports(status);

CREATE INDEX IF NOT EXISTS idx_cp_gpu_weekly_reports_cluster_period 
    ON gpu_usage_weekly_reports(cluster_name, period_start DESC);

CREATE INDEX IF NOT EXISTS idx_cp_gpu_weekly_reports_generated_at 
    ON gpu_usage_weekly_reports(generated_at DESC);

CREATE INDEX IF NOT EXISTS idx_cp_gpu_weekly_reports_period_start 
    ON gpu_usage_weekly_reports(period_start DESC);

-- GIN index for metadata JSONB queries
CREATE INDEX IF NOT EXISTS idx_cp_gpu_weekly_reports_metadata 
    ON gpu_usage_weekly_reports USING GIN(metadata);

-- Comments
COMMENT ON TABLE gpu_usage_weekly_reports IS 'Stores GPU usage weekly reports for all clusters (centralized in control plane)';
COMMENT ON COLUMN gpu_usage_weekly_reports.cluster_name IS 'Name of the cluster this report is for';

-- +goose Down
DROP TABLE IF EXISTS gpu_usage_weekly_reports;
