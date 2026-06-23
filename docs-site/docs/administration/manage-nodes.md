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
