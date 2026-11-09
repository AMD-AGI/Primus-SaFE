---
name: primus-safe
description: Primus-SaFE (GPU cluster platform on Kubernetes) product knowledge and operations. Use to answer questions about workspaces, workloads, nodes, flavors, opsjobs, and to list, inspect, troubleshoot or operate them through the SaFE MCP tools.
---

# Primus-SaFE

SaFE manages GPU clusters (Kubernetes + ROCm): it schedules workloads onto workspaces, manages nodes, and records faults. Answer from live data; never guess IDs.

## How to reach SaFE

Use the SaFE MCP tools (streamable HTTP endpoint `<SAFE_URL>/api/v1/safe-mcp/mcp`, header `Authorization: Bearer <the user's own SaFE API key>`; `GET .../safe-mcp/mcp/info` lists the tool names). Reads and writes alike go through these tools, so nothing in SaFE needs a shell, a script or a sandbox. Every call runs as the owner of the key, with that user's permissions.

- If the tools are not available in this session, say so and give the endpoint; do not fall back to curl.
- Never print the API key or write it to a file. When inspecting workloads or pods, never expand or quote environment variables, secrets or tokens; summarize env only by variable names when needed.
- Arguments are snake_case (`workload_id`, `workspace_id`, `cluster_id`, `phase`, `limit`). List tools take `limit` / `offset` and return `{"totalCount":N,"items":[...]}`.

## Concepts

- **Cluster**: a Kubernetes cluster managed by SaFE. `clusterId`.
- **Workspace**: a quota/queue unit inside a cluster: a set of nodes of one flavor, a queue policy, `scopes` (Train, Infer, Authoring, CICD, Sandbox) that say which workload kinds it accepts, and mounted volumes. Users submit workloads into a workspace. `workspaceId`.
- **Workload**: the unit of work. `groupVersionKind.kind`: PyTorchJob (distributed training), TorchFT (fault-tolerant training), Deployment / StatefulSet (serving), Authoring (interactive dev box), AutoscalingRunnerSet (CI runners), Sandbox. Phases: Pending, Running, Succeeded, Failed, Stopped (Updating / NotReady for services). `priority` 0 low / 1 medium / 2 high.
- **Node**: a machine in a cluster, belonging to at most one workspace. `phase` Ready or a failure phase (SSHFailed, ManagedFailed, ...); `available=false` plus a `message` means unschedulable (taints, health-check failure).
- **Flavor** (node flavor): a hardware shape (CPU, memory, GPU product and count, disks, RDMA). A workspace uses one flavor.
- **OpsJob**: an administrative job run by the platform: addon, preflight, reboot, dumplog, exportimage, and similar. Has its own phase and outputs.

## Which tool answers what

| Question | Tool |
|---|---|
| Who am I, my roles and workspaces | `user_self` |
| Workspaces (filter: `cluster_id`) | `workspace_list`, `workspace_get` |
| Flavors; free capacity of one | `flavor_list`, `flavor_get`, `flavor_avail` |
| Workloads (filters: `workspace_id`, `phase`, `kind`, `user_id`, `cluster_id`, `workload_id`, `limit`) | `workload_list` |
| One workload (conditions, pods, nodes) | `workload_get` |
| Workload events (scheduling, image pulls, restarts) | `workload_events` |
| Live pod logs; the containers of a pod | `workload_pod_logs` (pass `tail_lines`, 100 to 200 is enough), `workload_pod_containers` |
| Stored workload logs, also after the pod is gone | `workload_logs` (filters: `keywords`, `pod_names`, `dispatch_count`, `since`, `until`) |
| Nodes (filters: `workspace_id`, `cluster_id`, `flavor_id`, `phase`, `available`, `brief`) | `node_list`, `node_get`, `node_status`, `node_reboot_logs`, `node_pod_logs` |
| Clusters | `cluster_list`, `cluster_get`, `cluster_logs` |
| OpsJobs (filters: `phase`, `type`, `workspace_id`) | `opsjob_list`, `opsjob_get` |
| Image-pull and SSH secrets (names only, never values) | `secret_list` |

`workload_logs` and `workload_events` return a bounded amount, newest first unless you pass `order` asc; `node_pod_logs` returns the last `tail_lines` lines of the node agent's log in log order, newest last. `truncated: true` means entries were left out, text was cut, or more matched than came back. For `workload_logs` and `workload_events`, narrow with `since` / `until` or page with `offset`; `workload_logs` also takes `keywords` and `pod_names`. For `node_pod_logs`, ask for a recent window (`since_seconds`) or fewer lines; it has no paging.

## Changing things

Writes are tools too: `workload_create`, `workload_update`, `workload_stop`, `workload_resume`, `workload_delete`, `workload_batch_stop`, `workload_batch_delete`; `workspace_create`, `workspace_update`, `workspace_scale`, `workspace_manage_nodes`, `workspace_delete`; `node_create`, `node_update`, `node_cordon`, `node_uncordon`, `node_drain`, `node_reboot`, `node_delete`; `cluster_create`, `cluster_update`, `cluster_manage_nodes`, `cluster_delete`; `opsjob_create`, `opsjob_stop`, `opsjob_delete`. Field reference in `references/api-cheatsheet.md`.

- Call `cluster_list`, `flavor_list` and `workspace_list` before creating a workspace or workload, and use the IDs they return.
- Every write needs the user's approval. A host that holds write tools shows an approval card; in one that does not, ask in chat and wait for a yes. Either way say in one line what you are about to do and to which IDs. If the user denies, nothing was done: say so.
- Never remove, stop or reboot more than what the user named.

## The whole log as a file (the one case that needs the sandbox)

`workload_logs` is for reading in chat. Only when the user wants the complete log of a workload as a file, make the sandbox download it (it creates a log-dump OpsJob and can take minutes, so confirm first). `SAFE_URL` (base URL, no trailing slash) and `SAFE_API_KEY` (the user's own key) are normally in the sandbox environment; ask the user for the URL if it is not, and never print the key:

```
curl -L -o <file> -H "Authorization: Bearer $SAFE_API_KEY" -H 'Content-Type: application/json' -d '{}' "$SAFE_URL/api/v1/workloads/<id>/logs/download"
```

## Troubleshooting (short form; details in `references/troubleshooting.md`)

1. `workload_get`: read `phase`, top-level `message`, `conditions`, `pods[].containers[].reason/exitCode`, `queuePosition`.
2. Pending: `message` says why (in queue, no capacity, gang placement impossible, workspace has no available nodes). Check `node_list` with `workspace_id` and `available=false`, and `flavor_avail`; `workload_events` shows what the scheduler said.
3. Failed: exit code and reason from the pod; then `workload_pod_logs` (live) or `workload_logs` (stored).
4. Image pull errors: `workload_events` and the pod container `reason` (ErrImagePull, ImagePullBackOff): wrong image name/tag, missing registry secret (`secret_list`), registry unreachable.
5. Node unhealthy: `node_get` for `phase`, `available`, `message` (taints from failed health checks); recent `opsjob_list` of type reboot/preflight for that node; `node_pod_logs` for the node agent.

Report what the data shows, name the likely cause, then suggest the fix; do not change anything the user did not ask for.
