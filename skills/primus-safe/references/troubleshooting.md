# SaFE troubleshooting recipes

Always start from the workload detail: `GET /workloads/<id>` (or MCP `workload_get`). Fields that matter: `phase`, `message`, `conditions[]` (AdminScheduled, AdminDispatched, K8sRunning, K8sFailed, ...), `pods[]` (`phase`, `adminNodeName`, `containers[].reason`, `containers[].exitCode`), `queuePosition`, `dispatchCount`.

## Workload Pending
1. `message` names the stage: waiting in the queue, waiting for capacity, or not schedulable.
2. Queue: `queuePosition` > 0 means other workloads are ahead in the workspace; priority (0/1/2) and the workspace `queuePolicy` / `enablePreempt` decide order.
3. Capacity: compare requested `resources` (gpu x replica) with `GET /nodeflavors/<flavorId>/avail` and with `GET /nodes?workspaceId=<ws>&available=true&brief=true`. A multi-node job needs all replicas placed at once (gang scheduling): reduce replicas or wait.
4. Workspace has `currentNodeCount` 0 or fewer than `targetNodeCount`: nodes are not yet added; an admin has to add them.
5. Nodes unavailable (taints): `GET /nodes?workspaceId=<ws>&available=false` and read each `message`. For a functional test on tainted hardware, a workload may be submitted with `isTolerateAll: true`.
6. The console phase can lag the real pod state; confirm with `pods[].phase`.

## Workload Failed
1. `message` and the `K8sFailed` condition usually include pod, node, container, reason and exit code.
2. Exit code 137 / reason OOMKilled: raise `memory` (and `sharedMemory` for data loaders). Other non-zero codes: the application failed; read the logs.
3. Logs: live pod `GET /workloads/<id>/pods/<podId>/logs?tailLines=200`; after the pod is gone use `POST /workloads/<id>/logs`.
4. Multi-node NCCL/RCCL connection errors: network interface or RDMA device names (`NCCL_SOCKET_IFNAME`, `NCCL_IB_HCA`) differ from the cluster default for that node type; set them per job via `env`.
5. `dispatchCount` > 1 or `maxRetry` > 0: the job was retried/failed over; training resumes only from checkpoints written to workspace storage, not pod-local paths.

## Image pull failures
1. Pod container `reason` is ErrImagePull / ImagePullBackOff, or events contain "failed to pull image".
2. Check the exact image string in `images[]` (registry, repo, tag; the tag must exist).
3. Private registry: the workspace/user needs an image-pull secret (`GET /secrets`); see events for "unauthorized".
4. Registry unreachable from the cluster (proxy/DNS/firewall): ask the cluster admin; a different mirror may be needed.

## Node unhealthy / unavailable
1. `GET /nodes/<id>`: `phase` (Ready vs SSHFailed, ManagedFailed, ...), `available`, `message`.
2. `available=false` with taints: a health check failed (GPU, RDMA, storage monitor) or the node is cordoned/draining or reserved. The taint name identifies the failing monitor.
3. Recent history: `GET /opsjobs?type=reboot` / `preflight` (no node filter: match the node in each job's inputs); `GET /nodes/<id>/reboot/logs`; `GET /nodes/<id>/logs` for the node agent pod.
4. Remedies (admin only, confirm first): cordon/drain, reboot (opsjob), fix or disable the failing monitor, remove node from workspace.

## Permission errors
401: key missing/expired (`GET /apikeys/current`). 403: the user's role or workspace membership does not allow the action; `GET /users/self` shows roles and workspaces.
