---
sidebar_position: 2
title: Install
---

# Install Primus-SaFE

Most teams run Primus-SaFE on a single Kubernetes cluster; at larger scale one control plane can
manage several clusters as a fleet (see [Architecture](/architecture)). This guide covers the
single-cluster install.

<!-- @test
scope: page
mode: behavior
priority: P1
targets:
  console: { baseUrl: "${PRIMUS_CONSOLE_URL}", login: "${PRIMUS_ADMIN_LOGIN}" }
notes: "Install/bring-up is not tested (the env is provided). The one live check is that the seeded admin can sign in. Host and credentials come from .docs-test.env and are never committed."
do: open {{baseUrl}} and sign in with the seeded admin login (PRIMUS_ADMIN_LOGIN)
expect:
  - sign-in succeeds and the dashboard loads
-->

## Install on a single cluster

Run the commands from a deploy host after cloning the repository (`Primus-SaFE/`); that host
needs passwordless `root` SSH to every node. Steps 1, 2, and 4 are required; steps 3 and 6 are
optional.

### 1. Provision Kubernetes with Bootstrap — required (bare metal only)

Skip this step **only** if you already have a Kubernetes 1.21+ cluster with cluster-admin
`kubectl` and `helm` access. Otherwise it is required.

**a. Describe your nodes.** Edit the inventory file `Primus-SaFE/Bootstrap/hosts.ini`. It is a
standard Ansible inventory: list every machine under `[all]`, then assign each to roles. Use an
**odd number** of control-plane / etcd members (1 or 3) so etcd has a quorum.

```ini
[all]
node-01 ansible_host=10.0.0.11 ip=10.0.0.11 ansible_user=root
node-02 ansible_host=10.0.0.12 ip=10.0.0.12 ansible_user=root
node-03 ansible_host=10.0.0.13 ip=10.0.0.13 ansible_user=root
node-04 ansible_host=10.0.0.14 ip=10.0.0.14 ansible_user=root
# ...add the rest of your GPU worker nodes (node-05, node-06, ...)

[kube_control_plane]
node-01
node-02
node-03

[etcd]
node-01
node-02
node-03

[kube_node]
node-01
node-02
node-03
node-04
# ...and every other GPU worker node

[k8s_cluster:children]
kube_control_plane
kube_node
```

Here `node-01`–`node-03` are the control-plane / etcd members, and every node (including more
workers beyond `node-04`) goes under `[kube_node]` so it can run GPU workloads.

- `ansible_host` / `ip` are the addresses the installer and the other nodes use to reach each
  machine — use the private cluster network.
- Put every GPU worker in `[kube_node]`. A node may be both a control-plane node and a worker.
- If the deploy host reaches the nodes with a non-default SSH key, add
  `ansible_ssh_private_key_file=/path/to/key` to each `[all]` line.

**b. Run Bootstrap.**

```bash
cd Primus-SaFE/Bootstrap
bash bootstrap.sh
```

This clones Kubespray, provisions Kubernetes (v1.32.5, Flannel CNI), writes the kubeconfig to
`~/.kube/config`, and installs the base add-ons (cert-manager, the AMD GPU operator, the network
operator, and the scheduler plugins). Expect 20–40 minutes, then verify:

```bash
kubectl get nodes -o wide      # every node Ready
helm list -A
```

### 2. Set up a storage class — required

The platform stores persistent state — its database, message queue, and backups — on Kubernetes
**PersistentVolumes**, which need a default **StorageClass**. Create one based on your needs before proceeding:

- **Local path** — simplest, and what most evaluations use. Backs each volume with a directory
  on the host. It provides no replication and data is tied to that node:

  ```bash
  cd Primus-SaFE/Bootstrap/storage/local-path
  bash local-path.sh        # prompts for the directory to use, e.g. /data
  ```

- **Rook-Ceph** — production-grade replicated block storage (and an optional S3 endpoint). Use
  this when you need replication, shared (RWX) volumes, or object storage. Requires spare raw
  disks on the nodes:

  ```bash
  cd Primus-SaFE/Bootstrap/storage/ceph
  bash ceph.sh
  ```

You can choose any storage providers that supports StorageClass. Also save the StorageClass name, which
will be needed in Step 4.

### 3. Gateway and private registry — optional

Skip this step for a quick start: the platform works with built-in NodePort access and public
images. Add these when you want domain-based access or an in-cluster registry.

**Higress gateway** — provides a complete Ingress/Gateway solution for production-grade clusters.

   ```bash
   cd Primus-SaFE/Bootstrap/higress
   bash higress.sh
   ```

**Harbor registry** — an in-cluster container registry, useful for private images and to cache
public ones close to the cluster. It needs a StorageClass (step 2), cert-manager (installed by
Bootstrap), and the Higress gateway above:

```bash
cd Primus-SaFE/Bootstrap/harbor
# bash harbor.sh <admin-password> <harbor-domain> [storage-class] [ssh-key]
bash harbor.sh 'choose-a-strong-password' harbor.example.com local-path ~/.ssh/id_ed25519
```

This installs Harbor, issues a self-signed certificate, distributes its CA to the nodes, and
creates `primussafe` and `public` projects. When the platform installer detects Harbor, it uses
it automatically as a pull-through image cache.

### 4. Install the platform — required

:::note Install prerequisite
On a brand-new cluster the installer enables OpenSearch, and a pre-install hook expects a secret
that does not exist yet. Pre-create a placeholder before running the installer:

```bash
kubectl create namespace primus-safe 2>/dev/null
kubectl create secret generic primus-safe-opensearch-config -n primus-safe \
  --from-literal=username=admin --from-literal=password=admin \
  --from-literal=endpoint=primus-robust-logs.primus-robust.svc.cluster.local:9200
```
:::

Run the installer from a machine with `helm`, `kubectl`, and cluster-admin access:

```bash
cd Primus-SaFE/SaFE/bootstrap
bash install.sh
```

The script is interactive. The prompts you are most likely to set:

| Prompt | What it is |
|--------|------------|
| `ethernet nic` | The Ethernet interface distributed jobs use for NCCL/RCCL control traffic and TCP fallback (sets `NCCL_SOCKET_IFNAME`). Default `eno0`. |
| `rdma nic` | The RDMA/RoCE devices distributed jobs use for high-speed GPU-to-GPU transfers (sets `NCCL_IB_HCA`). Default `rdma0,…,rdma7`. |
| `storage_class` | The StorageClass from step 2 (default `local-path`; must already exist). |
| `ingress` | `nginx` (console on NodePort `30183`) or `higress` (requires step 3). |
| Image pull secret | Credentials for a private registry, or an empty placeholder. |
| `cluster_scale` | `small` / `medium` / `large` — sizes the control-plane replicas and resources. |
| S3 storage | Optional — endpoint, bucket, and keys for log download and object features. |
| SSO / OIDC | Optional — connect an external identity provider. |
| `csi_volume_handle` | Optional — enables a workspace persistent filesystem (PFS). |

To find the NIC names on a node:

- **Ethernet:** `ip -br link` — pick the interface that carries the node's cluster IP (e.g.
  `eno0`, `bond0`).
- **RDMA:** `rdma link` or `ibdev2netdev` (or `ls /sys/class/infiniband`) — list the HCA devices.

The two NIC values set a **cluster-wide default** for multi-node training. If they are wrong,
multi-node jobs fail to set up GPU-to-GPU communication (NCCL cannot find the interface/device) or
fall back to a slow path; single-node jobs are unaffected. Use **consistent NIC names across all
nodes** — one value is applied fleet-wide, so nodes with different names won't match (you can
override the NCCL variables per workload if a node differs).

The installer creates the required secrets, deploys the admin-plane services (apiserver,
webhooks, controllers, database operator), applies the custom resources, and writes a `.env`
file that persists the current settings that can be reused future upgrades.

### 5. Access the console

- **nginx ingress:** open `http://<any-node-ip>:30183`.
- **higress ingress:** open `https://<your-cluster-domain>` (Set up your DNS entry to point the IP to the Higress Gateway).

Log in with the seeded admin account **`root` / `root`** (role `system-admin`).
**Change this password immediately.**

### 6. Validate nodes with Primus-Bench — optional

Before running production jobs, use **Primus-Bench** to health-check and benchmark your nodes —
SSH reachability, network connectivity, I/O, and key system metrics:

```bash
cd Primus-SaFE/Bench
# list your nodes in hosts.ini, then:
bash run_bare_metal.sh
```

Primus-Bench also runs on SLURM and Kubernetes — see
[`Bench/README.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/Bench/README.md).

### Next

Run your first job: [Getting Started → First training job](/getting-started/first-training-job).

> Running several GPU clusters under one control plane (a **fleet**) is covered separately — see
> [Architecture](/architecture).
