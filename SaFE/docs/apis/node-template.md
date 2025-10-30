# NodeTemplate API

Node template defines the software environment configuration for nodes, including system addons, drivers, etc.

## API List

### 1. Create Node Template

**Endpoint**: `POST /api/custom/nodetemplates`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "templateId": "node-template-v1",
  "addOnTemplates": [
    "disable-os-upgrade",
    "enable-cpu-performance",
    "sysctl-inotify"
  ]
}
```

**Response**: `{ "id": "node-template-v1" }`

---

### 2. List Node Templates

**Endpoint**: `GET /api/v1/nodetemplates`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalCount": 1,
  "items": [
    {
      "templateId": "node-template-v1",
      "addOnTemplates": [
        "disable-os-upgrade",
        "enable-cpu-performance",
        "sysctl-inotify"
      ]
    }
  ]
}
```

---

### 3. Delete Node Template

**Endpoint**: `DELETE /api/v1/nodetemplates/{TemplateId}`

**Authentication Required**: Yes


**Response**: 200 OK with no response body

---

## Notes

- Node templates define software to be installed when a node is managed into the cluster
- Addons can also be manually installed by specifying nodes using opsjob.
- The addon is auto-registered with Safe and currently doesn't support dynamic creation.
