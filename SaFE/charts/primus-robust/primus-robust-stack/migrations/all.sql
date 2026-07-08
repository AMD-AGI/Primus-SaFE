-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- fault-manager database schema initialization

-- Fault events received from agents
CREATE TABLE IF NOT EXISTS fault_events (
    id              BIGSERIAL PRIMARY KEY,
    node_name       VARCHAR(255) NOT NULL,
    error_code      VARCHAR(50)  NOT NULL,
    category        VARCHAR(50)  NOT NULL,
    check_name      VARCHAR(255) NOT NULL,
    source          VARCHAR(50)  NOT NULL,
    exit_code       INT          NOT NULL,
    severity        VARCHAR(20)  NOT NULL,
    output          TEXT,
    duration        VARCHAR(50),
    event_time      TIMESTAMPTZ  NOT NULL,
    received_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    agent_version   VARCHAR(50),
    labels          JSONB,
    resolved        BOOLEAN      NOT NULL DEFAULT FALSE,
    resolved_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_fault_events_node_name ON fault_events (node_name);
CREATE INDEX IF NOT EXISTS idx_fault_events_category ON fault_events (category);
CREATE INDEX IF NOT EXISTS idx_fault_events_event_time ON fault_events (event_time DESC);
CREATE INDEX IF NOT EXISTS idx_fault_events_resolved ON fault_events (resolved) WHERE resolved = FALSE;
CREATE INDEX IF NOT EXISTS idx_fault_events_node_error ON fault_events (node_name, error_code, resolved);

-- Node registry (shared with resource-manager, metrics-manager, schedule-manager)
CREATE TABLE IF NOT EXISTS node_registry (
    node_name        VARCHAR(255) PRIMARY KEY,
    infra_type       VARCHAR(50)  NOT NULL DEFAULT 'kubernetes',
    health_status    VARCHAR(50)  NOT NULL DEFAULT 'healthy',
    manage_status    VARCHAR(50)  NOT NULL DEFAULT 'unmanaged',
    fault_count      INT          NOT NULL DEFAULT 0,
    healthy_streak   INT          NOT NULL DEFAULT 0,
    last_report_at   TIMESTAMPTZ,
    first_seen_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    flavor_id        VARCHAR(255),
    template_id      VARCHAR(255),
    pool_id          VARCHAR(255),
    unschedulable    BOOLEAN      NOT NULL DEFAULT FALSE,
    resources        JSONB,
    labels           JSONB,
    taints           JSONB,
    agent_connected  BOOLEAN      NOT NULL DEFAULT FALSE,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_registry_health ON node_registry (health_status);
CREATE INDEX IF NOT EXISTS idx_node_registry_infra ON node_registry (infra_type);
CREATE INDEX IF NOT EXISTS idx_node_registry_manage ON node_registry (manage_status);
CREATE INDEX IF NOT EXISTS idx_node_registry_pool ON node_registry (pool_id);

-- Action log: records all isolation/recovery operations
CREATE TABLE IF NOT EXISTS action_log (
    id               BIGSERIAL PRIMARY KEY,
    node_name        VARCHAR(255) NOT NULL,
    action_type      VARCHAR(50)  NOT NULL,
    infra_type       VARCHAR(50)  NOT NULL,
    trigger_event_id BIGINT,
    detail           JSONB,
    status           VARCHAR(20)  NOT NULL DEFAULT 'pending',
    error_message    TEXT,
    dry_run          BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_action_log_node ON action_log (node_name);
CREATE INDEX IF NOT EXISTS idx_action_log_status ON action_log (status);
CREATE INDEX IF NOT EXISTS idx_action_log_created ON action_log (created_at DESC);

-- Task queue: remediation tasks dispatched to agents
CREATE TABLE IF NOT EXISTS task_queue (
    id              BIGSERIAL PRIMARY KEY,
    task_id         VARCHAR(100) NOT NULL UNIQUE,
    node_name       VARCHAR(255) NOT NULL,
    action          VARCHAR(50)  NOT NULL,
    params          JSONB,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    trigger_event_id BIGINT,
    result_output   TEXT,
    error_message   TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    dispatched_at   TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_task_queue_node ON task_queue (node_name, status);
CREATE INDEX IF NOT EXISTS idx_task_queue_status ON task_queue (status);

-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Phase B: Faults table — aggregated fault state (counterpart of SaFE Fault CRD)

CREATE TABLE IF NOT EXISTS faults (
    id                BIGSERIAL PRIMARY KEY,
    name              VARCHAR(255) NOT NULL UNIQUE,     -- counterpart of Fault CRD metadata.name
    monitor_id        VARCHAR(255) NOT NULL,            -- counterpart of spec.monitorId (maps to error_code)
    message           TEXT,                              -- counterpart of spec.message
    node_name         VARCHAR(255) NOT NULL,            -- counterpart of spec.node.k8sName
    admin_node_name   VARCHAR(255),                     -- counterpart of spec.node.adminName
    cluster_name      VARCHAR(255),                     -- counterpart of spec.node.clusterName
    action            VARCHAR(255),                     -- counterpart of spec.action
    auto_repair       BOOLEAN NOT NULL DEFAULT TRUE,    -- counterpart of spec.isAutoRepairEnabled
    phase             VARCHAR(50) NOT NULL DEFAULT '',   -- counterpart of status.phase
    trigger_event_id  BIGINT,                           -- FK to fault_events.id
    labels            JSONB,                            -- counterpart of metadata.labels
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ                       -- soft delete
);

CREATE INDEX IF NOT EXISTS idx_faults_node ON faults (node_name);
CREATE INDEX IF NOT EXISTS idx_faults_cluster ON faults (cluster_name);
CREATE INDEX IF NOT EXISTS idx_faults_monitor ON faults (monitor_id);
CREATE INDEX IF NOT EXISTS idx_faults_phase ON faults (phase);
CREATE INDEX IF NOT EXISTS idx_faults_active ON faults (deleted_at) WHERE deleted_at IS NULL;

-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- D1.1: Structured GPU/RDMA device tables + device changelog
-- Consumed by: fault-manager (device report handler), diagapi

CREATE TABLE IF NOT EXISTS gpu_device (
    id          BIGSERIAL PRIMARY KEY,
    node_name   VARCHAR(255) NOT NULL,
    gpu_id      INT NOT NULL,
    gpu_model   VARCHAR(255),
    serial      VARCHAR(255),
    vram_mb     INT,
    pcie_bdf    VARCHAR(50),
    numa_node   INT,
    status      VARCHAR(50) DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_gpu_device UNIQUE (node_name, gpu_id)
);

CREATE INDEX IF NOT EXISTS idx_gpu_device_node ON gpu_device (node_name);
CREATE INDEX IF NOT EXISTS idx_gpu_device_model ON gpu_device (gpu_model);

CREATE TABLE IF NOT EXISTS rdma_device (
    id          BIGSERIAL PRIMARY KEY,
    node_name   VARCHAR(255) NOT NULL,
    ifname      VARCHAR(255) NOT NULL,
    node_guid   VARCHAR(255),
    if_index    INT,
    fw_version  VARCHAR(100),
    node_type   VARCHAR(50),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_rdma_device UNIQUE (node_name, ifname)
);

CREATE INDEX IF NOT EXISTS idx_rdma_device_node ON rdma_device (node_name);

CREATE TABLE IF NOT EXISTS node_device_changelog (
    id          BIGSERIAL PRIMARY KEY,
    node_name   VARCHAR(255) NOT NULL,
    device_type VARCHAR(50) NOT NULL,
    device_id   VARCHAR(255) NOT NULL,
    operation   VARCHAR(20) NOT NULL,
    detail      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ndc_node ON node_device_changelog (node_name);
CREATE INDEX IF NOT EXISTS idx_ndc_created ON node_device_changelog (created_at DESC);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- workload-manager: Unified workload instances table (K8s + Slurm)

CREATE TABLE IF NOT EXISTS workload_instances (
    id              BIGSERIAL PRIMARY KEY,

    -- Identity (unique per infra_type)
    instance_id     VARCHAR(255) NOT NULL,            -- Slurm: job_id, K8s: pod uid
    infra_type      VARCHAR(50)  NOT NULL DEFAULT 'slurm',  -- "slurm" | "kubernetes"
    name            VARCHAR(255) NOT NULL,            -- Job/workload name
    namespace       VARCHAR(255),                     -- K8s: namespace, Slurm: partition

    -- Ownership
    kind            VARCHAR(100),                     -- K8s: Deployment/Job/StatefulSet, Slurm: "batch"/"interactive"
    parent_id       VARCHAR(255),                     -- K8s: owner uid, Slurm: array job id (if any)
    user_name       VARCHAR(255),                     -- Submitting user
    image           TEXT,                             -- K8s: container image, Slurm: singularity/docker image (if any)

    -- Status
    state           VARCHAR(50) NOT NULL,             -- PENDING/RUNNING/COMPLETED/FAILED/CANCELLED/TIMEOUT
    nodes           TEXT,                             -- Allocated node list "node-01,node-02"

    -- GPU resources
    gpu_allocated   INT DEFAULT 0,                    -- Total GPU count (integer, for easy aggregation)
    gpu_model       VARCHAR(255),                     -- GPU model "MI325X" / "MI300X"
    gres            TEXT,                             -- Raw GPU resource string "gpu:mi325x:8" (Slurm specific)

    -- Execution details
    command         TEXT,                             -- Execution command / submit line
    cpus            INT,                              -- CPU count
    memory          VARCHAR(50),                      -- Memory
    start_time      TIMESTAMPTZ,                      -- Start time
    end_time        TIMESTAMPTZ,                      -- End time
    exit_code       INT,                              -- Exit code
    time_limit      INT,                              -- Time limit (seconds)
    work_dir        TEXT,                             -- Working directory

    -- Metadata
    labels          JSONB,                            -- K8s labels / Slurm job tags
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),

    -- Unique constraint: one instance per (infra_type, instance_id)
    CONSTRAINT uq_workload_instance UNIQUE (infra_type, instance_id)
);

CREATE INDEX IF NOT EXISTS idx_wi_state ON workload_instances (state);
CREATE INDEX IF NOT EXISTS idx_wi_infra ON workload_instances (infra_type);
CREATE INDEX IF NOT EXISTS idx_wi_nodes ON workload_instances USING gin (to_tsvector('simple', nodes));
CREATE INDEX IF NOT EXISTS idx_wi_updated ON workload_instances (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_wi_user ON workload_instances (user_name);
CREATE INDEX IF NOT EXISTS idx_wi_namespace ON workload_instances (namespace);

-- Workload snapshots: process tree and GPU binding snapshots captured at job start/end.
-- Enables post-mortem analysis after workloads complete.

CREATE TABLE IF NOT EXISTS workload_snapshots (
    id            BIGSERIAL PRIMARY KEY,
    instance_id   VARCHAR(100) NOT NULL,
    node_name     VARCHAR(255) NOT NULL,
    snapshot_type VARCHAR(50)  NOT NULL,  -- process_tree, gpu_binding
    trigger       VARCHAR(50)  NOT NULL DEFAULT 'job_start',  -- job_start, job_end, manual
    data          JSONB        NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workload_snapshots_instance ON workload_snapshots (instance_id);
CREATE INDEX IF NOT EXISTS idx_workload_snapshots_node ON workload_snapshots (node_name, created_at DESC);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- D2.2: Pod-level tracking for K8s workloads
-- Consumed by: workload-manager, diagapi

CREATE TABLE IF NOT EXISTS workload_pods (
    id              BIGSERIAL PRIMARY KEY,
    workload_id     VARCHAR(255) NOT NULL,
    pod_uid         VARCHAR(255) NOT NULL,
    pod_name        VARCHAR(255) NOT NULL,
    node_name       VARCHAR(255),
    namespace       VARCHAR(255),
    phase           VARCHAR(50),
    gpu_allocated   INT DEFAULT 0,
    container_image TEXT,
    start_at        TIMESTAMPTZ,
    end_at          TIMESTAMPTZ,
    labels          JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_workload_pod UNIQUE (workload_id, pod_uid)
);

CREATE INDEX IF NOT EXISTS idx_wp_workload ON workload_pods (workload_id);
CREATE INDEX IF NOT EXISTS idx_wp_pod ON workload_pods (pod_uid);
CREATE INDEX IF NOT EXISTS idx_wp_node ON workload_pods (node_name);
CREATE INDEX IF NOT EXISTS idx_wp_phase ON workload_pods (phase);

CREATE TABLE IF NOT EXISTS workload_pod_devices (
    id          BIGSERIAL PRIMARY KEY,
    pod_uid     VARCHAR(255) NOT NULL,
    container_id VARCHAR(255),
    device_type VARCHAR(50) NOT NULL DEFAULT 'gpu',
    device_id   VARCHAR(255) NOT NULL,
    bound_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wpd_pod ON workload_pod_devices (pod_uid);
CREATE INDEX IF NOT EXISTS idx_wpd_device ON workload_pod_devices (device_id);

CREATE TABLE IF NOT EXISTS pod_running_periods (
    id          BIGSERIAL PRIMARY KEY,
    pod_uid     VARCHAR(255) NOT NULL,
    gpu_count   INT DEFAULT 0,
    start_at    TIMESTAMPTZ NOT NULL,
    end_at      TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_prp_pod ON pod_running_periods (pod_uid);
CREATE INDEX IF NOT EXISTS idx_prp_range ON pod_running_periods (start_at, end_at);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- workload_execution: Async operations attached to workloads (RCA, coredump, profiling, etc.)

CREATE TABLE IF NOT EXISTS workload_execution (
    id              BIGSERIAL PRIMARY KEY,

    -- Workload reference
    workload_id     VARCHAR(255) NOT NULL,

    -- Execution type & trigger
    exec_type       VARCHAR(50)  NOT NULL,                -- rca / coredump / profiling / health_check / ...
    trigger_type    VARCHAR(50)  NOT NULL DEFAULT 'auto', -- auto / manual

    -- Status lifecycle: pending -> running -> completed | failed
    status          VARCHAR(50)  NOT NULL DEFAULT 'pending',

    -- Results (type-specific)
    result          JSONB,           -- structured result (e.g. rca_summary.json content for RCA)
    summary         TEXT,            -- one-line human-readable conclusion
    report          TEXT,            -- detailed report (markdown)
    error_message   TEXT,            -- error details if the execution itself failed

    -- Extensible metadata per exec_type
    metadata        JSONB,           -- tokens_used, iterations, duration_sec, model_used, affected_nodes, ...

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

-- Only one active (pending/running) execution per workload + type at a time.
-- Completed/failed records are not constrained, allowing re-runs.
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_exec_active
    ON workload_execution(workload_id, exec_type)
    WHERE status IN ('pending', 'running');

CREATE INDEX IF NOT EXISTS idx_workload_exec_workload ON workload_execution(workload_id);
CREATE INDEX IF NOT EXISTS idx_workload_exec_type_status ON workload_execution(exec_type, status);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Lens data plane integration: add owner-level metadata to workload tables.
--
-- workload_instances.labels: semantics changes from pod-level to owner-level
-- workload_instances.annotations: NEW - owner-level annotations (JSONB)
-- workload_instances.details: NEW - owner spec subset (JSONB)
-- workload_pods.annotations: NEW - pod-level annotations (JSONB)

ALTER TABLE workload_instances ADD COLUMN IF NOT EXISTS annotations JSONB;
ALTER TABLE workload_instances ADD COLUMN IF NOT EXISTS details JSONB;

CREATE INDEX IF NOT EXISTS idx_wi_annotations ON workload_instances USING gin (annotations);
CREATE INDEX IF NOT EXISTS idx_wi_labels ON workload_instances USING gin (labels);

ALTER TABLE workload_pods ADD COLUMN IF NOT EXISTS annotations JSONB;
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Training performance data (unified model for log/wandb/tensorflow sources)
-- Consumed by: training-analyzer, aggregator, API

CREATE TABLE IF NOT EXISTS training_performance (
    id              BIGSERIAL PRIMARY KEY,
    workload_id     VARCHAR(255) NOT NULL,
    node_name       VARCHAR(255),
    data_source     VARCHAR(50) NOT NULL DEFAULT 'log',  -- "log" / "wandb" / "tensorflow"
    iteration       INT NOT NULL DEFAULT 0,
    serial          INT NOT NULL DEFAULT 0,              -- run serial for multi-run workloads
    performance     JSONB,                               -- dynamic metric key-value pairs
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_training_perf UNIQUE (workload_id, data_source, serial, iteration)
);

CREATE INDEX IF NOT EXISTS idx_tp_workload ON training_performance (workload_id);
CREATE INDEX IF NOT EXISTS idx_tp_source ON training_performance (data_source);
CREATE INDEX IF NOT EXISTS idx_tp_created ON training_performance (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tp_workload_source ON training_performance (workload_id, data_source);

-- Training log pattern rules (DB-driven configurable regex engine)
-- Consumed by: training-analyzer logparser module

CREATE TABLE IF NOT EXISTS training_log_pattern (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(255),
    pattern         TEXT NOT NULL,                        -- regex with named capture groups
    pattern_type    VARCHAR(50) NOT NULL,                 -- "performance" / "training_event" / "checkpoint_event" / "blacklist"
    event_subtype   VARCHAR(100),                         -- e.g. "start_training", "save", "load"
    framework       VARCHAR(100),                         -- informational, does not affect matching
    priority        INT NOT NULL DEFAULT 0,               -- higher = matched first
    confidence      FLOAT NOT NULL DEFAULT 1.0,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    hit_count       BIGINT NOT NULL DEFAULT 0,            -- runtime hit statistics
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tlp_type ON training_log_pattern (pattern_type);
CREATE INDEX IF NOT EXISTS idx_tlp_enabled ON training_log_pattern (enabled) WHERE enabled = TRUE;

-- Training events (start/end training, checkpoint save/load)
-- Consumed by: training-analyzer

CREATE TABLE IF NOT EXISTS training_events (
    id                  BIGSERIAL PRIMARY KEY,
    workload_id         VARCHAR(255) NOT NULL,
    node_name           VARCHAR(255),
    event_type          VARCHAR(100) NOT NULL,  -- "StartTrain", "EndTrain", "CheckpointSave", "CheckpointLoad"
    serial              INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_te_workload ON training_events (workload_id);
CREATE INDEX IF NOT EXISTS idx_te_type ON training_events (event_type);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Workload intent analysis (unified Detection + Intent from Lens ai-advisor)
-- Consumed by: training-analyzer advisor module, diagapi

CREATE TABLE IF NOT EXISTS workload_intent (
    workload_id    VARCHAR(255) PRIMARY KEY,
    evidence       JSONB NOT NULL DEFAULT '{}',
    result         JSONB,
    status         VARCHAR(50) NOT NULL DEFAULT 'pending',
    layers_used    JSONB DEFAULT '[]',
    confidence     REAL NOT NULL DEFAULT 0,
    analysis_mode  VARCHAR(50),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wi_status ON workload_intent (status);
CREATE INDEX IF NOT EXISTS idx_wi_updated ON workload_intent (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_wi_confidence ON workload_intent (confidence) WHERE confidence > 0;

-- Hyperparameters collected from multiple sources (TensorBoard, WandB, config, cmdline)
-- Consumed by: training-analyzer advisor module, diagapi

CREATE TABLE IF NOT EXISTS workload_hyperparameters (
    id           BIGSERIAL PRIMARY KEY,
    workload_id  VARCHAR(255) NOT NULL,
    source       VARCHAR(50) NOT NULL,
    category     VARCHAR(100),
    params       JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_whp_workload ON workload_hyperparameters (workload_id);
CREATE INDEX IF NOT EXISTS idx_whp_source ON workload_hyperparameters (workload_id, source);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- D3.1: Workspace resource grouping
-- Consumed by: workspace sync module, diagapi

CREATE TABLE IF NOT EXISTS workspace (
    name        VARCHAR(255) PRIMARY KEY,
    gpu_model   VARCHAR(255),
    gpu_quota   INT DEFAULT 0,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workspace_node_mapping (
    id              BIGSERIAL PRIMARY KEY,
    workspace_name  VARCHAR(255) NOT NULL,
    node_name       VARCHAR(255) NOT NULL,
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_ws_node UNIQUE (workspace_name, node_name)
);

CREATE INDEX IF NOT EXISTS idx_wnm_workspace ON workspace_node_mapping (workspace_name);
CREATE INDEX IF NOT EXISTS idx_wnm_node ON workspace_node_mapping (node_name);

CREATE TABLE IF NOT EXISTS workspace_node_mapping_history (
    id              BIGSERIAL PRIMARY KEY,
    workspace_name  VARCHAR(255) NOT NULL,
    node_name       VARCHAR(255) NOT NULL,
    action          VARCHAR(20) NOT NULL,
    start_at        TIMESTAMPTZ NOT NULL,
    end_at          TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wnmh_workspace ON workspace_node_mapping_history (workspace_name);
CREATE INDEX IF NOT EXISTS idx_wnmh_range ON workspace_node_mapping_history (start_at, end_at);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- D4.1: GPU aggregation pre-computation tables
-- Consumed by: aggregator module, diagapi

CREATE TABLE IF NOT EXISTS gpu_hourly_stats (
    id              BIGSERIAL PRIMARY KEY,
    dimension_type  VARCHAR(50) NOT NULL,
    dimension_value VARCHAR(255) NOT NULL,
    stat_hour       TIMESTAMPTZ NOT NULL,
    total_gpus      INT DEFAULT 0,
    allocated_gpus  INT DEFAULT 0,
    allocation_rate REAL DEFAULT 0,
    avg_utilization REAL DEFAULT 0,
    p50_utilization REAL DEFAULT 0,
    p90_utilization REAL DEFAULT 0,
    p95_utilization REAL DEFAULT 0,
    total_gpu_hours REAL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_gpu_hourly UNIQUE (dimension_type, dimension_value, stat_hour)
);

CREATE INDEX IF NOT EXISTS idx_ghs_dimension ON gpu_hourly_stats (dimension_type, dimension_value);
CREATE INDEX IF NOT EXISTS idx_ghs_hour ON gpu_hourly_stats (stat_hour DESC);

CREATE TABLE IF NOT EXISTS cluster_overview_cache (
    id                BIGSERIAL PRIMARY KEY,
    total_nodes       INT DEFAULT 0,
    total_gpus        INT DEFAULT 0,
    allocated_gpus    INT DEFAULT 0,
    running_workloads INT DEFAULT 0,
    healthy_nodes     INT DEFAULT 0,
    faulted_nodes     INT DEFAULT 0,
    avg_utilization   REAL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workload_statistic (
    workload_id     VARCHAR(255) PRIMARY KEY,
    avg_utilization REAL DEFAULT 0,
    p50_utilization REAL DEFAULT 0,
    p90_utilization REAL DEFAULT 0,
    p95_utilization REAL DEFAULT 0,
    gpu_hours       REAL DEFAULT 0,
    stat_start      TIMESTAMPTZ,
    stat_end        TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_updated ON workload_statistic (updated_at DESC);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- D5.1: Alert system tables
-- Consumed by: telemetry-gateway (alert receiver), diagapi

CREATE TABLE IF NOT EXISTS alert_events (
    id          BIGSERIAL PRIMARY KEY,
    alert_name  VARCHAR(255) NOT NULL,
    severity    VARCHAR(20) NOT NULL DEFAULT 'warning',
    status      VARCHAR(20) NOT NULL DEFAULT 'firing',
    source      VARCHAR(50) NOT NULL DEFAULT 'metric',
    workload_id VARCHAR(255),
    node_name   VARCHAR(255),
    message     TEXT,
    labels      JSONB DEFAULT '{}',
    fired_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ae_status ON alert_events (status);
CREATE INDEX IF NOT EXISTS idx_ae_severity ON alert_events (severity);
CREATE INDEX IF NOT EXISTS idx_ae_node ON alert_events (node_name);
CREATE INDEX IF NOT EXISTS idx_ae_workload ON alert_events (workload_id);
CREATE INDEX IF NOT EXISTS idx_ae_fired ON alert_events (fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_ae_name_status ON alert_events (alert_name, status);

CREATE TABLE IF NOT EXISTS alert_rules (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    type        VARCHAR(20) NOT NULL,
    severity    VARCHAR(20) NOT NULL DEFAULT 'warning',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    config      JSONB NOT NULL DEFAULT '{}',
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ar_type ON alert_rules (type);
CREATE INDEX IF NOT EXISTS idx_ar_enabled ON alert_rules (enabled) WHERE enabled = TRUE;

CREATE TABLE IF NOT EXISTS alert_silences (
    id          BIGSERIAL PRIMARY KEY,
    rule_name   VARCHAR(255) NOT NULL,
    reason      TEXT,
    start_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_at      TIMESTAMPTZ NOT NULL,
    created_by  VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_as_rule ON alert_silences (rule_name);
CREATE INDEX IF NOT EXISTS idx_as_active ON alert_silences (start_at, end_at);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- P5 Aggregator: GPU utilization cache + aggregation state
-- Depends on: 007_gpu_aggregation.sql (gpu_hourly_stats, cluster_overview_cache, workload_statistic)

-- Four-level GPU utilization cache (realtime / 1h / 6h / 24h)
CREATE TABLE IF NOT EXISTS gpu_util_cache (
    workload_id  VARCHAR(255) NOT NULL,
    "window"     VARCHAR(10)  NOT NULL,  -- 'realtime' / '1h' / '6h' / '24h'
    utilization  FLOAT,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workload_id, "window")
);

CREATE INDEX IF NOT EXISTS idx_guc_updated ON gpu_util_cache (updated_at DESC);

-- Aggregation state tracking (persists last_processed_hour across restarts)
CREATE TABLE IF NOT EXISTS aggregation_state (
    key        VARCHAR(100) PRIMARY KEY,
    value      VARCHAR(255) NOT NULL,
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Log pattern auto-discovery support
-- Consumed by: training-analyzer logdiscovery module

-- Track the origin of patterns (manual vs auto-discovered)
ALTER TABLE training_log_pattern ADD COLUMN IF NOT EXISTS source VARCHAR(50) NOT NULL DEFAULT 'manual';
CREATE INDEX IF NOT EXISTS idx_tlp_source ON training_log_pattern (source);

-- Scan state tracking to avoid re-processing
CREATE TABLE IF NOT EXISTS log_discovery_scan_state (
    id              BIGSERIAL PRIMARY KEY,
    workload_id     VARCHAR(255) NOT NULL,
    last_scan_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    unmatched_count INT NOT NULL DEFAULT 0,
    patterns_created INT NOT NULL DEFAULT 0,

    CONSTRAINT uq_scan_state_workload UNIQUE (workload_id)
);

CREATE INDEX IF NOT EXISTS idx_ldss_workload ON log_discovery_scan_state (workload_id);
CREATE INDEX IF NOT EXISTS idx_ldss_last_scan ON log_discovery_scan_state (last_scan_at DESC);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Workspace node change tracking: records node added/removed events.

CREATE TABLE IF NOT EXISTS workspace_node_changelog (
    id              BIGSERIAL PRIMARY KEY,
    workspace_name  TEXT NOT NULL,
    node_name       TEXT NOT NULL,
    operation       TEXT NOT NULL,  -- 'added' / 'removed'
    detail          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_changelog_ws
    ON workspace_node_changelog (workspace_name, created_at DESC);
-- Alert system extensions for Lens data plane compatibility.

ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS labels JSONB;
CREATE INDEX IF NOT EXISTS idx_ae_labels ON alert_events USING gin (labels);

CREATE TABLE IF NOT EXISTS alert_rule_templates (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    severity    VARCHAR(50) NOT NULL DEFAULT 'warning',
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_rule_advices (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    severity    VARCHAR(50) NOT NULL DEFAULT 'warning',
    type        VARCHAR(50) NOT NULL DEFAULT 'metric',
    config      JSONB NOT NULL DEFAULT '{}',
    reason      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notification_channels (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    type        VARCHAR(50) NOT NULL,
    config      JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nc_type ON notification_channels (type);
-- gpu_workload: stores workload CRD information synced from the management plane.
-- Previously written by Lens adapter, now received via POST /api/v1/workloads/sync.
CREATE TABLE IF NOT EXISTS gpu_workload (
    id              BIGSERIAL PRIMARY KEY,
    uid             TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    workspace       TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'Workload',
    phase           TEXT NOT NULL DEFAULT '',
    gpu_request     INTEGER NOT NULL DEFAULT 0,
    parent_uid      TEXT,
    parent_workload_label TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ,
    end_at          TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_gpu_workload_workspace ON gpu_workload (workspace);
CREATE INDEX IF NOT EXISTS idx_gpu_workload_phase ON gpu_workload (phase) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_gpu_workload_parent_uid ON gpu_workload (parent_uid) WHERE parent_uid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_gpu_workload_parent_label ON gpu_workload (parent_workload_label) WHERE parent_workload_label != '';

-- workload_pod_reference: maps workload UIDs to pod UIDs with node placement.
-- Used for parent/child pod ref aggregation.
CREATE TABLE IF NOT EXISTS workload_pod_reference (
    id              BIGSERIAL PRIMARY KEY,
    workload_uid    TEXT NOT NULL,
    pod_uid         TEXT NOT NULL,
    pod_name        TEXT NOT NULL DEFAULT '',
    pod_namespace   TEXT NOT NULL DEFAULT '',
    node_name       TEXT NOT NULL DEFAULT '',
    gpu_count       INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workload_uid, pod_uid)
);

CREATE INDEX IF NOT EXISTS idx_workload_pod_ref_workload ON workload_pod_reference (workload_uid);

-- namespace_info: workspace metadata maintained by the node reconciler.
CREATE TABLE IF NOT EXISTS namespace_info (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    gpu_model       TEXT NOT NULL DEFAULT '',
    gpu_resource    TEXT NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- node_namespace_mapping: tracks which nodes belong to which namespace/workspace.
CREATE TABLE IF NOT EXISTS node_namespace_mapping (
    id              BIGSERIAL PRIMARY KEY,
    node_name       TEXT NOT NULL,
    namespace_name  TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (node_name, namespace_name)
);

CREATE INDEX IF NOT EXISTS idx_node_ns_mapping_ns ON node_namespace_mapping (namespace_name);

-- node_namespace_mapping_history: historical record of node-namespace assignments.
CREATE TABLE IF NOT EXISTS node_namespace_mapping_history (
    id              BIGSERIAL PRIMARY KEY,
    node_name       TEXT NOT NULL,
    namespace_name  TEXT NOT NULL,
    action          TEXT NOT NULL DEFAULT 'assign',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_ns_history_node ON node_namespace_mapping_history (node_name);
CREATE INDEX IF NOT EXISTS idx_node_ns_history_ns ON node_namespace_mapping_history (namespace_name);

-- nodes: K8s node inventory with labels (workspace/namespace mapping source).
-- Consumed by robust-api node-reconciler to populate namespace_info and
-- node_namespace_mapping. Producer is expected to be a node-label syncer
-- (e.g. workload-manager node informer) that upserts one row per node.
CREATE TABLE IF NOT EXISTS nodes (
    id              BIGSERIAL PRIMARY KEY,
    node_name       TEXT NOT NULL UNIQUE,
    labels          JSONB,
    annotations     JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_nodes_updated ON nodes (updated_at DESC);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

CREATE TABLE IF NOT EXISTS intent_flywheel_task (
    id          BIGSERIAL PRIMARY KEY,
    workload_id VARCHAR(255) NOT NULL,
    task_type   VARCHAR(50) NOT NULL,
    status      VARCHAR(50) NOT NULL DEFAULT 'pending',
    payload     JSONB,
    result      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_flywheel_status ON intent_flywheel_task (status);
CREATE INDEX IF NOT EXISTS idx_flywheel_workload ON intent_flywheel_task (workload_id);

-- =====================================================================
-- ADR 0003: GPU observability five-layer model.
-- New generic tables (device / pod_device / workload /
-- workload_instance_reference). They coexist with the legacy
-- gpu_device / rdma_device / workload_pod_devices / gpu_workload /
-- workload_pod_reference tables during migration; dedup at the end.
-- Tables + indexes only (no FK / view / trigger).
-- =====================================================================

-- device: generic physical device truth (gpu | rdma | pcie | rack ...)
CREATE TABLE IF NOT EXISTS device (
    id          BIGSERIAL PRIMARY KEY,
    node_name   TEXT NOT NULL,
    device_type TEXT NOT NULL,                 -- 'gpu' | 'rdma' | 'pcie' | 'rack' ...
    address     TEXT NOT NULL,                 -- PCIe: BDF; otherwise device-class locator
    serial      TEXT,
    uuid        TEXT,
    model       TEXT,
    vendor      TEXT,
    numa_node   INT,
    attributes  JSONB,                         -- type-specific: gpu{index,vram_mb}, rdma{ifname,guid} ...
    status      TEXT NOT NULL DEFAULT 'active',
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_device UNIQUE (node_name, device_type, address)
);
CREATE INDEX IF NOT EXISTS idx_device_node_type ON device (node_name, device_type);

-- pod_device: real pod -> device association (address-anchored)
CREATE TABLE IF NOT EXISTS pod_device (
    id           BIGSERIAL PRIMARY KEY,
    pod_uid      TEXT NOT NULL,
    container_id TEXT NOT NULL DEFAULT '',
    node_name    TEXT NOT NULL,
    device_id    BIGINT NOT NULL,              -- references device.id (logical, no FK)
    address      TEXT NOT NULL,                -- denormalized device.address; the metric-join anchor
    start_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_at       TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_pod_device UNIQUE (pod_uid, device_id)
);
CREATE INDEX IF NOT EXISTS idx_pod_device_node_addr ON pod_device (node_name, address);
CREATE INDEX IF NOT EXISTS idx_pod_device_active ON pod_device (pod_uid) WHERE end_at IS NULL;

-- workload: multi-level workload entities
CREATE TABLE IF NOT EXISTS workload (
    id          BIGSERIAL PRIMARY KEY,
    uid         TEXT NOT NULL,
    infra_type  TEXT NOT NULL,                 -- platform: 'kubernetes' | 'slurm'
    kind        TEXT,                          -- object type: PyTorchJob/Job/RayCluster/Deployment/Workload | batch/interactive
    name        TEXT,
    namespace   TEXT,
    parent_uid  TEXT,                          -- immediate parent ('' / NULL if root)
    root_uid    TEXT,                          -- top ancestor (convenience for roll-up)
    user_name   TEXT,
    labels      JSONB,
    annotations JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT uq_workload UNIQUE (uid)
);
CREATE INDEX IF NOT EXISTS idx_workload_parent ON workload (parent_uid);
CREATE INDEX IF NOT EXISTS idx_workload_ns ON workload (namespace);

-- workload_instance_reference: instance -> every ancestor workload (flat mapping)
CREATE TABLE IF NOT EXISTS workload_instance_reference (
    id           BIGSERIAL PRIMARY KEY,
    infra_type   TEXT NOT NULL,
    instance_id  TEXT NOT NULL,                -- pod UID (k8s) / job id (slurm)
    workload_uid TEXT NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_wir UNIQUE (infra_type, instance_id, workload_uid)
);
CREATE INDEX IF NOT EXISTS idx_wir_workload ON workload_instance_reference (workload_uid);
-- Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
-- Node GPU capacity + single active workspace association per node.
-- Producer: workload-manager k8s_watcher node informer (upserts nodes + the
-- workspace_node_mapping association via a label strategy). Capacity feeds the
-- per-workspace GPU occupancy-rate metric.

-- GPU configuration columns on the node inventory (labels/annotations already
-- present as JSONB). gpu_count is the per-node total GPU capacity.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS gpu_count    INT  NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS gpu_model    TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS gpu_resource TEXT NOT NULL DEFAULT '';

-- A node belongs to at most one workspace at a time. Drop any legacy duplicate
-- rows (keep the newest) so the unique index below can be created, then enforce
-- one active association per node.
DELETE FROM workspace_node_mapping a
USING workspace_node_mapping b
WHERE a.node_name = b.node_name AND a.id < b.id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_wnm_node_active ON workspace_node_mapping (node_name);
