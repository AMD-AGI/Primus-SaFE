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
  "labels": {"key1": "value1", "key2": "value2"},
  "enablePreempt": false,
  "isDefault": true,
  "imageSecretIds": ["image-secret-001"],
  "maxRuntime": {"Authoring": 168, "Infer": 720}
}
```

**Field Description**:

| Field | Type     | Required | Description                                                                                                                              |
|-------|----------|----------|------------------------------------------------------------------------------------------------------------------------------------------|
| name | string   | Yes | Workspace name                                                                                                                           |
| clusterId | string   | Yes | The cluster which workspace belongs to                                                                                                   |
| description | string   | No | Workspace description                                                                                                                    |
| flavorId | string   | Yes | Node flavor ID                                                                                                                           |
| replica | int      | No | Expected number of nodes                                                                                                                 |
| queuePolicy | string   | No | Queue policy: fifo (first-in-first-out)/balance (load balancing), default fifo                                                           |
| scopes | []string | No | Supported service modules: Train/Infer/Authoring/CICD, no limitation if not specified                                                    |
| volumes | []object | No | Storage volume configuration list                                                                                                        |
| enablePreempt | bool     | No | Whether to enable preemption, default false.  If enabled, higher-priority workload will preempt the lower-priority one in this workspace |
| isDefault | bool     | No | Whether to set as default workspace (accessible to all users)                                                                            |
| imageSecretIds | []string | No | List of image pull secret IDs                                                                                                            |
| maxRuntime | object   | No | The maximum workload runtime of each scope, Unit: hours                                                                                    |
| labels     | object   | No  | User-defined labels (key-value pairs). Keys cannot start with "primus-safe"   |
| teams | []object | No | Teams of the identity service whose members access the workspace, see [Team Bindings](#team-bindings) |
| accessPolicy | string | No | Whether users may request direct access: `closed` (default) or `approval`, see [Workspace Access Request API](./workspace-access-request.md) |

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
| requiresPrivilegedGrant | bool | No | Mount the volume only into workloads whose creator holds the `workload/privileged` grant in the workspace (the default role grants it to workspace members; workspace administrators and system administrators hold it as well). Workloads of users without the grant run without it, and a `hostpath` entry that names its `hostPath`, a path under it or a path containing it is refused for them. Workloads created before the option was set do not carry the grant mark: they lose the volume when they are dispatched again, and one that kept such a `hostpath` entry without the hostpath grant fails to dispatch. A `hostpath` volume without the option may not overlap the `hostPath` of one with it. |

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
      "isDefault": false,
      "maxRuntime": {"Authoring": 168, "Infer": 720},
      "gpuProduct": "MI300X"
    }
  ]
}
```

**Field Description**:

Only fields not already covered by "Create Workspace" are listed below. Other fields share the same meaning as in the creation request.


| Field         | Type   | Description                                                         |
|---------------|--------|---------------------------------------------------------------------|
| totalCount    | int    | The total number of workspaces                                      |
| workspaceId   | string | Workspace ID                                                        |
| workspaceName | string | Workspace name                                                      |
| userId        | string | User id of workspace creator                                        |
| targetNodeCount | int    | The target expected number of nodes in workspace                    |
| currentNodeCount | int    | The current total number of nodes                                   |
| abnormalNodeCount | int    | The current total number of abnormal nodes                          |
| phase    | string | The status of workspace, e.g. Creating, Running, Abnormal, Deleting |
| creationTime  | string | The workspace creation time                                         |
| maxRuntime  | Object | The maximum workload runtime of each scope, Unit: hours             |
| gpuProduct  | string | GPU product for this workspace (e.g. "MI300X", "MI325X"). The default is empty.             |
| teams       | []object | Team bindings of the workspace, see [Team Bindings](#team-bindings). Omitted when there are none |
| accessPolicy | string | `closed` or `approval`. Omitted when not set, which means `closed` |
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
  "imageSecretIds": ["image-secret-001"],
  "maxRuntime": {"Authoring": 168, "Infer": 720}
}
```

**Field Description**:

Only fields not already covered by "List Workspace" are listed below. Other fields share the same meaning as in the creation request or list response.


| Field         | Type   | Description                                                           |
|---------------|--------|-----------------------------------------------------------------------|
| totalQuota    | object | Total resources in the workspace: resource names and their quantities |
| availQuota   | object | The available resource of workspace: the sum of each available node's available resources (each clamped to a minimum of 0), so it never goes negative |
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
  "imageSecretIds": ["image-secret-001", "image-secret-002"],
  "maxRuntime": {"Authoring": 168, "Infer": 720},
  "teams": [{"name": "ml-infra", "role": "member"}],
  "accessPolicy": "approval"
}
```

**Field Description**: 

All fields are optional, only provided fields will be updated

All parameters have the same meaning as the corresponding parameters in "Create Workspace". `teams` replaces the whole list of team bindings.

**Notes**:
- `teams` and `accessPolicy` are validated: a team id that is not a DNS-1123 label, a team named twice, a role other than `member` or `manager`, or an access policy other than `closed` or `approval` is rejected with 400.
- A workspace manager may add and remove `member` team bindings. Adding, removing or changing a `manager` binding, and changing `accessPolicy`, need a system administrator (403 otherwise); sending the current value of `accessPolicy` is accepted. The same rules apply to `teams` and `accessPolicy` in "Create Workspace".
- A request made with a delegated token that sets `teams` or `accessPolicy` is refused with 403.
- When an image secret is bound to a workspace (by specifying it in the workspace’s image secrets during update), the secret will also be copied into the workspace’s Kubernetes namespace to ensure image pulls can succeed. Future updates to the bound secret are synchronized accordingly.
- When you grant a user as a workspace manager, the user will also be granted access to the workspace automatically (manager implies access).

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

```json
{
  "nodeIds": ["node-004", "node-005"],
  "action": "remove",
  "force": false
}
```

**Field Description**:

| Field    | Type | Required | Description             |
|----------|------|----------|-------------------------|
| nodeIds  | []string | Yes | List of node IDs        |
| action   | string | Yes | Action type: add/remove |
| force | bool | No | Force remove nodes            |

**Response**: 200 OK with no response body

---

### 7. Update Workspace Teams

Add and remove team bindings of a workspace incrementally.

**Endpoint**: `PUT /api/v1/workspaces/{WorkspaceId}/teams`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "add": [{"name": "ml-infra", "role": "member"}, {"name": "ml-leads", "role": "manager"}],
  "remove": ["old-team"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| add | []object | No | Bindings to add, each `{name, role}`. A team that is already bound gets the given role. `role` is `member` (default) or `manager` |
| remove | []string | No | Ids of the teams to unbind |

At least one of `add` and `remove` is required, and a team may be named only once in the request (400 otherwise).

**Permission Requirements**: update right on the workspace (system administrator or workspace manager). Any added, removed or changed `manager` binding needs a system administrator. Not available to delegated tokens.

**Response Example**:

```json
{
  "teams": [{"name": "ml-infra", "role": "member"}, {"name": "ml-leads", "role": "manager"}]
}
```

`teams` is the full list of bindings after the update, always an array.

---

### 8. List Workspace Members

Get the effective members of a workspace: users with direct access, bound teams and platform groups, each tagged with its source.

**Endpoint**: `GET /api/v1/workspaces/{WorkspaceId}/members`

**Authentication Required**: Yes

**Permission Requirements**: get right on the workspace. Not available to delegated tokens.

**Response Example**:

```json
{
  "users": [
    {"id": "user-001", "name": "zhangsan", "email": "zhangsan@example.com", "role": "manager", "source": "direct"}
  ],
  "teams": [
    {
      "id": "ml-infra",
      "displayName": "ML Infra",
      "role": "member",
      "memberCount": 12,
      "managers": [{"id": "user-002", "name": "lisi", "email": "lisi@example.com"}],
      "source": "team:ml-infra"
    }
  ],
  "groups": [
    {"name": "team-a-leads", "role": "manager", "source": "group:team-a-leads"}
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| users | []object | Users whose record names the workspace, or who manage it through their record or `managers`. `role` is `manager` or `member`; `source` is always `direct`. `preferredName` is present when recorded |
| teams | []object | Teams bound to the workspace. `role` is the binding role; `source` is `team:<id>`. `displayName`, `memberCount` and `managers` come from the identity service and are omitted when it cannot describe the team. Members of a team are listed by [List Team Members](./team.md#2-list-team-members) |
| groups | []object | Platform groups whose `auth.group_mappings` entry names the workspace (external mode only). `role` is `manager` when the mapping holds `workspace-admin`, `member` otherwise; `source` is `group:<name>` |

All three fields are always arrays. Team members who never signed in to SaFE are counted in `memberCount` but have no user row.

---

### 9. Workspace Catalog and Access Requests

`GET /api/v1/workspaces/catalog` lists the workspaces whose `accessPolicy` is `approval`. It and the access request routes are described in [Workspace Access Request API](./workspace-access-request.md).

---

## Team Bindings

`teams` binds teams of the identity service to the workspace. Each entry is:

| Field | Type | Description |
|-------|------|-------------|
| name | string | Team id in the identity service, a DNS-1123 label |
| role | string | `member` (default when empty) or `manager` |

- In external mode the members of a bound team access the workspace on every request; a `manager` binding makes every member of the team a manager of the workspace. The access is computed per request from the identity's teams and is never written to the user record, so leaving the team revokes it. See [External Authentication](./external-authentication.md#teams).
- In builtin mode bindings can be stored but grant nothing.
- A workspace manager may add and remove `member` bindings; a `manager` binding needs a system administrator. The bindings are changed by "Update Workspace Teams", by `teams` in "Create Workspace" and "Update Workspace", and from the team side by [Bind Team to Workspaces](./team.md#3-bind-team-to-workspaces).
- When the identity service deletes a team, its bindings are removed from every workspace.

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
- **availQuota**: Available quota. Computed as the sum over the workspace's
  available nodes (whose flavor matches the workspace flavor) of each node's
  available resources, where a node's available = node allocatable − system
  reserve (e.g. `cpu_reserve_percent`) − resources in use on that node, clamped
  to a minimum of 0. Clamping per node (instead of subtracting the aggregate
  used from the aggregate available) keeps an over-committed node from
  cancelling spare capacity on other nodes, so this value never goes negative
  and equals the sum of the per-node available reported by the node APIs.
- **usedQuota**: Quota used by workloads
- **abnormalQuota**: Quota occupied by abnormal nodes

## Notes

1. **Workspace Naming**: Actual ID is `{clusterId}-{name}`
2. **Node Flavor**: One workspace can only use one node flavor
3. **Default Workspace**: When set as default, all users can access it
4. **Manager Permissions**: Managers can manage all resources in the workspace
5. **Preemption Mechanism**: When enabled, high priority tasks can preempt low priority task resources
