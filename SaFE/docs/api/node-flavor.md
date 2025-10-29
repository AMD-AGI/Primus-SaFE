# NodeFlavor API

Node flavor defines the hardware resource configuration of nodes.

## API List

### 1. Create Node Flavor

**Endpoint**: `POST /api/custom/nodeflavors`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "name": "gpu-large",
  "displayName": "GPU Large",
  "description": "8x A100 GPU node",
  "cpu": "128",
  "gpu": "8",
  "memory": "512Gi",
  "ephemeralStorage": "1Ti",
  "rdma": "8"
}
```

**Response**: `{ "flavorId": "gpu-large" }`

---

### 2. List Node Flavors

**Endpoint**: `GET /api/custom/nodeflavors`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalCount": 3,
  "items": [
    {
      "flavorId": "gpu-large",
      "displayName": "GPU Large",
      "description": "8x A100 GPU node",
      "cpu": "128",
      "gpu": "8",
      "memory": "512Gi",
      "creationTime": "2025-01-10T08:00:00.000Z"
    }
  ]
}
```

---

### 3. Get Node Flavor Details

**Endpoint**: `GET /api/custom/nodeflavors/:name`

**Authentication Required**: Yes

---

### 4. Update Node Flavor

**Endpoint**: `PATCH /api/custom/nodeflavors/:name`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "description": "Updated description",
  "cpu": "256"
}
```

---

### 5. Delete Node Flavor

**Endpoint**: `DELETE /api/custom/nodeflavors/:name`

**Authentication Required**: Yes

**Prerequisites**: No nodes using this flavor

---

### 6. Get Node Flavor Availability

Check how many nodes can use this flavor.

**Endpoint**: `GET /api/custom/nodeflavors/:name/avail`

**Authentication Required**: Yes

**Response**: `{ "availableCount": 10 }`
