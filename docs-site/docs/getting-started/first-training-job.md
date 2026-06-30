---
sidebar_position: 3
title: Your first job
---

# Your first job

This page takes you from a freshly installed platform to a running training job: an admin first
registers capacity (a node flavor, nodes, a cluster, and a workspace), then you submit a job into
that workspace. For all submit options and fields, see
[Run a single-node training job](/tasks/run-single-node-training).

## Build out cluster capacity

A fresh install brings up the control plane and console, but **no cluster, nodes, or workspace
are registered yet** — that's an explicit admin step. Do it in this order (console: **System →**
each section):

### 1. Register an SSH secret
The platform manages nodes over SSH, so create a **Secret** of type **SSH** (Secrets → Create)
with a username and a key pair (or password) that can reach your nodes. See
[Administration → Manage access & quota](/administration/manage-access-and-quota).

### 2. Pick or create a node flavor
A **NodeFlavor** describes the per-node hardware (CPU/GPU/memory/RDMA); a workspace binds to
exactly one. Example flavors are seeded (`amd-mi300x/325x/355x-example`) — use one or create your
own under **Flavors**.

### 3. Register your nodes
Under **Nodes → Create Node**, add each node with its **hostname**, **private IP**, **flavor**,
**node template**, and the **SSH secret** from step 1 (default SSH port `22`). The resource
manager SSHes in and brings each node from `Managing` → `Ready`.

![Create Node form](/img/screenshots/create-node-form.png)

### 4. Create the cluster
Under **Clusters → Create Cluster**, name it, pick the SSH secret, and select the nodes from
step 3. A registered cluster shows phase `Ready`:

![Clusters list with a Ready cluster](/img/screenshots/clusters-list.png)

:::warning This form is the provisioning path
Create Cluster includes a **Kube Spray Image** and a **Managed Cluster** toggle — i.e. it can run
kubespray against the selected nodes. On a cluster that **already runs Kubernetes** (e.g. you
installed Primus-SaFE on an existing cluster), confirm the managed/provision behavior before
submitting so you don't re-provision a live cluster.
:::

### 5. Create a workspace
Under **Workspaces**, create one, bind it to a flavor, add nodes, and enable the **scopes** your
team needs (`Train`, `Infer`, `Authoring`, …). Details in
[Administration → Manage access & quota](/administration/manage-access-and-quota).

:::note Node health & taints
The node agent runs hardware health checks and **taints** nodes whose checks fail (network/RDMA,
storage/CSI, etc.). On hardware without those features configured, expect taints — configure the
real NICs/storage, disable the relevant monitors, or submit workloads with `isTolerateAll: true`
for testing. See [Manage nodes](/administration/manage-nodes).
:::

<!-- @test todo:
  - "Capacity onboarding (SSH secret, nodes, cluster, workspace) is an admin step the test environment provides already. Add a behavior test only when a disposable cluster is available to register against."
-->

## Submit a job (console)

Once your workspace has healthy nodes:

1. Sign in and select your **workspace**.
2. Create a **workload**, choose **PyTorchJob**, and set an **image** + **entrypoint**
   (e.g. `python train.py`) and the **resources** for one replica.
3. Submit.

That's it — you've launched a single-node training job.

<!-- @test
scope: page
mode: behavior
priority: P0
personas: [member]
preconditions: [running-cluster, workspace-with-quota]
do: using the current session, select a workspace with quota and follow the "Submit a job (console)" steps (use a pullable image; a trivial entryPoint is fine)
expect:
  - the submitted job appears in the PyTorch list (set the filter to All if needed)
  - its phase is Pending/Running, not Rejected/Failed
cleanup: delete the created workload via its row action
known_drift: console phase may stay Pending after the job actually ran (an optional kubectl cross-check shows the true state)
-->

## Where to go next

| You want to… | Go to |
|--------------|-------|
| All submit options & fields (UI/API) | [Run a single-node training job](/tasks/run-single-node-training) |
| Scale across nodes | [Run a multi-node distributed job](/tasks/run-multi-node-training) |
| Watch logs / shell in / get results | [Interact with your job](/tasks/interact-with-your-job) |
| Make jobs start faster | [Speed up workload startup](/tasks/speed-up-startup) |
