# Image Registry API

## Overview
The Image Registry API provides management capabilities for configuring and maintaining connections to external container image registries. It serves as a bridge between the system and various container registries, enabling seamless image import operations from multiple sources including Docker Hub, Harbor, and other Docker Registry V2 compliant registries. The API supports credential management, default registry configuration, and provides secure encrypted storage for sensitive authentication information.

### Core Concepts

An image registry configuration defines the connection parameters and credentials for accessing external container registries, with the following key characteristics:

* Registry Connection: Establishes and maintains secure connections to external image registries with authentication support.
* Credential Management: Securely stores registry credentials with encryption to protect sensitive information.
* Default Registry: Supports designation of a default registry for streamlined image import operations.
* Multi-Registry Support: Enables configuration of multiple registries to access images from various sources.

## API List

### 1. Create Image Registry Configuration

**Endpoint**: `POST /api/v1/image-registries`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "name": "docker-hub",
  "url": "https://registry-1.docker.io",
  "username": "myuser",
  "password": "mypassword",
  "default": false
}
```

**Field Description**:
- `name`: Registry name (unique)
- `url`: Registry URL
- `username`: Username
- `password`: Password
- `default`: Whether to set as default registry

**Response Example**:
```json
{
  "id": 1,
  "name": "docker-hub",
  "url": "https://registry-1.docker.io",
  "username": "myuser",
  "default": false,
  "created_at": 1705305600,
  "updated_at": 1705305600
}
```

---

### 2. List Image Registries

**Endpoint**: `GET /api/v1/image-registries`

**Authentication Required**: Yes

**Query Parameters**:
- `pageNum`: Page number, default 1
- `pageSize`: Records per page, default 10

**Response Example**:
```json
[
  {
    "id": 1,
    "name": "docker-hub",
    "url": "https://registry-1.docker.io",
    "username": "myuser",
    "default": false,
    "created_at": 1705305600,
    "updated_at": 1705305600
  }
]
```

---

### 3. Update Image Registry Configuration

**Endpoint**: `PUT /api/v1/image-registries/:id`

**Authentication Required**: Yes

**Path Parameters**:
- `id`: Registry ID

**Request Example**:
```json
{
  "name": "docker-hub",
  "url": "https://registry-1.docker.io",
  "username": "newuser",
  "password": "newpassword",
  "default": true
}
```

**Response Example**:
```json
{
  "id": 1,
  "name": "docker-hub",
  "url": "https://registry-1.docker.io",
  "username": "newuser",
  "default": true,
  "created_at": 1705305600,
  "updated_at": 1705392000
}
```

---

### 4. Delete Image Registry Configuration

**Endpoint**: `DELETE /api/v1/image-registries/:id`

**Authentication Required**: Yes

**Path Parameters**:
- `id`: Registry ID

**Response**: 200 OK with no response body

---

## Supported Image Registries

- Docker Hub
- Harbor
- Other registries compliant with Docker Registry V2 standard

## Notes

- Default registry is used as the default source for image import
- Deleting registry does not affect already imported images
- Passwords are stored encrypted
- Same URL can only be configured once
