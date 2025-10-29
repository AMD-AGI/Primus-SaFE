# Workspace API Documentation

## Overview

The Workspace API provides comprehensive management capabilities for workspace resources. A workspace is a core concept in the system, used to organize and isolate computing resources, user permissions, and workloads. Each workspace is associated with a specific cluster and defines a set of resource quotas, access policies, and service scopes.

These APIs allow users to create, list, retrieve, update, and delete workspaces, as well as manage nodes associated with specific workspaces.

Note: Each workload is submitted to a specified workspace.

## API Endpoints

### Create Workspace
```
POST /api/v1/workspaces
```


Creates a new workspace with specified configuration.

**Request Body:**
```json
{
  // The workspace name(display only). Used for generate workspace id.
  // The final ID is clusterId + "-" + name.
  "name": "string",
  // The cluster which workspace belongs to
  "clusterId": "string",
  // The workspace description
  "description": "string",
  // Queuing policy for workloads submitted in this workspace.
  // All workloads currently share the same policy, supports fifo (default) and balance.
  // 1. "fifo" means first-in, first-out: the workload that enters the queue first is served first.
  //    If the front workload does not meet the conditions for dispatch, it will wait indefinitely,
  //    and other tasks in the queue will also be blocked waiting.
  // 2. "balance" allows any workload that meets the resource conditions to be dispatched,
  //    avoiding blockage by the front workload in the queue. However, it is still subject to priority constraints.
  //    If a higher-priority task cannot be dispatched, lower-priority tasks will wait.
  "queuePolicy": "string",
  // The node flavor id of workspace, A workspace supports only one node flavor
  "flavorId": "string",
  // The expected number of nodes in the workspace
  "replica": 0,
  // Service modules available in this space. support: Train/Infer/Authoring, No limitation if not specified
  "scopes": ["Train", "Infer", "Authoring"],
  // Volumes used in this workspace
  "volumes": [
    {
      // access mode, default is ReadWriteMany
      "accessMode": "ReadWriteMany",
      // The following parameters are used for PVC creation. If using hostPath mounting, they are not required.
      // Capacity size, such as 100Gi. This is a required parameter when creating a PVC (PersistentVolumeClaim).
      "capacity": "100Ti",
      // Mount path to be used, equivalent to 'mountPath' in Kubernetes volume mounts.
      // +required
      "mountPath": "/my_path",
      // selector is a label query over volumes to consider for binding.
      // It cannot be used together with storageClass. If both are set, the selector takes priority
      "selector": {
        "pfs-name": "test-pv"
      },
      // Responsible for automatic PV creation
      "storageClass": "rbd",
      // The volume type, valid values includes: pfs/hostpath
      // If PFS is configured, a PVC will be automatically created in the workspace.
      "type": "pfs"
    }, {
      "accessMode": "ReadWriteMany",
      // Path on the host to mount. Required when volume type is hostpath
      "hostPath": "/home",
      "mountPath": "/home",
      "type": "hostpath"
    }
  ],
  // Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
  "enablePreempt": true,
  // User id of the workspace administrator
  "managers": ["string"],
  // Set the workspace as the default workspace (i.e., all users can access it)
  "isDefault": true,
  // Workspace image secret ID, used for downloading images
  "imageSecretIds": ["string"]
}
```


**Response:**
```json
{
  // The workspace id
  "workspaceId": "string"
}
```


### List Workspaces
```
GET /api/v1/workspaces?clusterId={clusterId}
```


Retrieves a list of workspaces with optional filtering by cluster ID.

**Query Parameters:**
- `clusterId` (optional): Filter results by cluster ID

**Response:**
```json
{
  // The total number of node templates, not limited by pagination
  "totalCount": 0,
  "items": [
    {
      // The workspace id
      "workspaceId": "string",
      // The workspace name
      "workspaceName": "string",
      // The cluster which workspace belongs to
      "clusterId": "string",
      // The node flavor id used by workspace
      "flavorId": "string",
      // User id of workspace creator
      "userId": "string",
      // The target expected number of nodes of workspace
      "targetNodeCount": 0,
      // The current total number of nodes
      "currentNodeCount": 0,
      // The current total number of abnormal nodes
      "abnormalNodeCount": 0,
      // The status of workspace, such as Creating, Running, Abnormal, Deleting
      "phase": "string",
      // The workspace creation time
      "creationTime": "string",
      // The workspace description
      "description": "string",
      // Queuing policy for workload submitted in this workspace
      // Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
      "queuePolicy": "string",
      // Support service module: Train/Infer/Authoring, No limitation if not specified
      "scopes": ["Train", "Infer", "Authoring"],
      // The store volumes used by workspace.  Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
      "volumes": [],
      // Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
      "enablePreempt": true,
      // User id of the workspace administrator
      "managers": [{"id": "string", "name": "string"}],
      // Set the workspace as the default workspace (i.e., all users can access it).
      "isDefault": true
    }
  ]
}
```


### Get Workspace
```
GET /api/v1/workspaces/{workspaceId}
```


Retrieves detailed information about a specific workspace.

**Response:**
```json
{
  // The workspace id
  "workspaceId": "string",
  // The workspace name
  "workspaceName": "string",
  // The cluster which workspace belongs to
  "clusterId": "string",
  // The node flavor id used by workspace
  "flavorId": "string",
  // User id of workspace creator
  "userId": "string",
  // The target expected number of nodes of workspace
  "targetNodeCount": 0,
  // The current total number of nodes
  "currentNodeCount": 0,
  // The current total number of abnormal nodes
  "abnormalNodeCount": 0,
  // The status of workspace, such as Creating, Running, Abnormal, Deleting
  "phase": "string",
  // The workspace creation time
  "creationTime": "string",
  // The workspace description
  "description": "string",
  // Queuing policy for workload submitted in this workspace
  // Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
  "queuePolicy": "string",
  // Support service module: Train/Infer/Authoring, No limitation if not specified
  "scopes": ["Train", "Infer", "Authoring"],
  // The store volumes used by workspace.  Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
  "volumes": [],
  // Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
  "enablePreempt": true,
  // User id of the workspace administrator
  "managers": [{"id": "string", "name": "string"}],
  // Set the workspace as the default workspace (i.e., all users can access it).
  "isDefault": true,
  // The total resource of workspace
  "totalQuota": {
    "amd.com/gpu": 1896,
    "cpu": 30322,
    "ephemeral-storage": 3275467021067664,
    "memory": 767056934719488,
    "rdma/hca": 237000
  },
  // The available resource of workspace
  "availQuota": {},
  // The abnormal resources of workspace
  "abnormalQuota": {},
  // The used resources of workspace
  "usedQuota": {},
  // The node currently in use has workloads running on it
  "usedNodeCount": 0,
  // Workspace image secret ID, used for downloading images
  "imageSecretIds": ["string"]
}
```


### Update Workspace
```
PATCH /api/v1/workspaces/{workspaceId}
```


Partially updates a workspace with specified fields.

**Request Body:**
```json
{
  // The node flavor id used by workspace
  "flavorId": "string",
  // The expected total node count
  "replica": 0,
  // Queuing policy for tasks submitted in this workspace. such as fifo, balance
  // Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
  "queuePolicy": "string",
  // Support service module: Train/Infer/Authoring, No limitation if not specified
  "scopes": ["Train", "Infer", "Authoring"],
  // The store volumes used by workspace, Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
  "volumes": [],
  // The workspace description
  "description": "string",
  // Whether preemption is enabled
  "enablePreempt": true,
  // User id of the workspace administrator
  "managers": ["string"],
  // Set the workspace as the default workspace (i.e., all users can access it).
  "isDefault": true,
  // Workspace image secret ID, used for downloading images
  "imageSecretIds": ["string"]
}
```


**Response:** No content

### Delete Workspace
```
DELETE /api/v1/workspaces/{workspaceId}
```


Deletes a specific workspace.

**Response:** No content

### Process Workspace Nodes
```
POST /api/v1/workspaces/{workspaceId}/nodes
```


Adds or removes nodes from a workspace.

**Request Body:**
```json
{
  // List of node ids to operate on.
  "action": "add|remove",
  // List of node ids to operate on.
  "nodeIds": ["string"]
}
```
