# 审计日志 API 文档

## 概述

审计日志用于记录平台上的所有写操作，包括用户的创建、修改、删除等行为。

---

## API 接口

### 查询审计日志列表

**接口**: `GET /api/v1/auditlogs`

**权限**: 仅管理员（system-admin）可访问

#### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| userId | string | 否 | 按用户ID筛选（精确匹配） |
| userName | string | 否 | 按用户名筛选（模糊匹配） |
| userType | string | 否 | 按用户类型筛选（逗号分隔支持多选：default,sso,apikey） |
| resourceType | string | 否 | 按资源类型筛选（逗号分隔支持多选：如 workloads,apikeys,deployments） |
| resourceName | string | 否 | 按资源名称筛选（模糊匹配） |
| httpMethod | string | 否 | 按HTTP方法筛选（逗号分隔支持多选：POST,PUT,PATCH,DELETE） |
| requestPath | string | 否 | 按请求路径筛选（模糊匹配） |
| responseStatus | int | 否 | 按响应状态码筛选 |
| startTime | string | 否 | 开始时间（RFC3339格式，如 2026-01-01T00:00:00Z） |
| endTime | string | 否 | 结束时间（RFC3339格式） |
| limit | int | 否 | 每页数量，默认 100，最大 100 |
| offset | int | 否 | 偏移量，默认 0 |
| sortBy | string | 否 | 排序字段，默认 createTime |
| order | string | 否 | 排序方向：asc / desc，默认 desc |

#### 响应结构

```json
{
  "totalCount": 100,
  "items": [
    {
      "id": 1,
      "userId": "a01e7b83f5661e327503f0eacbfef97d",
      "userName": "shuoshuo",
      "userType": "default",
      "clientIp": "10.176.17.167",
      "action": "approve deployment",
      "httpMethod": "POST",
      "requestPath": "/api/v1/cd/deployments/34/approve",
      "resourceType": "deployments",
      "resourceName": "34",
      "requestBody": "{\"approved\":true}",
      "responseStatus": 200,
      "latencyMs": 72,
      "traceId": "7b2d2cf552969247e747c55142b911a7",
      "createTime": "2026-01-17T14:59:45Z"
    }
  ]
}
```

#### 响应字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 | 审计日志ID |
| userId | string | 用户ID |
| userName | string | 用户名 |
| userType | string | 用户类型：default / sso / apikey |
| clientIp | string | 客户端IP |
| action | string | 操作描述（人可读），如 "create workload"、"delete apikey" |
| httpMethod | string | HTTP方法 |
| requestPath | string | 请求路径 |
| resourceType | string | 资源类型 |
| resourceName | string | 资源名称/ID |
| requestBody | string | 请求体（敏感信息已脱敏） |
| responseStatus | int | 响应状态码 |
| latencyMs | int64 | 响应延迟（毫秒） |
| traceId | string | 链路追踪ID |
| createTime | string | 创建时间（RFC3339格式） |

---

## Action 字段说明

`action` 字段提供人可读的操作描述：

| 操作 | action 值 |
|------|-----------|
| 创建资源 | create workload / create apikey / create workspace ... |
| 删除资源 | delete workload / delete apikey / delete workspace ... |
| 更新资源 | update workload / update user ... |
| 审批部署 | approve deployment |
| 回滚部署 | rollback deployment |
| 停止任务 | stop workload / stop opsjob |
| 克隆任务 | clone workload |
| 用户登录 | login |
| 用户登出 | logout |

---

## 示例请求

### 查询所有审计日志（默认返回最新100条）

```bash
GET /api/v1/auditlogs
```

### 按用户ID筛选

```bash
GET /api/v1/auditlogs?userId=a01e7b83f5661e327503f0eacbfef97d
```

### 按用户名筛选（模糊匹配）

```bash
GET /api/v1/auditlogs?userName=shuo
```

### 按用户类型筛选（逗号分隔支持多选）

```bash
# 单选
GET /api/v1/auditlogs?userType=default

# 多选：查看 default 和 sso 用户的操作
GET /api/v1/auditlogs?userType=default,sso
```

### 按资源类型筛选（逗号分隔支持多选）

```bash
# 单选
GET /api/v1/auditlogs?resourceType=workloads

# 多选：查看 workloads 和 apikeys 相关的操作
GET /api/v1/auditlogs?resourceType=workloads,apikeys
```

### 按HTTP方法筛选（逗号分隔支持多选）

```bash
# 单选
GET /api/v1/auditlogs?httpMethod=DELETE

# 多选：查看 POST 和 DELETE 操作
GET /api/v1/auditlogs?httpMethod=POST,DELETE
```

### 按时间范围筛选

```bash
GET /api/v1/auditlogs?startTime=2026-01-01T00:00:00Z&endTime=2026-01-31T23:59:59Z
```

### 分页查询

```bash
GET /api/v1/auditlogs?limit=50&offset=100
```

### 组合查询

```bash
GET /api/v1/auditlogs?userId=xxx&resourceType=apikeys&httpMethod=DELETE&limit=10
```

---

## 错误响应

| 状态码 | 说明 |
|--------|------|
| 401 | 未认证 |
| 403 | 无权限（非管理员） |
| 400 | 请求参数错误（如时间格式不正确） |
| 500 | 服务器内部错误 |

```json
{
  "errorCode": "Primus.00003",
  "errorMessage": "user not authorized to access audit logs"
}
```
