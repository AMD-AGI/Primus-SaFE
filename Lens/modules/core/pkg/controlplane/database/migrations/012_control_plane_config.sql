-- Control plane configuration table for storing global settings
-- like installer image version, default registry, etc.

CREATE TABLE IF NOT EXISTS control_plane_config (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value JSONB NOT NULL DEFAULT '{}',
    description TEXT,
    category VARCHAR(100),
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by VARCHAR(100)
);

-- Index for category queries
CREATE INDEX IF NOT EXISTS idx_control_plane_config_category ON control_plane_config(category);

-- Insert default configurations
INSERT INTO control_plane_config (key, value, description, category)
VALUES 
    ('installer.image', '{"repository": "primussafe/primus-lens-installer", "tag": "latest"}', 'Dataplane installer image configuration', 'installer'),
    ('default.registry', '{"url": "docker.io"}', 'Default container image registry', 'registry')
ON CONFLICT (key) DO NOTHING;

COMMENT ON TABLE control_plane_config IS 'Control plane level configuration settings';
COMMENT ON COLUMN control_plane_config.key IS 'Unique configuration key identifier';
COMMENT ON COLUMN control_plane_config.value IS 'JSONB value containing configuration data';
COMMENT ON COLUMN control_plane_config.category IS 'Configuration category for grouping';
