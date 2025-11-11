-- Job Execution History Database Schema
-- This file defines the database schema for tracking job execution history
-- Stores historical records of all job executions for debugging and monitoring

-- Table: job_execution_history
-- Stores execution history for all scheduled jobs
CREATE TABLE IF NOT EXISTS job_execution_history (
    id BIGSERIAL PRIMARY KEY,
    
    -- Job identification
    job_name VARCHAR(255) NOT NULL,
    job_type VARCHAR(100) NOT NULL,
    schedule VARCHAR(50),
    
    -- Execution status
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    
    -- Timing information
    started_at TIMESTAMP NOT NULL,
    ended_at TIMESTAMP,
    duration_seconds DOUBLE PRECISION,
    
    -- Error information
    error_message TEXT,
    error_stack TEXT,
    
    -- Execution context
    cluster_name VARCHAR(100),
    hostname VARCHAR(255),
    
    -- Metadata (for additional context)
    metadata JSONB,
    
    -- Statistics (optional metrics collected during execution)
    execution_stats JSONB,
    
    -- Record creation timestamp
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for job_execution_history
CREATE INDEX IF NOT EXISTS idx_job_execution_history_job_name ON job_execution_history(job_name);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_status ON job_execution_history(status);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_started_at ON job_execution_history(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_ended_at ON job_execution_history(ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_job_name_started_at ON job_execution_history(job_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_status_started_at ON job_execution_history(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_history_duration ON job_execution_history(duration_seconds) WHERE duration_seconds IS NOT NULL;

-- GIN index for metadata queries
CREATE INDEX IF NOT EXISTS idx_job_execution_history_metadata ON job_execution_history USING GIN(metadata);

-- Comments for documentation
COMMENT ON TABLE job_execution_history IS 'Stores execution history for all scheduled jobs with detailed timing and error information';

COMMENT ON COLUMN job_execution_history.job_name IS 'Name of the job (extracted from job type)';
COMMENT ON COLUMN job_execution_history.job_type IS 'Full type name of the job';
COMMENT ON COLUMN job_execution_history.status IS 'Execution status: running, success, failed, cancelled, timeout';
COMMENT ON COLUMN job_execution_history.started_at IS 'Timestamp when the job execution started';
COMMENT ON COLUMN job_execution_history.ended_at IS 'Timestamp when the job execution ended (NULL if still running)';
COMMENT ON COLUMN job_execution_history.duration_seconds IS 'Total execution duration in seconds';
COMMENT ON COLUMN job_execution_history.error_message IS 'Error message if execution failed';
COMMENT ON COLUMN job_execution_history.error_stack IS 'Full error stack trace for debugging';
COMMENT ON COLUMN job_execution_history.metadata IS 'Additional metadata about the execution context';
COMMENT ON COLUMN job_execution_history.execution_stats IS 'Metrics collected during job execution (e.g., records processed, bytes transferred)';
