# Cluster API

## Overview
The Cluster API provides comprehensive management capabilities for computing clusters. A cluster serves as the top-level resource container in the system, used to organize and manage node resources, network configurations, and the runtime environment for workloads. Each cluster is an independent Kubernetes environment that provides physical resource isolation and supports multi-tenancy.

### Core Concepts

A cluster is the root-level container for all computing resources, with the following key characteristics:

* Resource Isolation: Provides physical-level resource isolation to ensure clusters operate independently without interference.
* Node Management: Manages control plane nodes and worker nodes.
* Network Configuration: Defines Pod subnets, service addresses, and network plugin settings.
* Kubernetes Environment: Encapsulates a complete Kubernetes runtime environment, including version and API server configuration.

## API List

### 1. Create Cluster

Create a new Kubernetes cluster.

**Endpoint**: `POST /api/v1/clusters`

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
  "kubePodsSubnet": "10.0.0.0/16",
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

| Field | Type | Required | Description                                                                                                         |
|-------|------|----------|---------------------------------------------------------------------------------------------------------------------|
| name | string | Yes | Cluster name (unique identifier)                                                                                    |
| description | string | No | Cluster description                                                                                                 |
| nodes | []string | Yes | List of control plane node IDs                                                                                      |
| sshSecretId | string | Yes | SSH secret ID for node login                                                                                        |
| imageSecretId | string | No | Image registry secret ID                                                                                            |
| kubeSprayImage | string | Yes | KubeSpray image address. e.g. docker.io/kubespray:v2.20.0                                                           |
| kubePodsSubnet | string | Yes | Pod subnet, e.g. "10.0.0.0/16"                                  |
| kubeServiceAddress | string | Yes | Service address range, e.g. 192.168.0.0/16                                                                          |
| kubeNetworkPlugin | string | Yes | Network plugin, default flannel                                                                                     |
| kubeVersion | string | Yes | Kubernetes version, e.g.  1.32.5                                                                                    |
| kubeApiServerArgs | object | No | additional arguments for Kubernetes, e.g. {"max-mutating-requests-inflight":"5000","max-requests-inflight":"10000"} |
| labels | object | No | User-defined labels (key-value pairs). Keys cannot start with "primus-safe"                                           |
| isProtected | bool | No | Whether protected (protected clusters cannot be deleted directly)                                                   |

**Response Example**:

```json
{
  "clusterId": "prod-cluster"
}
```

**Field Description**:

| Field | Type | Description          |
|-------|------|----------------------|
| clusterId | string | Generated Cluster ID |

---

### 2. List Clusters

Get all clusters list.

**Endpoint**: `GET /api/v1/clusters`

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
      "creationTime": "2025-01-10T08:00:00"
    },
    {
      "clusterId": "dev-cluster",
      "userId": "user-002",
      "phase": "Creating",
      "isProtected": false,
      "creationTime": "2025-01-15T10:00:00"
    }
  ]
}
```
**Field Description**:

| Field | Type     | Description                                            |
|-------|----------|--------------------------------------------------------|
| totalCount | int      | The total number of clusters                           |
| clusterId | string   | Cluster ID                                             |
| userId | []string | User ID who created the cluster                        |
| phase | string   | Cluster status, e.g. Ready,Creating,Failed,Deleting |
| isProtected | bool     | Whether the cluster is under protection                |
| creationTime | string   | The cluster creation time. e.g. "2025-07-08T10:31:46"                       |


---

### 3. Get Cluster Details

Get detailed information about a specific cluster.

**Endpoint**: `GET /api/v1/clusters/{ClusterId}`

**Authentication Required**: No (Public endpoint)

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| ClusterId | Cluster ID |

**Response Example**:

```json
{
  "clusterId": "prod-cluster",
  "userId": "user-001",
  "phase": "Ready",
  "isProtected": true,
  "creationTime": "2025-01-10T08:00:00",
  "description": "Production environment cluster",
  "endpoint": "10.0.0.100:6443",
  "sshSecretId": "ssh-secret-001",
  "imageSecretId": "image-secret-001",
  "nodes": ["node-001", "node-002", "node-003"],
  "kubeSprayImage": "docker.io/kubespray:v2.20.0",
  "kubePodsSubnet": "10.0.0.0/16",
  "kubeServiceAddress": "10.254.0.0/16",
  "kubeNetworkPlugin": "flannel",
  "kubernetesVersion": "1.32.5",
  "kubeApiServerArgs": {
    "max-requests-inflight": "400"
  }
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| clusterId | string | Cluster ID |
| userId | string | User ID who created the cluster |
| phase | string | Cluster status, such as Ready,Creating,Failed,Deleting |
| isProtected | bool | Whether the cluster is under protection |
| creationTime | string | The cluster creation time, e.g. "2025-01-10T08:00:00" |
| description | string | Cluster description |
| endpoint | string | Kubernetes API server endpoint (host:port) |
| sshSecretId | string | SSH secret ID for node login |
| imageSecretId | string | Image registry secret ID |
| nodes | []string | List of node IDs of control plane |
| kubeSprayImage | string | KubeSpray image address, e.g. docker.io/kubespray:v2.20.0 |
| kubePodsSubnet | string | Pod subnet, e.g. 10.0.0.0/16 |
| kubeServiceAddress | string | Service address range, e.g. 10.254.0.0/16 |
| kubeNetworkPlugin | string | Network plugin, default flannel |
| kubernetesVersion | string | Kubernetes version, e.g. 1.32.5 |
| kubeApiServerArgs | object | Additional Kubernetes API server arguments |

---

### 4. Update Cluster

Update cluster configuration.

**Endpoint**: `PATCH /api/v1/clusters/{ClusterId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| ClusterId | Cluster ID |

**Request Parameters**:

```json
{
  "isProtected": false,
  "labels": {
    "region": "us-east"
  }
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| isProtected | bool | No | Whether to protect the cluster |
| labels | object | No | User-defined labels (key-value pairs). Keys cannot start with "primus-safe"

**Response**: 200 OK with no response body

---

### 5. Delete Cluster

Delete a specific cluster.

**Endpoint**: `DELETE /api/custom/clusters/{ClusterId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| ClusterId | Cluster ID |

**Prerequisites**:
- Cluster must not be protected (isProtected=false)
- No running workloads on the cluster

**Response**: 200 OK with no response body

---

### 6. Manage Cluster Nodes

Add or remove nodes from a cluster.

**Endpoint**: `POST /api/v1/clusters/{ClusterId}/nodes`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| ClusterId | Cluster ID |

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
| nodeIds | []string | Yes | List of node IDs to operate on |
| action | string | Yes | Action type: add/remove |

**Response Example**:

```json
{
  "totalCount": 2,
  "successCount": 2
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of nodes to operate on |
| successCount | int | Number of nodes processed successfully |

---

### 7. Get Cluster Creation Logs

Get logs from the cluster creation process.

**Endpoint**: `GET /api/v1/clusters/{ClusterId}/logs`

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
