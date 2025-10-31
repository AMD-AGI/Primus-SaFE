# GPU Resource Exporter

## Overview

The GPU Resource Exporter is a Kubernetes controller module within the Primus-Lens system that monitors, tracks, and exports GPU resource information from Kubernetes clusters. It provides comprehensive tracking of GPU workloads, pod lifecycle management, and resource allocation data.

## Features

- **GPU Pod Monitoring**: Automatically detects and tracks Kubernetes pods that request GPU resources
- **Workload Lifecycle Tracking**: Monitors the complete lifecycle of GPU workloads from creation to termination
- **Resource Allocation Tracking**: Records GPU allocation and usage across pods and nodes
- **Event Tracking**: Captures pod state transitions and condition changes
- **Owner Reference Tracing**: Traces pod ownership hierarchy (Pod → ReplicaSet → Deployment, etc.)
- **Snapshot Management**: Creates and maintains snapshots of pod specifications, metadata, and status
- **Finalizer Management**: Ensures proper cleanup and data persistence before resource deletion
- **Node Management**: Maintains kubelet service endpoints for metrics collection

## Architecture

The module consists of three main components:

### 1. Bootstrap (`pkg/bootstrap`)
- Initializes the controller and registers reconcilers
- Sets up Kubernetes scheme and client
- Initializes the listener manager

### 2. Reconcilers (`pkg/reconciler`)

#### GPU Pods Reconciler
- Monitors pods with GPU resource requests
- Filters events for GPU-enabled pods only
- Records pod snapshots, events, status, and resource allocation
- Traces owner references up the hierarchy
- Registers listeners for parent workloads

#### Node Reconciler
- Monitors cluster nodes
- Creates and maintains a kubelet service in `kube-system` namespace
- Manages service endpoints for kubelet metrics (ports 10250, 10255, 4194)
- Runs on a 30-second reconciliation loop

### 3. Listener Manager (`pkg/listener`)

#### Listener
- Watches individual workload resources (Deployments, Jobs, etc.)
- Manages finalizers to prevent premature deletion
- Saves workload state to database
- Exits gracefully when workload is deleted or no longer exists
- Performs periodic health checks (30-second intervals)

#### Manager
- Manages the lifecycle of all active listeners
- Recovers listeners for existing workloads on startup
- Performs garbage collection of terminated listeners (10-second intervals)
- Thread-safe listener registration and management

## Data Models

The exporter tracks the following data entities:

### GpuWorkload
- Workload identification (Group/Version/Kind, Namespace, Name, UID)
- Parent workload relationships
- GPU resource requests
- Status and lifecycle timestamps
- Labels

### GpuPods
- Pod identification and location
- GPU allocation
- Running and deletion status
- Phase tracking

### PodResource
- GPU model information
- Allocated GPU resources
- Resource lifecycle timestamps

### PodSnapshot
- Complete pod specification
- Metadata and status
- Resource version tracking

### GpuPodsEvent
- Pod phase transitions
- Event types (conditions)
- Restart counts

### WorkloadPodReference
- Links between workloads and their pods
- Enables hierarchical queries

## Configuration

The module inherits configuration from the Primus-Lens core system:

- Kubernetes client configuration
- Database connection settings
- Logging configuration
- Controller runtime settings

## Dependencies

### Core Dependencies
- `github.com/AMD-AGI/primus-lens/core`: Core Primus-Lens functionality
- `k8s.io/api`: Kubernetes API types
- `k8s.io/apimachinery`: Kubernetes API machinery
- `sigs.k8s.io/controller-runtime`: Controller runtime framework

### Database
- Compatible with PostgreSQL and MySQL
- Uses GORM for database operations

## Installation

### Prerequisites
- Kubernetes cluster (v1.34+)
- Go 1.24.5 or higher
- Primus-Lens core module
- Database (PostgreSQL or MySQL)

### Build

```bash
cd Lens/modules/exporters/gpu-resource-exporter
go build -o gpu-resource-exporter ./cmd/gpu-resource-exporter
```

### Deployment

The module is typically deployed as part of the Primus-Lens system. It runs as a Kubernetes controller with appropriate RBAC permissions.

Required RBAC permissions:
- **Pods**: Get, List, Watch, Update
- **Nodes**: Get, List, Watch
- **Services**: Create, Get, List, Update
- **Endpoints**: Create, Get, List, Update
- **Custom Resources**: Get, List, Watch, Update (for workload tracking)

## How It Works

### Pod Lifecycle Tracking

1. **Pod Detection**: The GPU Pods Reconciler filters for pods requesting GPU resources
2. **Snapshot Creation**: Creates a snapshot of the pod's spec, metadata, and status
3. **Resource Recording**: Records GPU allocation and model information
4. **Event Generation**: Compares snapshots to detect state changes and generates events
5. **Owner Tracing**: Follows owner references to identify parent workloads
6. **Listener Registration**: Registers listeners for parent workloads to track their lifecycle

### Workload Lifecycle Tracking

1. **Listener Creation**: When a workload is identified, a listener is registered
2. **Finalizer Addition**: Adds a finalizer to prevent premature deletion
3. **Watch Loop**: Continuously watches for changes to the workload
4. **State Persistence**: Saves workload state on every change
5. **Deletion Handling**: On deletion, persists final state and removes finalizer
6. **Health Checks**: Periodic verification that the resource still exists

### Garbage Collection

- Terminated listeners are cleaned up every 10 seconds
- Prevents memory leaks from long-running operations
- Maintains active listener registry

## Monitoring

### Logs

The module provides detailed logging for:
- Listener lifecycle events
- Reconciliation operations
- Error conditions
- Resource state changes

### Metrics

The module sets up kubelet service endpoints for metrics collection:
- **Port 10250**: HTTPS metrics
- **Port 10255**: HTTP metrics  
- **Port 4194**: cAdvisor metrics

## GPU Vendor Support

Currently supports:
- AMD GPUs (primary)
- Extensible to other GPU vendors through the resource name configuration

## Database Schema

The module creates and maintains the following database tables:
- `gpu_workloads`: Workload tracking
- `gpu_pods`: Pod status and allocation
- `pod_resources`: GPU resource allocation details
- `pod_snapshots`: Historical pod state
- `gpu_pods_events`: Pod lifecycle events
- `workload_pod_references`: Workload-pod relationships

## Troubleshooting

### Common Issues

**Listeners not starting**
- Check RBAC permissions
- Verify database connectivity
- Check logs for initialization errors

**Missing workload data**
- Ensure pods have owner references
- Verify workload still exists in cluster
- Check listener manager logs

**Finalizer not removed**
- Check for errors in listener cleanup
- Verify database write operations succeed
- Manually remove finalizer if necessary: `kubectl patch <resource> -p '{"metadata":{"finalizers":null}}'`

### Debug Mode

Enable detailed logging by setting the log level in the Primus-Lens core configuration.

## Contributing

This module is part of the AMD-AGI Primus-SaFE project. For contributions, please follow the project's contribution guidelines.

## License

This module is part of the Primus-SaFE project and follows the project's licensing terms.

## Related Modules

- **Primus-Lens Core**: Provides base functionality and shared libraries
- **Telemetry Processor**: Processes metrics collected from GPU workloads
- **System Tuner**: Uses GPU resource data for optimization decisions

