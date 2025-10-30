# Image Registry API

Image registry management API for configuring external image registries.

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

**Response**: `{ "id": 1 }`

---

### 2. List Image Registries

**Endpoint**: `GET /api/v1/image-registries`

**Authentication Required**: Yes

**Query Parameters**:
- `pageNum`: Page number, default 1
- `pageSize`: Records per page, default 10

**Response Example**:
```json
{
  "totalCount": 3,
  "items": [
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
}
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

**Response**: `{ "id": 1 }`

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
