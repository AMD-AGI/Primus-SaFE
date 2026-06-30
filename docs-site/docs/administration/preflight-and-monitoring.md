---
sidebar_position: 4
title: Pre-flight & in-flight monitoring
---

# Pre-flight & in-flight monitoring

The goal is **goodput** — keeping GPU time on useful work. You protect it on two timelines:
validate hardware **before** a production run, and watch health **while** it runs so faults are
caught and recovered.

## Pre-flight checks

A pre-flight check runs a test container against a target (a cluster, a workspace, or specific
nodes) and reports the result, so you catch bad hardware before a big job lands on it.

In the console, go to **System → Bench** and click **Create Bench**:

1. Set a **name**, an **image**, and the **entry point** (the check to run).
2. Choose the **Type** (node / cluster / workspace) and the **Value** (the specific target), and
   set the **resources** for the check.
3. Optionally enable **Toleration** so the check can run on nodes that are already tainted, and set
   a **Timeout**.
4. **Submit**, then track it in the list — its **Phase** moves to `Succeeded` or `Failed`. Open the
   job to read its report.

![Create Bench (pre-flight) form](/img/screenshots/preflight-create-form.png)

<!-- @test
scope: page
mode: behavior
priority: P1
personas: [admin]
preconditions: [running-cluster, workspace-with-quota]
do: System > Bench > Create Bench; pick Type = workspace and the test workspace, a pullable image and a trivial entry point; enable Toleration; Submit
expect:
  - the job appears in the Bench list and is accepted (phase Pending/Running, not Rejected)
cleanup: delete the bench job via its row action
-->

For deeper, standalone hardware/performance benchmarking outside the platform, use **Primus-Bench**
(bare-metal / SLURM / Kubernetes) — see
[`Bench/README.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/Bench/README.md).

## In-flight monitoring

### Faults

The **Node Agent** continuously checks each node. When a check fails it raises a **Fault**, which
**taints** the node so the scheduler stops placing work there; fault-tolerant jobs then fail over
to healthy capacity.

Review them in the console under **System → Faults** — each fault shows the **node**, an **error
ID** (the failing check, e.g. `201` networking, `309` storage CSI), the **action** taken (e.g.
`taint`), and timestamps. When the underlying issue is fixed the fault clears and the taint is
removed automatically; you can also resolve a fault from its row actions.

<!-- @test
scope: page
mode: contract
priority: P1
personas: [admin]
preconditions: [running-cluster]
do: open System > Faults (read-only)
expect:
  - faults are listed, each with a node, an error ID, the action taken (e.g. taint), and a creation time
-->

Which health checks run (and how to turn off ones your hardware doesn't need) is covered in
[Manage nodes → health monitors](/administration/manage-nodes). The recovery model is in
[Fault tolerance](/concepts/fault-tolerance).

### Dashboards

The platform ships **Grafana** dashboards for cluster- and job-level health (GPU utilization,
memory, network, temperatures); per workload it also reports `avgGpuUsage`. Open them from the
console.

### Hang detection

Long jobs can stop making progress without crashing. Enable **hang detection** on a workload (the
`isSupervised` option) so the platform flags a job that has stalled, even when nothing has failed.
