-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- See LICENSE for license information.

-- +goose Up
-- Cluster configuration table for control plane
-- Stores cluster connection info (K8S + Storage) from manual config or primus-safe sync

CREATE TABLE IF NOT EXISTS cluster_config (
    id              SERIAL PRIMARY KEY,
    cluster_name    VARCHAR(255) NOT NULL UNIQUE,
    display_name    VARCHAR(255),
    description     TEXT,
    
    -- Cluster Source: 'manual' or 'primus-safe'
    source          VARCHAR(50) NOT NULL DEFAULT 'manual',
    primus_safe_id  VARCHAR(255),
    
    -- K8S Connection Config
    k8s_endpoint    VARCHAR(512),
    k8s_ca_data     TEXT,
    k8s_cert_data   TEXT,
    k8s_key_data    TEXT,
    k8s_token       TEXT,
    
    -- Storage Config (Data Plane connection info)
    postgres_host       VARCHAR(255),
    postgres_port       INT DEFAULT 5432,
    postgres_username   VARCHAR(255),
    postgres_password   TEXT,
    postgres_db_name    VARCHAR(255),
    postgres_ssl_mode   VARCHAR(50) DEFAULT 'require',
    
    opensearch_host     VARCHAR(255),
    opensearch_port     INT DEFAULT 9200,
    opensearch_username VARCHAR(255),
    opensearch_password TEXT,
    opensearch_scheme   VARCHAR(10) DEFAULT 'https',
    
    prometheus_read_host    VARCHAR(255),
    prometheus_read_port    INT DEFAULT 8481,
    prometheus_write_host   VARCHAR(255),
    prometheus_write_port   INT DEFAULT 8480,
    
    -- Dataplane Deployment Status
    dataplane_status    VARCHAR(50) DEFAULT 'pending',
    dataplane_version   VARCHAR(100),
    dataplane_message   TEXT,
    last_deploy_time    TIMESTAMP,
    
    -- Storage Mode
    storage_mode            VARCHAR(50) DEFAULT 'external',
    managed_storage_config  JSONB DEFAULT '{}',
    
    -- Metadata
    status          VARCHAR(50) DEFAULT 'active',
    labels          JSONB DEFAULT '{}',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);

CREATE INDEX idx_cluster_config_name ON cluster_config(cluster_name);
CREATE INDEX idx_cluster_config_source ON cluster_config(source);
CREATE INDEX idx_cluster_config_status ON cluster_config(status);
CREATE INDEX idx_cluster_config_primus_safe_id ON cluster_config(primus_safe_id);
CREATE INDEX idx_cluster_config_dataplane_status ON cluster_config(dataplane_status);

-- +goose Down
DROP TABLE IF EXISTS cluster_config;
