---
sidebar_position: 2
title: Run a multi-node distributed job
---

# Run a multi-node distributed job

Run training across **several nodes** when one node's GPUs aren't enough. This builds on
[Run a single-node training job](/tasks/run-single-node-training) — everything there still
applies; this page covers only the multi-node differences.

As on the single-node page, the prose is the walkthrough for **both** a human and an agent: each
step says what to do and what a healthy result looks like, with no hidden test layer. Bookkeeping
lives in the run contract `docs-site/AGENTS.md`.

## Before you start

- A **workspace with GPU quota** (as for a single-node job).
- **At least two nodes that are Ready** in that workspace's cluster — otherwise a multi-node job
  can't be placed and will sit Pending forever.

> **Agent:** this walkthrough needs `workspace-with-quota` **and** `multiple-ready-nodes`. Check
> the Nodes view first; if fewer than two nodes are Ready, report **BLOCKED** (missing
> `multiple-ready-nodes`) rather than submitting a job that can never schedule.

## Step 1 — Start from the single-node form

In **Workloads → Training → PyTorch → Create PyTorch Job**, fill in **Basic information** exactly
as for a single node (name, a pullable image, a trivial entry point such as `sleep 120`). Use a
unique name suffix so re-runs don't collide.

## Step 2 — Ask for more than one node (Resources)

In **Resources**:

- Set **replicas** (or switch to **nodes** mode) to the number of nodes you want — for a smoke
  test, **2** is enough.
- Request the **full GPU count per replica** (e.g. `8`) so each replica occupies a whole node.

The platform creates one **master** and the rest as **workers**, starts them **together** (gang
scheduling — all-or-nothing, so partial allocations never hold GPUs idle), and places them with
**network locality** for fast collectives.

## Step 3 — Submit and read the result

Click **Submit**, then open the job's row / detail page and watch the phase. What the outcomes
mean — this is the pass/fail:

- **Healthy (pass):** the job is accepted (appears in the PyTorch list, not **Rejected**), and on
  its detail page it reaches **Running** with **all replicas scheduled together** — you see the
  master plus the expected number of workers all placed at once, not one running while others
  hang. That "all at once" is gang scheduling working.
- **Stuck Pending (investigate, usually a precondition):** if it never leaves Pending, the most
  common cause is not enough Ready nodes for the requested replica count — i.e. the
  `multiple-ready-nodes` precondition wasn't really met. This is a blocked run, not a pass.
- **Rejected / Failed (fail):** an input problem as on the single-node page (unpullable image, no
  quota, taints).

> **Agent:** fill the table, show it, and **PASS** only if the job reached Running with all
> replicas placed together. Then delete the workload via its row action (cleanup). Same known
> drift applies as on the single-node page (console phase can lag the real pod state); judge from
> the UI, note `kubectl` ground truth only as an aside.

| Check | Healthy result | Found |
|---|---|---|
| Accepted (in PyTorch list, not Rejected) | yes | _fill in_ |
| Replicas scheduled together (gang) | master + workers placed at once | _fill in_ |
| Phase | reaches Running | _fill in_ |
| Cleanup (workload deleted) | done | _fill in_ |

## Pin to specific nodes

By default the scheduler chooses nodes. To target particular machines, use **Specified nodes**
with a **node affinity** policy: **`required`** runs *only* on the listed nodes (staying pending
until they're free); **`preferred`** prefers them but may use others. When you specify nodes, the
replica counts follow automatically (PyTorchJob: master = 1, workers = the remaining nodes).

## Networking for collectives

Multi-node training depends on fast GPU-to-GPU communication:

- **Host networking** is enabled automatically for full-GPU multi-node jobs (API field
  `forceHostNetwork`) so RDMA/RoCE traffic isn't slowed by overlay networking.
- **RDMA** devices (`rdma/hca`) are requested the same way as GPUs.
- **NCCL/RCCL** cluster-wide defaults (`NCCL_SOCKET_IFNAME` / `NCCL_IB_HCA`) are set at install
  time (see [Install](/getting-started/install)); override per job with env vars when a node type
  differs.

:::note NIC drivers must be in your image
Your image sometimes needs the **RDMA NIC drivers** installed to use the fabric (e.g. Broadcom
NICs need the Broadcom drivers shipped in the image or installed from the entry point). Without
them, jobs fall back to a slow path or fail collectives — a commonly forgotten step.
:::

:::tip Preheat the image for large jobs
On big runs (32/64/128 nodes) one node can pull the image quickly while others pull slowly —
occasionally slow enough that the first pull times out before the rest finish, failing the job.
If you hit it, use the **preheat** toggle (or preheat ahead of time). See
[Speed up workload startup](/tasks/speed-up-startup).
:::

## Reliability knobs

Distributed jobs run long, so set these as needed: **priority** (higher can preempt lower),
**timeout** (max run time), **maxRetry** (auto-retry with checkpoint/restart), **dependencies**
(run after another workload), **isSupervised** (hang detection). Automatic fault recovery — a
failed node is drained and the job resumes from its latest checkpoint — is built in; see
[Fault tolerance](/concepts/fault-tolerance).

## Elastic / fault-tolerant variants

For elastic, group-based fault tolerance (groups can fail and recover independently), use
**TorchFT** instead of a plain PyTorchJob — see [Beyond training](/tasks/beyond-training).
