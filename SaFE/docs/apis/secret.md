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

**Field Description**:

| Field | Type | Required | Description                                       |
|-------|------|----------|---------------------------------------------------|
| name | string | Yes | Secret name                                       |
| type | string | Yes | Secret type: ssh/image                            |
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
| type | string | No | Filter by type: ssh/image (comma-separated) |

**Response Example**:

```json
{
  "totalCount": 2,
  "items": [{
    "secretId": "test-image-abc12",
    "secretName": "test-image",
    "type": "image",
    "params": [{
      "password": "SGFyYm9yMTIzNDU=",
      "server": "https://registry-1.docker.io",
      "username": "admin"
    }],
    "creationTime": "2025-09-27T01:19:28"
  }, {
    "secretId": "test-ssh-abc12",
    "secretName": "test-ssh",
    "type": "ssh",
    "params": [{
      "privateKey": "LS0tLS1CRUdJTi...",
      "publicKey": "c3NoLXJzYSBBQUFB...",
      "username": "admin"
    }],
    "creationTime": "2025-09-25T09:41:27"
  }]
}
```

**Field Description**:

The response fields are consistent with the "Create Secret" request above.

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
  "type": "ssh",
  "params": [
    {
      "username": "root",
      "privateKey": "LS0tLS1CRUdJTi...",
      "publicKey": "c3NoLXJzYSBBQUFB..."
    }
  ],
  "creationTime": "2025-01-10T10:00:00"
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
    }
  ]
}
```

**Field Description**: All fields are optional, only provided fields will be updated

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

## Secret Binding

### Manual Binding 

- Explicitly specify imageSecretIds when creating or updating workspace

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
