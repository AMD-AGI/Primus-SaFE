-- patch066-autovacuum_tuning.sql
-- Tune autovacuum parameters for frequently-updated tables to prevent bloat.
--
-- Problem:
--   workload_statistic: Updated every 60s per active workload. With ~30 workloads,
--     this generates ~1800 dead tuples/hour. Table grew to 17GB for 33 live rows.
--   workload_task_state: Updated every 10-30s for heartbeats, lock operations,
--     and status changes. Table grew to 11GB for 37K rows.
--
-- Solution:
--   Set aggressive autovacuum thresholds so dead tuples are cleaned up
--   before they accumulate into significant bloat.

-- workload_statistic: very few rows, updated very frequently
-- Trigger autovacuum after just 50 dead tuples (instead of default 50 + 20% of live rows)
ALTER TABLE workload_statistic SET (
    autovacuum_vacuum_threshold = 50,
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_analyze_threshold = 20,
    autovacuum_analyze_scale_factor = 0.01,
    autovacuum_vacuum_cost_delay = 2
);

-- workload_task_state: many rows, frequently updated (heartbeats, status changes)
-- Trigger autovacuum after 200 dead tuples or 2% of live rows
ALTER TABLE workload_task_state SET (
    autovacuum_vacuum_threshold = 200,
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_analyze_threshold = 100,
    autovacuum_analyze_scale_factor = 0.02,
    autovacuum_vacuum_cost_delay = 2
);

-- gpu_pods: very few rows but extremely high seq_scan count (8M+ scans for 16 rows).
-- This indicates frequent queries without proper index usage.
-- Adding indexes on commonly queried columns to reduce seq_scans.
CREATE INDEX IF NOT EXISTS idx_gpu_pods_node_name ON gpu_pods (node_name);
CREATE INDEX IF NOT EXISTS idx_gpu_pods_namespace ON gpu_pods (namespace);
CREATE INDEX IF NOT EXISTS idx_gpu_pods_owner_uid ON gpu_pods (owner_uid);
CREATE INDEX IF NOT EXISTS idx_gpu_pods_running ON gpu_pods (running) WHERE running = true;
CREATE INDEX IF NOT EXISTS idx_gpu_pods_deleted ON gpu_pods (deleted) WHERE deleted = false;
