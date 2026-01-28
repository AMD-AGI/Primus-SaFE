-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- Release management tables for multi-cluster deployment

-- 1. Release versions - defines available versions with their configurations
CREATE TABLE IF NOT EXISTS release_versions (
    id              SERIAL PRIMARY KEY,
    version_name    VARCHAR(50) NOT NULL UNIQUE,      -- 'v0.5.0', 'v0.5.1-rc1'
    channel         VARCHAR(20) DEFAULT 'stable',     -- 'stable', 'beta', 'canary'
    
    -- Chart configuration
    chart_repo      VARCHAR(255) NOT NULL DEFAULT 'oci://docker.io/primussafe',
    chart_version   VARCHAR(50) NOT NULL,             -- Helm chart version
    
    -- Image configuration
    image_registry  VARCHAR(255) NOT NULL DEFAULT 'docker.io/primussafe',
    image_tag       VARCHAR(100) NOT NULL,            -- Image tag
    
    -- Values configuration
    default_values  JSONB NOT NULL DEFAULT '{}',      -- Default Helm values
    values_schema   JSONB,                            -- JSON Schema for form generation
    
    -- Metadata
    status          VARCHAR(20) DEFAULT 'draft',      -- draft/active/deprecated
    release_notes   TEXT,
    created_by      VARCHAR(100),
    
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_release_versions_channel ON release_versions(channel);
CREATE INDEX idx_release_versions_status ON release_versions(status);

-- 2. Cluster release configs - per-cluster configuration and state
CREATE TABLE IF NOT EXISTS cluster_release_configs (
    id                  SERIAL PRIMARY KEY,
    cluster_name        VARCHAR(255) NOT NULL UNIQUE,
    
    -- Version subscription
    release_version_id  INT REFERENCES release_versions(id),
    values_override     JSONB DEFAULT '{}',           -- Cluster-specific value overrides
    
    -- Current deployment state
    deployed_version_id INT REFERENCES release_versions(id),
    deployed_values     JSONB,                        -- Complete values snapshot of current deployment
    deployed_at         TIMESTAMP,
    
    -- Sync status
    sync_status         VARCHAR(20) DEFAULT 'unknown', -- unknown/synced/out_of_sync/upgrading/failed
    last_sync_at        TIMESTAMP,
    last_sync_error     TEXT,
    
    -- Auto-upgrade settings
    auto_upgrade        BOOLEAN DEFAULT false,
    upgrade_channel     VARCHAR(20) DEFAULT 'stable', -- Which channel to follow for auto-upgrade
    
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_cluster_release_configs_sync ON cluster_release_configs(sync_status);
CREATE INDEX idx_cluster_release_configs_version ON cluster_release_configs(release_version_id);

-- 3. Release history - audit trail of all deployments
CREATE TABLE IF NOT EXISTS release_history (
    id                  SERIAL PRIMARY KEY,
    cluster_name        VARCHAR(255) NOT NULL,
    release_version_id  INT NOT NULL REFERENCES release_versions(id),
    
    -- Action info
    action              VARCHAR(20) NOT NULL,         -- install/upgrade/rollback/sync
    triggered_by        VARCHAR(100),                 -- User or 'system' for auto-upgrade
    
    -- Configuration snapshot (for rollback)
    values_snapshot     JSONB NOT NULL,               -- Complete values at time of deployment
    previous_version_id INT REFERENCES release_versions(id),
    
    -- Execution status
    status              VARCHAR(20) DEFAULT 'pending', -- pending/running/completed/failed
    error_message       TEXT,
    stages_completed    TEXT[],                       -- Array of completed stage names
    
    -- Linked task
    task_id             INT,                          -- Reference to dataplane_install_tasks
    
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at          TIMESTAMP,
    completed_at        TIMESTAMP
);

CREATE INDEX idx_release_history_cluster ON release_history(cluster_name);
CREATE INDEX idx_release_history_status ON release_history(status);
CREATE INDEX idx_release_history_version ON release_history(release_version_id);

-- +goose Down
DROP TABLE IF EXISTS release_history;
DROP TABLE IF EXISTS cluster_release_configs;
DROP TABLE IF EXISTS release_versions;
