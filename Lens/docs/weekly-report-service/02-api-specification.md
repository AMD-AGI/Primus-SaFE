# 周报服务 API 接口规范

## 1. 基础信息

**Base URL**: `/api/v1`

**Content-Type**: `application/json`

**认证方式**: Bearer Token（可选，根据实际需求）

## 2. API 端点列表

### 2.1 生成周报

**端点**: `POST /weekly-reports`

**描述**: 手动触发周报生成任务

**请求体**:

```json
{
  "cluster": "x-flannel",
  "time_range_days": 7,
  "start_time": "2025-11-11T00:00:00Z",  // 可选，不指定则自动计算
  "end_time": "2025-11-17T23:59:59Z",    // 可选
  "utilization_threshold": 30,
  "min_gpu_count": 1,
  "top_n": 20,
  "send_email": true,                     // 是否发送邮件
  "recipients": [                          // 可选，覆盖默认收件人
    "user@example.com"
  ]
}
```

**响应** (202 Accepted):

```json
{
  "report_id": "rpt_20251123_x_flannel_001",
  "status": "generating",
  "message": "Report generation started",
  "created_at": "2025-11-23T19:53:39Z",
  "estimated_completion_time": "2025-11-23T19:55:00Z"
}
```

**状态码**:
- `202`: 任务已接受，正在生成
- `400`: 请求参数错误
- `500`: 服务器错误

---

### 2.2 查询周报列表

**端点**: `GET /weekly-reports`

**描述**: 获取周报历史记录列表

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| cluster | string | 否 | 集群名称过滤 |
| status | string | 否 | 状态过滤：generating, completed, failed |
| start_date | string | 否 | 开始日期（ISO 8601） |
| end_date | string | 否 | 结束日期（ISO 8601） |
| page | int | 否 | 页码，默认 1 |
| page_size | int | 否 | 每页数量，默认 20，最大 100 |
| sort | string | 否 | 排序字段，默认 created_at |
| order | string | 否 | 排序方向：asc, desc，默认 desc |

**示例请求**:

```
GET /api/v1/weekly-reports?cluster=x-flannel&status=completed&page=1&page_size=10
```

**响应** (200 OK):

```json
{
  "reports": [
    {
      "id": "rpt_20251123_x_flannel_001",
      "cluster": "x-flannel",
      "status": "completed",
      "created_at": "2025-11-23T19:53:39Z",
      "completed_at": "2025-11-23T19:55:12Z",
      "time_range": {
        "start": "2025-11-11T00:00:00Z",
        "end": "2025-11-17T23:59:59Z",
        "days": 7
      },
      "summary": {
        "avg_utilization": 58.9,
        "avg_allocation": 73.25,
        "low_util_users_count": 17,
        "wasted_gpu_days": 400
      },
      "formats_available": ["html", "pdf", "json"],
      "email_sent": true,
      "file_size_bytes": {
        "html": 125678,
        "pdf": 856234,
        "json": 45231
      }
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 10,
    "total_items": 45,
    "total_pages": 5
  }
}
```

---

### 2.3 获取单个周报详情

**端点**: `GET /weekly-reports/{id}`

**描述**: 获取特定周报的详细信息和内容

**路径参数**:
- `id`: 周报 ID

**响应** (200 OK):

```json
{
  "id": "rpt_20251123_x_flannel_001",
  "cluster": "x-flannel",
  "status": "completed",
  "created_at": "2025-11-23T19:53:39Z",
  "completed_at": "2025-11-23T19:55:12Z",
  "time_range": {
    "start": "2025-11-11T00:00:00Z",
    "end": "2025-11-17T23:59:59Z",
    "days": 7,
    "week_label": "2025-W46"
  },
  "parameters": {
    "utilization_threshold": 30,
    "min_gpu_count": 1,
    "top_n": 20
  },
  "summary": {
    "avg_utilization": 58.9,
    "max_utilization": 78.91,
    "min_utilization": 20.55,
    "utilization_trend": "increasing",
    "avg_allocation": 73.25,
    "max_allocation": 108.27,
    "min_allocation": 50.83,
    "allocation_trend": "decreasing",
    "low_util_users_count": 17,
    "wasted_gpu_days": 400,
    "namespace_count": 3
  },
  "report_content": {
    "markdown": "# Cluster Usage Report\n\n...",
    "sections": [
      "cluster_overview",
      "namespace_comparison",
      "low_utilization_users"
    ]
  },
  "chart_data": {
    "cluster_usage_trend": {
      "xAxis": ["2025-11-17 12:00", ...],
      "series": [
        {
          "name": "GPU Utilization",
          "type": "line",
          "data": [37.93, 38.38, ...]
        },
        {
          "name": "GPU Allocation Rate",
          "type": "line",
          "data": [84.73, 84.0, ...]
        }
      ]
    }
  },
  "metadata": {
    "crew": "cluster_report",
    "agents_used": [
      "Cluster Overview Analysis Expert",
      "Namespace Usage Analysis Expert",
      "Low Utilization User Detection Expert",
      "Cluster Usage Report Writing Expert"
    ],
    "generation_duration_seconds": 93.5
  },
  "formats_available": ["html", "pdf", "json"],
  "download_urls": {
    "html": "/api/v1/weekly-reports/rpt_20251123_x_flannel_001/download?format=html",
    "pdf": "/api/v1/weekly-reports/rpt_20251123_x_flannel_001/download?format=pdf",
    "json": "/api/v1/weekly-reports/rpt_20251123_x_flannel_001/download?format=json"
  },
  "email_history": [
    {
      "sent_at": "2025-11-23T19:55:30Z",
      "recipients": ["admin@example.com", "team@example.com"],
      "status": "delivered",
      "message_id": "msg_abc123"
    }
  ]
}
```

**状态码**:
- `200`: 成功
- `404`: 周报不存在

---

### 2.4 下载周报

**端点**: `GET /weekly-reports/{id}/download`

**描述**: 下载指定格式的周报文件

**路径参数**:
- `id`: 周报 ID

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| format | string | 是 | 格式：html, pdf, json |
| inline | bool | 否 | 是否内联显示（浏览器预览），默认 false（下载） |

**示例请求**:

```
GET /api/v1/weekly-reports/rpt_20251123_x_flannel_001/download?format=pdf
```

**响应**:

- **HTML**: `Content-Type: text/html; charset=utf-8`
- **PDF**: `Content-Type: application/pdf`
- **JSON**: `Content-Type: application/json`

响应头:
```
Content-Disposition: attachment; filename="cluster-report-x-flannel-2025-W46.pdf"
Content-Length: 856234
```

**状态码**:
- `200`: 成功
- `404`: 周报不存在或格式不可用
- `500`: 文件读取错误

---

### 2.5 重新发送周报邮件

**端点**: `POST /weekly-reports/{id}/resend`

**描述**: 重新发送已生成的周报邮件

**路径参数**:
- `id`: 周报 ID

**请求体**:

```json
{
  "recipients": [
    "new-user@example.com"
  ],
  "cc": [],
  "subject_override": "重发：GPU 集群周报 - x-flannel - 2025-W46"  // 可选
}
```

**响应** (200 OK):

```json
{
  "message": "Email sent successfully",
  "sent_at": "2025-11-23T20:10:00Z",
  "recipients_count": 1,
  "message_id": "msg_def456"
}
```

**状态码**:
- `200`: 发送成功
- `404`: 周报不存在
- `500`: 发送失败

---

### 2.6 获取配置

**端点**: `GET /weekly-reports/config`

**描述**: 获取当前周报服务配置

**响应** (200 OK):

```json
{
  "scheduler": {
    "enabled": true,
    "cron": "0 9 * * 1",
    "timezone": "Asia/Shanghai",
    "next_run": "2025-11-25T09:00:00+08:00"
  },
  "clusters": [
    {
      "name": "x-flannel",
      "enabled": true,
      "time_range_days": 7,
      "utilization_threshold": 30,
      "min_gpu_count": 1,
      "top_n": 20
    }
  ],
  "email": {
    "enabled": true,
    "from": "GPU Cluster Reports <reports@example.com>",
    "recipients": {
      "to": ["admin@example.com", "team@example.com"],
      "cc": ["manager@example.com"]
    },
    "attach_pdf": true
  },
  "render": {
    "brand": {
      "company_name": "AMD AGI",
      "primary_color": "#ED1C24"
    },
    "chart_library": "echarts"
  },
  "storage": {
    "retention_days": 90
  }
}
```

---

### 2.7 更新配置

**端点**: `PUT /weekly-reports/config`

**描述**: 更新周报服务配置（需要管理员权限）

**请求体**:

```json
{
  "scheduler": {
    "enabled": true,
    "cron": "0 9 * * 1"
  },
  "clusters": [
    {
      "name": "x-flannel",
      "enabled": true,
      "time_range_days": 7
    }
  ],
  "email": {
    "enabled": true,
    "recipients": {
      "to": ["new-admin@example.com"]
    }
  }
}
```

**响应** (200 OK):

```json
{
  "message": "Configuration updated successfully",
  "updated_at": "2025-11-23T20:15:00Z",
  "restart_required": false
}
```

**状态码**:
- `200`: 更新成功
- `400`: 配置验证失败
- `403`: 权限不足

---

### 2.8 删除周报

**端点**: `DELETE /weekly-reports/{id}`

**描述**: 删除指定周报（包括文件）

**路径参数**:
- `id`: 周报 ID

**响应** (204 No Content):

```
(空响应体)
```

**状态码**:
- `204`: 删除成功
- `404`: 周报不存在
- `403`: 权限不足

---

### 2.9 获取任务状态

**端点**: `GET /weekly-reports/{id}/status`

**描述**: 查询周报生成任务的实时状态（用于长轮询）

**路径参数**:
- `id`: 周报 ID

**响应** (200 OK):

```json
{
  "id": "rpt_20251123_x_flannel_001",
  "status": "generating",
  "progress": 65,  // 0-100
  "current_step": "Rendering PDF",
  "started_at": "2025-11-23T19:53:39Z",
  "estimated_completion": "2025-11-23T19:55:00Z",
  "error": null
}
```

状态流转：
- `queued` → `generating` → `completed`
- `queued` → `generating` → `failed`

---

### 2.10 批量生成周报

**端点**: `POST /weekly-reports/batch`

**描述**: 为多个集群批量生成周报

**请求体**:

```json
{
  "clusters": ["x-flannel", "y-cluster", "z-cluster"],
  "time_range_days": 7,
  "send_email": true,
  "concurrent": true  // 是否并发生成
}
```

**响应** (202 Accepted):

```json
{
  "batch_id": "batch_20251123_001",
  "reports": [
    {
      "cluster": "x-flannel",
      "report_id": "rpt_20251123_x_flannel_001",
      "status": "queued"
    },
    {
      "cluster": "y-cluster",
      "report_id": "rpt_20251123_y_cluster_001",
      "status": "queued"
    },
    {
      "cluster": "z-cluster",
      "report_id": "rpt_20251123_z_cluster_001",
      "status": "queued"
    }
  ],
  "message": "Batch generation started"
}
```

---

## 3. WebSocket API（可选高级功能）

**端点**: `WS /weekly-reports/{id}/stream`

**描述**: 实时推送周报生成进度

**消息格式**:

```json
{
  "type": "progress",
  "data": {
    "progress": 45,
    "step": "Fetching data from Conductor API",
    "timestamp": "2025-11-23T19:54:00Z"
  }
}
```

```json
{
  "type": "completed",
  "data": {
    "report_id": "rpt_20251123_x_flannel_001",
    "download_urls": { ... }
  }
}
```

```json
{
  "type": "error",
  "data": {
    "error": "Failed to connect to Conductor API"
  }
}
```

---

## 4. 错误响应格式

所有错误响应遵循统一格式：

```json
{
  "error": {
    "code": "INVALID_PARAMETER",
    "message": "Invalid time_range_days: must be between 1 and 90",
    "details": {
      "field": "time_range_days",
      "value": 365
    }
  },
  "request_id": "req_abc123",
  "timestamp": "2025-11-23T19:53:39Z"
}
```

**常见错误码**:

| 错误码 | HTTP 状态 | 说明 |
|--------|-----------|------|
| INVALID_PARAMETER | 400 | 请求参数无效 |
| REPORT_NOT_FOUND | 404 | 周报不存在 |
| CONDUCTOR_API_ERROR | 502 | Conductor API 调用失败 |
| GENERATION_FAILED | 500 | 周报生成失败 |
| EMAIL_SEND_FAILED | 500 | 邮件发送失败 |
| UNAUTHORIZED | 401 | 未授权 |
| FORBIDDEN | 403 | 权限不足 |
| RATE_LIMIT_EXCEEDED | 429 | 请求频率超限 |

---

## 5. 认证和授权（可选）

如需要认证，使用 Bearer Token：

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

权限级别：
- **读取**: 查询周报、下载
- **生成**: 触发周报生成
- **管理**: 更新配置、删除周报

---

## 6. 速率限制

- 查询接口：100 请求/分钟
- 生成接口：10 请求/分钟
- 下载接口：50 请求/分钟

超限响应：
```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Too many requests, please try again later",
    "retry_after": 60
  }
}
```

响应头：
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1700760000
```

---

## 7. API 版本控制

当前版本：`v1`

未来新版本通过 URL 路径区分：
- `/api/v1/weekly-reports`
- `/api/v2/weekly-reports`

响应头中包含 API 版本信息：
```
X-API-Version: 1.0.0
```

