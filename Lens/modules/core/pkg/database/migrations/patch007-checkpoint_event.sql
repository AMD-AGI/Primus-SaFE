-- Create checkpoint_event table for tracking checkpoint operations
CREATE TABLE IF NOT EXISTS checkpoint_event (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(255) NOT NULL,
    pod_uuid VARCHAR(255),
    iteration INTEGER NOT NULL,
    checkpoint_path TEXT,
    event_type VARCHAR(50) NOT NULL,  -- start_saving, end_saving, loading
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    duration_ms BIGINT DEFAULT 0,
    size_bytes BIGINT DEFAULT 0,
    is_fast_ckpt BOOLEAN DEFAULT false,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    serial INTEGER DEFAULT 1,
    status VARCHAR(50) DEFAULT 'in_progress'  -- success, failed, in_progress
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_workload_uid ON checkpoint_event(workload_uid);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_iteration ON checkpoint_event(iteration);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_type ON checkpoint_event(event_type);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_status ON checkpoint_event(status);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_created_at ON checkpoint_event(created_at);
CREATE INDEX IF NOT EXISTS idx_checkpoint_event_workload_iteration ON checkpoint_event(workload_uid, iteration);

-- Add comment
COMMENT ON TABLE checkpoint_event IS 'Tracks checkpoint save/load events from training logs';
COMMENT ON COLUMN checkpoint_event.workload_uid IS 'Reference to the workload';
COMMENT ON COLUMN checkpoint_event.event_type IS 'Type of checkpoint event: start_saving, end_saving, loading';
COMMENT ON COLUMN checkpoint_event.duration_ms IS 'Duration of checkpoint operation in milliseconds';
COMMENT ON COLUMN checkpoint_event.is_fast_ckpt IS 'Whether this is a fast checkpoint';
COMMENT ON COLUMN checkpoint_event.metadata IS 'Additional metadata extracted from logs';

