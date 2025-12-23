# Node Exporter

## Overview

The Node Exporter is a comprehensive node-level monitoring agent within the Primus-Lens system that collects hardware device information, container lifecycle events, and resource utilization metrics from Kubernetes nodes. It specializes in AMD GPU monitoring, RDMA/InfiniBand device tracking, and container runtime integration, providing deep visibility into node-level resources.

## Features

- **AMD GPU Monitoring**: Collects AMD GPU device information, driver version, and utilization metrics
- **RDMA/InfiniBand Tracking**: Monitors RDMA devices and network statistics
- **Container Lifecycle Monitoring**: Tracks container events through containerd and Docker runtimes
- **Device-to-Container Mapping**: Associates GPU and RDMA devices with running containers
- **Ephemeral Storage Metrics**: Monitors pod and node ephemeral storage usage
- **Kubelet Integration**: Retrieves pod and resource information from the Kubelet API
- **gRPC Event Streaming**: Streams container events to the central Primus-Lens server
- **Prometheus Metrics Export**: Exposes metrics in Prometheus format
- **REST API**: Provides endpoints for querying device information and metrics
- **DRI Device Mapping**: Maps GPU devices to Linux DRI (Direct Rendering Infrastructure) devices

## Architecture

### Core Components

#### 1. Bootstrap (`pkg/bootstrap`)
- Initializes the server and configuration
- Registers API routes
- Starts collector goroutines

#### 2. Collectors (`pkg/collector`)

##### AMD GPU Collector (`amd-gpu.go`)
- Queries GPU information using AMD SMI (System Management Interface)
- Parses device details: GPU ID, BDF (Bus/Device/Function), ASIC model, serial number
- Maps GPU cards to DRI devices (`/dev/dri/cardX`, `/dev/dri/renderDXXX`)
- Refreshes GPU information every 5 seconds

##### GPU Metrics Collector (`gpu-metrics.go`)
- Collects GPU utilization per device
- Calculates GPU allocation rate based on Kubernetes requests
- Exports metrics:
  - `node_k8s_gpu_allocation_rate`: Percentage of GPUs allocated by Kubernetes
  - `gpu_utilization{gpu_id}`: Per-GPU utilization percentage

##### RDMA Collector (`rdma.go`, `rdma/collector.go`)
- Discovers RDMA devices (InfiniBand, RoCE)
- Collects RDMA statistics using `rdma statistic show` command
- Dynamically creates Prometheus metrics for RDMA counters
- Exports metrics like `rdma_stat_*{device,port}`

##### Container Device Tracker (`pod-device.go`)
- Monitors containerd events: ContainerCreate, TaskStart, TaskExit, TaskOOM, etc.
- Extracts device information from container runtime specs
- Maps GPU and RDMA devices to containers
- Streams container events to the central server via gRPC

##### Ephemeral Storage Monitor (`k8s-ephemeral-storage`)
- Queries Kubelet stats API for ephemeral storage usage
- Tracks per-pod and node-level storage metrics
- Exports metrics:
  - `pod_ephemeral_storage_usage_bytes{namespace,pod,node}`
  - `node_ephemeral_storage_usage_bytes{node}`
  - `node_ephemeral_storage_available_bytes{node}`
  - `node_ephemeral_storage_capacity_bytes{node}`
  - `node_ephemeral_storage_usage_percent{node}`

##### Container Runtime Integrations
- **Containerd** (`containerd/containerd.go`): Primary container runtime support
- **Docker** (`docker/docker.go`): Legacy Docker runtime support

#### 3. API Handlers (`pkg/api`)

Provides REST endpoints for querying node information:

##### GPU Endpoints
- `GET /gpus`: List all GPU devices with detailed information
- `GET /gpuDriverVersion`: Get AMD GPU driver version
- `GET /cardMetrics`: Get GPU utilization and performance metrics
- `GET /driMapping`: Get DRI device to GPU card mapping

##### RDMA Endpoints
- `GET /rdma`: List all RDMA/InfiniBand devices

##### Pod Endpoints
- `GET /pods`: Get container information with device assignments

##### Metrics Endpoint
- `GET /metrics`: Prometheus metrics endpoint

#### 4. gRPC Reporter (`pkg/collector/report`)
- Establishes gRPC streaming connection to central server
- Sends container lifecycle events
- Includes node metadata (node name, node IP) in requests
- Auto-reconnects on connection failure

#### 5. Kubelet Client (`pkg/kubelet`)
- Queries Kubelet API for pod information
- Retrieves resource statistics
- Filters GPU pods based on resource requests

## Data Flow

```
┌──────────────────────────────────────────────────────────┐
│                     Node Host                            │
│                                                           │
│  ┌────────────────────────────────────────────────────┐  │
│  │ Hardware Devices                                   │  │
│  │  - AMD GPUs (/dev/dri/cardX)                       │  │
│  │  - RDMA Devices (/dev/infiniband/uverbsX)          │  │
│  └────────────┬───────────────────────────────────────┘  │
│               │                                           │
│  ┌────────────▼───────────────────────────────────────┐  │
│  │ Container Runtime (containerd/Docker)              │  │
│  │  - Container lifecycle events                      │  │
│  │  - Device assignments                              │  │
│  └────────────┬───────────────────────────────────────┘  │
│               │                                           │
│  ┌────────────▼───────────────────────────────────────┐  │
│  │ Kubelet API                                        │  │
│  │  - Pod stats                                       │  │
│  │  - Resource metrics                                │  │
│  └────────────┬───────────────────────────────────────┘  │
└───────────────┼───────────────────────────────────────────┘
                │
┌───────────────▼───────────────────────────────────────────┐
│            Node Exporter Process                          │
│                                                            │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ Collectors (5-60s refresh cycles)                    │ │
│  │  ┌──────────────┐  ┌──────────────┐                 │ │
│  │  │ AMD GPU      │  │ RDMA         │                 │ │
│  │  │ Collector    │  │ Collector    │                 │ │
│  │  └──────────────┘  └──────────────┘                 │ │
│  │  ┌──────────────┐  ┌──────────────┐                 │ │
│  │  │ Container    │  │ Ephemeral    │                 │ │
│  │  │ Tracker      │  │ Storage      │                 │ │
│  │  └──────────────┘  └──────────────┘                 │ │
│  └────────────┬─────────────────────────────────────────┘ │
│               │                                            │
│  ┌────────────▼─────────────────────────────────────────┐ │
│  │ In-Memory State                                      │ │
│  │  - GPU device info                                   │ │
│  │  - RDMA device info                                  │ │
│  │  - DRI mappings                                      │ │
│  │  - Prometheus metrics                                │ │
│  └────────────┬─────────────────────────────────────────┘ │
│               │                                            │
│  ┌────────────▼─────────────────────────────────────────┐ │
│  │ Export Interfaces                                    │ │
│  │  ┌───────────────┐  ┌───────────────┐               │ │
│  │  │ REST API      │  │ gRPC Stream   │               │ │
│  │  │ (Gin)         │  │ (Events)      │               │ │
│  │  └───────────────┘  └───────────────┘               │ │
│  │  ┌───────────────┐                                   │ │
│  │  │ Prometheus    │                                   │ │
│  │  │ /metrics      │                                   │ │
│  │  └───────────────┘                                   │ │
│  └──────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

## Metrics Exported

### GPU Metrics

#### GPU Utilization
```
gpu_utilization{gpu_id="<id>"} <percentage>
```
Current GPU utilization percentage per device.

#### GPU Allocation Rate
```
node_k8s_gpu_allocation_rate <percentage>
```
Percentage of total GPUs allocated by Kubernetes workloads.

### RDMA Metrics

RDMA statistics are dynamically created based on available counters:

```
rdma_stat_rx_write_requests{device="<mlx5_X>",port="<port>"} <count>
rdma_stat_rx_read_requests{device="<mlx5_X>",port="<port>"} <count>
rdma_stat_tx_send_requests{device="<mlx5_X>",port="<port>"} <count>
... (additional RDMA counters)
```

### Ephemeral Storage Metrics

#### Pod-Level Storage
```
pod_ephemeral_storage_usage_bytes{namespace="<ns>",pod="<name>",node="<node>"} <bytes>
```

#### Node-Level Storage
```
node_ephemeral_storage_usage_bytes{node="<node>"} <bytes>
node_ephemeral_storage_available_bytes{node="<node>"} <bytes>
node_ephemeral_storage_capacity_bytes{node="<node>"} <bytes>
node_ephemeral_storage_usage_percent{node="<node>"} <ratio>
```

## API Reference

### GPU Endpoints

#### GET /gpus
Returns detailed information about all GPUs on the node.

**Response Example**:
```json
{
  "code": 0,
  "data": [
    {
      "gpu": 0,
      "asic": {
        "market_name": "AMD Instinct MI300X",
        "device_id": "0x74a1",
        "asic_serial": "0x123456789"
      },
      "bus": {
        "bdf": "0000:c1:00.0"
      },
      "dri_device": {
        "card": "/dev/dri/card0",
        "render": "/dev/dri/renderD128",
        "pci_addr": "0000:c1:00.0"
      }
    }
  ],
  "message": "success"
}
```

#### GET /gpuDriverVersion
Returns the AMD GPU driver version.

**Response Example**:
```json
{
  "code": 0,
  "data": "6.2.4",
  "message": "success"
}
```

#### GET /cardMetrics
Returns real-time GPU utilization and performance metrics.

**Response Example**:
```json
{
  "code": 0,
  "data": [
    {
      "gpu": 0,
      "gpu_use_percent": 75.5,
      "memory_use_percent": 60.2
    }
  ],
  "message": "success"
}
```

#### GET /driMapping
Returns the mapping between DRI devices and GPU cards.

**Response Example**:
```json
{
  "code": 0,
  "data": {
    "/dev/dri/card0": {
      "card": "/dev/dri/card0",
      "render": "/dev/dri/renderD128",
      "pci_addr": "0000:c1:00.0",
      "card_id": 0
    }
  },
  "message": "success"
}
```

### RDMA Endpoints

#### GET /rdma
Returns information about RDMA/InfiniBand devices.

**Response Example**:
```json
{
  "code": 0,
  "data": [
    {
      "if_index": 0,
      "if_name": "mlx5_0",
      "node_guid": "98:03:9b:ff:fe:12:34:56",
      "sys_image_guid": "98:03:9b:ff:fe:12:34:56",
      "port_guid": "98:03:9b:12:34:56:78:90",
      "link_layer": "InfiniBand"
    }
  ],
  "message": "success"
}
```

### Pod Endpoints

#### GET /pods
Returns container information with device assignments.

**Response Example**:
```json
[
  {
    "id": "abc123...",
    "pod_name": "gpu-workload-pod",
    "pod_namespace": "default",
    "pod_uuid": "uuid-123",
    "devices": {
      "gpu": [
        {
          "name": "AMD Instinct MI300X",
          "id": 0,
          "path": "/dev/dri/card0",
          "kind": "GPU",
          "uuid": "0x74a1",
          "serial": "0x123456789",
          "slot": "0000:c1:00.0"
        }
      ],
      "infiniband": [
        {
          "name": "mlx5_0",
          "id": 0,
          "path": "/dev/infiniband/uverbs0",
          "kind": "RDMA",
          "uuid": "98:03:9b:ff:fe:12:34:56"
        }
      ]
    }
  }
]
```

## Configuration

### Environment Variables

- `NODE_NAME`: Kubernetes node name (required)
- `NODE_IP`: Node IP address (required)

### Config File Options

```yaml
node_exporter:
  containerd_socket_path: "/hostrun/containerd/containerd.sock"  # Path to containerd socket
  grpc_server: "<telemetry-processor-address>:50051"             # gRPC server address for event streaming
  netflow:
    scan_port_listen_interval: 30s                               # Port scanning interval
```

### Required Host Mounts

The node exporter requires access to several host paths:

- `/hostrun/containerd/containerd.sock` → `/run/containerd/containerd.sock`: Containerd socket
- `/hostrun/docker.sock` → `/run/docker.sock`: Docker socket (optional)
- `/hostdev/dri/by-path` → `/dev/dri/by-path`: GPU DRI devices
- `/host-proc` → `/proc`: Host proc filesystem

## Installation

### Prerequisites

- Kubernetes cluster
- AMD GPUs with ROCm/amdgpu driver installed
- Containerd or Docker runtime
- RDMA/InfiniBand hardware (optional)
- Go 1.24.5 or higher (for building)

### Build

```bash
cd Lens/modules/exporters/node-exporter
go build -o node-exporter ./cmd/node-exporter
```

### Deployment

The module is typically deployed as a DaemonSet to run on every node:

**Example DaemonSet snippet**:
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: primus-lens-node-exporter
  namespace: primus-lens
spec:
  selector:
    matchLabels:
      app: primus-lens-node-exporter
  template:
    metadata:
      labels:
        app: primus-lens-node-exporter
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: node-exporter
        image: primus-lens/node-exporter:latest
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: NODE_IP
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
        volumeMounts:
        - name: containerd-sock
          mountPath: /hostrun/containerd
        - name: docker-sock
          mountPath: /hostrun
        - name: dev-dri
          mountPath: /hostdev/dri
        - name: proc
          mountPath: /host-proc
        securityContext:
          privileged: true
      volumes:
      - name: containerd-sock
        hostPath:
          path: /run/containerd
      - name: docker-sock
        hostPath:
          path: /run
      - name: dev-dri
        hostPath:
          path: /dev/dri
      - name: proc
        hostPath:
          path: /proc
```

## How It Works

### GPU Discovery and Monitoring

1. **Device Discovery**:
   - Executes `amd-smi` command to enumerate GPUs
   - Parses output to extract GPU ID, BDF, ASIC model, serial number
   - Scans `/hostdev/dri/by-path` for DRI device symlinks
   - Matches GPU devices to DRI cards and render nodes

2. **Metrics Collection**:
   - Queries AMD SMI for GPU utilization and memory usage
   - Queries Kubelet API for GPU allocation information
   - Calculates allocation rate: (allocated GPUs / total GPUs) × 100
   - Updates Prometheus metrics every 5 seconds

3. **DRI Mapping**:
   - Parses symlinks like `pci-0000:c1:00.0-card` and `pci-0000:c1:00.0-render`
   - Extracts PCI address from symlink name
   - Resolves target paths (`/dev/dri/card0`, `/dev/dri/renderD128`)
   - Creates bidirectional mapping between PCI addresses and DRI devices

### Container Lifecycle Tracking

1. **Initial Snapshot**:
   - On startup, lists all running containers via containerd CRI API
   - Extracts device information from container runtime specs
   - Reports snapshot to central server via gRPC

2. **Event Monitoring**:
   - Subscribes to containerd event stream
   - Receives events: ContainerCreate, TaskStart, TaskExit, TaskOOM, etc.
   - Updates container state and device assignments
   - Streams events to central server in real-time

3. **Device Assignment**:
   - Reads container runtime spec from containerd
   - Extracts device paths from `spec.linux.devices`
   - Matches device paths against GPU and RDMA device mappings
   - Constructs container-to-device associations

### RDMA Monitoring

1. **Device Discovery**:
   - Uses `ibv_devinfo` command to enumerate RDMA devices
   - Extracts device name, GUID, link layer (IB/RoCE)
   - Maps devices to interface indices

2. **Statistics Collection**:
   - Executes `rdma statistic show` command
   - Parses output to extract counters (RX/TX requests, bytes, etc.)
   - Dynamically creates Prometheus metrics for each counter type
   - Updates metrics every 5 seconds

### Ephemeral Storage Monitoring

1. **Kubelet Stats Query**:
   - Queries Kubelet stats API endpoint
   - Retrieves pod-level and node-level storage statistics

2. **Metric Generation**:
   - Extracts ephemeral storage usage for each pod
   - Calculates node-level usage, available, and capacity
   - Computes usage percentage: (used / capacity) × 100
   - Updates metrics every 10 seconds

## Troubleshooting

### AMD SMI Command Not Found

**Error**: `amd-smi: command not found`

**Solutions**:
- Install AMD ROCm/amdgpu driver on the host
- Ensure `amd-smi` is in the container PATH
- Mount ROCm installation directory into the container

### No GPU Devices Detected

**Symptom**: Empty response from `/gpus` endpoint

**Causes**:
- AMD GPU driver not installed
- DRI devices not mounted into container
- Insufficient permissions

**Solutions**:
```bash
# Check if GPUs are visible on host
lspci | grep -i vga
lspci | grep -i amd

# Check if DRI devices exist
ls -la /dev/dri/

# Verify container has access
kubectl exec -it <pod> -- ls -la /hostdev/dri/
```

### Containerd Connection Failed

**Error**: `containerd connection failed`

**Solutions**:
- Verify containerd socket path: `/run/containerd/containerd.sock`
- Check socket is mounted into container
- Ensure container has sufficient permissions (privileged mode)

### gRPC Stream Disconnections

**Symptom**: Container events not reaching central server

**Causes**:
- Network connectivity issues
- gRPC server unavailable
- Authentication failures

**Solutions**:
- Check gRPC server address configuration
- Verify network connectivity: `telnet <server> 50051`
- Check server logs for connection errors
- Stream auto-reconnects; monitor logs for retry attempts

### RDMA Statistics Not Available

**Symptom**: No `rdma_stat_*` metrics

**Causes**:
- RDMA tools not installed
- No RDMA hardware present
- InfiniBand drivers not loaded

**Solutions**:
```bash
# Install RDMA tools (Ubuntu/Debian)
apt-get install rdma-core

# Check RDMA devices
rdma link show
ibv_devinfo

# Load IB drivers
modprobe ib_uverbs
modprobe mlx5_ib
```

### High Memory Usage

**Symptom**: Node exporter consuming excessive memory

**Causes**:
- Large number of containers
- Frequent container churn
- Memory leaks in event processing

**Solutions**:
- Monitor goroutine count
- Check for goroutine leaks
- Adjust event buffer sizes
- Restart node exporter periodically

## Performance Considerations

### Collection Intervals

- **GPU metrics**: 5 seconds
- **RDMA statistics**: 5 seconds
- **RDMA device discovery**: 60 seconds
- **Ephemeral storage**: 10 seconds
- **Container events**: Real-time stream

### Resource Usage

- **CPU**: < 100m (0.1 core) typical
- **Memory**: 100-200 MB typical (depends on container count)
- **Network**: Minimal (event streaming only)

### Optimization Tips

1. **Adjust collection intervals** based on monitoring needs
2. **Filter containers** to only track GPU/RDMA containers
3. **Batch events** before sending to reduce gRPC overhead
4. **Limit metric cardinality** for large-scale deployments

## Dependencies

### Core Dependencies
- `github.com/AMD-AGI/Primus-SaFE/Lens/core`: Core Primus-Lens functionality
- `github.com/containerd/containerd`: Containerd client
- `github.com/docker/docker`: Docker client
- `github.com/prometheus/client_golang`: Prometheus metrics
- `google.golang.org/grpc`: gRPC client
- `k8s.io/cri-api`: Kubernetes CRI API
- `k8s.io/kubelet`: Kubelet API types

### System Dependencies
- AMD ROCm/amdgpu driver
- `amd-smi` command-line tool
- RDMA core libraries (optional)
- `rdma` and `ibv_devinfo` tools (optional)

## Security Considerations

- Requires privileged mode for device access
- Accesses host filesystems and sockets
- Reads container runtime information
- No credential or secret exposure in metrics
- gRPC communication should use TLS in production

## Future Enhancements

- NVIDIA GPU support
- Intel GPU support
- NVMe-oF device monitoring
- PCIe topology mapping
- Device health monitoring
- Predictive failure detection
- Multi-runtime support (CRI-O, etc.)
- Enhanced device telemetry

## Contributing

This module is part of the AMD-AGI Primus-SaFE project. For contributions, please follow the project's contribution guidelines.

## License

This module is part of the Primus-SaFE project and follows the project's licensing terms.

## Related Modules

- **Primus-Lens Core**: Provides base functionality and shared libraries
- **GPU Resource Exporter**: Tracks GPU resource allocation in Kubernetes
- **Network Exporter**: Monitors network traffic with eBPF
- **Telemetry Processor**: Processes metrics and events from exporters

