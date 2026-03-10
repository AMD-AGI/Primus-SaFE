# Component Probe and Alerts

This document describes the component health architecture: per-cluster **component-health-exporter**, VictoriaMetrics storage, and the API that aggregates health from VM.

## Architecture

- **component-health-exporter** (one Deployment per cluster): Probes workload controllers (Deployment, DaemonSet, StatefulSet) in the cluster using label selectors (`primus-lens-app-name`, `primus-safe-app-name`, and for kube-system `k8s-app`). Exposes Prometheus metrics on `/metrics` (health server port = httpPort + 1). No database or storage client; K8s only.
- **Metrics**: `primus_component_healthy` (0 or 1), `primus_component_replicas_desired`, `primus_component_replicas_ready` with labels `platform`, `app_name`, `namespace`, `kind`, `cluster`. Scraped by VMAgent and stored in the cluster’s VMCluster.
- **VMRule**: Deployed per cluster; expression `primus_component_healthy == 0` for 2m triggers `ComponentUnhealthy`. VMAlert evaluates and can send to telemetry-processor.
- **API**: `GET /api/v1/components/probe?cluster=xxx` does **not** call the K8s API. It uses `StorageClientSet.PrometheusRead` to query VictoriaMetrics for `primus_component_*` and returns the same JSON shape (kube-system + platform components). If `cluster` is omitted, the default cluster is used.

## Label usage

- **primus-lens-app-name**: Pod label on Lens components (api, jobs, node-exporter, gpu-resource-exporter, telemetry-processor, etc.). Exporter lists workloads with this label to report health.
- **primus-safe-app-name**: Pod label on Primus-SaFE components (e.g. primus-safe-adapter). Exporter lists workloads with this label for the `primus_safe` platform section.
- **Helm**: Use the `lens.podLabels` helper in `_helpers.tpl` to inject `primus-lens-app-name` (e.g. `{{ include "lens.podLabels" (dict "appName" "jobs" "root" .) }}`).

## Verification

1. Deploy component-health-exporter (dataplane and control-plane charts; bootstrap manifest). Ensure each cluster has one instance and Pod has Prometheus scrape annotations (port = httpPort + 1 for `/metrics`).
2. In VictoriaMetrics (or Grafana with VM datasource), query `primus_component_healthy` and `primus_component_replicas_desired` / `primus_component_replicas_ready` and confirm series per cluster.
3. Call `GET /api/v1/components/probe?cluster=<name>` and confirm the response matches the metrics (kube-system and platform sections).
4. To test alerting: scale a component to 0 or make it unhealthy; after 2m, `ComponentUnhealthy` should fire. Restore the component and confirm resolution.

## Related files

- Exporter: `Lens/modules/exporters/component-health-exporter/` (collector, bootstrap, main)
- API handler: `Lens/modules/api/pkg/api/unified_components_probe.go`
- VMRule: `Lens/deploy/metrics/rules/vmrule-component-health.yaml`
- Deploy: `Lens/bootstrap/manifests/app-component-health-exporter.yaml.tpl`, Helm `app-component-health-exporter.yaml` in dataplane and control-plane charts
