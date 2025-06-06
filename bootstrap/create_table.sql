create table if not exists workload
(
    id SERIAL PRIMARY KEY,
    workload_id VARCHAR(64) not null,
    display_name VARCHAR(64) not null,
    workspace VARCHAR(64) not null,
    cluster VARCHAR(64) not null,
    resource VARCHAR(1024) not null,
    image VARCHAR(128) not null,
    entrypoint TEXT not null,
    gvk VARCHAR(128) not null,
    phase VARCHAR(128),
    username VARCHAR(128),
    create_time TIMESTAMP,
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    delete_time TIMESTAMP,
    is_supervised boolean,
    is_deleted boolean,
    is_tolerate_all boolean,
    priority INT,
    max_retry INT,
    scheduler_order INT,
    dispatch_count int,
    ttl_second int,
    timeout int,
    env TEXT,
    description TEXT,
    pods TEXT,
    nodes TEXT,
    conditions TEXT,
    customer_labels TEXT,
    service TEXT,
    liveness VARCHAR(128),
    readiness VARCHAR(128)
);

create
    index T_WORKLOAD_ID_INDEX
    on workload (workload_id);

create
    index T_WORKLOAD_CREATE_TIME_INDEX
    on workload (create_time);

create
    index T_WORKLOAD_DISPATCH_CNT_INDEX
    on workload (dispatch_count);

create
    index T_WORKLOAD_WORKSPACE_INDEX
    on workload (workspace);

create
    index T_WORKLOAD_ENDTIME_INDEX
    on workload (end_time);

alter table workload OWNER TO "primus-safe";

create table if not exists fault
(
    id SERIAL PRIMARY KEY,
    uuid VARCHAR(36) not null,
    fault_id VARCHAR(128) not null,
    monitor_id VARCHAR(128) not null,
    message VARCHAR(1024),
    node VARCHAR(64),
    action VARCHAR(16),
    phase VARCHAR(16),
    cluster VARCHAR(64),
    create_time TIMESTAMP,
    update_time TIMESTAMP,
    delete_time TIMESTAMP,
    is_auto_repaired boolean
);

create
    index T_FAULT_CREATE_TIME_INDEX
    on fault (create_time);

create
    index T_FAULT_UUID_INDEX
    on fault (uuid);

alter table fault OWNER TO "primus-safe";

