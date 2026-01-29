-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- TraceLens Sessions table for control plane
-- Centralized storage for all TraceLens sessions across all data clusters
-- Pods run in management cluster, session metadata stored here

CREATE TABLE IF NOT EXISTS tracelens_sessions (
    id              SERIAL PRIMARY KEY,
    session_id      VARCHAR(128) NOT NULL UNIQUE,
    cluster_name    VARCHAR(128) NOT NULL,  -- Data cluster where profiler file belongs
    
    -- Workload info
    workload_uid    VARCHAR(128) NOT NULL,
    profiler_file_id INT NOT NULL,          -- Logical reference to profiler_files.id in data cluster
    
    -- User info
    user_id         VARCHAR(128),
    user_email      VARCHAR(256),
    
    -- Pod info (pod runs in management cluster)
    pod_name        VARCHAR(128),
    pod_namespace   VARCHAR(128) DEFAULT 'primus-lens',
    pod_ip          VARCHAR(64),
    pod_port        INT DEFAULT 8501,
    
    -- Status
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',
    status_message  TEXT,
    
    -- Resource profile
    resource_profile VARCHAR(32) DEFAULT 'medium',
    
    -- Additional config
    config          JSONB DEFAULT '{}',
    
    -- Timestamps
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ready_at        TIMESTAMP,
    expires_at      TIMESTAMP NOT NULL,
    last_accessed_at TIMESTAMP,
    deleted_at      TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_cp_tracelens_session_id 
    ON tracelens_sessions(session_id);

CREATE INDEX IF NOT EXISTS idx_cp_tracelens_cluster_name 
    ON tracelens_sessions(cluster_name);

CREATE INDEX IF NOT EXISTS idx_cp_tracelens_workload_uid 
    ON tracelens_sessions(workload_uid);

CREATE INDEX IF NOT EXISTS idx_cp_tracelens_status 
    ON tracelens_sessions(status);

CREATE INDEX IF NOT EXISTS idx_cp_tracelens_expires_at 
    ON tracelens_sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_cp_tracelens_profiler_file 
    ON tracelens_sessions(cluster_name, profiler_file_id);

-- Composite index for finding active sessions
CREATE INDEX IF NOT EXISTS idx_cp_tracelens_active_lookup 
    ON tracelens_sessions(workload_uid, profiler_file_id, status) 
    WHERE deleted_at IS NULL;

-- Comments
COMMENT ON TABLE tracelens_sessions IS 'Centralized TraceLens session management for all clusters';
COMMENT ON COLUMN tracelens_sessions.cluster_name IS 'Data cluster name where the profiler file belongs';
COMMENT ON COLUMN tracelens_sessions.profiler_file_id IS 'Reference to profiler_files.id in the data cluster database';

-- +goose Down
DROP TABLE IF EXISTS tracelens_sessions;
