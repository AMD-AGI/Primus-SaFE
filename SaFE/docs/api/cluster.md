# Cluster API

Cluster management API for creating and managing Kubernetes clusters, including control plane configuration and node management.

## API List

### 1. Create Cluster

Create a new Kubernetes cluster.

**Endpoint**: `POST /api/custom/clusters`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "name": "prod-cluster",
  "description": "Production environment cluster",
  "nodes": ["node-001", "node-002", "node-003"],
  "sshSecretId": "ssh-secret-001",
  "imageSecretId": "image-secret-001",
  "kubeSprayImage": "docker.io/kubespray:v2.20.0",
  "kubePodsSubnet": "10.244.0.0/16",
  "kubeServiceAddress": "10.96.0.0/16",
  "kubeNetworkPlugin": "flannel",
  "kubeVersion": "1.32.5",
  "kubeApiServerArgs": {
    "max-requests-inflight": "400"
  },
  "labels": {
    "env": "production",
    "region": "us-west"
  },
  "isProtected": true
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Cluster name (unique identifier) |
| description | string | No | Cluster description |
| nodes | []string | Yes | List of control plane node IDs |
| sshSecretId | string | No | SSH secret ID for node login |
| imageSecretId | string | No | Image registry secret ID |
| kubeSprayImage | string | No | KubeSpray image address |
| kubePodsSubnet | string | No | Pod subnet, default 10.244.0.0/16 |
| kubeServiceAddress | string | No | Service address range, default 10.96.0.0/16 |
| kubeNetworkPlugin | string | No | Network plugin, default flannel |
| kubeVersion | string | No | Kubernetes version |
| kubeApiServerArgs | object | No | API Server additional arguments |
| labels | object | No | Cluster labels |
| isProtected | bool | No | Whether protected (protected clusters cannot be deleted directly) |

**Response Example**:

```json
{
  "clusterId": "prod-cluster"
}
```

---

### 2. List Clusters

Get all clusters list.

**Endpoint**: `GET /api/custom/clusters`

**Authentication Required**: No (Public endpoint)

**Response Example**:

```json
{
  "totalCount": 3,
  "items": [
    {
      "clusterId": "prod-cluster",
      "userId": "user-001",
      "phase": "Ready",
      "isProtected": true,
      "creationTime": "2025-01-10T08:00:00.000Z"
    },
    {
      "clusterId": "dev-cluster",
      "userId": "user-002",
      "phase": "Creating",
      "isProtected": false,
      "creationTime": "2025-01-15T10:00:00.000Z"
    }
  ]
}
```

---

### 3. Get Cluster Details

Get detailed information about a specific cluster.

**Endpoint**: `GET /api/custom/clusters/:name`

**Authentication Required**: No (Public endpoint)

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Cluster ID |

**Response Example**:

```json
{
  "clusterId": "prod-cluster",
  "userId": "user-001",
  "phase": "Ready",
  "isProtected": true,
  "creationTime": "2025-01-10T08:00:00.000Z",
  "description": "Production environment cluster",
  "endpoint": "10.0.0.100:6443",
  "sshSecretId": "ssh-secret-001",
  "imageSecretId": "image-secret-001",
  "nodes": ["node-001", "node-002", "node-003"],
  "kubeSprayImage": "docker.io/kubespray:v2.20.0",
  "kubePodsSubnet": "10.244.0.0/16",
  "kubeServiceAddress": "10.96.0.0/16",
  "kubeNetworkPlugin": "flannel",
  "kubernetesVersion": "1.32.5",
  "kubeApiServerArgs": {
    "max-requests-inflight": "400"
  }
}
```

---

### 4. Update Cluster

Update cluster configuration.

**Endpoint**: `PATCH /api/custom/clusters/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Cluster ID |

**Request Parameters**:

```json
{
  "isProtected": false
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| isProtected | bool | No | Whether to protect the cluster |

**Response**: No content (204)

---

### 5. Delete Cluster

Delete a specific cluster.

**Endpoint**: `DELETE /api/custom/clusters/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Cluster ID |

**Prerequisites**:
- Cluster must not be protected (isProtected=false)
- No running workloads on the cluster

**Response**: No content (204)

---

### 6. Manage Cluster Nodes

Add or remove nodes from a cluster.

**Endpoint**: `POST /api/custom/clusters/:name/nodes`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Cluster ID |

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

**Response Example**:

```json
{
  "totalCount": 2,
  "successCount": 2
}
```

---

### 7. Get Cluster Creation Logs

Get logs from the cluster creation process.

**Endpoint**: `GET /api/custom/clusters/:name/logs`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | Cluster ID |

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| tailLines | int | No | Return last N lines of logs, default 1000 |
| sinceSeconds | int | No | Return logs from last N seconds |

**Response Example**:

```json
{
  "clusterId": "prod-cluster",
  "podId": "cluster-create-pod-abc123",
  "logs": [
    "2025-01-10 08:00:00 INFO Starting cluster creation...",
    "2025-01-10 08:01:00 INFO Installing Kubernetes on nodes...",
    "2025-01-10 08:10:00 INFO Cluster creation completed successfully"
  ]
}
```

---

## Cluster Status

| Status | Description |
|--------|-------------|
| Creating | Being created |
| Ready | Ready and available |
| Failed | Creation failed |
| Deleting | Being deleted |

## Network Plugins

| Plugin | Description |
|--------|-------------|
| flannel | Simple and easy-to-use Overlay network (default) |
| calico | BGP network with network policy support |
| cilium | High-performance network based on eBPF |

## Notes

1. **Control Plane Nodes**: Recommend using odd numbers of nodes (1/3/5)
2. **Protection Flag**: Protected clusters need to be unprotected before deletion
3. **Node Requirements**: Nodes must be registered and SSH accessible
4. **Deletion Restrictions**: All workloads must be stopped before deleting cluster
5. **Log Viewing**: Creation logs are only available during the creation process
