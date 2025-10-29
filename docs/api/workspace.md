# Workspace API

Workspace is a resource isolation unit in the cluster, providing independent running environment and resource quotas for users.

## API List

### 1. Create Workspace

Create a new workspace in a cluster.

**Endpoint**: `POST /api/custom/workspaces`

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
      "name": "shared-data",
      "type": "nfs",
      "path": "/mnt/data",
      "server": "nfs-server.example.com"
    }
  ],
  "enablePreempt": false,
  "managers": ["user-001", "user-002"],
  "isDefault": false,
  "imageSecretIds": ["image-secret-001"]
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Workspace name |
| clusterId | string | Yes | Cluster ID |
| description | string | No | Workspace description |
| flavorId | string | No | Node flavor ID |
| replica | int | No | Expected number of nodes |
| queuePolicy | string | No | Queue policy: fifo (first-in-first-out)/balance (load balancing), default fifo |
| scopes | []string | No | Supported service modules: Train/Infer/Authoring, no limitation if not specified |
| volumes | []object | No | Storage volume configuration list |
| enablePreempt | bool | No | Whether to enable preemption, default false |
| managers | []string | No | List of manager user IDs |
| isDefault | bool | No | Whether to set as default workspace (accessible to all users) |
| imageSecretIds | []string | No | List of image pull secret IDs |

**Volume Configuration**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Volume name |
| type | string | Yes | Volume type: nfs/hostPath/pvc |
| path | string | Yes | Mount path |
| server | string | No | NFS server address (required when type=nfs) |

**Response Example**:

```json
{
  "workspaceId": "prod-cluster-ai-team"
}
```

---

### 2. List Workspaces

Get workspace list with cluster filtering support.

**Endpoint**: `GET /api/custom/workspaces`

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
      "creationTime": "2025-01-12T09:00:00.000Z",
      "description": "AI team workspace",
      "queuePolicy": "fifo",
      "scopes": ["Train", "Infer", "Authoring"],
      "volumes": [
        {
          "name": "shared-data",
          "type": "nfs",
          "path": "/mnt/data",
          "server": "nfs-server.example.com"
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

---

### 3. Get Workspace Details

Get detailed information about a specific workspace, including resource quotas.

**Endpoint**: `GET /api/custom/workspaces/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workspace ID |

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
  "creationTime": "2025-01-12T09:00:00.000Z",
  "description": "AI team workspace",
  "queuePolicy": "fifo",
  "scopes": ["Train", "Infer", "Authoring"],
  "volumes": [
    {
      "name": "shared-data",
      "type": "nfs",
      "path": "/mnt/data",
      "server": "nfs-server.example.com"
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
    "cpu": "512",
    "gpu": "32",
    "memory": "2048Gi"
  },
  "availQuota": {
    "cpu": "256",
    "gpu": "16",
    "memory": "1024Gi"
  },
  "usedQuota": {
    "cpu": "256",
    "gpu": "16",
    "memory": "1024Gi"
  },
  "abnormalQuota": {
    "cpu": "0",
    "gpu": "0",
    "memory": "0"
  },
  "usedNodeCount": 2,
  "imageSecretIds": ["image-secret-001"]
}
```

---

### 4. Update Workspace

Update workspace configuration.

**Endpoint**: `PATCH /api/custom/workspaces/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workspace ID |

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

**Field Description**: All fields are optional, only provided fields will be updated

**Response**: No content (204)

---

### 5. Delete Workspace

Delete a specific workspace.

**Endpoint**: `DELETE /api/custom/workspaces/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workspace ID |

**Prerequisites**: No running workloads in the workspace

**Response**: No content (204)

---

### 6. Manage Workspace Nodes

Add or remove nodes from a workspace.

**Endpoint**: `POST /api/custom/workspaces/:name/nodes`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Workspace ID |

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

**Response**: No content (204)

---

## Workspace Status

| Status | Description |
|--------|-------------|
| Creating | Being created |
| Running | Running |
| Abnormal | Abnormal (some nodes unavailable) |
| Deleting | Being deleted |

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

| Module | Description |
|--------|-------------|
| Train | Training tasks |
| Infer | Inference services |
| Authoring | Development environment |

## Resource Quota

- **totalQuota**: Total workspace quota (number of nodes × node flavor)
- **availQuota**: Available quota (total quota - used quota - abnormal quota)
- **usedQuota**: Quota used by workloads
- **abnormalQuota**: Quota occupied by abnormal nodes
- **usedNodeCount**: Number of nodes currently running workloads

## Notes

1. **Workspace Naming**: Actual ID is `{clusterId}-{name}`
2. **Node Flavor**: One workspace can only use one node flavor
3. **Default Workspace**: When set as default, all users can access it
4. **Manager Permissions**: Managers can manage all resources in the workspace
5. **Preemption Mechanism**: When enabled, high priority tasks can preempt low priority task resources
