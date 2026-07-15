---
sidebar_position: 4
title: Pre-flight & in-flight monitoring
---

# Pre-flight & in-flight monitoring

The goal is **goodput** — keeping GPU time on useful work. You protect it on two timelines:
validate hardware **before** a production run, and watch health **while** it runs so faults are
caught and recovered.

This page serves two audiences at once:

- **For you (the reader):** each section says what to do and what a healthy result looks like,
  from running a pre-flight check to reading faults.
- **For an AI agent:** there are two concrete flows below — a **pre-flight check submit** (perform
  it) and a **read-only Faults check** — each with its own pass/fail outcome in plain language.
  No hidden test layer — bookkeeping (priority, personas, known drift) lives in the run contract
  `docs-site/AGENTS.md`.

## Pre-flight checks — submit a bench (golden path)

A pre-flight check runs a test container against a target (a cluster, a workspace, or specific
nodes) and reports the result, so you catch bad hardware before a big job lands on it.

### Before you start

- A **running cluster** you can reach, signed in as an **admin**.
- A **workspace with quota** to target, and a **pullable image** for the check.

> **Agent:** confirm both preconditions before Step 1. If there is no reachable cluster, no
> workspace with quota, or no pullable image, report **BLOCKED** (naming the missing one:
> `running-cluster` / `workspace-with-quota` / pullable image) rather than failing a step.

### Steps

In the console, go to **System → Bench** and click **Create Bench**:

1. Set a **name** (use a unique suffix for a repeatable run), an **image**, and the **entry
   point** (the check to run). A trivial entry point is fine for a smoke test — the point is that
   the platform *accepts and schedules* the check.
2. Choose the **Type** (node / cluster / workspace) and the **Value** (the specific target), and
   set the **resources** for the check. For this walkthrough pick **Type = workspace** and your
   test workspace.
3. Enable **Toleration** so the check can run on nodes that are already tainted, and set a
   **Timeout**.
4. **Submit**, then track it in the list — its **Phase** moves to `Succeeded` or `Failed`. Open
   the job to read its report.

![Create Bench (pre-flight) form](/img/screenshots/preflight-create-form.png)

:::note The check runs whatever your image does
A pre-flight check is only as good as the **image** you point it at — the platform runs the
container and reports pass/fail, but it does not ship a generic check image. In practice the check
image often has to be tailored to a cluster's hardware. If you don't have a suitable image,
prefer **Primus-Bench** (below), which ships ready-made node health/performance checks.
:::

### What you should see

- **Healthy (pass):** the job appears in the **Bench** list and is **accepted** — its phase is
  `Pending`/`Running` (and later `Succeeded`/`Failed`), not **Rejected**. Acceptance is the
  outcome this walkthrough checks; the check's own report tells you about the hardware.
- **If instead** the submission is **Rejected** or throws an error toast, that's usually an input
  problem (unpullable image, no quota, no schedulable target) — fix the input and resubmit.

> **Agent:** fill the table, show it, and report **PASS** only if the bench job was accepted
> (phase `Pending`/`Running`, not `Rejected`). Then **run cleanup**: delete the bench job via its
> row action.

| Check | Healthy result | Found |
|---|---|---|
| Bench job appears in Bench list | yes | _fill in_ |
| Phase after submit | Pending / Running (not Rejected) | _fill in_ |
| Cleanup (bench job deleted) | done | _fill in_ |

For deeper, standalone hardware/performance benchmarking outside the platform, use **Primus-Bench**
(bare-metal / SLURM / Kubernetes) — see
[`Bench/README.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/Bench/README.md).

## In-flight monitoring

### Faults — read-only check

The **Node Agent** continuously checks each node. When a check fails it raises a **Fault**, which
**taints** the node so the scheduler stops placing work there; fault-tolerant jobs then fail over
to healthy capacity.

Review them in the console under **System → Faults** — each fault shows the **node**, an **error
ID** (the failing check, e.g. `201` networking, `309` storage CSI), the **action** taken (e.g.
`taint`), and timestamps. When the underlying issue is fixed the fault clears and the taint is
removed automatically; you can also resolve a fault from its row actions.

This is a **read-only** check — open the page, don't resolve or mutate anything.

- **Before you start:** a **running cluster** you can reach, signed in as an **admin**. If not,
  report **BLOCKED** (missing `running-cluster` / admin access).
- **Healthy (pass):** the **Faults** page lists faults, each with a **node**, an **error ID**,
  the **action** taken (e.g. `taint`), and a **creation time**. An empty list is also healthy on
  a cluster with no current faults — the pass condition is that the documented columns are
  present and populated when faults exist.
- **If instead** the Faults page is missing, errors, or omits the node / error ID / action /
  timestamp columns, that's a fail.

> **Agent:** presence-check the Faults list and its columns; report PASS/FAIL from what you see.
> No cleanup — this check creates nothing and does not resolve faults.

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
