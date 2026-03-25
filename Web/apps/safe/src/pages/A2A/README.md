# A2A Protocol UI 开发指南

## 分支

```
feature/chenyi/a2a
```

## 概述

在 SaFE Web 的左侧导航栏 **AI Agent** 分组下新增 **A2A Protocol** 页面，内部用 Tab 切换三个视图：Dashboard、Agent Registry、Invocations。

数据全部从 SaFE API 读取，不硬编码 Agent 地址。

## 页面位置

```
AI Agent
  ├── ...已有的...
  └── A2A Protocol     ← 新增入口
        ├── Tab: Dashboard        总览
        ├── Tab: Agent Registry   Agent 列表
        └── Tab: Invocations      调用历史
```

## API 接口

所有接口需要认证，Header 带 Cookie Token 即可（和其他页面一样）。

### 1. 获取 Agent 列表

```
GET /api/v1/a2a/services?status=active

Response:
{
  "data": [
    {
      "id": 1,
      "serviceName": "ai-me",
      "displayName": "ai-me",
      "description": "",
      "endpoint": "http://ai-me-service.primus-lens.svc:8000",
      "a2aPathPrefix": "/a2a",
      "a2aAgentCard": "{...完整的 Agent Card JSON...}",
      "a2aSkills": "[...Skills 数组...]",
      "a2aHealth": "healthy",
      "a2aLastSeen": "2026-03-17T15:05:19Z",
      "k8sNamespace": "primus-lens",
      "k8sService": "ai-me-service",
      "discoverySource": "k8s-scanner",
      "status": "active",
      "createdAt": "2026-03-17T14:44:40Z",
      "updatedAt": "2026-03-17T15:05:19Z"
    }
  ],
  "total": 1
}
```

### 2. 获取单个 Agent 详情

```
GET /api/v1/a2a/services/:serviceName
```

### 3. 手动注册 Agent

```
POST /api/v1/a2a/services
Content-Type: application/json

{
  "serviceName": "my-agent",
  "displayName": "My Agent",
  "endpoint": "http://my-agent:8080",
  "a2aPathPrefix": "/a2a",
  "description": "A custom agent"
}
```

### 4. 删除 Agent

```
DELETE /api/v1/a2a/services/:serviceName
```

### 5. 获取调用日志

```
GET /api/v1/a2a/call-logs?limit=50&offset=0&caller=xxx&target=xxx

Response:
{
  "data": [
    {
      "id": 1,
      "traceId": "gw-1773747788705123123",
      "callerServiceName": "qa-bot",
      "callerUserId": "697f4f...",
      "targetServiceName": "ai-me",
      "skillId": "semantic_analysis",
      "status": "success",
      "latencyMs": 230,
      "requestSizeBytes": 101,
      "responseSizeBytes": 1650,
      "errorMessage": "",
      "createdAt": "2026-03-17T15:27:44Z"
    }
  ],
  "total": 2
}
```

### 6. 获取拓扑数据

```
GET /api/v1/a2a/topology

Response:
{
  "nodes": [
    { "serviceName": "ai-me", "displayName": "ai-me", "a2aHealth": "healthy", ... }
  ],
  "edges": [
    { "caller": "qa-bot", "target": "ai-me", "count": 5 }
  ]
}
```

## 三个 Tab 的内容

### Tab 1: Dashboard（总览）

参考：https://tw325.primus-safe.amd.com/a2a-dashboard/dashboard

**顶部 4 个指标卡片：**
- Calls Today：从 call-logs 按今天日期过滤计数
- Success Rate：call-logs 中 status=success 的比例
- Avg Latency：call-logs 中 latencyMs 的平均值
- Active Agents：services 中 status=active 的数量

**Agent Topology：**
- 从 `/api/v1/a2a/topology` 获取 nodes 和 edges
- 用 D3.js 或 ECharts graph 画拓扑图
- 节点颜色：healthy=绿色，unhealthy=红色，unknown=灰色
- 边上显示调用次数

**Call Volume (24h)：**
- 从 call-logs 按小时分组，画折线图
- 两条线：Success / Failure

**Skill Usage：**
- 从 call-logs 按 skillId 分组计数，画柱状图

**Live Invocations：**
- 最近 10 条 call-logs，表格展示
- 列：Time, Caller, Target, Skill, Latency, Status

### Tab 2: Agent Registry（Agent 列表）

参考：https://tw325.primus-safe.amd.com/a2a-dashboard/agents

**每个 Agent 一个卡片：**
- 名称 + 版本（从 a2aAgentCard 解析）
- 描述
- Skills 数量 + 健康状态
- Provider（从 a2aAgentCard 解析）
- Skills 标签列表
- discoverySource 标记（k8s-scanner / manual）

**操作按钮：**
- Refresh：重新拉取列表
- Register：弹窗手动注册新 Agent
- Delete：删除 Agent（确认弹窗）

### Tab 3: Invocations（调用历史）

参考：https://tw325.primus-safe.amd.com/a2a-dashboard/invocations

**表格：**
- 列：Time, Caller, Target, Skill, Latency, Status, Error
- 支持按 Caller / Target 筛选
- 分页

## 文件结构

```
Web/apps/safe/src/pages/A2A/
├── index.vue                    # 主页面，包含 Tab 切换
├── components/
│   ├── DashboardTab.vue         # Dashboard 总览 Tab
│   ├── AgentRegistryTab.vue     # Agent 列表 Tab
│   ├── InvocationsTab.vue       # 调用历史 Tab
│   ├── StatsCards.vue           # 4 个指标卡片
│   ├── TopologyGraph.vue        # 拓扑图
│   ├── CallVolumeChart.vue      # 调用量折线图
│   ├── SkillUsageChart.vue      # 技能使用柱状图
│   ├── AgentCard.vue            # 单个 Agent 卡片
│   └── RegisterDialog.vue       # 手动注册弹窗
```

## 路由配置

在 `Web/apps/safe/src/router/index.ts` 中，在 AI Agent 分组下新增：

```ts
{
  path: '/a2a',
  name: 'A2AProtocol',
  component: () => import('@/pages/A2A/index.vue'),
  meta: { title: 'A2A Protocol', icon: 'Connection' }
}
```

## 注意事项

1. `a2aAgentCard` 和 `a2aSkills` 字段是 JSON 字符串，需要 `JSON.parse()` 后使用
2. 调用日志的 `callerServiceName` 可能是 `"gateway"`（旧数据）或实际 Agent 名（新数据，通过 `X-Caller-Agent` header 传入）
3. 拓扑图的 edges 来自 call-logs 的聚合，count 表示调用次数
4. 所有时间字段是 UTC，前端需要转换为本地时间
5. 技术栈跟随项目：Vue 3 + Element Plus + ECharts
