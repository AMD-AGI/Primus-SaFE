# Workspace Access Request API

## Overview

A workspace whose `accessPolicy` is `approval` (see [Workspace API](./workspace.md)) accepts requests for member access. A user finds such workspaces in the catalog, submits a request with a reason, and an approver approves or rejects it. An approved request is a direct grant: the workspace is written to the requester's user record and stays until a manager or system administrator removes it. Requests are stored in the database; the routes answer 500 when the database is not enabled.

Common rules for every route on this page:

- A restricted (frozen) user gets 403.
- Delegated tokens are refused with 403.
- A user who holds no access yet (for example only `self-read`) is served the catalog, and may create, list, read and cancel the user's own requests; approve and reject stay closed to such a user.
- Create, approve, reject and cancel refuse a browser request sent from another site (an `Origin` that is neither the console nor the apiserver's own origin, or `Sec-Fetch-Site: cross-site`) with 403.

## Approvers

The approvers of a request are evaluated on every read and every decision:

- the direct managers of the workspace: the users in its `managers` list and the users whose record manages it. Management through a `manager` team binding or a group mapping does not count;
- system administrators, for every workspace.

A requester may not decide the user's own request, even as a manager of the workspace.

## Notifications

With notifications enabled, a new request is mailed to the direct manager with an e-mail address: the only one, or the one chosen in `notifyManager` when there are several. When the workspace has no such manager, it is mailed to the effective system administrators (in external mode, as their identity groups map them). A decision is mailed to the requester. Each mail carries the approval URL.

## Request Status

| Status | Description |
|--------|-------------|
| pending | Waiting for a decision |
| approved | Approved; the workspace was granted |
| rejected | Rejected by an approver |
| cancelled | Withdrawn by the requester |
| auto_closed | Closed by resource-manager after `workspace_access.pending_ttl` (default `336h`) without a decision; the requester is notified and may request again |

All statuses but `pending` are final. A user has at most one `pending` request per workspace.

## API List

### 1. List Workspace Catalog

List the workspaces that accept access requests, with the caller's status in each.

**Endpoint**: `GET /api/v1/workspaces/catalog`

**Authentication Required**: Yes

**Response Example**:

```json
{
  "totalCount": 1,
  "items": [
    {
      "workspaceId": "prod-cluster-ai-team",
      "workspaceName": "ai-team",
      "description": "AI team workspace",
      "managers": [{"id": "user-001", "name": "zhangsan", "email": "zhangsan@example.com"}],
      "teams": [{"id": "ml-infra", "displayName": "ML Infra", "role": "member"}],
      "myStatus": "pending",
      "pendingRequestId": 42
    }
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| workspaceId | string | Workspace ID |
| workspaceName | string | Display name of the workspace, or its ID |
| description | string | Workspace description |
| managers | []object | Direct managers of the workspace (`id`, `name`, `email`) |
| teams | []object | Teams bound to the workspace (`id`, `displayName`, `role`); `displayName` is the id when the identity service does not describe the team |
| myStatus | string | `member` when the caller already holds the workspace (directly, through a team or group, or as a system administrator), `pending` when the caller has a pending request, `none` otherwise |
| pendingRequestId | int | The caller's pending request, present when `myStatus` is `pending` |

Items are sorted by workspace ID.

---

### 2. Create Access Request

**Endpoint**: `POST /api/v1/workspace-access-requests`

**Authentication Required**: Yes

**Request Parameters**:

```json
{
  "workspace": "prod-cluster-ai-team",
  "reason": "Need GPUs for the Q4 training runs"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| workspace | string | Yes | Workspace ID |
| reason | string | No | Reason for the request, at most 2000 characters |
| notifyManager | string | Conditional | User ID of the direct manager the request is mailed to. Required when the workspace has more than one direct manager with an e-mail address; it must be one of them. Every direct manager can still approve |

**Errors**:
- 400 when `notifyManager` is missing while the workspace has several direct managers with an e-mail address, or names a user who is not one of them.
- 403 when the workspace does not exist or its `accessPolicy` is not `approval` (one message for both).
- 409 when the caller already holds the workspace, or already has a pending request for it.

**Response**: 201 Created with the request (see "Request Object").

---

### 3. List Access Requests

**Endpoint**: `GET /api/v1/workspace-access-requests`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| view | string | No | `mine` (default): every request of the caller. `approver`: the pending requests the caller may decide, without the caller's own. Any other value is 400 |

**Response Example**:

```json
{
  "totalCount": 1,
  "items": [ { "id": 42, "workspace": "prod-cluster-ai-team", "status": "pending" } ]
}
```

Each item is a "Request Object".

---

### 4. Get Access Request

**Endpoint**: `GET /api/v1/workspace-access-requests/{RequestId}`

**Authentication Required**: Yes

**Permission Requirements**: the requester or an approver. A missing request and a request the caller may not see get the same 403.

**Response**: the "Request Object" with two more fields:

| Field | Type | Description |
|-------|------|-------------|
| canDecide | bool | `true` when the caller is an approver, not the requester, and the request is pending |
| isRequester | bool | `true` when the caller is the requester |

---

### 5. Approve Access Request

**Endpoint**: `POST /api/v1/workspace-access-requests/{RequestId}/approve`

**Authentication Required**: Yes

**Request Parameters**:

```json
{"comment": "Approved for Q4"}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| comment | string | No | Decision comment, at most 2000 characters |

**Permission Requirements**: an approver who is not the requester.

The status changes in a conditional update, so of concurrent decisions only one takes effect. Approval then writes the workspace and the `default` role into the requester's user record, in builtin and in external mode, so the requester can use the workspace whatever `users.base_roles` holds. When the grant cannot be written, the request returns to pending and the error is returned.

**Errors**:
- 403 for a caller who is not an approver, or is the requester.
- 409 when the request is no longer pending, the workspace no longer exists, or its `accessPolicy` is no longer `approval`.

**Response**: 200 OK with the "Request Object".

---

### 6. Reject Access Request

**Endpoint**: `POST /api/v1/workspace-access-requests/{RequestId}/reject`

Same request body, permissions and errors as "Approve Access Request"; a workspace that no longer exists or no longer accepts requests may still have its requests rejected. Nothing is granted.

**Response**: 200 OK with the "Request Object".

---

### 7. Cancel Access Request

**Endpoint**: `POST /api/v1/workspace-access-requests/{RequestId}/cancel`

**Permission Requirements**: the requester. 409 when the request is no longer pending.

**Response**: 200 OK with the "Request Object", status `cancelled`.

---

## Request Object

```json
{
  "id": 42,
  "workspace": "prod-cluster-ai-team",
  "workspaceName": "ai-team",
  "userId": "user-003",
  "userName": "wangwu",
  "userEmail": "wangwu@example.com",
  "reason": "Need GPUs for the Q4 training runs",
  "status": "approved",
  "decidedBy": "user-001",
  "decidedByName": "zhangsan",
  "decisionComment": "Approved for Q4",
  "decidedAt": "2026-11-20T10:00:00",
  "creationTime": "2026-11-19T09:00:00",
  "updateTime": "2026-11-20T10:00:00",
  "approvalUrl": "https://console.example.com/workspace-access/requests/42"
}
```

| Field | Type | Description |
|-------|------|-------------|
| id | int | Request ID |
| workspace | string | Workspace ID |
| workspaceName | string | Workspace display name |
| userId, userName, userEmail | string | The requester |
| reason | string | Reason given by the requester |
| status | string | See "Request Status" |
| decidedBy, decidedByName | string | The user who approved, rejected or cancelled the request; empty for `pending` and `auto_closed` |
| decisionComment | string | Comment of the decision |
| decidedAt | string | Time of the decision |
| creationTime, updateTime | string | Creation and last update time |
| approvalUrl | string | Console page of the request: `<console origin>/workspace-access/requests/<id>`. The link carries no permission; the page applies the rules above |
