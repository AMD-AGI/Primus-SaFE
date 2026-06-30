---
sidebar_position: 3
title: Manage nodes
---

# Manage nodes

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/node.md`,
> `ops-job.md`, `workspace.md`

Day-2 node operations: bring capacity into the platform, move it between workspaces, take
nodes safely in and out of service, and clean up. Quota is **nodes × node flavor**, so most
node operations directly change how much compute a workspace has — see
[Workspace → Quota](/concepts/workspace#quota-and-node-flavor).

## Node lifecycle

A node moves through these phases (`phase` in the node list/detail):

| Phase | Meaning |
|-------|---------|
| `Managing` | Joining the cluster (addons installing). |
| `Ready` | Joined and schedulable. |
| `SSHFailed` / `HostnameFailed` | Bring-up failed (can't SSH / hostname setup failed). |
| `ManagedFailed` | Failed to join the cluster. |
| `Unmanaging` | Leaving the cluster. |
| `UnmanagedFailed` | Failed to leave the cluster. |

`available` (a separate boolean) tells you whether the node is currently schedulable; when it
is not, `message` explains why.

> **From the console (UI):** all of the operations below are also available under
> **System → Nodes** — **Create Node** to register, the row actions to taint/reboot/delete, and
> the node detail for capacity and logs. Each section shows the UI step and the equivalent API
> call.

<!-- @test
scope: page
mode: contract
priority: P1
personas: [admin]
preconditions: [running-cluster]
do: open System > Nodes (read-only — do NOT cordon, reboot, or delete any node)
expect:
  - registered nodes are listed, each with a phase (e.g. Ready) and an availability indicator
  - each node row exposes the day-2 actions (bind/unmanage, retry, reboot)
  - the Create Node form exposes hostname, private IP, flavor, node template, and SSH secret
-->
<!-- @test todo:
  - "Mutating node ops (taint/cordon/drain, reboot, delete) are destructive on a live cluster; add behavior tests only against a disposable node."
-->

## Register a node

A node must be SSH-reachable and is registered against an existing **node flavor** (hardware
profile), **node template** (software/addons), and **SSH secret** (login credentials). In the
console, open **System → Nodes → Create Node** and fill in the hostname, private IP, flavor,
template, and SSH secret:

![Create Node form](/img/screenshots/create-node-form.png)

The equivalent API call:

```bash
curl -X POST https://<your-console>/api/v1/nodes \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "privateIP": "192.168.1.100",
    "port": 22,
    "flavorId": "gpu-large",
    "templateId": "amd-gpu-template",
    "sshSecretId": "ssh-secret-001",
    "labels": { "datacenter": "us-west" }
  }'
```

- `hostname` defaults to `privateIP` if omitted.
- Custom `labels` **cannot** use the `primus-safe.amd.com/` prefix (reserved).
- The node enters `Managing` and becomes `Ready` once addons finish.

If a node gets stuck in `ManagedFailed`/`UnmanagedFailed`, retry without re-registering:

```bash
curl -X POST https://<your-console>/api/v1/nodes/retry \
  -H "Authorization: Bearer <admin-token>" \
  -d '{ "nodeIds": ["gpu-node-001", "gpu-node-002"] }'
```

View what happened during join/leave with the management logs:
`GET /api/v1/nodes/{nodeId}/logs`.

## Add nodes to an existing cluster

Adding capacity to a running cluster is a platform operation — **you do not re-run the
Bootstrap installer per node, and you do not SSH in to run Ansible by hand.** The flow is:

1. **Register the node** (above) — it reaches machine `Ready` once the resource-manager can SSH in.
2. **Bind it to the cluster** — **System → Clusters → \<cluster\> → add nodes**, or:

   ```bash
   curl -X POST https://<your-console>/api/v1/clusters/<clusterId>/nodes \
     -H "Authorization: Bearer <admin-token>" \
     -d '{ "action": "add", "nodeIds": ["gpu-node-001"] }'
   ```

**Under the hood:** binding a node to a cluster sets `node.spec.cluster`, and the
resource-manager runs **Kubespray `scale.yml`** against just that node **inside a pod** to join
it (`Managing` → `Managed`). The `node-agent` DaemonSet then lands on it automatically, and it
appears as schedulable capacity. Watch progress with `GET /api/v1/nodes/{nodeId}/logs`.

### Prerequisites for a node

| Requirement | Why |
|-------------|-----|
| SSH-reachable + a registered **SSH secret** | the resource-manager logs in to provision it |
| A **node flavor** | hardware profile used for quota/scheduling |
| A **node template** | defines the addons installed over SSH |
| **Not bound to another cluster**, not in a `Deleting`/terminating state | a node marked for deletion cannot be joined |
| Clean container runtime (no stray Docker), supported OS, `python3` | Kubespray `scale.yml` requirements |

:::warning Adopted (Bootstrap-built) clusters
If the cluster was built by hand with the Bootstrap installer and only later registered in the
console, the UI-driven join may stall (the platform's record can diverge from the real cluster).
As a fallback you can add the node to your Kubespray inventory under `[kube_node]` (leave
`[kube_control_plane]`/`[etcd]` untouched) and run `ansible-playbook -i <inventory> scale.yml
--limit=<node>` directly. Re-running the Bootstrap installer also works (it runs the idempotent
`cluster.yml`) but is heavier as it touches every node.
:::

:::note Node stuck in `Deleting`
A node bound to a cluster that is later deleted can get wedged with a `deletionTimestamp` and the
`primus-safe/node.finalizer`. It will neither finish deleting nor join. If it never actually
joined Kubernetes, clear the finalizer to let it delete, then re-register it fresh:
`kubectl patch node.amd.com <name> --type=merge -p '{"metadata":{"finalizers":null}}'`.
:::

## Inspect capacity

List nodes with their resources, current workloads, and availability:

```bash
# All Ready, available nodes in a cluster
GET /api/v1/nodes?clusterId=prod-cluster&phase=Ready&available=true

# Just the essentials (id, name, IP, availability + reason)
GET /api/v1/nodes?brief=true
```

Each node reports `totalResources` (defined by its flavor), `availResources`
(`total − used`), and the `workloads` currently running on it. The platform may reserve a
small slice of each node, so schedulable capacity is slightly below the raw flavor totals.

## Move nodes between workspaces

Capacity is assigned to a workspace by binding nodes to it; this is how you grow or shrink a
workspace's quota. Add or remove nodes on the workspace:

```bash
curl -X POST https://<your-console>/api/v1/workspaces/<workspaceId>/nodes \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{ "action": "add", "nodeIds": ["gpu-node-001"], "force": false }'
```

- `action` is `add` or `remove`.
- Removing a node reduces the workspace's `totalQuota`; in-flight workloads on that node may be
  affected, so prefer to drain first (next section). `force` overrides safety checks.

> **Not yet covered:** confirm the exact request body for the workspace-nodes endpoint
> (field names for `nodeIds`/`action`/`force`) against the handler.

## Take a node out of service (cordon / drain)

To stop new pods from landing on a node — or to evict what's already there — apply a **taint**
(from the node's **Edit** action in the console, or via a node update API call). Effects mirror
Kubernetes:

<!-- screenshot: System → Nodes → Edit/taint dialog (sanitized node) — add image -->


| Effect | Behavior |
|--------|----------|
| `NoSchedule` | No new pods scheduled (cordon). |
| `PreferNoSchedule` | Avoid scheduling new pods if possible. |
| `NoExecute` | No new pods **and** evict existing ones (drain). |

```bash
curl -X PATCH https://<your-console>/api/v1/nodes/<nodeId> \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{ "taints": [ { "key": "maintenance", "value": "true", "effect": "NoSchedule" } ] }'
```

To bring the node back into service, PATCH again with the taint removed (an empty `taints`
list clears them).

:::note
The system automatically prefixes taint keys with `primus-safe.`, so the example above
becomes `primus-safe.maintenance`.
:::

The platform's fault tolerance also taints nodes automatically when the Node Agent detects
problems — see [Pre-flight & in-flight monitoring](/administration/preflight-and-monitoring)
and [Fault tolerance](/concepts/fault-tolerance).

## Enable / disable health monitors (auto-taints)

The `node-agent` runs a set of health-check scripts (each has an ID, e.g. `201`, `309`). When a
check fails it sets a node condition and the platform applies a `primus-safe.<id>` `NoSchedule`
taint. On a cluster that lacks the hardware/storage a monitor expects, the check fails and taints
the node even though nothing is wrong — so you turn that monitor off.

Diagnose which monitor tainted a node (the taint/condition key is the monitor ID):

```bash
kubectl get nodes -o json | jq -r '.items[] | .metadata.name as $n
  | (.spec.taints[]? | select(.key|startswith("primus-safe")) | "\($n) \(.key)")'
# e.g. "<node> primus-safe.309"  -> monitor 309 (WekaFS CSI) is failing
```

There are **two ways to toggle a monitor**, depending on the monitor:

**A. Helm value toggles** (preferred) — available for `net_bnxt_load_204`, `net_ainic_load_205`,
`net_ainic_devices_208`, `sys_csi_wekafs_309`, `disk_nfs_exist_check_402`:

```bash
# e.g. a cluster with no WekaFS: turn off the Weka CSI check (309)
helm upgrade node-agent ./node-agent -n primus-safe --reuse-values \
  --set monitor.toggles.sys_csi_wekafs_309=off
```

(Or set the matching `node_agent_toggle_*` key in `SaFE/bootstrap/.env` and re-run the installer.)

**B. ConfigMap edit** — for monitors hardcoded `"on"` with no Helm value (e.g. `201`
`net_ib_status`, `401` `disk_nfs_mount`, `202`, `203`, `206`). Edit the rendered ConfigMap; the
node-agent hot-reloads:

```bash
kubectl -n primus-safe edit cm primus-safe-node-agent
# find the entry, change  "toggle":"on"  ->  "toggle":"off"
```

When the monitor is disabled (or the underlying issue is fixed and the check passes), the
condition clears and the `primus-safe.<id>` taint is removed automatically.

:::tip Common test-cluster toggles
A test cluster with **no NFS and no WekaFS** should disable `sys_csi_wekafs_309` (Helm value) and
`disk_nfs_mount_401` (ConfigMap edit — it defaults to `on` with empty mount settings and will
otherwise false-fault). Disable `net_ib_status_201` only if you have no working RoCE/IB fabric.
:::

## Reboot a node

A reboot runs as an asynchronous `reboot` **OpsJob**. Track it with the returned `jobId`:

```bash
curl -X POST https://<your-console>/api/v1/opsjobs \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{ "name": "reboot-node", "type": "reboot", "inputs": [ { "name": "node", "value": "gpu-node-001" } ] }'
# → { "jobId": "reboot-node-abc123" }
```

- Status: `GET /api/v1/opsjobs/{jobId}` (phases `Pending`/`Running`/`Succeeded`/`Failed`).
- Reboot history for a node: `GET /api/v1/nodes/{nodeId}/reboot/logs`.

## Remove a node from the cluster

A node must be unbound from its cluster before deletion unless you force it:

```bash
# Single node (must be unbound unless force=true)
curl -X DELETE "https://<your-console>/api/v1/nodes/<nodeId>?force=false" \
  -H "Authorization: Bearer <admin-token>"

# Batch
curl -X POST https://<your-console>/api/v1/nodes/delete \
  -H "Authorization: Bearer <admin-token>" \
  -d '{ "nodeIds": ["node-001", "node-002"], "force": false }'
```

:::warning Control-plane nodes
Control-plane nodes (`isControlPlane: true`) **cannot** be deleted or have their cluster
binding changed.
:::

## Quick reference

| Task | Endpoint |
|------|----------|
| Register node | `POST /api/v1/nodes` |
| Retry stuck manage/unmanage | `POST /api/v1/nodes/retry` |
| List / inspect capacity | `GET /api/v1/nodes` (`brief`, `phase`, `available`, `clusterId`, …) |
| Bind/unbind to workspace | `POST /api/v1/workspaces/{id}/nodes` (`add`/`remove`) |
| Cordon / drain (taint) | `PATCH /api/v1/nodes/{id}` (`taints`) |
| Reboot | `POST /api/v1/opsjobs` (`type: reboot`) |
| Reboot history | `GET /api/v1/nodes/{id}/reboot/logs` |
| Join/leave logs | `GET /api/v1/nodes/{id}/logs` |
| Delete (single / batch) | `DELETE /api/v1/nodes/{id}` · `POST /api/v1/nodes/delete` |

> **Not yet covered (capture so we don't lose it):**
> - [ ] Capture sanitized screenshots for the remaining UI steps (taint/Edit dialog, reboot
>       confirmation, delete confirmation). The Register-a-node screenshot is in place.
> - [ ] Node flavors and node templates as their own concept (what addons a template installs).
> - [ ] How draining interacts with fault-tolerant restart of training jobs.
