# GitHub Runners Exporter

A Kubernetes controller that watches GitHub Actions Runner Controller (ARC) resources and syncs them to the Primus Lens database.

## Overview

This exporter replaces the polling-based `github_runner_scanner` and `github_workflow_scanner` jobs with real-time watch-based reconcilers using the controller-runtime library.

## Components

### AutoScalingRunnerSetReconciler

Watches `AutoScalingRunnerSet` resources from the `actions.github.com/v1alpha1` API group and syncs them to the `github_runner_sets` table.

**Features:**
- Real-time discovery of AutoScalingRunnerSets
- Extracts GitHub owner/repo from `githubConfigUrl`
- Tracks runner pool configuration (min/max runners)
- Monitors current and desired runner counts
- Handles resource deletion gracefully

### EphemeralRunnerReconciler

Watches `EphemeralRunner` resources and creates `github_workflow_runs` records when runners complete.

**Features:**
- Real-time detection of completed workflow runs
- Extracts GitHub workflow metadata from annotations
- Matches runners to configured workflow collection configs
- Supports workflow and branch filtering

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                  github-runners-exporter                  │
│                                                          │
│  ┌────────────────────┐   ┌─────────────────────────┐   │
│  │ ARS Reconciler     │   │ EphemeralRunner         │   │
│  │                    │   │ Reconciler              │   │
│  │ • Watch ARS        │   │ • Watch EphemeralRunner │   │
│  │ • Upsert to DB     │   │ • Create run records    │   │
│  │ • Track status     │   │ • Match to configs      │   │
│  └─────────┬──────────┘   └───────────┬─────────────┘   │
│            │                          │                  │
│            ▼                          ▼                  │
│  ┌─────────────────────────────────────────────────┐    │
│  │              Database Facade                     │    │
│  │                                                  │    │
│  │  github_runner_sets │ github_workflow_runs       │    │
│  └─────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────┘
```

## Comparison with Polling Jobs

| Aspect | Polling Jobs | Reconciler |
|--------|-------------|------------|
| Discovery Latency | 2-5 minutes | Real-time (~seconds) |
| Resource Usage | Periodic full scans | Event-driven, minimal |
| Missed Events | Possible for fast-cycling resources | None (informer cache) |
| Code Pattern | Cron-based | controller-runtime |

## Integration

This exporter can be:

1. **Integrated into primus-safe-adapter**: Add bootstrap import to adapter's init
2. **Run standalone**: Use `cmd/main.go` as a separate deployment
3. **Integrated into control plane**: Add to the control plane deployment

### Option 1: Integrate into primus-safe-adapter

Add to `modules/adapter/primus-safe-adapter/pkg/bootstrap/bootstrap.go`:

```go
import (
    githubRunners "github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/bootstrap"
)

func Init(ctx context.Context, cfg *config.Config) error {
    // ... existing init code ...
    
    // Initialize GitHub Runners Exporter
    if err := githubRunners.RegisterController(ctx); err != nil {
        log.Errorf("Failed to initialize GitHub Runners Exporter: %v", err)
        // Don't fail startup, continue with degraded functionality
    }
    
    // ... rest of init ...
}
```

### Option 2: Standalone Deployment

Build and deploy as a separate container:

```bash
cd modules/exporters/github-runners-exporter
go build -o github-runners-exporter ./cmd/main.go
```

## Configuration

The exporter uses the standard Primus Lens configuration:

- **Database**: Connects to the configured PostgreSQL database
- **ClusterManager**: Uses the current cluster context
- **Logger**: Uses the global logger configuration

## CRD Dependencies

This exporter requires the GitHub Actions Runner Controller CRDs:

- `autoscalingrunnersets.actions.github.com/v1alpha1`
- `ephemeralrunners.actions.github.com/v1alpha1`

These CRDs are installed when deploying the GitHub Actions Runner Controller.

