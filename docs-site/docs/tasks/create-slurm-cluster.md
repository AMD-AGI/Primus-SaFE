---
sidebar_position: 8
title: Create a Slurm cluster
---

# Create a Slurm cluster

This page is a hands-on walkthrough for standing up a **Slurm cluster** on Primus-SaFE using the
Slinky `slurm-operator` (**v1.2.0**). A Slurm cluster is a long-lived resource — a controller
(`slurmctld`), a REST API, an optional accounting service, a login node, and one or more **node
pools** of `slurmd` workers — that you submit Slurm jobs to, unlike a one-shot training or Ray job.

Under the hood a Slurm cluster is a **per-workspace Helm release of the Slinky `slurm` chart**,
deployed into your workspace's Kubernetes namespace. You describe it in terms of **node pools**;
each pool becomes one `slurmd` NodeSet plus a Slurm **partition** of the same name.

It is written to serve two audiences at once:

- **For you (the reader):** every step says *what* to click and *what a healthy result looks
  like*, so you can go from an empty environment to a running Slurm cluster.
- **For an AI agent:** the steps are concrete and ordered, and each one states its own pass/fail
  outcome in plain language — so you can hand this page to an agent and have it perform the task
  and judge success without any hidden instructions.

There is no separate test file and no invisible annotation on this page: **the prose you read is
the test an agent runs.** The only thing kept elsewhere is bookkeeping (priority, personas, and
any known product drift), in the run contract `docs-site/AGENTS.md`.

## Before you start

You need three things. If any is missing, this walkthrough is **blocked** — stop and get it
first, don't work around it:

- A **workspace with the `Slurm` scope**, selected in the top-left workspace selector. The Slurm
  cluster is deployed into this workspace's namespace, and creating it is scoped to the workspace.
- A **running cluster** backing that workspace (System → Clusters shows it Ready).
- The **Slinky `slurm-operator` add-on (v1.2.0) installed** on that cluster, into the `slinky`
  namespace. It ships the Slurm CRDs (`nodesets`, `controllers`, `loginsets`, `restapis`,
  `accountings`, `tokens`) that a Slurm cluster's Helm release depends on. `cert-manager` must be
  present (it is a prerequisite of the operator).

> **Agent:** confirm all three preconditions before Step 1. If the `slurm-operator` add-on is not
> installed (no `slurm-operator.1.2.0` entry under System → Addons, or
> `kubectl get crd | grep slinky.slurm.net` returns nothing), report **BLOCKED** naming the
> precondition `slurm-operator-installed` — do not attempt the install yourself and do not report
> FAIL. Name every Slurm cluster you create `doc-as-test-create-slurm-cluster-<runid>`.

## Step 1 — Open the Slurm section

1. Select your **workspace** in the top-left workspace selector (it must have the **Slurm** scope).
2. Go to **Workloads → Slurm** and click **Create Slurm Cluster**.

You should now be on the **Create Slurm Cluster** form, with sections for **Basic information**,
**Node pools**, and **Advanced**. If the **Slurm** nav item isn't there, the workspace is missing
the **Slurm** scope — that's the sign to stop and ask a workspace admin to add it, not a problem
with these steps.

## Step 2 — Fill in Basic information

- **Name** — a name for the cluster. Use a unique name so repeated runs don't collide. (An agent
  running the test suite names it `doc-as-test-create-slurm-cluster-<runid>`.) The Helm release
  will be `slurm-<name>`. Keep it short: Slurm caps the internal cluster identifier
  (`<workspace>_slurm-<name>`) at 40 characters, so the form shows the maximum name length allowed
  in your workspace and blocks names that are too long. (A long test `<runid>` may exceed it —
  shorten the name if the form flags it.)
- **Workspace** — shown read-only; it is the currently selected workspace and cannot be changed
  from this form.
- **Enable accounting** — turns on Slurm's `slurmdbd` job-accounting subsystem, which records job
  history queryable with `sacct`. When enabled, Primus-SaFE automatically provisions a small,
  release-scoped MariaDB (Deployment + Service + PVC + password secret) in your workspace namespace
  and points `slurmdbd` at it — you do **not** need to bring your own database. Leave it off if you
  don't need job history; disabling it later (via **Edit**) removes the database and its data.

You no longer choose a namespace or a container image here: the namespace is your workspace's
namespace, and every component defaults to its Slinky image. (An optional image-tag override lives
under **Advanced** for cases where you must pin a specific Slurm build.)

## Step 3 — Define node pools

This is where you place the cluster's compute. Add **one or more** node pools; each pool becomes a
Slurm partition and a group of `slurmd` workers:

- **Partition name** — the pool/partition name (e.g. `main`). Must be unique within the cluster.
- **Node count** — how many `slurmd` workers the pool has.
- **GPU / CPU / memory per node** — the per-worker resources. GPU is an AMD GPU count
  (`amd.com/gpu`); CPU and memory accept Kubernetes quantities (e.g. `128`, `1024Gi`). Stay within
  what your workspace flavor and free quota allow.

Use **Add node pool** to create more partitions, and the row's delete button to remove one (at
least one pool is required). A healthy form has a name on every pool and no red validation errors.

The **first** pool's partition is marked as the cluster's **default partition**, so jobs submitted
without an explicit `-p/--partition` flag land there. Order your pools so the partition you want as
the default is first.

## Step 4 — Submit and read the result

Click **Submit**. Here is what each outcome means — this is the pass/fail for the whole
walkthrough:

- **Healthy (pass):** the cluster appears in the **Slurm** list for this workspace. Its status
  reaches **Running** once the controller is up, its **partitions** (your pool names) are listed,
  and the **Nodes (ready / desired)** column climbs toward the total node count you requested. In
  the cluster's workspace namespace the controller, restapi, and login pods are `Running` and the
  `slurmd` NodeSet pods become Ready.
- **Rejected / Failed (fail):** the submission was *not* accepted, or the release never becomes
  healthy. This is almost always one of: the `slurm-operator` add-on isn't installed (the Slinky
  CRDs are missing), the workspace lacks the **Slurm** scope, or there is no quota left in the
  workspace. Fix the input and resubmit — see [Troubleshooting](/troubleshooting).

> **Agent:** fill the table below from what you observe and report **PASS** only if the cluster
> reached **Running** with its partition(s) visible. Drive the console with the `user-Playwright`
> MCP. A `kubectl -n <workspaceId> get nodesets,controllers` check is allowed only as a
> ground-truth *aside*, never as the primary assertion. Save a screenshot of the result to
> `/tmp/doc-as-test-create-slurm-cluster-<runid>.png`. Then **run cleanup**.

| Check | Healthy result | Found |
|---|---|---|
| Cluster appears in Slurm list (this workspace) | yes | _fill in_ |
| Phase after submit | reaches Running | _fill in_ |
| Partition(s) visible in the list | yes | _fill in_ |
| Nodes ready climbs toward desired | yes | _fill in_ |
| Validation errors on the form | none | _fill in_ |
| Detail page opens with pools + pods (see below) | yes | _fill in_ |
| SSH action shows a login command (see below) | yes | _fill in_ |
| Job runs from the login node (see below) | yes | _fill in_ |
| Stop moves cluster to Stopped, kept in list (see below) | yes | _fill in_ |
| Stop frees compute: Nodes ready drops to 0 (see below) | yes | _fill in_ |
| Stop frees compute: Usage breakdown GPU/Nodes "Used" drops (see below) | yes | _fill in_ |
| Resume brings it back to Running | yes | _fill in_ |
| Cleanup (cluster deleted) | done | _fill in_ |

## View cluster details

Click a cluster's **name** in the Slurm list to open its **detail page**. This is the read-through
view of what the cluster has allocated, analogous to clicking into a Ray cluster:

- A header with the cluster name, current **status**, and quick actions (**Refresh**, **SSH**, and
  **Stop**/**Resume**).
- A row of summary cards: **Nodes (ready / desired)**, **Total GPUs**, **Partitions**, and whether
  **Accounting** is on.
- A **Node pools** section — one block per partition showing node count and per-node GPU/CPU/memory.
  The first partition is tagged **Default partition**.
- A **Pods** table listing the live pods of the cluster (controller, login, restapi, accounting, and
  the `slurmd` workers) with their role, node, and phase.

- **Healthy (pass):** the detail page loads, the pool block(s) match what you created, and the pods
  table lists the controller/login/worker pods (Running once the cluster is up).

> **Agent:** open the detail page by clicking the cluster name, confirm at least one node-pool block
> and a non-empty pods table are shown, and record it in the pass/fail table.

## Access the login node via SSH

A Slurm cluster isn't something you "log into" like a VM — you submit jobs from its
**login node**. Once the cluster is **Running**, the Slurm list gives you a ready-to-copy SSH
command that drops you onto that login node, where the usual Slurm tooling (`sinfo`, `squeue`,
`srun`, `sbatch`) is available.

**Prerequisites.** SSH must be enabled on the deployment, and you must have registered an SSH
**public key** under **Settings** — the gateway is key-based only, there are no passwords. If
neither is in place, the SSH action will tell you so instead of showing a command.

1. In the **Slurm** list, find your cluster and click the **SSH** action (the connection icon in
   the Actions column). It is enabled only when the cluster's status is **Running**.
2. A dialog opens with the full `ssh …` command. Click **Copy SSH command**.
3. Paste it into a terminal that has your SSH key loaded. You land on the login node.
4. Verify the cluster and run a job. The first partition is the default, so you can omit `-p`:

   ```bash
   sinfo               # partitions and node states (the default is marked with a "*")
   srun -N1 hostname   # runs on the default partition
   ```

The command routes through the SaFE SSH gateway (the same mechanism used for training and
authoring pods), so the host and port are the platform's shared SSH endpoint; the encoded
username is what routes you to this cluster's login node.

- **Healthy (pass):** the SSH action is enabled while Running, the dialog shows a non-empty
  `ssh …` command, connecting lands you on the login node, and `srun -N1 hostname` (default
  partition) returns a worker hostname.
- **Not ready (expected before Running):** while the cluster is still `Deployed`/`Pending` the
  action is disabled, and if invoked it reports that the login node isn't reachable yet — this is
  not a failure, just wait for **Running**.

> **Agent:** with the cluster Running, open the SSH dialog via the **SSH** action, confirm a
> non-empty command is shown, and record it in the pass/fail table. Verifying an actual shell
> session and `srun` is optional ground truth (it requires a registered key); if you cannot
> complete the shell step in the harness, mark the "Job runs from the login node" row as _n/a_
> and still assert the command was shown.

## Stop, resume, and clone

Beyond create/edit/delete, a Slurm cluster has the same lifecycle controls as other workloads, so
you can keep a history of what you've provisioned instead of deleting it:

- **Stop** (the pause action, or the **Stop** button on the detail page) scales the cluster's
  components to zero — the `slurmd` workers, login node, REST API, and (if accounting is on) the
  MariaDB — **freeing the compute**. The cluster stays in the list with status **Stopped**, and its
  configuration and data (persistent volumes) are preserved. Any jobs that were running are lost.
- **Resume** (the play action, shown only while **Stopped**) restores the saved node counts and
  brings the cluster back up. It returns to **Running** once the components are Ready again.
- **Clone** (the copy action) opens the create form pre-filled from an existing cluster's node
  pools and settings, with the name suggested as `<name>-copy`. Adjust anything you like and submit
  to create a brand-new cluster — the original is untouched.

Only **Delete** removes a cluster from the list (it uninstalls the Helm release); **Stop** keeps it
as history.

Stopping is a **two-phase** operation under the hood: the `slurmd` worker NodeSets (and the login
node) scale to zero first while the REST API is briefly kept alive so the operator can drain the
workers, then the REST API scales down too. The important, user-visible outcome is that **Stop
actually frees the compute** — a cluster that still holds its GPU nodes after Stop is a bug, even
if the status label says "Stopped". While the workers are draining the status may briefly read
**Stopping**; it settles on **Stopped** once compute is freed.

- **Healthy (pass):** clicking **Stop** moves the cluster to **Stopped** while it remains in the
  list, **and its compute is actually released**: its worker pods terminate, **Nodes (ready /
  desired)** drops to **0**, and the workspace **Usage breakdown** "Used" figures for **GPU** and
  **Nodes** drop by this cluster's footprint (e.g. a 1-node, 8-GPU cluster frees 1 node and 8 GPUs).
  Clicking **Resume** returns it to **Running** with workers coming back; **Clone** opens a
  pre-filled create form.

> **Agent:** before Stop, note the workspace **Usage breakdown** "Used" GPU and Nodes counts (Home
> dashboard). With the cluster Running, click **Stop** and confirm: (1) the status becomes
> **Stopped** and the row is still listed; (2) **Nodes (ready / desired)** for the cluster drops to
> **0** (poll/refresh — draining is not instant, allow up to a few minutes); and (3) the Usage
> breakdown "Used" GPU and Nodes drop by this cluster's footprint. A `kubectl -n <workspaceId> get
> nodeset` / `kubectl -n <workspaceId> get pods -l app.kubernetes.io/component=worker` check
> showing the worker NodeSet at `status.replicas=0` and no worker pods is allowed as a ground-truth
> aside, never as the primary assertion. **Fail** if the worker pods are still Running / Nodes ready
> stays above 0 / Used GPU does not drop, even if the label says Stopped. Then click **Resume** and
> confirm it heads back toward **Running**. Record each row in the pass/fail table.

## Editing a cluster later

Adding capacity or a new partition is an **edit**, not a new cluster: use the **Edit** row action
in the Slurm list, add/resize pools, and submit. This performs a Helm upgrade of the release in
place. (Edit is hidden while a cluster is **Stopped** — resume it first.)

## Cleanup

When you're done, delete the Slurm cluster via its **row action** in the Slurm list. This
uninstalls the Helm release from your workspace namespace. You can leave the `slurm-operator`
add-on installed — it is a shared precondition for this page, not test-created state.

## Doing the same over the REST API

The console is the primary path; for scripts and automation the same cluster is created and read
through the SaFE API. Every call is scoped to a workspace (`workspaceId`):

```bash
# Create a Slurm cluster (one partition "main" of 2 GPU nodes)
curl -X POST https://<your-console>/api/v1/clusters/<cluster>/slurmclusters \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "workspaceId": "<workspace>",
    "name": "my-slurm-cluster",
    "accountingEnabled": false,
    "pools": [
      { "name": "main", "nodes": 2, "gpu": 8, "cpu": "128", "memory": "1024Gi" }
    ]
  }'

# List / get Slurm clusters in a workspace
curl "https://<your-console>/api/v1/clusters/<cluster>/slurmclusters?workspaceId=<workspace>" \
  -H "Authorization: Bearer <api-key>"
curl "https://<your-console>/api/v1/clusters/<cluster>/slurmclusters/my-slurm-cluster?workspaceId=<workspace>" \
  -H "Authorization: Bearer <api-key>"

# Edit (resize / add pools) — helm upgrade
curl -X PATCH "https://<your-console>/api/v1/clusters/<cluster>/slurmclusters/my-slurm-cluster?workspaceId=<workspace>" \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{ "pools": [ { "name": "main", "nodes": 4, "gpu": 8 } ] }'

# Stop — scale to zero, keep the cluster as history (status "Stopped")
curl -X POST "https://<your-console>/api/v1/clusters/<cluster>/slurmclusters/my-slurm-cluster/stop?workspaceId=<workspace>" \
  -H "Authorization: Bearer <api-key>"

# Resume — restore the saved node counts
curl -X POST "https://<your-console>/api/v1/clusters/<cluster>/slurmclusters/my-slurm-cluster/resume?workspaceId=<workspace>" \
  -H "Authorization: Bearer <api-key>"

# Delete — helm uninstall (removes it from the list)
curl -X DELETE "https://<your-console>/api/v1/clusters/<cluster>/slurmclusters/my-slurm-cluster?workspaceId=<workspace>" \
  -H "Authorization: Bearer <api-key>"
```

## Next

- Install or manage the operator add-on → [System → Addons](/administration/observability#addons).
- Submit jobs to your Slurm cluster by connecting to the login node — see
  [Access the login node via SSH](#access-the-login-node-via-ssh) above — and using standard Slurm
  tooling (`sinfo`, `srun -p <partition>`, `sbatch`).
