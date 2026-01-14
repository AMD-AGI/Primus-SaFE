# Node Retry API Design Document

## 1. Overview

Provides retry functionality for failed node manage/unmanage operations, allowing users to retry when an operation fails without restarting the entire process.

### 1.1 Applicable Scenarios

| Failed State | Description | State After Retry |
|--------------|-------------|-------------------|
| `ManagedFailed` | Manage operation failed | `Managing` |
| `UnmanagedFailed` | Unmanage operation failed | `Unmanaging` |

### 1.2 Core Capabilities

- ✅ Support retry for failed manage operations
- ✅ Support retry for failed unmanage operations
- ✅ Automatic cleanup of failed Pods
- ✅ Pre-condition checks (machine ready, control plane check, etc.)
- ✅ Access control (same as manage operation)

---

## 2. API Interface

### 2.1 Retry Node Operation

**Request**

```
POST /api/v1/nodes/{nodeName}/retry
```

**Request Headers**

```
Authorization: Bearer <token>
Content-Type: application/json
```

**Path Parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| nodeName | string | Yes | Node name |

**Request Body**

No request body required.

**Success Response (200 OK)**

```json
{
  "message": "retry initiated successfully",
  "nodeId": "node-1",
  "previousPhase": "ManagedFailed",
  "currentPhase": "Managing"
}
```

**Response Fields**

| Field | Type | Description |
|-------|------|-------------|
| message | string | Operation result message |
| nodeId | string | Node ID |
| previousPhase | string | State before retry |
| currentPhase | string | State after retry |

---

## 3. Error Responses

### 3.1 Error Code List

| HTTP Status | Error Code | Error Message | Description |
|-------------|------------|---------------|-------------|
| 400 | Primus.00002 | node is not in a failed state | Node is not in failed state |
| 400 | Primus.00002 | machine is not ready, please wait and try again | Machine not ready (manage retry) |
| 400 | Primus.00002 | control plane node cannot be unmanaged | Control plane node cannot be unmanaged |
| 400 | Primus.00002 | cannot determine cluster ID for retry operation | Cannot determine cluster ID |
| 403 | Primus.00004 | Forbidden | User has no permission for this operation |
| 404 | Primus.00005 | node not found | Node does not exist |
| 500 | Primus.00001 | failed to delete failed pods | Error deleting failed pods |
| 500 | Primus.00001 | failed to reset node status | Error updating node status |

### 3.2 Error Response Format

```json
{
  "errorCode": "Primus.00002",
  "errorMessage": "node is not in a failed state, current phase: Managed. Only ManagedFailed or UnmanagedFailed nodes can be retried"
}
```

---

## 4. Pre-condition Checks

### 4.1 Manage Retry (ManagedFailed → Managing)

| Check Item | Condition | Error Message |
|------------|-----------|---------------|
| Machine ready | `node.IsMachineReady() == true` | machine is not ready, please wait and try again |

### 4.2 Unmanage Retry (UnmanagedFailed → Unmanaging)

| Check Item | Condition | Error Message |
|------------|-----------|---------------|
| Not control plane | `IsControlPlane(node) == false` | control plane node cannot be unmanaged |

---

## 5. State Transition Diagrams

### 5.1 Manage Flow

```
                    ┌─────────────────┐
                    │    Unmanaged    │
                    └────────┬────────┘
                             │ User initiates manage
                             ↓
                    ┌─────────────────┐
              ┌────▶│    Managing     │◀────┐
              │     └────────┬────────┘     │
              │              │              │
              │       Success│ Failure      │
              │              ↓              │
              │     ┌─────────────────┐     │
              │     │    Managed      │     │
              │     └─────────────────┘     │
              │                             │
              │           Failure ↓         │
              │     ┌─────────────────┐     │
              │     │  ManagedFailed  │     │
              │     └────────┬────────┘     │
              │              │              │
              │              │ User clicks Retry
              └──────────────┴──────────────┘
```

### 5.2 Unmanage Flow

```
                    ┌─────────────────┐
                    │     Managed     │
                    └────────┬────────┘
                             │ User initiates unmanage
                             ↓
                    ┌─────────────────┐
              ┌────▶│   Unmanaging    │◀────┐
              │     └────────┬────────┘     │
              │              │              │
              │       Success│ Failure      │
              │              ↓              │
              │     ┌─────────────────┐     │
              │     │   Unmanaged     │     │
              │     └─────────────────┘     │
              │                             │
              │           Failure ↓         │
              │     ┌─────────────────┐     │
              │     │ UnmanagedFailed │     │
              │     └────────┬────────┘     │
              │              │              │
              │              │ User clicks Retry
              └──────────────┴──────────────┘
```

---

## 6. Frontend Integration Notes

### 6.1 Retry Button Display Logic

```javascript
// Only show Retry button when in failed state
const showRetryButton = (node) => {
  return node.status.clusterStatus.phase === 'ManagedFailed' ||
         node.status.clusterStatus.phase === 'UnmanagedFailed';
};
```

### 6.2 Status Polling

```javascript
// After user clicks Retry, frontend needs to poll node status
const pollNodeStatus = async (nodeName) => {
  const response = await fetch(`/api/v1/nodes/${nodeName}`);
  const node = await response.json();
  
  const phase = node.status.clusterStatus.phase;
  
  if (phase === 'Managing' || phase === 'Unmanaging') {
    // Show "In Progress..." status, continue polling
    setTimeout(() => pollNodeStatus(nodeName), 3000);
  } else if (phase === 'Managed' || phase === 'Unmanaged') {
    // Operation successful
    showSuccess('Operation successful');
  } else if (phase === 'ManagedFailed' || phase === 'UnmanagedFailed') {
    // Operation failed, show Retry button
    showError('Operation failed, please retry');
  }
};
```

### 6.3 Button State Management

| Node State | Retry Button | Other Action Buttons |
|------------|--------------|---------------------|
| `Unmanaged` | Hidden | Show "Manage" |
| `Managing` | Hidden | All disabled |
| `Managed` | Hidden | Show "Unmanage" |
| `ManagedFailed` | **Visible** | Show "Manage" (or disabled) |
| `Unmanaging` | Hidden | All disabled |
| `UnmanagedFailed` | **Visible** | Show "Unmanage" (or disabled) |

### 6.4 Error Handling

```javascript
const handleRetry = async (nodeName) => {
  try {
    const response = await fetch(`/api/v1/nodes/${nodeName}/retry`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    });
    
    if (!response.ok) {
      const error = await response.json();
      
      // Provide user-friendly messages based on error
      switch (error.errorMessage) {
        case 'machine is not ready, please wait and try again':
          showWarning('Machine is not ready, please try again later');
          break;
        case 'control plane node cannot be unmanaged':
          showError('Control plane node cannot be unmanaged');
          break;
        default:
          showError(error.errorMessage);
      }
      return;
    }
    
    const result = await response.json();
    showInfo(`Retry initiated, status: ${result.previousPhase} → ${result.currentPhase}`);
    
    // Start polling status
    pollNodeStatus(nodeName);
    
  } catch (error) {
    showError('Network error, please retry');
  }
};
```

### 6.5 UI Interaction Recommendations

1. **Confirmation Dialog**: Show confirmation dialog before clicking Retry
   ```
   Are you sure you want to retry the manage/unmanage operation?
   [Cancel] [Confirm]
   ```

2. **Loading State**: Button shows loading state after click to prevent duplicate clicks

3. **Toast Notifications**:
   - Success: `Retry initiated`
   - Failure: Show specific error reason

4. **Status Icons**:
   - `Managing` / `Unmanaging`: Show loading spinner
   - `ManagedFailed` / `UnmanagedFailed`: Show red error icon + Retry button

---

## 7. Access Control

Retry operation uses the same permissions as manage/unmanage:

| Operation | Required Permission | Resource Type |
|-----------|---------------------|---------------|
| Retry | `update` | `nodes` |

---

## 8. Examples

### 8.1 cURL Example

```bash
# Retry a node that failed to manage
curl -X POST "http://localhost:8088/api/v1/nodes/node-1/retry" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json"
```

### 8.2 Success Response Example

```json
{
  "message": "retry initiated successfully",
  "nodeId": "node-1",
  "previousPhase": "ManagedFailed",
  "currentPhase": "Managing"
}
```

### 8.3 Failure Response Example

```json
{
  "errorCode": "Primus.00002",
  "errorMessage": "machine is not ready, please wait and try again"
}
```

---

## 9. Backend Implementation Notes

### 9.1 Core Logic

1. **Permission Check**: Verify user has `update` permission
2. **State Check**: Confirm node is in `ManagedFailed` or `UnmanagedFailed` state
3. **Pre-condition Checks**:
   - Manage retry: `IsMachineReady()`
   - Unmanage retry: `!IsControlPlane()`
4. **Cleanup Pods**: Delete previously failed KubeSpray Pods
5. **Reset Status**: Change state to `Managing` or `Unmanaging`
6. **Controller Takes Over**: State change triggers Controller to re-execute manage/unmanage

### 9.2 Code Locations

| File | Functionality |
|------|---------------|
| `apiserver/pkg/handlers/resources/node.go` | Retry API handler |
| `apiserver/pkg/handlers/resources/routers.go` | Route registration |
| `resource-manager/pkg/resource/node_controller.go` | Controller execution logic |

---

## 10. Change Log

| Date | Version | Description | Author |
|------|---------|-------------|--------|
| 2026-01-09 | v1.0 | Initial design | - |
