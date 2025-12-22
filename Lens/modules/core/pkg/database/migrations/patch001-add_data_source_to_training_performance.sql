-- Add data_source column to training_performance table
-- This column is used to distinguish the source of training performance data

-- Create enum type for data source if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'training_data_source') THEN
        CREATE TYPE training_data_source AS ENUM ('log', 'wandb', 'tensorflow');
    END IF;
END $$;

-- Add data_source column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'training_performance' 
        AND column_name = 'data_source'
    ) THEN
        ALTER TABLE training_performance 
        ADD COLUMN data_source training_data_source DEFAULT 'log';
    END IF;
END $$;

-- Add index for data_source queries (useful for filtering by source)
CREATE INDEX IF NOT EXISTS idx_training_performance_data_source
ON training_performance(data_source);

-- Add composite index for common queries (workload + data_source + iteration)
CREATE INDEX IF NOT EXISTS idx_training_performance_workload_source_iteration
ON training_performance(workload_uid, data_source, iteration DESC);

-- Add comments for documentation
COMMENT ON COLUMN training_performance.data_source IS 'Source of training performance data: log (parsed from logs), wandb (from W&B API), or tensorflow (from TensorFlow/TensorBoard)';
COMMENT ON INDEX idx_training_performance_data_source IS 'Index for filtering training performance data by source';
COMMENT ON INDEX idx_training_performance_workload_source_iteration IS 'Composite index for efficient queries by workload, data source, and iteration';

-- Note: Default value is 'log' for backward compatibility with existing data
-- Application layer should explicitly set data_source when inserting new records

