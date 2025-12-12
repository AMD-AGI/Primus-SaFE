# Primus Lens Installation Guide

This guide covers the installation of Primus Lens components using Helm charts.

## Overview

Primus Lens consists of two main deployment targets:

| Component | Chart | Description |
|-----------|-------|-------------|
| **Control Plane** | `primus-lens-apps-control-plane` | API, Adapter, and Config Exporter services |
| **Data Plane** | `primus-lens-installer` | Full dataplane installation including operators, infrastructure, and apps |
| **Apps Only** | `primus-lens-apps-dataplane` | Data plane applications only (for updates) |

## Prerequisites

- Kubernetes cluster (v1.24+)
- Helm 3.x
- kubectl configured with cluster access

## Installation Scenarios

### 1. Install Control Plane

Use this when deploying the control plane components (API, Primus Safe Adapter, Multi-Cluster Config Exporter).

```bash
# Install control plane
helm install primus-lens-cp ./charts/primus-lens-apps-control-plane \
  -n primus-lens \
  --create-namespace

# Or with custom values
helm install primus-lens-cp ./charts/primus-lens-apps-control-plane \
  -n primus-lens \
  --create-namespace \
  -f my-values.yaml
```

**Components installed:**
- `primus-lens-api` - Main API service
- `primus-safe-adapter` - Primus Safe integration
- `multi-cluster-config-exporter` - Multi-cluster configuration management

### 2. Install Data Plane (Full Installation)

Use this for a complete dataplane deployment. This installs everything from scratch including operators, infrastructure (PostgreSQL, OpenSearch, VictoriaMetrics), and all applications.

```bash
# Full dataplane installation
helm install primus-lens ./charts/primus-lens-installer \
  -n primus-lens \
  --create-namespace

# With custom profile (minimal, normal, large)
helm install primus-lens ./charts/primus-lens-installer \
  -n primus-lens \
  --create-namespace \
  --set profile=normal
```

**Components installed:**
- **Operators**: VictoriaMetrics, Fluent, OpenSearch, PostgreSQL, Grafana, Kube-State-Metrics
- **Infrastructure**: PostgreSQL cluster, OpenSearch cluster, VictoriaMetrics cluster
- **Applications**: All dataplane apps (telemetry-collector, jobs, exporters, etc.)

### 3. Update Data Plane Apps Only

Use this when the infrastructure is already running and you only want to update/upgrade the dataplane applications.

```bash
# Update dataplane apps only
helm upgrade primus-lens-apps ./charts/primus-lens-apps-dataplane \
  -n primus-lens

# Or fresh install of apps only (infrastructure must exist)
helm install primus-lens-apps ./charts/primus-lens-apps-dataplane \
  -n primus-lens
```

**Components updated:**
- `telemetry-collector` - Log and metrics processing
- `jobs` - Job scheduling and management
- `node-exporter` - Node-level metrics (DaemonSet)
- `gpu-resource-exporter` - GPU metrics
- `system-tuner` - System optimization (DaemonSet)
- `ai-advisor` - AI recommendations

## Quick Reference

| Scenario | Command |
|----------|---------|
| New control plane | `helm install primus-lens-cp ./charts/primus-lens-apps-control-plane -n primus-lens --create-namespace` |
| New data plane (full) | `helm install primus-lens ./charts/primus-lens-installer -n primus-lens --create-namespace` |
| Update data plane apps | `helm upgrade primus-lens-apps ./charts/primus-lens-apps-dataplane -n primus-lens` |

## Uninstallation

```bash
# Uninstall control plane
helm uninstall primus-lens-cp -n primus-lens

# Uninstall data plane
helm uninstall primus-lens -n primus-lens

# Delete namespace (removes all resources)
kubectl delete namespace primus-lens
```

## Configuration

Each chart has its own `values.yaml` file with configurable options. Key configurations include:

- `global.clusterName` - Cluster identifier
- `global.namespace` - Target namespace
- `global.storageClass` - Storage class for persistent volumes
- `profile` - Resource profile (minimal/normal/large) for installer chart

Refer to individual chart's `values.yaml` for complete configuration options.

