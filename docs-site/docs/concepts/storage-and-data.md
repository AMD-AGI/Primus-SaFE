---
sidebar_position: 4
title: Storage & data
---

# Storage & data

This page explains the storage options an AI workload uses on Primus-SaFE — ephemeral storage,
the workspace-level **volume** abstraction (`pfs` and `hostpath`), what backs PFS, and optional
object storage. It is explanation, not a walkthrough — there is no procedure to perform here.

It is written to serve two audiences at once:

- **For you (the reader):** a plain-language guide to which storage fits code, datasets,
  checkpoints, and logs, and how the pieces are wired together.
- **For an AI agent:** the named storage types, defaults, and settings below are concrete enough
  to confirm by presence. As a concept page it is **verify**-level — an agent checks that the
  documented options exist, it does not provision storage.

There is no separate test file and no invisible annotation on this page: the prose you read is
all there is. The only thing kept elsewhere is bookkeeping (priority, and any known product
bug), in the run contract `docs-site/AGENTS.md`.

During the life cycle of an AI workload, several different types of data are involved, including
code, datasets, checkpoints, and logs. Each type has different requirements for capacity,
performance, isolation, and lifetime. Primus-SaFE provides multiple storage solutions to
accommodate these varying requirements.

> **What an agent verifies here:** confirm the documented storage model exists — the **ephemeral
> storage** field on the workload submit form; the workspace-level **volume** abstraction with
> its two types (**`pfs`** and **`hostpath`**) from the table below; **PFS** backed by a CSI
> driver (**WekaFS CSI** by default) and auto-attached via `useWorkspaceStorage`; and the
> optional **S3** object storage. Presence/consistency only — nothing here is created or mounted.

Primus-SaFE has native support for Kubernetes ephemeral storage, which users can specify from the
UI when starting a workload. This storage is backed by locally attached writable devices on the
host and shares the same life cycle as the workload, so it is only suited to temporary data such
as logs. Keep in mind that if a host has multiple drives, they can be used both for ephemeral
storage and for forming a distributed file system such as Ceph. Plan ahead to balance large
scratch space against shared storage.

Primus-SaFE also provides a **volume** abstraction, attached at the **workspace** level, to
strike a balance between data sharing and isolation. Once a volume is configured, it is mounted
into every workload started by users in that workspace.

Primus-SaFE supports two types of volume:

| Type | What it is | Typical use |
|------|------------|-------------|
| **`pfs`** | A shared parallel filesystem exposed through a CSI interface. It creates one PV/PVC per workspace that is mounted on each node and shared across workloads. | Datasets, checkpoints, and models shared across a distributed job. |
| **`hostpath`** | A path on the node mounted into the pod. | Storage systems that only support a host mount, or sharing with other systems such as NFS-mounted home directories. |

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

The mount path for a volume is chosen when you create the workspace, and the console makes
the available options clear at that point, so it needs no separate walkthrough here.
