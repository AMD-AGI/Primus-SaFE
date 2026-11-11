# SaFE tool and field reference

The SaFE MCP tools (see SKILL.md) take snake_case arguments; the REST API behind them uses camelCase fields, shown in parentheses where they differ. Errors come back as the HTTP status plus SaFE's `errorCode` (404 NotFound, 401/403 auth); the message text is not passed on. List tools take `offset` (default 0) and `limit`.

## Read tools
- `user_self`: the caller (`roles`, `workspaces`, `managedWorkspaces`). `apikey_current`: the key in use, not the user.
- `cluster_list`, `cluster_get`, `cluster_logs`.
- `workspace_list`, `workspace_get`: `flavorId`, `scopes`, `currentNodeCount`, `targetNodeCount`, `phase`, `queuePolicy`, `enablePreempt`.
- `flavor_list`, `flavor_get`, `flavor_avail` (resources still free for that flavor).
- `workload_list` filters: `workspace_id`, `cluster_id`, `user_id`, `phase`, `kind`, `workload_id` (fuzzy), `sort_by`, `order`, `limit`, `offset`.
- `workload_get`: adds `images`, `entryPoints` (Base64), `conditions`, `pods`, `nodes`, `env`.
- `workload_pod_containers`, `workload_pod_logs` (`tail_lines`, `container`, `since_seconds`).
- `workload_logs` and `workload_events` (read-only searches): `since` / `until` (UTC RFC 3339), `limit` (default 100 / 50, at most 200), `offset`, `order` (default `desc`). `workload_logs` also takes `keywords`, `pod_names`, `node_names`, `dispatch_count` and returns `{total, returned, truncated, lines:[{time, message, pod, node, stream, dispatch_count}]}`; long lines are cut and the result is capped in size. `truncated` is true when lines were left out, a message was cut, or more lines matched than returned; `total` is only a lower bound when `totalIsLowerBound` is true.
- `node_list` filters: `workspace_id`, `cluster_id`, `flavor_id`, `node_id`, `phase`, `available`, `brief`, `limit`. `node_get`, `node_status`, `node_reboot_logs`, `node_pod_logs` (the node agent's log: the last `tail_lines` lines, newest last), `node_export`.
- `opsjob_list` filters: `cluster_id`, `workspace_id`, `user_name`, `phase`, `type`, `job_name`, `since`, `until`. `opsjob_get`.
- `secret_list` (`type`, `workspace_id`): id, name, type, workspaces, owner; never a value.
- `apikey_list`.
- `workload_logs_download` is NOT read-only: it creates a log-dump OpsJob and waits for it. Use `workload_logs` to read logs in chat.

## Write tools (each needs the user's approval)
- `workload_create`. Core arguments: `display_name`, `workspace_id`, `kind` (PyTorchJob, TorchFT, Deployment, StatefulSet, Authoring, RayJob, AutoscalingRunnerSet, GithubRunner), `images[]`, `resources[] {replica, cpu, gpu, memory, sharedMemory, ephemeralStorage}`, `entry_points[]` (Base64, index-aligned with `resources`), `priority`, `max_retry`, `timeout`, `env`, `is_tolerate_all`. Look up valid workspace and flavor IDs first.
- `workload_stop`, `workload_resume`, `workload_delete`, `workload_update`, `workload_batch_stop`, `workload_batch_delete` (`workload_ids`).
- `node_update` (labels, taints, flavor), `node_cordon`, `node_uncordon`, `node_drain`, `node_reboot`, `node_create`, `node_delete`; `workspace_manage_nodes` (add/remove nodes), `workspace_update`, `workspace_scale`, `workspace_create`, `workspace_delete`; `cluster_create`, `cluster_update`, `cluster_manage_nodes`, `cluster_delete`.
- `opsjob_create` (types such as addon, preflight, reboot, dumplog), `opsjob_stop`, `opsjob_delete`.
- `apikey_create`, `apikey_delete`.
