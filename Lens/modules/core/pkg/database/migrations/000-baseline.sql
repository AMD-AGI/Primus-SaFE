-- Baseline schema for primus-lens database
-- This file contains the initial table definitions
-- All subsequent patches build on top of this baseline

-- node table
CREATE TABLE IF NOT EXISTS node
(
    id                 serial PRIMARY KEY,
    name               varchar(65),
    address            varchar(32),
    gpu_name           varchar(128),
    gpu_allocation     integer,
    gpu_count          integer,
    gpu_utilization    double precision,
    status             varchar(64),
    created_at         timestamp with time zone,
    updated_at         timestamp with time zone,
    cpu                varchar(64),
    cpu_count          integer,
    memory             varchar(64),
    k8s_version        varchar(64),
    k8s_status         varchar(64),
    kubelet_version    varchar(64),
    containerd_version varchar(64),
    os                 varchar(64),
    driver_version     varchar(64),
    taints             jsonb
);

-- gpu_device table
CREATE TABLE IF NOT EXISTS gpu_device
(
    id               serial PRIMARY KEY,
    node_id          integer,
    gpu_id           integer,
    gpu_model        varchar(64),
    memory           integer,
    utilization      double precision,
    temperature      double precision,
    power            double precision,
    serial           varchar(128),
    rdma_device_name varchar(64),
    rdma_guid        varchar(64),
    rdma_lid         varchar(64),
    created_at       timestamp with time zone,
    updated_at       timestamp with time zone,
    numa_node        integer,
    numa_affinity    integer
);

-- gpu_pods table
CREATE TABLE IF NOT EXISTS gpu_pods
(
    id            serial PRIMARY KEY,
    namespace     varchar(64),
    name          varchar(64),
    node_name     varchar(64),
    uid           varchar(64),
    gpu_allocated integer,
    phase         varchar(64),
    deleted       boolean,
    created_at    timestamp with time zone,
    updated_at    timestamp with time zone,
    owner_uid     varchar(64),
    running       boolean,
    ip            varchar(32)
);

-- gpu_pods_event table
CREATE TABLE IF NOT EXISTS gpu_pods_event
(
    id            serial PRIMARY KEY,
    pod_uuid      varchar(64),
    pod_phase     varchar(64),
    event_type    varchar(64),
    created_at    timestamp with time zone,
    restart_count integer
);

-- gpu_workload table
CREATE TABLE IF NOT EXISTS gpu_workload
(
    id            serial PRIMARY KEY,
    group_version varchar(128),
    kind          varchar(64),
    namespace     varchar(64),
    name          varchar(64),
    uid           varchar(128),
    parent_uid    varchar(128),
    gpu_request   integer,
    created_at    timestamp with time zone,
    updated_at    timestamp with time zone,
    deleted_at    timestamp with time zone,
    end_at        timestamp with time zone,
    status        varchar(64),
    source        varchar(64),
    labels        jsonb,
    annotations   jsonb
);

-- pod_snapshot table
CREATE TABLE IF NOT EXISTS pod_snapshot
(
    id               serial PRIMARY KEY,
    pod_uid          varchar(64),
    pod_name         varchar(64),
    namespace        varchar(64),
    spec             jsonb,
    metadata         jsonb,
    status           jsonb,
    created_at       timestamp with time zone,
    resource_version integer
);

CREATE INDEX IF NOT EXISTS pod_snapshot_pod_uid_resource_version_index
    ON pod_snapshot (pod_uid ASC, resource_version DESC);

-- gpu_workload_snapshot table
CREATE TABLE IF NOT EXISTS gpu_workload_snapshot
(
    id               serial PRIMARY KEY,
    uid              varchar(64),
    group_version    varchar(128),
    kind             varchar(64),
    name             varchar(64),
    namespace        varchar(64),
    metadata         jsonb,
    detail           jsonb,
    resource_version integer,
    created_at       timestamp with time zone
);

-- workload_pod_reference table
CREATE TABLE IF NOT EXISTS workload_pod_reference
(
    id           serial PRIMARY KEY,
    workload_uid varchar(64),
    pod_uid      varchar(64),
    created_at   timestamp with time zone
);

-- fault table
CREATE TABLE IF NOT EXISTS fault
(
    id       serial PRIMARY KEY,
    code     varchar(64),
    kind     varchar(64),
    name     varchar(64),
    detail   jsonb,
    plugin   varchar(64),
    start_at timestamp with time zone,
    end_at   timestamp with time zone,
    level    integer
);

-- workload_resource table
CREATE TABLE IF NOT EXISTS workload_resource
(
    id               serial PRIMARY KEY,
    workload_uid     varchar(64),
    gpu_time_seconds double precision,
    gpu_request      integer,
    gpu_model        varchar(64),
    pod_count        integer,
    updated_at       timestamp with time zone
);

-- pod_resource table
CREATE TABLE IF NOT EXISTS pod_resource
(
    id            serial PRIMARY KEY,
    uid           varchar(64),
    gpu_model     varchar(64),
    gpu_allocated integer,
    created_at    timestamp with time zone,
    end_at        timestamp with time zone
);

-- node_container table
CREATE TABLE IF NOT EXISTS node_container
(
    id             serial PRIMARY KEY,
    container_id   varchar(128),
    container_name varchar(128),
    pod_uid        varchar(128),
    pod_name       varchar(128),
    pod_namespace  varchar(128),
    status         varchar(128),
    created_at     timestamp with time zone,
    updated_at     timestamp with time zone,
    node_name      varchar(128),
    source         varchar(128)
);

-- node_container_devices table
CREATE TABLE IF NOT EXISTS node_container_devices
(
    id            serial PRIMARY KEY,
    container_id  varchar(64),
    device_type   varchar(64),
    device_name   varchar(64),
    device_no     integer,
    device_uuid   varchar(64),
    created_at    timestamp with time zone,
    restart_count integer,
    updated_at    timestamp with time zone
);

-- node_container_event table
CREATE TABLE IF NOT EXISTS node_container_event
(
    id           serial PRIMARY KEY,
    container_id varchar(64),
    event_type   varchar(64),
    created_at   timestamp with time zone
);

CREATE INDEX IF NOT EXISTS node_container_event_container_id_index
    ON node_container_event (container_id);

-- training_performance table
CREATE TABLE IF NOT EXISTS training_performance
(
    id           serial PRIMARY KEY,
    pod_uuid     varchar(256),
    performance  jsonb,
    iteration    integer,
    created_at   timestamp with time zone,
    serial       integer,
    workload_uid varchar(256)
);

-- ai_workload_metadata table
CREATE TABLE IF NOT EXISTS ai_workload_metadata
(
    id           serial PRIMARY KEY,
    workload_uid varchar(256),
    type         varchar(256),
    framework    varchar(256),
    metadata     jsonb,
    created_at   timestamp with time zone
);

-- workload_event table
CREATE TABLE IF NOT EXISTS workload_event
(
    id                   serial PRIMARY KEY,
    workload_uid         varchar(256),
    type                 varchar(256),
    run_serial           integer,
    created_at           timestamp with time zone,
    pod_uid              varchar(256),
    nearest_workload_uid varchar(256)
);

-- rdma_device table
CREATE TABLE IF NOT EXISTS rdma_device
(
    id         serial PRIMARY KEY,
    node_id    integer,
    ifname     varchar(512),
    node_guid  varchar(256),
    if_index   integer,
    fw         varchar(64),
    node_type  varchar(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

-- node_device_changelog table
CREATE TABLE IF NOT EXISTS node_device_changelog
(
    id          serial PRIMARY KEY,
    node_id     integer,
    node_name   varchar(256),
    device_type varchar(32),
    device_name varchar(256),
    device_uuid varchar(256),
    op          varchar(32),
    created_at  timestamp with time zone
);

-- storage table
CREATE TABLE IF NOT EXISTS storage
(
    id         serial PRIMARY KEY,
    name       varchar(64),
    kind       varchar(64),
    config     jsonb,
    source     varchar(64),
    status     varchar(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);
