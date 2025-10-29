# Service API

Service-related interfaces provide log query and other functions.

## API List

### 1. Get Service Logs

Query system service logs.

**Endpoint**: `POST /api/custom/service/:name/logs`

**Authentication Required**: Yes

**Path Parameters**:
- `name`: Service name

**Request Example**:
```json
{
  "tailLines": 1000,
  "sinceSeconds": 3600
}
```

**Response Example**:
```json
{
  "serviceName": "scheduler",
  "logs": [
    "2025-01-15 10:00:00 INFO Scheduler started",
    "2025-01-15 10:00:05 INFO Processing workload queue..."
  ]
}
```

---

## Supported Services

- `scheduler`: Scheduler service
- `controller`: Controller service
- `apiserver`: API server
- `monitor`: Monitor service
