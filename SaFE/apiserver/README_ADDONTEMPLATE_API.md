# AddonTemplate API 文档

AddonTemplate API 用于查询和管理 Addon 模板，模板定义了 Addon 的默认配置和安装方式。

## 基础信息

- **基础路径**: `/api/v1/addontemplates`
- **认证**: 需要 Bearer Token
- **内容类型**: `application/json`
- **操作限制**: 仅支持查询操作（List/Get），创建和修改需要通过 kubectl 或其他管理工具

## API 端点

### 1. 获取 AddonTemplate 列表

**请求**:
```
GET /api/v1/addontemplates
```

**响应**:
```json
{
  "totalCount": 3,
  "items": [
    {
      "addonTemplateId": "amd-gpu-driver-v1",
      "type": "helm",
      "category": "gpu",
      "version": "1.0.0",
      "description": "AMD GPU driver for Kubernetes",
      "gpuChip": "amd",
      "required": true,
      "creationTime": "2025-10-31T10:00:00Z"
    },
    {
      "addonTemplateId": "nvidia-gpu-driver-v1",
      "type": "helm",
      "category": "gpu",
      "version": "2.0.0",
      "description": "NVIDIA GPU driver for Kubernetes",
      "gpuChip": "nvidia",
      "required": true,
      "creationTime": "2025-10-31T11:00:00Z"
    },
    {
      "addonTemplateId": "monitoring-stack",
      "type": "helm",
      "category": "system",
      "version": "1.5.0",
      "description": "Prometheus and Grafana monitoring stack",
      "gpuChip": "",
      "required": false,
      "creationTime": "2025-10-31T12:00:00Z"
    }
  ]
}
```

---

### 2. 获取单个 AddonTemplate

**请求**:
```
GET /api/v1/addontemplates/{template-id}
```

**响应**:
```json
{
  "addonTemplateId": "amd-gpu-driver-v1",
  "type": "helm",
  "category": "gpu",
  "version": "1.0.0",
  "description": "AMD GPU driver for Kubernetes clusters",
  "gpuChip": "amd",
  "required": true,
  "creationTime": "2025-10-31T10:00:00Z",
  "url": "https://charts.amd.com/gpu-driver",
  "action": "base64EncodedInstallScript",
  "icon": "base64EncodedIcon",
  "helmDefaultValues": "driver:\n  version: latest\n  replicas: 1",
  "helmDefaultNamespace": "kube-system",
  "helmStatus": {
    "values": "current values",
    "valuesYaml": "driver:\n  version: latest"
  }
}
```

---

## 字段说明

### 基础字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `addonTemplateId` | string | 模板唯一标识符 |
| `type` | string | 模板类型（helm/default） |
| `category` | string | 模板分类（system/gpu/network 等） |
| `version` | string | 模板版本号 |
| `description` | string | 模板描述 |
| `gpuChip` | string | 目标 GPU 类型（amd/nvidia/空表示通用） |
| `required` | boolean | 是否为必需模板 |
| `creationTime` | timestamp | 创建时间 |

### 详细字段（仅 Get 接口返回）

| 字段 | 类型 | 说明 |
|------|------|------|
| `url` | string | Helm Chart URL |
| `action` | string | 安装脚本（Base64 编码） |
| `icon` | string | 图标（Base64 编码） |
| `helmDefaultValues` | string | Helm 默认 values（YAML 格式） |
| `helmDefaultNamespace` | string | 默认部署命名空间 |
| `helmStatus` | object | Helm 状态信息 |

---

## 模板类型说明

### Type（模板类型）

- `helm`: Helm Chart 类型，支持通过 Helm 部署
- `default`: 默认类型，自定义安装方式

### Category（分类）

常见分类：
- `system`: 系统级组件（监控、日志等）
- `gpu`: GPU 相关组件
- `network`: 网络组件
- `storage`: 存储组件
- 其他自定义分类

### GpuChip（GPU 类型）

- `amd`: AMD GPU
- `nvidia`: NVIDIA GPU
- 空字符串: 适用于所有 GPU 或非 GPU 场景

### Required（必需标记）

- `true`: 必需模板，安装失败会终止流程
- `false`: 可选模板，安装失败仅记录日志

---

## 使用示例

### 示例 1: 查询所有模板

```bash
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN"
```

### 示例 2: 查询特定模板详情

```bash
curl -X GET "http://api-server/api/v1/addontemplates/amd-gpu-driver-v1" \
  -H "Authorization: Bearer $TOKEN"
```

### 示例 3: 筛选 AMD GPU 模板（客户端过滤）

```bash
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '.items[] | select(.gpuChip == "amd" or .gpuChip == "")'
```

### 示例 4: 查询必需模板（客户端过滤）

```bash
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '.items[] | select(.required == true)'
```

---

## 使用场景

### 1. 插件市场

展示所有可用的 Addon 模板供用户选择：

```bash
# 获取所有模板
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN"

# 按分类展示
jq 'group_by(.category)' templates.json
```

### 2. 集群初始化

获取必需的模板列表，用于新集群初始化：

```bash
# 获取所有必需模板
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '.items[] | select(.required == true)'
```

### 3. GPU 环境配置

根据 GPU 类型筛选适配的模板：

```bash
# AMD GPU
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '.items[] | select(.gpuChip == "amd" or .gpuChip == "")'

# NVIDIA GPU
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '.items[] | select(.gpuChip == "nvidia" or .gpuChip == "")'
```

### 4. 查看模板详细配置

在创建 Addon 前查看模板的默认配置：

```bash
curl -X GET "http://api-server/api/v1/addontemplates/amd-gpu-driver-v1" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '{
    name: .addonTemplateId,
    namespace: .helmDefaultNamespace,
    values: .helmDefaultValues
  }'
```

---

## 与 Addon 的关系

### 工作流程

```
1. 查询 AddonTemplate
   ↓
2. 选择合适的模板
   ↓
3. 使用模板创建 Addon
   ↓
4. Addon 继承模板配置
   ↓
5. 可选：覆盖部分配置
```

### 示例：完整流程

```bash
# 步骤 1: 查询可用模板
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN"

# 步骤 2: 查看特定模板详情
curl -X GET "http://api-server/api/v1/addontemplates/amd-gpu-driver-v1" \
  -H "Authorization: Bearer $TOKEN"

# 步骤 3: 基于模板创建 Addon
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gpu-driver",
    "template": "amd-gpu-driver-v1"
  }'
```

---

## 创建和管理模板

AddonTemplate 的创建和修改需要通过 Kubernetes CRD 方式：

### 创建模板示例（使用 kubectl）

```yaml
apiVersion: amd.com/v1
kind: AddonTemplate
metadata:
  name: my-custom-addon
spec:
  type: helm
  category: custom
  url: https://charts.example.com/my-addon
  version: 1.0.0
  description: My custom addon template
  gpuChip: ""  # 适用于所有类型
  required: false
  helmDefaultNamespace: default
  helmDefaultValues: |
    replicaCount: 1
    image:
      repository: my-app
      tag: latest
```

应用模板：

```bash
kubectl apply -f my-addon-template.yaml
```

---

## 注意事项

1. **只读 API**: 当前 API 只支持查询操作，不支持通过 API 创建或修改模板
2. **Base64 编码**: `action` 和 `icon` 字段使用 Base64 编码，需要解码后使用
3. **GPU 适配**: 空的 `gpuChip` 表示该模板适用于所有 GPU 类型或非 GPU 场景
4. **必需模板**: `required=true` 的模板应在集群初始化时优先安装
5. **版本管理**: 同一 Addon 的不同版本应创建不同的 AddonTemplate
6. **命名规范**: 建议使用 `{category}-{name}-{version}` 格式命名

---

## 模板命名最佳实践

### 推荐格式

- `{vendor}-{product}-{version}`: `amd-gpu-driver-v1`
- `{category}-{name}-{version}`: `gpu-monitoring-v2`
- `{purpose}-stack-{version}`: `monitoring-stack-v1`

### 示例

- ✅ `amd-gpu-driver-v1`
- ✅ `nvidia-gpu-operator-v2`
- ✅ `monitoring-stack-v1`
- ✅ `storage-csi-nfs-v1`
- ❌ `driver` (太简单)
- ❌ `gpu-1.0.0` (版本不应在名称中使用点号)

---

## 错误响应

### 404 Not Found

```json
{
  "error": "addontemplate not found"
}
```

原因：AddonTemplate 不存在

### 403 Forbidden

```json
{
  "error": "permission denied"
}
```

原因：没有查询权限或 Token 无效

---

## 相关文档

- [Addon API 文档](./README_ADDON_API.md)
- [Kubernetes CRD 文档](./CRD.md)
- [认证与授权](./AUTH.md)

