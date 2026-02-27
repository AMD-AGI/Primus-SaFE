# Component Probe and Alerts

This document describes the component liveness API, metrics, and alerting for kube-system core components and Primus-SaFE / Primus-Lens platform components.

## Overview

- **Unified API**: `GET /api/v1/components/probe` (or the configured API prefix) returns in one response:
  - **kube-system**: CoreDNS and NodeLocal DNS (desired/ready/healthy and per-pod status).
  - **platform**: Primus-SaFE components (by label `primus-safe-app-name`) and Primus-Lens components (by label `primus-lens-app-name`).
- **Metric**: `primus_lens_component_healthy` — gauge (1 = healthy, 0 = unhealthy) with labels `category`, `cluster`, `component`, `platform`, `app_name`, `namespace`. Emitted by the Lens API and scraped by VMAgent.
- **Alert**: `ComponentUnhealthy` — fired by VMAlert when `primus_lens_component_healthy == 0` for 2 minutes. Alerts are sent to the telemetry-processor and stored; they can be queried via the alerts API.

## Verification Steps (Alert Pipeline)

1. **Confirm metric in VictoriaMetrics**  
   After deployment, ensure the Lens API pod has Prometheus scrape annotations and is scraped by VMAgent. In VictoriaMetrics (or Grafana using the VM datasource), query for `primus_lens_component_healthy` and confirm time series exist.

2. **Trigger an unhealthy state**  
   For example:
   - Scale CoreDNS to 0: `kubectl scale deployment coredns -n kube-system --replicas=0`
   - Or delete a Primus-Lens component pod and do not let it recover (e.g. scale a Lens deployment to 0).

3. **Wait at least 2 minutes**  
   VMAlert evaluates every 30s and the rule has `for: 2m`, so the alert will fire after the condition holds for 2 minutes.

4. **Check VMAlert and alerts API**  
   - VMAlert: check firing alerts (e.g. VMAlert UI or VM query `ALERTS{alertname="ComponentUnhealthy"}`).
   - API: call `GET /api/v1/alerts` (or the equivalent alerts list endpoint) and confirm a `ComponentUnhealthy` alert appears with the expected labels (cluster, platform, component/app_name, namespace).

5. **Restore and confirm resolution**  
   Scale CoreDNS or the Lens component back up. After the metric returns to healthy, the alert should resolve and the alerts API should reflect the resolved state.

## Related Files

- API probe logic and types: `Lens/modules/api/pkg/api/component_probe.go`
- Unified endpoint: `Lens/modules/api/pkg/api/unified_components_probe.go`
- Metrics update loop: `Lens/modules/api/pkg/api/component_probe_metrics.go`
- Alert rule: `Lens/deploy/metrics/rules/vmrule-component-health.yaml`
- VMAlert notifier: `Lens/deploy/metrics/vmalert.yaml` (sends to telemetry-processor)
