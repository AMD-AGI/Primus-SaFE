# Node API

## Overview

The Node APIs provide management capabilities for computing nodes within the system. Nodes are the fundamental compute resources that form clusters and host workloads. These APIs allow users to create, list, retrieve, update, and delete nodes, as well as manage node configurations.

## API List

### 1. Create Node

Register a new node to the system.

**Endpoint**: `POST /api/v1/nodes`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "hostname": "gpu-node-001",
  "privateIP": "192.168.1.100",
  "publicIP": "203.0.113.100",
  "port": 22,
  "labels": {
    "gpu-type": "MI300X",
    "datacenter": "us-west"
  },
  "flavorId": "gpu-large",
  "templateId": "ubuntu-gpu-template",
  "sshSecretId": "ssh-secret-001"
}
```

**Field Description**:

| Field | Type | Required | Description                                          |
|-------|------|----------|------------------------------------------------------|
| hostname | string | No | Node hostname, uses privateIP if not specified       |
| privateIP | string | Yes | Node private IP                                      |
| publicIP | string | No | Node public IP, accessible from external networks    |
| port | int | No | SSH port, default 22                                 |
| labels | object | No | Node labels,  Keys cannot start with "primus-safe"   |
| flavorId | string | Yes | Associated node flavor ID                            |
| templateId | string | Yes | Associated node template ID (for addon installation) |
| sshSecretId | string | Yes | SSH secret ID                                        |

**Response Example**:

```json
{
  "nodeId": "gpu-node-001-abc123"
}
```
**Field Description**:

| Field | Type | Description       |
|-------|------|-------------------|
| nodeId | string | Generated Node ID |

---

### 2. List Nodes

Get node list with multiple filtering options.

**Endpoint**: `GET /api/v1/nodes`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description                                                                              |
|-----------|------|----------|------------------------------------------------------------------------------------------|
| clusterId | string | No | Filter by cluster ID                                                                     |
| workspaceId | string | No | Filter by workspace ID                                                                   |
| flavorId | string | No | Filter by node flavor ID                                                                 |
| nodeId | string | No | Filter by node ID                                                                        |
| search | string | No | Search by node name or IP address (case-insensitive partial match)                       |
| available | bool | No | Filter by availability: true (available)/false (unavailable)                             |
| phase | string | No | Filter by status (comma-separated)                                                       |
| isAddonsInstalled | bool | No | Filter by addon installation status                                                      |
| brief | bool | No | Brief mode, returns only ID, name, IP,  availability, and unavailability reason (if any) |
| offset | int | No | Pagination offset, default 0                                                             |
| limit | int | No | Records per page, default 100, -1 for all                                                |

**Request Examples**:

```bash
# Search by node name (case-insensitive)
GET /api/v1/nodes?search=smc300x

# Search by IP address
GET /api/v1/nodes?search=35.192

# Combined filtering: search + status + availability
GET /api/v1/nodes?search=gpu-node&phase=Ready&available=true

# Combined filtering: search + cluster + workspace
GET /api/v1/nodes?search=test&clusterId=safe-cluster&workspaceId=ai-team
```

**Response Example (Full mode)**:

```json
{
  "totalCount": 20,
  "items": [
    {
      "nodeId": "gpu-node-001",
      "nodeName": "gpu-node-001",
      "internalIP": "192.168.1.100",
      "clusterId": "prod-cluster",
      "workspace": {
        "id": "prod-cluster-ai-team",
        "name": "ai-team"
      },
      "phase": "Ready",
      "available": true,
      "message": "",
      "totalResources": {
        "amd.com/gpu": 8,
        "cpu": 256,
        "ephemeral-storage": 6856267152295,
        "memory": 1622049488896,
        "rdma/hca": 1000
      },
      "availResources": {
        "amd.com/gpu": 8,
        "cpu": 230,
        "ephemeral-storage": 6513453794680,
        "memory": 1540947014451,
        "rdma/hca": 1000
      },
      "creationTime": "2025-01-10T10:00:00",
      "workloads": [
        {
          "id": "training-job-001",
          "kind": "PyTorchJob",
          "userId": "user-001",
          "workspaceId": "prod-cluster-ai-team"
        }
      ],
      "isControlPlane": false,
      "isAddonsInstalled": true
    }
  ]
}
```

**Response Example (Brief mode, brief=true)**:

```json
{
  "totalCount": 20,
  "items": [
    {
      "nodeId": "gpu-node-001-abc123",
      "nodeName": "gpu-node-001",
      "internalIP": "192.168.1.100",
      "available": false,
      "message": "Node is not ready"
    }
  ]
}
```
**Field Description (Full mode)**:

| Field                   | Type | Description                                                                                   |
|-------------------------|------|-----------------------------------------------------------------------------------------------|
| totalCount              | int | Total number of nodes, unaffected by pagination                                               |
| nodeId                  | string | Node ID                                                                                       |
| nodeName                | string | Node name                                                                                     |
| internalIP              | string | Node internal IP                                                                              |
| clusterId               | string | Cluster ID the node belongs to                                                                |
| workspace.id            | string | Workspace ID bound to the node                                                                |
| workspace.name          | string | Workspace display name                                                                        |
| phase                   | string | Node status: Ready/SSHFailed/HostnameFailed/Managing/ManagedFailed/Unmanaging/UnmanagedFailed |
| available               | bool | Whether the node is schedulable                                                               |
| message                 | string | Reason when unavailable (empty if available)                                                  |
| totalResources          | object | Total resources map (key:string → value:int64)                                                |
| availResources          | object | Available resources map with the same semantics as totalResources                             |
| creationTime            | string | Creation time (RFC3339Short)                                                                  |
| workloads[].id          | string | Running workload ID on this node                                                              |
| workloads[].userId      | string | Submitter user ID of the workload                                                             |
| workloads[].workspaceId | string | Workspace ID the workload belongs to                                                          |
| workloads[].kind        | string | Running workload Kind on this node                                                            |
| isControlPlane          | bool | Whether the node is a control-plane node                                                      |
| isAddonsInstalled       | bool | Whether addons from the node-template are installed                                           |



---

### 3. Get Node Details

Get detailed information about a specific node.

**Endpoint**: `GET /api/v1/nodes/{NodeId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| NodeId | Node ID |

**Response Example**:

```json
{
  "nodeId": "gpu-node-001",
  "nodeName": "gpu-node-001",
  "internalIP": "192.168.1.100",
  "clusterId": "prod-cluster",
  "workspace": {
    "id": "prod-cluster-ai-team",
    "name": "ai-team"
  },
  "phase": "Ready",
  "available": true,
  "message": "",
  "totalResources": {
    "amd.com/gpu": 8,
    "cpu": 256,
    "ephemeral-storage": 6856267152295,
    "memory": 1622049488896,
    "rdma/hca": 1000
  },
  "availResources": {
    "amd.com/gpu": 8,
    "cpu": 230,
    "ephemeral-storage": 6513453794680,
    "memory": 1540947014451,
    "rdma/hca": 1000
  },
  "creationTime": "2025-01-10T10:00:00",
  "workloads": [
    {
      "id": "training-job-001",
      "kind": "PyTorchJob",
      "userId": "user-001",
      "workspaceId": "prod-cluster-ai-team"
    }
  ],
  "isControlPlane": false,
  "isAddonsInstalled": true,
  "flavorId": "gpu-large",
  "templateId": "amd-gpu-template",
  "taints": [
    {
      "key": "test-taint",
      "effect": "NoSchedule"
    }
  ],
  "customerLabels": {
    "gpu-type": "A100",
    "datacenter": "us-west"
  },
  "lastStartupTime": "2025-01-10T10:05:00Z"
}
```
**Field Description**:

Only fields not already covered by "List Nodes" are listed below. Other fields share the same meaning as in the list response.

| Field | Type   | Description                                   |
|-------|--------|-----------------------------------------------|
| flavorId | string | Node flavor ID                                |
| templateId | string | Node template ID                              |
| taints | object | The taints on node                            |
| labels | object | The labels by customer                        |
| lastStartupTime | string    | The last startup time on node (RFC3339Short)  |

---

### 4. Update Node

Update node configuration.

**Endpoint**: `PATCH /api/v1/nodes/{NodeId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| NodeId | Node ID |

**Request Parameters**:

```json
{
  "taints": [
    {
      "key": "maintenance",
      "value": "true",
      "effect": "NoSchedule"
    }
  ],
  "labels": {
    "gpu-type": "A100",
    "updated": "true"
  },
  "flavorId": "gpu-xlarge",
  "templateId": "amd-gpu-template-v2",
  "privateIP": "192.168.1.101",
  "port": 2222
}
```

**Field Description**:

All fields are optional, only provided fields will be updated

| Field | Type   | Description           |
|-------|--------|-----------------------|
| taints | object | The taints on node      |
| labels | object | The labels by customer |
| flavorId | string | Node flavor ID        |
| templateId | string | Node template ID      |
| privateIP | string |  Node private IP       |
| port | int |SSH port |

**Response**: 200 OK with no response body

---

### 5. Delete Node

Delete the specific node.

**Endpoint**: `DELETE /api/v1/nodes/{NodeId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| NodeId | Node ID |

**Prerequisites**: Node is not bound to any cluster

**Response**: 200 OK with no response body

---

### 6. Reboot Node

Reboot a specific node. This operation creates a reboot-type OpsJob to perform the reboot operation.

**Endpoint**: `POST /api/v1/opsjobs`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "name": "reboot-node",
  "type": "reboot",
  "inputs": [
    {
      "name": "node",
      "value": "gpu-node-001"
    }
  ],
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Job name (used to generate job ID with random suffix) |
| type | string | Yes | Must be "reboot" for node reboot operations |
| inputs[].name | string | Yes | Must be "node" |
| inputs[].value | string | Yes | Node ID to reboot |

**Response Example**:

```json
{
  "jobId": "reboot-node-abc123"
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| jobId | string | Generated OpsJob ID for tracking the reboot operation |

**Note**:
- Node reboot is an asynchronous operation. Use the returned jobId to track the operation status via OpsJob APIs.
- Refer to [OpsJob API](ops-job.md) for more details on tracking and managing the reboot operation.

---

### 7. List Node Reboot Logs

Get historical reboot logs for a specific node.

**Endpoint**: `GET /api/v1/nodes/{NodeId}/reboot/logs`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| NodeId | Node ID |

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| sinceTime | string | No | Start time filter (RFC3339 format) |
| untilTime | string | No | End time filter (RFC3339 format) |
| offset | int | No | Pagination offset, default 0 |
| limit | int | No | Records per page, default 100, -1 for all |
| sortBy | string | No | Sort by field, default create_time |
| order | string | No | Sort order: desc (default) or asc |

**Response Example**:

```json
{
  "totalCount": 10,
  "items": [
    {
      "userId": "user-001",
      "userName": "admin",
      "createTime": "2025-01-10T10:00:00Z"
    },
    {
      "userId": "user-002",
      "userName": "operator",
      "createTime": "2025-01-09T15:30:00Z"
    }
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of reboot logs, unaffected by pagination |
| items[].userId | string | User ID who initiated the reboot |
| items[].userName | string | Username who initiated the reboot |
| items[].createTime | string | Reboot operation creation time (RFC3339) |

---

### 8. Get Node Management Logs

Get operation logs for node joining/leaving cluster.

**Endpoint**: `GET /api/v1/nodes/{NodeId}/logs`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| NodeId | Node ID |

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| tailLines | int | No | Return last N lines of logs, default 1000 |
| sinceSeconds | int | No | Return logs from last N seconds |

**Response Example**:

```json
{
  "clusterId": "prod-cluster",
  "nodeId": "gpu-node-001-abc123",
  "podId": "node-manage-pod-xyz789",
  "logs": [
    "2025-01-10 10:00:00 INFO Starting node join process...",
    "2025-01-10 10:01:00 INFO Installing kubeadm...",
    "2025-01-10 10:05:00 INFO Node successfully joined cluster"
  ]
}
```

### 9. Batch Delete Nodes

Delete multiple nodes in batch.

**Endpoint**: `POST /api/v1/nodes/delete`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "nodeIds": [
    "node-001",
    "node-002"
  ]
}
```

**Response**: 200 OK with no response body

---

### 10. Export Node

Export node list with multiple filtering options.

**Endpoint**: `GET /api/v1/nodes/export`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| NodeId | Node ID |

**Query Parameters**:

| Parameter | Type | Required | Description                                                                              |
|-----------|------|----------|------------------------------------------------------------------------------------------|
| clusterId | string | No | Filter by cluster ID                                                                     |
| workspaceId | string | No | Filter by workspace ID                                                                   |
| flavorId | string | No | Filter by node flavor ID                                                                 |
| nodeId | string | No | Filter by node ID                                                                        |
| available | bool | No | Filter by availability: true (available)/false (unavailable)                             |
| phase | string | No | Filter by status (comma-separated)                                                       |
| isAddonsInstalled | bool | No | Filter by addon installation status                                                      |

**Response**: 200 OK with no response body

---

### 11. Retry Node Operations

Retry node manage or unmanage operations when needed.

**Endpoint**: `POST /api/v1/nodes/retry`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "nodeIds": ["node-001", "node-002"]
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| nodeIds | string[] | Yes | List of node IDs to retry (supports single or multiple nodes) |

**Response Example**:

```json
{
  "totalCount": 2,
  "successCount": 2,
  "successNodes": [
    {
      "nodeId": "node-001",
      "hasPods": true,
      "podsDeleted": ["safe-cluster-node-001-up"]
    },
    {
      "nodeId": "node-002",
      "hasPods": false
    }
  ],
  "failedNodes": null
}
```

**Response Fields**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of nodes requested |
| successCount | int | Number of nodes successfully processed |
| successNodes | array | Details of successfully processed nodes |
| successNodes[].nodeId | string | Node ID |
| successNodes[].hasPods | bool | Whether management pods were found |
| successNodes[].podsDeleted | string[] | Names of deleted pods (omitted if hasPods is false) |
| failedNodes | array | Details of failed nodes (null if all succeeded) |

---
## Node Status

| Status | Description             |
|--------|-------------------------|
| Ready | Ready                   |
| SSHFailed | SSH connection failed   |
| HostnameFailed | Hostname setup failed   |
| Managing | Joining cluster         |
| ManagedFailed | Failed to join cluster  |
| Unmanaging | Leaving cluster         |
| UnmanagedFailed | Failed to leave cluster |

## Taints

Taints are used to control Pod scheduling. Common Effect types:

| Effect | Description |
|--------|-------------|
| NoSchedule | Do not allow scheduling new Pods |
| PreferNoSchedule | Try not to schedule new Pods |
| NoExecute | Do not allow scheduling and evict existing Pods |

**Note**: System automatically adds `primus-safe.` prefix to taint keys

## Resource Statistics

- **totalResources**: Total node resources (defined by node flavor)
- **availResources**: Available resources (total resources - used resources)
- **workloads**: List of workloads currently running on this node

## Notes

1. **Node Registration**: Node must be SSH accessible and meet system requirements
2. **Node Flavor**: Not recommended to modify after creation, may cause inaccurate resource statistics
3. **Node Template**: Defines node's software environment and addons
4. **Control Plane Nodes**: Cannot be deleted or have cluster binding changed
5. **Node Labels**: Custom labels cannot use `primus-safe.amd.com/` prefix
6. **Deletion Restrictions**: Node must be removed from cluster before deletion
