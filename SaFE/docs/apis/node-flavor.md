# NodeFlavor API

## Overview

NodeFlavor defines a node's hardware configuration, such as CPU, GPU, memory, storage, and network resources. It represents a description of the expected configuration for a node

## API List

### 1. Create Node Flavor

**Endpoint**: `POST /api/v1/nodeflavors`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "name": "gpu-large",
  "cpu": {
    "product": " AMD_EPYC_9575F",
    "quantity": "256"
  },
  "gpu": {
    "product": " AMD_Instinct_MI325X",
    "resourceName": "amd.com/gpu",
    "quantity": "8"
  },
  "memory": "1024Gi",
  "rootDisk": {
    "type": "ssd",
    "quantity": "838Gi",
    "count": 1
  },
  "dataDisk": {
    "type": "nvme",
    "quantity": "3573Gi",
    "count": 8
  },
  "extendedResources": {
    "ephemeral-storage": "838Gi",
    "rdma/hca": "1k"
  }
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Used to generate flavor ID; normalized (e.g., lowercase) |
| cpu.product | string | No | CPU product name, e.g. AMD EPYC 9554 |
| cpu.quantity | string | Yes | CPU cores (resource.Quantity), e.g. "256" |
| memory | string | Yes | Memory size (resource.Quantity), e.g. "1024Gi" |
| gpu.product | string | No | GPU product name, e.g. AMD MI300X |
| gpu.resourceName | string | Conditionally | K8s resource name when gpu is set, e.g. "amd.com/gpu" |
| gpu.quantity | string | Conditionally | GPU count (resource.Quantity) when gpu is set, e.g. "8" |
| rootDisk.type | string | No | Root disk type, e.g. "ssd", "sata", "nvme" |
| rootDisk.quantity | string | Conditionally | Root disk size (resource.Quantity) when rootDisk is set |
| rootDisk.count | int | Conditionally | Number of root disks when rootDisk is set |
| dataDisk.type | string | No | Data disk type, e.g. "nvme" |
| dataDisk.quantity | string | Conditionally | Data disk size (resource.Quantity) when dataDisk is set |
| dataDisk.count | int | Conditionally | Number of data disks when dataDisk is set |
| extendedResources | object | No | Extra resources map: key:string → value:resource.Quantity (e.g. "ephemeral-storage": "838Gi", "rdma/hca": "1k") |

Note:
- If `rootDisk` is provided and `extendedResources["ephemeral-storage"]` is absent, the server auto-fills `ephemeral-storage = rootDisk.quantity * rootDisk.count`.

**Response**: `{ "flavorId": "gpu-large" }`

---

### 2. List Node Flavors

**Endpoint**: `GET /api/v1/nodeflavors`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "totalCount": 3,
  "items": [
    {
      "flavorId": "gpu-large",
      "cpu": {
        "product": "AMD_EPYC_9575F",
        "quantity": "256"
      },
      "gpu": {
        "product": "AMD_Instinct_MI325X",
        "resourceName": "amd.com/gpu",
        "quantity": "8"
      },
      "memory": "1024Gi",
      "rootDisk": {
        "type": "ssd",
        "quantity": "838Gi",
        "count": 1
      },
      "dataDisk": {
        "type": "nvme",
        "quantity": "3573Gi",
        "count": 8
      },
      "extendedResources": {
        "ephemeral-storage": "838Gi",
        "rdma/hca": "1k"
      }
    }
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of node flavors |
| flavorId | string | Node flavor ID |

---

Only fields not already covered by "Create Node Flavor" are listed below. Other fields share the same meaning as in the creation request.


### 3. Get Node Flavor Details

**Endpoint**: `GET /api/v1/nodeflavors/{FlavorId}`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "flavorId": "gpu-large",
  "cpu": {
    "product": "AMD_EPYC_9575F",
    "quantity": "256"
  },
  "gpu": {
    "product": "AMD_Instinct_MI325X",
    "resourceName": "amd.com/gpu",
    "quantity": "8"
  },
  "memory": "1024Gi",
  "rootDisk": {
    "type": "ssd",
    "quantity": "838Gi",
    "count": 1
  },
  "dataDisk": {
    "type": "nvme",
    "quantity": "3573Gi",
    "count": 8
  },
  "extendedResources": {
    "ephemeral-storage": "838Gi",
    "rdma/hca": "1k"
  }
}
```
**Field Description**:

The fields are consistent with the List response above.

---

### 4. Update Node Flavor

**Endpoint**: `PATCH /api/v1/nodeflavors/{FlavorId}`

**Authentication Required**: Yes

**Request Example**:
```json
{
  "cpu": {
    "product": "AMD_EPYC_9575F",
    "quantity": "256"
  },
  "memory": "1024Gi",
  "gpu": {
    "product": "AMD_Instinct_MI325X",
    "resourceName": "amd.com/gpu",
    "quantity": "8"
  },
  "rootDisk": {
    "type": "ssd",
    "quantity": "838Gi",
    "count": 1
  },
  "dataDisk": {
    "type": "nvme",
    "quantity": "3573Gi",
    "count": 8
  },
  "extendedResources": {
    "ephemeral-storage": "838Gi",
    "rdma/hca": "1k"
  }
}
```

**Field Description**:

All fields are optional, only provided fields will be updated

**Response**: 200 OK with no response body

---

### 5. Delete Node Flavor

**Endpoint**: `DELETE /api/v1/nodeflavors/{FlavorId}`

**Authentication Required**: Yes

**Response**: 200 OK with no response body

---

### 6. Get Node Flavor Availability

Returns the actually available portion of resource, with a note that resources may be reserved for the system.

**Endpoint**: `GET /api/v1/nodeflavors/{FlavorId}/avail`

**Authentication Required**: Yes

**Response Example**:
```json
{
    "amd.com/gpu": 8,
    "cpu": 230,
    "ephemeral-storage": 4113760290816,
    "memory": 1540947014451,
    "rdma/hca": 1000
}
```