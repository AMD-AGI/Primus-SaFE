# Primus-Lens API Module

## Overview

The `api` module is the RESTful API service of the Primus-Lens project, providing a complete set of HTTP APIs for cluster monitoring, node management, workload tracking, and GPU resource management. This module builds upon the `core` module infrastructure and exposes monitoring data and management capabilities through a RESTful interface.

## Key Features

- **Cluster Management APIs**: Cluster overview, GPU allocation, utilization statistics, consumer information
- **Node Management APIs**: GPU node list, node details, GPU device information, metrics queries
- **Workload Management APIs**: Workload listing, hierarchy, detailed information, metrics, training performance
- **GPU Monitoring APIs**: GPU utilization, allocation rate, historical trends, heatmaps
- **Storage Management APIs**: Storage statistics and usage information
- **RESTful Architecture**: Standard HTTP REST APIs with JSON request/response format
- **Middleware Integration**: CORS, error handling, distributed tracing, logging
- **Database Integration**: Seamless integration with core module's database layer

## Project Structure

```
api/
├── cmd/
│   └── primus-lens-api/
│       └── main.go           # Application entry point
├── pkg/
│   ├── api/                  # API handlers
│   │   ├── router.go         # Route registration
│   │   ├── cluster.go        # Cluster management APIs
│   │   ├── node.go           # Node management APIs
│   │   ├── workload.go       # Workload management APIs
│   │   ├── gpu.go            # GPU device APIs
│   │   ├── metrics.go        # Metrics query APIs
│   │   └── storage.go        # Storage management APIs
│   └── bootstrap/
│       └── bootstrap.go      # Server initialization and configuration
├── go.mod
└── go.sum
```

## API Endpoints

### Cluster Management

#### GET /api/clusters/overview
Get cluster overview with node statistics, GPU allocation, utilization, and storage information.

**Response:**
```json
{
  "code": 0,
  "data": {
    "totalNodes": 10,
    "healthyNodes": 9,
    "faultyNodes": 1,
    "fullyIdleNodes": 3,
    "partiallyIdleNodes": 4,
    "busyNodes": 2,
    "allocationRate": 0.75,
    "utilization": 0.62,
    "storageStat": { ... },
    "rdmaClusterStat": { ... }
  }
}
```

#### GET /api/clusters/consumers
Get GPU consumers (workloads) with pagination support.

**Query Parameters:**
- `pageNum` (int): Page number (default: 1)
- `pageSize` (int): Page size (default: 10)

**Response:**
```json
{
  "code": 0,
  "data": {
    "data": [
      {
        "kind": "Job",
        "name": "training-job-1",
        "namespace": "default",
        "uid": "abc123",
        "stat": {
          "gpuRequest": 8,
          "gpuUtilization": 0.85
        }
      }
    ],
    "total": 50
  }
}
```

#### GET /api/clusters/gpuHeatmap
Get GPU heatmap with top K GPU power and utilization statistics.

### Node Management

#### GET /api/nodes
Get list of GPU nodes with allocation and utilization information.

#### GET /api/nodes/:name
Get detailed information for a specific node.

**Response:**
```json
{
  "code": 0,
  "data": {
    "name": "gpu-node-1",
    "address": "192.168.1.100",
    "status": "Ready",
    "gpuModel": "AMD MI300X",
    "gpuCount": 8,
    "allocatedGpus": 6,
    "utilizationRate": 0.78
  }
}
```

#### GET /api/nodes/:name/gpuDevices
Get GPU device information for a specific node.

#### GET /api/nodes/:name/gpuMetrics
Get GPU metrics for a specific node.

**Query Parameters:**
- `start` (int64): Start timestamp (unix seconds)
- `end` (int64): End timestamp (unix seconds)
- `step` (int): Query step in seconds (default: 60)

#### GET /api/nodes/:name/workloads
Get workloads running on a specific node.

#### GET /api/nodes/:name/workloadsHistory
Get historical workload information for a specific node.

#### GET /api/nodes/gpuAllocation
Get GPU allocation information for all nodes.

#### GET /api/nodes/gpuUtilization
Get GPU utilization statistics for the cluster.

#### GET /api/nodes/gpuUtilizationHistory
Get historical GPU utilization data.

**Query Parameters:**
- `start` (int64): Start timestamp (unix seconds)
- `end` (int64): End timestamp (unix seconds)
- `step` (int): Query step in seconds (default: 60)

### Workload Management

#### GET /api/workloads
List workloads with filtering and pagination.

**Query Parameters:**
- `pageNum` (int): Page number
- `pageSize` (int): Page size
- `name` (string): Filter by workload name
- `kind` (string): Filter by workload kind (Job, Deployment, etc.)
- `namespace` (string): Filter by namespace
- `status` (string): Filter by status
- `labels` (string): Filter by labels

#### GET /api/workloads/:uid
Get detailed information for a specific workload.

#### GET /api/workloads/:uid/hierarchy
Get workload hierarchy (parent-child relationships).

#### GET /api/workloads/:uid/metrics
Get metrics for a specific workload.

**Query Parameters:**
- `start` (int64): Start timestamp
- `end` (int64): End timestamp
- `step` (int): Query step in seconds

#### GET /api/workloads/:uid/trainingPerformance
Get training performance metrics for AI/ML workloads.

#### GET /api/workloadMetadata
Get metadata for all workloads.

### Storage Management

#### GET /api/storage/stat
Get storage statistics including capacity, usage, and availability.

## Quick Start

### Requirements

- Go 1.24.5+
- PostgreSQL or MySQL database (configured in core module)
- Kubernetes cluster with GPU nodes
- Primus-Lens core module
- Prometheus or compatible metrics storage

### Installation

```bash
# Navigate to the api module directory
cd Lens/modules/api

# Install dependencies
go mod download

# Build the application
go build -o primus-lens-api ./cmd/primus-lens-api
```

### Configuration

The API module uses the configuration from the core module. Create a `config.yaml` file:

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
  password: password

# Logging configuration
logging:
  level: info
  format: json

# Metrics configuration
metrics:
  enabled: true
  port: 9090

# Tracing configuration
tracing:
  enabled: true
  jaeger:
    endpoint: http://localhost:14268/api/traces
```

### Running the Server

```bash
# Set config path (optional, defaults to config.yaml)
export CONFIG_PATH=/path/to/config.yaml

# Run the server
./primus-lens-api
```

The API server will start on the configured HTTP port (default: 8080).

### Using the APIs

```bash
# Get cluster overview
curl http://localhost:8080/api/clusters/overview

# List GPU nodes
curl http://localhost:8080/api/nodes

# Get node details
curl http://localhost:8080/api/nodes/gpu-node-1

# List workloads
curl "http://localhost:8080/api/workloads?pageNum=1&pageSize=10"

# Get workload details
curl http://localhost:8080/api/workloads/{uid}

# Get GPU utilization history
curl "http://localhost:8080/api/nodes/gpuUtilizationHistory?start=1609459200&end=1609545600&step=300"
```

## Development

### Adding New APIs

1. **Define Handler Function**: Create a handler function in the appropriate file (cluster.go, node.go, etc.)

```go
func getMyResource(c *gin.Context) {
    // Query data
    data, err := myHelper.GetData(c)
    if err != nil {
        _ = c.Error(err)
        return
    }
    
    // Return response
    c.JSON(http.StatusOK, rest.SuccessResp(c, data))
}
```

2. **Register Route**: Add the route in `router.go`

```go
func RegisterRouter(group *gin.RouterGroup) error {
    myGroup := group.Group("/myresources")
    {
        myGroup.GET("", getMyResourceList)
        myGroup.GET(":id", getMyResource)
    }
    return nil
}
```

3. **Use Helper Functions**: Leverage helper functions from the core module

```go
import (
    "github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
    "github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
)

data, err := gpu.GetGpuNodes(ctx, clientsets.GetCurrentClusterK8SClientSet(), metadata.GpuVendorAMD)
```

### Error Handling

All API handlers should use the unified error handling mechanism:

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/errors"

// Return error through gin context
if err != nil {
    _ = c.Error(errors.WrapError(err, "Failed to get data", errors.CodeDatabaseError))
    return
}

// Or create custom error
if resource == nil {
    _ = c.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
    return
}
```

### Response Format

Use the standard response format from the core module:

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/model/rest"

// Success response
c.JSON(http.StatusOK, rest.SuccessResp(c, data))

// Error responses are handled by middleware
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests for specific package
go test ./pkg/api/...

# Run tests with coverage
go test -cover ./...

# Run integration tests (requires running database and K8s cluster)
go test -tags=integration ./...
```

## Deployment

### Docker

```dockerfile
FROM golang:1.24.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o primus-lens-api ./cmd/primus-lens-api

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/primus-lens-api .
COPY config.yaml .

EXPOSE 8080
CMD ["./primus-lens-api"]
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: primus-lens-api
  template:
    metadata:
      labels:
        app: primus-lens-api
    spec:
      containers:
      - name: api
        image: primus-lens-api:latest
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: /etc/primus-lens/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/primus-lens
      volumes:
      - name: config
        configMap:
          name: primus-lens-config
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-api
spec:
  selector:
    app: primus-lens-api
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

## Dependencies

Main dependencies include:

- **Core Module**: github.com/AMD-AGI/primus-lens/core
- **Web Framework**: gin-gonic/gin
- **Kubernetes Client**: k8s.io/client-go, k8s.io/api

The module inherits all dependencies from the core module. See `go.mod` for the complete list.

## Best Practices

1. **Context Propagation**: Always pass context from gin.Context to helper functions
2. **Error Handling**: Use unified error handling with appropriate error codes
3. **Pagination**: Implement pagination for list APIs to avoid large responses
4. **Query Parameters**: Validate query parameters and provide sensible defaults
5. **Response Format**: Use consistent response format across all APIs
6. **Logging**: Add contextual logging for debugging and monitoring
7. **Metrics**: Expose metrics for API performance monitoring

## Monitoring

The API module exposes Prometheus metrics including:

- `http_requests_total`: Total HTTP requests by method, path, and status
- `http_request_duration_seconds`: HTTP request latency histogram
- `http_requests_in_flight`: Current number of HTTP requests being processed

Access metrics at: `http://localhost:9090/metrics`

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

