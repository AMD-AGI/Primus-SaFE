-- ============================================================
-- TraceLens Sessions Table
-- Manages on-demand TraceLens analysis pods for profiler files
-- ============================================================

-- Create tracelens_sessions table
-- Note: Business logic constraints (status values, resource profiles) are enforced in application code
CREATE TABLE IF NOT EXISTS tracelens_sessions (
    -- Primary key
    id SERIAL PRIMARY KEY,
    
    -- Session identification (unique identifier for this analysis session)
    session_id VARCHAR(64) UNIQUE NOT NULL,
    
    -- Association with profiler data
    workload_uid VARCHAR(64) NOT NULL,
    profiler_file_id INTEGER,  -- References profiler_files(id), enforced in application
    
    -- User tracking
    user_id VARCHAR(64),
    user_email VARCHAR(256),
    
    -- Kubernetes resources
    pod_name VARCHAR(128),
    pod_namespace VARCHAR(64) DEFAULT 'primus-lens',
    pod_ip VARCHAR(64),
    pod_port INTEGER DEFAULT 8501,
    
    -- Session status (validated in application code)
    -- Valid values: pending, creating, initializing, ready, failed, expired, deleted
    status VARCHAR(32) DEFAULT 'pending' NOT NULL,
    status_message TEXT,
    
    -- Lifecycle management
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    ready_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    last_accessed_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Resource profile (validated in application code)
    -- Valid values: small, medium, large
    resource_profile VARCHAR(16) DEFAULT 'medium',
    
    -- Configuration and metadata (JSON)
    config JSONB DEFAULT '{}'
);

-- ============================================================
-- Indexes for efficient queries
-- ============================================================

-- Index for status-based queries (find all ready/expired sessions)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_status 
    ON tracelens_sessions(status);

-- Index for workload-based queries (find sessions for a workload)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_workload 
    ON tracelens_sessions(workload_uid);

-- Index for finding sessions by profiler file
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_profiler_file 
    ON tracelens_sessions(profiler_file_id);

-- Index for user-based queries (find user's sessions)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_user 
    ON tracelens_sessions(user_id);

-- Index for cleanup queries (find expired sessions that need cleanup)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_expires 
    ON tracelens_sessions(expires_at) 
    WHERE status NOT IN ('deleted', 'expired');

-- Index for finding active sessions (for resource management)
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_active 
    ON tracelens_sessions(status, created_at) 
    WHERE status IN ('pending', 'creating', 'initializing', 'ready');

-- Composite index for session lookup by workload and status
CREATE INDEX IF NOT EXISTS idx_tracelens_sessions_workload_status 
    ON tracelens_sessions(workload_uid, status);

-- ============================================================
-- Comments for documentation
-- ============================================================

COMMENT ON TABLE tracelens_sessions IS 'Manages on-demand TraceLens analysis pods for profiler files';

COMMENT ON COLUMN tracelens_sessions.session_id IS 'Unique session identifier, format: tls-{workload_uid_prefix}-{file_id}-{timestamp}';
COMMENT ON COLUMN tracelens_sessions.workload_uid IS 'Associated GPU workload UID';
COMMENT ON COLUMN tracelens_sessions.profiler_file_id IS 'Associated profiler file ID (logical reference to profiler_files.id)';
COMMENT ON COLUMN tracelens_sessions.pod_name IS 'Kubernetes pod name, format: tracelens-session-{session_id}';
COMMENT ON COLUMN tracelens_sessions.pod_ip IS 'Pod cluster IP for proxy routing';
COMMENT ON COLUMN tracelens_sessions.status IS 'Session lifecycle status: pending -> creating -> initializing -> ready -> expired -> deleted';
COMMENT ON COLUMN tracelens_sessions.expires_at IS 'Automatic expiration time, default TTL is 1 hour';
COMMENT ON COLUMN tracelens_sessions.last_accessed_at IS 'Last proxy request time, used for idle detection';
COMMENT ON COLUMN tracelens_sessions.resource_profile IS 'Resource allocation profile: small (2GB), medium (4GB), large (8GB)';
COMMENT ON COLUMN tracelens_sessions.config IS 'Additional configuration as JSON (e.g., analysis options)';

