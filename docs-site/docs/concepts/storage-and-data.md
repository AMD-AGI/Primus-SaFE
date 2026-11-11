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

A volume can be limited to some workload types with `scopes`; it is then mounted only into workloads of those types.

A workload can also ask for host paths of its own with `hostpath`. Each entry must be the directory a `hostpath` volume of its workspace mounts into the workload's type (the volume's `hostPath`, joined with its `subPath` when it has one), or a directory under it. For a volume with `enableUserDir` that excludes the volume's directory itself and its user directories (`users` and everything below it): each user already gets their own directory from the volume. The pod mounts the declared directory (for a volume with `enableUserDir`, the directory directly under the volume's that holds the entry) and reaches the entry through `subPath`, so a symbolic link inside it cannot lead out of it; the entry is mounted read-only when the volume is `ReadOnlyMany`. Any other host path gives the workload the node's files, so it is allowed only to users who may run privileged workloads in the workspace (the `workload/privileged` grant, held by workspace administrators and system administrators). Sandboxes can only use declared paths.


## Your own directory and extra directories

A `hostpath` volume with `enableUserDir` gives every user a directory of their own,
`<volume>/users/<your user id>` (for example `/shared_nfs/users/<your user id>`). It is mounted
into your workloads automatically, and only you can see it: keep private data, code and
checkpoints there. The volume can also list shared directories (`sharedPaths`), such as model
and cache directories, that every workload of the workspace gets beside your own directory.

What a workload sees by default depends on its type, through the volume's `scopes`. A common
setup, used for example for `/shared_nfs`, mounts the whole volume into development
environments (Authoring), while PyTorch jobs, TorchFT, Monarch and Ray jobs, inference services
and the other types get only your own directory and the shared directories. When such a workload
needs more of the volume, name the directories yourself:

1. Open the create form of the workload (for example **Training → PyTorch Job → Create**).
2. Below the resource settings (the **Resource** section; **Ray Cluster** for Ray jobs), find **extra mounts**. It appears when a volume of the workspace
   has `enableUserDir`; its hint names the volume and your own directory.
3. Type a directory, for example `/shared_nfs/team-a`, and press Enter. Repeat for each directory.
4. Submit. Each directory is mounted at the same path inside the container.

Each directory must lie under a `hostpath` volume of the workspace that is mounted into this
workload type. The form refuses, before you submit:

- the volume itself (for example `/shared_nfs`);
- the `users` directory and anything below it, your own directory included: yours is already
  mounted, and other users' directories cannot be;
- a shared directory or a path inside one, which every workload already gets;
- a relative path or one with `..`.

**Clone** and **Resume** keep the extra directories of the original workload; you can change
them in the cloned form. **Edit** shows them but cannot change them: clone the workload instead.
Through the API, the same list is the `hostpath` field of the workload (see the API reference).

<!-- @test
mode: behavior
priority: P1
personas: [member]
preconditions: [running-cluster, workspace-with-userdir-volume]
do: follow "Your own directory and extra directories" steps 1 to 4 with one directory under the volume, then clone the workload
expect:
  - the form shows extra mounts with a hint that names your own directory under users
  - the form refuses the volume itself and a path under users before submit
  - the created workload's container has the directory mounted at the same path
  - the clone form is prefilled with the same extra mounts
cleanup: delete the workload and its clone via their row actions
-->

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
