---
sidebar_position: 2
title: Run a multi-node distributed job
---

# Run a multi-node distributed job

Run training across **several nodes** when one node's GPUs aren't enough. This builds on
[Run a single-node training job](/tasks/run-single-node-training) — everything there still
applies; this page covers only the multi-node differences.

## From single node to multi node (web console)

In **Workloads → Training → PyTorch → Create PyTorch Job**, fill in the job as for a single node,
then in **Resources**:

- Set the number of **replicas** (or switch to **nodes** mode) to the number of nodes you want —
  e.g. `4`.
- Request the **full GPU count per replica** (e.g. `8`) so each replica occupies a whole node.

The platform creates one **master** and the rest as **workers**, starts them **together** (gang
scheduling — all-or-nothing, so partial allocations never hold GPUs idle), and places them with
**network locality** for fast collectives. You watch and manage it exactly like a single-node job
on the [workload detail page](/tasks/interact-with-your-job).

## Pin to specific nodes

By default the scheduler chooses nodes. To target particular machines, use **Specified nodes**
with a **node affinity** policy:

- **`required`** — the job runs *only* on the listed nodes (it stays pending until they're free).
- **`preferred`** — the scheduler prefers the listed nodes but may use others.

When you specify nodes, the replica counts follow automatically (for PyTorchJob: master = 1,
workers = the remaining nodes).

## Networking for collectives

Multi-node training depends on fast GPU-to-GPU communication:

- **Host networking** — for full-GPU multi-node jobs the platform enables host networking
  automatically (the API field is `forceHostNetwork`) so RDMA/RoCE traffic isn't slowed by
  overlay networking.
- **RDMA** — jobs request RDMA devices (`rdma/hca`) the same way they request GPUs.
- **NCCL/RCCL settings** — the cluster-wide `NCCL_SOCKET_IFNAME` / `NCCL_IB_HCA` defaults are set
  at install time (see [Install](/getting-started/install)). Override them per job with env vars
  (e.g. `NCCL_DEBUG=INFO`, or a different interface) when a node type differs.

## Reliability knobs

Distributed jobs run long, so set these as needed:

| Setting | What it does |
|---------|--------------|
| `priority` | Low / Medium / High — higher priorities can preempt lower ones. |
| `timeout` | Maximum run time before the job is stopped. |
| `maxRetry` | How many times to auto-retry on failure (works with checkpoint/restart). |
| `dependencies` | Run only after another workload finishes. |
| `isSupervised` | Hang detection — flags a job that stops making progress even if nothing crashed. |

Automatic fault recovery (a failed node is drained and the job resumes from its latest
checkpoint) is built in — see [Fault tolerance](/concepts/fault-tolerance).

## Elastic / fault-tolerant variants

For elastic, group-based fault tolerance (groups can fail and recover independently), use
**TorchFT** instead of a plain PyTorchJob — see [Beyond training](/tasks/beyond-training).

<!-- @test
scope: page
mode: behavior
priority: P1
personas: [member]
preconditions: [workspace-with-quota, multiple-ready-nodes]
do: in a workspace with quota, follow "From single node to multi node" to create a PyTorchJob with replicas/nodes = 2 (use a pullable image and a trivial entry point)
expect:
  - the job is accepted (appears in the PyTorch list, not Rejected)
  - on its detail/status it reaches Running with all replicas scheduled together (gang)
cleanup: delete the created workload via its row action
-->
