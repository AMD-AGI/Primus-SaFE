-- profiler_files.sql
-- PyTorch Profiler files metadata and binary storage tables
-- Created: 2024-12-15

CREATE TABLE IF NOT EXISTS profiler_files (
    id SERIAL PRIMARY KEY,
    workload_uid VARCHAR(255) NOT NULL,
    pod_uid VARCHAR(255),
    pod_name VARCHAR(255),
    pod_namespace VARCHAR(255),
    file_name VARCHAR(512) NOT NULL,
    file_path TEXT,
    file_type VARCHAR(50) NOT NULL,
    file_size BIGINT,
    storage_type VARCHAR(20) NOT NULL,
    storage_path TEXT,
    storage_bucket VARCHAR(255),
    download_url TEXT,
    confidence VARCHAR(20),
    source_pid INT,
    detected_at TIMESTAMP,
    collected_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_profiler_files_workload_uid ON profiler_files(workload_uid);
CREATE INDEX IF NOT EXISTS idx_profiler_files_pod_uid ON profiler_files(pod_uid);
CREATE INDEX IF NOT EXISTS idx_profiler_files_file_type ON profiler_files(file_type);
CREATE INDEX IF NOT EXISTS idx_profiler_files_storage_type ON profiler_files(storage_type);
CREATE INDEX IF NOT EXISTS idx_profiler_files_collected_at ON profiler_files(collected_at);
CREATE INDEX IF NOT EXISTS idx_profiler_files_created_at ON profiler_files(created_at);

CREATE TABLE IF NOT EXISTS profiler_file_content (
    id SERIAL PRIMARY KEY,
    profiler_file_id INT NOT NULL,
    content BYTEA NOT NULL,
    content_encoding VARCHAR(20),
    chunk_index INT NOT NULL DEFAULT 0,
    total_chunks INT NOT NULL DEFAULT 1,
    chunk_size BIGINT,
    md5_hash VARCHAR(32),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_profiler_file_content_file_id ON profiler_file_content(profiler_file_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiler_file_content_file_chunk ON profiler_file_content(profiler_file_id, chunk_index);

CREATE TABLE IF NOT EXISTS profiler_analysis (
    id SERIAL PRIMARY KEY,
    profiler_file_id INT,
    workload_uid VARCHAR(255) NOT NULL,
    total_duration_ms FLOAT,
    gpu_utilization_avg FLOAT,
    cpu_utilization_avg FLOAT,
    memory_peak_mb FLOAT,
    memory_allocated_mb FLOAT,
    top_operations JSONB,
    bottlenecks JSONB,
    total_operators INT,
    unique_operators INT,
    analyzed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_profiler_analysis_file_id ON profiler_analysis(profiler_file_id);
CREATE INDEX IF NOT EXISTS idx_profiler_analysis_workload_uid ON profiler_analysis(workload_uid);

