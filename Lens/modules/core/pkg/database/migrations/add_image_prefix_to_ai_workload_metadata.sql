-- Add image_prefix column to ai_workload_metadata table
-- This column is used for efficient candidate workload queries in metadata reuse

-- Add image_prefix column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ai_workload_metadata' 
        AND column_name = 'image_prefix'
    ) THEN
        ALTER TABLE ai_workload_metadata 
        ADD COLUMN image_prefix TEXT;
    END IF;
END $$;

-- Add index for image_prefix + created_at queries (used in reuse matching)
CREATE INDEX IF NOT EXISTS idx_ai_workload_metadata_reuse_query
ON ai_workload_metadata(image_prefix, created_at DESC)
WHERE image_prefix IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN ai_workload_metadata.image_prefix IS 'Image repository address without tag, extracted from container image for efficient similarity matching';
COMMENT ON INDEX idx_ai_workload_metadata_reuse_query IS 'Composite index for efficient reuse candidate queries by image_prefix and time window';

-- Note: image_prefix should be populated by application layer when creating/updating workload metadata
-- Example: registry.example.com/primus:v1.2.3 -> registry.example.com/primus

