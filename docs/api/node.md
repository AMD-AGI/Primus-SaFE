# Node API

Node management API for registering, managing and monitoring physical servers or virtual machine nodes.

## API List

### 1. Create Node

Register a new node to the system.

**Endpoint**: `POST /api/custom/nodes`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "hostname": "gpu-node-001",
  "privateIP": "192.168.1.100",
  "publicIP": "203.0.113.100",
  "port": 22,
  "labels": {
    "gpu-type": "A100",
    "datacenter": "us-west"
  },
  "flavorId": "gpu-large",
  "templateId": "ubuntu-gpu-template",
  "sshSecretId": "ssh-secret-001"
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| hostname | string | No | Node hostname, uses privateIP if not specified |
| privateIP | string | Yes | Node private IP |
| publicIP | string | No | Node public IP |
| port | int | No | SSH port, default 22 |
| labels | object | No | Node labels |
| flavorId | string | Yes | Node flavor ID |
| templateId | string | Yes | Node template ID (environment configuration) |
| sshSecretId | string | Yes | SSH secret ID |

**Response Example**:

```json
{
  "nodeId": "gpu-node-001-abc123"
}
```

---

### 2. List Nodes

Get node list with multiple filtering options.

**Endpoint**: `GET /api/custom/nodes`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| clusterId | string | No | Filter by cluster ID, empty string for unbound nodes |
| workspaceId | string | No | Filter by workspace ID |
| flavorId | string | No | Filter by node flavor ID |
| nodeId | string | No | Filter by node ID |
| available | bool | No | Filter by availability: true (available)/false (unavailable) |
| phase | string | No | Filter by status (comma-separated) |
| isAddonsInstalled | bool | No | Filter by addon installation status |
| brief | bool | No | Brief mode, returns only ID, name and IP |
| offset | int | No | Pagination offset, default 0 |
| limit | int | No | Records per page, default 100, -1 for all |

**Response Example (Full mode)**:

```json
{
  "totalCount": 20,
  "items": [
    {
      "nodeId": "gpu-node-001-abc123",
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
        "cpu": "128",
        "gpu": "8",
        "memory": "512Gi",
        "ephemeralStorage": "1Ti"
      },
      "availResources": {
        "cpu": "64",
        "gpu": "4",
        "memory": "256Gi",
        "ephemeralStorage": "500Gi"
      },
      "creationTime": "2025-01-10T10:00:00.000Z",
      "workloads": [
        {
          "id": "training-job-001",
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
      "internalIP": "192.168.1.100"
    }
  ]
}
```

---

### 3. Get Node Details

Get detailed information about a specific node.

**Endpoint**: `GET /api/custom/nodes/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Node ID |

**Response Example**:

```json
{
  "nodeId": "gpu-node-001-abc123",
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
    "cpu": "128",
    "gpu": "8",
    "memory": "512Gi",
    "ephemeralStorage": "1Ti"
  },
  "availResources": {
    "cpu": "64",
    "gpu": "4",
    "memory": "256Gi",
    "ephemeralStorage": "500Gi"
  },
  "creationTime": "2025-01-10T10:00:00.000Z",
  "workloads": [
    {
      "id": "training-job-001",
      "userId": "user-001",
      "workspaceId": "prod-cluster-ai-team"
    }
  ],
  "isControlPlane": false,
  "isAddonsInstalled": true,
  "flavorId": "gpu-large",
  "templateId": "ubuntu-gpu-template",
  "taints": [
    {
      "key": "gpu",
      "value": "true",
      "effect": "NoSchedule"
    }
  ],
  "customerLabels": {
    "gpu-type": "A100",
    "datacenter": "us-west"
  },
  "lastStartupTime": "2025-01-10T10:05:00.000Z"
}
```

---

### 4. Update Node

Update node configuration.

**Endpoint**: `PATCH /api/custom/nodes/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Node ID |

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
  "templateId": "ubuntu-gpu-template-v2",
  "port": 2222,
  "privateIP": "192.168.1.101"
}
```

**Field Description**: All fields are optional, only provided fields will be updated

**Response**: No content (204)

---

### 5. Delete Node

Delete node (deregister from system).

**Endpoint**: `DELETE /api/custom/nodes/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Node ID |

**Prerequisites**: Node is not bound to any cluster

**Response**: No content (204)

---

### 6. Get Node Management Logs

Get operation logs for node joining/leaving cluster.

**Endpoint**: `GET /api/custom/nodes/:name/logs`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Node ID |

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

---

## Node Status

| Status | Description |
|--------|-------------|
| Ready | Ready and available |
| SSHFailed | SSH connection failed |
| HostnameFailed | Hostname setup failed |
| Managing | Joining cluster |
| ManagedFailed | Failed to join cluster |
| Unmanaging | Leaving cluster |
| UnmanagedFailed | Failed to leave cluster |

## Taints

Taints are used to control Pod scheduling. Common Effect types:

| Effect | Description |
|--------|-------------|
| NoSchedule | Do not allow scheduling new Pods |
| PreferNoSchedule | Try not to schedule new Pods |
| NoExecute | Do not allow scheduling and evict existing Pods |

**Note**: System automatically adds `primus-safe.amd.com/` prefix to taint keys

## Resource Statistics

- **totalResources**: Total node resources (defined by node flavor)
- **availResources**: Available resources (total resources - allocated resources)
- **workloads**: List of workloads currently running on this node

## Notes

1. **Node Registration**: Node must be SSH accessible and meet system requirements
2. **Node Flavor**: Not recommended to modify after creation, may cause inaccurate resource statistics
3. **Node Template**: Defines node's software environment and addons
4. **Control Plane Nodes**: Cannot be deleted or have cluster binding changed
5. **Node Labels**: Custom labels cannot use `primus-safe.amd.com/` prefix
6. **Deletion Restrictions**: Node must be removed from cluster before deletion
