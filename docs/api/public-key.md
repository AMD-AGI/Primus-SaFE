# PublicKey API

Public key management API for managing users' SSH public keys for passwordless login.

## API List

### 1. Create Public Key

**Endpoint**: `POST /api/custom/publickeys`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "name": "my-laptop-key",
  "description": "Laptop public key",
  "publicKey": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC..."
}
```

**Response**: Empty response on success (HTTP 200)

---

### 2. List Public Keys

**Endpoint**: `GET /api/custom/publickeys`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalCount": 3,
  "items": [
    {
      "id": 123,
      "userId": "user-001",
      "description": "Laptop public key",
      "publicKey": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC...",
      "status": true,
      "createTime": "2025-01-10T08:00:00.000Z",
      "updateTime": "2025-01-10T08:00:00.000Z"
    }
  ]
}
```

---

### 3. Delete Public Key

**Endpoint**: `DELETE /api/custom/publickeys/:id`

**Authentication Required**: Yes

**Response**: Empty response on success (HTTP 200)

---

### 4. Update Public Key Status

**Endpoint**: `PATCH /api/custom/publickeys/:id/status`

**Authentication Required**: Yes

**Request**: `{ "status": false }`

**Status Values**:
- `true`: Active, can be used
- `false`: Disabled, cannot be used

**Response**: Empty response on success (HTTP 200)

---

### 5. Update Public Key Description

**Endpoint**: `PATCH /api/custom/publickeys/:id/description`

**Authentication Required**: Yes

**Request**: `{ "description": "New description" }`

**Response**: Empty response on success (HTTP 200)

---

## Notes

- Public keys are used for SSH passwordless login to workload Pods
- One user can add multiple public keys
- Disabled public keys cannot be used for login
- Public key format must conform to OpenSSH standard
