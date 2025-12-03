# Workload API

The Workload API is a core set of interfaces for managing workloads, enabling users to create, manage, monitor, and operate various types of computing tasks. These APIs support multiple workload types, including machine learning training jobs (PyTorchJob), deployed services (Deployment), stateful applications (StatefulSet), and development machines (Authoring).


## API List

### 1. Create Workload

Create a new workload.

**Endpoint**: `POST /api/v1/workloads`

**Authentication Required**: Yes

***PytorchJob Request Example***:
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
  "specifiedNodes": [],
  "isSupervised": false,
  "ttlSecondsAfterFinished": 60,
  "customerLabels": {
    "key": "val"
  },
  "liveness": {
    "path": "/status",
    "port": 8088,
    "initialDelaySeconds": 600,
    "periodSeconds": 3,
    "failureThreshold": 3
  },
  "service": {
    "protocol": "TCP",
    "port": 8080,
    "nodePort": 12345,
    "targetPort": 8088,
    "serviceType": "NodePort",
    "extends": {}
  },
  "dependencies": ["my-pre-training-job"],
  "cronJobs": [
    {
      "schedule": "2025-09-30T16:04:00.000Z",
      "action": "start"
    }
  ],
  "secrets": [
    {
      "id": "test-secret-id1",
      "type": "image"
    }
  ],
  "isTolerateAll": false
}
```

***CICD (AutoscalingRunnerSet) Request Example***:

```json
{
  "workspaceId": "cluster-workspace",
  "displayName": "test-cicd",
  "groupVersionKind": {
    "kind": "AutoscalingRunnerSet",
    "version": "v1"
  },
  "entryPoint": "bash run.sh",
  "image": "primussafe/buildah-runner:v2.329.0-3",
  "resource": {
    "replica": 1,
    "cpu": "8",
    "memory": "16Gi",
    "ephemeralStorage": "100Gi"
  },
  "env": {
    "GITHUB_CONFIG_URL": "https://github.com/AMD-AGI/Primus-SaFE",
    "GITHUB_PAT": "your token"
  }
}
```

Notes for CICD (AutoscalingRunnerSet):
- Only GitHub is supported for CICD integration at this time. Other providers are not supported.
- Required env: `GITHUB_CONFIG_URL` must be set in `env` to the GitHub repository/organization URL.
- Required env: `GITHUB_PAT` must be provided in `env` (GitHub Personal Access Token). The system will automatically create a secret (with key `github_token`) from this PAT and attach it to the workload(Its lifecycle is also controlled by the workload)
- Multi-node evaluation: set `"UNIFIED_JOB_ENABLE": "true"` in `env` to enable multi-node evaluation in CICD.
- Required NFS storage: CICD workloads require NFS storage support enabled in the workspace. This is especially important when `UNIFIED_JOB_ENABLE` is set to `true` in the environment variables for multi-node evaluation scenarios.


**Field Description**:

| Field                        | Type | Required | Description                                                                                                                              |
|------------------------------|------|----------|------------------------------------------------------------------------------------------------------------------------------------------|
| displayName                  | string | Yes      | Workload display name                                                                                                                    |
| description                  | string | No       | Workload description                                                                                                                     |
| workspaceId                  | string | Yes      | Workspace ID                                                                                                                             |
| groupVersionKind.kind        | string | Yes      | Workload type: PyTorchJob/Deployment/StatefulSet/Authoring/AutoscalingRunnerSet                                                          |
| groupVersionKind.version     | string | Yes      | Version, usually v1                                                                                                                      |
| image                        | string | Yes      | Image address                                                                                                                            |
| entryPoint                   | string | Yes      | Startup command/script (Base64 encoded)                                                                                                  |
| resource.cpu                 | string | Yes      | Number of CPU cores                                                                                                                      |
| resource.gpu                 | string | No       | Number of GPU cards                                                                                                                      |
| resource.memory              | string | Yes      | Memory size, e.g. "256Gi"                                                                                                                |
| resource.replica             | int | Yes      | Number of replicas                                                                                                                       |
| priority                     | int | No       | Priority (0-2), default 0                                                                                                                |
| timeout                      | int | No       | Timeout in seconds, 0 means no timeout                                                                                                   |
| maxRetry                     | int | No       | Maximum retry count, default 0                                                                                                           |
| env                          | object | No       | Environment variable key-value pairs                                                                                                     |
| specifiedNodes               | []string | No       | List of specified nodes to run on                                                                                                        |
| isSupervised                 | bool | No       | When enabled, it performs operations like hang detection                                                                                 |
| ttlSecondsAfterFinished      | int | No       | The lifecycle of the workload after completion, in seconds. Default is 60                                                                |
| customerLabels               | object | No       | The workload will run on nodes with the user-specified labels                                                                            |
| liveness.path                | string | No       | Liveness probe HTTP path                                                                                                                 |
| liveness.port                | int | No       | Liveness probe port                                                                                                                      |
| liveness.initialDelaySeconds | int | No       | Liveness initial delay seconds, default 600                                                                                              |
| liveness.periodSeconds       | int | No       | Liveness check period seconds, default 3                                                                                                 |
| liveness.failureThreshold    | int | No       | Liveness failure threshold, default 3                                                                                                    |
| service.protocol             | string | No       | Service protocol, e.g. TCP/UDP, default TCP                                                                                              |
| service.port                 | int | No       | Service port for external access                                                                                                         |
| service.nodePort             | int | No       | Service NodePort (for NodePort type)                                                                                                     |
| service.targetPort           | int | No       | Target container port                                                                                                                    |
| service.serviceType          | string | No       | Service type, e.g. ClusterIP/NodePort/LoadBalancer                                                                                       |
| service.extends              | object | No       | Additional service fields (advanced)                                                                                                     |
| dependencies                 | []string | No       | Dependent workload IDs that must complete first                                                                                          |
| cronJobs[].schedule          | string | No       | Scheduled trigger time (RFC3339 Milli timestamp)                                                                                         |
| cronJobs[].action            | string | No       | Action to perform, e.g. start                                                                                                            |
 | secrets                     | []object | No       | Secrets automatically use all image secrets bound to the workspace.  You can also define your own Secret, such as a token used for CI/CD |
 | secrets[].id                 | string | Yes      | Secret ID                                                                                                                                |
 | isTolerateAll                | bool | No       | Whether to tolerate all node taints                                                                                                      |

 

**Response Example**:

```json
{
  "workloadId": "my-training-job-abc12"
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| workloadId | string | Generated workload ID |

---

### 2. List Workloads

Get workload list with filtering and pagination support.

**Endpoint**: `GET /api/v1/workloads`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description                                                                                                                    |
|-----------|------|----------|--------------------------------------------------------------------------------------------------------------------------------|
| workspaceId | string | No | Filter by workspace ID                                                                                                         |
| clusterId | string | No | Filter by cluster ID                                                                                                           |
| userId | string | No | Filter by user ID                                                                                                              |
| userName | string | No | Filter by username (fuzzy match)                                                                                               |
| phase | string | No | Filter by status: Succeeded/Failed/Pending/Running/Stopped/Updating/NotReady (comma-separated)                                 |
| kind | string | No | Filter by type: Deployment/PyTorchJob/StatefulSet/Authoring/AutoscalingRunnerSet (comma-separated)                             |
| description | string | No | Filter by description (fuzzy match)                                                                                            |
| workloadId | string | No | Filter by workload ID (fuzzy match)                                                                                            |
| since | string | No | Start time, RFC3339 Milli format: 2006-01-02T15:04:05.000Z                                                                     |
| until | string | No | End time, similar to since                                                                                                     |
| offset | int | No | Pagination offset, default 0                                                                                                   |
| limit | int | No | Records per page, default 100                                                                                                  |
| sortBy | string | No | Sort field, default creationTime                                                                                               |
| order | string | No | Sort order: desc/asc, default desc                                                                                             |
| scaleRunnerSet | string | No | Filter by Scale Runner Set ID. This is the ID of the CICD-created AutoscalingRunnerSet; lists all workloads associated with it |

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
        "ephemeralStorage": "256Gi",
        "sharedMemory": "64Gi",
        "replica": 1
      },
      "groupVersionKind": {
        "kind": "PyTorchJob",
        "version": "v1"
      },
      "creationTime": "2025-01-15T10:30:00",
      "startTime": "2025-01-15T10:31:00",
      "endTime": "",
      "deletionTime": "",
      "duration": "1h30m45s",
      "queuePosition": 0,
      "dispatchCount": 1,
      "isTolerateAll": false,
      "timeout": 3600,
      "secondsUntilTimeout": 1800,
      "message": "",
      "avgGpuUsage": 22.13
    }
  ]
}
```
**Field Description**:

| Field | Type | Description                                                                     |
|-------|------|---------------------------------------------------------------------------------|
| totalCount | int | Total number of workloads matching the query (not limited by pagination)        |
| workloadId | string | Workload ID                                                                     |
| displayName | string | Workload display name                                                           |
| description | string | Workload description                                                            |
| workspaceId | string | Workspace the workload belongs to                                               |
| clusterId | string | Cluster the workload belongs to                                                 |
| userId | string | ID of the user who submitted the workload                                       |
| userName | string | Username of the submitter                                                       |
| phase | string | Status: Pending/Running/Succeeded/Failed/Stopped/Updating/NotReady                      |
| message | string | Pending reason (shown when applicable)                                          |
| priority | int | Scheduling priority (0-2), default 0                                            |
| creationTime | string | Creation time (RFC3339), e.g. "2025-01-15T10:30:00"                             |
| startTime | string | Start time (RFC3339)                                                            |
| endTime | string | End time (RFC3339), empty if not finished                                       |
| deletionTime | string | Deletion time (RFC3339), empty if not deleted                                   |
| duration | string | Human-readable duration from start to end (or now), e.g. "1h30m45s"             |
| secondsUntilTimeout | int | Seconds until timeout from start; -1 if not started                             |
| queuePosition | int | Queue position when workload is pending                                         |
| dispatchCount | int | Number of dispatch attempts                                                     |
| isTolerateAll | bool | Whether to tolerate all node taints                                             |
| groupVersionKind.kind | string | Workload type: PyTorchJob/Deployment/StatefulSet/Authoring/AutoscalingRunnerSet |
| groupVersionKind.version | string | API version (usually v1)                                                        |
| timeout | int | Timeout seconds (0 means no timeout)                                            |
| workloadUid | string | Workload UID                                                                    |
| k8sObjectUid | string | Corresponding Kubernetes object UID (e.g., PyTorchJob UID)                      |
| avgGpuUsage | float | Average GPU usage in the last 3 hours; -1 if unavailable                        |
| scaleRunnerSet | string | Associated Scale Runner Set ID for CI/CD workloads (if any)                     |
| resource.cpu | string | CPU cores, e.g. "128"                                                           |
| resource.gpu | string | GPU cards, e.g. "8"                                                             |
| resource.memory | string | Memory size, e.g. "256Gi"                                                       |
| resource.ephemeralStorage | string | Ephemeral storage size, e.g. "256Gi"                                            |
| resource.sharedMemory | string | Shared memory size, e.g. "64Gi"                                                 |
| resource.replica | int | Replica count                                                                   |
---

### 3. Get Workload Details

Get detailed information about a specific workload.

**Endpoint**: `GET /api/v1/workloads/{WorkloadId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkloadId | Workload ID |

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
  "creationTime": "2025-01-15T10:30:00",
  "startTime": "2025-01-15T10:31:00",
  "endTime": "",
  "duration": "1h30m45s",
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
  "specifiedNodes": [],
  "cronJobs": [
    {
      "schedule": "2025-10-26T10:41:00.000Z",
      "action": "start"
    }
  ],
  "secrets": [
    {
      "id": "test-secret-id1",
      "type": "image"
    }, {
      "id": "test-secret-id2",
      "type": "general"
    }
  ],
  "workloadUid": "a8e357ad-f73d-43ac-99fe-118886d5e193",
  "k8sObjectUid": "f89f34e5-82da-49d7-9b89-0b2af523bc5a"
}
```

**Field Description**:

Only fields not already covered by "List Workloads" are listed below. Other fields share the same meaning as in the list response.

| Field | Type      | Description                                                                                                                             |
|-------|-----------|-----------------------------------------------------------------------------------------------------------------------------------------|
| image | string    | Image address used by the workload                                                                                                      |
| entryPoint | string    | Startup command/script (in base64 encoding)                                                                                             |
| isSupervised | bool      | When enabled, it performs operations like hang detection                                                                                |
| maxRetry | int       | Failure retry limit. default 0                                                                                                          |
| ttlSecondsAfterFinished | int       | The lifecycle after completion, in seconds, default 60.                                                                                 |
| env | object    | Environment variables key-value pairs                                                                                                   |
| conditions[].type | string    | Condition type, e.g. AdminScheduled/AdminDispatched/K8sPending/K8sSucceeded/K8sFailed/K8sRunning/AdminFailover/AdminFailed/AdminStopped |
| conditions[].status | string    | Condition status: True/False/Unknown                                                                                                    |
| conditions[].lastTransitionTime | string    | Last transition time of the condition                                                                                                   |
| conditions[].reason | string    | Dispatch count                                                                                                                          |
| conditions[].message | string    | Human-readable message for the condition                                                                                                |
| pods[].podId | string    | Pod ID of a workload pod                                                                                                                |
| pods[].phase | string    | Pod phase, e.g. Pending/Running/Succeeded/Failed                                                                                        |
| pods[].k8sNodeName | string    | The Kubernetes node that the Pod is scheduled on                                                                                        |
| pods[].adminNodeName | string    | The Admin Node name where the pod is scheduled on                                                                                       |
| pods[].sshAddr | string    | SSH address for direct login into the container                                                                                         |
| pods[].startTime | string    | Pod start time                                                                                                                          |
| pods[].endTime | string    | Pod end time                                                                                                                            |
| pods[].hostIP | string    | The node IP address where the Pod is running                                                                                            |
| pods[].podIP | string    | The pod IP address where the Pod is running                                                                                             |
| pods[].rank | string    | The rank of pod, only for pytorch-job                                                                                                   |
| pods[].containers[].name | string    | Container name                                                                                                                          |
| pods[].containers[].reason | string    | (brief) reason from the last termination of the container                                                                               |
| pods[].containers[].message | string    | Message regarding the last termination of the container                                                                                 |
| pods[].containers[].exitCode | int32     | Exit status from the last termination of the container                                                                                  |
| nodes | [][]string | The node used for each workload execution, e.g. [["node-001"]]                                                                          |
| ranks | [][]string | The rank is only valid for the PyTorch job and corresponds one-to-one with the nodes listed above, e.g. [["0"]]                         |
| customerLabels | object    | Custom labels associated with the workload                                                                                              |
| specifiedNodes | []string  | The nodes explicitly specified to run on                                                                                                |
| liveness | object    | Refer to the CreateWorkload parameter                                                                                                   |
| readiness | object    | Refer to the CreateWorkload parameter                                                                                                   |
| service | object    | Refer to the CreateWorkload parameter                                                                                                   |
| dependencies | object    | Refer to the CreateWorkload parameter                                                                                                   |
| cronJobs | object    | Refer to the CreateWorkload parameter                                                                                                   |
| secrets  | object  | Refer to the CreateWorkload parameter                                                                                                   |
| workloadUid | string    | UID of the workload                                                                                                                     |
| k8sObjectUid | string    | K8s object UID corresponding to the workload. e.g. Associated PyTorchJob UID                                                            |

> Other fields not listed here are identical to those in the "List Workloads" Field Description.

---

### 4. Update Workload

Partially update workload configuration (only when running).

**Endpoint**: `PATCH /api/v1/workloads/{WorkloadId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkloadId | Workload ID |

**Request Parameters**:

```json
{
  "priority": 1,
  "replica": 2,
  "cpu": "256",
  "gpu": "16",
  "memory": "512Gi",
  "ephemeralStorage": "512Gi",
  "sharedMemory": "64Gi",
  "image": "harbor.example.com/ai/pytorch:2.1",
  "entryPoint": "YmFzaCBydW4uc2gK",
  "description": "New description",
  "timeout": 7200,
  "maxRetry": 3,
  "env": {
    "NEW_VAR": "value"
  },
  "cronJobs": [
    {
      "schedule": "2025-09-30T16:04:00.000Z",
      "action": "start"
    }
  ]
}
```

**Field Description**: All fields are optional; only provided fields will be updated

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| priority | int | No | Scheduling priority (0-2); unchanged if omitted |
| replica | int | No | Desired replica count; cannot change when nodes are specified |
| cpu | string | No | CPU cores, e.g. "256" |
| gpu | string | No | GPU cards, e.g. "16" |
| memory | string | No | Memory size, e.g. "512Gi" |
| ephemeralStorage | string | No | Ephemeral storage size, e.g. "512Gi" |
| sharedMemory | string | No | Shared memory size, e.g. "64Gi" |
| image | string | No | Image address; non-empty string required if provided |
| entryPoint | string | No | Startup command/script (Base64 encoded) |
| description | string | No | Workload description |
| timeout | int | No | Timeout in seconds; 0 means no timeout |
| maxRetry | int | No | Failure retry limit |
| env | object | No | Environment variable key-value pairs |
| cronJobs[].schedule | string | No | Scheduled trigger time (RFC3339), e.g. "2025-09-30T16:04:00.000Z" |
| cronJobs[].action | string | No | Action to perform, e.g. "start" |

**Response**: 200 OK with no response body

---

### 5. Delete Workload

Delete a specific workload.

**Endpoint**: `DELETE /api/v1/workloads/{WorkloadId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkloadId | Workload ID |

**Response**: 200 OK with no response body

---

### 6. Stop Workload

Stop a running workload.

**Endpoint**: `POST /api/custom/workloads/{WorkloadId}/stop`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkloadId | Workload ID |

**Response**: 200 OK with no response body

---

### 7. Batch Delete Workloads

Delete multiple workloads in batch.

**Endpoint**: `POST /api/v1/workloads/delete`

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

**Response**: 200 OK with no response body

---

### 8. Batch Stop Workloads

Stop multiple workloads in batch.

**Endpoint**: `POST /api/v1/workloads/stop`

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

**Response**: 200 OK with no response body

---

### 9. Clone Workloads

Clone existing workloads in batch.

**Endpoint**: `POST /api/v1/workloads/clone`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "workloadIds": [
    "workload-001"
  ]
}
```

**Response**: 200 OK with no response body

---

### 10. Get Workload Pod Logs

Get logs from a specific pod of the workload.

**Endpoint**: `GET /api/custom/workloads/{WorkloadId}/pods/{PodId}/logs`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkloadId | Workload ID |
| PodId | Pod ID |

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

**Endpoint**: `GET /api/custom/workloads/{WorkloadId}/pods/{PodId}/containers`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkloadId | Workload ID |
| PodId | Pod ID |

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
**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| containers[].name | string | Container name in the Pod |
| shells | []string | Supported interactive shells for exec |

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
    "port": {
        "protocol": "TCP",
        "port": 8080,
        "targetPort": 8080,
        "nodePort": 12345
    },
    "externalDomain": "http://tas.primus-safe.amd.com/safe-cluster/safe-cluster-dev/test-infer-htmqc",
    "internalDomain": "test-infer-htmqc.safe-cluster-dev.svc.cluster.local:8080",
    "clusterIp": "10.254.205.115",
    "type": "ClusterIP"
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| port | object | Kubernetes Service port info (protocol, port, targetPort). |
| externalDomain | string | Public URL via Higress when enabled and system host is set; empty otherwise. |
| internalDomain | string | In-cluster DNS address of the Service with port (service.namespace.svc.cluster.local:port). |
| clusterIp | string | ClusterIP assigned to the Service; empty for headless/None. |
| type | string | Service type: ClusterIP, NodePort. |

Port fields:
| Field | Type | Description |
|-------|------|-------------|
| protocol | string | Protocol of the service port (e.g., TCP/UDP). |
| port | integer | Service port exposed by the Service. |
| targetPort | integer | Container port targeted by the Service. |
| nodePort | integer | Node port allocated when Service type is NodePort; 0 or omitted otherwise. |

---

## Workload Status

| Status | Description                                                     |
|--------|-----------------------------------------------------------------|
| Pending | Waiting for scheduling                                          |
| Running | Currently running                                               |
| Succeeded | Completed successfully                                          |
| Failed | Execution failed                                                |
| Stopped | Stopped                                                         |
| Updating | Being updated, only for Deployment/StatefulSet                                                 |
| NotReady | not ready, only for Deployment/StatefulSet/AutoscalingRunnerSet |


## Workload Types

| Type | Description | Use Case |
|------|-------------|----------|
| PyTorchJob | PyTorch distributed training | Deep learning training |
| Deployment | K8s stateless deployment | Inference service |
| StatefulSet | K8s stateful deployment | Stateful service |
| Authoring | Development environment | Interactive development |
| AutoscalingRunnerSet | GitHub Actions autoscaling runner set | CI/CD runners |

## Notes

1. **EntryPoint Encoding**: `entryPoint` field must be Base64 encoded
2. **Node Specification**: When `specifiedNodes` is set, `replica` will be automatically set to the number of nodes
3. **Resource Units**: CPU is in cores, memory format like "256Gi"
4. **Priority**: 0 is low, 1 is med, 2 is high (requires appropriate permissions)
5. **Timeout Setting**: timeout of 0 means no timeout, otherwise in seconds
6. **Secrets**: User-defined secrets: image-type secrets are added to k8s imagePullSecrets, and default-type secrets are mounted under /etc/secrets/{secret.id}.
