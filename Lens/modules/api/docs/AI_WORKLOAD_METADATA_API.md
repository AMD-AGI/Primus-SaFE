# AI Workload Metadata API Documentation

## 概述

AI Workload Metadata API 提供了管理 AI 工作负载元数据的接口，并集成了检测冲突日志（Detection Conflict Log）的查询功能。这些 API 允许您查看、创建、更新和删除工作负载元数据，同时可以查看框架检测过程中产生的冲突信息。

## API 端点

### 1. 获取 AI 工作负载元数据（带冲突信息）

**端点**: `GET /v1/ai-workload-metadata/:workload_uid`

**描述**: 根据 workload UID 获取 AI 工作负载元数据，同时返回相关的检测冲突信息。

**路径参数**:
- `workload_uid` (string, required): 工作负载的唯一标识符

**查询参数**:
- `cluster` (string, optional): 集群名称

**响应示例**:
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "workload_uid": "abc-123-def-456",
    "type": "training",
    "framework": "primus",
    "metadata": {
      "framework_detection": {
        "framework": "primus",
        "confidence": 0.95,
        "status": "confirmed"
      },
      "workload_signature": {
        "image": "registry.example.com/primus:v1.2.3"
      }
    },
    "image_prefix": "registry.example.com/primus",
    "created_at": "2025-01-15T10:30:00Z",
    "has_conflicts": true,
    "unresolved_conflicts": 1,
    "conflict_summary": [
      {
        "id": 1,
        "source_1": "wandb",
        "source_2": "image",
        "framework_1": "primus",
        "framework_2": "megatron",
        "confidence_1": 0.95,
        "confidence_2": 0.80,
        "resolution_strategy": "highest_confidence",
        "resolved_framework": "primus",
        "resolved_confidence": 0.95,
        "created_at": "2025-01-15T10:25:00Z",
        "resolved_at": "2025-01-15T10:26:00Z"
      }
    ]
  }
}
```

---

### 2. 列出 AI 工作负载元数据

**端点**: `GET /v1/ai-workload-metadata`

**描述**: 列出所有 AI 工作负载元数据，支持按框架类型和冲突状态过滤。

**查询参数**:
- `cluster` (string, optional): 集群名称
- `framework` (string, optional): 按框架名称过滤（如 "primus", "megatron", "deepspeed"）
- `type` (string, optional): 按类型过滤（如 "training", "inference"）
- `has_conflict` (boolean, optional): 按冲突状态过滤
  - `true`: 仅显示有冲突的工作负载
  - `false`: 仅显示无冲突的工作负载

**响应示例**:
```json
{
  "code": 200,
  "data": {
    "data": [
      {
        "id": 1,
        "workload_uid": "abc-123",
        "type": "training",
        "framework": "primus",
        "has_conflicts": true,
        "unresolved_conflicts": 2,
        "conflict_summary": [...]
      },
      {
        "id": 2,
        "workload_uid": "def-456",
        "type": "training",
        "framework": "megatron",
        "has_conflicts": false,
        "unresolved_conflicts": 0
      }
    ],
    "total": 2
  }
}
```

---

### 3. 标注工作负载框架（用户标注）

**端点**: `POST /v1/ai-workload-metadata/:workload_uid/annotate`

**描述**: 让用户标注某个工作负载的框架信息。此标注会按照 framework detection 的标准格式存储到 metadata 中。

**路径参数**:
- `workload_uid` (string, required): 工作负载的唯一标识符

**查询参数**:
- `cluster` (string, optional): 集群名称

**请求体**:
```json
{
  "framework": "primus",
  "type": "training",
  "confidence": 1.0,
  "evidence": {
    "reason": "确认为 Primus 训练任务",
    "user": "admin@example.com",
    "notes": "经过日志分析确认"
  }
}
```

**字段说明**:
- `framework` (string, required): 框架名称（primus, megatron, deepspeed, pytorch, tensorflow, jax 等）
- `type` (string, optional): 任务类型（training, inference），默认为 "training"
- `confidence` (float, optional): 置信度 [0.0-1.0]，默认为 1.0（用户标注默认完全确定）
- `evidence` (object, optional): 标注证据/备注，可以包含任意信息

**响应示例（新建）**:
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "workload_uid": "abc-123-def-456",
    "type": "training",
    "framework": "primus",
    "metadata": {
      "framework_detection": {
        "framework": "primus",
        "type": "training",
        "confidence": 1.0,
        "status": "confirmed",
        "sources": [
          {
            "source": "user",
            "framework": "primus",
            "type": "training",
            "confidence": 1.0,
            "detected_at": "2025-01-15T10:30:00Z",
            "evidence": {
              "method": "user_annotation",
              "reason": "确认为 Primus 训练任务",
              "user": "admin@example.com",
              "annotated_at": "2025-01-15T10:30:00Z"
            }
          }
        ],
        "conflicts": [],
        "version": "1.0",
        "updated_at": "2025-01-15T10:30:00Z"
      }
    },
    "created_at": "2025-01-15T10:30:00Z"
  }
}
```

**响应示例（更新）**:
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "workload_uid": "abc-123-def-456",
    "type": "training",
    "framework": "primus",
    "metadata": {
      "framework_detection": {
        "framework": "primus",
        "type": "training",
        "confidence": 1.0,
        "status": "confirmed",
        "sources": [
          {
            "source": "wandb",
            "framework": "megatron",
            "confidence": 0.80,
            "detected_at": "2025-01-15T10:25:00Z"
          },
          {
            "source": "user",
            "framework": "primus",
            "type": "training",
            "confidence": 1.0,
            "detected_at": "2025-01-15T10:30:00Z",
            "evidence": {
              "method": "user_annotation",
              "reason": "确认为 Primus 训练任务",
              "user": "admin@example.com",
              "annotated_at": "2025-01-15T10:30:00Z"
            }
          }
        ],
        "conflicts": [],
        "version": "1.0",
        "updated_at": "2025-01-15T10:30:00Z"
      }
    },
    "updated_at": "2025-01-15T10:30:00Z"
  }
}
```

**说明**:
- 如果 workload 不存在 metadata，会自动创建新记录
- 如果已存在 metadata，会将用户标注作为新的 source 添加到 `framework_detection.sources` 中
- 如果已有用户标注，会更新现有的用户标注
- 用户标注的 source 固定为 "user"
- 标注后的 status 自动设置为 "confirmed"

---

### 4. 更新 AI 工作负载元数据

**端点**: `PUT /v1/ai-workload-metadata/:workload_uid`

**描述**: 更新现有的 AI 工作负载元数据。

**路径参数**:
- `workload_uid` (string, required): 工作负载的唯一标识符

**查询参数**:
- `cluster` (string, optional): 集群名称

**请求体**:
```json
{
  "workload_uid": "abc-123-def-456",
  "type": "training",
  "framework": "primus",
  "metadata": {
    "framework_detection": {
      "framework": "primus",
      "confidence": 0.98,
      "status": "verified"
    }
  }
}
```

---

### 5. 删除 AI 工作负载元数据

**端点**: `DELETE /v1/ai-workload-metadata/:workload_uid`

**描述**: 删除指定的 AI 工作负载元数据。

**路径参数**:
- `workload_uid` (string, required): 工作负载的唯一标识符

**查询参数**:
- `cluster` (string, optional): 集群名称

**响应示例**:
```json
{
  "code": 200,
  "data": {
    "message": "metadata deleted successfully"
  }
}
```

---

### 6. 获取工作负载的检测冲突日志

**端点**: `GET /v1/ai-workload-metadata/:workload_uid/conflicts`

**描述**: 获取特定工作负载的所有检测冲突日志，包含详细的证据信息。

**路径参数**:
- `workload_uid` (string, required): 工作负载的唯一标识符

**查询参数**:
- `cluster` (string, optional): 集群名称
- `page` (int, optional): 页码，默认为 1
- `page_size` (int, optional): 每页大小，默认为 20，最大 100

**响应示例**:
```json
{
  "code": 200,
  "data": {
    "data": [
      {
        "id": 1,
        "workload_uid": "abc-123-def-456",
        "source_1": "wandb",
        "source_2": "image",
        "framework_1": "primus",
        "framework_2": "megatron",
        "confidence_1": 0.95,
        "confidence_2": 0.80,
        "resolution_strategy": "highest_confidence",
        "resolved_framework": "primus",
        "resolved_confidence": 0.95,
        "evidence_1": {
          "method": "import_detection",
          "framework_layer": "wrapper",
          "wrapper_framework": "primus",
          "base_framework": "megatron"
        },
        "evidence_2": {
          "image_name": "megatron-lm:latest",
          "pattern_matched": "megatron"
        },
        "created_at": "2025-01-15T10:25:00Z",
        "resolved_at": "2025-01-15T10:26:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20
  }
}
```

---

### 7. 列出所有检测冲突

**端点**: `GET /v1/detection-conflicts`

**描述**: 列出所有工作负载的最近检测冲突记录。

**查询参数**:
- `cluster` (string, optional): 集群名称
- `page` (int, optional): 页码，默认为 1
- `page_size` (int, optional): 每页大小，默认为 20，最大 100

**响应示例**:
```json
{
  "code": 200,
  "data": {
    "data": [
      {
        "id": 1,
        "workload_uid": "abc-123",
        "source_1": "wandb",
        "source_2": "image",
        "framework_1": "primus",
        "framework_2": "megatron",
        "confidence_1": 0.95,
        "confidence_2": 0.80,
        "resolution_strategy": "highest_confidence",
        "resolved_framework": "primus",
        "resolved_confidence": 0.95,
        "created_at": "2025-01-15T10:25:00Z"
      },
      {
        "id": 2,
        "workload_uid": "def-456",
        "source_1": "wandb",
        "source_2": "component",
        "framework_1": "deepspeed",
        "framework_2": "pytorch",
        "confidence_1": 0.85,
        "confidence_2": 0.70,
        "resolution_strategy": "",
        "created_at": "2025-01-15T11:00:00Z"
      }
    ],
    "total": 2,
    "page": 1,
    "page_size": 20
  }
}
```

---

## 数据模型

### AiWorkloadMetadata

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int32 | 自增主键 |
| `workload_uid` | string | 工作负载唯一标识符 |
| `type` | string | 工作负载类型（如 "training", "inference"） |
| `framework` | string | 检测到的框架名称 |
| `metadata` | object | 元数据 JSON 对象 |
| `image_prefix` | string | 镜像仓库地址（不含标签） |
| `created_at` | timestamp | 创建时间 |
| `has_conflicts` | boolean | 是否存在冲突（仅响应中） |
| `unresolved_conflicts` | int | 未解决冲突数量（仅响应中） |
| `conflict_summary` | array | 冲突摘要（仅响应中） |

### DetectionConflictLog

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 自增主键 |
| `workload_uid` | string | 工作负载唯一标识符 |
| `source_1` | string | 第一个检测源（如 "wandb", "image", "component"） |
| `source_2` | string | 第二个检测源 |
| `framework_1` | string | 源1检测到的框架 |
| `framework_2` | string | 源2检测到的框架 |
| `confidence_1` | float64 | 源1的置信度（0.00-1.00） |
| `confidence_2` | float64 | 源2的置信度（0.00-1.00） |
| `resolution_strategy` | string | 冲突解决策略（如 "highest_confidence", "priority_based"） |
| `resolved_framework` | string | 解决后的框架 |
| `resolved_confidence` | float64 | 解决后的置信度 |
| `evidence_1` | object | 源1的证据详情 |
| `evidence_2` | object | 源2的证据详情 |
| `created_at` | timestamp | 冲突检测时间 |
| `resolved_at` | timestamp | 冲突解决时间 |

---

## 使用场景

### 场景 1：用户标注工作负载框架

```bash
# 标注某个工作负载的框架信息
curl -X POST "http://localhost:8080/v1/ai-workload-metadata/abc-123-def-456/annotate" \
  -H "Content-Type: application/json" \
  -d '{
    "framework": "primus",
    "type": "training",
    "confidence": 1.0,
    "evidence": {
      "reason": "经过日志分析确认为 Primus 训练任务",
      "user": "admin@example.com"
    }
  }'

# 更正错误的框架检测
curl -X POST "http://localhost:8080/v1/ai-workload-metadata/def-456-ghi-789/annotate" \
  -H "Content-Type: application/json" \
  -d '{
    "framework": "megatron",
    "type": "training",
    "evidence": {
      "reason": "系统检测为 deepspeed，但实际使用 Megatron-LM",
      "correction": true
    }
  }'
```

### 场景 2：查看有冲突的工作负载

```bash
# 列出所有有冲突的工作负载
curl "http://localhost:8080/v1/ai-workload-metadata?has_conflict=true"
```

### 场景 3：查看特定工作负载的冲突详情

```bash
# 获取工作负载元数据和冲突摘要
curl "http://localhost:8080/v1/ai-workload-metadata/abc-123-def-456"

# 获取详细的冲突日志（包含证据）
curl "http://localhost:8080/v1/ai-workload-metadata/abc-123-def-456/conflicts"
```

### 场景 4：按框架类型过滤

```bash
# 列出所有使用 Primus 框架的工作负载
curl "http://localhost:8080/v1/ai-workload-metadata?framework=primus"

# 列出所有使用 Megatron 且有冲突的工作负载
curl "http://localhost:8080/v1/ai-workload-metadata?framework=megatron&has_conflict=true"
```

### 场景 5：查看系统级别的冲突概况

```bash
# 查看最近的所有冲突
curl "http://localhost:8080/v1/detection-conflicts?page=1&page_size=50"
```

---

## 集成说明

这些 API 与以下组件集成：

1. **WandB Exporter**: 通过 WandB 检测上报框架信息
2. **Framework Detection Manager**: 框架检测管理器，处理多源检测和冲突解决
3. **Detection Conflict Log**: 记录所有检测冲突，用于调试和优化检测逻辑
4. **User Annotation**: 用户标注接口，允许手动标注和更正框架信息

### 框架检测流程

1. **自动检测**: 系统通过多个数据源（WandB、镜像、组件等）自动检测框架
2. **冲突记录**: 当不同数据源检测到不同框架时，自动记录冲突
3. **用户标注**: 用户可以通过标注接口手动标注或更正框架信息
4. **数据融合**: 用户标注会作为新的检测源融合到现有检测结果中

### 标注接口的特点

- **标准格式**: 标注数据按照 `framework_detection` 标准格式存储
- **source 标识**: 用户标注的 source 固定为 "user"
- **高置信度**: 用户标注默认置信度为 1.0
- **可更新**: 重复标注会更新现有的用户标注记录
- **evidence 存储**: 可以附加标注理由、用户信息等证据

### 使用场景

通过这些 API，您可以：

- ✅ **查看检测结果**: 查看哪些工作负载存在框架检测冲突
- ✅ **分析冲突原因**: 查看详细的冲突证据和来源
- ✅ **手动标注**: 为未检测或检测错误的工作负载标注正确的框架
- ✅ **更正错误**: 更正自动检测的错误结果
- ✅ **评估准确性**: 评估框架检测的准确性
- ✅ **调优策略**: 调优检测策略和优先级

---

## 错误处理

所有 API 遵循统一的错误响应格式：

```json
{
  "code": 400,
  "message": "invalid request body",
  "data": null
}
```

常见错误码：
- `400`: 请求参数错误
- `404`: 资源不存在
- `500`: 服务器内部错误

