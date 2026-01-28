-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- Cluster deployment history table
-- Tracks dataplane installation/upgrade/uninstall operations

CREATE TABLE IF NOT EXISTS cluster_deployment_history (
    id              SERIAL PRIMARY KEY,
    cluster_name    VARCHAR(255) NOT NULL,
    
    -- Deployment Info
    deployment_type VARCHAR(50) NOT NULL,  -- 'install', 'upgrade', 'uninstall'
    version         VARCHAR(100),
    values_yaml     TEXT,
    
    -- Status
    status          VARCHAR(50) NOT NULL,  -- 'started', 'success', 'failed'
    message         TEXT,
    logs            TEXT,
    
    -- Timing
    started_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    finished_at     TIMESTAMP
);

CREATE INDEX idx_deployment_history_cluster ON cluster_deployment_history(cluster_name);
CREATE INDEX idx_deployment_history_status ON cluster_deployment_history(status);
CREATE INDEX idx_deployment_history_started_at ON cluster_deployment_history(started_at);

-- +goose Down
DROP TABLE IF EXISTS cluster_deployment_history;
