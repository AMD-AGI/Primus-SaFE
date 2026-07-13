---
sidebar_position: 1
title: GPU metrics & dashboards
---

# GPU metrics & dashboards

Primus-SaFE ships a built-in, self-contained metrics stack: per-GPU and per-workload
utilization, memory, power, temperature, PCIe and RDMA throughput, rendered as Grafana
dashboards inside the console. It runs on Primus-SaFE's own components (VictoriaMetrics plus a
metrics enricher) and needs **no external observability add-on**.

<!-- @test
scope: page
mode: verify
priority: P1
targets: [console]
do: open a workload's detail page and check its Metrics (Grafana) panel, and the Homepage GPU chart
expect:
  - the workload detail page exposes a Metrics (Grafana) panel/tab
  - the Homepage shows a "GPU Utilization & Allocation" chart
-->

## Where to check your metrics

Everything is in the console once metrics are enabled (see [below](#enable-metrics-admin-one-time)):

- **Per-workload dashboard** — open a workload from its list (Training, Authoring, Infer, …) and
  open its **Metrics** (Grafana) panel on the detail page. You get GPU utilization, memory used,
  socket power, junction/memory temperature, PCIe bandwidth, per-pod CPU/memory/IO, and RDMA
  throughput, already filtered to that workload's pods.
- **Workspace Homepage** — the **GPU Utilization & Allocation** chart and the resource cards
  summarize the workspace's GPU usage over time.
- **Node view** — **System → Nodes** shows a per-node GPU-utilization column.

Under the hood the per-workload panel is the Grafana dashboard `training-workload`, filtered by
the workload's UID; the metrics enricher labels every GPU series with its owning workload so the
panels resolve automatically, with no per-workload configuration.

<!-- @test
mode: behavior
priority: P1
personas: [member]
preconditions: [observability-installed, running-cluster, workspace-with-quota]
do: submit a GPU training job by following getting-started/first-training-job "Submit a job (console)", wait until it is Running, then open its detail page Metrics (Grafana) panel
expect:
  - within a few minutes the GPU panels (utilization/memory/power) render non-empty series for the job's pods
  - the panels are scoped to this workload only (no other workloads' GPUs)
cleanup: delete the created workload via its row action
-->

## Enable metrics (admin, one-time)

Metrics are off by default. Turn them on once per platform; there is no per-workload setup
afterwards.

### During install

When you run `bootstrap/install.sh`, answer **`y`** to:

```text
install SaFE observability metrics stack (VictoriaMetrics + enricher) ? (y/n)
```

That one answer does everything: it installs the **`primus-safe-observability`** stack (its own
Helm release in the `primus-safe-observability` namespace: VictoriaMetrics + vmagent + the AMD
GPU/RDMA/network exporters + the metrics enricher), and it turns on Grafana and the direct-metrics
path in the platform (`grafana.enable=true`, `observability.metrics.enable=true`).

### On an already-installed platform

If the platform is already up, enable it explicitly:

1. Install the observability stack:
   ```bash
   helm dependency build SaFE/charts/primus-safe-observability
   helm upgrade --install primus-safe-observability SaFE/charts/primus-safe-observability \
     -n primus-safe-observability --create-namespace \
     --set global.clusterName=<cluster-name>
   ```
   `<cluster-name>` must match the Primus-SaFE Cluster name so Grafana resolves its datasource.
2. In the platform values set `grafana.enable=true` and `observability.metrics.enable=true`, then
   re-run `bootstrap/upgrade.sh` (or `helm upgrade` the `primus-safe` release).

Once installed, the per-workload dashboards and Homepage charts light up automatically.

:::note Self-contained, no primus-robust
This metrics path is Primus-SaFE's own; it does not depend on the primus-robust data-plane add-on.
All images are published by Primus-SaFE (`primussafe/*`). Log search is documented separately in
[Monitoring, logs & dashboards](/administration/observability) and will move into this
Observability section alongside metrics.
:::

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Metrics panel shows **504** | Grafana isn't enabled — set `grafana.enable=true` and upgrade. |
| Grafana loads but **"datasource not found"** | The observability stack isn't installed, or `global.clusterName` doesn't match the Cluster name. |
| Panels render but stay **empty** | No workload is scheduled on GPUs yet, or the exporters/enricher aren't running in `primus-safe-observability`. |
| GPU-utilization column shows `-` | No metrics collected yet (exporters or enricher not up). |

<!-- @test
mode: verify
priority: P2
targets: [console]
do: with metrics NOT installed, open a workload's Metrics panel
expect:
  - the panel surfaces a clear "datasource not found" / not-configured state rather than a blank crash
-->
