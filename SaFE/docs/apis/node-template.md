# NodeTemplate API

Node template defines the software environment configuration for nodes, including system addons, drivers, etc.

## API List

### 1. Create Node Template

**Endpoint**: `POST /api/custom/nodetemplates`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "name": "ubuntu-gpu-v1",
  "displayName": "Ubuntu GPU Template",
  "description": "Ubuntu 22.04 with NVIDIA drivers",
  "addons": [
    {
      "name": "nvidia-driver",
      "version": "535.129.03"
    },
    {
      "name": "cuda",
      "version": "12.2"
    }
  ]
}
```

**Response**: `{ "templateId": "ubuntu-gpu-v1" }`

---

### 2. List Node Templates

**Endpoint**: `GET /api/custom/nodetemplates`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalCount": 2,
  "items": [
    {
      "templateId": "ubuntu-gpu-v1",
      "displayName": "Ubuntu GPU Template",
      "description": "Ubuntu 22.04 with NVIDIA drivers",
      "creationTime": "2025-01-10T08:00:00.000Z"
    }
  ]
}
```

---

### 3. Delete Node Template

**Endpoint**: `DELETE /api/custom/nodetemplates/:name`

**Authentication Required**: Yes

**Prerequisites**: No nodes using this template

**Response**: 200 OK with no response body

---

## Notes

- Node templates define software to be installed during node initialization
- Cannot be modified after creation, need to delete and recreate
- Templates in use cannot be deleted
