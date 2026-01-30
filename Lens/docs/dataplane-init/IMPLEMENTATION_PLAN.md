# Dataplane Initialization - Implementation Plan

## Overview

This document outlines the step-by-step implementation plan for the new dataplane initialization architecture.

## Current State Analysis

### Existing Components

| Component | Location | Status |
|-----------|----------|--------|
| Installer Module | `modules/installer/` | Active |
| Controller Job | `modules/control-plane-controller/pkg/jobs/dataplane_installer/` | Active |
| Operators Chart | `charts/primus-lens-operators/` | Active |
| Infrastructure Chart | `charts/primus-lens-infrastructure/` | Active |
| Init Chart | `charts/primus-lens-init/` | Active |
| Apps Chart | `charts/primus-lens-apps-dataplane/` | Active |

### Known Issues

1. **Wait Logic Bug**: `wait_infrastructure` doesn't retry on "no matching resources found"
2. **Storage Config Bug**: `storageClass` can be null/empty
3. **Secret Name Bug**: Wrong PostgreSQL secret name format
4. **Operator Conflict**: CP/DP shared cluster causes ClusterRole conflicts
5. **Image Version Bug**: Old `primus-lens-jobs` component interfering

## Implementation Phases

---

## Phase 1: Bug Fixes (Immediate)

**Goal**: Fix critical bugs blocking current deployments

### Task 1.1: Fix Wait Logic
**Status**: ✅ Completed (commit `4f6a5498`)

- Added `waitForPodsWithRetry` function
- Handles "no matching resources found" with retry loop
- Postgres is required (fails on timeout), others are optional

### Task 1.2: Fix Storage Configuration
**Status**: ✅ Completed (commit `4f6a5498`)

- API now builds `ManagedStorage` config from cluster config or defaults
- Installer ensures `storageClass` is never empty

### Task 1.3: Fix PostgreSQL Secret Name
**Status**: ✅ Completed (commit `bd457a97`)

- Changed from Zalando format to CrunchyData PGO format
- `primus-lens-pguser-primus-lens` instead of `primus-lens.primus-lens.credentials.postgresql.acid.zalan.do`

### Task 1.4: Remove Legacy Component
**Status**: ✅ Completed (manual)

- Deleted `primus-lens-jobs` deployment
- This was causing job creation conflicts

---

## Phase 2: Stage Interface Refactoring

**Goal**: Implement the new Stage interface with proper separation of concerns

### Task 2.1: Define New Stage Interface

**File**: `modules/installer/pkg/installer/stage.go`

```go
package installer

import (
    "context"
    "time"
)

// Stage represents an installation stage with lifecycle methods
type Stage interface {
    // Name returns the unique identifier for this stage
    Name() string
    
    // CheckPrerequisites verifies all dependencies are met
    // Returns slice of missing prerequisites (empty if all met)
    CheckPrerequisites(ctx context.Context, client *ClusterClient, config *StageConfig) ([]string, error)
    
    // ShouldRun determines if this stage needs to execute
    // Returns: shouldRun, reason, error
    ShouldRun(ctx context.Context, client *ClusterClient, config *StageConfig) (bool, string, error)
    
    // Execute performs the main installation action
    Execute(ctx context.Context, client *ClusterClient, config *StageConfig) error
    
    // WaitForReady waits until resources are ready
    WaitForReady(ctx context.Context, client *ClusterClient, config *StageConfig, timeout time.Duration) error
    
    // Rollback attempts to undo changes (best effort)
    Rollback(ctx context.Context, client *ClusterClient, config *StageConfig) error
}

// StageConfig contains all configuration needed by stages
type StageConfig struct {
    ClusterName  string
    Namespace    string
    Kubeconfig   []byte
    
    StorageClass string
    
    PostgresEnabled  bool
    PostgresSize     string
    PostgresReplicas int
    
    VMEnabled bool
    VMSize    string
    
    OpenSearchEnabled  bool
    OpenSearchSize     string
    OpenSearchReplicas int
    
    ImageRegistry string
    ImageTag      string
}

// StageResult captures the outcome of stage execution
type StageResult struct {
    Stage     string
    Status    string // "skipped", "completed", "failed"
    Reason    string
    Duration  time.Duration
    Error     error
}
```

### Task 2.2: Implement ClusterClient

**File**: `modules/installer/pkg/installer/cluster_client.go`

```go
package installer

import (
    "context"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

// ClusterClient provides methods to interact with target cluster
type ClusterClient struct {
    clientset  *kubernetes.Clientset
    kubeconfig []byte
}

// NewClusterClient creates a client from kubeconfig bytes
func NewClusterClient(kubeconfig []byte) (*ClusterClient, error) {
    config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
    if err != nil {
        return nil, err
    }
    
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        return nil, err
    }
    
    return &ClusterClient{
        clientset:  clientset,
        kubeconfig: kubeconfig,
    }, nil
}

// NamespaceExists checks if a namespace exists
func (c *ClusterClient) NamespaceExists(ctx context.Context, name string) (bool, error)

// ClusterRoleExists checks if a ClusterRole exists
func (c *ClusterClient) ClusterRoleExists(ctx context.Context, name string) (bool, error)

// DeploymentReady checks if a deployment is ready
func (c *ClusterClient) DeploymentReady(ctx context.Context, namespace, name string) (bool, error)

// SecretExists checks if a secret exists
func (c *ClusterClient) SecretExists(ctx context.Context, namespace, name string) (bool, error)

// GetSecret retrieves a secret
func (c *ClusterClient) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error)

// ApplyYAML applies a YAML manifest
func (c *ClusterClient) ApplyYAML(ctx context.Context, yaml []byte) error

// StorageClassExists checks if a StorageClass exists
func (c *ClusterClient) StorageClassExists(ctx context.Context, name string) (bool, error)
```

### Task 2.3: Refactor Existing Stages

Migrate existing stages to new interface:

| Stage | Old Implementation | New Implementation |
|-------|-------------------|-------------------|
| operators | `OperatorsStage` | `OperatorStage` (one per operator) |
| wait_operators | `WaitOperatorsStage` | Merged into `OperatorStage.WaitForReady` |
| infrastructure | `InfrastructureStage` | Split into `PostgresStage`, `VMStage`, `OpenSearchStage` |
| wait_infrastructure | `WaitInfraStage` | Merged into each infrastructure stage |
| init | `InitStage` | `DatabaseInitStage` |
| database_migration | `DatabaseMigrationStage` | Keep, add prerequisites check |
| storage_secret | `StorageSecretStage` | Keep, add prerequisites check |
| applications | `ApplicationsStage` | Keep |
| wait_applications | `WaitAppsStage` | Merged into `ApplicationsStage.WaitForReady` |

---

## Phase 3: Operator Stage Split

**Goal**: Install operators independently with proper detection

### Task 3.1: Create Individual Operator Stages

**File**: `modules/installer/pkg/installer/stages/operator_pgo.go`

```go
package stages

type PGOOperatorStage struct {
    helmClient *HelmClient
}

func (s *PGOOperatorStage) Name() string {
    return "operator-pgo"
}

func (s *PGOOperatorStage) CheckPrerequisites(ctx context.Context, client *ClusterClient, config *StageConfig) ([]string, error) {
    var missing []string
    
    // Check namespace exists
    exists, _ := client.NamespaceExists(ctx, "postgres-operator")
    if !exists {
        // Will be created by Helm, not a blocker
    }
    
    return missing, nil
}

func (s *PGOOperatorStage) ShouldRun(ctx context.Context, client *ClusterClient, config *StageConfig) (bool, string, error) {
    // Check if PGO ClusterRole exists
    exists, err := client.ClusterRoleExists(ctx, "pgo")
    if err != nil {
        return false, "", err
    }
    
    if exists {
        return false, "PGO operator already installed (ClusterRole 'pgo' exists)", nil
    }
    
    return true, "PGO operator not found", nil
}

func (s *PGOOperatorStage) Execute(ctx context.Context, client *ClusterClient, config *StageConfig) error {
    values := map[string]interface{}{
        // PGO specific values
    }
    
    return s.helmClient.Install(ctx, client.kubeconfig, "postgres-operator", "pgo", "pgo", values)
}

func (s *PGOOperatorStage) WaitForReady(ctx context.Context, client *ClusterClient, config *StageConfig, timeout time.Duration) error {
    return client.WaitForDeploymentReady(ctx, "postgres-operator", "pgo", timeout)
}
```

### Task 3.2: Create Similar Stages for Other Operators

- `operator_victoriametrics.go`
- `operator_opensearch.go`
- `operator_grafana.go`
- `operator_fluent.go`
- `operator_kube_state_metrics.go`

### Task 3.3: Update Stage Registry

```go
func GetOperatorStages() []Stage {
    return []Stage{
        &PGOOperatorStage{},
        &VMOperatorStage{},
        &OpenSearchOperatorStage{},
        &GrafanaOperatorStage{},
        &FluentOperatorStage{},
        &KubeStateMetricsStage{},
    }
}
```

---

## Phase 4: Infrastructure Stage Split

**Goal**: Deploy each infrastructure component independently

### Task 4.1: PostgreSQL Stage

**File**: `modules/installer/pkg/installer/stages/postgres.go`

```go
type PostgresStage struct{}

func (s *PostgresStage) Name() string {
    return "infra-postgres"
}

func (s *PostgresStage) CheckPrerequisites(ctx context.Context, client *ClusterClient, config *StageConfig) ([]string, error) {
    var missing []string
    
    // Check PGO operator is ready
    ready, _ := client.DeploymentReady(ctx, "postgres-operator", "pgo")
    if !ready {
        missing = append(missing, "PGO operator not ready")
    }
    
    // Check StorageClass exists
    if config.StorageClass != "" {
        exists, _ := client.StorageClassExists(ctx, config.StorageClass)
        if !exists {
            missing = append(missing, fmt.Sprintf("StorageClass '%s' not found", config.StorageClass))
        }
    }
    
    return missing, nil
}

func (s *PostgresStage) ShouldRun(ctx context.Context, client *ClusterClient, config *StageConfig) (bool, string, error) {
    if !config.PostgresEnabled {
        return false, "PostgreSQL disabled in config", nil
    }
    
    // Check if PostgresCluster exists and is healthy
    // ... implementation
}

func (s *PostgresStage) WaitForReady(ctx context.Context, client *ClusterClient, config *StageConfig, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    
    for time.Now().Before(deadline) {
        // 1. Check PostgresCluster CR status
        // 2. Check pods are running
        // 3. Check user secret exists
        // 4. Optional: test connection
        
        time.Sleep(10 * time.Second)
    }
    
    return fmt.Errorf("timeout waiting for PostgresCluster")
}
```

### Task 4.2: VictoriaMetrics Stage

Similar structure to PostgresStage.

### Task 4.3: OpenSearch Stage

Similar structure to PostgresStage.

---

## Phase 5: Executor Refactoring

**Goal**: Implement the new stage executor with proper lifecycle

### Task 5.1: Implement Stage Executor

**File**: `modules/installer/pkg/installer/executor.go`

```go
package installer

type Executor struct {
    client *ClusterClient
    config *StageConfig
}

func (e *Executor) ExecuteStages(ctx context.Context, stages []Stage) ([]StageResult, error) {
    var results []StageResult
    
    for _, stage := range stages {
        result := e.executeStage(ctx, stage)
        results = append(results, result)
        
        if result.Status == "failed" {
            return results, result.Error
        }
    }
    
    return results, nil
}

func (e *Executor) executeStage(ctx context.Context, stage Stage) StageResult {
    startTime := time.Now()
    result := StageResult{Stage: stage.Name()}
    
    // 1. Check prerequisites
    missing, err := stage.CheckPrerequisites(ctx, e.client, e.config)
    if err != nil {
        result.Status = "failed"
        result.Error = fmt.Errorf("prerequisites check failed: %w", err)
        result.Duration = time.Since(startTime)
        return result
    }
    if len(missing) > 0 {
        result.Status = "failed"
        result.Error = fmt.Errorf("missing prerequisites: %v", missing)
        result.Duration = time.Since(startTime)
        return result
    }
    
    // 2. Check if should run
    shouldRun, reason, err := stage.ShouldRun(ctx, e.client, e.config)
    if err != nil {
        result.Status = "failed"
        result.Error = fmt.Errorf("should run check failed: %w", err)
        result.Duration = time.Since(startTime)
        return result
    }
    if !shouldRun {
        result.Status = "skipped"
        result.Reason = reason
        result.Duration = time.Since(startTime)
        log.Infof("Skipping stage %s: %s", stage.Name(), reason)
        return result
    }
    
    // 3. Execute
    log.Infof("Executing stage: %s", stage.Name())
    if err := stage.Execute(ctx, e.client, e.config); err != nil {
        result.Status = "failed"
        result.Error = fmt.Errorf("execution failed: %w", err)
        result.Duration = time.Since(startTime)
        return result
    }
    
    // 4. Wait for ready
    timeout := e.getStageTimeout(stage.Name())
    if err := stage.WaitForReady(ctx, e.client, e.config, timeout); err != nil {
        result.Status = "failed"
        result.Error = fmt.Errorf("wait for ready failed: %w", err)
        result.Duration = time.Since(startTime)
        return result
    }
    
    result.Status = "completed"
    result.Duration = time.Since(startTime)
    log.Infof("Stage %s completed in %v", stage.Name(), result.Duration)
    return result
}

func (e *Executor) getStageTimeout(stageName string) time.Duration {
    timeouts := map[string]time.Duration{
        "infra-postgres":    10 * time.Minute,
        "infra-vm":          5 * time.Minute,
        "infra-opensearch":  10 * time.Minute,
        "database-init":     5 * time.Minute,
        "database-migration": 5 * time.Minute,
        "apps":              10 * time.Minute,
    }
    
    if t, ok := timeouts[stageName]; ok {
        return t
    }
    return 5 * time.Minute // default
}
```

---

## Phase 6: Chart Reorganization

**Goal**: Split monolithic charts into focused components

### Task 6.1: Create Prerequisites Chart

```yaml
# charts/primus-lens-prerequisites/Chart.yaml
apiVersion: v2
name: primus-lens-prerequisites
version: 1.0.0
description: Basic resources for Primus-Lens deployment
```

### Task 6.2: Split Operators Chart

Create individual charts:
- `charts/operators/pgo/`
- `charts/operators/victoriametrics-operator/`
- `charts/operators/opensearch-operator/`
- `charts/operators/grafana-operator/`
- `charts/operators/fluent-operator/`

### Task 6.3: Split Infrastructure Chart

Create individual charts:
- `charts/infrastructure/postgres/`
- `charts/infrastructure/victoriametrics/`
- `charts/infrastructure/opensearch/`

---

## Phase 7: Testing

### Task 7.1: Unit Tests

- Test each stage's `CheckPrerequisites`
- Test each stage's `ShouldRun` logic
- Test configuration merge logic

### Task 7.2: Integration Tests

- Test full deployment flow
- Test resume from failure
- Test skip existing resources
- Test CP/DP shared cluster scenario

### Task 7.3: E2E Tests

- Deploy to real cluster
- Verify all components working
- Test upgrade scenarios

---

## Timeline

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Bug Fixes | ✅ Done | None |
| Phase 2: Stage Interface | 1 week | Phase 1 |
| Phase 3: Operator Split | 1 week | Phase 2 |
| Phase 4: Infrastructure Split | 1 week | Phase 3 |
| Phase 5: Executor Refactoring | 1 week | Phase 4 |
| Phase 6: Chart Reorganization | 1 week | Phase 5 |
| Phase 7: Testing | 1 week | Phase 6 |

**Total Estimated Time**: 6 weeks

---

## Migration Strategy

### Backward Compatibility

1. Keep existing stage names in database
2. Map old stages to new stages
3. Support resume from old stage format

### Gradual Rollout

1. Deploy new executor with feature flag
2. Test with non-production clusters
3. Enable for new installations
4. Migrate existing installations

### Rollback Plan

1. Keep old code in parallel
2. Feature flag to switch between old/new
3. Monitor metrics for issues

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Installation success rate | > 95% |
| Average installation time | < 30 minutes |
| Resume from failure success | > 90% |
| Operator conflict resolution | 100% |
| Storage config errors | 0 |

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing deployments | Medium | High | Feature flag, gradual rollout |
| Longer installation time | Low | Medium | Parallel operator installation |
| New bugs introduced | Medium | Medium | Comprehensive testing |
| Chart compatibility issues | Low | Medium | Keep Helm abstraction |

---

## Appendix: File Structure

```
modules/installer/
├── cmd/
│   └── main.go
├── pkg/
│   └── installer/
│       ├── cluster_client.go      # NEW
│       ├── config.go
│       ├── executor.go            # NEW
│       ├── helm_client.go
│       ├── installer.go
│       ├── stage.go               # NEW (interface)
│       ├── stages/                # NEW (directory)
│       │   ├── operator_pgo.go
│       │   ├── operator_vm.go
│       │   ├── operator_opensearch.go
│       │   ├── operator_grafana.go
│       │   ├── operator_fluent.go
│       │   ├── postgres.go
│       │   ├── victoriametrics.go
│       │   ├── opensearch.go
│       │   ├── database_init.go
│       │   ├── database_migration.go
│       │   ├── storage_secret.go
│       │   └── applications.go
│       ├── stages.go              # KEEP (legacy, map to new)
│       └── types.go
├── go.mod
└── go.sum
```
