# Nodes API

The Nodes API provides operations for managing and monitoring GPU nodes, including node details, GPU devices, metrics, and workload information.

## Endpoints

### List GPU Nodes

Retrieves a paginated list of GPU nodes in the cluster with filtering support.

**Endpoint:** `GET /api/nodes`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `pageNum` | integer | No | 1 | Page number |
| `pageSize` | integer | No | 10 | Number of items per page |
| `name` | string | No | - | Filter by node name (partial match) |
| `status` | string | No | - | Filter by node status (Ready, NotReady, etc.) |
| `gpuModel` | string | No | - | Filter by GPU model |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "data": [
      {
        "name": "gpu-node-1",
        "ip": "192.168.1.100",
        "gpuName": "AMD MI300X",
        "gpuCount": 8,
        "gpuAllocation": 6,
        "gpuUtilization": 0.78,
        "status": "Ready",
        "statusColor": "green"
      },
      {
        "name": "gpu-node-2",
        "ip": "192.168.1.101",
        "gpuName": "AMD MI300X",
        "gpuCount": 8,
        "gpuAllocation": 8,
        "gpuUtilization": 0.92,
        "status": "Ready",
        "statusColor": "green"
      }
    ],
    "total": 10
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Node name |
| `ip` | string | Node IP address |
| `gpuName` | string | GPU model name |
| `gpuCount` | integer | Total number of GPUs on the node |
| `gpuAllocation` | integer | Number of GPUs currently allocated |
| `gpuUtilization` | float | Current GPU utilization (0.0 to 1.0) |
| `status` | string | Node status (Ready, NotReady, Unknown) |
| `statusColor` | string | UI color indicator for status |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all nodes with pagination
curl -X GET "http://localhost:8080/api/nodes?pageNum=1&pageSize=20"

# Filter by node name
curl -X GET "http://localhost:8080/api/nodes?name=gpu-node&pageNum=1"

# Filter by status
curl -X GET "http://localhost:8080/api/nodes?status=Ready"
```

---

### Get Node Details

Retrieves detailed information for a specific GPU node.

**Endpoint:** `GET /api/nodes/:name`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Node name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "name": "gpu-node-1",
    "health": "Ready",
    "cpu": "128 X Intel Xeon Platinum 8358",
    "memory": "512GB",
    "os": "Ubuntu 22.04.3 LTS",
    "staticGpuDetails": "8 X AMD MI300X",
    "kubeletVersion": "v1.28.2",
    "containerdVersion": "1.7.2",
    "gpuDriverVersion": "6.0.0"
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Node name |
| `health` | string | Node health status |
| `cpu` | string | CPU information (count x model) |
| `memory` | string | Total memory |
| `os` | string | Operating system |
| `staticGpuDetails` | string | GPU information (count x model) |
| `kubeletVersion` | string | Kubelet version |
| `containerdVersion` | string | Containerd version |
| `gpuDriverVersion` | string | GPU driver version |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Node does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/nodes/gpu-node-1
```

---

### Get GPU Devices

Retrieves GPU device information for a specific node.

**Endpoint:** `GET /api/nodes/:name/gpuDevices`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Node name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "deviceId": 0,
      "model": "AMD MI300X",
      "memory": "192GB",
      "utilization": 0.85,
      "temperature": 78.5,
      "power": 650.2
    },
    {
      "deviceId": 1,
      "model": "AMD MI300X",
      "memory": "192GB",
      "utilization": 0.72,
      "temperature": 75.3,
      "power": 620.8
    }
  ],
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `deviceId` | integer | GPU device ID (0-based index) |
| `model` | string | GPU model name |
| `memory` | string | GPU memory capacity |
| `utilization` | float | Current utilization (0.0 to 1.0) |
| `temperature` | float | Current temperature (°C) |
| `power` | float | Current power consumption (Watts) |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Node does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/nodes/gpu-node-1/gpuDevices
```

---

### Get Node GPU Metrics

Retrieves historical GPU metrics for a specific node.

**Endpoint:** `GET /api/nodes/:name/gpuMetrics`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Node name |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `start` | int64 | Yes | - | Start timestamp (Unix seconds) |
| `end` | int64 | Yes | - | End timestamp (Unix seconds) |
| `step` | integer | No | 60 | Query resolution in seconds |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "gpu_utilization": {
      "series": [
        {
          "name": "GPU-0",
          "points": [
            [0.75, 1609459200000],
            [0.82, 1609459260000],
            [0.78, 1609459320000]
          ]
        },
        {
          "name": "GPU-1",
          "points": [
            [0.65, 1609459200000],
            [0.72, 1609459260000],
            [0.68, 1609459320000]
          ]
        }
      ],
      "config": {
        "yAxisUnit": "%"
      }
    },
    "gpu_allocation_rate": {
      "series": [
        {
          "name": "Allocation",
          "points": [
            [0.875, 1609459200000],
            [0.875, 1609459260000],
            [1.0, 1609459320000]
          ]
        }
      ],
      "config": {
        "yAxisUnit": "%"
      }
    }
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `gpu_utilization` | object | GPU utilization time series |
| `gpu_allocation_rate` | object | GPU allocation rate time series |
| `series` | array | Array of time series data |
| `name` | string | Series name (GPU identifier) |
| `points` | array | Array of [value, timestamp] pairs |
| `config.yAxisUnit` | string | Y-axis unit for visualization |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid timestamp or step value
- `404 Not Found` - Node does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get metrics for the last hour with 5-minute intervals
curl -X GET "http://localhost:8080/api/nodes/gpu-node-1/gpuMetrics?start=1609459200&end=1609462800&step=300"
```

---

### Get Node Workloads

Lists workloads currently running on a specific node.

**Endpoint:** `GET /api/nodes/:name/workloads`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Node name |

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
        "gpuAllocated": 8,
        "gpuAllocatedNode": 4,
        "nodeName": "gpu-node-1",
        "status": "Running"
      },
      {
        "kind": "Deployment",
        "name": "inference-service",
        "namespace": "production",
        "uid": "def456",
        "gpuAllocated": 4,
        "gpuAllocatedNode": 2,
        "nodeName": "gpu-node-1",
        "status": "Running"
      }
    ],
    "total": 3
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Workload kind |
| `name` | string | Workload name |
| `namespace` | string | Kubernetes namespace |
| `uid` | string | Unique identifier |
| `gpuAllocated` | integer | Total GPUs allocated to workload |
| `gpuAllocatedNode` | integer | GPUs allocated on this specific node |
| `nodeName` | string | Node name |
| `status` | string | Workload status |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `404 Not Found` - Node does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET "http://localhost:8080/api/nodes/gpu-node-1/workloads?pageNum=1&pageSize=20"
```

---

### Get Node Workloads History

Retrieves historical workload information for a specific node.

**Endpoint:** `GET /api/nodes/:name/workloadsHistory`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Node name |

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
        "gpuAllocated": 4,
        "podName": "training-job-1-pod-0",
        "podNamespace": "default",
        "startTime": 1609459200,
        "endTime": 1609465800
      },
      {
        "kind": "Job",
        "name": "training-job-2",
        "namespace": "ml-team",
        "uid": "def456",
        "gpuAllocated": 2,
        "podName": "training-job-2-pod-0",
        "podNamespace": "ml-team",
        "startTime": 1609370000,
        "endTime": 1609376400
      }
    ],
    "total": 25
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Workload kind |
| `name` | string | Workload name |
| `namespace` | string | Workload namespace |
| `uid` | string | Workload unique identifier |
| `gpuAllocated` | integer | Number of GPUs allocated to the pod |
| `podName` | string | Pod name |
| `podNamespace` | string | Pod namespace |
| `startTime` | int64 | Pod start time (Unix seconds) |
| `endTime` | int64 | Pod end time (Unix seconds) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `404 Not Found` - Node does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET "http://localhost:8080/api/nodes/gpu-node-1/workloadsHistory?pageNum=1&pageSize=50"
```

---

### Get GPU Allocation Info

Retrieves GPU allocation information for all nodes in the cluster.

**Endpoint:** `GET /api/nodes/gpuAllocation`

**Query Parameters:** None

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "nodeName": "gpu-node-1",
      "totalGpus": 8,
      "allocatedGpus": 6,
      "freeGpus": 2,
      "allocationRate": 0.75
    },
    {
      "nodeName": "gpu-node-2",
      "totalGpus": 8,
      "allocatedGpus": 8,
      "freeGpus": 0,
      "allocationRate": 1.0
    }
  ],
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/nodes/gpuAllocation
```

---

### Get GPU Utilization

Retrieves current GPU utilization and allocation rate for the cluster.

**Endpoint:** `GET /api/nodes/gpuUtilization`

**Query Parameters:** None

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "allocationRate": 0.75,
    "utilization": 0.62
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `allocationRate` | float | GPU allocation rate (0.0 to 1.0) |
| `utilization` | float | GPU utilization rate (0.0 to 1.0) |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/nodes/gpuUtilization
```

---

### Get GPU Utilization History

Retrieves historical GPU utilization and allocation data for the cluster.

**Endpoint:** `GET /api/nodes/gpuUtilizationHistory`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `start` | int64 | Yes | - | Start timestamp (Unix seconds) |
| `end` | int64 | Yes | - | End timestamp (Unix seconds) |
| `step` | integer | No | 60 | Query resolution in seconds |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "allocationRate": [
      [0.75, 1609459200000],
      [0.78, 1609459260000],
      [0.80, 1609459320000]
    ],
    "utilization": [
      [0.62, 1609459200000],
      [0.65, 1609459260000],
      [0.68, 1609459320000]
    ],
    "vramUtilization": [
      [0.55, 1609459200000],
      [0.58, 1609459260000],
      [0.60, 1609459320000]
    ]
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `allocationRate` | array | Array of [value, timestamp] pairs for allocation rate |
| `utilization` | array | Array of [value, timestamp] pairs for GPU utilization |
| `vramUtilization` | array | Array of [value, timestamp] pairs for VRAM utilization |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid timestamp or step value
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get hourly data for the last 24 hours
curl -X GET "http://localhost:8080/api/nodes/gpuUtilizationHistory?start=1609459200&end=1609545600&step=3600"
```

---

## Data Models

### GPUNode

```go
type GPUNode struct {
    Name           string  // Node name
    Ip             string  // Node IP address
    GpuName        string  // GPU model name
    GpuCount       int     // Total GPU count
    GpuAllocation  int     // Allocated GPUs
    GpuUtilization float64 // GPU utilization
    Status         string  // Node status
    StatusColor    string  // Status color for UI
}
```

### GpuNodeDetail

```go
type GpuNodeDetail struct {
    Name              string // Node name
    Health            string // Health status
    Cpu               string // CPU information
    Memory            string // Memory capacity
    OS                string // Operating system
    StaticGpuDetails  string // GPU information
    KubeletVersion    string // Kubelet version
    ContainerdVersion string // Containerd version
    GPUDriverVersion  string // GPU driver version
}
```

### GpuDeviceInfo

```go
type GpuDeviceInfo struct {
    DeviceId    int     // GPU device ID
    Model       string  // GPU model
    Memory      string  // GPU memory
    Utilization float64 // Current utilization
    Temperature float64 // Current temperature
    Power       float64 // Current power consumption
}
```

---

## Notes

- All timestamp parameters use Unix seconds
- All timestamp values in responses use Unix milliseconds
- GPU utilization and allocation rates are returned as floats between 0.0 and 1.0
- Node status values: `Ready`, `NotReady`, `Unknown`, `SchedulingDisabled`
- Status colors: `green` (Ready), `red` (NotReady), `gray` (Unknown), `yellow` (SchedulingDisabled)

---

## Error Handling

Common error responses:

```json
{
  "code": 404,
  "message": "Node 'gpu-node-99' not found",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 400,
  "message": "invalid start timestamp",
  "traceId": "trace-abc123"
}
```

