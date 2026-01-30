-- System Configuration Database Schema
-- This file defines the database schema for system configuration storage
-- that stores key-value pairs with JSON values

-- Table: system_config
-- Stores system configuration key-value pairs with JSONB values
CREATE TABLE IF NOT EXISTS system_config (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value JSONB NOT NULL,
    
    -- Description and metadata
    description TEXT,
    category VARCHAR(100),
    is_encrypted BOOLEAN DEFAULT FALSE,
    
    -- Version control
    version INTEGER DEFAULT 1,
    
    -- Access control
    is_readonly BOOLEAN DEFAULT FALSE,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255),
    updated_by VARCHAR(255)
);

-- Indexes for system_config
CREATE UNIQUE INDEX IF NOT EXISTS idx_system_config_key ON system_config(key);
CREATE INDEX IF NOT EXISTS idx_system_config_category ON system_config(category);
CREATE INDEX IF NOT EXISTS idx_system_config_updated_at ON system_config(updated_at DESC);

-- GIN index for JSONB value queries (for advanced JSON queries)
CREATE INDEX IF NOT EXISTS idx_system_config_value ON system_config USING GIN(value);

-- Comments for documentation
COMMENT ON TABLE system_config IS 'System configuration storage with key-value pairs and JSONB values';
COMMENT ON COLUMN system_config.key IS 'Unique configuration key identifier';
COMMENT ON COLUMN system_config.value IS 'JSONB value containing configuration data';
COMMENT ON COLUMN system_config.description IS 'Human-readable description of the configuration';
COMMENT ON COLUMN system_config.category IS 'Configuration category for grouping related configs';
COMMENT ON COLUMN system_config.is_encrypted IS 'Flag indicating if the value is encrypted';
COMMENT ON COLUMN system_config.version IS 'Configuration version for tracking changes';
COMMENT ON COLUMN system_config.is_readonly IS 'Flag indicating if the configuration is read-only';
COMMENT ON COLUMN system_config.created_at IS 'Timestamp when the configuration was first created';
COMMENT ON COLUMN system_config.updated_at IS 'Timestamp when the configuration was last updated';
COMMENT ON COLUMN system_config.created_by IS 'User who created the configuration';
COMMENT ON COLUMN system_config.updated_by IS 'User who last updated the configuration';

-- Table: system_config_history
-- Stores historical versions of configuration changes
CREATE TABLE IF NOT EXISTS system_config_history (
    id BIGSERIAL PRIMARY KEY,
    config_id BIGINT NOT NULL,
    key VARCHAR(255) NOT NULL,
    old_value JSONB,
    new_value JSONB NOT NULL,
    version INTEGER NOT NULL,
    change_reason TEXT,
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    changed_by VARCHAR(255)
);

-- Indexes for system_config_history
CREATE INDEX IF NOT EXISTS idx_system_config_history_config_id ON system_config_history(config_id);
CREATE INDEX IF NOT EXISTS idx_system_config_history_key ON system_config_history(key);
CREATE INDEX IF NOT EXISTS idx_system_config_history_changed_at ON system_config_history(changed_at DESC);

-- Comments for documentation
COMMENT ON TABLE system_config_history IS 'Historical versions of configuration changes for audit trail';
COMMENT ON COLUMN system_config_history.config_id IS 'Reference to the system_config table';
COMMENT ON COLUMN system_config_history.old_value IS 'Previous configuration value before the change';
COMMENT ON COLUMN system_config_history.new_value IS 'New configuration value after the change';
COMMENT ON COLUMN system_config_history.version IS 'Configuration version number';
COMMENT ON COLUMN system_config_history.change_reason IS 'Reason for the configuration change';
COMMENT ON COLUMN system_config_history.changed_at IS 'Timestamp when the configuration was changed';
COMMENT ON COLUMN system_config_history.changed_by IS 'User who made the configuration change';

