# OpsJob API

## Overview

OpsJob(operations job) performs specific administrative tasks in the system. Common examples include addon installation, node preflight checks, image saves, and system reboots. These jobs automate routine maintenance and operational procedures across the infrastructure.

## API List

### 1. Create OpsJob

**Endpoint**: `POST /api/v1/opsjobs`

**Authentication Required**: Yes

**Request Example (addon)**:
```json
{
  "name": "addon-upgrade",
  "type": "addon",
  "inputs": [
    { "name": "addon.template", "value": "disable-os-upgrade" },
    { "name": "workspace", "value": "prod-cluster-ai-team" }
  ],
  "type": "addon",
  "batchCount": 2,
  "availableRatio": 1.0,
  "securityUpgrade": true,
  "timeoutSecond": 1800,
  "excludedNodes": ["node-id1"],
  "ttlSecondsAfterFinished": 600,
  "isTolerateAll": false
}
```

**Request Example (preflight)**:
```json
{
  "name": "preflight-check",
  "type": "preflight",
  "inputs": [
    { "name": "cluster", "value": "prod-cluster" }
  ],
  "resource": { "cpu": "8", "memory": "32Gi" },
  "image": "harbor.example.com/tools/preflight:latest",
  "entryPoint": "YmFzaCAtYyAnZWNobyAnJw==",
  "env": { "CHECK_DISK": "true" },
  "hostpath": ["/var/log"],
  "ttlSecondsAfterFinished": 3600,
  "timeoutSecond": 7200,
  "isTolerateAll": true,
  "hostpath": ["/nfs_models"]
}
```

**Request Example (dumplog)**:
```json
{
  "name": "dump-training-log",
  "type": "dumplog",
  "inputs": [
    { "name": "workload", "value": "my-training-job-abc123" }
  ],
  "timeoutSecond": 1800
}
```

**Request Example (reboot)**:
```json
{
  "name": "reboot-node",
  "type": "reboot",
  "inputs": [
    { "name": "node", "value": "gpu-node-001" }
  ],
  "ttlSecondsAfterFinished": 3600
}
```

**Request Example (exportimage)**:
```json
{
  "name": "export-workload-image",
  "type": "exportimage",
  "inputs": [
    { "name": "workload", "value": "pytorch-training-001" }
  ],
  "timeoutSecond": 3600
}
```

**Note**: 
- The system will automatically retrieve the workload's image and add it to inputs as `{ "name": "image", "value": "..." }`.
- The target image name will be converted to **lowercase** to comply with Harbor naming requirements. For example, `docker.io/library/busybox:latest` will be exported as `custom/library/busybox:20251113`.
- The export process uses **HTTPS (port 443)** to connect to Harbor registry. Ensure your Harbor instance is configured with HTTPS.

**Request Parameters**:

| Parameter | Type | Required | Description                                                                                               |
|-----------|------|----------|-----------------------------------------------------------------------------------------------------------|
| name | string | Yes | Used to generate ops job ID; normalized with random suffix                                                |
| type | string | Yes | Ops job type: addon/preflight/dumplog/reboot/exportimage                                                  |
| inputs[].name | string | Yes | Target selector; allowed: node, addon.template, workload, workspace, cluster, node.template               |
| inputs[].value | string | Yes | Value for the selector (e.g. nodeId, workloadId, workspaceId, clusterId)                                  |
| timeoutSecond | int | No | Timeout seconds; ≤0 means no timeout                                                                      |
| ttlSecondsAfterFinished | int | No | Job TTL after completion                                                                                  |
| excludedNodes | []string | No | Nodes to exclude from execution                                                                           |
| isTolerateAll | bool | No | Whether to tolerate node taints                                                                           |
| resource | object | Conditionally | Only for preflight; container resources, e.g. {cpu, gpu, memory, ephemeralStorage, sharedMemory, replica} |
| image | string | Conditionally | Only for preflight; container image                                                                       |
| entryPoint | string | Conditionally | Only for preflight; startup command (Base64)                                                              |
| env | object | Conditionally | Only for preflight; environment variables key-value                                                       |
| hostpath | []string | Conditionally | Only for preflight; host paths to mount                                                                   |
| batchCount | int | Conditionally | Only for addon; parallel nodes per batch (default 1)                                                      |
| availableRatio | float | Conditionally | Only for addon; success ratio threshold (default 1.0)                                                     |
| securityUpgrade | bool | Conditionally | Only for addon; wait until node idle(no workloads) before upgrade                                                      |

Notes:
- At least one scope selector must be provided via inputs: node/workload/workspace/cluster.
- For dumplog, inputs must include a workload selector.
- For addon, typically include `addon.template` and one of node/workload/workspace/cluster.
- For preflight, inputs must include one of node/workload/workspace/cluster.
- For exportimage, inputs must include a workload selector. The job will export the workload's image to Harbor registry.

**Response**: `{ "jobId": "opsjob-abc123" }`

---

### 2. List Ops Jobs

**Endpoint**: `GET /api/v1/opsjobs`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description                                                |
|-----------|------|----------|------------------------------------------------------------|
| offset | int | No | Pagination offset, default 0                               |
| limit | int | No | Records per page, default 100                              |
| sortBy | string | No | Sort field, default creationTime                           |
| order | string | No | Sort order: desc/asc, default desc                         |
| since | string | No | Start time (RFC3339 with milliseconds); default until-720h |
| until | string | No | End time (RFC3339 with milliseconds); default now          |
| clusterId | string | No | Filter by cluster ID                                       |
| userName | string | No | Filter by submitter username (fuzzy match)                 |
| phase | string | No | Filter by job status: Succeeded/Failed/Running/Pending     |
| type | string | No | Filter by job type: addon/dumplog/preflight/reboot/exportimage |

**Response Example**:
```json
{
  "totalCount": 5,
  "items": [
    {
      "jobId": "opsjob-addon-abc123",
      "jobName": "addon-upgrade",
      "clusterId": "prod-cluster",
      "workspaceId": "prod-cluster-ai-team",
      "userId": "user-001",
      "userName": "alice",
      "type": "addon",
      "phase": "Running",
      "creationTime": "2025-01-10T08:00:00",
      "startTime": "2025-01-10T08:05:00",
      "endTime": "",
      "deletionTime": "",
      "timeoutSecond": 0
    }
  ]
}
```

**Field Description**:

| Field | Type | Description                                         |
|-------|------|-----------------------------------------------------|
| totalCount | int | Total number of ops jobs, not limited by pagination |
| jobId | string | Job ID                                              |
| jobName | string | Job display name                                    |
| clusterId | string | The cluster which the job belongs to                |
| workspaceId | string | The workspace which the job belongs to              |
| userId | string | User ID of job submitter                            |
| userName | string | Username of job submitter                           |
| type | string | Job type: addon/dumplog/preflight/reboot/exportimage |
| phase | string | Job status: Succeeded/Failed/Running/Pending        |
| creationTime | string | Creation time (RFC3339)                             |
| startTime | string | Start time (RFC3339), empty if not started          |
| endTime | string | End time (RFC3339), empty if not finished           |
| deletionTime | string | Deletion time (RFC3339), empty if not deleted       |
| timeoutSecond | int | Timeout seconds (≤0 means no timeout)               |

---

### 3. Get Operational Job Details

**Endpoint**: `GET /api/v1/opsjobs/{JobId}`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "jobId": "opsjob-preflight-def456",
  "jobName": "preflight-check",
  "clusterId": "prod-cluster",
  "workspaceId": "prod-cluster-ai-team",
  "userId": "user-002",
  "userName": "bob",
  "type": "preflight",
  "phase": "Succeeded",
  "creationTime": "2025-01-09T12:00:00",
  "startTime": "2025-01-09T12:01:00",
  "endTime": "2025-01-09T12:10:00",
  "deletionTime": "",
  "timeoutSecond": 7200,
  "conditions": [
    {
      "type": "Running",
      "status": "True",
      "reason": "Started",
      "message": "job started",
      "lastTransitionTime": "2025-01-09T12:01:00Z"
    },
    {
      "type": "Succeeded",
      "status": "True",
      "reason": "Succeed",
      "message": "job finished successfully",
      "lastTransitionTime": "2025-01-09T12:10:00Z"
    }
  ],
  "inputs": [
    { "name": "cluster", "value": "prod-cluster" }
  ],
  "outputs": [
    { "name": "report", "value": "s3://bucket/preflight/report.json" }
  ],
  "env": { "CHECK_DISK": "true" },
  "resource": { "cpu": "8", "memory": "32Gi" },
  "image": "harbor.example.com/tools/preflight:latest",
  "entryPoint": "YmFzaCAtYyAnZWNobyAnJw==",
  "isTolerateAll": true,
  "hostpath": ["/var/log", "/nfs_models"]
}
```

**Field Description**:

Only fields not already covered by "List Ops Jobs" are listed below. Other fields share the same meaning as in the list response.

| Field | Type | Description                                                                                |
|-------|------|--------------------------------------------------------------------------------------------|
| conditions[] | object | Job condition history (k8s metav1.Condition)                                               |
| conditions[].type | string | Condition type, e.g. Running/Succeeded/Failed/Pending                                      |
| conditions[].status | string | Condition status: True/False/Unknown                                                       |
| conditions[].reason | string | Brief reason for the condition change                                                      |
| conditions[].message | string | Human-readable message                                                                     |
| conditions[].lastTransitionTime | string | Last transition time (RFC3339)                                                             |
| inputs[] | object | Job inputs (name/value), e.g. node/workload/workspace/cluster/addon.template/node.template |
| outputs[] | object | Job outputs (name/value), implementation-defined                                           |
| env | object | Environment variables key-value                                                            |
| resource | object | Preflight only: container resources used by the job                                        |
| image | string | Preflight only: container image                                                            |
| entryPoint | string | Preflight only: startup command (Base64)                                                   |
| isTolerateAll | bool | Whether the job tolerates node taints                                                      |
| hostpath | []string | Preflight only: host paths to mount                                                        |


---

### 4. Stop Operational Job

**Endpoint**: `POST /api/v1/opsjobs/{JobId}/stop`

**Authentication Required**: Yes

**Response**: 200 OK with no response body

---

### 5. Delete Operational Job

**Endpoint**: `DELETE /api/v1/opsjobs/{JobId}`

**Authentication Required**: Yes

**Response**: 200 OK with no response body