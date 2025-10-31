# Addon 管理系统文档

本文档介绍 Addon 管理系统的整体架构和 API 使用方法。

## 📚 文档索引

- [Addon API 文档](./README_ADDON_API.md) - Addon 实例管理 API
- [AddonTemplate API 文档](./README_ADDONTEMPLATE_API.md) - AddonTemplate 查询 API

## 🎯 系统概述

Addon 管理系统用于在 Kubernetes 集群中部署和管理扩展组件（如 GPU 驱动、监控栈、存储插件等）。

### 核心概念

```
AddonTemplate (模板)
    ↓ 定义配置
    ↓
Addon (实例)
    ↓ 部署到
    ↓
Cluster (集群)
```

### 关系说明

- **AddonTemplate**: 定义 Addon 的模板和默认配置（what to install）
- **Addon**: 基于 AddonTemplate 的具体安装实例（installed instance）
- **Cluster**: Addon 部署的目标 Kubernetes 集群

### Addon 命名机制

Addon 使用**双重命名机制**：

| 名称类型 | 示例 | 说明 |
|---------|------|------|
| **releaseName** | `gpu-driver` | 用户指定的 Helm Release 名称 |
| **name** | `my-cluster-kube-system-gpu-driver` | 系统生成的 Kubernetes CRD 对象名称 |

**命名规则**: `name = {cluster}-{namespace}-{releaseName}`

> ⚠️ **重要**: GET/PATCH/DELETE 操作必须使用完整的 `name`，而不是 `releaseName`

## 🚀 快速开始

### 第 1 步：查询可用模板

```bash
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN"
```

### 第 2 步：创建 Addon

```bash
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "gpu-driver",
    "template": "amd-gpu-driver-v1"
  }'

# 响应包含生成的名称
{
  "name": "my-cluster-default-gpu-driver",
  "releaseName": "gpu-driver",
  ...
}
```

### 第 3 步：查看 Addon 状态（使用生成的名称）

```bash
# 注意：使用完整的生成名称
curl -X GET "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-default-gpu-driver" \
  -H "Authorization: Bearer $TOKEN"
```

## 📋 API 概览

### Addon API（实例管理）

| 方法 | 端点 | 说明 |
|------|------|------|
| POST | `/api/v1/clusters/:cluster/addons` | 创建 Addon |
| GET | `/api/v1/clusters/:cluster/addons` | 列出 Addons |
| GET | `/api/v1/clusters/:cluster/addons/:name` | 获取 Addon |
| PATCH | `/api/v1/clusters/:cluster/addons/:name` | 更新 Addon |
| DELETE | `/api/v1/clusters/:cluster/addons/:name` | 删除 Addon |

### AddonTemplate API（模板查询）

| 方法 | 端点 | 说明 |
|------|------|------|
| GET | `/api/v1/addontemplates` | 列出模板 |
| GET | `/api/v1/addontemplates/:name` | 获取模板 |

> 注意：AddonTemplate 的创建和修改需要通过 kubectl 操作 Kubernetes CRD

## 💡 使用场景

### 场景 1：部署 GPU 驱动

```bash
# 1. 查询 GPU 相关模板
curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '.items[] | select(.category == "gpu")'

# 2. 创建 GPU 驱动 Addon
curl -X POST "http://api-server/api/v1/clusters/gpu-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "amd-gpu-driver",
    "template": "amd-gpu-driver-v1",
    "namespace": "kube-system"
  }'

# 响应中的生成名称: gpu-cluster-kube-system-amd-gpu-driver
```

### 场景 2：集群初始化

```bash
# 1. 获取所有必需模板
REQUIRED_TEMPLATES=$(curl -X GET "http://api-server/api/v1/addontemplates" \
  -H "Authorization: Bearer $TOKEN" | \
  jq -r '.items[] | select(.required == true) | .addonTemplateId')

# 2. 批量创建必需 Addons
for template in $REQUIRED_TEMPLATES; do
  curl -X POST "http://api-server/api/v1/clusters/new-cluster/addons" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"releaseName\": \"$template\",
      \"template\": \"$template\"
    }"
done
```

### 场景 3：自定义配置部署

```bash
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "monitoring",
    "template": "monitoring-stack-v1",
    "namespace": "monitoring",
    "values": "prometheus:\n  retention: 30d\ngrafana:\n  adminPassword: secret"
  }'

# 生成的名称: my-cluster-monitoring-monitoring
```

### 场景 4：更新 Addon 配置

```bash
# 使用完整的生成名称 - 更新 values
curl -X PATCH "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-monitoring-monitoring" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "template": "monitoring-stack-v1",
    "values": "prometheus:\n  retention: 60d"
  }'

# 更新多个字段
curl -X PATCH "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-monitoring-monitoring" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Production monitoring stack",
    "template": "monitoring-stack-v1",
    "values": "prometheus:\n  retention: 90d\n  replicas: 2"
  }'

# 切换到新版本模板
curl -X PATCH "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-monitoring-monitoring" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Upgrade to monitoring v2",
    "template": "monitoring-stack-v2",
    "values": "prometheus:\n  retention: 90d"
  }'
```

### 场景 5：同一集群多命名空间部署

```bash
# 在 prod 命名空间部署监控
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "monitoring",
    "template": "monitoring-stack-v1",
    "namespace": "prod"
  }'
# 生成名称: my-cluster-prod-monitoring

# 在 dev 命名空间部署监控
curl -X POST "http://api-server/api/v1/clusters/my-cluster/addons" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "releaseName": "monitoring",
    "template": "monitoring-stack-v1",
    "namespace": "dev"
  }'
# 生成名称: my-cluster-dev-monitoring
```

## 🔑 核心特性

### 1. 模板驱动

所有 Addon 必须基于 AddonTemplate 创建，确保配置标准化：

```
模板定义 → Addon 继承 → 可选覆盖
```

### 2. 配置继承与覆盖

```json
{
  "releaseName": "gpu-driver",         // Helm Release 名称
  "template": "amd-gpu-driver-v1",     // 继承模板配置
  "namespace": "gpu-system",           // 覆盖命名空间
  "values": "driver:\n  version: 24.04" // 覆盖 values
}
```

**响应**:
```json
{
  "name": "my-cluster-gpu-system-gpu-driver",  // 生成的 CRD 名称
  "releaseName": "gpu-driver",                 // Helm Release 名称
  ...
}
```

### 3. RESTful 设计

- Cluster 作为资源路径的一部分
- 标准的 HTTP 方法（GET/POST/PATCH/DELETE）
- 统一的响应格式

### 4. 状态跟踪

Addon 包含详细的部署状态信息：
- 部署时间
- 版本号
- Chart 版本
- 部署状态

## 📊 数据流

### 创建流程

```
用户请求
  ↓
API 验证
  ↓
获取 AddonTemplate
  ↓
继承模板配置
  ↓
应用用户覆盖
  ↓
创建 Addon CRD
  ↓
Helm 部署
  ↓
更新状态
```

### 查询流程

```
用户请求
  ↓
API 验证
  ↓
查询 Addon CRD
  ↓
转换响应格式
  ↓
返回结果
```

## 🛡️ 安全性

### 认证

所有 API 都需要 Bearer Token 认证：

```bash
-H "Authorization: Bearer $TOKEN"
```

### 授权

基于 RBAC 的权限控制：
- 创建/删除 Addon：需要 `create`/`delete` 权限
- 查询 Addon：需要 `get`/`list` 权限
- 更新 Addon：需要 `update` 权限

## ⚠️ 注意事项

### Addon 创建

1. **双重命名机制**: 
   - `releaseName`: 用户指定的 Helm Release 名称（在请求中提供）
   - `name`: 系统生成的 CRD 对象名称（格式：`{cluster}-{namespace}-{releaseName}`）
2. **必须指定 template**: 不支持完全手动配置
3. **Cluster 在 URL 中**: 不在请求体中指定
4. **配置优先级**: 用户配置 > 模板配置
5. **名称唯一性**: 同一集群的同一命名空间内，`releaseName` 必须唯一
6. **命名空间隔离**: 可以在不同命名空间使用相同的 `releaseName`

### Addon 更新

1. **使用生成名称**: GET/PATCH/DELETE 操作必须使用完整的生成 `name`
2. **可更新字段**: 可以更新 `description`、`template` 和 `values`
3. **不可变字段**: `releaseName` 和 `namespace` 不能更改
4. **模板切换**: 支持切换到不同的 AddonTemplate（如版本升级）
5. **必填字段**: PATCH 操作时 `template` 为必填字段
6. **滚动更新**: 配置更新会触发 Helm upgrade

### AddonTemplate

1. **只读 API**：不支持通过 API 创建或修改
2. **版本管理**：不同版本应创建不同的模板
3. **兼容性**：确保模板与目标集群版本兼容

## 🔧 故障排查

### 常见错误

#### 1. "template is required"

**原因**: 未提供 template 字段

**解决**:
```bash
# 错误
{"releaseName": "addon1"}

# 正确
{"releaseName": "addon1", "template": "template-v1"}
```

#### 2. "cluster parameter is required in URL path"

**原因**：URL 路径中缺少 cluster 参数

**解决**：
```bash
# 错误
POST /api/v1/addons

# 正确
POST /api/v1/clusters/my-cluster/addons
```

#### 3. "addon not found"

**原因**: 使用了错误的名称或 Addon 不存在

**解决**:
```bash
# 错误：使用 releaseName
curl -X GET "http://api-server/api/v1/clusters/my-cluster/addons/gpu-driver"

# 正确：使用完整的生成 name
curl -X GET "http://api-server/api/v1/clusters/my-cluster/addons/my-cluster-kube-system-gpu-driver"

# 或者先列出所有 Addons 查看正确的名称
curl -X GET "http://api-server/api/v1/clusters/my-cluster/addons"
```

#### 4. "addontemplate not found"

**原因**: 指定的模板不存在

**解决**:
```bash
# 先查询可用模板
curl -X GET "http://api-server/api/v1/addontemplates"
```

## 📖 相关资源

- [Kubernetes CRD 文档](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
- [Helm 文档](https://helm.sh/docs/)
- [认证与授权指南](./AUTH.md)

## 🤝 贡献

如有问题或建议，请联系开发团队。

