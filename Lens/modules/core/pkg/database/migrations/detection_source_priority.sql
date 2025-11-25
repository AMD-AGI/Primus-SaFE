-- Detection Source Priority Database Schema
-- This file defines the database schema for storing detection source priority configuration
-- which determines how different detection sources are weighted and prioritized

-- Table: detection_source_priority
-- Stores priority and confidence configuration for different detection sources
CREATE TABLE IF NOT EXISTS detection_source_priority (
    id SERIAL PRIMARY KEY,
    source_name VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128),
    priority INTEGER NOT NULL,  -- Priority level (higher is better)
    base_confidence DECIMAL(3,2) NOT NULL,  -- Base confidence level (0.00-1.00)
    confidence_decay DECIMAL(3,2) DEFAULT 1.0,  -- Decay rate for reused detections (0.00-1.00)
    enabled BOOLEAN NOT NULL DEFAULT true,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for detection_source_priority
CREATE INDEX IF NOT EXISTS idx_detection_source_priority_enabled
ON detection_source_priority(enabled, priority DESC);

CREATE INDEX IF NOT EXISTS idx_detection_source_priority_name
ON detection_source_priority(source_name);

-- Comments for documentation
COMMENT ON TABLE detection_source_priority IS 'Configuration for detection source priority and confidence levels';
COMMENT ON COLUMN detection_source_priority.source_name IS 'Unique source identifier (e.g., user, component, log, image)';
COMMENT ON COLUMN detection_source_priority.display_name IS 'Human-readable source name';
COMMENT ON COLUMN detection_source_priority.priority IS 'Priority level for conflict resolution (higher wins)';
COMMENT ON COLUMN detection_source_priority.base_confidence IS 'Base confidence level for this source (0.00-1.00)';
COMMENT ON COLUMN detection_source_priority.confidence_decay IS 'Decay multiplier for reused detections (e.g., 0.9 means 10% decay)';
COMMENT ON COLUMN detection_source_priority.enabled IS 'Whether this detection source is enabled';
COMMENT ON COLUMN detection_source_priority.description IS 'Description of the detection source';
COMMENT ON COLUMN detection_source_priority.created_at IS 'Timestamp when the configuration was created';
COMMENT ON COLUMN detection_source_priority.updated_at IS 'Timestamp when the configuration was last updated (managed by application)';

-- Insert default detection source priorities
INSERT INTO detection_source_priority (source_name, display_name, priority, base_confidence, confidence_decay, description) VALUES
('user', '用户标注', 100, 1.0, 1.0, '用户手动标注，最高优先级'),
('component', '组件判断', 80, 0.85, 1.0, '组件通过镜像、标签等分析得出'),
('reuse', '复用检测', 75, 0.85, 0.9, '从相似 Workload 复用的结果'),
('log', '日志分析', 60, 0.70, 1.0, '通过日志模式匹配识别'),
('image', '镜像推断', 40, 0.60, 1.0, '仅通过镜像名称推断'),
('default', '默认推断', 20, 0.30, 1.0, '默认或未知情况')
ON CONFLICT (source_name) DO NOTHING;

