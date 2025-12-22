-- Add soft delete support to namespace_info table
-- This migration adds deleted_at column for soft delete functionality

-- Add deleted_at column (nullable for soft delete)
ALTER TABLE namespace_info 
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

-- Create index for soft delete queries (filter out deleted records efficiently)
CREATE INDEX IF NOT EXISTS idx_namespace_info_deleted_at ON namespace_info(deleted_at);

-- Composite index for common queries: find active namespaces by name
CREATE INDEX IF NOT EXISTS idx_namespace_info_name_deleted_at ON namespace_info(name, deleted_at);

-- Comment for documentation
COMMENT ON COLUMN namespace_info.deleted_at IS 'Timestamp when the namespace info was soft deleted (NULL means not deleted)';

