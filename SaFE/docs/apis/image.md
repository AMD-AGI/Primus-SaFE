# Image API

## Overview
The Image API provides comprehensive management capabilities for container images within the system. It enables users to import images from external registries into the internal Harbor registry, manage image lifecycle, and track import progress. The API supports both flat and hierarchical image listing formats, handles multi-architecture images automatically, and provides detailed progress tracking for asynchronous import operations. Images serve as the foundation for running workloads, providing the necessary runtime environments and dependencies.

### Core Concepts

An image represents a container image artifact stored in the internal Harbor registry, with the following key characteristics:

* Image Import: Asynchronous process of pulling images from external registries and storing them in the internal Harbor registry.
* Multi-Architecture Support: Automatically handles images built for different CPU architectures (amd64, arm64, etc.).
* Status Tracking: Provides real-time status updates during import operations, including layer-by-layer progress information.
* Image Lifecycle: Manages the complete lifecycle from import to deletion, with detailed metadata and statistics.

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
[
  {
    "id": 123,
    "tag": "pytorch:2.0-cuda11.8",
    "description": "PyTorch 2.0 with CUDA 11.8",
    "created_at": 1705305600,
    "created_by": "admin"
  }
]
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
          "createdTime": "2025-01-15T10:00:00",
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

**Response**: 200 OK with no response body

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

**Description**: Internal API called by tools/sync-image to report image upload progress during import operations

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
  "status": "healthy",
  "components": [
    {
      "name": "core",
      "status": "healthy"
    },
    {
      "name": "database",
      "status": "healthy"
    },
    {
      "name": "redis",
      "status": "healthy"
    }
  ],
  "private_project_count": 5,
  "public_project_count": 5,
  "private_repo_count": 250,
  "public_repo_count": 250,
  "total_storage": 1099511627776
}
```

**Field Description**:
- `status`: Overall Harbor health status
- `components`: Health status of Harbor components
- `private_project_count`: Number of private projects
- `public_project_count`: Number of public projects
- `private_repo_count`: Number of private repositories
- `public_repo_count`: Number of public repositories
- `total_storage`: Total storage used in bytes

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
