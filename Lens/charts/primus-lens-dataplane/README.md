# Primus Lens Helm Chart

Primus Lens is an AI training platform observability and monitoring solution that provides comprehensive insights into your training workloads.

## 🎯 Features

- 📊 **Metrics Collection**: VictoriaMetrics for high-performance metrics storage
- 📝 **Log Aggregation**: OpenSearch for powerful log search and analytics  
- 📈 **Visualization**: Grafana with pre-configured dashboards
- 🗄️ **Database**: PostgreSQL for metadata and state management
- 🔄 **Auto-scaling**: Kubernetes-native deployment with operator pattern
- 🚀 **Production-ready**: Multiple profiles for different cluster sizes
- 🎛️ **Smart Orchestration**: Helm Hooks ensure correct deployment order

## 📋 Prerequisites

- Kubernetes 1.24+
- Helm 3.8+
- StorageClass configured for persistent volumes
- (Optional) Ingress controller for external access
- Recommended: 30+ minutes for full deployment

## 🚀 Quick Start

### 1. Add Dependencies

```bash
cd Lens/charts
helm dependency update
```

### 2. Install with Default Configuration

```bash
helm install primus-lens . \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m \
  --wait
```

### 3. Access the Console

For SSH tunnel access (default):

```bash
# Web Console
kubectl port-forward -n primus-lens svc/primus-lens-web 30180:80
# Open: http://localhost:30180

# Grafana
kubectl port-forward -n primus-lens svc/grafana-service 30182:3000
# Open: http://localhost:30182/grafana
# Default credentials: admin / admin
```

## 📊 Deployment Order

**Important**: The chart uses a carefully orchestrated deployment order to ensure dependencies are met:

```
Phase 0: Namespace & Secrets (pre-install hooks)
    ↓
Phase 1: Operators (6 sub-charts auto-deployed)
    ↓
Phase 2: Wait for Operators (pre-install hook)
    ↓
Phase 3: Infrastructure CRs (PostgreSQL, OpenSearch, VictoriaMetrics)
    ↓
Phase 4: Wait for Infrastructure Ready (post-install hook) ⭐ NEW
    ↓
Phase 5: Database Initialization (post-install hook)
    ↓
Phase 6: Application Components (API, Web, Exporters)
    ↓
Phase 7: Monitoring (FluentBit, VMAgent) (post-install hook) ⭐ NEW
    ↓
Phase 8: Grafana & Ingress
```

**Why this order?**
- FluentBit and VMAgent depend on `telemetry-processor` app being ready
- Database initialization requires PostgreSQL pods to be running
- Apps need database schema to be initialized

See [DEPLOYMENT_ORDER.md](DEPLOYMENT_ORDER.md) for detailed flow.

## ⚙️ Configuration

### Profile Selection

Choose a profile based on your cluster size:

| Profile | Use Case | OpenSearch Disk | PG Data | VM Storage |
|---------|----------|----------------|---------|------------|
| `minimal` | Testing, small clusters | 30Gi | 20Gi | 30Gi |
| `normal` | Production, moderate workloads | 50Gi | 50Gi | 50Gi |
| `large` | Large-scale production | 100Gi | 100Gi | 100Gi |

```bash
helm install primus-lens . \
  --set profile=large \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m
```

### Custom Values

Create a custom values file:

```yaml
# custom-values.yaml
global:
  clusterName: "my-cluster"
  storageClass: "fast-ssd"
  accessType: "ingress"
  domain: "example.com"

profile: "normal"

apps:
  api:
    replicas: 3
```

Install with custom values:

```bash
helm install primus-lens . \
  -f custom-values.yaml \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m
```

### Environment-specific Deployments

#### Development

```bash
helm install primus-lens-dev . \
  -f values-dev.yaml \
  --namespace primus-lens-dev \
  --create-namespace \
  --timeout 30m
```

#### Production

```bash
# Set sensitive values via command line
helm install primus-lens . \
  -f values-prod.yaml \
  --set global.imagePullSecrets[0].credentials.username=$DOCKER_USER \
  --set global.imagePullSecrets[0].credentials.password=$DOCKER_PASS \
  --set grafana.adminPassword=$GRAFANA_PASS \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m \
  --wait
```

## 📝 Configuration Parameters

### Global Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.clusterName` | Cluster identifier | `my-cluster` |
| `global.namespace` | Target namespace | `primus-lens` |
| `global.storageClass` | StorageClass for PVCs | `local-path` |
| `global.accessMode` | Access mode for volumes | `ReadWriteOnce` |
| `global.imageRegistry` | Container image registry | `docker.io` |
| `global.accessType` | Access method (`ssh-tunnel` or `ingress`) | `ssh-tunnel` |
| `global.domain` | Domain for ingress | `lens-primus.ai` |

### Application Components

| Parameter | Description | Default |
|-----------|-------------|---------|
| `apps.api.enabled` | Enable API service | `true` |
| `apps.api.replicas` | Number of API replicas | `2` |
| `apps.web.enabled` | Enable web console | `true` |
| `apps.nodeExporter.enabled` | Enable node exporter | `true` |

### Infrastructure Components

| Parameter | Description | Default |
|-----------|-------------|---------|
| `database.enabled` | Enable PostgreSQL | `true` |
| `opensearch.enabled` | Enable OpenSearch | `true` |
| `victoriametrics.enabled` | Enable VictoriaMetrics | `true` |
| `grafana.enabled` | Enable Grafana | `true` |

## 🔄 Upgrading

### Upgrade to New Version

```bash
helm upgrade primus-lens . \
  -f values.yaml \
  --namespace primus-lens \
  --timeout 30m
```

### Change Configuration

```bash
helm upgrade primus-lens . \
  --set profile=large \
  --set apps.api.replicas=5 \
  --namespace primus-lens
```

## ↩️ Rollback

```bash
# List release history
helm history primus-lens -n primus-lens

# Rollback to previous version
helm rollback primus-lens -n primus-lens

# Rollback to specific revision
helm rollback primus-lens 3 -n primus-lens
```

## 🗑️ Uninstallation

```bash
# Uninstall release
helm uninstall primus-lens -n primus-lens

# Delete namespace (this will delete all PVCs!)
kubectl delete namespace primus-lens
```

## 🔍 Troubleshooting

### Check Deployment Status

```bash
# Overall status
helm status primus-lens -n primus-lens

# Check pods
kubectl get pods -n primus-lens

# Check operators
kubectl get pods -n primus-lens | grep operator
```

### Check Initialization Jobs

```bash
# List jobs
kubectl get jobs -n primus-lens

# Check wait-operators job
kubectl logs -n primus-lens job/primus-lens-wait-operators

# Check wait-infrastructure job (NEW)
kubectl logs -n primus-lens job/primus-lens-wait-infrastructure

# Check postgres-init job
kubectl logs -n primus-lens job/primus-lens-postgres-init
```

### Common Issues

#### Operators Not Ready

Check operator pods:
```bash
kubectl get pods -n primus-lens -l app.kubernetes.io/component=operator
kubectl describe pod <operator-pod-name> -n primus-lens
```

#### Infrastructure Not Ready (NEW Issue)

If `wait-infrastructure` job times out:
```bash
# Check PostgreSQL
kubectl get postgrescluster -n primus-lens
kubectl describe postgrescluster primus-lens -n primus-lens

# Check OpenSearch  
kubectl get opensearchcluster -n primus-lens
kubectl describe opensearchcluster primus-lens-logs -n primus-lens

# Check VictoriaMetrics
kubectl get vmcluster -n primus-lens
kubectl describe vmcluster primus-lens-vmcluster -n primus-lens
```

#### Database Initialization Failed

Check PostgreSQL cluster:
```bash
kubectl get postgrescluster -n primus-lens
kubectl describe postgrescluster primus-lens -n primus-lens
kubectl logs -n primus-lens job/primus-lens-postgres-init
```

#### Storage Issues

Check PVCs:
```bash
kubectl get pvc -n primus-lens
kubectl describe pvc <pvc-name> -n primus-lens
```

#### Image Pull Errors

Update image pull secret:
```bash
kubectl create secret docker-registry primus-lens-image \
  --docker-server=docker.io \
  --docker-username=<username> \
  --docker-password=<password> \
  -n primus-lens \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────┐
│                  Primus Lens                    │
├─────────────────────────────────────────────────┤
│                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐     │
│  │   API    │  │   Jobs   │  │Telemetry │     │
│  │ Service  │  │ Service  │  │Collector │     │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘     │
│       │             │              │           │
│  ┌────┴─────────────┴──────────────┴─────┐    │
│  │            PostgreSQL                  │    │
│  └────────────────────────────────────────┘    │
│                                                 │
│  ┌──────────────┐    ┌─────────────────┐      │
│  │ VictoriaMetrics│    │  OpenSearch     │      │
│  │   (Metrics)    │    │    (Logs)       │      │
│  └────────┬───────┘    └────────┬────────┘      │
│           │                     │               │
│  ┌────────┴─────────────────────┴────────┐     │
│  │              Grafana                   │     │
│  └────────────────────────────────────────┘     │
│                                                 │
└─────────────────────────────────────────────────┘

Deployment Flow:
1. Operators → 2. Infrastructure CRs → 3. Wait Ready →
4. Init DB → 5. Apps → 6. Monitoring → 7. Grafana
```

## 📚 Documentation

- [QUICKSTART.md](QUICKSTART.md) - 5-minute quick start guide
- [DEPLOYMENT_ORDER.md](DEPLOYMENT_ORDER.md) - Detailed deployment flow
- [DEPLOYMENT_SUMMARY.md](DEPLOYMENT_SUMMARY.md) - Implementation summary
- [STRUCTURE.md](STRUCTURE.md) - Directory structure
- [templates/README.md](templates/README.md) - Templates directory guide
- [DIRECTORY_RESTRUCTURE_SUMMARY.md](DIRECTORY_RESTRUCTURE_SUMMARY.md) - Directory restructure notes
- [CHANGELOG.md](CHANGELOG.md) - Version history
- [Makefile](Makefile) - Convenient commands

## 🛠️ Makefile Commands

Use `make` for convenient operations:

```bash
make help           # Show all commands
make deps           # Download dependencies
make install        # Install with defaults
make install-dev    # Install dev environment
make upgrade        # Upgrade release
make status         # Show status
make logs-init      # View init job logs
make port-forward-web      # Port forward to web console
make port-forward-grafana  # Port forward to Grafana
```

## 🤝 Support

For issues and questions:
- GitHub Issues: https://github.com/AMD-AGI/Primus-SaFE/issues
- Design Document: [HELM_REFACTOR_DESIGN.md](../bootstrap/HELM_REFACTOR_DESIGN.md)
- Quick Start: [QUICKSTART.md](QUICKSTART.md)

## 📄 License

See [LICENSE](../../LICENSE) file for details.

## ✨ What's New

### Latest Changes

- ⭐ **Improved Deployment Order**: Added `wait-for-infrastructure` job to ensure PostgreSQL, OpenSearch, and VictoriaMetrics are ready before database initialization
- ⭐ **Smart Monitoring Deployment**: FluentBit and VMAgent now deploy after applications (depends on telemetry-processor)
- ✅ Better error messages and troubleshooting
- ✅ More reliable deployment with proper dependency handling

See [CHANGELOG.md](CHANGELOG.md) for full history.
