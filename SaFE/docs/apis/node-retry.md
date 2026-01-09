# Node Retry API 设计文档

## 1. 功能概述

提供节点纳管/解纳管失败后的重试功能，允许用户在操作失败时点击重试，而无需重新发起整个流程。

### 1.1 适用场景

| 失败状态 | 描述 | 重试后状态 |
|---------|------|-----------|
| `ManagedFailed` | 纳管失败 | `Managing` |
| `UnmanagedFailed` | 解纳管失败 | `Unmanaging` |

### 1.2 核心能力

- ✅ 支持纳管失败重试
- ✅ 支持解纳管失败重试
- ✅ 自动清理失败的 Pod
- ✅ 前置条件检查（机器就绪、工作空间解绑等）
- ✅ 权限控制（与纳管操作相同）

---

## 2. API 接口

### 2.1 重试节点操作

**请求**

```
POST /api/v1/nodes/{nodeName}/retry
```

**请求头**

```
Authorization: Bearer <token>
Content-Type: application/json
```

**路径参数**

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| nodeName | string | 是 | 节点名称 |

**请求体**

无需请求体。

**成功响应 (200 OK)**

```json
{
  "message": "retry initiated successfully",
  "nodeId": "node-1",
  "previousPhase": "ManagedFailed",
  "currentPhase": "Managing"
}
```

**响应字段说明**

| 字段 | 类型 | 描述 |
|------|------|------|
| message | string | 操作结果消息 |
| nodeId | string | 节点 ID |
| previousPhase | string | 重试前的状态 |
| currentPhase | string | 重试后的状态 |

---

## 3. 错误响应

### 3.1 错误码列表

| HTTP 状态码 | 错误码 | 错误消息 | 描述 |
|------------|--------|---------|------|
| 400 | Primus.00002 | node is not in a failed state | 节点不在失败状态 |
| 400 | Primus.00002 | machine is not ready, please wait and try again | 机器未就绪（纳管重试） |
| 400 | Primus.00002 | control plane node cannot be unmanaged | 控制平面节点不能解纳管 |
| 400 | Primus.00002 | node is still bound to a workspace, please unbind first | 工作空间未解绑（解纳管重试） |
| 400 | Primus.00002 | cannot determine cluster ID for retry operation | 无法确定集群 ID |
| 403 | Primus.00004 | Forbidden | 用户无权限执行此操作 |
| 404 | Primus.00005 | node not found | 节点不存在 |
| 500 | Primus.00001 | failed to delete failed pods | 删除失败 Pod 出错 |
| 500 | Primus.00001 | failed to reset node status | 更新节点状态出错 |

### 3.2 错误响应格式

```json
{
  "errorCode": "Primus.00002",
  "errorMessage": "node is not in a failed state, current phase: Managed. Only ManagedFailed or UnmanagedFailed nodes can be retried"
}
```

---

## 4. 前置条件检查

### 4.1 纳管重试 (ManagedFailed → Managing)

| 检查项 | 条件 | 错误消息 |
|--------|------|---------|
| 机器就绪 | `node.IsMachineReady() == true` | machine is not ready, please wait and try again |

### 4.2 解纳管重试 (UnmanagedFailed → Unmanaging)

| 检查项 | 条件 | 错误消息 |
|--------|------|---------|
| 非控制平面节点 | `IsControlPlane(node) == false` | control plane node cannot be unmanaged |
| 工作空间已解绑 | `GetWorkspaceId(node) == ""` | node is still bound to a workspace, please unbind first |

---

## 5. 状态流转图

### 5.1 纳管流程

```
                    ┌─────────────────┐
                    │    Unmanaged    │
                    └────────┬────────┘
                             │ 用户发起纳管
                             ↓
                    ┌─────────────────┐
              ┌────▶│    Managing     │◀────┐
              │     └────────┬────────┘     │
              │              │              │
              │         成功 │ 失败         │
              │              ↓              │
              │     ┌─────────────────┐     │
              │     │    Managed      │     │
              │     └─────────────────┘     │
              │                             │
              │              ↓ 失败         │
              │     ┌─────────────────┐     │
              │     │  ManagedFailed  │     │
              │     └────────┬────────┘     │
              │              │              │
              │              │ 用户点击 Retry
              └──────────────┴──────────────┘
```

### 5.2 解纳管流程

```
                    ┌─────────────────┐
                    │     Managed     │
                    └────────┬────────┘
                             │ 用户发起解纳管
                             ↓
                    ┌─────────────────┐
              ┌────▶│   Unmanaging    │◀────┐
              │     └────────┬────────┘     │
              │              │              │
              │         成功 │ 失败         │
              │              ↓              │
              │     ┌─────────────────┐     │
              │     │   Unmanaged     │     │
              │     └─────────────────┘     │
              │                             │
              │              ↓ 失败         │
              │     ┌─────────────────┐     │
              │     │ UnmanagedFailed │     │
              │     └────────┬────────┘     │
              │              │              │
              │              │ 用户点击 Retry
              └──────────────┴──────────────┘
```

---

## 6. 前端对接注意事项

### 6.1 Retry 按钮显示逻辑

```javascript
// 只有在失败状态时显示 Retry 按钮
const showRetryButton = (node) => {
  return node.status.clusterStatus.phase === 'ManagedFailed' ||
         node.status.clusterStatus.phase === 'UnmanagedFailed';
};
```

### 6.2 状态轮询

```javascript
// 用户点击 Retry 后，前端需要轮询节点状态
const pollNodeStatus = async (nodeName) => {
  const response = await fetch(`/api/v1/nodes/${nodeName}`);
  const node = await response.json();
  
  const phase = node.status.clusterStatus.phase;
  
  if (phase === 'Managing' || phase === 'Unmanaging') {
    // 显示 "操作中..." 状态，继续轮询
    setTimeout(() => pollNodeStatus(nodeName), 3000);
  } else if (phase === 'Managed' || phase === 'Unmanaged') {
    // 操作成功
    showSuccess('操作成功');
  } else if (phase === 'ManagedFailed' || phase === 'UnmanagedFailed') {
    // 操作失败，显示 Retry 按钮
    showError('操作失败，请重试');
  }
};
```

### 6.3 按钮状态管理

| 节点状态 | Retry 按钮 | 其他操作按钮 |
|---------|-----------|-------------|
| `Unmanaged` | 隐藏 | 显示"纳管" |
| `Managing` | 隐藏 | 全部禁用 |
| `Managed` | 隐藏 | 显示"解纳管" |
| `ManagedFailed` | **显示** | 显示"纳管"（或禁用） |
| `Unmanaging` | 隐藏 | 全部禁用 |
| `UnmanagedFailed` | **显示** | 显示"解纳管"（或禁用） |

### 6.4 错误处理

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
      
      // 根据错误消息给用户友好提示
      switch (error.errorMessage) {
        case 'machine is not ready, please wait and try again':
          showWarning('机器未就绪，请稍后再试');
          break;
        case 'node is still bound to a workspace, please unbind first':
          showWarning('请先解绑工作空间');
          break;
        case 'control plane node cannot be unmanaged':
          showError('控制平面节点不能解纳管');
          break;
        default:
          showError(error.errorMessage);
      }
      return;
    }
    
    const result = await response.json();
    showInfo(`重试已发起，状态: ${result.previousPhase} → ${result.currentPhase}`);
    
    // 开始轮询状态
    pollNodeStatus(nodeName);
    
  } catch (error) {
    showError('网络错误，请重试');
  }
};
```

### 6.5 UI 交互建议

1. **确认对话框**：点击 Retry 前显示确认对话框
   ```
   确定要重试纳管/解纳管操作吗？
   [取消] [确定]
   ```

2. **Loading 状态**：点击后按钮显示 loading 状态，防止重复点击

3. **Toast 提示**：
   - 成功：`重试已发起`
   - 失败：显示具体错误原因

4. **状态图标**：
   - `Managing` / `Unmanaging`：显示 loading spinner
   - `ManagedFailed` / `UnmanagedFailed`：显示红色错误图标 + Retry 按钮

---

## 7. 权限控制

Retry 操作使用与纳管/解纳管相同的权限：

| 操作 | 所需权限 | 资源类型 |
|------|---------|---------|
| Retry | `update` | `nodes` |

---

## 8. 示例

### 8.1 cURL 示例

```bash
# 重试纳管失败的节点
curl -X POST "http://localhost:8088/api/v1/nodes/node-1/retry" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json"
```

### 8.2 成功响应示例

```json
{
  "message": "retry initiated successfully",
  "nodeId": "node-1",
  "previousPhase": "ManagedFailed",
  "currentPhase": "Managing"
}
```

### 8.3 失败响应示例

```json
{
  "errorCode": "Primus.00002",
  "errorMessage": "machine is not ready, please wait and try again"
}
```

---

## 9. 后端实现要点

### 9.1 核心逻辑

1. **权限检查**：验证用户有 `update` 权限
2. **状态检查**：确认节点处于 `ManagedFailed` 或 `UnmanagedFailed`
3. **前置条件检查**：
   - 纳管重试：`IsMachineReady()`
   - 解纳管重试：`!IsControlPlane()` && `GetWorkspaceId() == ""`
4. **清理 Pod**：删除之前失败的 KubeSpray Pod
5. **状态重置**：将状态改为 `Managing` 或 `Unmanaging`
6. **Controller 接管**：状态变化触发 Controller 重新执行纳管/解纳管

### 9.2 代码位置

| 文件 | 功能 |
|------|------|
| `apiserver/pkg/handlers/resources/node.go` | Retry API 处理 |
| `apiserver/pkg/handlers/resources/routers.go` | 路由注册 |
| `resource-manager/pkg/resource/node_controller.go` | Controller 执行逻辑 |

---

## 10. 变更记录

| 日期 | 版本 | 描述 | 作者 |
|------|------|------|------|
| 2026-01-09 | v1.0 | 初始设计 | - |

