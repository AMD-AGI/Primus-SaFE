---
sidebar_position: 2
title: Install
---

# Install Primus-SaFE

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `Bootstrap/README.md`,
> `SaFE/docs/installation/install.md`, `Bench/README.md`

:::note this page needs rework
it's missing a lots of steps. we should follow an actual setup routine based on the current code base

:::

Start with a single cluster — that is the path fits most users. Scale to a multi-cluster
fleet only when one control plane has to manage several GPU clusters.

## Single cluster (the common path)

### 1. (Bare metal only) Build the cluster with Bootstrap

If you do not already have Kubernetes, use **Bootstrap** to provision it. Edit `hosts.yaml`
with your nodes and roles (control plane, etcd, workers), then run:

```bash
bash bootstrap.sh
```

This brings up Kubernetes (via Kubespray) and installs the core add-ons. If you already have
a Kubernetes 1.21+ cluster, skip this step.

### 2. Install the platform

Run the installer from a machine with `helm`, `kubectl`, and cluster-admin access:

```bash
cd bootstrap
./install.sh
```

The script is interactive. The prompts you are most likely to set:

| Prompt | What it is |
|--------|------------|
| `storage_class` | The Kubernetes StorageClass for persistent state (default `local-path`; must already exist). |
| `ingress` | `nginx` (NodePort `30183`) or `higress` (domain at `https://<cluster>.primus-safe.amd.com`). |
| Image pull secret | Credentials for a private registry, or an empty placeholder secret. |
| `cluster_scale` | `small` / `medium` / `large` — sizes the control-plane replicas and resources. |
| S3 storage | Optional — endpoint, bucket, and keys for log download and S3 features. |
| SSO / OIDC | Optional — connect an external IdP (see the fleet section below). |
| `csi_volume_handle` | Optional — CSI handle that enables workspace persistent filesystem (PFS). |

The installer creates the required secrets, deploys the admin-plane services (apiserver,
webhooks, controllers, Postgres operator), applies the custom resources, and writes a `.env`
file so future upgrades reuse your answers.

### 3. Access the console

- **nginx:** open `http://<any-node-ip>:30183`
- **higress:** open `https://<cluster>.primus-safe.amd.com` (the web Service is also exposed
  as a NodePort — e.g. `http://<node-ip>:32494` — if you don't have DNS for the domain).

Log in with the seeded admin account **`root` / `root`** (created by the `primus-safe-cr`
chart, role `system-admin`). **Change this password immediately** on any reachable host.

:::note Temporary install prerequisite (until fixed upstream)
On a brand-new cluster the installer enables OpenSearch and a pre-install hook expects a
secret that doesn't exist yet, which fails the install. Until this is fixed, pre-create a
placeholder before running `install.sh`:

```bash
kubectl create namespace primus-safe 2>/dev/null
kubectl create secret generic primus-safe-opensearch-config -n primus-safe \
  --from-literal=username=admin --from-literal=password=admin \
  --from-literal=endpoint=primus-robust-logs.primus-robust.svc.cluster.local:9200
```
:::

### 4. (Optional) Verify with Primus-Bench

Before running production jobs, run **Primus-Bench** to health-check and benchmark your nodes
(SSH reachability, network connectivity, I/O, and system metrics). It runs standalone on bare
metal, SLURM, or Kubernetes:

```bash
bash run_bare_metal.sh
```

See [`Bench/README.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/Bench/README.md) for
SLURM and Kubernetes modes.

### Next

Run your first job: [Getting Started → First training job](/getting-started/first-training-job).

---

## Build out cluster capacity

A fresh install brings up the control plane and console, but **no cluster, nodes, or workspace
are registered yet** — that's an explicit admin step. Do it in this order (console: **System →**
each section):

### 1. Register an SSH secret
The platform manages nodes over SSH, so create a **Secret** of type **SSH** (Secrets → Create)
with a username and key pair (or password) that can reach your nodes. See
[Administration → Manage access & quota](/administration/manage-access-and-quota).

<!-- screenshot: Secrets → Create (SSH) — add sanitized image -->

### 2. Pick or create a node flavor
A **NodeFlavor** describes the per-node hardware (CPU/GPU/memory/RDMA); a workspace binds to
exactly one. Example flavors are seeded (`amd-mi300x/325x/355x-example`) — use one or create
your own under **Flavors**.

### 3. Register your nodes
Under **Nodes → Create Node**, add each node with its **hostname**, **private IP**, **flavor**,
**node template**, and the **SSH secret** from step 1 (default SSH port `22`). The
resource-manager SSHes in and brings each node from `Managing` → `Ready`. (API:
`POST /api/v1/nodes`.)

![Create Node form](/img/screenshots/create-node-form.png)

### 4. Create the cluster
Under **Clusters → Create Cluster**, name it (e.g. `default`), pick the SSH secret, and select
the nodes from step 3. A registered cluster shows phase `Ready`:

![Clusters list with a Ready cluster](/img/screenshots/clusters-list.png)

:::warning This form is the provisioning path
Create Cluster includes **Kube Spray Image** and a **Managed Cluster** toggle — i.e. it can run
kubespray against the selected nodes. On a cluster that **already runs Kubernetes** (e.g. you
installed SaFE on an existing cluster), confirm the managed/provision behavior before
submitting so you don't re-provision a live cluster.
:::

### 5. Create a workspace
Under **Workspaces**, create one (e.g. `test`), bind it to a flavor, add nodes, and enable the
**scopes** your team needs (`Train`, `Infer`, `Authoring`, …). Details in
[Administration → Manage access & quota](/administration/manage-access-and-quota).

Once a workspace has healthy nodes, you can
[run your first job](/getting-started/first-training-job). Day-2 operations on this capacity
(moving nodes, taints, reboot) live in [Administration → Manage nodes](/administration/manage-nodes).

:::note Node health & taints
node-agent runs hardware health checks and **taints** nodes whose checks fail (network/RDMA,
storage/CSI, etc.). On hardware without those features configured, expect taints — either
configure the real NICs/storage, disable the relevant monitors, or submit workloads with
`isTolerateAll: true` for testing. See [Manage nodes](/administration/manage-nodes).
:::

---

## Scaling to a fleet

The install above produces a working cluster where the control plane and data plane sit
together — the right choice for most teams. The same platform also runs as a **fleet**: one
control plane managing several data-plane GPU clusters (see
[Architecture → Control plane vs. data plane](/architecture)).

You do not run a different installer for this. The difference is operational rather than a
separate setup procedure:

- Each additional GPU cluster is registered as a `Cluster` resource (single-cluster users
  never create one by hand; fleet admins do).
- SaFE auto-installs the per-cluster add-ons workloads depend on — scheduler-plugins (gang +
  topology), CSI storage, the AMD GPU operator, and the training operator.
- The `node-agent` data plane runs on each managed cluster's GPU nodes.

:::note
The end-to-end fleet flow — cluster registration, credentials, and networking — and the
**SSO / OIDC** setup (the `primus-safe-sso` secret and `sso.enable`) are not yet documented
here. We will add them once verified against the resource-manager code.
:::
