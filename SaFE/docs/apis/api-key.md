# API Key API

## Overview

The API Key API provides management capabilities for programmatic access to the Primus-SaFE system. API Keys enable secure authentication for automated scripts, CI/CD pipelines, and third-party integrations without requiring user credentials. Each API Key is bound to a specific user and inherits the user's permissions.

### Core Concepts

An API Key is a credential for programmatic access, with the following key characteristics:

* **Secure Generation**: Each API Key is cryptographically generated with the `ak-` prefix, ensuring uniqueness and security.
* **User Binding**: API Keys are bound to the creating user and inherit the user's permissions.
* **Time-Limited**: Each key has a configurable TTL (Time To Live), with a maximum of 366 days.
* **IP Whitelisting**: Optional IP address or CIDR range restrictions for enhanced security.
* **Soft Deletion**: Deleted keys are marked as deleted rather than physically removed, enabling audit trails.

### Authentication

API Keys use the standard `Authorization: Bearer` header:

```bash
curl -H "Authorization: Bearer ak-your-api-key-here" https://api.example.com/api/v1/workloads
```

## API List

### 1. Create API Key

Create a new API Key for programmatic access.

**Endpoint**: `POST /api/v1/apikeys`

**Authentication Required**: Yes (User Token or Cookie)

**Request Example**:

```json
{
  "name": "ci-cd-pipeline",
  "ttlDays": 90,
  "whitelist": ["192.168.1.0/24", "10.0.0.1"]
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Display name for the API Key (max 100 characters) |
| ttlDays | int | Yes | Validity period in days (1-366) |
| whitelist | []string | No | List of allowed IP addresses or CIDR ranges |

**Response Example**:

```json
{
  "id": 123,
  "name": "ci-cd-pipeline",
  "userId": "user-zhangsan-abc123",
  "apiKey": "ak-dGVzdC1rZXktMTIzNDU2Nzg5MA...",
  "expirationTime": "2026-04-07T08:00:00Z",
  "creationTime": "2026-01-07T08:00:00Z",
  "whitelist": ["192.168.1.0/24", "10.0.0.1"],
  "deleted": false
}
```

**Response Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| id | int64 | Unique identifier for the API Key |
| name | string | Display name |
| userId | string | Owner user ID |
| apiKey | string | **The actual API Key value (only returned during creation!)** |
| expirationTime | string | Expiration time (RFC3339 format) |
| creationTime | string | Creation time (RFC3339 format) |
| whitelist | []string | Allowed IP addresses/CIDRs |
| deleted | bool | Deletion status |

> ⚠️ **Important**: The `apiKey` field is **only returned once** during creation. Store it securely as it cannot be retrieved again.

---

### 2. List API Keys

List all API Keys for the authenticated user with pagination support.

**Endpoint**: `GET /api/v1/apikeys`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| offset | int | No | 0 | Pagination offset |
| limit | int | No | 100 | Records per page |
| sortBy | string | No | creationTime | Sort field (creationTime, expirationTime) |
| order | string | No | desc | Sort order (desc, asc) |

**Response Example**:

```json
{
  "totalCount": 2,
  "items": [
    {
      "id": 123,
      "name": "ci-cd-pipeline",
      "userId": "user-zhangsan-abc123",
      "expirationTime": "2026-04-07T08:00:00Z",
      "creationTime": "2026-01-07T08:00:00Z",
      "whitelist": ["192.168.1.0/24"],
      "deleted": false,
      "deletionTime": null
    },
    {
      "id": 124,
      "name": "monitoring-script",
      "userId": "user-zhangsan-abc123",
      "expirationTime": "2026-02-07T08:00:00Z",
      "creationTime": "2026-01-07T10:00:00Z",
      "whitelist": [],
      "deleted": false,
      "deletionTime": null
    }
  ]
}
```

**Response Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of API Keys |
| items | []object | List of API Key objects |

Each item contains:

| Field | Type | Description |
|-------|------|-------------|
| id | int64 | Unique identifier |
| name | string | Display name |
| userId | string | Owner user ID |
| expirationTime | string | Expiration time (RFC3339 format) |
| creationTime | string | Creation time (RFC3339 format) |
| whitelist | []string | Allowed IP addresses/CIDRs |
| deleted | bool | Deletion status |
| deletionTime | string/null | Deletion time (RFC3339 format, null if not deleted) |

> 🔒 **Security Note**: The list API does **not** return the actual API Key values for security reasons.

---

### 3. Delete API Key

Perform soft deletion on an API Key.

**Endpoint**: `DELETE /api/v1/apikeys/:id`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| id | API Key ID (numeric) |

**Response**: 200 OK with empty response body `{}`

**Error Responses**:

| Status | Error Code | Description |
|--------|------------|-------------|
| 400 | Primus.00002 | Invalid ID format |
| 403 | Primus.00003 | API Key belongs to another user |
| 404 | Primus.00005 | API Key not found |

---

## Using API Keys

### Authentication Method

Use the `Authorization: Bearer` header with your API Key:

```bash
# Using API Key to list workloads
curl -X GET "https://api.example.com/api/v1/workloads?workspaceId=workspace-001" \
  -H "Authorization: Bearer ak-your-api-key-here"

# Using API Key to create a workload
curl -X POST "https://api.example.com/api/v1/workloads" \
  -H "Authorization: Bearer ak-your-api-key-here" \
  -H "Content-Type: application/json" \
  -d '{
    "displayName": "my-job",
    "workspaceId": "workspace-001",
    ...
  }'
```

### API Key vs User Token

| Feature | API Key | User Token |
|---------|---------|------------|
| Prefix | `ak-` | No specific prefix |
| Obtained via | Create API Key endpoint | User login |
| Validity | Configurable (1-366 days) | Session-based (typically hours) |
| Use case | Automation, CI/CD, scripts | Interactive web console |
| IP restriction | Supported (whitelist) | Not supported |
| Can be revoked | Yes (soft delete) | Logout invalidates |

### IP Whitelist

When an API Key has a non-empty whitelist, requests are only allowed from the specified IP addresses or CIDR ranges:

```json
{
  "whitelist": [
    "192.168.1.100",      // Single IP
    "10.0.0.0/8",         // CIDR range
    "2001:db8::1",        // IPv6 address
    "2001:db8::/32"       // IPv6 CIDR
  ]
}
```

If the whitelist is empty or not specified, the API Key can be used from any IP address.

---

## Best Practices

### 1. Key Management
- **Use descriptive names**: Name keys based on their purpose (e.g., "github-actions-deploy", "monitoring-cron")
- **Rotate regularly**: Set appropriate TTL and rotate keys before expiration
- **Minimal permissions**: Create keys with the minimum required permissions

### 2. Security
- **Never commit keys**: Do not commit API Keys to version control
- **Use environment variables**: Store keys in environment variables or secret management systems
- **Enable IP whitelisting**: Restrict keys to known IP addresses when possible
- **Audit usage**: Regularly review and delete unused keys

### 3. Error Handling
- **Handle expiration**: Implement logic to detect and refresh expired keys
- **Handle IP blocks**: Ensure client IPs are in the whitelist if configured

---

## Error Responses

### API Key Authentication Errors

| Error Message | Description |
|---------------|-------------|
| `invalid API key` | The provided API Key does not exist |
| `API key deleted` | The API Key has been deleted |
| `API key expired` | The API Key has expired |
| `IP not allowed` | Client IP is not in the whitelist |

### Management API Errors

| Error Code | HTTP Status | Description |
|------------|-------------|-------------|
| Primus.00002 | 400 | Invalid request (empty name, invalid TTL, etc.) |
| Primus.00003 | 403 | Permission denied (key belongs to another user) |
| Primus.00005 | 404 | API Key not found |
| Primus.00009 | 401 | Authentication required |

---

## Notes

1. **One-time display**: The API Key value is only shown once during creation. Store it securely.
2. **Secure storage**: API Keys are stored as HMAC-SHA256 hashes in the database, using the system crypto secret as the HMAC key. Even if the database is compromised, the original keys cannot be recovered without access to the crypto secret.
3. **User permissions**: API Keys inherit the permissions of the creating user.
4. **Soft deletion**: Deleted keys cannot be recovered or reused.
5. **Key format**: All API Keys start with the `ak-` prefix.
6. **TTL limits**: Maximum validity period is 366 days.
7. **Name duplication**: Multiple keys can have the same name (for user convenience).

