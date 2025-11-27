# Secret API

Secret management API for managing SSH keys and image registry authentication information.

## API List

### 1. Create Secret

Create new SSH key or image registry secret.

**Endpoint**: `POST /api/v1/secrets`

**Authentication Required**: Yes

**SSH Key Request Example**:

```json
{
  "name": "my-ssh-key",
  "type": "ssh",
  "params": [
    {
      "username": "root",
      "password": "MyPassword123!",
      "privateKey": "",
      "publicKey": ""
    }
  ]
}
```

**SSH Key (Key Pair) Request Example**:

```json
{
  "name": "my-ssh-keypair",
  "type": "ssh",
  "params": [
    {
      "username": "root",
      "password": "",
      "privateKey": "LS0tLS1CRUdJTi...(Base64 encoded private key)",
      "publicKey": "c3NoLXJzYSBBQUFB...(Base64 encoded public key)"
    }
  ]
}
```

**Image Registry Secret Request Example**:

```json
{
  "name": "harbor-secret",
  "type": "image",
  "workspaceIds": ["dev"],
  "params": [
    {
      "server": "harbor.example.com",
      "username": "admin",
      "password": "SGFyYm9yMTIzNDU=(Base64 encoded password)"
    },
    {
      "server": "docker.io",
      "username": "dockeruser",
      "password": "RG9ja2VyMTIzNDU=(Base64 encoded password)"
    }
  ]
}
```

**GitHub PAT Secret Request Example**:

```json
{
  "name": "github-secret",
  "type": "general",
  "workspaceIds": ["dev"],
  "params": [{
        "github_token": "your github token"
  }]
}
```

**Field Description**:

| Field | Type | Required | Description                                       |
|-------|------|----------|---------------------------------------------------|
| name | string | Yes | Secret name                                       |
| type | string | Yes | Secret type: ssh/image/general                            |
| workspaceIds | []string | No | The workspace used by this Secret. If not specified, it can be cluster-wide(but it is typically configurable by administrators)                            |
| params | []object | Yes | Authentication parameter list                     |

**SSH Key Parameters**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| username | string | Yes | SSH username |
| password | string | Conditional | SSH password (either password or key pair) |
| privateKey | string | Conditional | Private key (Base64 encoded, either password or key pair) |
| publicKey | string | Conditional | Public key (Base64 encoded, used with privateKey) |

**Image Registry Parameters**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| server | string | Yes | Image registry address |
| username | string | Yes | Username |
| password | string | Yes | Password (Base64 encoded) |


**General Parameters**:

For secrets with type=general:

- params is a list of free-form key-value maps (map<string, string>)
- Keys and values are fully user-defined
- Use Base64 for sensitive values when needed (tokens, passwords)
- Example: store a GitHub Personal Access Token (PAT) with key github_token

| Example Key | Value Description                 |
|-------------|-----------------------------------|
| github_token| GitHub Personal Access Token (PAT)|



**Response Example**:

```json
{
  "secretId": "my-ssh-key"
}
```

---

### 2. List Secrets

Get secret list with type filtering support.

**Endpoint**: `GET /api/v1/secrets`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| type | string | No | Filter by type: ssh/image/general (comma-separated) |
| workspaceId | string | No | Filter by workspace ID, e.g. 'dev' |

**Response Example**:

```json
{
  "totalCount": 2,
  "items": [{
    "secretId": "test-image-abc12",
    "secretName": "test-image",
    "workspaceIds": [
      "dev", "prod"
    ],
    "type": "image",
    "creationTime": "2025-09-27T01:19:28",
    "userId": "user-zhangsan-abc123",
    "userName": "zhangsan"
  }, {
    "secretId": "test-ssh-abc12",
    "secretName": "test-ssh",
    "type": "ssh",
    "creationTime": "2025-09-25T09:41:27",
    "userId": "user-zhangsan-abc123",
    "userName": "zhangsan"
  }]
}
```

**Field Description**:

Most response fields are consistent with the "Create Secret" request above.
Additional fields:

| Field | Type | Description |
|-------|------|-------------|
| userId | string | Secret creator user ID |
| userName | string | Secret creator username |

---

### 3. Get Secret Details

Get detailed information about a specific secret.

**Endpoint**: `GET /api/v1/secrets/{SecretId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| SecretId | Secret ID |

**Response Example**:

```json
{
  "secretId": "my-ssh-key",
  "secretName": "my-ssh-key",
  "type": "image",
  "workspaceIds": [
      "dev", "prod"
  ],
  "params": [
    {
      "server": "harbor.example.com",
      "username": "admin",
      "password": "SGFyYm9yMTIzNDU=(Base64 encoded password)"
    },
    {
      "server": "docker.io",
      "username": "dockeruser",
      "password": "RG9ja2VyMTIzNDU=(Base64 encoded password)"
    }
  ],
  "creationTime": "2025-01-10T10:00:00",
  "userId": "user-zhangsan-abc123",
  "userName": "zhangsan"
}
```

**Field Description**:

The response fields are consistent with the "Create Secret" request above.


---

### 4. Update Secret

Update secret authentication information or binding settings.

**Endpoint**: `PATCH /api/v1/secrets/{SecretId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| SecretId | Secret ID |

**Request Parameters**:

```json
{
  "params": [
    {
      "server": "harbor.example.com",
      "username": "newuser",
      "password": "TmV3UGFzc3dvcmQ="
    },
    {
      "server": "docker.io",
      "username": "dockeruser",
      "password": "RG9ja2VyMTIzNDU="
    }
  ],
  "workspaceIds": ["dev", "prod"]
}
```

**Field Description**:

- All fields are optional; only provided fields will be updated
- When provided, `params` and `workspaceIds` will replace the existing configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| params | []object | No | Update secret parameters. For image secrets supports multiple registries; sensitive values should be Base64 |
| workspaceIds | []string | No | Replace the set of workspaces bound to this secret |

**Response**: 200 OK with no response body

---

### 5. Delete Secret

Delete a specific secret.

**Endpoint**: `DELETE /api/v1/secrets/{SecretId}`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| SecretId | Secret ID |

**Prerequisites**: Secret is not being used by any cluster or node

**Response**: 200 OK with no response body

---

### Deletion Behavior

- When a secret is deleted, the system will automatically remove its bindings from associated clusters and workspaces (e.g., workspace-level secret references), preventing dangling references.

---

## Secret Types

### SSH Key (type=ssh)

Used for node SSH login, supports two authentication methods:

1. **Password Authentication**: Provide username and password
2. **Key Pair Authentication**: Provide username, privateKey and publicKey

**Use Cases**:
- Specify sshSecretId during node registration
- Specify sshSecretId during cluster creation

### Image Registry Key (type=image)

Used for pulling images from private registries.

**Features**:
- Supports multiple registry configurations (multiple params)
- Each server can only have one authentication configuration

**Use Cases**:
- Bind imageSecretIds to workspace
- Specify imageSecretId during cluster creation

### General Secret (type=general)

Used for storing arbitrary key-value authentication/configuration data.

**Features**:
- params is a list of free-form maps; keys and values are user-defined
- Use Base64 for sensitive values (e.g., tokens, passwords)
- Flexible for various integrations (e.g., GitHub PAT, cloud tokens)

**Use Cases**:
- Store GitHub Personal Access Token under key `github_token`
- Provide custom credentials for third-party services or tools

## Secret Binding

### Manual Binding 

- Per-secret assignment (by user): When creating or updating a secret, specify `workspaceIds` to declare which workspaces the secret belongs to. The secret owner (or admins) can use the secret in those workspaces; other members cannot by default.
- Workspace-level binding (by admin): Administrators can bind selected secrets to a workspace for organization-wide use via the Workspace update API. When bound, all members of that workspace can reference the secret. 


## Base64 Encoding

Following fields need Base64 encoding:
- SSH keys: `privateKey`, `publicKey`
- Image registry: `password`

**Encoding Example** (Linux/Mac):
```bash
echo -n "MyPassword" | base64
```

**Decoding Example**:
```bash
echo "TXlQYXNzd29yZA==" | base64 -d
```

## Secret Update Strategy

When updating a secret, the system automatically:

1. **Updates Associated Clusters**: If cluster uses this secret, updates cluster's secret reference
2. **Updates Associated Workspaces**: If workspace is bound to this secret, updates workspace's secret reference
3. **Version Control**: System tracks secret versions through ResourceVersion

## Notes

1. **Secret Name**: Cannot be modified after creation, need to delete and recreate with new name
2. **Base64 Encoding**: Private keys, public keys and passwords must be Base64 encoded
3. **Key Pair**: When using key pair authentication, privateKey and publicKey must be matched
4. **Multiple Registries**: Image secrets can configure multiple registries, but each registry can only have one authentication
