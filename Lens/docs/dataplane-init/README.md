# Dataplane Initialization Architecture

This document describes the redesigned architecture for Primus-Lens dataplane initialization.

## Overview

The dataplane initialization process deploys the storage infrastructure (PostgreSQL, OpenSearch, VictoriaMetrics) and applications to a target Kubernetes cluster. The installer runs as a Job in the Control Plane cluster and uses the target cluster's kubeconfig to deploy resources.

## Current Architecture Issues

| Issue | Description | Impact |
|-------|-------------|--------|
| **Monolithic Charts** | All operators bundled in one chart | One failure affects all |
| **Mixed Wait Logic** | Wait logic embedded in Execute | Hard to handle "not found" vs "not ready" |
| **Deep Config Nesting** | API Request → Task → InstallConfig → Values | Config gets lost in translation |
| **Implicit Dependencies** | Stages assume previous stages succeeded | Race conditions, missing resources |
| **Resource Conflicts** | CP/DP share cluster, ClusterRole conflicts | Manual intervention required |

## New Architecture

### Design Principles

1. **Single Responsibility**: Each stage does one thing well
2. **Explicit Dependencies**: Prerequisites checked before execution
3. **Observable**: Clear success/failure states for each step
4. **Idempotent**: Any step can be safely retried
5. **Declarative First**: Use CRs directly when possible, Helm for complex charts

### Phase Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Control Plane                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐     ┌──────────────────┐     ┌─────────────────────────┐  │
│  │  Frontend   │────▶│  primus-lens-api │────▶│  PostgreSQL (CP DB)     │  │
│  └─────────────┘     └────────┬─────────┘     └─────────────────────────┘  │
│                               │                            ▲                │
│                               │ Create Task                │                │
│                               ▼                            │                │
│                    ┌──────────────────────┐                │                │
│                    │  control-plane-      │                │                │
│                    │  controller          │────────────────┘                │
│                    │  (polls every 30s)   │   Read config                   │
│                    └──────────┬───────────┘                                 │
│                               │                                             │
│                               │ Creates K8s Job                             │
│                               ▼                                             │
│                    ┌──────────────────────┐                                 │
│                    │   Installer Job      │                                 │
│                    │   (runs stages)      │                                 │
│                    └──────────┬───────────┘                                 │
│                               │                                             │
└───────────────────────────────┼─────────────────────────────────────────────┘
                                │
                                │ Uses kubeconfig from DB
                                ▼
┌───────────────────────────────────────────────────────────────────────────────┐
│                           Dataplane Cluster                                    │
├───────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│   Phase 1: Prerequisites                                                      │
│   ┌─────────────────────────────────────────────────────────────────────────┐│
│   │ 1.1 namespace      - Create primus-lens namespace                       ││
│   │ 1.2 rbac           - Create ServiceAccount, Roles                       ││
│   │ 1.3 pull-secrets   - Create image pull secrets (optional)               ││
│   └─────────────────────────────────────────────────────────────────────────┘│
│                                    │                                          │
│                                    ▼                                          │
│   Phase 2: Operators (installed independently)                                │
│   ┌─────────────────────────────────────────────────────────────────────────┐│
│   │ 2.1 pgo-operator           ─┐                                           ││
│   │ 2.2 victoriametrics-operator│─ Each: Check → Install/Skip → Wait       ││
│   │ 2.3 opensearch-operator    ─┤                                           ││
│   │ 2.4 grafana-operator       ─┤                                           ││
│   │ 2.5 fluent-operator        ─┘                                           ││
│   └─────────────────────────────────────────────────────────────────────────┘│
│                                    │                                          │
│                                    ▼                                          │
│   Phase 3: Storage Infrastructure (deployed independently)                    │
│   ┌─────────────────────────────────────────────────────────────────────────┐│
│   │ 3.1 postgres         - Apply PostgresCluster CR → Wait for healthy      ││
│   │ 3.2 victoriametrics  - Apply VMCluster CR → Wait for healthy            ││
│   │ 3.3 opensearch       - Apply OpenSearchCluster CR → Wait for healthy    ││
│   └─────────────────────────────────────────────────────────────────────────┘│
│                                    │                                          │
│                                    ▼                                          │
│   Phase 4: Database Setup                                                     │
│   ┌─────────────────────────────────────────────────────────────────────────┐│
│   │ 4.1 database-init      - Create databases, users, permissions           ││
│   │ 4.2 database-migration - Run SQL migrations                             ││
│   └─────────────────────────────────────────────────────────────────────────┘│
│                                    │                                          │
│                                    ▼                                          │
│   Phase 5: Applications                                                       │
│   ┌─────────────────────────────────────────────────────────────────────────┐│
│   │ 5.1 storage-secret   - Create connection info secret                    ││
│   │ 5.2 core-apps        - Deploy API, Jobs, Grafana                        ││
│   │ 5.3 monitoring       - Deploy Fluent-bit, Dashboards                    ││
│   └─────────────────────────────────────────────────────────────────────────┘│
│                                                                               │
└───────────────────────────────────────────────────────────────────────────────┘
```

## Detailed Stage Design

### Stage Interface

```go
type Stage interface {
    // Name returns the unique stage identifier
    Name() string
    
    // CheckPrerequisites verifies all dependencies are met
    // Returns list of missing prerequisites
    CheckPrerequisites(ctx context.Context, client ClusterClient) ([]string, error)
    
    // ShouldRun checks if this stage needs to execute (idempotency)
    // Returns: shouldRun, reason, error
    ShouldRun(ctx context.Context, client ClusterClient) (bool, string, error)
    
    // Execute performs the stage's main action
    Execute(ctx context.Context, client ClusterClient, config *StageConfig) error
    
    // WaitForReady waits until the stage's resources are ready
    WaitForReady(ctx context.Context, client ClusterClient, timeout time.Duration) error
    
    // Rollback reverts the stage's changes (best effort)
    Rollback(ctx context.Context, client ClusterClient) error
}
```

### Phase 1: Prerequisites

#### Stage 1.1: Namespace

```yaml
# No Helm - direct YAML apply
apiVersion: v1
kind: Namespace
metadata:
  name: primus-lens
  labels:
    app.kubernetes.io/managed-by: primus-lens-installer
```

**Check**: `kubectl get namespace primus-lens`
**Skip if**: Namespace exists with correct labels
**Execute**: `kubectl apply -f namespace.yaml`

#### Stage 1.2: RBAC

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: primus-lens-sa
  namespace: primus-lens
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: primus-lens-installer
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: primus-lens-installer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: primus-lens-installer
subjects:
  - kind: ServiceAccount
    name: primus-lens-sa
    namespace: primus-lens
```

### Phase 2: Operators

Each operator follows the same pattern:

```go
type OperatorStage struct {
    Name           string
    ChartName      string
    ChartRepo      string
    DetectionKey   string  // ClusterRole or CRD to check
    Namespace      string
    Values         map[string]interface{}
}

func (s *OperatorStage) ShouldRun(ctx context.Context, c ClusterClient) (bool, string, error) {
    // Check if operator already exists (installed by CP or another release)
    exists, err := c.ClusterRoleExists(ctx, s.DetectionKey)
    if err != nil {
        return false, "", err
    }
    
    if exists {
        return false, fmt.Sprintf("Operator %s already installed (ClusterRole %s exists)", s.Name, s.DetectionKey), nil
    }
    
    return true, "Operator not found", nil
}

func (s *OperatorStage) WaitForReady(ctx context.Context, c ClusterClient, timeout time.Duration) error {
    return c.WaitForDeploymentReady(ctx, s.Namespace, s.DeploymentName, timeout)
}
```

**Operators to install:**

| Operator | Chart | Detection Key | Namespace |
|----------|-------|---------------|-----------|
| PGO | pgo | `pgo` (ClusterRole) | `postgres-operator` |
| VictoriaMetrics | vm-operator | `vm-operator-*` | `vm-operator` |
| OpenSearch | opensearch-operator | `opensearch-operator-manager-role` | `opensearch-operator` |
| Grafana | grafana-operator | `grafana-operator-manager-role` | `grafana-operator` |
| Fluent | fluent-operator | `fluent-operator` | `fluent` |

### Phase 3: Storage Infrastructure

#### Stage 3.1: PostgreSQL

**Prerequisites:**
- PGO operator deployment is Ready
- StorageClass exists

**Execute:**
```yaml
apiVersion: postgres-operator.crunchydata.com/v1beta1
kind: PostgresCluster
metadata:
  name: primus-lens
  namespace: primus-lens
spec:
  postgresVersion: 16
  instances:
    - name: instance1
      replicas: 1
      dataVolumeClaimSpec:
        storageClassName: {{ .StorageClass }}
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: {{ .PostgresSize }}
  users:
    - name: primus-lens
      databases: ["primus_lens"]
      options: "SUPERUSER"
```

**Wait for Ready:**
```go
func (s *PostgresStage) WaitForReady(ctx context.Context, c ClusterClient, timeout time.Duration) error {
    startTime := time.Now()
    
    for {
        if time.Since(startTime) > timeout {
            return fmt.Errorf("timeout waiting for PostgresCluster")
        }
        
        // 1. Check PostgresCluster CR status
        pg, err := c.GetPostgresCluster(s.Namespace, "primus-lens")
        if err != nil {
            log.Infof("PostgresCluster not found yet, waiting...")
            time.Sleep(10 * time.Second)
            continue
        }
        
        // 2. Check if healthy
        if pg.Status.State != "healthy" {
            log.Infof("PostgresCluster state: %s", pg.Status.State)
            time.Sleep(10 * time.Second)
            continue
        }
        
        // 3. Check if user secret exists (final confirmation)
        _, err = c.GetSecret(s.Namespace, "primus-lens-pguser-primus-lens")
        if err != nil {
            log.Info("Waiting for user secret to be created...")
            time.Sleep(10 * time.Second)
            continue
        }
        
        log.Info("PostgresCluster is ready")
        return nil
    }
}
```

#### Stage 3.2: VictoriaMetrics

**Prerequisites:**
- VM operator deployment is Ready
- StorageClass exists

**Execute:**
```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMCluster
metadata:
  name: primus-lens-vmcluster
  namespace: primus-lens
spec:
  retentionPeriod: "30d"
  vmstorage:
    replicaCount: 1
    storage:
      volumeClaimTemplate:
        spec:
          storageClassName: {{ .StorageClass }}
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: {{ .VMSize }}
  vmselect:
    replicaCount: 1
  vminsert:
    replicaCount: 1
```

#### Stage 3.3: OpenSearch

**Prerequisites:**
- OpenSearch operator deployment is Ready
- StorageClass exists

**Execute:**
```yaml
apiVersion: opensearch.opster.io/v1
kind: OpenSearchCluster
metadata:
  name: primus-lens-logs
  namespace: primus-lens
spec:
  general:
    version: "2.11.0"
    serviceName: primus-lens-logs
  nodePools:
    - component: nodes
      replicas: {{ .OpenSearchReplicas }}
      diskSize: {{ .OpenSearchSize }}
      persistence:
        storageClass: {{ .StorageClass }}
```

### Phase 4: Database Setup

#### Stage 4.1: Database Init

**Prerequisites:**
- PostgresCluster is healthy
- User secret `primus-lens-pguser-primus-lens` exists

**Execute:**
Run as K8s Job or direct psql:
```sql
-- Create primus_lens database if not exists
CREATE DATABASE primus_lens;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE primus_lens TO "primus-lens";
```

#### Stage 4.2: Database Migration

**Prerequisites:**
- Database init completed
- Can connect to database

**Execute:**
```go
func (s *MigrationStage) Execute(ctx context.Context, c ClusterClient, config *StageConfig) error {
    // Get connection info from secret
    secret, _ := c.GetSecret(config.Namespace, "primus-lens-pguser-primus-lens")
    
    dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
        host, port, user, password, dbname)
    
    // Run migrations
    m, _ := migrate.New("file:///migrations", dsn)
    return m.Up()
}
```

### Phase 5: Applications

#### Stage 5.1: Storage Secret

**Prerequisites:**
- PostgresCluster ready
- VMCluster ready
- OpenSearchCluster ready

**Execute:**
```go
func (s *StorageSecretStage) Execute(ctx context.Context, c ClusterClient, config *StageConfig) error {
    // Collect connection info
    pgSecret, _ := c.GetSecret(config.Namespace, "primus-lens-pguser-primus-lens")
    osSecret, _ := c.GetSecret(config.Namespace, "primus-lens-logs-admin-password")
    
    storageConfig := StorageConfig{
        Postgres: PostgresConfig{
            Host:     "primus-lens-primary.primus-lens.svc.cluster.local",
            Port:     5432,
            Username: "primus-lens",
            Password: string(pgSecret.Data["password"]),
            Database: "primus_lens",
        },
        OpenSearch: OpenSearchConfig{
            Host:     "primus-lens-logs-nodes.primus-lens.svc.cluster.local",
            Port:     9200,
            Username: "admin",
            Password: string(osSecret.Data["password"]),
        },
        Prometheus: PrometheusConfig{
            ReadEndpoint:  "http://vmselect-primus-lens-vmcluster.primus-lens.svc.cluster.local:8481/select/0/prometheus",
            WriteEndpoint: "http://vminsert-primus-lens-vmcluster.primus-lens.svc.cluster.local:8480/insert/0/prometheus",
        },
    }
    
    // Create secret
    return c.CreateSecret(config.Namespace, "primus-lens-storage-config", storageConfig)
}
```

#### Stage 5.2: Core Apps

**Prerequisites:**
- Storage secret exists

**Execute:**
Use Helm chart `primus-lens-apps-dataplane`:
```go
func (s *AppsStage) Execute(ctx context.Context, c ClusterClient, config *StageConfig) error {
    values := map[string]interface{}{
        "global": map[string]interface{}{
            "namespace":    config.Namespace,
            "storageClass": config.StorageClass,
        },
        "api": map[string]interface{}{
            "enabled":  true,
            "replicas": 2,
        },
        "jobs": map[string]interface{}{
            "enabled": true,
        },
        "grafana": map[string]interface{}{
            "enabled": true,
        },
    }
    
    return helm.Install(ctx, config.Namespace, "pla", "primus-lens-apps-dataplane", values)
}
```

## Configuration Management

### Flat Configuration Structure

```go
type DataplaneConfig struct {
    // Basic
    ClusterName  string `json:"cluster_name"`
    Namespace    string `json:"namespace"`
    StorageClass string `json:"storage_class"`
    
    // PostgreSQL
    PostgresEnabled bool   `json:"postgres_enabled"`
    PostgresSize    string `json:"postgres_size"`
    PostgresReplicas int   `json:"postgres_replicas"`
    
    // VictoriaMetrics
    VMEnabled bool   `json:"vm_enabled"`
    VMSize    string `json:"vm_size"`
    
    // OpenSearch
    OpenSearchEnabled  bool   `json:"opensearch_enabled"`
    OpenSearchSize     string `json:"opensearch_size"`
    OpenSearchReplicas int    `json:"opensearch_replicas"`
    
    // Images
    ImageRegistry string `json:"image_registry"`
    ImageTag      string `json:"image_tag"`
}
```

### Configuration Merge Priority

```
Priority (highest to lowest):
1. Task-level config (from API request)
2. Cluster-level config (from cluster_config table)
3. Default values

func BuildConfig(task *Task, cluster *ClusterConfig) *DataplaneConfig {
    config := &DataplaneConfig{
        // Defaults
        Namespace:          "primus-lens",
        StorageClass:       "local-path",
        PostgresEnabled:    true,
        PostgresSize:       "10Gi",
        PostgresReplicas:   1,
        VMEnabled:          true,
        VMSize:             "10Gi",
        OpenSearchEnabled:  true,
        OpenSearchSize:     "10Gi",
        OpenSearchReplicas: 1,
    }
    
    // Apply cluster config
    if cluster.ManagedStorageConfig.StorageClass != "" {
        config.StorageClass = cluster.ManagedStorageConfig.StorageClass
    }
    if cluster.ManagedStorageConfig.PostgresSize != "" {
        config.PostgresSize = cluster.ManagedStorageConfig.PostgresSize
    }
    // ... more fields
    
    // Apply task config (highest priority)
    if task.InstallConfig.ManagedStorage != nil {
        ms := task.InstallConfig.ManagedStorage
        if ms.StorageClass != "" {
            config.StorageClass = ms.StorageClass
        }
        // ... more fields
    }
    
    return config
}
```

## Charts Reorganization

### Proposed Structure

```
charts/
├── prerequisites/                    # NEW: Basic resources
│   ├── templates/
│   │   ├── namespace.yaml
│   │   ├── serviceaccount.yaml
│   │   ├── clusterrole.yaml
│   │   └── clusterrolebinding.yaml
│   └── values.yaml
│
├── operators/                        # SPLIT: One chart per operator
│   ├── pgo/                          # Use upstream chart as dependency
│   │   ├── Chart.yaml
│   │   └── values.yaml
│   ├── victoriametrics-operator/
│   │   ├── Chart.yaml
│   │   └── values.yaml
│   ├── opensearch-operator/
│   │   ├── Chart.yaml
│   │   └── values.yaml
│   ├── grafana-operator/
│   │   ├── Chart.yaml
│   │   └── values.yaml
│   └── fluent-operator/
│       ├── Chart.yaml
│       └── values.yaml
│
├── infrastructure/                   # SPLIT: One chart per resource
│   ├── postgres/
│   │   ├── templates/
│   │   │   └── postgres-cluster.yaml
│   │   └── values.yaml
│   ├── victoriametrics/
│   │   ├── templates/
│   │   │   └── vmcluster.yaml
│   │   └── values.yaml
│   └── opensearch/
│       ├── templates/
│       │   └── opensearch-cluster.yaml
│       └── values.yaml
│
├── primus-lens-init/                 # KEEP: Database init
│
└── primus-lens-apps-dataplane/       # KEEP: Applications
```

### Benefits

| Aspect | Before | After |
|--------|--------|-------|
| **Granularity** | All-or-nothing install | Per-component control |
| **Debugging** | Hard to identify failed component | Clear per-stage status |
| **Upgrades** | Upgrade all at once | Upgrade independently |
| **Conflicts** | Hard to handle existing resources | Skip existing, install missing |
| **Rollback** | Complex | Per-stage rollback |

## Implementation Plan

### Phase 1: Core Infrastructure (Week 1-2)

1. **Refactor Stage Interface**
   - Add `CheckPrerequisites` method
   - Add `ShouldRun` method  
   - Separate `WaitForReady` from `Execute`

2. **Implement Wait Logic with Retry**
   - Handle "no matching resources found"
   - Add exponential backoff
   - Add timeout per stage

3. **Fix Configuration Handling**
   - Flatten config structure
   - Implement merge logic with clear priority

### Phase 2: Operator Management (Week 2-3)

1. **Split Operators Chart**
   - Create individual charts for each operator
   - Update detection logic per operator

2. **Implement Operator Stages**
   - One stage per operator
   - Independent install/skip decision

### Phase 3: Infrastructure Management (Week 3-4)

1. **Split Infrastructure Chart**
   - PostgresCluster as separate chart/stage
   - VMCluster as separate chart/stage
   - OpenSearchCluster as separate chart/stage

2. **Implement Proper Wait Logic**
   - Wait for CR status
   - Wait for secrets to be created
   - Optional: connection test

### Phase 4: Testing & Documentation (Week 4-5)

1. **Integration Tests**
   - Test each stage independently
   - Test full flow
   - Test resume from failed stage

2. **Documentation**
   - Update deployment guide
   - Add troubleshooting guide

## Backward Compatibility

The new architecture maintains backward compatibility:

1. **API Contract**: Same API endpoints and request format
2. **Database Schema**: No changes to task/config tables
3. **Helm Charts**: Can still use existing charts with new stages
4. **Gradual Migration**: Can enable new stages incrementally

## Error Handling

### Stage Failure Handling

```go
func (i *Installer) ExecuteStages(ctx context.Context, stages []Stage) error {
    for _, stage := range stages {
        // Check prerequisites
        missing, err := stage.CheckPrerequisites(ctx, i.client)
        if err != nil {
            return fmt.Errorf("failed to check prerequisites for %s: %w", stage.Name(), err)
        }
        if len(missing) > 0 {
            return fmt.Errorf("stage %s missing prerequisites: %v", stage.Name(), missing)
        }
        
        // Check if should run
        shouldRun, reason, err := stage.ShouldRun(ctx, i.client)
        if err != nil {
            return fmt.Errorf("failed to check if %s should run: %w", stage.Name(), err)
        }
        if !shouldRun {
            log.Infof("Skipping stage %s: %s", stage.Name(), reason)
            continue
        }
        
        // Execute
        log.Infof("Executing stage: %s", stage.Name())
        if err := stage.Execute(ctx, i.client, i.config); err != nil {
            // Save current stage for resume
            i.saveProgress(stage.Name())
            return fmt.Errorf("stage %s failed: %w", stage.Name(), err)
        }
        
        // Wait for ready
        if err := stage.WaitForReady(ctx, i.client, i.config.Timeout); err != nil {
            i.saveProgress(stage.Name())
            return fmt.Errorf("stage %s not ready: %w", stage.Name(), err)
        }
        
        log.Infof("Stage %s completed", stage.Name())
    }
    
    return nil
}
```

### Resume from Failure

The installer supports resuming from a failed stage:

1. On failure, current stage is saved to `dataplane_install_tasks.current_stage`
2. On retry, installer starts from the saved stage
3. Each stage's `ShouldRun` ensures idempotency

## Monitoring & Observability

### Stage Metrics

```go
var (
    stageExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "installer_stage_duration_seconds",
            Help:    "Time spent in each installation stage",
            Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
        },
        []string{"stage", "cluster", "status"},
    )
    
    stageExecutionTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "installer_stage_executions_total",
            Help: "Total number of stage executions",
        },
        []string{"stage", "cluster", "status"},
    )
)
```

### Logging

Each stage logs:
- Prerequisites check result
- ShouldRun decision and reason
- Execution progress
- Wait status updates
- Completion or failure

## Conclusion

This redesigned architecture provides:

1. **Better Observability**: Clear status for each component
2. **Improved Reliability**: Proper wait logic with retries
3. **Easier Debugging**: Isolated stages, clear error messages
4. **Flexible Deployment**: Skip existing resources, install only what's needed
5. **Maintainability**: Single responsibility per stage
