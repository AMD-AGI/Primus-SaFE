# Job Execution Histories API

The Job Execution Histories API provides operations for querying and analyzing historical job execution records, including listing with advanced filtering, retrieving details, getting recent failures, and generating statistics.

## Endpoints

### List Job Execution Histories

Retrieves a paginated list of job execution histories with advanced filtering and sorting support.

**Endpoint:** `GET /api/job-execution-histories`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `page_num` | integer | No | 1 | Page number (must be positive) |
| `page_size` | integer | No | 20 | Number of items per page (max: 100) |
| `job_name` | string | No | - | Filter by job name (supports fuzzy matching) |
| `job_type` | string | No | - | Filter by job type (supports fuzzy matching) |
| `status` | string | No | - | Filter by status (running/success/failed/cancelled/timeout) |
| `cluster_name` | string | No | - | Filter by cluster name |
| `hostname` | string | No | - | Filter by hostname |
| `start_time_from` | string | No | - | Start time range begin (RFC3339 format) |
| `start_time_to` | string | No | - | Start time range end (RFC3339 format) |
| `min_duration` | float | No | - | Minimum execution duration in seconds |
| `max_duration` | float | No | - | Maximum execution duration in seconds |
| `order_by` | string | No | started_at DESC | Sort field and direction |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "data": [
      {
        "id": 12345,
        "job_name": "training-model-v2",
        "job_type": "pytorch",
        "status": "success",
        "cluster_name": "gpu-cluster-01",
        "hostname": "worker-node-03",
        "started_at": "2025-11-05T10:30:00Z",
        "finished_at": "2025-11-05T12:45:30Z",
        "duration": 8130.5,
        "exit_code": 0,
        "error_message": null,
        "metadata": {
          "gpu_count": 8,
          "batch_size": 256
        }
      },
      {
        "id": 12344,
        "job_name": "data-preprocessing",
        "job_type": "spark",
        "status": "failed",
        "cluster_name": "gpu-cluster-01",
        "hostname": "worker-node-05",
        "started_at": "2025-11-05T09:15:00Z",
        "finished_at": "2025-11-05T09:20:15Z",
        "duration": 315.2,
        "exit_code": 1,
        "error_message": "OutOfMemoryError: Unable to allocate memory",
        "metadata": {
          "memory_limit": "32GB"
        }
      }
    ],
    "total": 1523,
    "pageNum": 1,
    "pageSize": 20
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | int64 | Unique execution history ID |
| `job_name` | string | Name of the job |
| `job_type` | string | Type/category of the job |
| `status` | string | Execution status (running/success/failed/cancelled/timeout) |
| `cluster_name` | string | Name of the cluster where job executed |
| `hostname` | string | Hostname of the node where job executed |
| `started_at` | string | Job start time (RFC3339 format) |
| `finished_at` | string | Job finish time (RFC3339 format, null if still running) |
| `duration` | float | Execution duration in seconds |
| `exit_code` | integer | Exit code of the job process |
| `error_message` | string | Error message if job failed (null if successful) |
| `metadata` | object | Additional job metadata (JSON) |
| `total` | integer | Total number of records matching the filter |
| `pageNum` | integer | Current page number |
| `pageSize` | integer | Number of items per page |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (e.g., invalid time format)
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all job execution histories with default pagination
curl -X GET "http://localhost:8080/api/job-execution-histories"

# Filter by job name and status
curl -X GET "http://localhost:8080/api/job-execution-histories?job_name=training&status=failed"

# Filter by time range (RFC3339 format)
curl -X GET "http://localhost:8080/api/job-execution-histories?start_time_from=2025-11-01T00:00:00Z&start_time_to=2025-11-05T23:59:59Z"

# Filter by duration range (jobs that took between 1 hour and 3 hours)
curl -X GET "http://localhost:8080/api/job-execution-histories?min_duration=3600&max_duration=10800"

# Complex filter with pagination and sorting
curl -X GET "http://localhost:8080/api/job-execution-histories?cluster_name=gpu-cluster-01&status=success&page_num=2&page_size=50&order_by=duration DESC"
```

---

### Get Job Execution History

Retrieves detailed information about a specific job execution history by ID.

**Endpoint:** `GET /api/job-execution-histories/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | int64 | Yes | Job execution history ID |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 12345,
    "job_name": "training-model-v2",
    "job_type": "pytorch",
    "status": "success",
    "cluster_name": "gpu-cluster-01",
    "hostname": "worker-node-03",
    "started_at": "2025-11-05T10:30:00Z",
    "finished_at": "2025-11-05T12:45:30Z",
    "duration": 8130.5,
    "exit_code": 0,
    "error_message": null,
    "metadata": {
      "gpu_count": 8,
      "batch_size": 256,
      "learning_rate": 0.001,
      "epochs": 100
    },
    "created_at": "2025-11-05T10:30:00Z",
    "updated_at": "2025-11-05T12:45:30Z"
  },
  "traceId": "trace-def456"
}
```

**Response Fields:**

Same as the list endpoint, with possible additional fields:
- `created_at` - Record creation time
- `updated_at` - Record last update time

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid history ID format
- `404 Not Found` - History record not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get specific job execution history
curl -X GET "http://localhost:8080/api/job-execution-histories/12345"
```

---

### Get Recent Failures

Retrieves the most recent failed job executions for quick troubleshooting.

**Endpoint:** `GET /api/job-execution-histories/recent-failures`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `limit` | integer | No | 10 | Number of records to return (max: 100) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 12350,
      "job_name": "model-inference",
      "job_type": "tensorflow",
      "status": "failed",
      "cluster_name": "gpu-cluster-02",
      "hostname": "worker-node-08",
      "started_at": "2025-11-05T14:20:00Z",
      "finished_at": "2025-11-05T14:25:30Z",
      "duration": 330.0,
      "exit_code": 137,
      "error_message": "Container killed due to memory limit",
      "metadata": {
        "oom_killed": true
      }
    },
    {
      "id": 12348,
      "job_name": "data-validation",
      "job_type": "python",
      "status": "timeout",
      "cluster_name": "gpu-cluster-01",
      "hostname": "worker-node-02",
      "started_at": "2025-11-05T13:00:00Z",
      "finished_at": "2025-11-05T14:00:00Z",
      "duration": 3600.0,
      "exit_code": 124,
      "error_message": "Job exceeded maximum execution time",
      "metadata": {
        "timeout_seconds": 3600
      }
    }
  ],
  "traceId": "trace-ghi789"
}
```

**Response Fields:**

Returns an array of job execution history objects with the same fields as the list endpoint.

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get 10 most recent failures (default)
curl -X GET "http://localhost:8080/api/job-execution-histories/recent-failures"

# Get 50 most recent failures
curl -X GET "http://localhost:8080/api/job-execution-histories/recent-failures?limit=50"
```

---

### Get Job Statistics

Retrieves statistical analysis for a specific job name, including success rate, average duration, and failure patterns.

**Endpoint:** `GET /api/job-execution-histories/statistics/:job_name`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `job_name` | string | Yes | Name of the job to get statistics for |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "job_name": "training-model-v2",
    "total_executions": 145,
    "successful_executions": 132,
    "failed_executions": 10,
    "cancelled_executions": 2,
    "timeout_executions": 1,
    "success_rate": 0.9103,
    "average_duration": 7856.3,
    "min_duration": 6420.0,
    "max_duration": 9180.5,
    "median_duration": 7800.0,
    "std_duration": 542.8,
    "last_execution_at": "2025-11-05T12:45:30Z",
    "last_success_at": "2025-11-05T12:45:30Z",
    "last_failure_at": "2025-11-04T15:20:10Z",
    "common_errors": [
      {
        "error_message": "CUDA out of memory",
        "count": 5
      },
      {
        "error_message": "Connection timeout to data server",
        "count": 3
      }
    ]
  },
  "traceId": "trace-jkl012"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `job_name` | string | Name of the job |
| `total_executions` | integer | Total number of job executions |
| `successful_executions` | integer | Number of successful executions |
| `failed_executions` | integer | Number of failed executions |
| `cancelled_executions` | integer | Number of cancelled executions |
| `timeout_executions` | integer | Number of timed-out executions |
| `success_rate` | float | Success rate (0.0 to 1.0) |
| `average_duration` | float | Average execution duration in seconds |
| `min_duration` | float | Minimum execution duration in seconds |
| `max_duration` | float | Maximum execution duration in seconds |
| `median_duration` | float | Median execution duration in seconds |
| `std_duration` | float | Standard deviation of duration |
| `last_execution_at` | string | Timestamp of last execution (RFC3339 format) |
| `last_success_at` | string | Timestamp of last successful execution (RFC3339 format) |
| `last_failure_at` | string | Timestamp of last failed execution (RFC3339 format) |
| `common_errors` | array | List of most common error messages with occurrence count |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - job_name parameter is required
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Get statistics for a specific job
curl -X GET "http://localhost:8080/api/job-execution-histories/statistics/training-model-v2"

# URL encode job name if it contains special characters
curl -X GET "http://localhost:8080/api/job-execution-histories/statistics/data-preprocessing%20v1.0"
```

---

## Common Error Response Format

All endpoints follow a consistent error response format:

```json
{
  "code": 400,
  "message": "invalid start_time_from format, use RFC3339",
  "data": null,
  "traceId": "trace-error-123"
}
```

## Time Format

All time-related query parameters and response fields use **RFC3339 format** (e.g., `2025-11-05T10:30:00Z`).

## Notes

1. **Pagination**: The list endpoint supports pagination with `page_num` and `page_size` parameters. Maximum page size is limited to 100 items.

2. **Fuzzy Matching**: The `job_name` and `job_type` filters support fuzzy matching (partial string matching).

3. **Duration Filtering**: Use `min_duration` and `max_duration` to filter jobs by execution time. Values are in seconds and support decimal precision.

4. **Status Values**: Valid status values are:
   - `running` - Job is currently executing
   - `success` - Job completed successfully
   - `failed` - Job failed with an error
   - `cancelled` - Job was cancelled by user
   - `timeout` - Job exceeded time limit

5. **Sorting**: The `order_by` parameter accepts field names with optional direction (ASC/DESC). Common sort fields include:
   - `started_at` (default: DESC)
   - `finished_at`
   - `duration`
   - `job_name`
   - `status`

6. **Statistics Calculation**: Job statistics are calculated from all historical executions of the specified job name, not limited by time range.

