-- Framework Configuration Database Schema
-- This file defines the database schema for storing framework configuration
-- including log patterns, image patterns, and priority settings

-- Table: framework_config
-- Stores framework configuration information for detection
CREATE TABLE IF NOT EXISTS framework_config (
    id SERIAL PRIMARY KEY,
    framework_name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128),
    description TEXT,
    priority INTEGER NOT NULL DEFAULT 50,  -- Priority for conflict resolution (higher is better)
    base_confidence DECIMAL(3,2) NOT NULL DEFAULT 0.70,  -- Base confidence level (0.00-1.00)
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL,  -- Framework-specific configuration (patterns, keys, etc.)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for framework_config
CREATE INDEX IF NOT EXISTS idx_framework_config_enabled 
ON framework_config(enabled, priority DESC);

CREATE INDEX IF NOT EXISTS idx_framework_config_name
ON framework_config(framework_name);

-- Comments for documentation
COMMENT ON TABLE framework_config IS 'Framework configuration for AI workload detection';
COMMENT ON COLUMN framework_config.framework_name IS 'Unique framework identifier (e.g., primus, deepspeed, megatron)';
COMMENT ON COLUMN framework_config.display_name IS 'Human-readable framework name';
COMMENT ON COLUMN framework_config.priority IS 'Priority for conflict resolution, higher value means higher priority';
COMMENT ON COLUMN framework_config.base_confidence IS 'Base confidence level for this framework (0.00-1.00)';
COMMENT ON COLUMN framework_config.enabled IS 'Whether this framework detection is enabled';
COMMENT ON COLUMN framework_config.config IS 'JSONB configuration containing log_patterns, image_patterns, env_keys, etc.';
COMMENT ON COLUMN framework_config.created_at IS 'Timestamp when the configuration was created';
COMMENT ON COLUMN framework_config.updated_at IS 'Timestamp when the configuration was last updated (managed by application)';

-- Insert default framework configurations
INSERT INTO framework_config (framework_name, display_name, priority, base_confidence, config) VALUES
('primus', 'Primus', 80, 0.75, '{
    "log_patterns": [
        {"pattern": "primus-rocm-memory", "weight": 1.0, "category": "memory", "description": "Primus ROCm memory allocation"},
        {"pattern": "PrimusTrainer", "weight": 0.9, "category": "framework", "description": "Primus trainer initialization"},
        {"pattern": "primus\\\\.distributed", "weight": 0.8, "category": "distributed", "description": "Primus distributed training"}
    ],
    "image_patterns": ["primus", "primus-training"],
    "env_keys": ["PRIMUS_CONFIG", "PRIMUS_MASTER_ADDR"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb),
('deepspeed', 'DeepSpeed', 75, 0.75, '{
    "log_patterns": [
        {"pattern": "DeepSpeed Engine", "weight": 1.0, "category": "framework", "description": "DeepSpeed engine initialization"},
        {"pattern": "deepspeed\\\\.runtime", "weight": 0.9, "category": "runtime", "description": "DeepSpeed runtime"}
    ],
    "image_patterns": ["deepspeed"],
    "env_keys": ["DEEPSPEED_CONFIG"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb),
('megatron', 'Megatron-LM', 75, 0.75, '{
    "log_patterns": [
        {"pattern": "Megatron-LM", "weight": 1.0, "category": "framework", "description": "Megatron-LM framework"},
        {"pattern": "megatron\\\\.model", "weight": 0.9, "category": "model", "description": "Megatron model initialization"}
    ],
    "image_patterns": ["megatron", "megatron-lm"],
    "env_keys": ["MEGATRON_CONFIG"],
    "label_keys": ["ai.amd.com/framework"]
}'::jsonb)
ON CONFLICT (framework_name) DO NOTHING;

