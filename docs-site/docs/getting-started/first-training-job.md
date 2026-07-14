---
sidebar_position: 3
title: Your first job
---

# Your first job

This page takes you from a freshly installed platform to a running training job: an admin first
registers capacity (a node flavor, nodes, a cluster, and a workspace), then you submit a job into
that workspace. For all submit options and fields, see
[Run a single-node training job](/tasks/run-single-node-training).

It is written to serve two audiences at once. For **you (the reader)** the whole flow is here,
from empty install to running job. For an **AI agent** the golden path to perform is
**"Submit a job (console)"** — concrete numbered steps, each stating its own healthy result — while
the capacity-onboarding section above it is admin setup the demo environment already provides and
is only presence-checked. There is no hidden test layer on this page; bookkeeping (priority,
known product bugs) lives in the run contract `docs-site/AGENTS.md`.

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

*Not fully documented yet as a repeatable procedure: capacity onboarding (SSH secret, nodes,
cluster, workspace) is an admin step the demo environment already provides. An agent should not
perform these five steps as a test — only presence-check that the **System** sections (Secrets,
Flavors, Nodes, Clusters, Workspaces) and their **Create** controls exist, and that a workspace
with quota is already available for the golden path below.*

## Submit a job (console)

This is the golden path — perform it end to end. Once your workspace has healthy nodes:

### Before you start

- A **running cluster** with at least one healthy, schedulable node.
- A **workspace with GPU quota** you can select — this is the job's target.
- A **pullable container image** (a public image such as `docker.io/rocm/pytorch:latest` is fine).

> **Agent:** confirm the preconditions before Step 1. If no workspace with quota is available (or
> no running cluster), report **BLOCKED** (naming `workspace-with-quota` / `running-cluster`)
> rather than failing a step — do not onboard capacity yourself, that is the admin section above.

### Step 1 — Select your workspace
Sign in and select your **workspace** in the top-left workspace selector.

### Step 2 — Create the workload
Create a **workload**, choose **PyTorchJob**, and set an **image** + **entrypoint**
(e.g. `python train.py`; a trivial command such as `sleep 60` is fine for a smoke test) and the
**resources** for one replica.

### Step 3 — Submit and read the result
Click **Submit**. That's it — you've launched a single-node training job. Here is what each
outcome means — this is the pass/fail for the walkthrough:

- **Healthy (pass):** the job appears in the **PyTorch** list (if the list looks empty, switch the
  filter to **All**), and its phase is **Pending** or **Running** — the scheduler accepted it.
- **If instead you see** the job **Rejected** or **Failed**, or an error toast on submit, **that
  means** the submission was not accepted — usually an unpullable image, no quota left, or a node
  taint blocking placement **(fail)**. Fix the input and resubmit; see
  [Troubleshooting](/troubleshooting).

> **Agent:** fill the table below, show it to the user, and report **PASS** only if the job
> reached Pending/Running (not Rejected/Failed). Then **run cleanup**: delete the workload you
> created via its row action, so the run is repeatable.
>
> Known product drift to account for (do **not** count as a new failure): the console phase can
> stay **Pending** even after the job has actually run to completion. If Pending persists, an
> optional `kubectl` cross-check can note the true state as a ground-truth aside — but the
> UI-visible pass condition here is only "accepted, not Rejected/Failed".

| Check | Healthy result | Found |
|---|---|---|
| Job appears in PyTorch list | yes (filter = All if needed) | _fill in_ |
| Phase after submit | Pending or Running | _fill in_ |
| Not Rejected/Failed | true | _fill in_ |
| Cleanup (workload deleted) | done | _fill in_ |

## Where to go next

| You want to… | Go to |
|--------------|-------|
| All submit options & fields (UI/API) | [Run a single-node training job](/tasks/run-single-node-training) |
| Scale across nodes | [Run a multi-node distributed job](/tasks/run-multi-node-training) |
| Watch logs / shell in / get results | [Interact with your job](/tasks/interact-with-your-job) |
| Make jobs start faster | [Speed up workload startup](/tasks/speed-up-startup) |
