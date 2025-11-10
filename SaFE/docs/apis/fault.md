# Fault API

## Overview

Fault is the core object for recording system node failures, featuring a unique identifier (ID) and associated handling actions (e.g., tainting). It is automatically generated either by node-agent reports or Kubernetes node status changes, ensuring timely tracking and response to node issues.

## API List

### 1. List Faults

**Endpoint**: `GET /api/v1/faults`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| offset | int | No | Pagination offset, default 0 |
| limit | int | No | Records per page, default 100 |
| sortBy | string | No | Sort field, default creationTime |
| order | string | No | Sort order: desc/asc, default desc |
| nodeId | string | No | Filter by node ID (fuzzy match) |
| monitorId | string | No | Filter by monitor ID; multiple IDs comma-separated |
| clusterId | string | No | Filter by cluster ID |
| onlyOpen | bool | No | Only return currently open faults |

**Response Example**:
```json
{
  "totalCount": 2,
  "items": [
    {
      "id": "3c5b58e5-36e0-4b24-8ffa-6fe1b3a1c2f7",
      "nodeId": "node-001",
      "monitorId": "net-monitor-123",
      "message": "network unreachable",
      "action": "taint",
      "phase": "Failed",
      "clusterId": "prod-cluster",
      "creationTime": "2025-01-15T10:00:00",
      "deletionTime": ""
    }
  ]
}
```

**Field Description**:

| Field | Type | Description                                     |
|-------|------|-------------------------------------------------|
| totalCount | int | Total number of faults, not limited by pagination |
| id | string | Unique fault ID                                 |
| nodeId | string | Related node ID                                 |
| monitorId | string | Monitor ID (from node-agent)                    |
| message | string | Fault message                                   |
| action | string | Action taken, e.g. taint                        |
| phase | string | Fault status, e.g. Succeeded/Failed             |
| clusterId | string | Cluster ID                                      |
| creationTime | string | Creation time (RFC3339Short)                    |
| deletionTime | string | Deletion time (RFC3339Short), empty if not deleted   |

---

### 2. Stop Fault

**Endpoint**: `POST /api/v1/faults/{FaultId}/stop`

**Authentication Required**: Yes

**Description**: Stop fault(removes it from the k8s cluster)

**Response**: 200 OK with no response body

---

### 3. Delete Fault

**Endpoint**: `DELETE /api/v1/faults/{FaultId}`

**Authentication Required**: Yes

**Description**: Stop fault and delete the record from database

**Response**: 200 OK with no response body

---

## Notes

- Fault can affect workloads on the node (if configured).
- Deleting a fault will remove the taint associated with it.
