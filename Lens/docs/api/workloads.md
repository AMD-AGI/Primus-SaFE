# Workloads API

The Workloads API provides operations for managing and monitoring GPU workloads including listing, details, hierarchy, metrics, and training performance data.

## Endpoints

### List Workloads

Retrieves a paginated list of workloads with filtering and sorting support.

**Endpoint:** `GET /api/workloads`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `pageNum` | integer | No | 1 | Page number |
| `pageSize` | integer | No | 10 | Number of items per page |
| `name` | string | No | - | Filter by workload name (partial match) |
| `kind` | string | No | - | Filter by workload kind (Job, Deployment, StatefulSet, etc.) |
| `namespace` | string | No | - | Filter by namespace |
| `status` | string | No | - | Filter by status (Running, Completed, Failed, etc.) |
| `orderBy` | string | No | - | Sort field (start_at, end_at) |

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
        "gpuAllocation": {
          "AMD_Instinct_MI300X_OAM": 8
        },
        "status": "Running",
        "statusColor": "green",
        "startAt": 1609459200,
        "endAt": 0,
        "source": "k8s"
      },
      {
        "kind": "Deployment",
        "name": "inference-service",
        "namespace": "production",
        "uid": "def456",
        "gpuAllocated": 4,
        "gpuAllocation": {
          "AMD_Instinct_MI300X_OAM": 4
        },
        "status": "Running",
        "statusColor": "green",
        "startAt": 1609370000,
        "endAt": 0,
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
| `kind` | string | Workload kind (Job, Deployment, StatefulSet, DaemonSet, etc.) |
| `name` | string | Workload name |
| `namespace` | string | Kubernetes namespace |
| `uid` | string | Unique identifier |
| `gpuAllocated` | integer | Total number of GPUs allocated |
| `gpuAllocation` | object | GPU allocation details by model |
| `status` | string | Workload status |
| `statusColor` | string | UI color indicator for status |
| `startAt` | int64 | Start time (Unix seconds) |
| `endAt` | int64 | End time (Unix seconds, 0 if still running) |
| `source` | string | Workload source (k8s, slurm, docker, etc.) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all workloads with pagination
curl -X GET "http://localhost:8080/api/workloads?pageNum=1&pageSize=20"

# Filter by namespace and status
curl -X GET "http://localhost:8080/api/workloads?namespace=default&status=Running"

# Filter by kind and sort by start time
curl -X GET "http://localhost:8080/api/workloads?kind=Job&orderBy=start_at"

# Search by name
curl -X GET "http://localhost:8080/api/workloads?name=training"
```

---

### Get Workload Details

Retrieves detailed information for a specific workload.

**Endpoint:** `GET /api/workloads/:uid`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `uid` | string | Yes | Workload unique identifier |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "apiVersion": "batch/v1",
    "kind": "Job",
    "name": "training-job-1",
    "namespace": "default",
    "uid": "abc123",
    "gpuAllocation": {
      "AMD_Instinct_MI300X_OAM": 8
    },
    "pods": [
      {
        "nodeName": "gpu-node-1",
        "podNamespace": "default",
        "podName": "training-job-1-pod-0"
      },
      {
        "nodeName": "gpu-node-2",
        "podNamespace": "default",
        "podName": "training-job-1-pod-1"
      }
    ],
    "startTime": 1609459200,
    "endTime": 0,
    "source": "k8s"
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `apiVersion` | string | Kubernetes API version |
| `kind` | string | Workload kind |
| `name` | string | Workload name |
| `namespace` | string | Kubernetes namespace |
| `uid` | string | Unique identifier |
| `gpuAllocation` | object | GPU allocation by model |
| `pods` | array | List of pods belonging to this workload |
| `startTime` | int64 | Start time (Unix seconds) |
| `endTime` | int64 | End time (Unix seconds, 0 if still running) |
| `source` | string | Workload source |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Workload does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/workloads/abc123
```

---

### Get Workload Hierarchy

Retrieves the hierarchical structure of a workload, showing parent-child relationships (e.g., Deployment → ReplicaSet → Pod).

**Endpoint:** `GET /api/workloads/:uid/hierarchy`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `uid` | string | Yes | Workload unique identifier |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "kind": "Deployment",
    "name": "inference-service",
    "namespace": "production",
    "uid": "root-uid-123",
    "children": [
      {
        "kind": "ReplicaSet",
        "name": "inference-service-7d8f9c",
        "namespace": "production",
        "uid": "rs-uid-456",
        "children": [
          {
            "kind": "Pod",
            "name": "inference-service-7d8f9c-abc",
            "namespace": "production",
            "uid": "pod-uid-789",
            "children": []
          },
          {
            "kind": "Pod",
            "name": "inference-service-7d8f9c-def",
            "namespace": "production",
            "uid": "pod-uid-101",
            "children": []
          }
        ]
      }
    ]
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
| `children` | array | Array of child workloads (recursive structure) |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Workload does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/workloads/abc123/hierarchy
```

**Use Cases:**
- Visualize workload relationships in a tree structure
- Understand resource ownership and dependencies
- Debug workload issues by examining parent-child relationships

---

### Get Workload Metrics

Retrieves GPU metrics for a specific workload over a time range.

**Endpoint:** `GET /api/workloads/:uid/metrics`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `uid` | string | Yes | Workload unique identifier |

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
    "GPU Utilization": {
      "serial": 1,
      "series": [
        {
          "name": "GPU-0",
          "points": [
            [0.85, 1609459200000],
            [0.87, 1609459260000],
            [0.83, 1609459320000]
          ]
        },
        {
          "name": "GPU-1",
          "points": [
            [0.78, 1609459200000],
            [0.82, 1609459260000],
            [0.80, 1609459320000]
          ]
        }
      ],
      "config": {
        "yAxisUnit": "%"
      }
    },
    "GPU Memory Utilization": {
      "serial": 2,
      "series": [
        {
          "name": "GPU-0",
          "points": [
            [0.92, 1609459200000],
            [0.93, 1609459260000],
            [0.91, 1609459320000]
          ]
        }
      ],
      "config": {
        "yAxisUnit": "%"
      }
    },
    "GPU Power": {
      "serial": 3,
      "series": [
        {
          "name": "GPU-0",
          "points": [
            [650.5, 1609459200000],
            [655.2, 1609459260000],
            [648.8, 1609459320000]
          ]
        }
      ],
      "config": {
        "yAxisUnit": "W"
      }
    },
    "TrainingPerformance": {
      "serial": 4,
      "series": [
        {
          "name": "TFLOPS",
          "points": [
            [450.5, 1609459200000],
            [455.2, 1609459260000],
            [448.8, 1609459320000]
          ]
        }
      ],
      "config": {
        "yAxisUnit": "TFLOPS"
      }
    }
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

Each metric (GPU Utilization, GPU Memory Utilization, GPU Power, TrainingPerformance) contains:

| Field | Type | Description |
|-------|------|-------------|
| `serial` | integer | Display order |
| `series` | array | Array of time series data |
| `name` | string | Series name (GPU identifier) |
| `points` | array | Array of [value, timestamp] pairs |
| `config.yAxisUnit` | string | Y-axis unit for visualization |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid timestamp or step value
- `404 Not Found` - Workload does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get metrics for the last hour with 5-minute intervals
curl -X GET "http://localhost:8080/api/workloads/abc123/metrics?start=1609459200&end=1609462800&step=300"
```

---

### Get Training Performance

Retrieves training performance metrics for AI/ML workloads in a format optimized for Grafana.

**Endpoint:** `GET /api/workloads/:uid/trainingPerformance`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `uid` | string | Yes | Workload unique identifier |

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `start` | int64 | Yes | Start timestamp (Unix milliseconds) |
| `end` | int64 | Yes | End timestamp (Unix milliseconds) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "metric": "samples_per_second",
      "value": 1250.5,
      "timestamp": 1609459200000
    },
    {
      "metric": "samples_per_second",
      "value": 1255.2,
      "timestamp": 1609459260000
    },
    {
      "metric": "loss",
      "value": 0.0125,
      "timestamp": 1609459200000
    },
    {
      "metric": "loss",
      "value": 0.0118,
      "timestamp": 1609459260000
    },
    {
      "metric": "accuracy",
      "value": 0.95,
      "timestamp": 1609459200000
    },
    {
      "metric": "accuracy",
      "value": 0.96,
      "timestamp": 1609459260000
    }
  ]
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `metric` | string | Metric name (e.g., samples_per_second, loss, accuracy) |
| `value` | float | Metric value |
| `timestamp` | int64 | Timestamp (Unix milliseconds) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `404 Not Found` - Workload does not exist
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get training performance for the last hour
START=$(date -d '1 hour ago' +%s)000
END=$(date +%s)000
curl -X GET "http://localhost:8080/api/workloads/abc123/trainingPerformance?start=$START&end=$END"
```

**Use Cases:**
- Monitor training job progress in real-time
- Visualize training metrics in Grafana dashboards
- Analyze training performance and efficiency
- Debug training issues

---

### Get Workload Metadata

Retrieves metadata for filtering workloads, including available namespaces and workload kinds.

**Endpoint:** `GET /api/workloadMetadata`

**Query Parameters:** None

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "namespaces": [
      "default",
      "production",
      "ml-team",
      "research"
    ],
    "kinds": [
      "Job",
      "Deployment",
      "StatefulSet",
      "DaemonSet",
      "ReplicaSet"
    ]
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `namespaces` | array | List of namespaces containing workloads |
| `kinds` | array | List of workload kinds in the system |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/workloadMetadata
```

**Use Cases:**
- Populate filter dropdowns in UI
- Validate filter parameters before querying
- Discover available workload types

---

## Data Models

### WorkloadListItem

```go
type WorkloadListItem struct {
    Kind          string             // Workload kind
    Name          string             // Workload name
    Namespace     string             // Kubernetes namespace
    Uid           string             // Unique identifier
    GpuAllocated  int                // Total GPUs allocated
    GpuAllocation map[string]int     // GPU allocation by model
    Status        string             // Workload status
    StatusColor   string             // Status color for UI
    StartAt       int64              // Start time (Unix seconds)
    EndAt         int64              // End time (Unix seconds)
    Source        string             // Workload source
}
```

### WorkloadInfo

```go
type WorkloadInfo struct {
    ApiVersion    string             // Kubernetes API version
    Kind          string             // Workload kind
    Name          string             // Workload name
    Namespace     string             // Kubernetes namespace
    Uid           string             // Unique identifier
    GpuAllocation map[string]int     // GPU allocation by model
    Pods          []WorkloadInfoPod  // Associated pods
    StartTime     int64              // Start time (Unix seconds)
    EndTime       int64              // End time (Unix seconds)
    Source        string             // Workload source
}
```

### WorkloadHierarchyItem

```go
type WorkloadHierarchyItem struct {
    Kind      string                   // Workload kind
    Name      string                   // Workload name
    Namespace string                   // Kubernetes namespace
    Uid       string                   // Unique identifier
    Children  []WorkloadHierarchyItem  // Child workloads (recursive)
}
```

### MetricsGraph

```go
type MetricsGraph struct {
    Serial int                 // Display order
    Series []MetricsSeries     // Time series data
    Config MetricsGraphConfig  // Configuration
}

type MetricsSeries struct {
    Name   string        // Series name
    Points [][2]float64  // Array of [value, timestamp] pairs
}

type MetricsGraphConfig struct {
    YAxisUnit string  // Y-axis unit
}
```

---

## Workload Status Values

| Status | Description |
|--------|-------------|
| `Running` | Workload is currently running |
| `Completed` | Workload completed successfully |
| `Failed` | Workload failed |
| `Pending` | Workload is pending (waiting for resources) |
| `Unknown` | Workload status is unknown |

**Status Colors:**
- `green` - Running, Completed
- `red` - Failed
- `yellow` - Pending
- `gray` - Unknown

---

## Notes

- The `uid` parameter is the Kubernetes UID of the workload
- Training performance data is only available for AI/ML workloads that report metrics
- Workload hierarchy reflects Kubernetes owner references
- All timestamp parameters in metrics endpoints use Unix seconds
- All timestamp values in responses use Unix milliseconds
- GPU allocation is reported per GPU model to support heterogeneous clusters
- Source field indicates where the workload originated (k8s, slurm, docker, etc.)

---

## Error Handling

Common error responses:

```json
{
  "code": 404,
  "message": "Workload with UID 'invalid-uid' not found",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 400,
  "message": "invalid start time format",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 400,
  "message": "workloadUid is required",
  "traceId": "trace-abc123"
}
```

