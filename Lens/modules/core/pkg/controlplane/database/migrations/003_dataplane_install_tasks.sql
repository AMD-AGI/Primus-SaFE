-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- Dataplane install tasks table for control plane
-- Stores installation tasks that are executed by the dataplane installer job

CREATE TABLE IF NOT EXISTS dataplane_install_tasks (
    id              SERIAL PRIMARY KEY,
    cluster_name    VARCHAR(255) NOT NULL,
    
    -- Task info
    task_type       VARCHAR(50) NOT NULL DEFAULT 'install',  -- 'install', 'upgrade', 'uninstall'
    current_stage   VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Stage progression for lens-managed: pending -> operators -> infrastructure -> init -> storage_secret -> applications -> completed
    -- Stage progression for external:     pending -> init -> storage_secret -> applications -> completed
    
    -- Storage mode
    storage_mode    VARCHAR(50) NOT NULL DEFAULT 'external',  -- 'lens-managed' or 'external'
    
    -- Config (JSON)
    install_config  JSONB NOT NULL DEFAULT '{}',
    
    -- Status
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',  -- 'pending', 'running', 'completed', 'failed'
    error_message   TEXT,
    retry_count     INT DEFAULT 0,
    max_retries     INT DEFAULT 3,
    
    -- Timestamps
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at      TIMESTAMP,
    completed_at    TIMESTAMP
);

CREATE INDEX idx_install_tasks_cluster ON dataplane_install_tasks(cluster_name);
CREATE INDEX idx_install_tasks_status ON dataplane_install_tasks(status);

-- +goose Down
DROP TABLE IF EXISTS dataplane_install_tasks;
