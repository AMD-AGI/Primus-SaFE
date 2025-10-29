# Log API

Log query API provides log query capabilities for workloads and services.

## API List

### 1. Get Workload Logs

Query aggregated workload logs (based on Elasticsearch).

**Endpoint**: `POST /api/custom/workloads/:name/logs`

**Authentication Required**: Yes

**Path Parameters**:
- `name`: Workload ID

**Request Example**:
```json
{
  "startTime": "2025-01-15T10:00:00.000Z",
  "endTime": "2025-01-15T11:00:00.000Z",
  "keyword": "error",
  "limit": 1000
}
```

**Response Example**:
```json
{
  "totalCount": 150,
  "logs": [
    {
      "timestamp": "2025-01-15T10:30:00.000Z",
      "podName": "training-job-worker-0",
      "containerName": "pytorch",
      "message": "ERROR: Connection timeout"
    }
  ]
}
```

---

### 2. Get Workload Log Context

Get context (N lines before and after) for a specific log line.

**Endpoint**: `POST /api/custom/workloads/:name/logs/:docId/context`

**Authentication Required**: Yes

**Path Parameters**:
- `name`: Workload ID
- `docId`: Log document ID

**Request Example**:
```json
{
  "beforeLines": 10,
  "afterLines": 10
}
```

---

## Query Description

### Time Range
- `startTime`: Start time (ISO 8601 format)
- `endTime`: End time (ISO 8601 format)
- Maximum query range: 24 hours

### Keyword Search
- Supports regular expressions
- Case sensitive
- Supports multiple keywords (space-separated)

### Limitations
- Single query returns maximum 10,000 log lines
- Recommend using time range to narrow query scope

## Notes

- Log query function depends on Elasticsearch
- Logs are retained for 30 days by default
- Large log queries may impact performance
