# Py-Spy Profiling API

The Py-Spy API provides endpoints for profiling Python processes running in Kubernetes pods using [py-spy](https://github.com/benfred/py-spy), a sampling profiler for Python programs.

## Overview

The Py-Spy profiling feature allows you to:
- Create sampling profiles for Python processes in pods
- Generate flamegraphs (SVG) or speedscope (JSON) format profiles
- Store profiles in database for later retrieval
- Download profiling results through the API

## Architecture

```
┌──────────┐     ┌──────────────┐     ┌───────────────┐     ┌───────────────┐
│   API    │────▶│  Database    │◀────│  Jobs Module  │────▶│ Node-Exporter │
│ (Create) │     │ (Task State) │     │  (Dispatch)   │     │  (Execute)    │
└──────────┘     └──────────────┘     └───────────────┘     └───────────────┘
     │                  │                    │                      │
     │                  │                    │                      ▼
     │                  │                    │               ┌─────────────┐
     │                  ▼                    │               │   py-spy    │
     │           ┌──────────────┐            │               └─────────────┘
     └──────────▶│  Database    │◀───────────┘
      (Query)    │ (File Store) │
                 └──────────────┘
```

## Endpoints

### Create Profiling Task

Creates a new py-spy sampling task for a Python process.

**Endpoint:** `POST /v1/pyspy/sample`

**Request Body:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name (for multi-cluster deployments) |
| `pod_uid` | string | Yes | - | Target pod UID |
| `pod_name` | string | No | - | Pod name (for display) |
| `pod_namespace` | string | No | - | Pod namespace |
| `node_name` | string | Yes | - | Node where the pod is running |
| `pid` | integer | Yes | - | Host PID of the Python process |
| `duration` | integer | No | 30 | Sampling duration in seconds |
| `rate` | integer | No | 100 | Sampling rate in Hz |
| `format` | string | No | flamegraph | Output format: `flamegraph`, `speedscope`, or `raw` |
| `native` | boolean | No | false | Include native stack frames |
| `subprocesses` | boolean | No | false | Profile subprocesses |

**Request Example:**

```json
{
  "pod_uid": "bbd7ae19-2cb2-4826-89d5-50104d408126",
  "pod_name": "training-job-master-0",
  "pod_namespace": "default",
  "node_name": "gpu-node-1",
  "pid": 12345,
  "duration": 10,
  "rate": 100,
  "format": "flamegraph",
  "cluster": "production"
}
```

**Response:**

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "task_id": "pyspy-9657adaf",
    "status": "pending",
    "pod_uid": "bbd7ae19-2cb2-4826-89d5-50104d408126",
    "pod_name": "training-job-master-0",
    "pod_namespace": "default",
    "node_name": "gpu-node-1",
    "pid": 12345,
    "duration": 10,
    "format": "flamegraph",
    "created_at": "2026-01-07T13:54:04.493505Z"
  },
  "tracing": null
}
```

**Status Codes:**
- `201 Created` - Task created successfully
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

---

### Get Task Status

Retrieves the status and details of a specific profiling task.

**Endpoint:** `GET /v1/pyspy/task/:id`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Task ID |

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Target cluster name |

**Response:**

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "task_id": "pyspy-9657adaf",
    "status": "completed",
    "pod_uid": "bbd7ae19-2cb2-4826-89d5-50104d408126",
    "pod_name": "training-job-master-0",
    "pod_namespace": "default",
    "node_name": "gpu-node-1",
    "pid": 12345,
    "duration": 10,
    "format": "flamegraph",
    "output_file": "/var/lib/lens/pyspy/profiles/pyspy-9657adaf/profile.svg",
    "file_size": 21210,
    "created_at": "2026-01-07T13:54:04.493505Z",
    "started_at": "2026-01-07T13:54:09Z",
    "completed_at": "2026-01-07T13:54:19Z",
    "file_path": "/api/v1/pyspy/file/pyspy-9657adaf/profile.svg"
  },
  "tracing": null
}
```

**Task Status Values:**

| Status | Description |
|--------|-------------|
| `pending` | Task created, waiting for execution |
| `running` | Task is being executed |
| `completed` | Task completed successfully |
| `failed` | Task failed (see `error` field) |
| `cancelled` | Task was cancelled |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Task not found
- `500 Internal Server Error` - Server error

---

### List Tasks

Lists profiling tasks with filtering support.

**Endpoint:** `POST /v1/pyspy/tasks`

**Request Body:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |
| `pod_uid` | string | No | - | Filter by pod UID |
| `pod_namespace` | string | No | - | Filter by namespace |
| `node_name` | string | No | - | Filter by node name |
| `status` | string | No | - | Filter by status |
| `limit` | integer | No | 50 | Maximum number of results (max: 100) |
| `offset` | integer | No | 0 | Pagination offset |

**Request Example:**

```json
{
  "pod_uid": "bbd7ae19-2cb2-4826-89d5-50104d408126",
  "cluster": "production",
  "limit": 10
}
```

**Response:**

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "tasks": [
      {
        "task_id": "pyspy-9657adaf",
        "status": "completed",
        "pod_uid": "bbd7ae19-2cb2-4826-89d5-50104d408126",
        "pod_name": "training-job-master-0",
        "pod_namespace": "default",
        "node_name": "gpu-node-1",
        "pid": 12345,
        "duration": 10,
        "format": "flamegraph",
        "output_file": "/var/lib/lens/pyspy/profiles/pyspy-9657adaf/profile.svg",
        "file_size": 21210,
        "created_at": "2026-01-07T13:54:04.493505Z",
        "started_at": "2026-01-07T13:54:09Z",
        "completed_at": "2026-01-07T13:54:19Z",
        "file_path": "/api/v1/pyspy/file/pyspy-9657adaf/profile.svg"
      }
    ],
    "total": 1,
    "limit": 10,
    "offset": 0
  },
  "tracing": null
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

---

### Cancel Task

Cancels a pending or running profiling task.

**Endpoint:** `POST /v1/pyspy/task/:id/cancel`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Task ID |

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Target cluster name |

**Request Body:**

```json
{
  "reason": "User requested cancellation"
}
```

**Response:**

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "task_id": "pyspy-9657adaf",
    "status": "cancelled",
    "message": "Task cancelled successfully"
  },
  "tracing": null
}
```

**Status Codes:**
- `200 OK` - Task cancelled successfully
- `400 Bad Request` - Task cannot be cancelled (already completed/failed)
- `404 Not Found` - Task not found
- `500 Internal Server Error` - Server error

---

### Get File Information

Retrieves file metadata for a completed profiling task.

**Endpoint:** `GET /v1/pyspy/file/:task_id`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `task_id` | string | Task ID |

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Target cluster name |

**Response:**

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "task_id": "pyspy-9657adaf",
    "files": [
      {
        "task_id": "pyspy-9657adaf",
        "file_name": "profile.svg",
        "file_size": 21210,
        "format": "flamegraph",
        "storage_type": "database",
        "storage_path": "444",
        "download_url": "/v1/pyspy/file/pyspy-9657adaf/profile.svg?cluster=production"
      }
    ]
  },
  "tracing": null
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Task or file not found
- `500 Internal Server Error` - Server error

---

### Download Profile File

Downloads the profiling output file.

**Endpoint:** `GET /v1/pyspy/file/:task_id/:filename`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `task_id` | string | Task ID |
| `filename` | string | File name (e.g., `profile.svg`) |

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster` | string | No | Target cluster name |

**Response:**

Returns the file content with appropriate content-type header:
- `image/svg+xml` for flamegraph format
- `application/json` for speedscope format
- `text/plain` for raw format

**Status Codes:**
- `200 OK` - File content returned
- `404 Not Found` - Task or file not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Download flamegraph SVG
curl -o profile.svg "http://localhost:8989/v1/pyspy/file/pyspy-9657adaf/profile.svg?cluster=production"

# Download speedscope JSON
curl -o profile.json "http://localhost:8989/v1/pyspy/file/pyspy-4d46eb3b/profile.json?cluster=production"
```

---

## Output Formats

### Flamegraph (SVG)

Interactive SVG flamegraph visualization. Can be opened directly in a web browser.

**Features:**
- Interactive zoom and pan
- Hover to see function details
- Search functionality
- Color-coded by function type

### Speedscope (JSON)

JSON format compatible with [Speedscope](https://www.speedscope.app/).

**Usage:**
1. Download the profile.json file
2. Open https://www.speedscope.app/
3. Drag and drop the file to visualize

### Raw

Plain text format with stack traces.

---

## Multi-Cluster Support

The Py-Spy API supports multi-cluster deployments. Use the `cluster` parameter to specify which cluster to target:

```bash
# Create task in specific cluster
curl -X POST "http://api-server:8989/v1/pyspy/sample" \
  -H "Content-Type: application/json" \
  -d '{
    "cluster": "production",
    "pod_uid": "...",
    "node_name": "...",
    "pid": 12345
  }'

# Query tasks from specific cluster
curl "http://api-server:8989/v1/pyspy/task/pyspy-xxx?cluster=production"
```

---

## File Storage

Profiling results are stored in the database by default:
- Files are chunked and compressed for efficient storage
- Maximum file size: 50MB
- Files are automatically cleaned up based on retention policy

Storage types:
- `database` - Stored in PostgreSQL (profiler_files table)
- `object_storage` - Stored in S3/MinIO (if configured)

---

## Error Handling

### Common Errors

| Error | Description | Solution |
|-------|-------------|----------|
| `pod_uid is required` | Missing pod UID | Provide the pod UID |
| `node_name is required` | Missing node name | Provide the node where pod runs |
| `pid is required` | Missing process ID | Provide the Python process host PID |
| `task not found` | Invalid task ID | Check the task ID |
| `file not found` | Task has no output file | Task may have failed |
| `process not found` | PID doesn't exist | Verify the process is still running |
| `permission denied` | CAP_SYS_PTRACE not available | Container needs ptrace capability |

### Failed Task Example

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "task_id": "pyspy-failed123",
    "status": "failed",
    "error": "process 12345 not found",
    "created_at": "2026-01-07T10:00:00Z",
    "started_at": "2026-01-07T10:00:05Z"
  },
  "tracing": null
}
```

---

## Examples

### Complete Workflow

```bash
# 1. Create a profiling task
RESPONSE=$(curl -s -X POST "http://api-server:8989/v1/pyspy/sample" \
  -H "Content-Type: application/json" \
  -d '{
    "pod_uid": "bbd7ae19-2cb2-4826-89d5-50104d408126",
    "pod_name": "training-job-master-0",
    "pod_namespace": "default",
    "node_name": "gpu-node-1",
    "pid": 12345,
    "duration": 10,
    "format": "flamegraph",
    "cluster": "production"
  }')

TASK_ID=$(echo $RESPONSE | jq -r '.data.task_id')
echo "Created task: $TASK_ID"

# 2. Wait for task completion
sleep 15

# 3. Check task status
curl -s "http://api-server:8989/v1/pyspy/task/${TASK_ID}?cluster=production" | jq

# 4. Get file information
curl -s "http://api-server:8989/v1/pyspy/file/${TASK_ID}?cluster=production" | jq

# 5. Download the profile
curl -o profile.svg "http://api-server:8989/v1/pyspy/file/${TASK_ID}/profile.svg?cluster=production"

# 6. Open in browser
open profile.svg
```

### Profile Multiple Formats

```bash
# Flamegraph (SVG)
curl -X POST "http://api-server:8989/v1/pyspy/sample" \
  -H "Content-Type: application/json" \
  -d '{"pod_uid": "...", "node_name": "...", "pid": 12345, "format": "flamegraph"}'

# Speedscope (JSON)
curl -X POST "http://api-server:8989/v1/pyspy/sample" \
  -H "Content-Type: application/json" \
  -d '{"pod_uid": "...", "node_name": "...", "pid": 12345, "format": "speedscope"}'

# Raw text
curl -X POST "http://api-server:8989/v1/pyspy/sample" \
  -H "Content-Type: application/json" \
  -d '{"pod_uid": "...", "node_name": "...", "pid": 12345, "format": "raw"}'
```

---

## Requirements

### Container Requirements

For py-spy to work, the target container must have:
- `CAP_SYS_PTRACE` capability enabled
- Access to the host PID namespace (for node-exporter)

### Supported Python Versions

py-spy supports profiling:
- Python 2.7
- Python 3.3+

---

## Related APIs

- [Workloads API](./workloads.md) - For workload information and metrics
- [Nodes API](./nodes.md) - For node information

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2026-01-07 | Initial release with flamegraph, speedscope, and raw formats |

