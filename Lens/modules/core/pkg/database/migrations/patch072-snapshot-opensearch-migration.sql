-- patch072: Create pod_snapshot_latest for single-row-per-pod upsert pattern.
-- Historical snapshots are now stored in OpenSearch (pod-snapshot-YYYY.MM.DD).
-- The old pod_snapshot table is kept for the migration script to read from.

CREATE TABLE IF NOT EXISTS pod_snapshot_latest (
    id              serial PRIMARY KEY,
    pod_uid         varchar(64) NOT NULL,
    pod_name        varchar(256),
    namespace       varchar(256),
    spec            jsonb,
    metadata        jsonb,
    status          jsonb,
    resource_version integer,
    created_at      timestamp with time zone,
    CONSTRAINT uq_pod_snapshot_latest_pod_uid UNIQUE (pod_uid)
);

CREATE INDEX IF NOT EXISTS idx_pod_snapshot_latest_pod_uid ON pod_snapshot_latest (pod_uid);

-- Seed with the latest row per pod from the old table.
-- ON CONFLICT handles the case where this migration is re-run.
INSERT INTO pod_snapshot_latest (pod_uid, pod_name, namespace, spec, metadata, status, resource_version, created_at)
SELECT DISTINCT ON (pod_uid)
    pod_uid, pod_name, namespace, spec, metadata, status, resource_version, created_at
FROM pod_snapshot
ORDER BY pod_uid, resource_version DESC
ON CONFLICT (pod_uid) DO NOTHING;
