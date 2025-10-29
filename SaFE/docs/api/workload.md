# Workload API

Workload is the core resource of the platform, representing various types of tasks that need to run in the cluster, including training jobs, inference services, deployments, etc.

## API List

### 1. Create Workload

Create a new workload.

**Endpoint**: `POST /api/custom/workloads`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "displayName": "my-training-job",
  "description": "Training job description",
  "workspaceId": "cluster-workspace",
  "groupVersionKind": {
    "kind": "PyTorchJob",
    "version": "v1"
  },
  "image": "harbor.example.com/ai/pytorch:2.0",
  "entryPoint": "cHl0aG9uIHRyYWluLnB5",
  "resource": {
    "cpu": "128",
    "gpu": "8",
    "memory": "256Gi",
    "replica": 1
  },
  "priority": 0,
  "timeout": 3600,
  "maxRetry": 0,
  "env": {
    "NCCL_DEBUG": "INFO",
    "CUDA_VISIBLE_DEVICES": "0,1,2,3,4,5,6,7"
  },
  "specifiedNodes": []
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| displayName | string | Yes | Workload display name |
| description | string | No | Workload description |
| workspaceId | string | No | Workspace ID |
| groupVersionKind.kind | string | Yes | Workload type: PyTorchJob/Deployment/StatefulSet/Authoring |
| groupVersionKind.version | string | Yes | Version, usually v1 |
| image | string | Yes | Image address |
| entryPoint | string | No | Startup command (Base64 encoded) |
| resource.cpu | string | Yes | Number of CPU cores |
| resource.gpu | string | No | Number of GPU cards |
| resource.memory | string | Yes | Memory size, e.g., "256Gi" |
| resource.replica | int | Yes | Number of replicas |
| priority | int | No | Priority (0-2), default 0 |
| timeout | int | No | Timeout in seconds, 0 means no timeout |
| maxRetry | int | No | Maximum retry count, default 0 |
| env | object | No | Environment variable key-value pairs |
| specifiedNodes | []string | No | List of specified nodes to run on |

**Response Example**:

```json
{
  "workloadId": "my-training-job-abc123"
}
```

---

### 2. List Workloads

Get workload list with filtering and pagination support.

**Endpoint**: `GET /api/custom/workloads`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| workspaceId | string | No | Filter by workspace ID |
| clusterId | string | No | Filter by cluster ID |
| userId | string | No | Filter by user ID |
| userName | string | No | Filter by username (fuzzy match) |
| phase | string | No | Filter by status: Succeeded/Failed/Pending/Running/Stopped (comma-separated) |
| kind | string | No | Filter by type: Deployment/PyTorchJob/StatefulSet/Authoring (comma-separated) |
| description | string | No | Filter by description (fuzzy match) |
| workloadId | string | No | Filter by workload ID (fuzzy match) |
| since | string | No | Start time, format: 2006-01-02T15:04:05.000Z |
| until | string | No | End time, similar to since |
| offset | int | No | Pagination offset, default 0 |
| limit | int | No | Records per page, default 100 |
| sortBy | string | No | Sort field, default create_time |
| order | string | No | Sort order: desc/asc, default desc |

**Response Example**:

```json
{
  "totalCount": 100,
  "items": [
    {
      "workloadId": "my-training-job-abc123",
      "displayName": "my-training-job",
      "description": "Training job description",
      "workspaceId": "cluster-workspace",
      "clusterId": "cluster-001",
      "userId": "user-001",
      "userName": "zhangsan",
      "phase": "Running",
      "priority": 0,
      "resource": {
        "cpu": "128",
        "gpu": "8",
        "memory": "256Gi",
        "replica": 1
      },
      "groupVersionKind": {
        "kind": "PyTorchJob",
        "version": "v1"
      },
      "creationTime": "2025-01-15T10:30:00.000Z",
      "startTime": "2025-01-15T10:31:00.000Z",
      "endTime": "",
      "runtime": "1h30m45s",
      "schedulerOrder": 0,
      "dispatchCount": 1,
      "isTolerateAll": false,
      "timeout": 3600,
      "secondsUntilTimeout": 1800,
      "message": ""
    }
  ]
}
```

---

### 3. Get Workload Details

Get detailed information about a specific workload.

**Endpoint**: `GET /api/custom/workloads/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |

**Response Example**:

```json
{
  "workloadId": "my-training-job-abc123",
  "displayName": "my-training-job",
  "description": "Training job description",
  "workspaceId": "cluster-workspace",
  "clusterId": "cluster-001",
  "userId": "user-001",
  "userName": "zhangsan",
  "phase": "Running",
  "priority": 0,
  "image": "harbor.example.com/ai/pytorch:2.0",
  "entryPoint": "python train.py",
  "resource": {
    "cpu": "128",
    "gpu": "8",
    "memory": "256Gi",
    "replica": 1
  },
  "isSupervised": false,
  "maxRetry": 0,
  "timeout": 3600,
  "ttlSecondsAfterFinished": 60,
  "groupVersionKind": {
    "kind": "PyTorchJob",
    "version": "v1"
  },
  "creationTime": "2025-01-15T10:30:00.000Z",
  "startTime": "2025-01-15T10:31:00.000Z",
  "endTime": "",
  "runtime": "1h30m45s",
  "secondsUntilTimeout": 1800,
  "env": {
    "NCCL_DEBUG": "INFO"
  },
  "conditions": [
    {
      "type": "Dispatched",
      "status": "True",
      "lastTransitionTime": "2025-01-15T10:31:00.000Z",
      "reason": "Dispatch0",
      "message": "workload dispatched"
    }
  ],
  "pods": [
    {
      "podId": "my-training-job-abc123-worker-0",
      "phase": "Running",
      "nodeName": "node-001",
      "sshAddr": "ssh user-001.my-training-job-abc123-worker-0.cluster-workspace@10.0.0.1"
    }
  ],
  "nodes": [["node-001"]],
  "ranks": [["0"]],
  "customerLabels": {},
  "specifiedNodes": []
}
```

---

### 4. Update Workload

Partially update workload configuration (only when running).

**Endpoint**: `PATCH /api/custom/workloads/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |

**Request Parameters**:

```json
{
  "priority": 1,
  "replica": 2,
  "cpu": "256",
  "gpu": "16",
  "memory": "512Gi",
  "image": "harbor.example.com/ai/pytorch:2.1",
  "description": "New description",
  "timeout": 7200,
  "env": {
    "NEW_VAR": "value"
  }
}
```

**Field Description**: All fields are optional, only provided fields will be updated

**Response**: No content (204)

---

### 5. Delete Workload

Delete a specific workload.

**Endpoint**: `DELETE /api/custom/workloads/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |

**Response**: No content (204)

---

### 6. Stop Workload

Stop a running workload.

**Endpoint**: `POST /api/custom/workloads/:name/stop`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |

**Response**: No content (204)

---

### 7. Batch Delete Workloads

Delete multiple workloads in batch.

**Endpoint**: `POST /api/custom/workloads/delete`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "workloadIds": [
    "workload-001",
    "workload-002",
    "workload-003"
  ]
}
```

**Response**: No content (204)

---

### 8. Batch Stop Workloads

Stop multiple workloads in batch.

**Endpoint**: `POST /api/custom/workloads/stop`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "workloadIds": [
    "workload-001",
    "workload-002"
  ]
}
```

**Response**: No content (204)

---

### 9. Clone Workloads

Clone existing workloads in batch.

**Endpoint**: `POST /api/custom/workloads/clone`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "workloadIds": [
    "workload-001"
  ]
}
```

**Response**: No content (204)

---

### 10. Get Workload Pod Logs

Get logs from a specific pod of the workload.

**Endpoint**: `GET /api/custom/workloads/:name/pods/:podId/logs`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |
| podId | Pod ID |

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| tailLines | int | No | Return last N lines of logs, default 1000 |
| container | string | No | Container name, default main container |
| sinceSeconds | int | No | Return logs from last N seconds |

**Response Example**:

```json
{
  "workloadId": "my-training-job-abc123",
  "podId": "my-training-job-abc123-worker-0",
  "namespace": "cluster-workspace",
  "logs": [
    "2025-01-15 10:31:00 INFO Starting training...",
    "2025-01-15 10:31:05 INFO Epoch 1/100",
    "2025-01-15 10:32:00 INFO Loss: 0.5432"
  ]
}
```

---

### 11. Get Workload Pod Containers

Get container list and available shells for a workload pod.

**Endpoint**: `GET /api/custom/workloads/:name/pods/:podId/containers`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |
| podId | Pod ID |

**Response Example**:

```json
{
  "containers": [
    {
      "name": "pytorch"
    },
    {
      "name": "sidecar"
    }
  ],
  "shells": ["bash", "sh", "zsh"]
}
```

---

### 12. Get Workload Service

Get Service information associated with the workload.

**Endpoint**: `GET /api/custom/workloads/:name/service`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workload ID |

**Response Example**:

```json
{
  "serviceName": "my-training-job-abc123",
  "clusterIP": "10.96.0.100",
  "ports": [
    {
      "name": "http",
      "port": 8080,
      "targetPort": 8080,
      "protocol": "TCP"
    }
  ]
}
```

---

## Workload Status

| Status | Description |
|--------|-------------|
| Pending | Waiting for scheduling |
| Running | Currently running |
| Succeeded | Completed successfully |
| Failed | Execution failed |
| Stopped | Stopped |
| Updating | Being updated |

## Workload Types

| Type | Description | Use Case |
|------|-------------|----------|
| PyTorchJob | PyTorch distributed training | Deep learning training |
| Deployment | K8s stateless deployment | Inference service |
| StatefulSet | K8s stateful deployment | Stateful service |
| Authoring | Development environment | Interactive development |

## Notes

1. **EntryPoint Encoding**: `entryPoint` field must be Base64 encoded
2. **Node Specification**: When `specifiedNodes` is set, `replica` will be automatically set to the number of nodes
3. **Resource Units**: CPU is in cores, memory format like "256Gi"
4. **Priority**: 0 is normal, 1 is high, 2 is highest (requires appropriate permissions)
5. **Timeout Setting**: timeout of 0 means no timeout, otherwise in seconds
