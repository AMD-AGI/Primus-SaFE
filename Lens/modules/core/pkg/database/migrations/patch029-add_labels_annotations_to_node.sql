-- Add labels and annotations columns to node table
-- These columns store Kubernetes node labels and annotations as JSONB

-- Add labels column if it doesn't exist
ALTER TABLE node 
ADD COLUMN IF NOT EXISTS labels JSONB DEFAULT '{}';

-- Add annotations column if it doesn't exist
ALTER TABLE node 
ADD COLUMN IF NOT EXISTS annotations JSONB DEFAULT '{}';

-- Create GIN index for labels to support efficient JSONB queries
CREATE INDEX IF NOT EXISTS idx_node_labels ON node USING GIN (labels);

-- Create GIN index for annotations to support efficient JSONB queries
CREATE INDEX IF NOT EXISTS idx_node_annotations ON node USING GIN (annotations);

-- Add comments for documentation
COMMENT ON COLUMN node.labels IS 'Kubernetes node labels stored as JSONB key-value pairs';
COMMENT ON COLUMN node.annotations IS 'Kubernetes node annotations stored as JSONB key-value pairs';

