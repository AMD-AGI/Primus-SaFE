# Primus Lens Jobs Module

## Overview

The Primus Lens Jobs module is a Go-based scheduled job orchestration system designed for monitoring and managing GPU resources, nodes, devices, and workloads in a Kubernetes cluster. It provides comprehensive observability through Prometheus metrics and gRPC-based event streaming.

## Architecture

The module consists of the following key components:

- **Job Scheduler**: Cron-based job scheduler that executes periodic tasks
- **Event Server**: gRPC server that receives and processes container events from node agents
- **Database Integration**: Persistent storage for nodes, devices, containers, and workloads
- **Prometheus Exporter**: Metrics export for monitoring and alerting
- **Kubernetes Integration**: Direct interaction with Kubernetes API for resource management

## Features

### Scheduled Jobs

The module implements several scheduled jobs that run at different intervals:

#### 1. GPU Allocation Job
- **Schedule**: Every 30 seconds
- **Purpose**: Monitors cluster-level GPU allocation rate
- **Metrics**: `gpu_allocation_rate`

#### 2. GPU Consumers Job
- **Schedule**: Every 30 seconds
- **Purpose**: Tracks GPU consumers (pods/workloads) and their resource usage
- **Metrics**:
  - `consumer_pod_gpu_usage`: GPU utilization per consumer
  - `consumer_pod_gpu_allocated`: Allocated GPUs per consumer
  - `consumer_pod_active`: Number of active pods per consumer

#### 3. Node Info Job
- **Schedule**: Every 10 seconds
- **Purpose**: Collects and updates node information including:
  - Node status and addresses
  - GPU device information
  - CPU and memory resources
  - Driver versions
  - GPU allocation and utilization
- **Database**: Updates `nodes` table

#### 4. Device Info Job
- **Schedule**: Every 10 seconds
- **Purpose**: Collects detailed device information for each node:
  - **GPU Devices**: Model, memory, utilization, temperature, power, serial number, NUMA affinity
  - **RDMA Devices**: Interface name, node GUID, firmware version
- **Features**:
  - Tracks device additions and removals
  - Creates device change logs for auditing
- **Database**: Updates `gpu_devices`, `rdma_devices`, and `node_device_changelog` tables

#### 5. GPU Workload Job
- **Schedule**: Every 20 seconds
- **Purpose**: Monitors GPU workloads (Deployments, StatefulSets, Jobs, etc.)
  - Tracks workload status (Running, Done, Deleted)
  - Monitors GPU resource requests
  - Counts active pods per workload
- **Database**: Updates `gpu_workload` table

#### 6. GPU Pod Job
- **Schedule**: Every 5 seconds
- **Purpose**: Tracks individual GPU pods and their lifecycle
  - Updates pod phase changes
  - Marks deleted pods
- **Database**: Updates `gpu_pods` table

#### 7. Storage Scan Job
- **Schedule**: Every 1 minute
- **Purpose**: Scans and discovers storage backends in the cluster
  - Detects JuiceFS and other storage systems
  - Tracks storage health status
- **Database**: Updates `storage` table

### Event Server

The module runs a gRPC server that receives real-time container events from node agents.

#### Container Event Streaming

**Endpoints**:
- `StreamContainerEvents`: Receives events from Kubernetes containers
- `StreamDockerContainerEvents`: Receives events from Docker containers

**Event Processing**:
- Container lifecycle events (create, start, stop, delete)
- Device assignments (GPU, InfiniBand)
- Container snapshots

**Metrics**:
- `primus_lens_jobs_container_event_recv_total`: Total events received
- `primus_lens_jobs_container_event_error_total`: Total event errors
- `primus_lens_jobs_upstream_connected`: Upstream connection status
- `primus_lens_jobs_event_processing_duration_seconds`: Event processing latency

**Database Operations**:
- Updates `node_containers` table
- Tracks container-device associations in `node_container_devices` table
- Records container events in `node_container_events` table

## Installation

### Prerequisites

- Go 1.24.5 or higher
- Kubernetes cluster (1.23+)
- Access to Kubernetes API
- Database (PostgreSQL/MySQL)
- Primus Lens Core module

### Configuration

The module requires the following configuration in the Primus Lens config:

```yaml
jobs:
  grpc_port: 50051  # Port for gRPC event server
```

### Build

```bash
cd Lens/modules/jobs
go build -o primus-lens-jobs ./cmd/primus-lens-jobs
```

### Run

```bash
./primus-lens-jobs
```

The application will:
1. Initialize the server with pre-init bootstrap function
2. Start the gRPC event server on the configured port
3. Register Kubernetes schemes
4. Start all scheduled jobs

## Dependencies

### Key Dependencies

- **Kubernetes Client**: `k8s.io/client-go`, `sigs.k8s.io/controller-runtime`
- **Cron Scheduler**: `github.com/robfig/cron/v3`
- **gRPC**: `google.golang.org/grpc`
- **Prometheus**: `github.com/prometheus/client_golang`
- **Primus Lens Core**: `github.com/AMD-AGI/Primus-SaFE/Lens/core`

See `go.mod` for complete dependency list.

## Job Interface

All jobs implement the following interface:

```go
type Job interface {
    Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error
    Schedule() string
}
```

- **Run**: Executes the job logic
- **Schedule**: Returns a cron expression (e.g., `@every 30s`, `@every 1m`)

## Metrics

The module exports the following Prometheus metrics:

### Job Metrics
- `gpu_allocation_rate`: Cluster GPU allocation rate
- `consumer_pod_gpu_usage`: GPU utilization by consumer
- `consumer_pod_gpu_allocated`: GPUs allocated by consumer
- `consumer_pod_active`: Active pods per consumer

### Event Server Metrics
- `primus_lens_jobs_container_event_recv_total`: Container events received
- `primus_lens_jobs_container_event_error_total`: Container event errors
- `primus_lens_jobs_upstream_connected`: Upstream connection status
- `primus_lens_jobs_upstream_error_total`: Upstream errors
- `primus_lens_jobs_event_processing_duration_seconds`: Event processing duration

## Database Schema

The module interacts with the following database tables:

- `nodes`: Node information
- `gpu_devices`: GPU device details
- `rdma_devices`: RDMA/InfiniBand device details
- `node_device_changelog`: Device change audit log
- `gpu_pods`: GPU pod tracking
- `gpu_workload`: GPU workload tracking
- `node_containers`: Container information
- `node_container_devices`: Container-device associations
- `node_container_events`: Container lifecycle events
- `storage`: Storage backend information

## Development

### Adding a New Job

1. Create a new package under `pkg/jobs/`
2. Implement the `Job` interface
3. Register the job in `pkg/jobs/interface.go`:

```go
var jobs = []Job{
    // ... existing jobs
    &your_job.YourJob{},
}
```

### Testing

A test gRPC server is available for development:

```bash
cd cmd/grpc-server-test
go run main.go
```

## Monitoring

The module provides comprehensive monitoring through:

1. **Prometheus Metrics**: Expose metrics at the standard Prometheus endpoint
2. **Logging**: Structured logging using the core logger package
3. **Database Auditing**: Device change logs and event history

## Error Handling

- Jobs are executed independently; errors in one job do not affect others
- All errors are logged with context for troubleshooting
- Failed operations are counted in error metrics
- Database operations use transactions where appropriate

## Performance Considerations

- Jobs run concurrently using goroutines and sync.WaitGroup
- Event processing is asynchronous
- Database operations are optimized with proper indexing
- Metrics collection uses efficient gauges and counters

## License

This module is part of the AMD-AGI Primus-SaFE project.

## Related Modules

- **Primus Lens Core**: Core functionality and shared utilities
- **Primus Lens API**: REST API for accessing collected data
- **Node Exporter**: Agent running on nodes to collect device metrics and send events

