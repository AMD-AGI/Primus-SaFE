# User API

User management API provides user registration, authentication, and permission management capabilities.

## API List

### 1. User Registration

Create a new user account.

**Endpoint**: `POST /api/custom/users`

**Authentication Required**: No (Public endpoint)

**Request Parameters**:

```json
{
  "name": "zhangsan",
  "email": "zhangsan@example.com",
  "password": "SecurePassword123!",
  "type": "default",
  "workspaces": [],
  "avatarUrl": "https://example.com/avatar.jpg"
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Username (unique) |
| email | string | No | Email address |
| password | string | Yes* | Password (required for regular user registration) |
| type | string | No | User type: default/teams, only system admin can specify |
| workspaces | []string | No | List of accessible workspace IDs (only system admin can specify) |
| avatarUrl | string | No | Avatar URL |

**Response Example**:

```json
{
  "id": "user-zhangsan-abc123"
}
```

---

### 2. User Login

User authentication and access token retrieval.

**Endpoint**: `POST /api/custom/login`

**Authentication Required**: No (Public endpoint)

**Request Parameters**:

```json
{
  "name": "zhangsan",
  "password": "SecurePassword123!",
  "type": "default"
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Username |
| password | string | Yes | Password |
| type | string | No | Login type: default/teams |

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
  "creationTime": "2025-01-10T08:00:00.000Z",
  "restrictedType": 0,
  "avatarUrl": "https://example.com/avatar.jpg",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expire": 1736505600
}
```

---

### 3. User Logout

Clear user session (Web Console only).

**Endpoint**: `POST /api/custom/logout`

**Authentication Required**: No

**Response**: No content (204)

---

### 4. List Users

Get user list with filtering support.

**Endpoint**: `GET /api/custom/users`

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
      "creationTime": "2025-01-10T08:00:00.000Z",
      "restrictedType": 0,
      "avatarUrl": "https://example.com/avatar.jpg"
    }
  ]
}
```

---

### 5. Get User Details

Get detailed information about a specific user or current user.

**Endpoint**: `GET /api/custom/users/:name`

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
  "creationTime": "2025-01-10T08:00:00.000Z",
  "restrictedType": 0,
  "avatarUrl": "https://example.com/avatar.jpg"
}
```

---

### 6. Update User

Update user information or permissions.

**Endpoint**: `PATCH /api/custom/users/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | User ID |

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
| workspaces | []string | No | List of accessible workspace IDs |
| avatarUrl | string | No | Avatar URL |
| password | string | No | New password |
| restrictedType | int | No | Restriction type: 0 (normal)/1 (frozen) |
| email | string | No | Email address |

**Permission Requirements**:
- Updating roles and workspaces requires system admin permission
- Users can update their own password, email and avatar

**Response**: No content (204)

---

### 7. Delete User

Delete a specific user.

**Endpoint**: `DELETE /api/custom/users/:name`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| name | User ID |

**Permission Requirements**: System administrator

**Response**: No content (204)

---

## User Types

| Type | Description |
|------|-------------|
| default | Default user, uses username/password authentication |
| teams | Enterprise user, uses third-party authentication |

## User Roles

| Role | Description | Permissions |
|------|-------------|-------------|
| system-admin | System administrator | Full control, can manage all resources |
| default | Regular user | Can only access authorized workspaces |

## User Restriction Types

| Type | Description |
|------|-------------|
| 0 | Normal status |
| 1 | Frozen status (cannot login or use system) |

## Workspace Permissions

### Regular Access (workspaces)
Users can in these workspaces:
- Submit and manage their own workloads
- View workspace information and resource usage

### Manager Permissions (managedWorkspaces)
Workspace managers can:
- Manage all users' workloads in the workspace
- Modify workspace configuration
- View all users' resource usage

## Token

- **Format**: Base64-encoded JWT token
- **Validity**: Specified in `expire` field in login response (Unix timestamp)
- **Usage**: Add `Authorization: Bearer <token>` in request header
- **Storage**: Web Console uses Cookie for automatic management, API calls need manual management

## Notes

1. **Username Uniqueness**: Username must be unique in the system
2. **Password Security**: Recommend using strong passwords, system stores Base64 encoded passwords
3. **Permission Inheritance**: System administrators have all permissions, no workspace configuration needed
4. **Self-Registration**: Regular users have no workspace access permissions after registration, need admin authorization
5. **Email Verification**: System uses MD5 hash of email as identifier for quick lookup
