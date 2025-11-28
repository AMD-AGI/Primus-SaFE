# Workspace API

## Overview

The Workspace API provides comprehensive management capabilities for workspace resources. A workspace is a core concept in the system, used to organize and isolate computing resources, user permissions, and workloads. Each workspace is associated with a specific cluster and defines a set of resource quotas, access policies, and service scopes.

These APIs allow users to create, list, retrieve, update, and delete workspaces, as well as manage nodes associated with specific workspaces.

Note: Each workload is submitted to a specified workspace.

## API List

### 1. Create Workspace

Create a new workspace in a cluster.

**Endpoint**: `POST /api/v1/workspaces`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "name": "ai-team",
  "clusterId": "prod-cluster",
  "description": "AI team workspace",
  "flavorId": "gpu-large",
  "replica": 4,
  "queuePolicy": "fifo",
  "scopes": ["Train", "Infer", "Authoring"],
  "volumes": [
    {
      "accessMode": "ReadWriteMany",
      "capacity": "100Ti",
      "mountPath": "/mnt/data",
      "selector": {
        "pfs-name": "test-pv"
      },
      "storageClass": "rbd",
      "type": "pfs"
    }, {
      "accessMode": "ReadWriteMany",
      "hostPath": "/home",
      "mountPath": "/home",
      "type": "hostpath"
    }
  ],
  "enablePreempt": false,
  "isDefault": true,
  "imageSecretIds": ["image-secret-001"]
}
```

**Field Description**:

| Field | Type | Required | Description                                                                                                                              |
|-------|------|----------|------------------------------------------------------------------------------------------------------------------------------------------|
| name | string | Yes | Workspace name                                                                                                                           |
| clusterId | string | Yes | The cluster which workspace belongs to                                                                                                   |
| description | string | No | Workspace description                                                                                                                    |
| flavorId | string | Yes | Node flavor ID                                                                                                                           |
| replica | int | No | Expected number of nodes                                                                                                                 |
| queuePolicy | string | No | Queue policy: fifo (first-in-first-out)/balance (load balancing), default fifo                                                           |
| scopes | []string | No | Supported service modules: Train/Infer/Authoring/CICD, no limitation if not specified                                                    |
| volumes | []object | No | Storage volume configuration list                                                                                                        |
| enablePreempt | bool | No | Whether to enable preemption, default false.  If enabled, higher-priority workload will preempt the lower-priority one in this workspace |
| isDefault | bool | No | Whether to set as default workspace (accessible to all users)                                                                            |
| imageSecretIds | []string | No | List of image pull secret IDs                                                                                                            |

**Volume Configuration**:

| Field | Type   | Required | Description                                                                                                  |
|-------|--------|----------|--------------------------------------------------------------------------------------------------------------|
| type | string | Yes | Volume type: pfs/hostpath. If pfs is configured, a PVC will be automatically created in the workspace.       |
| mountPath | string | Yes | Mount path to be used, equivalent to 'mountPath' in Kubernetes volume mounts                                 |
| hostPath | string | No | Path on the host to mount (required when type=hostpath)                                                      |
| accessMode | string | No | Access mode, default ReadWriteMany                                                                           |
| capacity | string | No | Capacity size, such as 100Gi. This is a required parameter when creating a PVC (type=pfs)                    |
| selector | object | No | Selector is a label query over volumes to consider for binding. It cannot be used together with storageClass |
| storageClass | string | No | esponsible for automatic PV creation                                                                         |

**Response Example**:

```json
{
  "workspaceId": "prod-cluster-ai-team"
}
```

**Field Description**:

| Field | Type | Description            |
|-------|------|------------------------|
| workspaceId | string | Generated workspace ID |


---

### 2. List Workspaces

Get workspace list with cluster filtering support.

**Endpoint**: `GET /api/v1/workspaces`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| clusterId | string | No | Filter by cluster ID |

**Response Example**:

```json
{
  "totalCount": 5,
  "items": [
    {
      "workspaceId": "prod-cluster-ai-team",
      "workspaceName": "ai-team",
      "clusterId": "prod-cluster",
      "flavorId": "gpu-large",
      "userId": "user-001",
      "targetNodeCount": 4,
      "currentNodeCount": 4,
      "abnormalNodeCount": 0,
      "phase": "Running",
      "creationTime": "2025-01-12T09:00:00",
      "description": "AI team workspace",
      "queuePolicy": "fifo",
      "scopes": ["Train", "Infer", "Authoring"],
      "volumes": [
        {
          "accessMode": "ReadWriteMany",
          "capacity": "100Ti",
          "mountPath": "/mnt/data",
          "storageClass": "rbd",
          "type": "pfs"
        }, {
          "accessMode": "ReadWriteMany",
          "hostPath": "/home",
          "mountPath": "/home",
          "type": "hostpath"
        }
      ],
      "enablePreempt": false,
      "managers": [
        {
          "id": "user-001",
          "name": "zhangsan"
        }
      ],
      "isDefault": false
    }
  ]
}
```

**Field Description**:

Only fields not already covered by "Create Workspace" are listed below. Other fields share the same meaning as in the creation request.


| Field         | Type  | Description                                                         |
|---------------|-------|---------------------------------------------------------------------|
| totalCount    | int   | The total number of workspaces                                      |
| workspaceId   | string | Workspace ID                                                        |
| workspaceName | string | Workspace name                                                      |
| userId        | string | User id of workspace creator                                        |
| targetNodeCount | int   | The target expected number of nodes in workspace                    |
| currentNodeCount | int   | The current total number of nodes                                   |
| abnormalNodeCount | int   | The current total number of abnormal nodes                          |
| phase    | string | The status of workspace, e.g. Creating, Running, Abnormal, Deleting |
| creationTime  | string | The workspace creation time                                         |

---

### 3. Get Workspace Details

Get detailed information about a specific workspace, including resource quotas.

**Endpoint**: `GET /api/v1/workspaces/{WorkspaceId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkspaceId | Workspace ID |

**Response Example**:

```json
{
  "workspaceId": "prod-cluster-ai-team",
  "workspaceName": "ai-team",
  "clusterId": "prod-cluster",
  "flavorId": "gpu-large",
  "userId": "user-001",
  "targetNodeCount": 4,
  "currentNodeCount": 4,
  "abnormalNodeCount": 0,
  "phase": "Running",
  "creationTime": "2025-01-12T09:00:00",
  "description": "AI team workspace",
  "queuePolicy": "fifo",
  "scopes": ["Train", "Infer", "Authoring"],
  "volumes": [
    {
      "accessMode": "ReadWriteMany",
      "capacity": "100Ti",
      "mountPath": "/mnt/data",
      "storageClass": "rbd",
      "type": "pfs"
    }, {
      "accessMode": "ReadWriteMany",
      "hostPath": "/home",
      "mountPath": "/home",
      "type": "hostpath"
    }
  ],
  "enablePreempt": false,
  "managers": [
    {
      "id": "user-001",
      "name": "zhangsan"
    }
  ],
  "isDefault": false,
  "totalQuota": {
    "amd.com/gpu": 1024,
    "cpu": 12288,
    "ephemeral-storage": 1407374883553280,
    "memory": 140737488355328,
    "rdma/hca": 128000
  },
  "availQuota": {
    "amd.com/gpu": 512,
    "cpu": 6144,
    "ephemeral-storage": 703687441776640,
    "memory": 70368744177664,
    "rdma/hca": 64000
  },
  "usedQuota": {
    "amd.com/gpu": 512,
    "cpu": 6144,
    "ephemeral-storage": 703687441776640,
    "memory": 70368744177664,
    "rdma/hca": 64000
  },
  "abnormalQuota": {
    "amd.com/gpu": 0,
    "cpu": 0,
    "ephemeral-storage": 0,
    "memory": 0,
    "rdma/hca": 0
  },
  "usedNodeCount": 64,
  "imageSecretIds": ["image-secret-001"]
}
```

**Field Description**:

Only fields not already covered by "List Workspace" are listed below. Other fields share the same meaning as in the creation request or list response.


| Field         | Type   | Description                                                           |
|---------------|--------|-----------------------------------------------------------------------|
| totalQuota    | object | Total resources in the workspace: resource names and their quantities |
| availQuota   | object | The available resource of workspace                                   |
| abnormalQuota | object | The abnormal resources of workspace                                   |
| usedQuota        | object | The used resources of workspace                                       |
| usedNodeCount | int    | The node currently in use has workloads running on it                 |
---

### 4. Update Workspace

Update workspace configuration.

**Endpoint**: `PATCH /api/v1/workspaces/{WorkspaceId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkspaceId | Workspace ID |

**Request Parameters**:

```json
{
  "description": "New description",
  "flavorId": "gpu-xlarge",
  "replica": 8,
  "queuePolicy": "balance",
  "scopes": ["Train", "Infer"],
  "volumes": [],
  "enablePreempt": true,
  "managers": ["user-001", "user-003"],
  "isDefault": false,
  "imageSecretIds": ["image-secret-001", "image-secret-002"]
}
```

**Field Description**: 

All fields are optional, only provided fields will be updated

All parameters have the same meaning as the corresponding parameters in "Create Workspace".

**Response**: 200 OK with no response body

---

### 5. Delete Workspace

Delete a specific workspace.

**Endpoint**: `DELETE /api/v1/workspaces/{WorkspaceId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkspaceId | Workspace ID |

**Prerequisites**: No running workloads in the workspace

**Response**: 200 OK with no response body

---

### 6. Manage Workspace Nodes

Add or remove nodes from a workspace.

**Endpoint**: `POST /api/v1/workspaces/{WorkspaceId}/nodes`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| WorkspaceId | Workspace ID |

**Request Parameters**:

```json
{
  "nodeIds": ["node-004", "node-005"],
  "action": "add"
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| nodeIds | []string | Yes | List of node IDs |
| action | string | Yes | Action type: add/remove |

**Response**: 200 OK with no response body

---

## Workspace Status

| Status | Description                      |
|--------|----------------------------------|
| Creating | Being created                    |
| Running | Running                          |
| Abnormal | Abnormal (all nodes unavailable) |
| Deleting | Being deleted                    |

## Queue Policy

### FIFO (First-In-First-Out)
- Workloads in queue execute in submission order
- If front task lacks resources, subsequent tasks wait
- Suitable for fair scheduling scenarios

### Balance (Load Balancing)
- Any task meeting resource conditions can be scheduled
- Avoids front task blocking
- Still subject to priority constraints
- Suitable for resource utilization priority scenarios

## Service Modules

| Module    | Description          |
|-----------|----------------------|
| Train     | Training jobs        |
| Infer     | Inference services   |
| Authoring | Development environment |
| CICD      | CICD runner            |

## Resource Quota

- **totalQuota**: Total workspace quota (number of nodes × node flavor)
- **availQuota**: Available quota (total quota - used quota - abnormal quota)
- **usedQuota**: Quota used by workloads
- **abnormalQuota**: Quota occupied by abnormal nodes

## Notes

1. **Workspace Naming**: Actual ID is `{clusterId}-{name}`
2. **Node Flavor**: One workspace can only use one node flavor
3. **Default Workspace**: When set as default, all users can access it
4. **Manager Permissions**: Managers can manage all resources in the workspace
5. **Preemption Mechanism**: When enabled, high priority tasks can preempt low priority task resources
