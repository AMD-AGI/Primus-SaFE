# Fault API

Fault injection API for simulating node failures for system fault tolerance testing.

## API List

### 1. List Faults

**Endpoint**: `GET /api/custom/faults`

**Authentication Required**: Yes

**Query Parameters**:
- `workspaceId`: Workspace ID
- `clusterId`: Cluster ID

**Response Example**:
```json
{
  "totalCount": 2,
  "items": [
    {
      "faultId": "node-001-network-fault",
      "nodeId": "node-001",
      "type": "NetworkFault",
      "phase": "Running",
      "creationTime": "2025-01-15T10:00:00.000Z",
      "description": "Network fault simulation"
    }
  ]
}
```

---

### 2. Stop Fault

**Endpoint**: `POST /api/custom/faults/:name/stop`

**Authentication Required**: Yes

**Description**: Stop fault injection and restore node to normal state

**Response**: Empty response on success (HTTP 200)

---

### 3. Delete Fault

**Endpoint**: `DELETE /api/custom/faults/:name`

**Authentication Required**: Yes

**Description**: Delete fault record

**Response**: Empty response on success (HTTP 200)

---

## Fault Types

| Type | Description |
|------|-------------|
| NetworkFault | Network failure |
| DiskFault | Disk failure |
| CPUFault | CPU failure |
| MemoryFault | Memory failure |

## Notes

- Fault injection will affect workloads on the node
- Recommended for use in test environments only
- Faults are reflected as taints on nodes
