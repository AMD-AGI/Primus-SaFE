---
sidebar_position: 6
title: Monitoring, logs & dashboards
---

# Monitoring, logs & dashboards

This page is about watching your workloads: reading their **logs**, viewing their **GPU
dashboards**, turning those features on (admin), and validating hardware with **Primus-Bench**.

<!-- @test
scope: page
mode: verify
priority: P2
targets: [console]
do: open a running workload's detail page and check its log views
expect:
  - the Pods tab exposes a per-pod live log view (works without the observability add-on)
  - the Logs (search) tab is present when opensearch.enable is on
-->

## View a workload's logs

Open the workload from its list (Training, Infer, Authoring, …) and go to its detail page:

- **Live pod logs** — the **Pods** tab → pick a pod → **Logs**. This tails the container output and
  works on any cluster, with nothing extra to install.
- **Search logs** — the **Logs** tab searches across the workload's pods, with history and filters.
  This needs observability enabled (below); when it is off, the tab is hidden or empty.

## View a workload's metrics (Grafana)

On the same detail page, open the **Metrics / Grafana** panel. You see GPU utilization, memory,
network, temperature, and throughput (Tflops/iteration) for the workload's pods — already filtered
to that workload. The workload and **System → Nodes** lists also show a GPU-utilization column.

If the panels are empty or show a "datasource not found" error, observability isn't fully set up —
see the next section.

<!-- @test
mode: behavior
priority: P2
personas: [member]
preconditions: [observability-installed]
do: on a running workload, search in the Logs tab and open the Metrics/Grafana panel
expect:
  - the Logs search returns matching log lines (not a robust-analyzer error)
  - the Grafana panel renders GPU/throughput metrics for the workload's pods
cleanup: none (read-only)
-->

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
