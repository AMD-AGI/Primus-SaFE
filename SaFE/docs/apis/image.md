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
    "created_by": "admin",
    "secretId": ""
  },
  {
    "id": 124,
    "tag": "my-private-image:v1",
    "description": "Private image requiring authentication",
    "created_at": 1705305700,
    "created_by": "admin",
    "secretId": "my-image-secret"
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
          "includeType": "full",
          "secretId": ""
        },
        {
          "imageTag": "private-v1",
          "description": "Private image",
          "createdTime": "2025-01-15T11:00:00",
          "userName": "admin",
          "status": "ready",
          "id": 124,
          "size": 2147483648,
          "arch": "amd64",
          "os": "linux",
          "digest": "sha256:def456...",
          "includeType": "full",
          "secretId": "my-image-secret"
        }
      ]
    }
  ]
}
```

**New Field for Private Image Support**:
- `secretId`: The secret ID associated with the image for source registry authentication (empty if not provided)

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

**Request Example (Private Image with Secret)**:
```json
{
  "source": "my-private-registry.com/myrepo/myimage:v1",
  "secretId": "my-image-secret-id"
}
```

**Field Description**:
- `source`: Source image full address
- `sourceRegistry`: Source image registry address (optional)
- `secretId`: Optional secret ID for private image authentication. The secret must be of type "image" (created via `/api/v1/secrets` with `type=image`). If provided, the image will be marked as private and require this secret to pull.

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

**Notes on Private Images**:
- When `secretId` is provided, the secret is used to authenticate against the source registry during import
- The secret must exist in the system and be of type "image"
- When listing images, check if `secretId` is empty to determine if it was imported from a private source

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

### 6. List Exported Images

**Endpoint**: `GET /api/v1/images/custom`

**Authentication Required**: Yes

**Description**: Lists images that have been exported from workloads using OpsJob (type: exportimage). This endpoint queries the ops_job table to retrieve export history and present it in the same format as the standard image list.

**Query Parameters**: Same as "List Images" endpoint, except:
- Does NOT support `tag` parameter (no tag field in ops_job table)
- Does NOT support `flat` parameter (always returns list format)
- `ready=true` filters jobs with phase='Succeeded'
- **NEW**: `workload=xxx` filters by workload ID

**Response Example**:
```json
{
  "totalCount": 2,
  "items": [
    {
      "imageName": "harbor.tas.primus-safe.amd.com/custom/library/busybox:20251113162030",
      "workload": "my-busybox-workload",
      "status": "Succeeded",
      "createdTime": "2025-11-13T12:14:27Z",
      "label": "Production backup",
      "log": "Image exported successfully"
    },
    {
      "imageName": "harbor.tas.primus-safe.amd.com/custom/rocm/pytorch:20251113143015",
      "workload": "pytorch-training-001",
      "status": "Failed",
      "createdTime": "2025-11-13T11:30:15Z",
      "label": "Test export",
      "log": "failed to push image: 401 Unauthorized"
    }
  ]
}
```

**Field Description** (6 fields total):
1. `imageName`: **Full** target image path in Harbor including registry (from ops_job.outputs.target), e.g. `harbor.tas.primus-safe.amd.com/custom/library/busybox:20251113162030`
2. `workload`: Source workload ID (from ops_job.inputs.workload)
3. `status`: Export job status (from ops_job.phase): `Pending`/`Running`/`Succeeded`/`Failed`
4. `createdTime`: Export job creation time (from ops_job.creation_time, RFC3339 format)
5. `label`: User-defined label (from ops_job.inputs.label), empty if not provided during job creation
6. `log`: Status message or error details (from ops_job.conditions[].message)

**Note**: The `imageName` field contains the complete image path including registry URL, so you can directly use it with `docker pull` or `nerdctl pull` commands.

**Notes**:
- This endpoint queries the `ops_job` table (type='exportimage'), not the `image` table
- Returns a simplified list format with **6 fields**
- Use `ready=true` to filter only successfully exported images (phase='Succeeded')
- Use `workload=xxx` to filter exports from a specific workload
- To add a label to an export job, include `{ "name": "label", "value": "your label text" }` in the inputs when creating the OpsJob

---

### 7. List Prewarm Images

**Endpoint**: `GET /api/v1/images/prewarm`

**Authentication Required**: Yes

**Description**: Lists image prewarm jobs that have been created using OpsJob (type: prewarm). This endpoint queries the ops_job table to retrieve prewarm history and shows the status of image pre-pulling operations across cluster nodes.

**Query Parameters**: Same as "List Images" endpoint, except:
- Does NOT support `tag` parameter (no tag field in ops_job table)
- Does NOT support `flat` parameter (always returns list format)
- `ready=true` filters jobs with phase='Succeeded'
- **NEW**: `image=xxx` filters by image name
- **NEW**: `workspace=xxx` filters by workspace ID
- **NEW**: `status=xxx` filters by prewarm status (from outputs.status field)

**Response Example**:
```json
{
  "totalCount": 2,
  "items": [
    {
      "imageName": "harbor.example.com/ai/pytorch:2.0-rocm5.7",
      "workspaceId": "workspace-001",
      "workspaceName": "AI Training Workspace",
      "status": "Completed",
      "prewarmProgress": "100%",
      "createdTime": "2024-11-20T10:30:00Z",
      "endTime": "2024-11-20T10:35:00Z",
      "userName": "admin",
      "errorMessage": ""
    },
    {
      "imageName": "docker.io/rocm/pytorch:rocm6.0_ubuntu22.04_py3.10",
      "workspaceId": "workspace-002",
      "workspaceName": "Test Workspace",
      "status": "Failed",
      "prewarmProgress": "50%",
      "createdTime": "2024-11-20T11:00:00Z",
      "endTime": "2024-11-20T11:05:00Z",
      "userName": "user123",
      "errorMessage": "Failed to pull image: connection timeout"
    }
  ]
}
```

**Field Description** (9 fields total):
1. `imageName`: Full image path to prewarm (from ops_job.inputs.image)
2. `workspaceId`: Target workspace ID (from ops_job.inputs.workspace)
3. `workspaceName`: Target workspace display name (fetched from Workspace CR)
4. `status`: Prewarm job status (from ops_job.outputs.status): `Completed`/`Failed`/`Running`
5. `prewarmProgress`: Progress percentage (from ops_job.outputs.prewarm_progress), e.g. `85%`
6. `createdTime`: Prewarm job creation time (from ops_job.creation_time, RFC3339 format)
7. `endTime`: Prewarm job completion time (from ops_job.end_time, RFC3339 format)
8. `userName`: User who created the prewarm job (from ops_job.user_name)
9. `errorMessage`: Error details if failed (from ops_job.conditions[].message)

**Notes**:
- This endpoint queries the `ops_job` table (type='prewarm'), not the `image` table
- Returns a list format with **9 fields** including progress tracking
- The prewarm job creates a DaemonSet to pull images to all nodes in the specified workspace
- Use `ready=true` to filter only successfully completed prewarm jobs (phase='Succeeded')
- Use `status=Completed` to filter by the actual prewarm operation status (from outputs)
- Use `workspace=xxx` to view prewarm operations for a specific workspace
- Progress is reported in real-time: `0%` → `50%` → `100%`

---

### 8. Get Harbor Statistics

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
