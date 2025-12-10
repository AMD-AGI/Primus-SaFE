-- Primus Lens Database Initialization Script
-- This script creates the necessary database schema for Primus Lens

-- Create database if not exists
create database "primus-lens";

-- Grant privileges to user 'primus-lens'
GRANT CONNECT ON DATABASE "primus-lens" TO "primus-lens";
GRANT CREATE, TEMPORARY ON DATABASE "primus-lens" TO "primus-lens";
GRANT USAGE ON SCHEMA public TO "primus-lens";
GRANT CREATE ON SCHEMA public TO "primus-lens";

ALTER SYSTEM SET max_connections = 500;

-- Connect to the database
\c "primus-lens"

-- Grant privileges on the database
GRANT CONNECT ON DATABASE "primus-lens" TO "primus-lens";
GRANT CREATE, TEMPORARY ON DATABASE "primus-lens" TO "primus-lens";
GRANT USAGE ON SCHEMA public TO "primus-lens";
GRANT CREATE ON SCHEMA public TO "primus-lens";

-- Create tables
create table node
(
    id                 serial constraint node_pk primary key,
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

alter table node owner to "primus-lens";

create table gpu_device
(
    id               serial constraint gpu_device_pk primary key,
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

alter table gpu_device owner to "primus-lens";

create table gpu_pods
(
    id            serial constraint gpu_pods_pk primary key,
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

alter table gpu_pods owner to "primus-lens";

create table gpu_pods_event
(
    id            serial constraint gpu_pods_event_pk primary key,
    pod_uuid      varchar(64),
    pod_phase     varchar(64),
    event_type    varchar(64),
    created_at    timestamp with time zone,
    restart_count integer
);

alter table gpu_pods_event owner to "primus-lens";

create table gpu_workload
(
    id            serial constraint gpu_workload_pk primary key,
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

alter table gpu_workload owner to "primus-lens";

create table pod_snapshot
(
    id               serial constraint pod_snapshot_pk primary key,
    pod_uid          varchar(64),
    pod_name         varchar(64),
    namespace        varchar(64),
    spec             jsonb,
    metadata         jsonb,
    status           jsonb,
    created_at       timestamp with time zone,
    resource_version integer
);

alter table pod_snapshot owner to "primus-lens";

create index pod_snapshot_pod_uid_resource_version_index
    on pod_snapshot (pod_uid asc, resource_version desc);

create table gpu_workload_snapshot
(
    id               serial constraint gpu_workload_snapshot_pk primary key,
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

alter table gpu_workload_snapshot owner to "primus-lens";

create table workload_pod_reference
(
    id           serial,
    workload_uid varchar(64),
    pod_uid      varchar(64),
    created_at   timestamp with time zone
);

alter table workload_pod_reference owner to "primus-lens";

create table fault
(
    id       serial constraint fault_pk primary key,
    code     varchar(64),
    kind     varchar(64),
    name     varchar(64),
    detail   jsonb,
    plugin   varchar(64),
    start_at timestamp with time zone,
    end_at   timestamp with time zone,
    level    integer
);

alter table fault owner to "primus-lens";

create table workload_resource
(
    id               serial constraint workload_resource_pk primary key,
    workload_uid     varchar(64),
    gpu_time_seconds double precision,
    gpu_request      integer,
    gpu_model        varchar(64),
    pod_count        integer,
    updated_at       timestamp with time zone
);

alter table workload_resource owner to "primus-lens";

create table pod_resource
(
    id            serial constraint pod_resource_pk primary key,
    uid           varchar(64),
    gpu_model     varchar(64),
    gpu_allocated integer,
    created_at    timestamp with time zone,
    end_at        timestamp with time zone
);

alter table pod_resource owner to "primus-lens";

create table node_container
(
    id             serial constraint node_container_pk primary key,
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

alter table node_container owner to "primus-lens";

create table node_container_devices
(
    id            serial constraint node_container_devices_pk primary key,
    container_id  varchar(64),
    device_type   varchar(64),
    device_name   varchar(64),
    device_no     integer,
    device_uuid   varchar(64),
    created_at    timestamp with time zone,
    restart_count integer,
    updated_at    timestamp with time zone
);

alter table node_container_devices owner to "primus-lens";

create table node_container_event
(
    id           serial constraint node_container_event_pk primary key,
    container_id varchar(64),
    event_type   varchar(64),
    created_at   timestamp with time zone
);

alter table node_container_event owner to "primus-lens";

create index node_container_event_container_id_index
    on node_container_event (container_id);

create table training_performance
(
    id           serial constraint training_performance_pk primary key,
    pod_uuid     varchar(256),
    performance  jsonb,
    iteration    integer,
    created_at   timestamp with time zone,
    serial       integer,
    workload_uid varchar(256)
);

alter table training_performance owner to "primus-lens";

create table ai_workload_metadata
(
    id           serial constraint ai_workload_metadata_pk primary key,
    workload_uid varchar(256),
    type         varchar(256),
    framework    varchar(256),
    metadata     jsonb,
    created_at   timestamp with time zone
);

alter table ai_workload_metadata owner to "primus-lens";

create table workload_event
(
    id                   serial constraint workload_event_pk primary key,
    workload_uid         varchar(256),
    type                 varchar(256),
    run_serial           integer,
    created_at           timestamp with time zone,
    pod_uid              varchar(256),
    nearest_workload_uid varchar(256)
);

alter table workload_event owner to "primus-lens";

create table rdma_device
(
    id         serial constraint rdma_device_pk primary key,
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

alter table rdma_device owner to "primus-lens";

create table node_device_changelog
(
    id          serial constraint node_device_changelog_pk primary key,
    node_id     integer,
    node_name   varchar(256),
    device_type varchar(32),
    device_name varchar(256),
    device_uuid varchar(256),
    op          varchar(32),
    created_at  timestamp with time zone
);

alter table node_device_changelog owner to "primus-lens";

create table storage
(
    id         serial constraint storage_pk primary key,
    name       varchar(64),
    kind       varchar(64),
    config     jsonb,
    source     varchar(64),
    status     varchar(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

alter table storage owner to "primus-lens";

-- Grant ownership of all tables to primus-lens user
DO
$$
    DECLARE
        r RECORD;
    BEGIN
        FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public'
            LOOP
                EXECUTE format('ALTER TABLE public.%I OWNER TO "primus-lens";', r.tablename);
            END LOOP;
    END
$$;

