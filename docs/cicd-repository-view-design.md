# CICD Repository View 前端设计文档

## 概述

在 SaFE 前端的 CICD 页面（`/cicd`）新增 "Repository View" Tab，展示 GitHub 仓库维度的 CI/CD 观测数据。数据来自 SaFE apiserver 的 `/api/v1/github-workflow/*` API。

## 页面结构

```
/cicd
├── [Tab: Workloads]     ← 现有的 CICD workload 列表
└── [Tab: Repository View] ← 新增
    ├── 仓库列表页
    │   ├── 统计卡片 (总仓库数 / 总运行数 / 运行中 / 采集配置数)
    │   ├── 搜索框
    │   └── 仓库卡片列表
    │       ├── owner/repo 名称 + GitHub 链接
    │       ├── 最近运行状态 (running / completed / failed)
    │       ├── 是否有采集配置
    │       └── 点击 → 仓库详情页
    │
    └── 仓库详情页 (/cicd/repo/:owner/:repo)
        ├── Header: owner/repo + GitHub 链接 + 返回按钮
        ├── [Tab: Runs]       ← 该仓库的 workflow run 列表
        │   ├── 过滤: status / workflow_name
        │   └── 表格: run_id, workflow, workload, status, started_at
        │       └── 点击展开 → Jobs + Steps
        │
        ├── [Tab: Metrics]    ← 该仓库的采集指标 (如果有 config)
        │   ├── Config 选择器 (如果一个仓库有多个 config)
        │   ├── Fields 信息面板 (dimensions / metrics, 来自 /fields API)
        │   ├── 图表区域
        │   │   ├── 根据 display_settings 自动配置:
        │   │   │   - chart_type: line / bar
        │   │   │   - group_by: 默认分组维度
        │   │   │   - group_mode: dimension / metric
        │   │   ├── 用户可切换 Y 轴指标 / 分组维度
        │   │   └── ECharts 渲染
        │   └── 原始数据表格 (show_raw_data_by_default 控制默认展开)
        │
        └── [Tab: Settings]   ← 采集配置管理
            ├── 当前 config 详情
            ├── 编辑 file_patterns / workflow_patterns / branch_patterns
            └── 编辑 display_settings
```

## API 依赖

### 现有 API (已实现)

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/github-workflow/collection-configs` | 列表页: 获取所有配置 |
| GET | `/github-workflow/collection-configs/:id/fields` | 详情页-Metrics: 获取字段列表 + display_settings |
| GET | `/github-workflow/collection-configs/:id/metrics` | 详情页-Metrics: 获取指标数据 |
| POST | `/github-workflow/collection-configs` | 详情页-Settings: 创建配置 |
| DELETE | `/github-workflow/collection-configs/:id` | 详情页-Settings: 删除配置 |
| GET | `/github-workflow/runs` | 详情页-Runs: 按仓库过滤 runs |
| GET | `/github-workflow/runs/:id` | 详情页-Runs: run 详情 |
| GET | `/github-workflow/runs/:id/jobs` | 详情页-Runs: jobs + steps |
| GET | `/github-workflow/stats` | 列表页: 汇总统计 |

### 需要新增的 API

| 方法 | 路径 | 用途 | 说明 |
|------|------|------|------|
| GET | `/github-workflow/repositories` | 列表页: 仓库列表 | 聚合 runs + configs 按 owner/repo 分组 |
| GET | `/github-workflow/repositories/:owner/:repo` | 详情页: 仓库汇总 | 单个仓库的统计信息 |
| PUT | `/github-workflow/collection-configs/:id` | 详情页-Settings: 更新配置 | 修改 file_patterns / display_settings |

## 需要新增的 API 详细设计

### GET /github-workflow/repositories

按 owner/repo 聚合 runs 和 configs，返回仓库列表。

**Request**: 无参数 (或 `search` 搜索)

**Response**:
```json
{
  "repositories": [
    {
      "github_owner": "AMD-AGI",
      "github_repo": "Primus-Turbo",
      "total_runs": 3,
      "running_runs": 1,
      "completed_runs": 2,
      "failed_runs": 0,
      "latest_run_at": "2026-03-19T10:17:04Z",
      "latest_workflow": "unittest-pytorch-gfx942",
      "config_count": 1,
      "config_ids": [1]
    }
  ],
  "count": 3
}
```

**SQL**:
```sql
SELECT github_owner, github_repo,
       count(*) as total_runs,
       count(*) FILTER (WHERE status = 'running') as running_runs,
       count(*) FILTER (WHERE status = 'completed') as completed_runs,
       count(*) FILTER (WHERE conclusion = 'failure') as failed_runs,
       max(started_at) as latest_run_at
FROM github_workflow_runs
GROUP BY github_owner, github_repo
ORDER BY max(started_at) DESC NULLS LAST
```

### GET /github-workflow/repositories/:owner/:repo

单个仓库的汇总信息。

**Response**:
```json
{
  "github_owner": "AMD-AGI",
  "github_repo": "Primus-Turbo",
  "total_runs": 3,
  "running_runs": 1,
  "completed_runs": 2,
  "failed_runs": 0,
  "workflows": ["unittest-pytorch-gfx942", "unittest-jax-gfx942"],
  "configs": [
    {
      "id": 1,
      "name": "Primus-Turbo MI325 Benchmark",
      "display_settings": {...},
      "metrics_count": 9434
    }
  ]
}
```

### PUT /github-workflow/collection-configs/:id

更新采集配置。

**Request**:
```json
{
  "name": "Updated name",
  "file_patterns": ["**/results/*.csv"],
  "workflow_patterns": ["benchmark.yml"],
  "display_settings": {
    "default_chart_type": "line",
    "default_chart_group_by": "Op"
  }
}
```

## 图表渲染逻辑

### 数据获取
```
1. GET /collection-configs/:id/fields → 获取 fields + display_settings
2. GET /collection-configs/:id/metrics?limit=5000 → 获取 row_data
```

### 根据 display_settings 渲染
```
display_settings.default_chart_group_mode:
  - "dimension": 按 display_settings.default_chart_group_by 分组
    例: group_by="Op" → X轴: date, Y轴: value, 每条线: Op 的不同值
  - "metric": 按不同 metric 字段分组
    例: Y轴上多条线: Tokens/s/GPU, TFLOP/s/GPU, Step Time (s)
  - "none" 或空: 单条线

display_settings.default_chart_type:
  - "line": 折线图
  - "bar": 柱状图

display_settings.show_raw_data_by_default:
  - true: 默认展开原始数据表格
  - false: 默认折叠
```

### ECharts 配置生成
```javascript
// group_mode = "dimension", group_by = "Op"
const groups = [...new Set(metrics.map(m => m.row_data["Op"]))]
const series = groups.map(g => ({
  name: g,
  type: chartType,
  data: metrics
    .filter(m => m.row_data["Op"] === g)
    .map(m => [m.row_data["date"] || m.created_at, m.row_data["value"]])
}))
```
