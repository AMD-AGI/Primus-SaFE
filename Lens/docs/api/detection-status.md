# Detection Status API

The Detection Status API provides endpoints to query and manage AI framework detection status for workloads. It tracks the detection progress, coverage from various sources, evidence collected, and related tasks.

## Endpoints

### Get Detection Summary

Returns a summary of all detection statuses across workloads.

**Endpoint:** `GET /api/v1/detection-status/summary`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name, defaults to current cluster |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total_workloads": 150,
    "status_counts": {
      "unknown": 20,
      "suspected": 30,
      "confirmed": 80,
      "verified": 15,
      "conflict": 5
    },
    "detection_state_counts": {
      "pending": 10,
      "in_progress": 5,
      "completed": 130,
      "failed": 5
    },
    "recent_detections": [
      {
        "workload_uid": "abc-123",
        "status": "confirmed",
        "detection_state": "completed",
        "framework": "pytorch",
        "confidence": 0.95,
        "updated_at": "2025-12-22T10:00:00Z"
      }
    ]
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `total_workloads` | integer | Total number of workloads with detection records |
| `status_counts` | object | Count of workloads by detection status |
| `detection_state_counts` | object | Count of workloads by detection state |
| `recent_detections` | array | Recently updated detections (last 10) |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get detection summary
curl -X GET "http://localhost:8080/api/v1/detection-status/summary"

# Get detection summary for a specific cluster
curl -X GET "http://localhost:8080/api/v1/detection-status/summary?cluster=gpu-cluster-01"
```

---

### List Detection Statuses

Lists detection statuses with filtering and pagination.

**Endpoint:** `GET /api/v1/detection-status`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |
| `status` | string | No | - | Filter by detection status (unknown, suspected, confirmed, verified, conflict) |
| `state` | string | No | - | Filter by detection state (pending, in_progress, completed, failed) |
| `page` | integer | No | 1 | Page number |
| `page_size` | integer | No | 20 | Items per page (max: 100) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "data": [
      {
        "workload_uid": "abc-123",
        "status": "confirmed",
        "detection_state": "completed",
        "framework": "pytorch",
        "frameworks": ["pytorch", "deepspeed"],
        "workload_type": "training",
        "confidence": 0.95,
        "framework_layer": "wrapper",
        "wrapper_framework": "deepspeed",
        "base_framework": "pytorch",
        "evidence_count": 5,
        "evidence_sources": ["process", "log", "image"],
        "attempt_count": 2,
        "max_attempts": 5,
        "last_attempt_at": "2025-12-22T09:00:00Z",
        "next_attempt_at": null,
        "confirmed_at": "2025-12-22T09:30:00Z",
        "created_at": "2025-12-20T08:00:00Z",
        "updated_at": "2025-12-22T10:00:00Z",
        "coverage": [],
        "tasks": [],
        "has_conflicts": false
      }
    ],
    "total": 150,
    "page": 1,
    "page_size": 20
  },
  "traceId": "trace-def456"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `workload_uid` | string | Unique workload identifier |
| `status` | string | Detection status: unknown, suspected, confirmed, verified, conflict |
| `detection_state` | string | Active detection state: pending, in_progress, completed, failed |
| `framework` | string | Primary detected framework |
| `frameworks` | array | All detected frameworks |
| `workload_type` | string | training or inference |
| `confidence` | float | Aggregated confidence [0-1] |
| `framework_layer` | string | wrapper or base |
| `wrapper_framework` | string | Wrapper framework name (e.g., DeepSpeed, Accelerate) |
| `base_framework` | string | Base framework name (e.g., PyTorch, TensorFlow) |
| `evidence_count` | integer | Total evidence records |
| `evidence_sources` | array | Sources that contributed evidence |
| `attempt_count` | integer | Detection attempts made |
| `max_attempts` | integer | Max detection attempts |
| `last_attempt_at` | string | Last detection attempt time (RFC3339 format) |
| `next_attempt_at` | string | Next scheduled attempt time (RFC3339 format) |
| `confirmed_at` | string | When detection was confirmed (RFC3339 format) |
| `created_at` | string | Detection record creation time (RFC3339 format) |
| `updated_at` | string | Last update time (RFC3339 format) |
| `has_conflicts` | boolean | Whether conflicts exist |
| `total` | integer | Total number of records matching the filter |
| `page` | integer | Current page number |
| `page_size` | integer | Number of items per page |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all detection statuses with default pagination
curl -X GET "http://localhost:8080/api/v1/detection-status"

# Filter by status
curl -X GET "http://localhost:8080/api/v1/detection-status?status=confirmed"

# Filter by detection state
curl -X GET "http://localhost:8080/api/v1/detection-status?state=in_progress"

# With pagination
curl -X GET "http://localhost:8080/api/v1/detection-status?page=2&page_size=50"
```

---

### Get Detection Status

Retrieves the full detection status for a specific workload.

**Endpoint:** `GET /api/v1/detection-status/:workload_uid`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "workload_uid": "abc-123",
    "status": "confirmed",
    "detection_state": "completed",
    "framework": "pytorch",
    "frameworks": ["pytorch", "deepspeed"],
    "workload_type": "training",
    "confidence": 0.95,
    "framework_layer": "wrapper",
    "wrapper_framework": "deepspeed",
    "base_framework": "pytorch",
    "evidence_count": 5,
    "evidence_sources": ["process", "log", "image"],
    "attempt_count": 2,
    "max_attempts": 5,
    "last_attempt_at": "2025-12-22T09:00:00Z",
    "next_attempt_at": null,
    "confirmed_at": "2025-12-22T09:30:00Z",
    "created_at": "2025-12-20T08:00:00Z",
    "updated_at": "2025-12-22T10:00:00Z",
    "coverage": [
      {
        "source": "log",
        "status": "collected",
        "attempt_count": 3,
        "last_attempt_at": "2025-12-22T09:00:00Z",
        "last_success_at": "2025-12-22T09:00:00Z",
        "evidence_count": 2,
        "covered_from": "2025-12-20T08:00:00Z",
        "covered_to": "2025-12-22T09:00:00Z",
        "log_available_from": "2025-12-20T08:00:00Z",
        "log_available_to": "2025-12-22T10:00:00Z",
        "has_gap": true
      },
      {
        "source": "process",
        "status": "collected",
        "attempt_count": 1,
        "last_success_at": "2025-12-20T08:30:00Z",
        "evidence_count": 2
      }
    ],
    "tasks": [
      {
        "task_type": "detection_coordinator",
        "status": "completed",
        "created_at": "2025-12-20T08:00:00Z",
        "updated_at": "2025-12-22T09:30:00Z",
        "coordinator_state": "confirmed"
      }
    ],
    "has_conflicts": false,
    "conflicts": []
  },
  "traceId": "trace-ghi789"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - workload_uid is required
- `404 Not Found` - Detection not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get detection status for a specific workload
curl -X GET "http://localhost:8080/api/v1/detection-status/abc-123"
```

---

### Get Detection Coverage

Returns detection coverage status for each source of a workload.

**Endpoint:** `GET /api/v1/detection-status/:workload_uid/coverage`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "workload_uid": "abc-123",
    "coverage": [
      {
        "source": "process",
        "status": "collected",
        "attempt_count": 1,
        "last_attempt_at": "2025-12-22T09:00:00Z",
        "last_success_at": "2025-12-22T09:00:00Z",
        "evidence_count": 2
      },
      {
        "source": "log",
        "status": "collecting",
        "attempt_count": 3,
        "last_attempt_at": "2025-12-22T09:00:00Z",
        "evidence_count": 1,
        "covered_from": "2025-12-20T08:00:00Z",
        "covered_to": "2025-12-22T08:00:00Z",
        "log_available_from": "2025-12-20T08:00:00Z",
        "log_available_to": "2025-12-22T10:00:00Z",
        "has_gap": true
      },
      {
        "source": "image",
        "status": "not_applicable",
        "attempt_count": 0,
        "evidence_count": 0
      }
    ],
    "total": 3
  },
  "traceId": "trace-jkl012"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `source` | string | Detection source: process, log, image, label, wandb, import |
| `status` | string | Coverage status: pending, collecting, collected, failed, not_applicable |
| `attempt_count` | integer | Collection attempts |
| `last_attempt_at` | string | Last collection attempt (RFC3339 format) |
| `last_success_at` | string | Last successful collection (RFC3339 format) |
| `last_error` | string | Last error if any |
| `evidence_count` | integer | Evidence records from this source |
| `covered_from` | string | Log scan start time (log source only, RFC3339 format) |
| `covered_to` | string | Log scan end time (log source only, RFC3339 format) |
| `log_available_from` | string | Earliest available log time (log source only, RFC3339 format) |
| `log_available_to` | string | Latest available log time (log source only, RFC3339 format) |
| `has_gap` | boolean | Whether there's uncovered log window (log source only) |

**Coverage Sources:**

| Source | Description |
|--------|-------------|
| `process` | Process probe detection |
| `log` | Log-based detection |
| `image` | Container image detection |
| `label` | Kubernetes label detection |
| `wandb` | Weights & Biases integration detection |
| `import` | Python import detection |

**Coverage Status:**

| Status | Description |
|--------|-------------|
| `pending` | Not yet started |
| `collecting` | Collection in progress |
| `collected` | Successfully collected |
| `failed` | Collection failed |
| `not_applicable` | Source not applicable for workload |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - workload_uid is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get detection coverage for a workload
curl -X GET "http://localhost:8080/api/v1/detection-status/abc-123/coverage"
```

---

### Initialize Detection Coverage

Initializes detection coverage records for a workload.

**Endpoint:** `POST /api/v1/detection-status/:workload_uid/coverage/initialize`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |

**Response (Created - 201):**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "coverage initialized",
    "coverage": [
      {
        "source": "process",
        "status": "pending",
        "attempt_count": 0,
        "evidence_count": 0
      },
      {
        "source": "log",
        "status": "pending",
        "attempt_count": 0,
        "evidence_count": 0
      }
    ],
    "count": 6
  },
  "traceId": "trace-mno345"
}
```

**Response (Already Initialized - 200):**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "coverage already initialized",
    "count": 6
  },
  "traceId": "trace-pqr678"
}
```

**Status Codes:**
- `200 OK` - Coverage already initialized
- `201 Created` - Coverage initialized successfully
- `400 Bad Request` - workload_uid is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Initialize detection coverage for a workload
curl -X POST "http://localhost:8080/api/v1/detection-status/abc-123/coverage/initialize"
```

---

### Get Uncovered Log Window

Returns the uncovered log time window for a workload (gap between scanned and available logs).

**Endpoint:** `GET /api/v1/detection-status/:workload_uid/coverage/log-gap`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |

**Response (Has Gap):**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "workload_uid": "abc-123",
    "has_gap": true,
    "gap_from": "2025-12-22T08:00:00Z",
    "gap_to": "2025-12-22T10:00:00Z",
    "gap_duration_seconds": 7200
  },
  "traceId": "trace-stu901"
}
```

**Response (No Gap):**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "workload_uid": "abc-123",
    "has_gap": false
  },
  "traceId": "trace-vwx234"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `workload_uid` | string | Workload unique identifier |
| `has_gap` | boolean | Whether there is an uncovered log window |
| `gap_from` | string | Gap start time (RFC3339 format, only when has_gap is true) |
| `gap_to` | string | Gap end time (RFC3339 format, only when has_gap is true) |
| `gap_duration_seconds` | float | Gap duration in seconds (only when has_gap is true) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - workload_uid is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get uncovered log window for a workload
curl -X GET "http://localhost:8080/api/v1/detection-status/abc-123/coverage/log-gap"
```

---

### Get Detection Tasks

Returns detection-related tasks for a workload.

**Endpoint:** `GET /api/v1/detection-status/:workload_uid/tasks`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "workload_uid": "abc-123",
    "tasks": [
      {
        "task_type": "detection_coordinator",
        "status": "completed",
        "lock_owner": "",
        "created_at": "2025-12-20T08:00:00Z",
        "updated_at": "2025-12-22T09:30:00Z",
        "attempt_count": 2,
        "coordinator_state": "confirmed",
        "ext": {
          "coordinator_state": "confirmed",
          "attempt_count": 2
        }
      },
      {
        "task_type": "process_probe",
        "status": "completed",
        "created_at": "2025-12-20T08:05:00Z",
        "updated_at": "2025-12-20T08:10:00Z",
        "attempt_count": 1
      },
      {
        "task_type": "log_detection",
        "status": "pending",
        "created_at": "2025-12-22T10:00:00Z",
        "updated_at": "2025-12-22T10:00:00Z",
        "next_attempt_at": "2025-12-22T10:30:00Z"
      }
    ],
    "total": 3
  },
  "traceId": "trace-yza567"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `task_type` | string | Type of the detection task |
| `status` | string | Task status |
| `lock_owner` | string | Current lock owner |
| `created_at` | string | Task creation time (RFC3339 format) |
| `updated_at` | string | Last update time (RFC3339 format) |
| `attempt_count` | integer | Number of attempts |
| `next_attempt_at` | string | Next attempt time (RFC3339 format) |
| `coordinator_state` | string | State for coordinator tasks |
| `ext` | object | Additional task data |

**Detection Task Types:**

| Task Type | Description |
|-----------|-------------|
| `detection_coordinator` | Main detection orchestration task |
| `active_detection` | Active detection task |
| `process_probe` | Process inspection task |
| `log_detection` | Log scanning task |
| `image_probe` | Container image analysis task |
| `label_probe` | Kubernetes label analysis task |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - workload_uid is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get detection tasks for a workload
curl -X GET "http://localhost:8080/api/v1/detection-status/abc-123/tasks"
```

---

### Get Detection Evidence

Returns evidence records collected for a workload.

**Endpoint:** `GET /api/v1/detection-status/:workload_uid/evidence`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |
| `source` | string | No | - | Filter by source (process, log, image, label, etc.) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "workload_uid": "abc-123",
    "evidence": [
      {
        "id": 1,
        "workload_uid": "abc-123",
        "source": "process",
        "source_type": "active",
        "framework": "pytorch",
        "workload_type": "training",
        "confidence": 0.95,
        "framework_layer": "base",
        "wrapper_framework": "",
        "base_framework": "pytorch",
        "evidence": {
          "cmdline": "python train.py",
          "process_name": "python",
          "detected_imports": ["torch", "torch.distributed"]
        },
        "detected_at": "2025-12-20T08:30:00Z",
        "created_at": "2025-12-20T08:30:00Z"
      },
      {
        "id": 2,
        "workload_uid": "abc-123",
        "source": "log",
        "source_type": "passive",
        "framework": "deepspeed",
        "workload_type": "training",
        "confidence": 0.85,
        "framework_layer": "wrapper",
        "wrapper_framework": "deepspeed",
        "base_framework": "pytorch",
        "evidence": {
          "pattern_matched": "DeepSpeed initialized",
          "log_timestamp": "2025-12-20T08:35:00Z"
        },
        "detected_at": "2025-12-20T08:35:00Z",
        "created_at": "2025-12-20T08:35:00Z"
      }
    ],
    "total": 2
  },
  "traceId": "trace-bcd890"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique evidence record ID |
| `workload_uid` | string | Workload unique identifier |
| `source` | string | Evidence source: process, log, image, label, etc. |
| `source_type` | string | passive or active |
| `framework` | string | Detected framework |
| `workload_type` | string | training or inference |
| `confidence` | float | Detection confidence [0-1] |
| `framework_layer` | string | wrapper or base |
| `wrapper_framework` | string | Wrapper framework name |
| `base_framework` | string | Base framework name |
| `evidence` | object | Evidence details (varies by source) |
| `detected_at` | string | When evidence was collected (RFC3339 format) |
| `created_at` | string | Record creation time (RFC3339 format) |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - workload_uid is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get all evidence for a workload
curl -X GET "http://localhost:8080/api/v1/detection-status/abc-123/evidence"

# Get evidence filtered by source
curl -X GET "http://localhost:8080/api/v1/detection-status/abc-123/evidence?source=log"
```

---

### Report Log Detection

Receives log detection report from telemetry-processor (internal API).

**Endpoint:** `POST /api/v1/detection-status/log-report`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | - | Cluster name |

**Request Body:**

```json
{
  "workload_uid": "abc-123",
  "detected_at": "2025-12-22T10:00:00Z",
  "log_timestamp": "2025-12-22T09:55:00Z",
  "framework": "pytorch",
  "confidence": 0.9,
  "pattern_matched": "torch.distributed.init_process_group"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `workload_uid` | string | Yes | Workload unique identifier |
| `detected_at` | string | No | Detection timestamp (RFC3339 format) |
| `log_timestamp` | string | Yes | Original log timestamp (RFC3339 format) |
| `framework` | string | No | Detected framework name |
| `confidence` | float | No | Detection confidence [0-1] |
| `pattern_matched` | string | No | Pattern that matched |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok"
  },
  "traceId": "trace-efg123"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid request body
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Report log detection
curl -X POST "http://localhost:8080/api/v1/detection-status/log-report" \
  -H "Content-Type: application/json" \
  -d '{
    "workload_uid": "abc-123",
    "log_timestamp": "2025-12-22T09:55:00Z",
    "framework": "pytorch",
    "confidence": 0.9,
    "pattern_matched": "torch.distributed.init_process_group"
  }'
```

---

### Trigger Detection

Manually triggers detection for a workload.

**Endpoint:** `POST /api/v1/detection-status/:workload_uid/trigger`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workload_uid` | string | Yes | Unique workload identifier |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "detection triggered for workload abc-123",
    "workload_uid": "abc-123",
    "task_type": "detection_coordinator",
    "status": "pending"
  },
  "traceId": "trace-hij456"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - workload_uid is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Trigger detection for a workload
curl -X POST "http://localhost:8080/api/v1/detection-status/abc-123/trigger"
```

---

## Common Error Response Format

All endpoints follow a consistent error response format:

```json
{
  "code": 400,
  "message": "workload_uid is required",
  "data": null,
  "traceId": "trace-error-123"
}
```

## Time Format

All time-related query parameters and response fields use **RFC3339 format** (e.g., `2025-11-05T10:30:00Z`).

## Notes

1. **Detection Status Values**: Valid detection status values are:
   - `unknown` - Framework not yet detected
   - `suspected` - Evidence suggests a framework but not confirmed
   - `confirmed` - Framework detection confirmed with high confidence
   - `verified` - Detection verified by user or additional sources
   - `conflict` - Conflicting evidence from multiple sources

2. **Detection State Values**: Valid detection state values are:
   - `pending` - Detection not yet started
   - `in_progress` - Detection currently running
   - `completed` - Detection completed
   - `failed` - Detection failed

3. **Pagination**: The list endpoint supports pagination with `page` and `page_size` parameters. Maximum page size is limited to 100 items.

4. **Confidence Score**: Confidence values range from 0 to 1, where higher values indicate higher certainty of detection.

5. **Framework Layers**: Detection distinguishes between:
   - `base` - Base framework (e.g., PyTorch, TensorFlow)
   - `wrapper` - Wrapper framework built on top of base (e.g., DeepSpeed, Accelerate, Lightning)

6. **Source Types**: Evidence sources are categorized as:
   - `active` - Actively probed (e.g., process inspection)
   - `passive` - Passively collected (e.g., log parsing)

