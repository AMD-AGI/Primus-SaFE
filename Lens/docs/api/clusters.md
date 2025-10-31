# Clusters API

The Clusters API provides cluster-level monitoring and statistics for GPU resources, nodes, storage, and RDMA networks.

## Endpoints

### Get Cluster Overview

Retrieves a comprehensive overview of the cluster including node statistics, GPU allocation and utilization, storage information, and RDMA statistics.

**Endpoint:** `GET /api/clusters/overview`

**Parameters:** None

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "totalNodes": 10,
    "healthyNodes": 9,
    "faultyNodes": 1,
    "fullyIdleNodes": 3,
    "partiallyIdleNodes": 4,
    "busyNodes": 2,
    "allocationRate": 0.75,
    "utilization": 0.62,
    "storageStat": {
      "totalCapacity": "1000TB",
      "usedCapacity": "450TB",
      "availableCapacity": "550TB",
      "utilizationRate": 0.45
    },
    "rdmaClusterStat": {
      "totalPorts": 80,
      "activePorts": 76,
      "bandwidth": "400Gbps"
    }
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `totalNodes` | integer | Total number of GPU nodes in the cluster |
| `healthyNodes` | integer | Number of healthy GPU nodes |
| `faultyNodes` | integer | Number of nodes with faults or issues |
| `fullyIdleNodes` | integer | Number of nodes with no GPU allocation |
| `partiallyIdleNodes` | integer | Number of nodes with partial GPU allocation |
| `busyNodes` | integer | Number of fully allocated nodes |
| `allocationRate` | float | GPU allocation rate (0.0 to 1.0) |
| `utilization` | float | GPU utilization rate (0.0 to 1.0) |
| `storageStat` | object | Storage statistics |
| `rdmaClusterStat` | object | RDMA network statistics |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/clusters/overview
```

---

### Get GPU Consumers

Lists all GPU consumers (workloads) in the cluster with their allocation and utilization statistics. Supports pagination.

**Endpoint:** `GET /api/clusters/consumers`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `pageNum` | integer | No | 1 | Page number |
| `pageSize` | integer | No | 10 | Number of items per page |

**Response:**

```json
{
  "code": 0,
  "message": "success",
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
        },
        "pods": null,
        "source": "k8s"
      },
      {
        "kind": "Deployment",
        "name": "inference-service",
        "namespace": "production",
        "uid": "def456",
        "stat": {
          "gpuRequest": 4,
          "gpuUtilization": 0.62
        },
        "pods": null,
        "source": "k8s"
      }
    ],
    "total": 50
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Workload kind (Job, Deployment, StatefulSet, etc.) |
| `name` | string | Workload name |
| `namespace` | string | Kubernetes namespace |
| `uid` | string | Unique identifier |
| `stat.gpuRequest` | integer | Number of GPUs requested |
| `stat.gpuUtilization` | float | Current GPU utilization (0.0 to 1.0) |
| `source` | string | Workload source (k8s, slurm, etc.) |
| `total` | integer | Total number of consumers |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get first page with 20 items
curl -X GET "http://localhost:8080/api/clusters/consumers?pageNum=1&pageSize=20"

# Get second page with default page size
curl -X GET "http://localhost:8080/api/clusters/consumers?pageNum=2"
```

---

### Get GPU Heatmap

Retrieves GPU heatmap data showing the top K GPUs by power consumption, temperature, and utilization. Useful for identifying hot spots and resource-intensive workloads.

**Endpoint:** `GET /api/clusters/gpuHeatmap`

**Query Parameters:** None (currently returns top 5 GPUs for each metric)

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "power": {
      "serial": 2,
      "unit": "W",
      "yAxisMax": 850,
      "yAxisMin": 0,
      "data": [
        {
          "nodeName": "gpu-node-1",
          "deviceId": 0,
          "value": 750.5,
          "gpuModel": "AMD MI300X"
        },
        {
          "nodeName": "gpu-node-2",
          "deviceId": 1,
          "value": 732.3,
          "gpuModel": "AMD MI300X"
        }
      ]
    },
    "temperature": {
      "serial": 3,
      "unit": "℃",
      "yAxisMax": 110,
      "yAxisMin": 20,
      "data": [
        {
          "nodeName": "gpu-node-1",
          "deviceId": 0,
          "value": 82.5,
          "gpuModel": "AMD MI300X"
        },
        {
          "nodeName": "gpu-node-3",
          "deviceId": 2,
          "value": 79.8,
          "gpuModel": "AMD MI300X"
        }
      ]
    },
    "utilization": {
      "serial": 1,
      "unit": "%",
      "yAxisMax": 100,
      "yAxisMin": 0,
      "data": [
        {
          "nodeName": "gpu-node-2",
          "deviceId": 1,
          "value": 98.5,
          "gpuModel": "AMD MI300X"
        },
        {
          "nodeName": "gpu-node-1",
          "deviceId": 0,
          "value": 95.2,
          "gpuModel": "AMD MI300X"
        }
      ]
    }
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `power` | object | Power consumption heatmap |
| `temperature` | object | Temperature heatmap |
| `utilization` | object | Utilization heatmap |
| `serial` | integer | Display order |
| `unit` | string | Measurement unit |
| `yAxisMax` | integer | Maximum Y-axis value for visualization |
| `yAxisMin` | integer | Minimum Y-axis value for visualization |
| `data` | array | Array of top K GPU data points |
| `nodeName` | string | Node where the GPU is located |
| `deviceId` | integer | GPU device ID on the node |
| `value` | float | Metric value |
| `gpuModel` | string | GPU model name |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/clusters/gpuHeatmap
```

**Use Cases:**

1. **Monitoring Hot Spots**: Identify GPUs with high power consumption or temperature
2. **Load Balancing**: Discover heavily utilized GPUs for workload distribution
3. **Alerting**: Set up alerts when certain GPUs exceed thresholds
4. **Capacity Planning**: Understand resource utilization patterns

---

## Data Models

### GpuClusterOverview

```go
type GpuClusterOverview struct {
    TotalNodes         int              // Total GPU nodes
    HealthyNodes       int              // Healthy nodes
    FaultyNodes        int              // Faulty nodes
    FullyIdleNodes     int              // Fully idle nodes
    PartiallyIdleNodes int              // Partially idle nodes
    BusyNodes          int              // Busy nodes
    AllocationRate     float64          // GPU allocation rate
    Utilization        float64          // GPU utilization
    StorageStat        StorageStat      // Storage statistics
    RdmaClusterStat    RdmaClusterStat  // RDMA statistics
}
```

### TopLevelGpuResource

```go
type TopLevelGpuResource struct {
    Kind      string   // Workload kind
    Name      string   // Workload name
    Namespace string   // Kubernetes namespace
    Uid       string   // Unique identifier
    Stat      GpuStat  // GPU statistics
    Pods      []Pod    // Associated pods
    Source    string   // Workload source
}
```

### Heatmap

```go
type Heatmap struct {
    Serial   int              // Display order
    Unit     string           // Measurement unit
    YAxisMax int              // Max Y-axis value
    YAxisMin int              // Min Y-axis value
    Data     []HeatmapPoint   // Data points
}

type HeatmapPoint struct {
    NodeName string  // Node name
    DeviceId int     // GPU device ID
    Value    float64 // Metric value
    GpuModel string  // GPU model
}
```

---

## Notes

- The GPU allocation rate represents the percentage of GPUs that have been allocated to workloads
- The GPU utilization rate represents the actual usage of allocated GPUs
- Faulty nodes are determined based on hardware errors, driver issues, or node status
- The heatmap currently shows top 5 GPUs per metric; this may become configurable in future versions
- All percentage values are returned as floats between 0.0 and 1.0

---

## Error Handling

If an error occurs, the API returns an appropriate HTTP status code and error message:

```json
{
  "code": 500,
  "message": "Failed to retrieve cluster overview: database connection timeout",
  "traceId": "trace-abc123"
}
```

Use the `traceId` for debugging and log correlation.

