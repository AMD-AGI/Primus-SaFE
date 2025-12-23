-- Framework Detection Metrics Database Schema
-- This file defines the database schema for storing framework detection metrics
-- used for monitoring and analysis of detection performance

-- Table: framework_detection_metrics
-- Stores aggregated metrics for framework detection
CREATE TABLE IF NOT EXISTS framework_detection_metrics (
    id BIGSERIAL PRIMARY KEY,
    metric_time TIMESTAMP NOT NULL,
    source VARCHAR(64),  -- Detection source (component, log, reuse, etc.)
    framework VARCHAR(64),  -- Framework name (primus, deepspeed, etc.)
    status VARCHAR(32),  -- Detection status (unknown, suspected, confirmed, verified, conflict, reused)
    
    -- Count metrics
    total_count INTEGER NOT NULL DEFAULT 0,
    conflict_count INTEGER DEFAULT 0,
    reuse_count INTEGER DEFAULT 0,
    
    -- Confidence metrics
    avg_confidence DECIMAL(4,3),
    min_confidence DECIMAL(4,3),
    max_confidence DECIMAL(4,3),
    
    -- Latency metrics (milliseconds)
    avg_detection_latency_ms INTEGER,
    p50_detection_latency_ms INTEGER,
    p95_detection_latency_ms INTEGER,
    p99_detection_latency_ms INTEGER,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for time-series queries
CREATE INDEX IF NOT EXISTS idx_detection_metrics_time
ON framework_detection_metrics(metric_time DESC);

-- Index for source and framework filtering
CREATE INDEX IF NOT EXISTS idx_detection_metrics_source_framework
ON framework_detection_metrics(source, framework, metric_time DESC);

-- Index for status filtering
CREATE INDEX IF NOT EXISTS idx_detection_metrics_status
ON framework_detection_metrics(status, metric_time DESC);

-- Index for framework-specific queries
CREATE INDEX IF NOT EXISTS idx_detection_metrics_framework
ON framework_detection_metrics(framework, metric_time DESC);

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_detection_metrics_composite
ON framework_detection_metrics(metric_time DESC, framework, source, status);

-- Comments for documentation
COMMENT ON TABLE framework_detection_metrics IS 'Aggregated metrics for framework detection monitoring';
COMMENT ON COLUMN framework_detection_metrics.metric_time IS 'Time bucket for the metrics (typically hourly)';
COMMENT ON COLUMN framework_detection_metrics.source IS 'Detection source (user, component, reuse, log, image)';
COMMENT ON COLUMN framework_detection_metrics.framework IS 'Framework name (primus, deepspeed, megatron, etc.)';
COMMENT ON COLUMN framework_detection_metrics.status IS 'Detection status (unknown, suspected, confirmed, verified, conflict, reused)';
COMMENT ON COLUMN framework_detection_metrics.total_count IS 'Total number of detections in this time bucket';
COMMENT ON COLUMN framework_detection_metrics.conflict_count IS 'Number of detections with conflicts';
COMMENT ON COLUMN framework_detection_metrics.reuse_count IS 'Number of detections from reuse';
COMMENT ON COLUMN framework_detection_metrics.avg_confidence IS 'Average confidence level (0.000-1.000)';
COMMENT ON COLUMN framework_detection_metrics.min_confidence IS 'Minimum confidence level in this bucket';
COMMENT ON COLUMN framework_detection_metrics.max_confidence IS 'Maximum confidence level in this bucket';
COMMENT ON COLUMN framework_detection_metrics.avg_detection_latency_ms IS 'Average detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.p50_detection_latency_ms IS 'P50 (median) detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.p95_detection_latency_ms IS 'P95 detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.p99_detection_latency_ms IS 'P99 detection latency in milliseconds';
COMMENT ON COLUMN framework_detection_metrics.created_at IS 'Timestamp when the metric record was created';

-- Note: Metrics are aggregated and inserted by application layer
-- Recommended aggregation interval: 1 hour
-- Example aggregation logic:
--   SELECT DATE_TRUNC('hour', detected_at) as metric_time,
--          source, framework, status,
--          COUNT(*) as total_count,
--          AVG(confidence) as avg_confidence,
--          MIN(confidence) as min_confidence,
--          MAX(confidence) as max_confidence
--   FROM detection_sources
--   GROUP BY metric_time, source, framework, status;

-- For TimescaleDB users, consider creating a hypertable:
-- SELECT create_hypertable('framework_detection_metrics', 'metric_time', if_not_exists => TRUE);

