# Primus-Lens Core Module

## Overview

The `core` module is the fundamental infrastructure library of the Primus-Lens project, providing a complete set of reusable infrastructure components. This module encapsulates commonly used functional modules, including database access, logging, monitoring metrics, HTTP server, Kubernetes client integration, etc., providing unified foundational capabilities for upper-level applications.

## Key Features

- **Configuration Management**: YAML-based configuration loading and management
- **Database Access Layer**: GORM-based ORM framework with auto-generated DAL layer
- **Logging System**: Integration with Logrus and Zap, supporting structured logging and distributed tracing
- **Metrics Collection**: Prometheus-based monitoring metrics collection and reporting
- **HTTP Server**: Gin framework-based RESTful API server
- **Kubernetes Integration**: Encapsulated K8s client toolset
- **Distributed Tracing**: Jaeger-integrated distributed tracing
- **Error Handling**: Unified error code and error handling mechanism
- **Utility Library**: Collection of common utility functions

## Project Structure

```
pkg/
├── clientsets/           # Kubernetes client collection
│   ├── clientset.go     # Main client
│   ├── k8s.go           # K8s client
│   ├── kubelet.go       # Kubelet client
│   ├── multicluster.go  # Multi-cluster support
│   ├── node_exporter.go # Node Exporter client
│   └── storage.go       # Storage client
├── config/              # Configuration management
│   └── config.go        # Configuration loading and structure definition
├── constant/            # Constant definitions
│   ├── device.go        # Device-related constants
│   ├── k8s.go           # K8s-related constants
│   ├── lens.go          # Lens-related constants
│   ├── reconciler.go    # Reconciler constants
│   └── training.go      # Training-related constants
├── context/             # Context management
│   └── context.go       # Context utility functions
├── controller/          # Controller management
│   └── manager.go       # Controller manager
├── database/            # Database access layer
│   ├── dal/             # Data Access Layer (auto-generated)
│   ├── facade.go        # Database facade pattern interface
│   ├── filter/          # Query filters
│   ├── gen/             # Code generation tools
│   └── model/           # Data models
├── errors/              # Error handling
│   ├── error.go         # Error definitions
│   └── error_code.go    # Error code definitions
├── helper/              # Helper utilities
│   ├── fault/           # Fault handling
│   ├── gpu/             # GPU-related utilities
│   ├── kubelet/         # Kubelet utilities
│   ├── metadata/        # Metadata utilities
│   ├── node/            # Node utilities
│   ├── prom/            # Prometheus query utilities
│   ├── rdma/            # RDMA statistics utilities
│   ├── storage/         # Storage utilities
│   └── workload/        # Workload utilities
├── logger/              # Logging system
│   ├── common/          # Common utilities
│   ├── conf/            # Configuration definitions
│   ├── formatter/       # Formatters
│   ├── hook/            # Hooks
│   ├── log/             # Log implementation
│   └── logrus/          # Logrus wrapper
├── metrics/             # Monitoring metrics
│   ├── counter.go       # Counter
│   ├── gauge.go         # Gauge
│   ├── histogram.go     # Histogram
│   ├── timer.go         # Timer
│   └── opts.go          # Options configuration
├── model/               # Business models
│   ├── amd-smi.go       # AMD SMI model
│   ├── cluster.go       # Cluster model
│   ├── gpu.go           # GPU model
│   ├── nodes.go         # Node model
│   ├── workload.go      # Workload model
│   ├── storage.go       # Storage model
│   └── rest/            # REST request/response models
├── pb/                  # Protocol Buffers
│   └── exporter/        # Exporter gRPC definitions
├── router/              # HTTP routing
│   ├── middleware/      # Middleware
│   │   ├── cors.go      # CORS middleware
│   │   ├── handle-error.go  # Error handling middleware
│   │   └── tracing.go   # Tracing middleware
│   └── router.go        # Router initialization
├── server/              # HTTP server
│   ├── server.go        # Server initialization
│   └── health.go        # Health check
├── sql/                 # SQL database utilities
│   ├── callbacks/       # Callbacks
│   ├── conn.go          # Connection management
│   ├── drivers.go       # Database drivers
│   ├── gen/             # Code generation
│   ├── logger.go        # SQL logger
│   ├── metrics/         # SQL metrics
│   └── opts.go          # Options configuration
├── trace/               # Distributed tracing
│   ├── trace.go         # Tracing implementation
│   └── caller.go        # Caller information
└── utils/               # Utility functions
    ├── goroutineUtil/   # Goroutine utilities
    ├── k8sUtil/         # K8s utilities
    ├── mapUtil/         # Map utilities
    ├── netUtil/         # Network utilities
    ├── regexUtil/       # Regex utilities
    ├── sliceUtil/       # Slice utilities
    └── stringUtil/      # String utilities
```

## Quick Start

### Requirements

- Go 1.24.5+
- PostgreSQL or MySQL database
- Kubernetes cluster (optional, for K8s integration features)

### Installation

```bash
go get github.com/AMD-AGI/primus-lens/core
```

### Basic Usage

#### 1. Configuration File

Create a `config.yaml` configuration file:

```yaml
multiCluster: false
httpPort: 8080

controller:
  namespace: default
  leaderElectionId: primus-lens-leader
  metricsPort: 19191
  healthzPort: 19192
  pprofPort: 19193

nodeExporter:
  containerd_socket_path: /run/containerd/containerd.sock
  grpc_server: localhost:9090

jobs:
  grpc_port: 9091

netflow:
  scan_port_listen_interval_seconds: 2
  policy_config_path: /etc/netflow/policy.yaml
```

#### 2. Initialize Server

```go
package main

import (
    "context"
    "github.com/AMD-AGI/primus-lens/core/pkg/server"
)

func main() {
    ctx := context.Background()
    if err := server.InitServer(ctx); err != nil {
        panic(err)
    }
}
```

#### 3. Using Database

```go
import (
    "context"
    "github.com/AMD-AGI/primus-lens/core/pkg/database"
    "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
)

// Create node record
func CreateNode(ctx context.Context) error {
    node := &model.Node{
        Name:    "node-1",
        Address: "192.168.1.100",
        Status:  "Ready",
    }
    return database.CreateNode(ctx, node)
}

// Query node
func GetNode(ctx context.Context, name string) (*model.Node, error) {
    return database.GetNodeByName(ctx, name)
}
```

#### 4. Using Logger

```go
import (
    "github.com/AMD-AGI/primus-lens/core/pkg/logger"
    "github.com/AMD-AGI/primus-lens/core/pkg/logger/conf"
)

func Example() {
    log := logger.NewLogger()
    config := &conf.LogConfig{
        Level: conf.LevelInfo,
    }
    log.InitLogger(config)
    
    log.Infof("Server started on port %d", 8080)
    log.WithField("user_id", 123).Info("User logged in")
}
```

#### 5. Using Metrics

```go
import (
    "github.com/AMD-AGI/primus-lens/core/pkg/metrics"
)

func Example() {
    // Create counter
    counter := metrics.NewCounter("requests_total", "Total requests")
    counter.Inc()
    
    // Create gauge
    gauge := metrics.NewGauge("active_connections", "Active connections")
    gauge.Set(42)
    
    // Create histogram
    histogram := metrics.NewHistogram("request_duration", "Request duration")
    histogram.Observe(0.5)
}
```

#### 6. Using Context

```go
import (
    "context"
    appContext "github.com/AMD-AGI/primus-lens/core/pkg/context"
)

func Example() {
    ctx := context.Background()
    
    // Store object in context
    ctx = appContext.WithObject(ctx, "user_id", "123")
    
    // Get object from context
    if userID, ok := appContext.GetValue(ctx, "user_id"); ok {
        println(userID)
    }
}
```

## Core Components

### Configuration Management

The configuration module supports loading configuration from YAML files. Specify the configuration file path through the `CONFIG_PATH` environment variable (defaults to `config.yaml`).

### Database Access Layer

- Uses GORM as ORM framework
- Supports PostgreSQL and MySQL
- Auto-generated DAL layer provides type-safe database operations
- Provides Facade pattern high-level database operation interfaces

### Logging System

- Supports multiple log levels: Trace, Debug, Info, Warning, Error, Fatal
- Supports structured logging
- Integrated distributed tracing (trace_id, span_id)
- Supports log rotation (lumberjack)

### Monitoring Metrics

- Prometheus-based metrics collection
- Supports Counter, Gauge, Histogram, Summary metric types
- Provides unified metric naming and label management

### HTTP Server

- Based on Gin framework
- Built-in CORS, error handling, tracing middleware
- Supports health check endpoints
- Supports graceful shutdown

### Kubernetes Integration

- Encapsulated K8s client with multi-cluster support
- Kubelet client for node-level operations
- Supports CRI (Container Runtime Interface)
- Provides utility functions for K8s resource operations

## Utility Library

### goroutineUtil

Provides safe goroutine execution with panic recovery.

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/utils/goroutineUtil"

goroutineUtil.SafeGo(func() {
    // Your code
})
```

### k8sUtil

Provides utility functions for K8s resource operations.

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"

// Get Pod owner
owner := k8sUtil.GetPodOwner(pod)

// Parse GVK
gvk := k8sUtil.ParseGVK(obj)
```

### stringUtil, sliceUtil, mapUtil

Provides common string, slice, and map operation functions.

## Development Guide

### Code Generation

Database models and DAL layer are auto-generated using code generation tools:

```bash
# Generate database DAL layer
cd pkg/database/gen
go run main.go

# Generate SQL-related code
cd pkg/sql/gen
go run main.go
```

### Adding New Data Models

1. Define model structure in `pkg/database/model/`
2. Run code generation tools to generate DAL layer
3. Add high-level operation interfaces in `pkg/database/facade.go`

### Adding New Metrics

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/metrics"

var myCounter = metrics.NewCounter(
    "my_metric",
    "Description of my metric",
    metrics.WithNamespace("my_namespace"),
    metrics.WithLabels(map[string]string{"env": "prod"}),
)
```

## Dependencies

Main dependencies include:

- **Web Framework**: gin-gonic/gin
- **ORM**: gorm.io/gorm
- **Database Drivers**: gorm.io/driver/postgres, gorm.io/driver/mysql
- **Logging**: sirupsen/logrus, go.uber.org/zap
- **Monitoring**: prometheus/client_golang
- **Tracing**: uber/jaeger-client-go
- **Kubernetes**: k8s.io/client-go, k8s.io/api
- **Controller Runtime**: sigs.k8s.io/controller-runtime

See `go.mod` file for the complete list of dependencies.

## Best Practices

1. **Use Context Propagation**: All database operations and HTTP requests should pass context
2. **Structured Logging**: Use `WithField` or `WithFields` to add log fields
3. **Error Handling**: Use unified error handling mechanism with error codes and messages
4. **Metric Naming**: Follow Prometheus naming conventions, use lowercase and underscores
5. **Safe Goroutines**: Use `goroutineUtil.SafeGo` to launch goroutines

## Testing

```bash
# Run all tests
go test ./...

# Run tests for specific package
go test ./pkg/database/...

# Run tests with coverage
go test -cover ./...
```

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the project
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Create a Pull Request

## License

See the LICENSE file in the project root directory.

## Contact

For questions or suggestions, please submit an Issue or contact the project maintainers.
