---
sidebar_position: 2
title: Install
---

# Install Primus-SaFE

Most teams run Primus-SaFE on a single Kubernetes cluster; at larger scale one control plane can
manage several clusters as a fleet (see [Architecture](/architecture)). This guide covers the
single-cluster install.

This page is the **complete single-cluster install runbook**, written for two audiences at once:

- **For you (the reader):** follow the numbered steps from a deploy host to a working console.
- **For an AI agent:** the steps are concrete and ordered, and each states its own healthy
  result — so, given a target environment, **you can perform the install end to end** and verify
  it. Where a step needs a site-specific choice (node inventory, storage class, ingress type, NIC
  names, cluster name), **ask the operator** instead of guessing, and after each long-running
  step (Bootstrap, `install.sh`) **pause on the verification output** before continuing.

There is no hidden test layer on this page. Bookkeeping **and the docs-as-test run scope** — e.g.
that the regression environment is *already installed*, so the suite re-checks only sign-in rather
than re-running bring-up — live in the run contract `docs-site/AGENTS.md`, not here.

## Before you start

To install from scratch you need:

- A **deploy host** with passwordless `root` SSH to every node, and `kubectl` + `helm` installed.
- Your **target nodes**: the control-plane / etcd machines (an **odd number** — 1 or 3), reachable
  on a private network. GPU worker nodes are added later through the console, not here.
- A decision on **storage** (a default StorageClass — step 2) and **ingress** (`nginx` NodePort
  vs. `higress` domain — step 3).

> **Agent:** this runbook is executable — you may perform the install against a target
> environment. Confirm you have the deploy host, node list, and SSH access first; if any is
> missing, report **BLOCKED** naming what's absent rather than guessing. Ask the operator for the
> site-specific values called out in each step (inventory, storage class, ingress, NIC names,
> cluster name) before running that step.

## Install on a single cluster

Run the commands from a deploy host after cloning the repository (`Primus-SaFE/`); that host
needs passwordless `root` SSH to every node. Steps 1, 2, and 4 are required; steps 3 and 6 are
optional.

### 1. Provision Kubernetes with Bootstrap — required (bare metal only)

Skip this step **only** if you already have a Kubernetes 1.21+ cluster with cluster-admin
`kubectl` and `helm` access. Otherwise it is required.

**a. Describe your nodes.** Edit the inventory file `Primus-SaFE/Bootstrap/hosts.ini`. It is a
standard Ansible inventory. Use it to define **only the control-plane / etcd nodes** — use an
**odd number** of them (1 or 3) so etcd has a quorum. Listing the same nodes under `[kube_node]`
lets the control plane also run workloads.

```ini
[all]
node-01 ansible_host=10.0.0.11 ip=10.0.0.11 ansible_user=root
node-02 ansible_host=10.0.0.12 ip=10.0.0.12 ansible_user=root
node-03 ansible_host=10.0.0.13 ip=10.0.0.13 ansible_user=root

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

[k8s_cluster:children]
kube_control_plane
kube_node
```

:::note Add GPU workers through the console, not here
The recommended practice is **not** to add extra GPU worker nodes to `hosts.ini`. Use this file
only to define and configure the control-plane / etcd nodes; add worker nodes to the cluster
later through the Primus-SaFE console (see [Manage nodes](/administration/manage-nodes)). Listing
the control-plane nodes under `[kube_node]` is what lets them also run workloads.
:::

- `ansible_host` / `ip` are the addresses the installer and the other nodes use to reach each
  machine — use the private cluster network.
- If the deploy host reaches the nodes with a non-default SSH key, add
  `ansible_ssh_private_key_file=/path/to/key` to each `[all]` line.

:::warning Node hostnames are set from this file
Running Kubespray renames each node's hostname to the name you give it here. Keep these names
stable and correct up front, because a node's hostname must stay the same for downstream systems
(for example IAM / identity management) to keep working.
:::

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

**Healthy (pass):** every node reports `Ready`, and `helm list -A` shows the base add-ons
(cert-manager, the AMD GPU operator, the network operator, scheduler plugins). **If instead** a
node is `NotReady` or Bootstrap exits non-zero, that means provisioning failed — read the
Kubespray/Ansible error, fix it (usually SSH reachability or an inventory mistake), and re-run
before continuing.

> **Agent:** Bootstrap takes 20–40 minutes. Run it, then **pause** and confirm the two checks
> above pass before moving on. If any node is not `Ready`, stop and surface the error rather than
> proceeding to storage.

### 2. Set up a storage class — required

The platform stores persistent state — its database, message queue, and backups — on Kubernetes
**PersistentVolumes**, which need a default **StorageClass**. Without one those volumes never
bind and the install stalls. Create one before installing; choose based on your needs:

- **Local path** — simplest, and what most evaluations use. Backs each volume with a directory
  on the node where the pod runs (no replication; data is tied to that node):

  ```bash
  cd Primus-SaFE/Bootstrap/storage/local-path
  bash local-path.sh        # prompts for the directory to use, e.g. /data
  ```

  Installs the local-path provisioner and a `local-path` StorageClass.

- **Rook-Ceph** — production-grade replicated block storage (and an optional S3 endpoint). Use
  this when you need replication, shared (RWX) volumes, or object storage. Requires spare raw
  disks on the nodes:

  ```bash
  cd Primus-SaFE/Bootstrap/storage/ceph
  bash ceph.sh
  ```

In short: **local-path** is simple and fine for evaluation and single-node durability;
**Rook-Ceph** (or any production CSI) replicates across nodes, survives a node failure, and adds
shared (RWX) volumes and S3. Any CSI that provides a default StorageClass works. Note the name —
you give it to the installer in step 4.

**Healthy (pass):** `kubectl get storageclass` lists a **default** class (marked
`(default)`). **If instead** there is no default StorageClass, the platform's PersistentVolumes
never bind and step 4 stalls — set one before continuing.

> **Agent:** ask the operator which storage path to use (local-path for an evaluation, a
> production CSI otherwise) rather than choosing yourself; then verify a default StorageClass
> exists before step 4.

### 3. Gateway and private registry — optional

Skip this step for a quick start: the platform works with built-in NodePort access and public
images. Add these when you want domain-based access or an in-cluster registry.

**Higress gateway** — serves the console (and SSH-to-pods on port `2222`) on a **domain** instead
of a node port. It is a two-part setup:

1. Install the gateway here, once:

   ```bash
   cd Primus-SaFE/Bootstrap/higress
   bash higress.sh
   ```

2. Then, in step 4, choose `higress` as the ingress and enter a cluster name; the installer
   publishes the console through this gateway at your domain. (If you choose `nginx` instead, you
   skip Higress entirely and reach the console on NodePort `30183`.)

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
| `cluster name` | Names the cluster and becomes its subdomain (e.g. `tas325` → `tas325.primus-safe.amd.com`). You use this to reach the cluster ingress later, so pick it deliberately. |
| `ethernet nic` | The Ethernet interface distributed jobs use for NCCL/RCCL control traffic and TCP fallback (sets `NCCL_SOCKET_IFNAME`). Default `eno0`. |
| `rdma nic` | The RDMA/RoCE devices distributed jobs use for high-speed GPU-to-GPU transfers (sets `NCCL_IB_HCA`). Default `rdma0,…,rdma7`. |
| `storage_class` | The StorageClass from step 2 (default `local-path`; must already exist). |
| `ingress` | `nginx` (console on NodePort `30183`) or `higress` (domain-based; requires step 3). |
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
file so future upgrades reuse your answers.

**Healthy (pass):** `install.sh` finishes without error and the admin-plane pods reach `Running`:

```bash
kubectl get pods -n primus-safe      # apiserver, controllers, webhooks, db operator all Running
```

**If instead** pods stay `Pending`/`CrashLoopBackOff`, that usually points back to storage (no
bound PVC — recheck step 2) or the missing OpenSearch secret (the note above); fix and re-run
(`install.sh` is idempotent).

> **Agent:** `install.sh` is **interactive** — do not answer its prompts blindly. Ask the
> operator for `cluster name`, `ingress`, `storage_class`, and the NIC values (help them find NICs
> with the commands below), then run it and **pause** on the pod-status check before Step 5.

### 5. Access the console

This is the final verification that the install is healthy. Reach the console, then sign in:

- **nginx ingress:** open `http://<any-node-ip>:30183`.
- **higress ingress:** open `https://<your-cluster-domain>` (if DNS for the domain is not set up
  yet, the web service is also reachable on its NodePort).

Log in with the seeded admin account **`root` / `root`** (role `system-admin`).
**Change this password immediately.**

:::warning Higress (HTTPS): the console uses a self-signed certificate by default — you must address this
With the **higress** ingress the installer generates a **self-signed** TLS certificate for the
console domain. Browsers show a "Not secure" warning, and — because it relies on a WebSocket —
**WebShell (the in-browser pod terminal) silently fails to connect ("Disconnected")** until the
certificate is trusted. (The **nginx** ingress serves plain HTTP on NodePort `30183` and is not
affected.) Address it one of two ways:

- **Recommended — bring your own certificate.** Before `install.sh`, pre-create the ingress TLS
  secret with a cert signed by a CA your machines trust, covering the console **and** apiserver
  hostnames (the installer keeps an existing secret):

  ```bash
  kubectl create namespace primus-safe 2>/dev/null
  kubectl create secret tls default -n primus-safe --cert=fullchain.crt --key=tls.key
  # SANs must include:  <cluster>.primus-safe.amd.com  and  apiserver.<cluster>.primus-safe.amd.com
  ```
- **Or trust the self-signed cert** on each client (import into the OS/browser Trusted Root store,
  then restart the browser) — fine for evaluation.

See [Troubleshooting → WebShell shows "Disconnected"](/troubleshooting).

Here is what each outcome means — this is the pass/fail for the checkable part of this page:

- **Healthy (pass):** sign-in succeeds and the dashboard loads — you land on the console home
  with the navigation visible.
- **If instead you see** the login rejected, a blank page, or an error after submitting
  credentials, **that means** the admin login or the install is not healthy **(fail)** — capture
  the error and treat it as a real failure, not a missing fixture (a *missing* console or missing
  credentials is BLOCKED, not FAIL).

> **Agent:** sign in, fill the table below, show it to the user, and report **PASS** only if the
> dashboard loaded. No cleanup is needed (signing in creates nothing). A *missing* console or
> missing credentials is **BLOCKED**, not FAIL.

| Check | Healthy result | Found |
|---|---|---|
| Console reachable in a browser | page loads | _fill in_ |
| Sign in with seeded admin | succeeds | _fill in_ |
| Dashboard loads | console home with nav visible | _fill in_ |

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
