---
sidebar_position: 6
title: Monitoring, logs & dashboards
---

# Monitoring, logs & dashboards

This page is about watching your workloads: reading their **logs**, viewing their **GPU
dashboards**, turning those features on (admin), and validating hardware with **Primus-Bench**.

This page serves two audiences at once:

- **For you (the reader):** each section says where to click and what a healthy view looks like,
  from live pod logs to Grafana panels.
- **For an AI agent:** there's a concrete **presence check** that works on any cluster (live pod
  logs + the Logs tab) and a richer **behavior check** that needs the observability add-on
  installed — each with its own pass/fail outcome in plain language. No hidden test layer —
  bookkeeping (priority, personas, known drift) lives in the run contract
  `docs-site/AGENTS.md`.

## View a workload's logs

Open the workload from its list (Training, Infer, Authoring, …) and go to its detail page:

- **Live pod logs** — the **Pods** tab → pick a pod → **Logs**. This tails the container output and
  works on any cluster, with nothing extra to install.
- **Search logs** — the **Logs** tab searches across the workload's pods, with history and filters.
  This needs observability enabled (below); when it is off, the tab is hidden or empty.

### Presence check (works without the add-on)

On a **running workload's** detail page:

- **Healthy (pass):** the **Pods** tab exposes a **per-pod live log view** (this works without the
  observability add-on), and the **Logs** (search) tab is **present** when `opensearch.enable` is
  on.
- **If instead** the Pods tab has no per-pod log view at all, that's a fail. The **Logs** tab
  being absent is only a fail when `opensearch.enable` is on — otherwise it's expected to be
  hidden (see below).

> **Before you start:** a running workload to open. If none is running, report **BLOCKED**
> (missing a running workload) rather than failing. **Agent:** presence-check the two log views
> and report PASS/FAIL. No cleanup — this is read-only.

## View a workload's metrics (Grafana)

On the same detail page, open the **Metrics / Grafana** panel. You see GPU utilization, memory,
network, temperature, and throughput (Tflops/iteration) for the workload's pods — already filtered
to that workload. The workload and **System → Nodes** lists also show a GPU-utilization column.

If the panels are empty or show a "datasource not found" error, observability isn't fully set up —
see the next section.

### Behavior check (needs observability installed)

This deeper check confirms logs and metrics actually **render data**, not just that the controls
exist. It requires the observability stack (below) to be installed on the workload's cluster.

> **Before you start:** the `observability-installed` precondition — **primus-robust** plus
> Grafana/GPU exporter on the data cluster. **Agent:** if the add-on is not installed (Logs search
> errors with a `robust-analyzer` message, or the Grafana panel is empty / "datasource not
> found"), report **BLOCKED** (missing `observability-installed`) — do **not** fail the run for a
> missing fixture.

On a running workload with observability installed:

- **Healthy (pass):** searching in the **Logs** tab returns matching log lines (not a
  robust-analyzer error), and the **Grafana** panel renders GPU/throughput metrics for the
  workload's pods.
- **If instead** the Logs search errors or the Grafana panel stays empty *after* you've confirmed
  the add-on is installed, that's a fail (not a BLOCKED). No cleanup — this is read-only.

## Enable observability (admin, one-time)

Log search and dashboards are served by an observability stack. Turn it on once per platform +
data cluster:

1. **Turn on Grafana** — set `grafana.enable=true` (the platform already runs `grafana-operator`).
2. **Install the GPU metrics exporter** — add the **`amd-gpu-operator`** add-on on each data
   cluster from **System → Addons**; it ships the AMD GPU device-metrics exporter.
3. **Install the observability add-on** — deploy **primus-robust** on each data cluster (it collects
   logs and GPU metrics from your workloads). It is provided separately from the core platform.
4. **Point Grafana at each cluster** — add an entry under `grafana.dataClusters` with the cluster's
   `name` (must match the cluster name) and its `vmSelectUrl` (the metrics endpoint).

Once installed, log search and per-workload dashboards light up automatically — there is no
per-workload configuration. (`opensearch.enable` controls whether the **Logs** tab is shown.)

Node **health and performance** validation with Primus-Bench is a separate concern from live
monitoring — see [Pre-flight & in-flight monitoring](/administration/preflight-and-monitoring).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| **Logs** tab missing | `opensearch.enable` is off — turn it on. |
| Log search errors / empty (mentions `robust-analyzer`) | The **primus-robust** add-on isn't installed/reachable on that workload's cluster. |
| Grafana panel shows **504** | Grafana isn't enabled — set `grafana.enable=true`. |
| Grafana loads but **"datasource not found"** | Add the cluster to `grafana.dataClusters` with `name` = the cluster name. |
| Panels empty (datasource resolves) | The `amd-gpu-operator` exporter or primus-robust isn't running on the data cluster. |
| GPU-utilization column shows `-` | No metrics collected yet (exporter / primus-robust not up). |

> **Not yet covered (capture so we don't lose it):**
> - [ ] Console screenshots for installing the `amd-gpu-operator` and primus-robust add-ons.
> - [ ] The graceful "observability not installed" empty state (planned; error `Primus.00050`).
> - [ ] Aligning the `grafana.enable` default between fresh install (off) and upgrade (on).
