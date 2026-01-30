-- Add install_scope column to dataplane_install_tasks
-- This allows separating infrastructure initialization from apps deployment

ALTER TABLE dataplane_install_tasks 
ADD COLUMN IF NOT EXISTS install_scope VARCHAR(50) NOT NULL DEFAULT 'full';

COMMENT ON COLUMN dataplane_install_tasks.install_scope IS 'Install scope: full, infrastructure, or apps';

-- Create index for scope-based queries
CREATE INDEX IF NOT EXISTS idx_dataplane_install_tasks_scope 
ON dataplane_install_tasks(cluster_name, install_scope);

-- Add infrastructure status columns to cluster_config
-- This tracks the one-time infrastructure initialization status separately from app deployments

ALTER TABLE cluster_config 
ADD COLUMN IF NOT EXISTS infrastructure_status VARCHAR(50) NOT NULL DEFAULT 'not_initialized';

ALTER TABLE cluster_config 
ADD COLUMN IF NOT EXISTS infrastructure_message TEXT DEFAULT '';

ALTER TABLE cluster_config 
ADD COLUMN IF NOT EXISTS infrastructure_time TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN cluster_config.infrastructure_status IS 'Infrastructure status: not_initialized, initializing, ready, failed';
COMMENT ON COLUMN cluster_config.infrastructure_message IS 'Infrastructure status message or error';
COMMENT ON COLUMN cluster_config.infrastructure_time IS 'Timestamp when infrastructure was last initialized';

-- Create index for infrastructure status queries
CREATE INDEX IF NOT EXISTS idx_cluster_config_infra_status 
ON cluster_config(infrastructure_status);
