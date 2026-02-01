# Persistent Volume API

## Overview

The Persistent Volume API provides access to Kubernetes PersistentVolume (PV) resources within a specified workspace

### Core Concepts

Persistent Volumes are cluster-level storage resources that provide storage capacity to workloads through PersistentVolumeClaims (PVC). The API exposes the following information:

* **Capacity**: Storage size allocated to the PV
* **Access Modes**: How the volume can be mounted (ReadWriteOnce, ReadOnlyMany, ReadWriteMany)
* **Claim Reference**: Which PVC is bound to this PV
* **Storage Class**: The storage class used for provisioning
* **Reclaim Policy**: What happens to the PV when released (Retain, Recycle, Delete)
* **Phase**: Current status of the PV (Available, Bound, Released, Failed)

### Access Control

> ⚠️ **Workspace Permission Required**: Users must have list permission on the specified workspace to query its associated PersistentVolumes.

## API List

### List Persistent Volumes

Query PersistentVolumes associated with a specific workspace.

**Endpoint**: `GET /api/v1/pvs`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| workspaceId | string | Yes | - | Filter by workspace ID (max 64 characters) |

**Request Examples**:

```bash
# List all PVs for a specific workspace
GET /api/v1/pvs?workspaceId=my-workspace

```

**Response Example**:

```json
{
  "totalCount": 2,
  "items": [
    {
      "labels": {
        "primus-safe.pfs-selector": "weka-fs"
      },
      "capacity": {
        "storage": "100Gi"
      },
      "accessModes": ["ReadWriteMany"],
      "claimRef": {
        "kind": "PersistentVolumeClaim",
        "namespace": "my-workspace",
        "name": "data-volume",
        "uid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
      },
      "volumeMode": "Filesystem",
      "storageClassName": "weka-storageclass",
      "persistentVolumeReclaimPolicy": "Retain",
      "phase": "Bound",
      "message": ""
    },
    {
      "labels": {
        "primus-safe.pfs-selector": "nfs-storage"
      },
      "capacity": {
        "storage": "50Gi"
      },
      "accessModes": ["ReadWriteOnce"],
      "volumeMode": "Filesystem",
      "storageClassName": "nfs-storageclass",
      "persistentVolumeReclaimPolicy": "Delete",
      "phase": "Available",
      "message": ""
    }
  ]
}
```

**Response Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of PVs matching the query |
| items | array | List of PersistentVolume entries |
| items[].labels | object | Labels attached to the PV (filtered to show relevant labels) |
| items[].capacity | object | Storage capacity of the PV (e.g., `{"storage": "100Gi"}`) |
| items[].accessModes | array | List of access modes supported by the PV |
| items[].claimRef | object | Reference to the bound PVC (null if unbound) |
| items[].claimRef.kind | string | Always "PersistentVolumeClaim" |
| items[].claimRef.namespace | string | Namespace of the bound PVC |
| items[].claimRef.name | string | Name of the bound PVC |
| items[].claimRef.uid | string | UID of the bound PVC |
| items[].volumeMode | string | Volume mode: "Filesystem" or "Block" |
| items[].storageClassName | string | Name of the StorageClass used |
| items[].persistentVolumeReclaimPolicy | string | Reclaim policy (Retain/Recycle/Delete) |
| items[].phase | string | Current phase of the PV |
| items[].message | string | Additional status message (if any) |

---

## Field Values Reference

### accessModes - Access Modes

| Value | Description |
|-------|-------------|
| `ReadWriteOnce` | Can be mounted as read-write by a single node |
| `ReadOnlyMany` | Can be mounted as read-only by multiple nodes |
| `ReadWriteMany` | Can be mounted as read-write by multiple nodes |
| `ReadWriteOncePod` | Can be mounted as read-write by a single pod |

---

### phase - PV Phases

| Value | Description |
|-------|-------------|
| `Available` | PV is available and not yet bound to a PVC |
| `Bound` | PV is bound to a PVC |
| `Released` | PVC was deleted but PV is not yet reclaimed |
| `Failed` | PV failed automatic reclamation |

---

### persistentVolumeReclaimPolicy - Reclaim Policies

| Value | Description |
|-------|-------------|
| `Retain` | PV is kept after PVC is deleted (manual reclamation required) |
| `Recycle` | Basic scrub (`rm -rf /volume/*`) before making available again (deprecated) |
| `Delete` | PV is deleted when PVC is deleted |

---

### volumeMode - Volume Modes

| Value | Description |
|-------|-------------|
| `Filesystem` | Volume is mounted as a filesystem (default) |
| `Block` | Volume is presented as a raw block device |

---

## Error Responses

| HTTP Status | Error Code | Description |
|-------------|------------|-------------|
| 400 | Bad Request | Invalid query parameters or missing workspaceId |
| 401 | Unauthorized | Not authenticated |
| 403 | Forbidden | User does not have permission to access the workspace |
| 404 | Not Found | Workspace not found |
| 500 | Internal Server Error | Failed to retrieve PVs from cluster |

**Error Response Example**:

```json
{
  "errorCode": "Primus.00003",
  "errorMessage": "forbidden: user does not have permission to access workspace resources"
}
```

---

## Notes

1. **Workspace Association**: PVs are associated with workspaces through labels (`primus-safe.workspace.id`). Only PVs with matching workspace labels are returned.

2. **Cluster Scope**: PVs are queried from the data plane cluster associated with the workspace, not the control plane.
