# PublicKey API

## Overview
The PublicKey API provides management capabilities for users' SSH public keys, enabling secure passwordless authentication to workload containers. The API allows users to register, manage, and control multiple SSH public keys, which are used for establishing secure shell connections to running Pods. Public keys follow the OpenSSH standard and can be enabled or disabled individually, providing flexible access control without requiring password-based authentication.

### Core Concepts

A public key is an SSH credential used for passwordless authentication, with the following key characteristics:

* Passwordless Authentication: Enables secure SSH access to workload Pods without requiring password input.
* Multi-Key Support: Users can register and manage multiple public keys for different devices or purposes.
* Key Status Control: Individual keys can be enabled or disabled, providing granular access control.
* OpenSSH Compatibility: Supports standard OpenSSH public key formats (RSA, ECDSA, Ed25519) for broad compatibility.

## API List

### 1. Create Public Key

**Endpoint**: `POST /api/v1/publickeys`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "name": "my-laptop-key",
  "description": "Laptop public key",
  "publicKey": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC..."
}
```

**Response**: 200 OK with no response body

---

### 2. List Public Keys

**Endpoint**: `GET /api/v1/publickeys`

**Authentication Required**: Yes

**Query Parameters**:
- `offset`: Pagination offset, default 0
- `limit`: Records per page, default 10
- `sortBy`: Sort field (e.g., createTime, updateTime)
- `order`: Sort order, asc/desc

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

**Endpoint**: `DELETE /api/v1/publickeys/:id`

**Authentication Required**: Yes

**Response**: 200 OK with no response body

---

### 4. Update Public Key Status

**Endpoint**: `PATCH /api/v1/publickeys/:id/status`

**Authentication Required**: Yes

**Request**: `{ "status": false }`

**Status Values**:
- `true`: Active, can be used
- `false`: Disabled, cannot be used

**Response**: 200 OK with no response body

---

### 5. Update Public Key Description

**Endpoint**: `PATCH /api/v1/publickeys/:id/description`

**Authentication Required**: Yes

**Request**: `{ "description": "New description" }`

**Response**: 200 OK with no response body

---

## Notes

- Public keys are used for SSH passwordless login to workload Pods
- One user can add multiple public keys
- Disabled public keys cannot be used for login
- Public key format must conform to OpenSSH standard
