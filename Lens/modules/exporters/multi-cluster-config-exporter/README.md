# Multi-Cluster Config Exporter

A Kubernetes operator that automatically synchronizes storage configurations across multiple Kubernetes clusters in the Primus-Lens ecosystem.

## Overview

The Multi-Cluster Config Exporter is a component of the Primus-SaFE/Lens system that monitors and aggregates storage configuration secrets from multiple Kubernetes clusters. It watches for configuration changes and periodically syncs storage configs to maintain consistency across a multi-cluster environment.

## Features

- **Automatic Configuration Discovery**: Watches Kubernetes secrets for multi-cluster configuration changes
- **Periodic Synchronization**: Automatically syncs storage configurations from all configured clusters every 30 seconds
- **Self-Healing**: Automatically reconnects and recovers from watch failures
- **Aggregated Configuration**: Collects and consolidates storage configs into a single unified secret
- **Hot Reload**: Dynamically reloads cluster configurations when changes are detected

## Architecture

The exporter consists of three main components:

### 1. Main Entry Point (`cmd/multi-cluster-config-exporter/main.go`)
Initializes the server with the bootstrap function to start the exporter.

### 2. Bootstrap Module (`pkg/bootstrap/bootstrap.go`)
- Registers required Kubernetes schemes
- Initializes the MultiClusterStorageConfigListener
- Sets up the controller framework

### 3. Controller (`pkg/controller/multi_cluster.go`)
The core logic that:
- Watches the `multi-k8s-config` secret for cluster configuration changes
- Collects storage configuration secrets from all configured clusters
- Aggregates the collected configs into a unified `multi-storage-config` secret
- Manages periodic sync tasks with automatic restart on configuration changes

## How It Works

1. **Watch Phase**: The exporter starts by watching the `multi-k8s-config` secret in the configured namespace
2. **Discovery Phase**: When cluster configurations are detected, it initializes clients for all configured clusters
3. **Collection Phase**: Periodically (every 30 seconds), it fetches the storage config secret from each cluster
4. **Aggregation Phase**: Collects all storage configs and updates the `multi-storage-config` secret in the current cluster
5. **Update Phase**: When the `multi-k8s-config` secret is modified, the exporter automatically reloads and restarts the sync process

## Configuration

The exporter monitors the following Kubernetes secrets:

- **Input Secret**: `multi-k8s-config` - Contains kubeconfig data for all clusters to monitor
- **Output Secret**: `multi-storage-config` - Contains aggregated storage configurations from all clusters
- **Source Secret**: `storage-config` - The storage configuration secret fetched from each cluster

## Deployment

### Prerequisites

- Go 1.24.5 or higher
- Access to Kubernetes cluster(s)
- Appropriate RBAC permissions to read/write secrets

### Building

```bash
cd cmd/multi-cluster-config-exporter
go build -o multi-cluster-config-exporter
```

### Running

```bash
./multi-cluster-config-exporter
```

The exporter will automatically:
1. Connect to the Kubernetes cluster
2. Start watching for configuration changes
3. Begin periodic synchronization tasks

## Development

### Project Structure

```
multi-cluster-config-exporter/
├── cmd/
│   └── multi-cluster-config-exporter/
│       └── main.go                    # Application entry point
├── pkg/
│   ├── bootstrap/
│   │   └── bootstrap.go              # Initialization logic
│   └── controller/
│       └── multi_cluster.go          # Main controller implementation
├── go.mod                            # Go module definition
├── go.sum                            # Go module checksums
└── README.md                         # This file
```

### Key Dependencies

- `k8s.io/api` - Kubernetes API types
- `k8s.io/apimachinery` - Kubernetes API machinery
- `k8s.io/client-go` - Kubernetes client library
- `github.com/AMD-AGI/primus-lens/core` - Primus-Lens core framework

## Logging

The exporter provides detailed logging for:
- Configuration change detection
- Cluster connection status
- Sync operation progress and results
- Error conditions and retry attempts

## Error Handling

- **Watch Failures**: Automatically reconnects after 10 seconds
- **Cluster Fetch Failures**: Logs warnings and continues with other clusters
- **Secret Not Found**: Creates new secret if it doesn't exist

## Monitoring

The exporter logs key operational events:
- Cluster configuration changes
- Successful/failed sync operations
- Number of clusters configured and synced
- Connection status for each cluster

## License

This project is part of the Primus-SaFE/Lens system developed by AMD-AGI.

