# Node API Documentation

## Overview

The Node APIs provide management capabilities for computing nodes within the system. Nodes are the fundamental compute resources that form clusters and host workloads. These APIs allow users to create, list, retrieve, update, and delete nodes, as well as manage node configurations.

## API Endpoints

### Create Node
```
POST /api/v1/nodes
```

Creates a new node with specified configuration.

**Request Body:**
```json
{
  // Node hostname. If not specified, it will be assigned the value of PrivateIP
  "hostname": "string",
  // Node private ip, required
  "privateIP": "string",
  // Node public IP, accessible from external networks, optional
  "publicIP": "string",
  // SSH port, default is 22
  "port": 22,
  // Node labels
  "labels": {
    "key": "string"
  },
  // Associated node flavor id
  "flavorId": "string",
  // Associated node template id
  "templateId": "string",
  // The secret id for ssh
  "sshSecretId": "string"
}
```


**Response:**
```json
{
  // The node id
  "nodeId": "string"
}
```


### List Nodes
```
GET /api/v1/custom/nodes?workspaceId={workspaceId}&clusterId={clusterId}&flavorId={flavorId}&nodeId={nodeId}&available={available}&phase={phase}&isAddonsInstalled={isAddonsInstalled}&brief={brief}&offset={offset}&limit={limit}
```


Retrieves a list of nodes with optional filtering.

**Query Parameters:**
- `workspaceId` (optional): Filter results by workspace ID
- `clusterId` (optional): Filter results by cluster ID
- `flavorId` (optional): Filter results by node flavor ID
- `nodeId` (optional): Filter results by node ID
- `available` (optional): Filter results based on node availability
- `phase` (optional): Filter results by node phase, such as Ready, SSHFailed, HostnameFailed, Managing, ManagedFailed, Unmanaging, UnmanagedFailed. If specifying multiple kind queries, separate them with commas
- `isAddonsInstalled` (optional): Filter results based on addon installation status
- `brief` (optional): Return only basic node information. If enabled, only the node id, node Name and node IP will be returned.
- `offset` (optional): Starting offset for results (default: 0)
- `limit` (optional): Limit number of results (default: 100, -1 for all)

**Response (Detailed):**
```json
{
  // TotalCount indicates the total number of faults, not limited by pagination
  "totalCount": 0,
  "items": [
    {
      // node id
      "nodeId": "string",
      // node name
      "nodeName": "string",
      // the internal ip of k8s cluster
      "internalIP": "string",
      // The cluster id of node
      "clusterId": "string",
      // The workspace id and name of node
      "workspace": {
        "id": "string",
        "name": "string"
      },
      "phase": "string",
      "available": true,
      "message": "string",
      "totalResources": {...},
      "availResources": {...},
      "creationTime": "string",
      "workloads": [
        {
          "id": "string",
          "userId": "string",
          "workspaceId": "string"
        }
      ],
      "isControlPlane": true,
      "isAddonsInstalled": true
    }
  ]
}
```


**Response (Brief):**
```json
{
  "totalCount": 0,
  "items": [
    {
      "nodeId": "string",
      "nodeName": "string",
      "internalIP": "string"
    }
  ]
}
```


### Get Node
```
GET /api/v1/custom/nodes/{nodeId}
```


Retrieves detailed information about a specific node.

**Response:**
```json
{
  "nodeId": "string",
  "nodeName": "string",
  "internalIP": "string",
  "clusterId": "string",
  "workspace": {
    "id": "string",
    "name": "string"
  },
  "phase": "string",
  "available": true,
  "message": "string",
  "totalResources": {...},
  "availResources": {...},
  "creationTime": "string",
  "workloads": [
    {
      "id": "string",
      "userId": "string",
      "workspaceId": "string"
    }
  ],
  "isControlPlane": true,
  "isAddonsInstalled": true,
  "flavorId": "string",
  "templateId": "string",
  "taints": [...],
  "customerLabels": {
    "key": "string"
  },
  "lastStartupTime": "string"
}
```


### Update Node
```
PATCH /api/v1/custom/nodes/{nodeId}
```


Partially updates a node with specified fields.

**Request Body:**
```json
{
  "taints": [...],
  "labels": {
    "key": "string"
  },
  "flavorId": "string",
  "templateId": "string",
  "port": 0,
  "privateIP": "string"
}
```


**Response:** No content

### Delete Node
```
DELETE /api/v1/custom/nodes/{nodeId}
```


Deletes a specific node.

**Response:** No content

### Get Node Logs
```
GET /api/v1/custom/nodes/{nodeId}/logs
```


Retrieves logs from the pod associated with node management operations.

**Response:**
```json
{
  "clusterId": "string",
  "nodeId": "string",
  "podId": "string",
  "logs": ["string"]
}
```


## Data Models

### CreateNodeRequest
| Field | Type | Description |
|-------|------|-------------|
| hostname | string | Node hostname (optional) |
| privateIP | string | Node private IP (required) |
| publicIP | string | Node public IP |
| port | integer | SSH port (default: 22) |
| labels | object | Node labels |
| flavorId | string | Associated node flavor ID |
| templateId | string | Associated node template ID |
| sshSecretId | string | SSH secret ID |

### Node Phases
- [Ready](file://C:\Project\Primus-SaFE\apis\pkg\apis\amd\v1\well_known_constants.go#L130-L130): Node is ready to accept workloads
- `SSHFailed`: SSH connection to node failed
- `HostnameFailed`: Hostname configuration failed
- `Managing`: Node is being managed
- `ManagedFailed`: Node management failed
- `Unmanaging`: Node is being unmanaged
- `UnmanagedFailed`: Node unmanagement failed

### NodeResponseItem
| Field | Type | Description |
|-------|------|-------------|
| nodeId | string | Unique node identifier |
| nodeName | string | Display name of the node |
| internalIP | string | Internal IP address |
| clusterId | string | Associated cluster ID |
| workspace | object | Associated workspace |
| phase | string | Current node phase |
| available | boolean | Node scheduling availability |
| message | string | Reason if node is unavailable |
| totalResources | object | Total node resources |
| availResources | object | Available node resources |
| creationTime | string | Node creation timestamp |
| workloads | array | Workloads running on the node |
| isControlPlane | boolean | Whether node is control plane |
| isAddonsInstalled | boolean | Whether addons are installed |

### PatchNodeRequest
All fields are optional and only provided fields will be updated:
- taints (array): Taints to modify on the node
- labels (object): Labels to modify on the node
- flavorId (string): Node flavor ID to modify
- templateId (string): Node template ID to modify
- port (integer): SSH port
- privateIP (string): Node private IP