# Addon API 文档

Addon API 用于管理集群中的插件/扩展组件，所有 Addon 必须基于 AddonTemplate 创建。

## 基础信息

- **基础路径**: `/api/v1/clusters/:cluster/addons`
- **认证**: 需要 Bearer Token
- **内容类型**: `application/json`

## 核心概念

### Addon 命名机制

Addon 使用**双重命名机制**：

1. **ReleaseName**: 用户指定的 Helm Release 名称（在请求中提供）
2. **Name**: 系统自动生成的 Kubernetes CRD 对象名称

**命名规则**:
```
name = {cluster}-{namespace}-{releaseName}
```

**示例**:
- Cluster: `my-cluster`
- Namespace: `kube-system`
- ReleaseName: `gpu-driver`
- **生成的 Name**: `my-cluster-kube-system-gpu-driver`

> **注意**: 如果未指定 namespace，默认使用 `default`

---

## API 端点

### 1. 创建 Addon

**请求**:
```
POST /api/v1/clusters/{cluster-id}/addons
```

**请求体**:
```json
{
  "releaseName": "gpu-driver",
  "template": "amd-gpu-driver-template",
  "namespace": "kube-system",
  "values": "driver:\n  version: latest",
  "description": "AMD GPU driver addon"
}
```

**字段说明**:
- `releaseName` (必填): Helm Release 名称（用户定义）
- `template` (必填): AddonTemplate ID
- `namespace` (可选): 部署命名空间，不指定则使用模板默认值
- `values` (可选): Helm values（YAML 格式），不指定则使用模板默认值
- `description` (可选): 描述信息

**响应**:
```json
{
  "name": "my-cluster-kube-system-gpu-driver",
  "releaseName": "gpu-driver",
  "description": "AMD GPU driver addon",
  "template": "amd-gpu-driver-template",
  "namespace": "kube-system",
  "values": "driver:\n  version: latest",
  "cluster": "my-cluster",
  "status": {
    "status": "deployed",
    "version": 1,
    "chartVersion": "1.0.0",
    "firstDeployed": "2025-10-31T10:00:00Z",
    "lastDeployed": "2025-10-31T10:00:00Z"
  }
}
```

**响应字段说明**:
- `name`: 系统生成的 Kubernetes CRD 对象名称
- `releaseName`: 用户指定的 Helm Release 名称
- `cluster`: 所属集群名称

---

### 2. 获取 Addon 列表

**请求**:
```
GET /api/v1/clusters/{cluster-id}/addons
```

**查询参数**: 无

**响应**:
```json
{
  "totalCount": 2,
  "items": [
    {
      "name": "my-cluster-default-addon-1",
      "releaseName": "addon-1",
      "template": "template-1",
      "namespace": "default",
      "cluster": "my-cluster",
      "status": {
        "status": "deployed",
        "version": 1
      }
    },
    {
      "name": "my-cluster-kube-system-addon-2",
      "releaseName": "addon-2",
      "template": "template-2",
      "namespace": "kube-system",
      "cluster": "my-cluster",
      "status": {
        "status": "deployed",
        "version": 1
      }
    }
  ]
}
```

**说明**: 
- 列表按 `name`（生成的名称）排序
- 返回所有字段，包括状态信息

---

### 3. 获取单个 Addon

**请求**:
```
GET /api/v1/clusters/{cluster-id}/addons/{addon-name}
```

**URL 参数**:
- `addon-name`: Addon 的完整名称（系统生成的，格式为 `{cluster}-{namespace}-{releaseName}`）

**示例**:
```
GET /api/v1/clusters/my-cluster/addons/my-cluster-kube-system-gpu-driver
```

**响应**: 与创建 Addon 的响应格式相同

```json
{
  "name": "my-cluster-kube-system-gpu-driver",
  "releaseName": "gpu-driver",
  "description": "AMD GPU driver addon",
  "template": "amd-gpu-driver-template",
  "namespace": "kube-system",
  "values": "driver:\n  version: latest",
  "cluster": "my-cluster",
  "status": {
    "status": "deployed",
    "version": 1,
    "chartVersion": "1.0.0"
  }
}
```

---

### 4. 更新 Addon

**请求**:
```
PATCH /api/v1/clusters/{cluster-id}/addons/{addon-name}
```

**URL 参数**:
- `addon-name`: Addon 的完整名称（系统生成的）

**请求体**:
```json
{
  "description": "Updated GPU driver description",
  "template": "amd-gpu-driver-v2",
  "values": "driver:\n  version: 24.04"
}
```

**字段说明**:
- `description` (可选): 更新 Addon 描述
- `template` (必填): 更新 AddonTemplate 引用
- `values` (可选): 更新 Helm values（YAML 格式）

**说明**: 
- 可以更新 `description`、`template` 和 `values` 字段
- 不能更改 `releaseName` 和 `namespace`（不可变字段）
- `template` 字段为必填，如果不想更改可传入当前值
- 所有字段都是可选的，只需提供要更新的字段

**响应**: 空响应（204 No Content）

---

### 5. 删除 Addon

**请求**:
```
DELETE /api/v1/clusters/{cluster-id}/addons/{addon-name}
```

**URL 参数**:
- `addon-name`: Addon 的完整名称（系统生成的）

**示例**:
```
DELETE /api/v1/clusters/my-cluster/addons/my-cluster-kube-system-gpu-driver
```

**响应**: 空响应（204 No Content）

**说明**: 删除 Addon 会卸载对应的 Helm Release

---

## 状态字段说明

`AddonStatus` 包含以下字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | 部署状态（deployed, failed 等） |
| `version` | int | Release 版本号 |
| `chartVersion` | string | Chart 版本 |
| `firstDeployed` | timestamp | 首次部署时间 |
| `lastDeployed` | timestamp | 最后部署时间 |
| `deleted` | timestamp | 删除时间 |
| `description` | string | 状态描述 |
| `notes` | string | 部署说明 |
| `values` | string | 当前使用的 values |
| `previousVersion` | int | 上一个版本号 |

---

## 使用示例

### 示例 1: 使用模板默认配置创建 Addon

```bash
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "gpu-driver",
    "template": "amd-gpu-driver-v1"
  }'

# 响应示例
{
  "name": "my-cluster-default-gpu-driver",
  "releaseName": "gpu-driver",
  "template": "amd-gpu-driver-v1",
  "namespace": "default",
  "cluster": "my-cluster",
  "status": {...}
}
```

### 示例 2: 创建 Addon 并自定义配置

```bash
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "gpu-driver",
    "template": "amd-gpu-driver-v1",
    "namespace": "gpu-system",
    "values": "driver:\n  version: 24.04\n  replicas: 2",
    "description": "Custom GPU driver"
  }'

# 生成的名称: my-cluster-gpu-system-gpu-driver
```

### 示例 3: 查询集群的所有 Addon

```bash
curl -X GET "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN"
```

### 示例 4: 获取特定 Addon（使用完整的生成名称）

```bash
# 注意：必须使用完整的生成名称
curl -X GET "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-gpu-system-gpu-driver" \
  -H "Authorization: Bearer $TOKEN"
```

### 示例 5: 更新 Addon 配置

```bash
# 使用完整的生成名称 - 只更新 values
curl -X PATCH "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-gpu-system-gpu-driver" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "template": "amd-gpu-driver-v1",
    "values": "driver:\n  version: 24.10"
  }'

# 更新多个字段
curl -X PATCH "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-gpu-system-gpu-driver" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Updated GPU driver with new version",
    "template": "amd-gpu-driver-v1",
    "values": "driver:\n  version: 24.10\n  replicas: 3"
  }'

# 切换到新的模板版本
curl -X PATCH "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-gpu-system-gpu-driver" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Upgrade to v2 template",
    "template": "amd-gpu-driver-v2",
    "values": "driver:\n  version: 24.10"
  }'
```

### 示例 6: 删除 Addon

```bash
# 使用完整的生成名称
curl -X DELETE "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-gpu-system-gpu-driver" \
  -H "Authorization: Bearer $TOKEN"
```

### 示例 7: 同一集群不同命名空间部署相同 Release

```bash
# 在 default 命名空间创建
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "monitoring",
    "template": "prometheus-v1"
  }'
# 生成名称: my-cluster-default-monitoring

# 在 dev 命名空间创建同名 Release
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "monitoring",
    "template": "prometheus-v1",
    "namespace": "dev"
  }'
# 生成名称: my-cluster-dev-monitoring
```

---

## 注意事项

### 命名相关

1. **双重命名**: 区分 `releaseName`（用户指定）和 `name`（系统生成）
2. **名称生成规则**: `name = {cluster}-{namespace}-{releaseName}`
3. **名称唯一性**: 同一集群的同一命名空间内，`releaseName` 必须唯一
4. **API 操作**: GET/PATCH/DELETE 操作必须使用完整的生成 `name`，而不是 `releaseName`

### 创建和配置

5. **必须基于模板**: 所有 Addon 必须指定有效的 AddonTemplate
6. **Cluster 参数**: Cluster ID 通过 URL 路径指定，不在请求体中
7. **配置覆盖**: `namespace` 和 `values` 为可选，未指定时使用模板默认值
8. **只读字段**: `name`、`cluster` 和 `status` 字段由系统自动设置

### 更新和删除

9. **可更新字段**: 可以更新 `description`、`template` 和 `values`
10. **不可变字段**: `releaseName` 和 `namespace` 不能更改
11. **部分更新**: PATCH 支持部分更新，但 `template` 字段为必填
12. **模板切换**: 可以通过更新 `template` 字段切换到不同的 AddonTemplate
13. **删除注意**: 删除 Addon 会同时删除 Helm Release

### 命名空间隔离

11. **命名空间隔离**: 可以在不同命名空间部署相同 `releaseName` 的 Addon
12. **默认命名空间**: 未指定 `namespace` 时，使用模板的默认命名空间或 `default`

---

## 命名规则详解

### 为什么需要两个名称？

| 名称 | 用途 | 示例 | 说明 |
|------|------|------|------|
| `releaseName` | Helm Release 标识 | `gpu-driver` | 用户友好，简短 |
| `name` | Kubernetes CRD 对象标识 | `my-cluster-kube-system-gpu-driver` | 全局唯一，包含上下文 |

### 命名规则示例

| Cluster | Namespace | ReleaseName | 生成的 Name |
|---------|-----------|-------------|------------|
| `cluster-1` | `default` | `gpu` | `cluster-1-default-gpu` |
| `cluster-1` | `kube-system` | `gpu` | `cluster-1-kube-system-gpu` |
| `cluster-2` | `default` | `gpu` | `cluster-2-default-gpu` |
| `prod` | (空) | `monitoring` | `prod-default-monitoring` |

### 命名优势

1. **全局唯一性**: 通过包含 cluster 和 namespace 避免名称冲突
2. **命名空间隔离**: 支持同一集群不同命名空间使用相同 releaseName
3. **可追溯性**: 从名称即可知道 Addon 所属的集群和命名空间
4. **Helm 兼容**: releaseName 保持简短，符合 Helm 惯例

---

## 错误响应

### 400 Bad Request

```json
{
  "error": "template is required"
}
```

常见原因：
- 未提供 `template` 字段
- `releaseName` 字段为空
- Cluster 参数无效
- 模板类型不是 Helm

### 404 Not Found

```json
{
  "error": "addon not found"
}
```

常见原因：
- 使用了错误的 Addon 名称（应使用生成的完整名称，而不是 releaseName）
- Addon 不存在
- Cluster 不存在
- AddonTemplate 不存在

### 403 Forbidden

```json
{
  "error": "permission denied"
}
```

常见原因：
- 没有操作权限
- Token 无效或过期

---

## 相关文档

- [AddonTemplate API 文档](./README_ADDONTEMPLATE_API.md)
- [认证与授权](./AUTH.md)

