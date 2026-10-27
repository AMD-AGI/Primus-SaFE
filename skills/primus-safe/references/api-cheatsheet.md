# SaFE API cheatsheet

Conventions: base `$SAFE_URL/api/v1`, `Authorization: Bearer $SAFE_API_KEY`, JSON bodies. Errors are `{"errorCode":"...","errorMessage":"..."}` (404 NotFound, 401/403 auth). List calls take `offset` (default 0) and `limit`.

## Read endpoints
- `GET /users/self`: current user (`roles`, `workspaces`).
- `GET /clusters`, `GET /clusters/<id>`
- `GET /workspaces`, `GET /workspaces/<id>`: `flavorId`, `scopes`, `currentNodeCount`, `targetNodeCount`, `phase`, `queuePolicy`, `enablePreempt`.
- `GET /nodeflavors`, `GET /nodeflavors/<id>`, `GET /nodeflavors/<id>/avail`
- `GET /workloads` filters: `workspaceId`, `phase` (comma-separated), `clusterId`, `userId`, `userName`, `kind` (comma-separated), `description`, `since`, `sortBy`, `order`, `limit`, `offset`.
- `GET /workloads/<id>`: adds `images`, `entryPoints` (Base64), `conditions`, `pods`, `nodes`, `env`.
- `GET /workloads/<id>/pods/<podId>/containers`, `GET /workloads/<id>/pods/<podId>/logs?tailLines=&container=&sinceSeconds=`
- `POST /workloads/<id>/logs`, `/events` (read-only despite POST).
- `POST /workloads/<id>/logs/download` is NOT read-only: it may create a log-dump OpsJob and waits for it. Same for MCP `workload_logs_download`; confirm with the user first.
- `GET /nodes` filters: `workspaceId`, `clusterId`, `flavorId`, `nodeId`, `search`, `available`, `phase`, `brief`, `limit`, `offset`. `GET /nodes/<id>`, `GET /nodes/<id>/reboot/logs`.
- `GET /opsjobs` filters: `clusterId`, `workspaceId`, `userName`, `phase`, `type`, `jobName`, `since`, `until`. `GET /opsjobs/<id>`.
- `GET /apikeys`, `GET /apikeys/current`.

## Write endpoints (confirm with the user first)
- `POST /workloads` create. Core fields: `displayName`, `workspaceId`, `groupVersionKind {version, kind}`, `resources[] {replica, cpu, gpu, memory, sharedMemory, ephemeralStorage}`, `images[]`, `entryPoints[]` (Base64, index-aligned with `resources`), `priority`, `maxRetry`, `timeout`, `env`, `isTolerateAll`. Look up valid workspace and flavor IDs first.
- `POST /workloads/<id>/stop`, `POST /workloads/<id>/resume`, `DELETE /workloads/<id>`, `PATCH /workloads/<id>`, `POST /workloads/stop` and `/delete` (batch).
- `PATCH /nodes/<id>` (labels, taints, flavor), `POST /nodes/delete`, `POST /workspaces/<id>/nodes` (add/remove nodes), `PATCH /workspaces/<id>`.
- `POST /opsjobs` (types such as addon, preflight, reboot, dumplog), `POST /opsjobs/<id>/stop`.

The MCP tools wrap the same operations with typed arguments; when available they are less error-prone than hand-built bodies.
