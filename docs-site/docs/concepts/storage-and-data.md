---
sidebar_position: 4
title: Storage & data
---

# Storage & data

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workspace.md`,
> `persistent-volume.md`

Your code, datasets, checkpoints, and models need somewhere to live that outlasts any single
pod. In Primus-SaFE, storage is attached at the **workspace** level and mounted into every
workload that runs there.

## Volume types

A workspace declares one or more **volumes**, each of a `type`:

| Type | What it is | Typical use |
|------|------------|-------------|
| **`pfs`** | A shared parallel filesystem, mounted `ReadWriteMany` across nodes. A PV/PVC is created per workspace. | Datasets, checkpoints, and models shared across a distributed job. |
| **`hostpath`** | A path on the node mounted into the pod. | Node-local data; caches. |

Because PFS is `ReadWriteMany`, every replica of a multi-node job reads and writes the same
files — which is what makes checkpoint/resume and shared datasets work.

## What backs PFS

PFS is provisioned as a Kubernetes PersistentVolume/PersistentVolumeClaim per workspace,
backed by a **CSI driver**. In a default install that driver is **WekaFS CSI**
(`csi_volume_handle` at install time); volumes are matched by a selector label
(`primus-safe.pfs-selector`) to the workspace's PVC. Other CSI/NFS storage classes can back a
PV as well.

When you submit a workload, the platform attaches the workspace's PVCs automatically
(`useWorkspaceStorage` defaults to `true`), so your job sees the shared filesystem at its
configured `mountPath`.

:::note
The platform's workspace PFS path is **WekaFS CSI** in the core install. JuiceFS appears only
as an optional Bootstrap script and is not the default workspace storage path. The
[Architecture](/architecture) page stays vendor-neutral on purpose.
:::

## Object storage (optional)

S3 can be enabled at install (`s3.enable`) for importing models and downloading logs. When
configured, the platform can sync between S3 and PFS — useful for getting large model
artifacts onto the shared filesystem.

## Where things land

- **Datasets** — staged on PFS so all nodes of a job read the same copy.
- **Checkpoints** — written to PFS, which is what failover resumes from.
- **Models** — downloaded/imported to PFS (optionally via S3).

> **Not yet covered (capture so we don't lose it):**
> - [ ] Exact mount paths and the per-workspace PV template details.
> - [ ] Sizing guidance for PFS capacity vs. node-local (`ephemeralStorage`).
