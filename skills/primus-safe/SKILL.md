---
name: primus-safe
description: Primus-SaFE (GPU cluster platform on Kubernetes) product knowledge and operations. Use to answer questions about workspaces, workloads, nodes, flavors, opsjobs, API keys, and to list, inspect or troubleshoot them through the SaFE MCP server or REST API.
---

# Primus-SaFE

SaFE manages GPU clusters (Kubernetes + ROCm): it schedules workloads onto workspaces, manages nodes, and records faults. Answer from live API data; never guess IDs.

## Setup

- `SAFE_URL`: base URL of the SaFE deployment, no trailing slash (e.g. `https://<your-safe-host>`). Ask the user or check the site skill if unset.
- `SAFE_API_KEY`: the user's own API key, normally already in the environment. Never print it, never write it to files.
- Auth: `Authorization: Bearer $SAFE_API_KEY` works on every SaFE API (REST and MCP). Actions run as the key's owner. Verify who you are:
  `curl -s -H "Authorization: Bearer $SAFE_API_KEY" "$SAFE_URL/api/v1/users/self"`
- All REST paths are under `/api/v1/`. Lists return `{"totalCount":N,"items":[...]}` and take `limit` / `offset`.

## Concepts

- **Cluster**: a Kubernetes cluster managed by SaFE. `clusterId`.
- **Workspace**: a quota/queue unit inside a cluster: a set of nodes of one flavor, a queue policy, `scopes` (Train, Infer, Authoring, CICD, Sandbox) that say which workload kinds it accepts, and mounted volumes. Users submit workloads into a workspace. `workspaceId`.
- **Workload**: the unit of work. `groupVersionKind.kind`: PyTorchJob (distributed training), TorchFT (fault-tolerant training), Deployment / StatefulSet (serving), Authoring (interactive dev box), AutoscalingRunnerSet (CI runners), Sandbox. Phases: Pending, Running, Succeeded, Failed, Stopped (Updating / NotReady for services). `priority` 0 low / 1 medium / 2 high.
- **Node**: a machine in a cluster, belonging to at most one workspace. `phase` Ready or a failure phase (SSHFailed, ManagedFailed, ...); `available=false` plus a `message` means unschedulable (taints, health-check failure).
- **Flavor** (node flavor): a hardware shape (CPU, memory, GPU product and count, disks, RDMA). A workspace uses one flavor.
- **OpsJob**: an administrative job run by the platform: addon, preflight, reboot, dumplog, exportimage, and similar. Has its own phase and outputs.
- **API key**: per-user credential (`ak-...`); `GET /api/v1/apikeys/current` describes the key in use. The key value is shown only at creation.

## Preferred path: SaFE MCP server

Endpoint (streamable HTTP): `$SAFE_URL/api/v1/safe-mcp/mcp`, header `Authorization: Bearer <API key>`. `GET $SAFE_URL/api/v1/safe-mcp/mcp/info` lists the tool names (about 50): `cluster_*`, `workspace_*`, `workload_*` (list, get, create, stop, delete, update, pod_logs, logs_download), `node_*` (list, get, status, cordon, drain, reboot, ...), `opsjob_*`, `flavor_list/get`, `apikey_*`. If the agent has the MCP tools mounted, call them directly. They take snake_case arguments (`workload_id`, `cluster_id`, `workspace_id`, `phase`, `limit`).

Rules: call `cluster_list` and `flavor_list` before creating workspaces or workloads. Read freely; for anything that stops, deletes, downloads logs (`workload_logs_download`), drains, reboots or creates, state what you will do and get the user's confirmation first.

No MCP mounted? Call the same tools over JSON-RPC:
```
curl -s -H "Authorization: Bearer $SAFE_API_KEY" -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' -X POST "$SAFE_URL/api/v1/safe-mcp/mcp" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workload_list","arguments":{"phase":"Failed","limit":5}}}'
```
(`"method":"tools/list"` returns every tool with its input schema.)

## REST fallbacks (read-only)

`H="Authorization: Bearer $SAFE_API_KEY"`; pipe through `python3 -m json.tool` or `jq` if available.

| Question | Command |
|---|---|
| Who am I, what role | `curl -s -H "$H" "$SAFE_URL/api/v1/users/self"` |
| Workspaces | `curl -s -H "$H" "$SAFE_URL/api/v1/workspaces"` |
| Flavors / free capacity of one | `.../api/v1/nodeflavors`, `.../api/v1/nodeflavors/<flavorId>/avail` |
| Workloads (filters: `workspaceId`, `phase`, `kind`, `userName`, `clusterId`, `limit`) | `curl -s -H "$H" "$SAFE_URL/api/v1/workloads?phase=Failed&limit=5"` |
| One workload (conditions, pods, nodes) | `curl -s -H "$H" "$SAFE_URL/api/v1/workloads/<workloadId>"` |
| Workload events | `curl -s -H "$H" -H 'Content-Type: application/json' -X POST -d '{}' "$SAFE_URL/api/v1/workloads/<id>/events"` (POST, read-only) |
| Pod logs (live pods) | `.../api/v1/workloads/<id>/pods/<podId>/logs?tailLines=100` |
| Workload logs (stored, also after the pod is gone) | `curl -s -H "$H" -H 'Content-Type: application/json' -X POST -d '{"limit":50}' "$SAFE_URL/api/v1/workloads/<id>/logs"` |
| Nodes (filters: `workspaceId`, `clusterId`, `phase`, `available`, `search`, `brief=true`) | `curl -s -H "$H" "$SAFE_URL/api/v1/nodes?available=false&brief=true"` |
| OpsJobs (filters: `phase`, `type`, `workspaceId`, `userName`) | `curl -s -H "$H" "$SAFE_URL/api/v1/opsjobs?phase=Failed&limit=5"` |
| My API keys | `curl -s -H "$H" "$SAFE_URL/api/v1/apikeys"` |

Writes: prefer the MCP tools; REST routes and body fields are in `references/api-cheatsheet.md`.

## Troubleshooting (short form; details in `references/troubleshooting.md`)

1. Get the workload (`workloads/<id>`): read `phase`, top-level `message`, `conditions`, `pods[].containers[].reason/exitCode`, `queuePosition`.
2. Pending: `message` says why (in queue, no capacity, gang placement impossible, workspace has no available nodes). Check `nodes?workspaceId=<ws>&available=false` and `nodeflavors/<flavor>/avail`.
3. Failed: exit code and reason from the pod; then pod logs (live) or stored logs (POST `/logs`).
4. Image pull errors: events and the pod container `reason` (ErrImagePull, ImagePullBackOff): wrong image name/tag, missing registry secret, registry unreachable.
5. Node unhealthy: node `phase`, `available`, `message` (taints from failed health checks); recent opsjobs of type reboot/preflight for that node.

Report what the data shows, name the likely cause, then suggest the fix; do not change anything without confirmation.
