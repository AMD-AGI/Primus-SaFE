# OpsJob API

Operational job API for managing system operational tasks.

## API List

### 1. Create Operational Job

**Endpoint**: `POST /api/custom/opsjobs`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "type": "BackupJob",
  "description": "Database backup",
  "schedule": "0 2 * * *",
  "parameters": {
    "target": "database",
    "retention": "7d"
  }
}
```

**Response**: `{ "jobId": "backup-job-abc123" }`

---

### 2. List Operational Jobs

**Endpoint**: `GET /api/custom/opsjobs`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalCount": 5,
  "items": [
    {
      "jobId": "backup-job-abc123",
      "type": "BackupJob",
      "description": "Database backup",
      "phase": "Running",
      "creationTime": "2025-01-10T08:00:00.000Z",
      "lastRunTime": "2025-01-15T02:00:00.000Z"
    }
  ]
}
```

---

### 3. Get Operational Job Details

**Endpoint**: `GET /api/custom/opsjobs/:name`

**Authentication Required**: Yes

---

### 4. Stop Operational Job

**Endpoint**: `POST /api/custom/opsjobs/:name/stop`

**Authentication Required**: Yes

**Response**: Empty response on success (HTTP 200)

---

### 5. Delete Operational Job

**Endpoint**: `DELETE /api/custom/opsjobs/:name`

**Authentication Required**: Yes

**Response**: Empty response on success (HTTP 200)

---

## Job Types

| Type | Description |
|------|-------------|
| BackupJob | Backup job |
| CleanupJob | Cleanup job |
| MonitorJob | Monitoring job |
| MaintenanceJob | Maintenance job |
