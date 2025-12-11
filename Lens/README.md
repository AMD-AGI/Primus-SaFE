# Primus-Lens

A comprehensive Kubernetes GPU cluster monitoring and management platform designed for AI/ML workloads, with specialized support for AMD GPUs.

## Overview

Primus-Lens is an observability and management system that provides deep visibility into GPU clusters running AI/ML workloads on Kubernetes. It tracks GPU allocation, utilization, node health, network performance, and training metrics in real-time, offering comprehensive monitoring from the cluster level down to individual devices.

## Key Features

- **GPU Resource Management**: Complete tracking of AMD GPU allocation, utilization, and performance metrics
- **Multi-Cluster Support**: Unified view and management across multiple Kubernetes clusters
- **Network Monitoring**: eBPF-based TCP flow analysis with minimal overhead
- **Hardware Device Tracking**: GPU, RDMA/InfiniBand, and device-to-container mapping
- **Workload Lifecycle Management**: Track GPU workloads from creation to termination
- **Training Performance Analysis**: Extract and analyze training metrics from AI/ML workloads
- **Real-Time Telemetry**: gRPC-based event streaming and metrics collection
- **RESTful API**: Comprehensive API for cluster monitoring and management
- **Storage Monitoring**: Track storage backend health and usage
- **System Optimization**: Automatic kernel parameter tuning for containerized workloads

## Architecture

Primus-Lens consists of multiple interconnected modules working together to provide comprehensive cluster observability:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Node Level (DaemonSets)                                    │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │ │
│  │  │ Node         │  │ Network      │  │ System       │    │ │
│  │  │ Exporter     │  │ Exporter     │  │ Tuner        │    │ │
│  │  │              │  │              │  │              │    │ │
│  │  │ - GPU Info   │  │ - eBPF TCP   │  │ - Kernel     │    │ │
│  │  │ - RDMA       │  │ - Flow       │  │   Params     │    │ │
│  │  │ - Containers │  │   Analysis   │  │ - File Limits│    │ │
│  │  └──────┬───────┘  └──────────────┘  └──────────────┘    │ │
│  └─────────┼────────────────────────────────────────────────┘ │
│            │ gRPC Events                                       │
│  ┌─────────▼────────────────────────────────────────────────┐ │
│  │ Control Plane                                             │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │ │
│  │  │ Telemetry    │  │ Jobs         │  │ GPU Resource │   │ │
│  │  │ Processor    │  │ Scheduler    │  │ Exporter     │   │ │
│  │  │              │  │              │  │              │   │ │
│  │  │ - Metrics    │  │ - Node Info  │  │ - GPU Pods   │   │ │
│  │  │   Processing │  │ - Devices    │  │ - Workload   │   │ │
│  │  │ - Log        │  │ - GPU Stats  │  │   Lifecycle  │   │ │
│  │  │   Analysis   │  │ - Storage    │  │              │   │ │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │ │
│  └─────────┼──────────────────┼──────────────────┼─────────┘ │
│            │                  │                  │            │
│  ┌─────────▼──────────────────▼──────────────────▼─────────┐ │
│  │ Storage & API Layer                                      │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │ │
│  │  │ Database     │  │ Prometheus   │  │ RESTful API  │  │ │
│  │  │ (PostgreSQL/ │  │ (Metrics)    │  │ Service      │  │ │
│  │  │  MySQL)      │  │              │  │              │  │ │
│  │  └──────────────┘  └──────────────┘  └──────┬───────┘  │ │
│  └────────────────────────────────────────────────┼────────┘ │
└───────────────────────────────────────────────────┼──────────┘
                                                    │
                                           ┌────────▼────────┐
                                           │   Web UI /      │
                                           │   Monitoring    │
                                           │   Dashboards    │
                                           └─────────────────┘
```

## Project Structure

```
Lens/
├── modules/
│   ├── core/                              # Core infrastructure library
│   ├── api/                               # RESTful API service for cluster monitoring
│   ├── telemetry-processor/              # Telemetry data processing and enrichment
│   ├── jobs/                              # Scheduled job orchestration for periodic tasks
│   ├── system-tuner/                      # System parameter optimization daemon
│   └── exporters/
│       ├── node-exporter/                 # Node-level monitoring agent (GPU, RDMA, containers)
│       ├── network-exporter/              # eBPF-based network traffic monitoring
│       ├── gpu-resource-exporter/         # GPU resource tracking and workload lifecycle
│       └── multi-cluster-config-exporter/ # Multi-cluster configuration synchronization
│
├── apis/                                  # API definitions and protobuf specifications
├── bootstrap/                             # Bootstrap configurations and scripts
├── ci/                                    # CI/CD pipelines and automation
├── deploy/                                # Kubernetes deployment manifests
├── docs/                                  # Documentation
└── README.md                              # This file
```

## Module Architecture and Build System

Each module in Primus Lens is self-contained with its own build configuration, allowing for independent deployment and custom dependencies.

### Module Structure

```
Lens/modules/
├── api/
│   ├── cmd/
│   ├── pkg/
│   └── installer/
│       └── Dockerfile          # Module-specific Dockerfile
├── jobs/
│   ├── cmd/
│   ├── pkg/
│   └── installer/
│       └── Dockerfile          # With PDF rendering support
├── exporters/
│   ├── node-exporter/
│   │   └── installer/
│   │       └── Dockerfile      # (optional, falls back to ci/Dockerfile)
│   └── ...
└── ...
```

### Build System

#### Dockerfile Location Priority

The CI/CD system uses the following priority for Dockerfile selection:

1. **Module-specific Dockerfile** (Recommended)
   - Location: `{module}/installer/Dockerfile`
   - Used if exists
   - Allows module-specific dependencies

2. **Fallback to central Dockerfile**
   - Location: `Lens/ci/Dockerfile`
   - Used if module doesn't have its own Dockerfile
   - Standard lightweight build

#### GitHub Actions Flow

```yaml
# Automatic Dockerfile detection
if [[ -f "$MODULE_DOCKERFILE" ]]; then
  Use module's own Dockerfile
else
  Use Lens/ci/Dockerfile
fi
```

### Standard Module Types

#### Lightweight Module

Example: `modules/api/`

```
api/
├── cmd/
│   └── primus-lens-api/
│       └── main.go
├── pkg/
│   └── api/
│       └── *.go
└── installer/
    └── Dockerfile              # ~200MB, standard dependencies
```

#### Module with Special Requirements

Example: `modules/jobs/` (needs Chrome for PDF)

```
jobs/
├── cmd/
│   └── primus-lens-jobs/
│       └── main.go
├── pkg/
│   └── jobs/
│       └── *.go
└── installer/
    └── Dockerfile              # ~1GB, includes Chrome/Chromium
```

### Creating a New Module

#### Step 1: Create Module Directory

```bash
mkdir -p Lens/modules/my-module/{cmd/my-module,pkg/my-module,installer}
```

#### Step 2: Create Dockerfile (Optional)

If your module has special dependencies, create `installer/Dockerfile`:

```dockerfile
# Lens/modules/my-module/installer/Dockerfile

ARG BUILDPATH
ARG APPNAME
ARG BASEPATH

# Stage 1: Build
FROM golang:1.24.7 AS builder
WORKDIR /app

ARG GITHUB_TOKEN
ARG BASEPATH
ARG BUILDPATH
ARG APPNAME
ENV BUILDPATH=${BUILDPATH}
ENV APPNAME=${APPNAME}
ENV BASEPATH=${BASEPATH}
ENV GOPRIVATE="github.com/AMD-AGI/*"

RUN git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"

COPY . .
RUN cd $BASEPATH && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

# Stage 2: Runtime
FROM --platform=linux/amd64 docker.io/primussafe/ubuntu-rdma-base:0.0.1

ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

WORKDIR /root/

# Install module-specific dependencies here
# RUN apt-get update && apt-get install -y ...

COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh

CMD ["./run.sh"]
```

#### Step 3: Update GitHub Actions (if needed)

The workflow automatically detects modules. Add to `.github/workflows/primus-lens.yml`:

```yaml
files_yaml: |
  my_module:
    - Lens/modules/my-module/**
    - Lens/modules/core/**
```

And in the matrix generation:

```yaml
if [[ "${{ steps.changed-files.outputs.my_module_any_changed }}" == "true" ]]; then
  MATRIX+='{"name":"my-module","buildpath":"cmd/my-module","basepath":"Lens/modules/my-module/"},'
fi
```

### When to Create Module-Specific Dockerfile

Create `installer/Dockerfile` when:

✅ Module needs special runtime dependencies  
✅ Module needs specific system packages  
✅ Module needs larger base image  
✅ Module has unique requirements (e.g., Chrome, GPU drivers)

Use `Lens/ci/Dockerfile` (no installer directory) when:

✅ Module has standard dependencies only  
✅ Module can use ubuntu-rdma-base image  
✅ No special runtime requirements

### Build Arguments

All Dockerfiles receive these build arguments:

| Argument | Example | Description |
|----------|---------|-------------|
| `BUILDPATH` | `cmd/primus-lens-jobs` | Path to main.go |
| `BASEPATH` | `Lens/modules/jobs/` | Module base path |
| `APPNAME` | `primus-lens-jobs` | Application name |
| `GITHUB_TOKEN` | `ghp_xxx` | GitHub token for private repos |
| `GOPROXY` | `https://goproxy.io,direct` | Go module proxy |

### Testing Locally

#### Build with Module Dockerfile

```bash
# Navigate to project root
cd Primus-SaFE

# Build specific module
buildah bud \
  --build-arg BUILDPATH=cmd/primus-lens-jobs \
  --build-arg BASEPATH=Lens/modules/jobs/ \
  --build-arg APPNAME=primus-lens-jobs \
  --build-arg GITHUB_TOKEN=${GITHUB_TOKEN} \
  -t primus-lens-jobs:test \
  -f Lens/modules/jobs/installer/Dockerfile .

# Run
docker run --rm primus-lens-jobs:test
```

#### Test Dockerfile Selection

```bash
# Check if module Dockerfile exists
MODULE="jobs"
BASEPATH="Lens/modules/${MODULE}/"

if [[ -f "${BASEPATH}installer/Dockerfile" ]]; then
  echo "Will use: ${BASEPATH}installer/Dockerfile"
else
  echo "Will use: Lens/ci/Dockerfile"
fi
```

### Image Size Comparison

| Module | Dockerfile Location | Base Image | Size | Features |
|--------|-------------------|-----------|------|----------|
| **api** | `modules/api/installer/` | ubuntu-rdma-base | ~200MB | Standard |
| **jobs** | `modules/jobs/installer/` | debian+chromium | ~1GB | PDF rendering |
| **node-exporter** | (uses fallback) | ubuntu-rdma-base | ~200MB | Standard |

### Best Practices

#### 1. Self-Contained Modules

✅ Each module should be independently deployable  
✅ Module-specific dependencies in module's Dockerfile  
✅ Don't rely on shared state or assumptions

#### 2. Dockerfile Optimization

✅ Use multi-stage builds  
✅ Clean up package caches  
✅ Only install required dependencies  
✅ Document why each dependency is needed

#### 3. Consistent Structure

✅ Follow the standard directory structure  
✅ Use consistent naming conventions  
✅ Include README in module directory

#### 4. Documentation

✅ Document special dependencies in module README  
✅ Explain why custom Dockerfile is needed  
✅ Provide deployment examples

### Troubleshooting Module Builds

#### Issue: CI not using module Dockerfile

**Problem**: CI still uses `Lens/ci/Dockerfile`

**Solution**:
```bash
# Check file exists
ls -la Lens/modules/your-module/installer/Dockerfile

# Ensure it's committed
git status

# Check GitHub Actions logs for message
"📄 Using module-specific Dockerfile: ..."
```

#### Issue: Build fails with module Dockerfile

**Problem**: Build fails, but works with central Dockerfile

**Solution**:
1. Check all ARGs are properly used
2. Verify COPY paths are correct
3. Test locally first
4. Check build logs for specific errors

#### Issue: Image too large

**Problem**: Module image is unexpectedly large

**Solution**:
1. Check if you're cleaning up package caches
2. Use `.dockerignore` to exclude unnecessary files
3. Consider using Alpine for smaller base
4. Remove unnecessary dependencies

## Core Modules

### 1. Core Module

**Purpose**: Foundational infrastructure library for all Primus-Lens components

**Key Features**:
- Configuration management (YAML-based)
- Database access layer (GORM + auto-generated DAL)
- Logging system (Logrus/Zap with distributed tracing)
- Prometheus metrics collection
- HTTP server framework (Gin)
- Kubernetes client integration
- Multi-cluster support
- Error handling and utilities

**Technology Stack**: Go 1.24+, GORM, Gin, Prometheus, Logrus/Zap, Kubernetes client-go

### 2. API Module

**Purpose**: RESTful API service for cluster monitoring and management

**Key Endpoints**:
- `/api/clusters/*` - Cluster overview, GPU statistics, consumers
- `/api/nodes/*` - Node management, GPU devices, metrics
- `/api/workloads/*` - Workload tracking, hierarchy, performance
- `/api/storage/*` - Storage statistics

**Features**:
- Pagination and filtering support
- Time-series metrics queries
- Training performance data access
- GPU heatmap generation

**Port**: 8080 (default)

### 3. Telemetry Processor

**Purpose**: Process and enrich telemetry data from various sources

**Capabilities**:
- Prometheus remote write protocol support
- Log reception and parsing (HTTP endpoint)
- Training performance extraction from logs
- Device-to-pod and pod-to-workload caching
- Workload context enrichment for metrics
- Log latency tracking

**Key Endpoints**:
- `POST /prometheus` - Metrics ingestion
- `POST /logs` - Log reception
- `GET /pods/cache` - Device-pod cache inspection

### 4. Jobs Module

**Purpose**: Scheduled job orchestration for periodic monitoring tasks

**Jobs**:
- **GPU Allocation** (30s): Cluster GPU allocation rate
- **GPU Consumers** (30s): GPU consumer tracking
- **Node Info** (10s): Node status and resource updates
- **Device Info** (10s): GPU/RDMA device tracking with changelog
- **GPU Workload** (20s): Workload status monitoring
- **GPU Pod** (5s): Pod lifecycle tracking
- **Storage Scan** (1m): Storage backend discovery

**gRPC Server**: Receives container events from node exporters (port 50051)

### 5. System Tuner

**Purpose**: Automatic Linux kernel parameter optimization

**Parameters Managed**:
- `vm.max_map_count`: 262144 (for Elasticsearch, etc.)
- `nofile` limits: 131072 (file descriptors)

**Check Interval**: 30 seconds

**Deployment**: DaemonSet with privileged access

## Exporters

### Node Exporter

**Purpose**: Comprehensive node-level monitoring agent

**Monitored Resources**:
- **AMD GPUs**: Device info, driver version, utilization, DRI mapping
- **RDMA/InfiniBand**: Device discovery, statistics
- **Containers**: Lifecycle events via containerd/Docker
- **Ephemeral Storage**: Pod and node storage metrics

**Collection Intervals**:
- GPU metrics: 5s
- RDMA statistics: 5s
- Ephemeral storage: 10s
- Container events: Real-time stream

**Ports**:
- REST API: Part of main service
- Prometheus metrics: `/metrics` endpoint

### Network Exporter

**Purpose**: eBPF-based network traffic monitoring

**Features**:
- TCP connection tracking (kprobes)
- TCP flow analysis (tracepoints)
- Traffic direction detection (ingress/egress)
- Network policy enforcement
- RTT measurement
- Kubernetes-aware classification

**Metrics**:
- `primus_lens_network_tcp_flow_egress`
- `primus_lens_network_tcp_flow_ingress`
- `primus_lens_network_k8s_tcp_flow`
- `primus_lens_network_tcp_flow_rtt` (histogram)

**Requirements**: Linux kernel 5.8+, CAP_BPF/CAP_PERFMON

### GPU Resource Exporter

**Purpose**: Kubernetes controller for GPU resource tracking

**Responsibilities**:
- Monitor GPU pod lifecycle
- Track workload ownership hierarchy
- Manage finalizers for data persistence
- Create pod snapshots and events
- Maintain workload-pod relationships
- Node kubelet service management

**Data Tracked**:
- GPU workloads
- GPU pods
- Pod resources
- Pod snapshots
- GPU pod events
- Workload-pod references

### Multi-Cluster Config Exporter

**Purpose**: Synchronize storage configurations across multiple clusters

**Workflow**:
1. Watch `multi-k8s-config` secret for cluster configurations
2. Initialize clients for all configured clusters
3. Periodically (30s) fetch `storage-config` secrets from each cluster
4. Aggregate configs into unified `multi-storage-config` secret
5. Hot reload on configuration changes

**Features**:
- Automatic configuration discovery
- Self-healing (reconnects on failures)
- Aggregated configuration management
- Dynamic cluster reload

## Prerequisites

### System Requirements

- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+, or similar)
- **Kubernetes**: v1.23+
- **Go**: 1.24.5+
- **Database**: PostgreSQL 12+ or MySQL 8+
- **Kernel**: Linux 5.8+ (for eBPF support)

### Hardware Requirements

- **GPU**: AMD GPUs with ROCm driver (MI200/MI300 series)
- **RDMA**: InfiniBand or RoCE network (optional)
- **CPU**: Multi-core CPU (8+ cores recommended)
- **Memory**: 16GB+ RAM
- **Storage**: 100GB+ for database and logs

### Software Dependencies

- AMD ROCm/amdgpu driver
- `amd-smi` command-line tool
- Containerd or Docker runtime
- RDMA core libraries (optional, for InfiniBand)
- Prometheus or compatible metrics storage
- Clang/LLVM (for building eBPF programs)

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/AMD-AGI/Primus-SaFE.git
cd Primus-SaFE/Lens
```

### 2. Configure the System

Create a `config.yaml` file:

```yaml
multiCluster: false
httpPort: 8080

# Database configuration
database:
  type: postgres
  host: localhost
  port: 5432
  database: primus_lens
  username: postgres
  password: your_password
  maxIdleConns: 10
  maxOpenConns: 100

# Logging configuration
logging:
  level: info
  format: json
  output: stdout

# Metrics configuration
metrics:
  enabled: true
  port: 9090

# Node Exporter configuration
nodeExporter:
  containerd_socket_path: /run/containerd/containerd.sock
  grpc_server: telemetry-processor:50051

# Jobs configuration
jobs:
  grpc_port: 50051

# Network flow configuration
netflow:
  scan_port_listen_interval_seconds: 30
```

### 3. Build All Modules

```bash
# Build Core (if needed as standalone)
cd modules/core
go build ./...

# Build API
cd ../api
go build -o primus-lens-api ./cmd/primus-lens-api

# Build Telemetry Processor
cd ../telemetry-processor
go build -o telemetry-processor ./cmd/telemetry-processor

# Build Jobs
cd ../jobs
go build -o primus-lens-jobs ./cmd/primus-lens-jobs

# Build System Tuner
cd ../system-tuner
go build -o system-tuner ./cmd/system-tuner

# Build Exporters
cd ../exporters/node-exporter
go build -o node-exporter ./cmd/node-exporter

cd ../network-exporter
go build -o network-exporter ./cmd/network-exporter

cd ../gpu-resource-exporter
go build -o gpu-resource-exporter ./cmd/gpu-resource-exporter

cd ../multi-cluster-config-exporter
go build -o multi-cluster-config-exporter ./cmd/multi-cluster-config-exporter
```

### 4. Database Setup

```bash
# Create database
psql -U postgres -c "CREATE DATABASE primus_lens;"

# Run migrations (handled automatically by core module on first run)
```

### 5. Deploy to Kubernetes

```bash
# Apply deployment manifests
kubectl apply -f deploy/

# Verify deployments
kubectl get pods -n primus-lens

# Check logs
kubectl logs -n primus-lens -l app=primus-lens-api
```

## Configuration

### Multi-Cluster Setup

For multi-cluster monitoring, create a `multi-k8s-config` secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: multi-k8s-config
  namespace: primus-lens
type: Opaque
data:
  cluster1: <base64-encoded-kubeconfig>
  cluster2: <base64-encoded-kubeconfig>
```

### Network Policy Configuration

For network monitoring, configure IP classification policies in `policy.yaml`:

```yaml
internalHosts:
  - 10.0.0.0/8
  - 172.16.0.0/12
  - 192.168.0.0/16
k8sPod:
  - 10.244.0.0/16
k8sSvc:
  - 10.96.0.0/12
dns:
  - 8.8.8.8
  - 8.8.4.4
```

## Monitoring and Observability

### Prometheus Metrics

All modules expose Prometheus metrics at `/metrics` endpoint:

- **Node Exporter**: GPU utilization, RDMA stats, ephemeral storage
- **Network Exporter**: TCP flows, connections, RTT histograms
- **Jobs**: Container events, processing duration
- **Telemetry Processor**: Log latency, event creation
- **API**: HTTP request metrics

### Logs

Structured JSON logs with trace IDs for distributed tracing:

```json
{
  "level": "info",
  "msg": "Request completed",
  "trace_id": "abc123",
  "span_id": "def456",
  "method": "GET",
  "path": "/api/nodes",
  "status": 200,
  "duration": 0.123
}
```

### Health Checks

- API: `GET /healthz`
- Individual modules: Check pod status

## API Documentation

Comprehensive API documentation is available in the [`docs/api/`](docs/api/README.md) directory.

Quick examples:

```bash
# Get cluster overview
curl http://localhost:8080/api/clusters/overview

# List GPU nodes
curl "http://localhost:8080/api/nodes?pageNum=1&pageSize=10"

# Get workload metrics
curl "http://localhost:8080/api/workloads/{uid}/metrics?start=1609459200&end=1609545600"

# Get training performance
curl "http://localhost:8080/api/workloads/{uid}/trainingPerformance"
```

## Performance Considerations

- **Node Exporter**: < 100m CPU, 100-200 MB memory
- **Network Exporter**: < 1% CPU overhead, minimal network impact
- **eBPF**: Zero-copy event collection, kernel-space filtering
- **Database**: Index optimization for frequent queries
- **Caching**: In-memory caches for device and workload mappings
- **Metrics**: TTL-based expiry to prevent unbounded growth

## Security Considerations

- **Privileged Access**: Node Exporter and System Tuner require privileged mode
- **eBPF Safety**: eBPF programs verified by kernel verifier
- **Database**: Use strong passwords, enable SSL/TLS
- **API**: Deploy behind reverse proxy with authentication
- **gRPC**: Enable TLS for production deployments
- **RBAC**: Appropriate Kubernetes RBAC permissions for controllers

## Troubleshooting

### Common Issues

#### GPU Not Detected

```bash
# Check GPU visibility
lspci | grep -i amd

# Verify amd-smi is available
amd-smi list

# Check DRI devices
ls -la /dev/dri/
```

#### eBPF Load Failures

```bash
# Check kernel version
uname -r

# Verify eBPF support
bpftool feature probe

# Install kernel headers
apt-get install linux-headers-$(uname -r)
```

#### Database Connection Issues

- Verify database is running and accessible
- Check credentials in `config.yaml`
- Ensure database exists
- Review database logs

#### Missing Metrics

- Verify Prometheus is scraping endpoints
- Check pod-device cache population
- Review network policies
- Inspect module logs for errors

## Contributing

We welcome contributions! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Follow Go best practices and style guidelines
4. Add tests for new functionality
5. Update documentation as needed
6. Commit your changes (`git commit -m 'Add AmazingFeature'`)
7. Push to the branch (`git push origin feature/AmazingFeature`)
8. Open a Pull Request

## License

This project is part of the Primus-SaFE ecosystem developed by AMD-AGI. See LICENSE file for details.

## Support

For issues, questions, or feature requests:

- Open an issue on GitHub
- Contact the project maintainers
- Consult module-specific README files for detailed documentation

## Roadmap

### Planned Features

- NVIDIA GPU support
- Intel GPU support
- Multi-architecture support (ARM64)
- Enhanced anomaly detection
- Historical trend analysis
- Predictive maintenance
- Web UI dashboard
- Advanced alerting rules
- Service mesh integration
- CRI-O runtime support

## Related Projects

- **Primus-SaFE**: Parent project for AI infrastructure management
- **Kubernetes**: Container orchestration platform
- **Prometheus**: Metrics collection and storage
- **Grafana**: Visualization and dashboards
- **Jaeger**: Distributed tracing

## Acknowledgments

This project leverages several open-source technologies:

- Kubernetes and its ecosystem
- eBPF and Cilium libraries
- Prometheus monitoring stack
- GORM ORM framework
- Gin web framework
- And many more listed in go.mod files

---

**Primus-Lens** - Comprehensive GPU Cluster Observability for AI/ML Workloads

