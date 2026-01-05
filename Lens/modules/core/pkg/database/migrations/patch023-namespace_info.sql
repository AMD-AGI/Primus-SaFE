-- Namespace Information Database Schema
-- This file defines the database schema for storing namespace GPU resource information

-- Table: namespace_info
-- Stores namespace GPU configuration and resource allocation information
CREATE TABLE IF NOT EXISTS namespace_info (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    gpu_model VARCHAR(100) NOT NULL,
    gpu_resource INT NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for namespace_info
CREATE UNIQUE INDEX IF NOT EXISTS idx_namespace_info_name ON namespace_info(name);
CREATE INDEX IF NOT EXISTS idx_namespace_info_gpu_model ON namespace_info(gpu_model);
CREATE INDEX IF NOT EXISTS idx_namespace_info_updated_at ON namespace_info(updated_at DESC);

-- Comments for documentation
COMMENT ON TABLE namespace_info IS 'Namespace GPU resource information storage';
COMMENT ON COLUMN namespace_info.name IS 'Namespace name (unique identifier)';
COMMENT ON COLUMN namespace_info.gpu_model IS 'GPU model type allocated to this namespace';
COMMENT ON COLUMN namespace_info.gpu_resource IS 'Number of GPU cards allocated to this namespace';
COMMENT ON COLUMN namespace_info.created_at IS 'Timestamp when the namespace info was first created';
COMMENT ON COLUMN namespace_info.updated_at IS 'Timestamp when the namespace info was last updated';

