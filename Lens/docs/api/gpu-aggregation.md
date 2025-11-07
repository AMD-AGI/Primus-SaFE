# GPU Aggregation API

The GPU Aggregation API provides operations for querying GPU resource allocation and utilization statistics aggregated across different dimensions (cluster, namespace, label/annotation) and time periods (hourly statistics and real-time snapshots).

## Endpoints

### Metadata Endpoints

#### Get Clusters

Retrieves a list of all available cluster names in the system.

**Endpoint:** `GET /api/gpu-aggregation/clusters`

**Query Parameters:**

None

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    "gpu-cluster-01",
    "gpu-cluster-02",
    "gpu-cluster-03"
  ],
  "traceId": "trace-abc123"
}
```

**Response Fields:**

Returns an array of cluster names (strings).

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get all cluster names
curl -X GET "http://localhost:8080/api/gpu-aggregation/clusters"
```

---

#### Get Namespaces

Retrieves a list of distinct namespaces that have GPU allocation data within the specified time range for a given cluster.

**Endpoint:** `GET /api/gpu-aggregation/namespaces`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Cluster name (uses default if not specified) |
| `start_time` | string | Yes | Start time in RFC3339 format |
| `end_time` | string | Yes | End time in RFC3339 format |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    "ml-training",
    "ml-inference",
    "data-processing",
    "production",
    "development"
  ],
  "traceId": "trace-def456"
}
```

**Response Fields:**

Returns an array of namespace names (strings) that have GPU allocation data in the specified time range.

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (e.g., invalid time format)
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get namespaces for default cluster in the last 7 days
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces?start_time=2025-10-29T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get namespaces for a specific cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces?cluster=gpu-cluster-02&start_time=2025-11-01T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get namespaces for the last 24 hours
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces?start_time=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)&end_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

---

#### Get Dimension Keys

Retrieves a list of distinct label or annotation keys that have GPU allocation data within the specified time range for a given cluster.

**Endpoint:** `GET /api/gpu-aggregation/dimension-keys`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Cluster name (uses default if not specified) |
| `dimension_type` | string | Yes | Dimension type: `label` or `annotation` |
| `start_time` | string | Yes | Start time in RFC3339 format |
| `end_time` | string | Yes | End time in RFC3339 format |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    "team",
    "project",
    "environment",
    "priority",
    "cost-center"
  ],
  "traceId": "trace-ghi789"
}
```

**Response Fields:**

Returns an array of dimension key names (strings) that have GPU allocation data in the specified time range.

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (e.g., invalid time format or dimension_type)
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get all label keys for default cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/dimension-keys?dimension_type=label&start_time=2025-11-01T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get annotation keys for a specific cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/dimension-keys?cluster=gpu-cluster-02&dimension_type=annotation&start_time=2025-10-01T00:00:00Z&end_time=2025-11-01T00:00:00Z"

# Get label keys for the last 7 days
curl -X GET "http://localhost:8080/api/gpu-aggregation/dimension-keys?dimension_type=label&start_time=$(date -u -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ)&end_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

---

### Statistics Endpoints

#### Get Cluster Hourly Statistics

Retrieves hourly aggregated GPU statistics at the cluster level, including allocation rates and utilization metrics.

**Endpoint:** `GET /api/gpu-aggregation/cluster/hourly-stats`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Cluster name (uses default if not specified) |
| `start_time` | string | Yes | Start time in RFC3339 format (e.g., 2025-11-05T00:00:00Z) |
| `end_time` | string | Yes | End time in RFC3339 format (e.g., 2025-11-05T23:59:59Z) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 1,
      "cluster_name": "gpu-cluster-01",
      "stat_hour": "2025-11-05T14:00:00Z",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 96.5,
      "allocation_rate": 0.7539,
      "avg_utilization": 0.6823,
      "max_utilization": 0.9850,
      "min_utilization": 0.1234,
      "p50_utilization": 0.6750,
      "p95_utilization": 0.9200,
      "sample_count": 3600,
      "created_at": "2025-11-05T14:00:00Z",
      "updated_at": "2025-11-05T15:00:00Z"
    },
    {
      "id": 2,
      "cluster_name": "gpu-cluster-01",
      "stat_hour": "2025-11-05T15:00:00Z",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 102.3,
      "allocation_rate": 0.7992,
      "avg_utilization": 0.7156,
      "max_utilization": 0.9920,
      "min_utilization": 0.2100,
      "p50_utilization": 0.7100,
      "p95_utilization": 0.9350,
      "sample_count": 3600,
      "created_at": "2025-11-05T15:00:00Z",
      "updated_at": "2025-11-05T16:00:00Z"
    }
  ],
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique record ID |
| `cluster_name` | string | Name of the cluster |
| `stat_hour` | string | Statistical hour (rounded down to the hour), RFC3339 format |
| `total_gpu_capacity` | integer | Total GPU capacity in the cluster |
| `allocated_gpu_count` | float | Number of allocated GPUs (can be fractional) |
| `allocation_rate` | float | GPU allocation rate (0.0 to 1.0) |
| `avg_utilization` | float | Average GPU utilization during the hour (0.0 to 1.0) |
| `max_utilization` | float | Maximum GPU utilization during the hour (0.0 to 1.0) |
| `min_utilization` | float | Minimum GPU utilization during the hour (0.0 to 1.0) |
| `p50_utilization` | float | Median (50th percentile) GPU utilization (0.0 to 1.0) |
| `p95_utilization` | float | 95th percentile GPU utilization (0.0 to 1.0) |
| `sample_count` | integer | Number of samples collected during this hour |
| `created_at` | string | Record creation time (RFC3339 format) |
| `updated_at` | string | Record last update time (RFC3339 format) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (e.g., invalid time format)
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get cluster hourly stats for a specific time range
curl -X GET "http://localhost:8080/api/gpu-aggregation/cluster/hourly-stats?start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get stats for a specific cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/cluster/hourly-stats?cluster=gpu-cluster-02&start_time=2025-11-01T00:00:00Z&end_time=2025-11-01T23:59:59Z"

# Get stats for the last 24 hours
curl -X GET "http://localhost:8080/api/gpu-aggregation/cluster/hourly-stats?start_time=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)&end_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

---

### Get Namespace Hourly Statistics

Retrieves hourly aggregated GPU statistics at the namespace level, showing resource allocation and utilization per namespace.

**Endpoint:** `GET /api/gpu-aggregation/namespaces/hourly-stats`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Cluster name (uses default if not specified) |
| `namespace` | string | No | Namespace name (returns all namespaces if not specified) |
| `start_time` | string | Yes | Start time in RFC3339 format |
| `end_time` | string | Yes | End time in RFC3339 format |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 1,
      "cluster_name": "gpu-cluster-01",
      "namespace": "ml-training",
      "stat_hour": "2025-11-05T14:00:00Z",
      "total_gpu_capacity": 32,
      "allocated_gpu_count": 28.5,
      "avg_utilization": 0.8234,
      "max_utilization": 0.9650,
      "min_utilization": 0.5100,
      "active_workload_count": 5,
      "created_at": "2025-11-05T14:00:00Z",
      "updated_at": "2025-11-05T15:00:00Z"
    },
    {
      "id": 2,
      "cluster_name": "gpu-cluster-01",
      "namespace": "ml-inference",
      "stat_hour": "2025-11-05T14:00:00Z",
      "total_gpu_capacity": 16,
      "allocated_gpu_count": 14.0,
      "avg_utilization": 0.6723,
      "max_utilization": 0.8900,
      "min_utilization": 0.3200,
      "active_workload_count": 8,
      "created_at": "2025-11-05T14:00:00Z",
      "updated_at": "2025-11-05T15:00:00Z"
    }
  ],
  "traceId": "trace-def456"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique record ID |
| `cluster_name` | string | Name of the cluster |
| `namespace` | string | Kubernetes namespace |
| `stat_hour` | string | Statistical hour (RFC3339 format) |
| `total_gpu_capacity` | integer | Total GPU capacity quota for this namespace (average during the hour) |
| `allocated_gpu_count` | float | Number of allocated GPUs in this namespace |
| `avg_utilization` | float | Average GPU utilization (0.0 to 1.0) |
| `max_utilization` | float | Maximum GPU utilization (0.0 to 1.0) |
| `min_utilization` | float | Minimum GPU utilization (0.0 to 1.0) |
| `active_workload_count` | integer | Number of active workloads during this hour |
| `created_at` | string | Record creation time (RFC3339 format) |
| `updated_at` | string | Record last update time (RFC3339 format) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get stats for all namespaces
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces/hourly-stats?start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get stats for a specific namespace
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces/hourly-stats?namespace=ml-training&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get stats for a specific namespace and cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces/hourly-stats?cluster=gpu-cluster-01&namespace=production&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"
```

---

### Get Label/Annotation Hourly Statistics

Retrieves hourly aggregated GPU statistics grouped by Kubernetes labels or annotations, allowing analysis of resource usage by custom dimensions.

**Endpoint:** `GET /api/gpu-aggregation/labels/hourly-stats`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Cluster name (uses default if not specified) |
| `dimension_type` | string | Yes | Dimension type: `label` or `annotation` |
| `dimension_key` | string | Yes | The label or annotation key to query |
| `dimension_value` | string | No | Specific value to filter (returns all values for the key if not specified) |
| `start_time` | string | Yes | Start time in RFC3339 format |
| `end_time` | string | Yes | End time in RFC3339 format |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 1,
      "cluster_name": "gpu-cluster-01",
      "dimension_type": "label",
      "dimension_key": "team",
      "dimension_value": "research",
      "stat_hour": "2025-11-05T14:00:00Z",
      "allocated_gpu_count": 48.0,
      "avg_utilization": 0.7845,
      "max_utilization": 0.9500,
      "min_utilization": 0.4200,
      "active_workload_count": 12,
      "created_at": "2025-11-05T14:00:00Z",
      "updated_at": "2025-11-05T15:00:00Z"
    },
    {
      "id": 2,
      "cluster_name": "gpu-cluster-01",
      "dimension_type": "label",
      "dimension_key": "team",
      "dimension_value": "engineering",
      "stat_hour": "2025-11-05T14:00:00Z",
      "allocated_gpu_count": 32.0,
      "avg_utilization": 0.6523,
      "max_utilization": 0.8800,
      "min_utilization": 0.3100,
      "active_workload_count": 8,
      "created_at": "2025-11-05T14:00:00Z",
      "updated_at": "2025-11-05T15:00:00Z"
    }
  ],
  "traceId": "trace-ghi789"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique record ID |
| `cluster_name` | string | Name of the cluster |
| `dimension_type` | string | Type of dimension: `label` or `annotation` |
| `dimension_key` | string | Key of the label or annotation |
| `dimension_value` | string | Value of the label or annotation |
| `stat_hour` | string | Statistical hour (RFC3339 format) |
| `allocated_gpu_count` | float | Number of allocated GPUs for this dimension |
| `avg_utilization` | float | Average GPU utilization (0.0 to 1.0) |
| `max_utilization` | float | Maximum GPU utilization (0.0 to 1.0) |
| `min_utilization` | float | Minimum GPU utilization (0.0 to 1.0) |
| `active_workload_count` | integer | Number of active workloads with this label/annotation |
| `created_at` | string | Record creation time (RFC3339 format) |
| `updated_at` | string | Record last update time (RFC3339 format) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (missing required fields or invalid dimension_type)
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get stats for all values of a specific label key
curl -X GET "http://localhost:8080/api/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get stats for a specific label key-value pair
curl -X GET "http://localhost:8080/api/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&dimension_value=research&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get stats for annotations
curl -X GET "http://localhost:8080/api/gpu-aggregation/labels/hourly-stats?dimension_type=annotation&dimension_key=project-id&dimension_value=proj-12345&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"

# Get stats for a specific cluster and label
curl -X GET "http://localhost:8080/api/gpu-aggregation/labels/hourly-stats?cluster=gpu-cluster-02&dimension_type=label&dimension_key=priority&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

---

### Get Latest Snapshot

Retrieves the most recent GPU allocation snapshot, providing a real-time view of current GPU allocation across different dimensions.

**Endpoint:** `GET /api/gpu-aggregation/snapshots/latest`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Cluster name (uses default if not specified) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 12345,
    "cluster_name": "gpu-cluster-01",
    "snapshot_time": "2025-11-05T14:30:25Z",
    "dimension_type": "cluster",
    "dimension_key": "",
    "dimension_value": "",
    "total_gpu_capacity": 128,
    "allocated_gpu_count": 96,
    "allocation_details": {
      "workloads": [
        {
          "namespace": "ml-training",
          "name": "training-job-1",
          "kind": "Job",
          "gpu_count": 8
        },
        {
          "namespace": "ml-inference",
          "name": "inference-service",
          "kind": "Deployment",
          "gpu_count": 4
        }
      ],
      "by_namespace": {
        "ml-training": 64,
        "ml-inference": 32
      }
    },
    "created_at": "2025-11-05T14:30:25Z"
  },
  "traceId": "trace-jkl012"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique snapshot ID |
| `cluster_name` | string | Name of the cluster |
| `snapshot_time` | string | Time when the snapshot was taken (RFC3339 format) |
| `dimension_type` | string | Dimension type: `cluster`, `namespace`, `label`, or `annotation` |
| `dimension_key` | string | Key for label/annotation dimensions (empty for cluster/namespace) |
| `dimension_value` | string | Value for label/annotation/namespace dimensions (empty for cluster) |
| `total_gpu_capacity` | integer | Total GPU capacity |
| `allocated_gpu_count` | integer | Number of currently allocated GPUs |
| `allocation_details` | object | Detailed allocation information in JSON format, including workload details |
| `created_at` | string | Record creation time (RFC3339 format) |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - No snapshot found for the specified cluster
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get latest snapshot for default cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots/latest"

# Get latest snapshot for a specific cluster
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots/latest?cluster=gpu-cluster-02"
```

---

### List Snapshots

Retrieves a list of historical GPU allocation snapshots within a specified time range.

**Endpoint:** `GET /api/gpu-aggregation/snapshots`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | default | Cluster name |
| `start_time` | string | No | 24 hours ago | Start time in RFC3339 format |
| `end_time` | string | No | now | End time in RFC3339 format |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 12345,
      "cluster_name": "gpu-cluster-01",
      "snapshot_time": "2025-11-05T14:30:00Z",
      "dimension_type": "cluster",
      "dimension_key": "",
      "dimension_value": "",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 96,
      "allocation_details": {
        "by_namespace": {
          "ml-training": 64,
          "ml-inference": 32
        }
      },
      "created_at": "2025-11-05T14:30:00Z"
    },
    {
      "id": 12344,
      "cluster_name": "gpu-cluster-01",
      "snapshot_time": "2025-11-05T14:25:00Z",
      "dimension_type": "cluster",
      "dimension_key": "",
      "dimension_value": "",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 94,
      "allocation_details": {
        "by_namespace": {
          "ml-training": 62,
          "ml-inference": 32
        }
      },
      "created_at": "2025-11-05T14:25:00Z"
    }
  ],
  "traceId": "trace-mno345"
}
```

**Response Fields:**

Same as the "Get Latest Snapshot" endpoint, returned as an array.

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid time format
- `500 Internal Server Error` - Database or server error

**Example:**

```bash
# Get snapshots for the last 24 hours (default)
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots"

# Get snapshots for a specific time range
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots?start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Get snapshots for a specific cluster and time range
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots?cluster=gpu-cluster-02&start_time=2025-11-04T00:00:00Z&end_time=2025-11-05T00:00:00Z"

# Get snapshots for the last hour
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots?start_time=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)&end_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

---

## Common Error Response Format

All endpoints follow a consistent error response format:

```json
{
  "code": 400,
  "message": "Invalid start_time format, use RFC3339 format",
  "data": null,
  "traceId": "trace-error-123"
}
```

## Time Format

All time-related query parameters and response fields use **RFC3339 format** (e.g., `2025-11-05T14:30:00Z`). This is an ISO 8601 compatible format that includes timezone information.

Examples:
- `2025-11-05T00:00:00Z` - UTC midnight
- `2025-11-05T14:30:25Z` - 2:30:25 PM UTC
- `2025-11-05T14:30:25+08:00` - With timezone offset

## Data Models

### ClusterGpuHourlyStats

Represents hourly aggregated GPU statistics at the cluster level.

```go
type ClusterGpuHourlyStats struct {
    ID                int32     // Unique record ID
    ClusterName       string    // Cluster name
    StatHour          time.Time // Statistical hour (rounded down)
    TotalGpuCapacity  int32     // Total GPU capacity
    AllocatedGpuCount float64   // Allocated GPU count
    AllocationRate    float64   // Allocation rate (0.0-1.0)
    AvgUtilization    float64   // Average utilization
    MaxUtilization    float64   // Maximum utilization
    MinUtilization    float64   // Minimum utilization
    P50Utilization    float64   // Median utilization
    P95Utilization    float64   // 95th percentile utilization
    SampleCount       int32     // Number of samples
    CreatedAt         time.Time // Record creation time
    UpdatedAt         time.Time // Record update time
}
```

### NamespaceGpuHourlyStats

Represents hourly aggregated GPU statistics at the namespace level.

```go
type NamespaceGpuHourlyStats struct {
    ID                  int32     // Unique record ID
    ClusterName         string    // Cluster name
    Namespace           string    // Kubernetes namespace
    StatHour            time.Time // Statistical hour
    TotalGpuCapacity    int32     // Total GPU capacity quota
    AllocatedGpuCount   float64   // Allocated GPU count
    AvgUtilization      float64   // Average utilization
    MaxUtilization      float64   // Maximum utilization
    MinUtilization      float64   // Minimum utilization
    ActiveWorkloadCount int32     // Number of active workloads
    CreatedAt           time.Time // Record creation time
    UpdatedAt           time.Time // Record update time
}
```

### LabelGpuHourlyStats

Represents hourly aggregated GPU statistics grouped by labels or annotations.

```go
type LabelGpuHourlyStats struct {
    ID                  int32     // Unique record ID
    ClusterName         string    // Cluster name
    DimensionType       string    // "label" or "annotation"
    DimensionKey        string    // Label/annotation key
    DimensionValue      string    // Label/annotation value
    StatHour            time.Time // Statistical hour
    AllocatedGpuCount   float64   // Allocated GPU count
    AvgUtilization      float64   // Average utilization
    MaxUtilization      float64   // Maximum utilization
    MinUtilization      float64   // Minimum utilization
    ActiveWorkloadCount int32     // Number of active workloads
    CreatedAt           time.Time // Record creation time
    UpdatedAt           time.Time // Record update time
}
```

### GpuAllocationSnapshots

Represents a point-in-time snapshot of GPU allocation.

```go
type GpuAllocationSnapshots struct {
    ID                int32     // Unique snapshot ID
    ClusterName       string    // Cluster name
    SnapshotTime      time.Time // Snapshot timestamp
    DimensionType     string    // cluster/namespace/label/annotation
    DimensionKey      string    // Key for label/annotation
    DimensionValue    string    // Value for dimension
    TotalGpuCapacity  int32     // Total GPU capacity
    AllocatedGpuCount int32     // Allocated GPU count
    AllocationDetails ExtType   // JSON with detailed allocation info
    CreatedAt         time.Time // Record creation time
}
```

---

## Use Cases

### 1. Discovery and Exploration

Query available clusters, namespaces, and label/annotation keys to understand the data landscape:

```bash
# First, discover all available clusters
curl -X GET "http://localhost:8080/api/gpu-aggregation/clusters"

# Then, get all namespaces in a specific cluster for the last 30 days
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces?cluster=gpu-cluster-01&start_time=2025-10-06T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# Finally, discover what labels are being used
curl -X GET "http://localhost:8080/api/gpu-aggregation/dimension-keys?dimension_type=label&start_time=2025-10-06T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# After discovering available labels, query specific label values
curl -X GET "http://localhost:8080/api/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&start_time=2025-11-01T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

### 2. Cost Allocation and Chargeback

Query namespace-level hourly statistics to calculate GPU usage costs per team or project:

```bash
curl -X GET "http://localhost:8080/api/gpu-aggregation/namespaces/hourly-stats?namespace=ml-training&start_time=2025-11-01T00:00:00Z&end_time=2025-11-30T23:59:59Z"
```

### 2. Capacity Planning

Analyze cluster-level utilization trends to plan for capacity expansion:

```bash
curl -X GET "http://localhost:8080/api/gpu-aggregation/cluster/hourly-stats?start_time=2025-10-01T00:00:00Z&end_time=2025-11-01T00:00:00Z"
```

### 3. Team Resource Usage Tracking

Monitor GPU consumption by team using labels:

```bash
curl -X GET "http://localhost:8080/api/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"
```

### 4. Real-time Monitoring

Get current GPU allocation status:

```bash
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots/latest"
```

### 5. Historical Analysis

Compare allocation patterns over time:

```bash
curl -X GET "http://localhost:8080/api/gpu-aggregation/snapshots?start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"
```

---

## Notes

1. **Hourly Aggregation**: Hourly statistics are computed and stored hourly. The `stat_hour` field is always rounded down to the hour (e.g., `2025-11-05T14:00:00Z`).

2. **Utilization Metrics**: 
   - All utilization values are represented as floats between 0.0 and 1.0
   - `avg_utilization` is the mean utilization during the hour
   - `p50_utilization` (median) and `p95_utilization` provide percentile-based insights
   - These metrics help identify both typical usage and peak utilization patterns

3. **Fractional GPU Allocation**: 
   - `allocated_gpu_count` can be fractional (e.g., 96.5) to support:
     - GPU sharing mechanisms (e.g., MIG, time-slicing)
     - Average allocation over time periods
     - Partial GPU allocation schemes

4. **Allocation Rate vs Utilization**:
   - **Allocation Rate**: Percentage of GPUs that have been allocated/requested by workloads
   - **Utilization**: Actual usage of allocated GPUs based on metrics (e.g., GPU compute usage)
   - A high allocation rate with low utilization may indicate resource waste

5. **Dimension Types**:
   - `cluster`: Cluster-wide aggregation
   - `namespace`: Per-namespace aggregation
   - `label`: Grouped by Kubernetes labels
   - `annotation`: Grouped by Kubernetes annotations

6. **Snapshots vs Hourly Stats**:
   - **Snapshots**: Point-in-time allocation data, typically collected every few minutes
   - **Hourly Stats**: Aggregated statistics computed from metrics over each hour
   - Use snapshots for real-time monitoring and recent history
   - Use hourly stats for trend analysis and historical reporting

7. **Sample Count**: The `sample_count` field in cluster hourly stats indicates how many data points were collected from Prometheus during that hour. A low sample count may indicate data collection issues.

8. **Default Time Range**: When querying snapshots without time parameters, the API defaults to the last 24 hours.

9. **Cluster Selection**: If the `cluster` parameter is not specified, the system uses the default cluster configured in the cluster manager.

10. **Metadata Endpoints**: The metadata endpoints (`/clusters`, `/namespaces`, `/dimension-keys`) help you discover available resources before querying detailed statistics. These endpoints are particularly useful for:
    - Building dynamic user interfaces with auto-populated filters
    - Validating cluster, namespace, or label existence before queries
    - Understanding the data structure and available dimensions
    - The namespace and dimension-keys endpoints require time ranges because they query from historical data to show what has been active during that period

---

## Error Handling

Common error scenarios and their responses:

### Invalid Time Format

```json
{
  "code": 400,
  "message": "Invalid start_time format",
  "traceId": "trace-error-001"
}
```

### Invalid Dimension Type

```json
{
  "code": 400,
  "message": "Invalid request parameters",
  "traceId": "trace-error-002"
}
```

### Snapshot Not Found

```json
{
  "code": 404,
  "message": "No snapshot found",
  "traceId": "trace-error-003"
}
```

### Database Error

```json
{
  "code": 500,
  "message": "Failed to get cluster hourly stats",
  "traceId": "trace-error-004"
}
```

Use the `traceId` field for log correlation and debugging. Check server logs with the trace ID to get detailed error information.

---

## Best Practices

1. **Metadata Discovery**:
   - Use the metadata endpoints (`/clusters`, `/namespaces`, `/dimension-keys`) before querying statistics
   - Cache cluster and namespace lists as they change infrequently
   - Refresh dimension keys periodically to discover new labels/annotations
   - Build dynamic UI dropdowns based on metadata endpoints for better user experience

2. **Time Range Selection**: 
   - For hourly stats, query full hour intervals for consistent results
   - Avoid querying very large time ranges (>30 days) in a single request
   - Consider pagination or chunking for long-term historical analysis

3. **Caching**: 
   - Hourly stats are immutable once the hour has passed
   - Consider caching completed hourly data on the client side
   - Only refresh real-time data (current hour and snapshots) frequently

4. **Label/Annotation Queries**:
   - Start with querying all values for a key to understand the data
   - Filter by specific values for detailed analysis
   - Ensure labels/annotations are consistently applied to workloads

5. **Monitoring Integration**:
   - Integrate snapshot data into dashboards for real-time monitoring
   - Set up alerts based on allocation_rate thresholds
   - Use utilization percentiles (p95) for capacity planning alerts

6. **Performance**:
   - Hourly stats queries are generally faster than snapshot queries
   - Use appropriate time ranges to limit result set size
   - Consider implementing client-side pagination for large result sets

