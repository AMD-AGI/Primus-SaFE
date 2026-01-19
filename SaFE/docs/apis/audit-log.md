# Audit Log API

## Overview

The Audit Log API provides read-only access to system operation records for security compliance and operational monitoring. All write operations (POST, PUT, PATCH, DELETE) in the system are automatically recorded, enabling administrators to track user activities, investigate security incidents, and maintain compliance audit trails.

### Core Concepts

Audit logs capture the following information for each write operation:

* **User Identity**: Who performed the operation (userId, userName, userType)
* **Operation Details**: What was done (HTTP method, request path, action description)
* **Resource Information**: Which resource was affected (resourceType, resourceName)
* **Request/Response**: Request body (with sensitive data redacted) and response status
* **Timing**: When the operation occurred and how long it took
* **Tracing**: Distributed tracing ID for cross-service correlation

### Access Control

> ⚠️ **Admin Only**: The Audit Log API is restricted to system administrators. Regular users cannot access audit logs.

## API List

### List Audit Logs

Query audit logs with flexible filtering, sorting, and pagination support.

**Endpoint**: `GET /api/v1/auditlogs`

**Authentication Required**: Yes (Admin role required)

**Query Parameters**:

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| offset | int | No | 0 | Pagination offset |
| limit | int | No | 100 | Records per page (max 100) |
| sortBy | string | No | create_time | Sort field (create_time, user_id) |
| order | string | No | desc | Sort order (desc, asc) |
| userId | string | No | - | Filter by user ID (exact match) |
| userName | string | No | - | Filter by user name (partial match) |
| userType | string | No | - | Filter by user type (comma-separated, e.g., "default,sso") |
| resourceType | string | No | - | Filter by resource type (comma-separated, e.g., "workloads,apikeys") |
| resourceName | string | No | - | Filter by resource name (partial match) |
| httpMethod | string | No | - | Filter by HTTP method (comma-separated, e.g., "POST,DELETE") |
| requestPath | string | No | - | Filter by request path (partial match) |
| startTime | string | No | - | Start time filter (RFC3339 format) |
| endTime | string | No | - | End time filter (RFC3339 format) |
| responseStatus | int | No | - | Filter by HTTP response status code |

**Request Examples**:

```bash
# Get latest 20 audit logs
GET /api/v1/auditlogs?limit=20

# Filter by user name
GET /api/v1/auditlogs?userName=admin

# Filter by multiple user types
GET /api/v1/auditlogs?userType=default,sso

# Filter by resource type and HTTP method
GET /api/v1/auditlogs?resourceType=workloads,apikeys&httpMethod=POST,DELETE

# Filter by time range
GET /api/v1/auditlogs?startTime=2026-01-01T00:00:00Z&endTime=2026-01-31T23:59:59Z

# Combined filters
GET /api/v1/auditlogs?userName=admin&resourceType=workloads&httpMethod=DELETE&limit=50
```

**Response Example**:

```json
{
  "totalCount": 156,
  "items": [
    {
      "id": 1001,
      "userId": "a01e7b83f5661e327503f0eacbfef97d",
      "userName": "zhangsan",
      "userType": "default",
      "clientIp": "10.176.17.167",
      "action": "create workload",
      "httpMethod": "POST",
      "requestPath": "/api/v1/workloads",
      "resourceType": "workloads",
      "resourceName": "",
      "requestBody": "{\"name\": \"my-training-job\", \"image\": \"pytorch:latest\"}",
      "responseStatus": 200,
      "latencyMs": 256,
      "traceId": "7b2d2cf552969247e747c55142b911a7",
      "createTime": "2026-01-17T10:30:45Z"
    },
    {
      "id": 1000,
      "userId": "b02f8c94g6772f438614g1fbdcgf08d8",
      "userName": "lisi",
      "userType": "sso",
      "clientIp": "10.176.17.200",
      "action": "delete apikey",
      "httpMethod": "DELETE",
      "requestPath": "/api/v1/apikeys/42",
      "resourceType": "apikeys",
      "resourceName": "42",
      "responseStatus": 200,
      "latencyMs": 15,
      "traceId": "8c3e3dg663070358f858d66253c022b8",
      "createTime": "2026-01-17T10:25:12Z"
    }
  ]
}
```

**Response Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of records matching the query |
| items | array | List of audit log entries |
| items[].id | int64 | Unique identifier for the audit log entry |
| items[].userId | string | ID of the user who performed the operation |
| items[].userName | string | Display name of the user |
| items[].userType | string | User authentication type (default/sso/apikey) |
| items[].clientIp | string | Client IP address |
| items[].action | string | Human-readable action description |
| items[].httpMethod | string | HTTP method (POST/PUT/PATCH/DELETE) |
| items[].requestPath | string | Full request URL path |
| items[].resourceType | string | Type of resource being operated on |
| items[].resourceName | string | Specific resource identifier (if applicable) |
| items[].requestBody | string | Request body (sensitive data redacted) |
| items[].responseStatus | int | HTTP response status code |
| items[].latencyMs | int64 | Request processing time in milliseconds |
| items[].traceId | string | Distributed tracing ID |
| items[].createTime | string | Timestamp when the operation occurred (RFC3339) |

---

## Filter Parameters Reference

### userType - User Types

Supports multiple values (comma-separated).

| Value | Description |
|-------|-------------|
| `default` | Standard user (username/password login) |
| `sso` | SSO single sign-on user |
| `apikey` | API Key authentication |

**Example**: `?userType=default,sso`

---

### httpMethod - HTTP Methods

Supports multiple values (comma-separated).

| Value | Description |
|-------|-------------|
| `POST` | Create operations |
| `PUT` | Replace operations |
| `PATCH` | Update operations |
| `DELETE` | Delete operations |

**Example**: `?httpMethod=POST,DELETE`

---

### resourceType - Resource Types

Supports multiple values (comma-separated).

#### Authentication

| Value | Description |
|-------|-------------|
| `login` | User login |
| `logout` | User logout |
| `auth` | Token verification (internal) |

#### User & Access

| Value | Description |
|-------|-------------|
| `users` | User management |
| `apikeys` | API Key management |
| `publickeys` | SSH public key management |

#### Compute Resources

| Value | Description |
|-------|-------------|
| `workloads` | Workloads (create, delete, stop, clone) |
| `nodes` | Node management |
| `clusters` | Cluster management |
| `workspaces` | Workspace management |
| `nodetemplates` | Node templates |
| `nodeflavors` | Node flavors/specs |

#### Operations

| Value | Description |
|-------|-------------|
| `opsjobs` | Operations jobs |
| `faults` | Fault records |
| `addons` | Cluster addons |
| `service` | Service logs |

#### Security

| Value | Description |
|-------|-------------|
| `secrets` | Kubernetes secrets |

#### Images

| Value | Description |
|-------|-------------|
| `images` | Image management |
| `images:import` | Image import |
| `image-registries` | Image registry management |

#### CD (Continuous Deployment)

| Value | Description |
|-------|-------------|
| `deployments` | Deployment requests (create, approve, rollback) |

#### AI Playground

| Value | Description |
|-------|-------------|
| `playground` | Playground features (chat, sessions, models) |
| `datasets` | Dataset management |

**Example**: `?resourceType=workloads,nodes,clusters`

---

## Action Description Format

The `action` field provides a human-readable description of the operation:

| HTTP Method | Action Format | Example |
|-------------|---------------|---------|
| POST | `create {resource}` | `create workload` |
| PUT | `replace {resource}` | `replace secret` |
| PATCH | `update {resource}` | `update workspace` |
| DELETE | `delete {resource}` | `delete apikey` |

**Special Cases**:

| Resource | Action |
|----------|--------|
| login | `login` |
| logout | `logout` |
| CD approve | `approve deployment` |
| CD rollback | `rollback deployment` |
| workload stop | `stop workload` |
| workload clone | `clone workload` |

---

## Error Responses

| HTTP Status | Error Code | Description |
|-------------|------------|-------------|
| 401 | Unauthorized | Not authenticated |
| 403 | Forbidden | User is not an administrator |
| 400 | Bad Request | Invalid query parameters |
| 500 | Internal Server Error | Database query failed |

**Error Response Example**:

```json
{
  "errorCode": "Primus.00003",
  "errorMessage": "forbidden: user does not have permission to access audit logs"
}
```

---

## Notes

1. **Sensitive Data Redaction**: Request bodies containing `password`, `token`, `secret`, `apiKey`, or `api_key` fields are automatically redacted to `[REDACTED]`.

2. **Login/Logout Auditing**: Login and logout operations are audited separately with detailed information:
   - Successful login: Records actual user identity
   - Failed login: Records as `login-failed:{username}`
   - Logout: Records user identity from session cookies

3. **Batch Operations**: For batch operations (e.g., batch delete nodes), `resourceName` will be empty as the specific resource IDs are in the request body.

4. **Time Range Queries**: For optimal performance, always use `startTime` and `endTime` filters when querying historical data.

5. **Trace ID**: The `traceId` field enables correlation with distributed tracing systems (e.g., Jaeger) for debugging complex request flows.
