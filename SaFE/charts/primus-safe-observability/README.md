# primus-safe-observability

A self-contained, SaFE-owned metrics stack so SaFE can ship GA observability
without depending on the data-plane **primus-robust** addon.

## What it deploys

| Component | Chart | Purpose |
|-----------|-------|---------|
| VictoriaMetrics operator | `vmOperator` | Provides `VMCluster` / `VMAgent` CRDs + controller |
| VMCluster | `vmcluster` | `vminsert` / `vmselect` / `vmstorage` (the TSDB + query engine) |
| VMAgent | `vmagent` | Scrapes exporters, remote-writes **only** to this stack's `vminsert` |
| kube-state-metrics | `kubeStateMetrics` | Kubernetes object-state metrics |
| gpu / rdma / network exporters | `gpu-exporter`, ... | Node-level AMD GPU / RDMA / network metrics (DaemonSets) |

`device-exporter` and `telemetry-gateway` are intentionally **excluded** — they
power robust's workload-relabeled (`workload_gpu_*`) series, which is deferred
derived data. Node-level dashboards work without them.

## How SaFE consumes it

- Grafana Prometheus datasource points **directly** at
  `http://vmselect-primus-safe-vmcluster.primus-safe-observability.svc:8481/select/0/prometheus`
  (provisioned by resource-manager's `GrafanaDatasourceSyncer` when
  `observability.metrics.enable=true`).
- The primus-safe value `observability.metrics.endpoint` must match that DNS.

## Install / teardown (forward-compatible switch-back)

Installed as its own Helm release, gated by the operator during `install.sh`
when the metrics feature is enabled:

```bash
helm dependency build charts/primus-safe-observability
helm upgrade --install primus-safe-observability charts/primus-safe-observability \
  -n primus-safe-observability --create-namespace \
  --set global.clusterName=<cluster>
```

To switch back to primus-robust once it reaches GA:

```bash
helm uninstall primus-safe-observability -n primus-safe-observability
# then set observability.metrics.enable=false in primus-safe values
```

No SaFE code changes are needed either way — the direct-metrics path is fully
gated by `observability.metrics.enable`.

## Validation status / TODO

This chart was assembled by vendoring the robust subcharts and repointing them
at a SaFE namespace + vminsert. It still needs, in a real cluster with `helm`:

1. `helm dependency build` (unpacks the operator/KSM `.tgz` deps).
2. `helm template` render check.
3. Confirm the exporters' `ServiceMonitor` CRs are picked up by VMAgent — this
   requires the prometheus-operator `ServiceMonitor` CRD to exist so the VM
   operator can convert them to `VMServiceScrape` (or switch the exporters to
   emit `VMServiceScrape` / use vmagent `inlineScrapeConfig`).
4. Set real exporter image registry/tags.
