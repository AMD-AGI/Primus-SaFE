# User API

User management API provides user registration, authentication, and permission management capabilities.

## API List

### 1. User Registration

Create a new user account.

**Endpoint**: `POST /api/v1/users`

Served when `auth.mode` is `builtin`. External mode does not register this route.

**Authentication Required**: No (Public endpoint)

**Request Parameters**:

```json
{
  "name": "zhangsan",
  "email": "zhangsan@example.com",
  "password": "your-password",
  "avatarUrl": "https://example.com/avatar.jpg"
}
```

**Field Description**:

| Field | Type | Required | Description                                                      |
|-------|------|----------|------------------------------------------------------------------|
| name | string | Yes | Username (unique)                                                |
| email | string | No | Email address                                                    |
| password | string | Yes* | Password (required for regular user registration)                |
| avatarUrl | string | No | Avatar URL                                                       |

**Response Example**:

```json
{
  "id": "user-zhangsan-abc123"
}
```

---

### 2. User Login

User authentication and access token retrieval.

**Endpoint**: `POST /api/v1/login`

**Authentication Required**: No (Public endpoint)

**Request Example (password)**:
```json
{
  "name": "zhangsan",
  "password": "your-password",
  "type": "default"
}
```

**Request Example (code)**:
```json
{
  "code": "abc123",
  "type": "sso"
}
```
**Field Description**:

| Field    | Type | Required | Description             |
|----------|------|----------|-------------------------|
| name     | string | Conditionally | Username                |
| password | string | Conditionally | User Password           |
| type     | string | No | Login type: default/sso |
| code     | string | Conditionally |  Authorization code returned by OAuth2 server, used to exchange for access token and ID token      |
| codeVerifier | string | No | The PKCE code verifier (RFC 7636) of a code the console asked for at the `sso.login` provider (form field `code_verifier`). A code that comes with one is exchanged at that provider as a public client; a code without one is exchanged at the provider of the `sso` Secret, as before. |

**Response Example**:

```json
{
  "id": "user-zhangsan-abc123",
  "name": "zhangsan",
  "email": "zhangsan@example.com",
  "type": "default",
  "roles": ["default"],
  "workspaces": [
    {
      "id": "prod-cluster-ai-team",
      "name": "ai-team"
    }
  ],
  "managedWorkspaces": [],
  "creationTime": "2025-01-10T08:00:00",
  "restrictedType": 0,
  "avatarUrl": "https://example.com/avatar.jpg",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expire": 1736505600
}
```

**Response Field Description**:

| Field | Type | Description                                                   |
|-------|------|---------------------------------------------------------------|
| id | string | User ID                                                       |
| name | string | Username                                                      |
| email | string | Email address                                                 |
| preferredName | string | Preferred username from the SSO identity provider (the `preferred_username` claim), recorded when the user signs in through SSO. Omitted when none is recorded, e.g. for a default user |
| type | string | User type: default/sso                                        |
| roles | []string | User roles. e.g. system-admin, system-admin-readonly, default, self-read (reads the user's own record only) |
| workspaces | []object | Workspaces the user can access (non-admin only)               |
| managedWorkspaces | []object | Workspaces the user can manage (non-admin only)               |
| creationTime | string | User creation time (RFC3339)                                  |
| restrictedType | int | Restriction: 0 normal, 1 frozen                               |
| avatarUrl | string | Avatar URL                                                    |
| token | string | User token, encrypted internally. Empty with `auth.mode: external`: the browser keeps the identity provider's session cookie, and the response sets no cookie |
| expire | int | Token expire time (Unix seconds); with `auth.mode: external`, the end of the identity provider's session (0 when it reports none) |

Nested workspace object:

| Field | Type | Description |
|-------|------|-------------|
| id | string | Workspace ID |
| name | string | Workspace name |

---

### 3. User Logout

Clear user session (Web Console only).

**Endpoint**: `POST /api/v1/logout`

**Authentication Required**: No

**Response**: 200 OK with no response body

---

### 4. List Users

Get user list with filtering support.

**Endpoint**: `GET /api/v1/users`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| name | string | No | Filter by username (URL encoded) |
| email | string | No | Filter by email (URL encoded) |
| workspaceId | string | No | Filter by workspace (returns users with access to this workspace) |

**Response Example**:

```json
{
  "totalCount": 10,
  "items": [
    {
      "id": "user-zhangsan-abc123",
      "name": "zhangsan",
      "email": "zhangsan@example.com",
      "type": "default",
      "roles": ["default"],
      "workspaces": [
        {
          "id": "prod-cluster-ai-team",
          "name": "ai-team"
        }
      ],
      "managedWorkspaces": [
        {
          "id": "prod-cluster-dev-team",
          "name": "dev-team"
        }
      ],
      "creationTime": "2025-01-10T08:00:00",
      "restrictedType": 0,
      "avatarUrl": "https://example.com/avatar.jpg",
      "sources": ["direct", "team:ml-infra"]
    }
  ]
}
```
**Field Description**:

The response fields are consistent with the "User Login" response above, plus:

| Field | Type | Description |
|-------|------|-------------|
| sources | []string | Present only when the list is filtered by `workspaceId`: how the user reaches that workspace. `direct` when the user record names it; `team:<id>` for each team recorded on the user (pushed by the identity service through SCIM) that is bound to the workspace. Team sources apply in external mode with SCIM enabled |

With `workspaceId`, the list holds the users that have a `direct` or a `team:<id>` source for the workspace. Users who reach the workspace only through a team, and the `team:<id>` sources, are listed only to callers who may get that workspace. Team members who have no SaFE user record yet are not listed; use [List Workspace Members](./workspace.md#8-list-workspace-members) for the full member view. `workspaces` of each row remains the user's direct grants.


---

### 5. Get User Details

Get detailed information about a specific user or current user.

**Endpoint**: `GET /api/v1/users/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | User ID, or use `self` to get current user information |

**Response Example**:

```json
{
  "id": "user-zhangsan-abc123",
  "name": "zhangsan",
  "email": "zhangsan@example.com",
  "type": "default",
  "roles": ["default"],
  "workspaces": [
    {
      "id": "prod-cluster-ai-team",
      "name": "ai-team"
    }
  ],
  "managedWorkspaces": [],
  "creationTime": "2025-01-10T08:00:00",
  "restrictedType": 0,
  "avatarUrl": "https://example.com/avatar.jpg"
}
```
**Field Description**:

The response fields are consistent with the "User Login" response above.

---

### 6. Update User

Update user information or permissions.

**Endpoint**: `PATCH /api/v1/users/{UserId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| UserId | User ID |

**Request Parameters**:

```json
{
  "roles": ["system-admin"],
  "workspaces": ["workspace-001", "workspace-002"],
  "avatarUrl": "https://example.com/new-avatar.jpg",
  "password": "NewPassword456!",
  "restrictedType": 0,
  "email": "newemail@example.com"
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| roles | []string | No | User roles list |
| workspaces | []string | No | List of accessible workspace IDs, replaces the whole list |
| addWorkspaces | []string | No | Workspace IDs to add to the accessible workspaces; exclusive with `workspaces` |
| removeWorkspaces | []string | No | Workspace IDs to remove from the accessible workspaces; exclusive with `workspaces` |
| avatarUrl | string | No | Avatar URL |
| password | string | No | New password |
| restrictedType | int | No | Restriction type: 0 (normal)/1 (frozen) |
| email | string | No | Email address |

When `auth.mode` is `external`, a change of `restrictedType` between normal (`0`) and frozen (`1`) is sent to the identity service first: `PUT /api/v1/identities/{UserId}/status` with `{"status":"disabled"}` when freezing and `{"status":"active"}` when unfreezing. The call uses the caller's bearer credential. The local user is updated only after that call succeeds. If the identity service cannot be reached the response is 503; if it refuses, 502. Clearing a freeze also removes the annotation `primus-safe.scim.deactivated`. Builtin mode updates the local user only. See [External Authentication](external-authentication.md).

**Permission Requirements**:
- Updating roles and workspaces requires system admin permission
- `addWorkspaces` and `removeWorkspaces` are checked against the named workspaces only: the caller must be a system admin or manage every workspace named in the request. The two lists must not share an ID, and an empty ID or a request that also carries `workspaces` is rejected with 400
- Users can update their own password, email and avatar

**Response**: 200 OK with no response body

---

### 7. Delete User

Delete a specific user.

**Endpoint**: `DELETE /api/v1/users/{UserId}`

Served when `auth.mode` is `builtin`. The route removes the user from workspaces the user manages and deletes the local user. External mode does not register this route.

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| UserId | User ID |

**Permission Requirements**: System administrator

**Response**: 200 OK with no response body

---

### 8. Get Own Workspaces

Get the workspaces the current user can access and the ones the current user manages. The lists are the `workspaces` and `managedWorkspaces` fields of "Get User Details", served on their own: a client that learns who the user is from another source (for example external authentication) can still ask SaFE which workspaces the user can reach.

**Endpoint**: `GET /api/v1/users/self/workspaces`

**Authentication Required**: Yes

**Permission Requirements**: None beyond being signed in; the response is always about the caller. Another user's lists are served by "Get User Workspaces".

**Response Example**:

```json
{
  "workspaces": [
    {
      "id": "prod-cluster-ai-team",
      "name": "ai-team",
      "sources": ["direct", "team:ml-infra"]
    }
  ],
  "managedWorkspaces": []
}
```

**Field Description**:

| Field | Description |
|-------|-------------|
| workspaces | The workspaces the user can access, each with its ID, display name and sources |
| managedWorkspaces | The workspaces the user manages, each with its ID, display name and sources |
| sources | How the user reaches the workspace, in this order: `direct` (the user record, or for `managedWorkspaces` also the workspace's `managers`), then the sorted `group:<name>` (a group mapping) and `team:<id>` (a team binding) sources, then, in `workspaces` only, `default` (a default workspace while `users.join_default_workspaces` is on) |

Both fields are always arrays. A system administrator reaches every workspace without being listed in any, so both lists are empty for one; check `roles` first. A workspace that no longer exists is left out. When `auth.mode` is `external`, the lists include the workspaces the groups and teams of the request map to (see [External Authentication](external-authentication.md)).

---

### 9. Get User Workspaces

Get the effective workspaces of another user, with the sources of each.

**Endpoint**: `GET /api/v1/users/{UserId}/workspaces`

**Authentication Required**: Yes

**Permission Requirements**: system administrator, or a workspace manager for the workspaces the caller manages. Each workspace is returned only when the caller may update the user's access to it; when none is visible, the caller needs that right without a workspace restriction, otherwise 403. Not available to delegated tokens.

**Response**: the shape of "Get Own Workspaces". The groups and teams are those the identity service recorded on the user through SCIM, mapped as for a request (external mode with SCIM enabled); otherwise only the user record and default workspaces count. Both lists are empty for a target who is a system administrator.

---

## User Types

| Type    | Description                                         |
|---------|-----------------------------------------------------|
| default | Default user, uses username/password authentication |
| sso     | Enterprise user, uses third-party authentication    |

## User Roles

| Role              | Description                    | Permissions                                             |
|-------------------|--------------------------------|---------------------------------------------------------|
| system-admin      | System administrator           | Full control, can manage all resources                  |
| system-admin-only | System administrator read-only | Can view all resources, but cannot perform operations such as creating, updating, or deleting                            |
| default           | Regular user                   | Can only access authorized workspaces or other resource |

## User Restriction Types

| Type | Description |
|------|-------------|
| 0 | Normal status |
| 1 | Frozen status (cannot use system) |

## Workspace Permissions

### Regular Access (workspaces)
Users can in these workspaces:
- Submit and manage their own workloads
- Can view others' workloads in the same workspace, but not modify or delete them.
- View workspace information and resource usage
- Can only create workloads with low or medium priority

### Manager Permissions (managedWorkspaces)
Workspace managers can:
- Manage all users' workloads in the workspace, including deleting workloads
- Modify workspace configuration
- Grant another user permission to access the workspace 
- Add and remove `member` team bindings of the workspace
- Approve and reject access requests to the workspace, when they manage it directly (see [Workspace Access Request API](./workspace-access-request.md))
- Can create workloads with the highest priority

## Token

- **Format**: Custom token, encrypted internally
- **Validity**: Specified in `expire` field in login response (Unix timestamp)
- **Usage**: Add `Authorization: Bearer <token>` in request header
- **Storage**: Web Console uses Cookie for automatic management, API calls need manual management

## Notes

1. **Username Uniqueness**: Username must be unique in the system
2. **Password Security**: Recommend using strong passwords, system stores Base64 encoded passwords
3. **Permission Inheritance**: System administrators have all permissions, no workspace configuration needed
4. **Self-Registration**: Regular users have no workspace(except for public workspaces) access permissions after registration, need admin authorization
