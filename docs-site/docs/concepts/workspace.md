---
sidebar_position: 1
title: Workspace
---

# Workspace

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workspace.md`,
> `node-flavor.md`

A **workspace** provides multi-tenant isolation on the platform: an isolated environment with its own quota, storage, and access control.

## What a workspace gives you

- **Quota** — a pool of CPU, GPU, memory, and storage your jobs draw from.
- **Isolation** — workloads, secrets, and storage are scoped to the workspace (it maps to a
  Kubernetes namespace).
- **Access** — members can use it; managers (workspace-admins) administer it.
- **Scopes** — which kinds of workloads are allowed: `Train`, `Infer`, `Authoring`, `CICD`.

## Quota and node flavor

A workspace is bound to **one node flavor** — a hardware profile that defines the GPU
resource type and per-node CPU/GPU/memory (e.g. 8× `amd.com/gpu` on MI300X). Quota is
effectively **number of nodes × the node flavor**:

| Quota field | Meaning |
|-------------|---------|
| `totalQuota` | Total resources in the workspace (nodes × flavor). |
| `usedQuota` | Resources currently consumed by workloads. |
| `availQuota` | What's free (`total − used − abnormal`). |
| `abnormalQuota` | Resources stuck on unhealthy nodes. |

You **pick** a flavor when creating a workspace; **admins define** flavors. One workspace
cannot mix flavors. The system may reserve a small portion of a node's resources to run administrative tasks, so the schedulable amount is slightly below the raw flavor totals.

## Scheduling within a workspace

- **Queue policy** — `fifo` (strict submission order; a blocked job holds the line) or
  `balance` (any job that fits can run; still honors priority).
- **Preemption** — when `enablePreempt` is on, a higher-priority workload can preempt a
  lower-priority one in the same workspace.
- **Max runtime** — optional per-scope time caps (e.g. `Authoring: 168` hours).

## Access model (brief)

- **Member** — can use the workspace (submit and manage their own work).
- **Manager (workspace-admin)** — administers the workspace and its resources; granting
  someone manager also grants access.
- A **default** workspace (`isDefault`) is accessible to all users.

Adding teammates and credentials is a task, not a concept — see
[Manage access & quota](/administration/manage-access-and-quota).

## Status

A workspace is `Creating` → `Running`, and can become `Abnormal` (all nodes unavailable) or
`Deleting`. You cannot delete a workspace that still has running workloads.
