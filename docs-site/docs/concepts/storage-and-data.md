---
sidebar_position: 4
title: Storage & data
---

# Storage & data

During the life cycle of an AI workload, several different types of data are involved, including code, datasets, checkpoints, and logs. Each type has different requirements for capacity, performance, isolation, and lifetime. Primus-SaFE provides multiple storage solutions to accommodate these varying requirements.

Primus-SaFE has native support for Kubernetes ephemeral storage, which users can specify from the UI when starting a workload. This storage is backed by locally attached writable devices on the host and shares the same life cycle as the workload, so it is only suited to temporary data such as logs. Keep in mind that if a host has multiple drives, they can be used both for ephemeral storage and for forming a distributed file system such as Ceph. Plan ahead to balance large scratch space against shared storage.

Primus-SaFE also provides a **volume** abstraction, attached at the **workspace** level, to strike a balance between data sharing and isolation. Once a volume is configured, it is mounted into every workload started by users in that workspace.


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
