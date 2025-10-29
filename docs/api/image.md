# Image API

Image management API for managing and importing container images.

## API List

### 1. List Images

**Endpoint**: `GET /api/v1/images`

**Authentication Required**: Yes

**Query Parameters**:
- `page_num`: Page number, default 0
- `page_size`: Records per page, default 10
- `orderBy`: Sort field
- `order`: Sort order, asc/desc
- `tag`: Filter by tag
- `ready`: Filter by ready status
- `userName`: Filter by username
- `flat`: Flat output (true/false)

**Response Example (flat=true)**:
```json
{
  "totalCount": 50,
  "images": [
    {
      "id": 123,
      "tag": "pytorch:2.0-cuda11.8",
      "description": "PyTorch 2.0 with CUDA 11.8",
      "created_at": 1705305600,
      "created_by": "admin"
    }
  ]
}
```

**Response Example (flat=false)**:
```json
{
  "totalCount": 50,
  "images": [
    {
      "registryHost": "harbor.example.com",
      "repo": "ai/pytorch",
      "artifacts": [
        {
          "imageTag": "2.0-cuda11.8",
          "description": "PyTorch 2.0 with CUDA 11.8",
          "createdTime": "2025-01-15T10:00:00.000Z",
          "userName": "admin",
          "status": "ready",
          "id": 123,
          "size": 5368709120,
          "arch": "amd64",
          "os": "linux",
          "digest": "sha256:abc123...",
          "includeType": "full"
        }
      ]
    }
  ]
}
```

---

### 2. Delete Image

**Endpoint**: `DELETE /api/v1/images/:id`

**Authentication Required**: Yes

**Path Parameters**:
- `id`: Image ID

**Response**: Empty response on success (HTTP 200)

---

### 3. Import Image

Import image from external registry to internal Harbor.

**Endpoint**: `POST /api/v1/images:import`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "source": "docker.io/pytorch/pytorch:2.0.0-cuda11.7-cudnn8-runtime",
  "sourceRegistry": "docker.io"
}
```

**Field Description**:
- `source`: Source image full address
- `sourceRegistry`: Source image registry address (optional)

**Response Example**:
```json
{
  "state": 1,
  "message": "Image import started",
  "alreadyImageId": 0
}
```

**State Codes**:
- `0`: Import failed
- `1`: Import started
- `2`: Image already exists

---

### 4. Update Image Import Progress

**Endpoint**: `PUT /api/v1/images:import/:name/progress`

**Authentication Required**: Yes

**Description**: Used by image import service to update import progress

---

### 5. Get Image Import Details

**Endpoint**: `GET /api/v1/images/:id/importing-details`

**Authentication Required**: Yes

**Path Parameters**:
- `id`: Image ID

**Response Example**:
```json
{
  "layersDetail": {
    "layer1": {
      "status": "completed",
      "progress": 100
    },
    "layer2": {
      "status": "downloading",
      "progress": 45
    }
  }
}
```

---

### 6. Get Harbor Statistics

**Endpoint**: `GET /api/v1/harbor/stats`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalProjects": 10,
  "totalImages": 500,
  "totalSize": 1099511627776
}
```

---

## Image Status

| Status | Description |
|--------|-------------|
| ready | Ready and available |
| importing | Being imported |
| failed | Import failed |

## Notes

- Image import is asynchronous operation
- Importing large images may take significant time
- Supports multiple source registries (Docker Hub, private registries, etc.)
- Automatically handles multi-architecture images during import
