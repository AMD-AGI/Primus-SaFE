---
sidebar_position: 4
title: Storage & data
---

# Storage & data

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workspace.md`,
> `persistent-volume.md`

During the life-cycle of AI workloads, there are differnt types of data involved including code, datasets, checkpoints and logs. These data has differnt characteristics of requirement such as capacity, performance, isolation, and various life cycles. Primus-SaFE include multiple storage solutions to accommondate the various data requirements.

Primus-SaFE has native support for Kubernetes ephemeral storage that users can specify from the UI when starting a workload. Such storage is backed up locally-attached writeable devices on the host that has the same life-cycle as the workloads, therefore it's only suited for temporary data such as logs. Also keep in mind that if there are multiple drives on the host, they can be used for both ephemeral storage and forming a distributed file system such as Ceph. Plan ahead of time to balance between large scrape space and shared storage. 

Primus-SaFE also provides an abstraction of **volume** that attatched at the **workspace** level to strike a balance between data sharing and isolation. Once a volume is configured, it will be mounted into every workload started by the users.


Primus-SaFE supports two type of volumes:
| Type | What it is | Typical use |
|------|------------|-------------|
| **`pfs`** | A shared parallel filesystem that supports CSI interface. It creates a PV/PVC per workspace and mounted on each node and shared across workloads. | Datasets, checkpoints, and models shared across a distributed job. |
| **`hostpath`** | A path on the node mounted into the pod. | Storage systems only support host mount, or sharing with other systems such as NFS mount Home Dirs  |


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


> **Not yet covered (capture so we don't lose it):**
> - [ ] Exact mount paths and the per-workspace PV template details.
> - [ ] Sizing guidance for PFS capacity vs. node-local (`ephemeralStorage`).
