---
sidebar_position: 8
title: Troubleshooting
---

# Troubleshooting

A symptom → likely cause → fix runbook. Each entry links to the page with the full detail.

<!-- @test none: diagnostic runbook (prose) — nothing for the agent to execute. -->

## A job stays `Pending` and never runs

Usual causes:

- **No free quota** — the workspace's nodes are fully used. Check capacity and free up or add
  nodes ([Manage nodes](/administration/manage-nodes)).
- **Gang scheduling can't place all pods** — a multi-node job needs all replicas to fit at once.
  Reduce replicas/GPUs or free capacity.
- **Nodes are tainted/unhealthy** — the node agent tainted a node whose health check failed (see
  below), so the scheduler skips it.

For a quick functional test on hardware with expected taints, submit with `isTolerateAll: true`.

## A node shows unhealthy / tainted

The node agent runs health checks and applies a `primus-safe.<id>` taint when one fails — often on
a cluster that lacks the hardware a monitor expects (e.g. a WekaFS, NFS, or RDMA check on a node
that has none). Identify the failing monitor and either fix the hardware or disable that monitor —
see [Manage nodes → health monitors](/administration/manage-nodes). Automatic faults are covered in
[Pre-flight & in-flight monitoring](/administration/preflight-and-monitoring).

## OOM or NCCL errors in a job

- **OOM** — raise the job's `memory` (and `sharedMemory` for data loaders).
- **NCCL/RCCL can't connect (multi-node)** — the network interface or RDMA device names don't match
  the cluster default. Confirm `NCCL_SOCKET_IFNAME` / `NCCL_IB_HCA` (set at
  [install](/getting-started/install)) and override them per job if a node type differs (see
  [Run a multi-node distributed job](/tasks/run-multi-node-training)).

## Failover didn't resume from a checkpoint

Automatic recovery resumes from your **latest checkpoint**, so checkpoints must be written to
**workspace storage** (not a pod-local path). See [Fault tolerance](/concepts/fault-tolerance) and
[Storage & data](/concepts/storage-and-data).

## A workload's phase looks stuck

The console workload **phase can lag** the real pod state (e.g. it may read `Pending` after the pod
is already `Running` or finished). Confirm the true state from the workload's pod/detail before
assuming a failure.

## Install fails on a brand-new cluster

A fresh cluster needs the OpenSearch placeholder secret created before `install.sh` — see the
prerequisite note on the [Install](/getting-started/install) page.

## Pre-flight / Bench can't reach nodes

Bench and node management connect over SSH. Confirm passwordless SSH from the deploy host to every
node and that the registered **SSH secret** is correct (see
[Install](/getting-started/install) and [Manage nodes](/administration/manage-nodes)).
