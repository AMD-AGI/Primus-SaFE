# CICD Repository View 前端设计文档

## 概述

在 SaFE 前端的 CICD 页面（`/cicd`）新增 "Repository View" Tab，展示 GitHub 仓库维度的 CI/CD 观测数据。数据来自 SaFE apiserver 的 `/api/v1/github-workflow/*` API。

## 页面结构

```
/cicd
├── [Tab: Workloads]       ← 现有的 CICD workload 列表
└── [Tab: Repository View] ← 新增
    ├── 仓库列表页
    └── 仓库详情页 (/cicd/repo/:owner/:repo)
        ├── [Tab: Runs]
        ├── [Tab: Metrics]
        └── [Tab: Settings]
```

---

## 页面 1：仓库列表

### 样例图

```
┌─────────────────────────────────────────────────────────────────────┐
│  CICD                                                               │
│  ┌──────────────┬───────────────────┐                               │
│  │  Workloads   │  Repository View  │                               │
│  └──────────────┴───────────────────┘                               │
│                                                                     │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐   │
│  │ 📦 3       │  │ ▶ 0        │  │ ✅ 7       │  │ ⚙ 3        │   │
│  │ Repos      │  │ Running    │  │ Completed  │  │ Configs    │   │
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘   │
│                                                                     │
│  🔍 [Search by repository...                              ]        │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ 🔗 AMD-AGI / Primus              ↗ GitHub                  │   │
│  │    3 runs  │  0 running  │  3 completed  │  0 failed        │   │
│  │    📊 2 collection configs                                  │   │
│  │    Last run: 2 hours ago  │  Workflows: run-unittest-jax    │   │
│  ├─────────────────────────────────────────────────────────────┤   │
│  │ 🔗 AMD-AGI / Primus-Turbo        ↗ GitHub                  │   │
│  │    3 runs  │  0 running  │  3 completed  │  0 failed        │   │
│  │    📊 1 collection config (Primus-Turbo MI325 Benchmark)    │   │
│  │    Last run: 4 hours ago  │  Workflows: unittest-pytorch... │   │
│  ├─────────────────────────────────────────────────────────────┤   │
│  │ 🔗 ROCm / unified-training-dockers  ↗ GitHub               │   │
│  │    1 runs  │  0 running  │  1 completed  │  0 failed        │   │
│  │    ⚠️ No collection config                                   │   │
│  │    Last run: 3 hours ago  │  Workflows: run_ai_agent        │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### 数据来源
- `GET /api/v1/github-workflow/repositories` → 仓库列表
- `GET /api/v1/github-workflow/stats` → 统计卡片

---

## 页面 2：仓库详情 — Runs Tab

### 样例图

```
┌─────────────────────────────────────────────────────────────────────┐
│  ← Back    🔗 AMD-AGI / Primus-Turbo  ↗ GitHub                     │
│                                                                     │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐   │
│  │ ▶ 0        │  │ ✅ 3       │  │ ❌ 0       │  │ 📊 9434    │   │
│  │ Running    │  │ Completed  │  │ Failed     │  │ Metrics    │   │
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘   │
│                                                                     │
│  ┌─────────┬─────────┬──────────┐                                  │
│  │  Runs   │ Metrics │ Settings │                                  │
│  └─────────┴─────────┴──────────┘                                  │
│                                                                     │
│  Status: [All ▼]  Workflow: [All ▼]  [🔍 Search]                   │
│                                                                     │
│  ┌──────┬──────────────────────────┬──────────┬────────┬─────────┐ │
│  │ Run  │ Workflow                 │ Workload │ Status │ Started │ │
│  ├──────┼──────────────────────────┼──────────┼────────┼─────────┤ │
│  │23294 │ ✅ unittest-pytorch-gfx  │ turbo-.. │ done   │ 2h ago  │ │
│  │      │   └─ 4 jobs: ✅✅✅✅     │  -r7gd6  │        │         │ │
│  ├──────┼──────────────────────────┼──────────┼────────┼─────────┤ │
│  │23293 │ ✅ unittest-jax-gfx942   │ turbo-.. │ done   │ 5h ago  │ │
│  │      │   └─ 3 jobs: ✅✅✅       │  -c7gml  │        │         │ │
│  ├──────┼──────────────────────────┼──────────┼────────┼─────────┤ │
│  │23283 │ ✅ unittest-pytorch-gfx  │ turbo-.. │ done   │ 12h ago │ │
│  │      │   └─ 4 jobs: ✅✅✅✅     │  -s227d  │        │         │ │
│  └──────┴──────────────────────────┴──────────┴────────┴─────────┘ │
│                                                                     │
│  ▸ 展开 Run #23294 的 Jobs:                                         │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │  Job                    │ Status │ Runner              │ Time │ │
│  ├─────────────────────────┼────────┼─────────────────────┼──────┤ │
│  │  code-lint (3.12)       │ ✅     │ GitHub Actions      │ 18s  │ │
│  │  install-dependencies   │ ✅     │ turbo-...-wcwg6     │ 80s  │ │
│  │  unittest-jax-gfx942    │ ✅     │ turbo-...-lr598     │ 52m  │ │
│  │  unittest-pytorch-gfx   │ ✅     │ turbo-...-s227d     │ 2h   │ │
│  └─────────────────────────┴────────┴─────────────────────┴──────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

### 数据来源
- `GET /api/v1/github-workflow/repositories/:owner/:repo` → Header 统计
- `GET /api/v1/github-workflow/runs?github_owner=X&github_repo=Y` → Run 列表
- `GET /api/v1/github-workflow/runs/:id/jobs` → 展开的 Jobs

---

## 页面 3：仓库详情 — Metrics Tab

### 样例图（group_mode = "dimension", group_by = "Op"）

```
┌─────────────────────────────────────────────────────────────────────┐
│  ┌─────────┬─────────┬──────────┐                                  │
│  │  Runs   │ Metrics │ Settings │                                  │
│  └─────────┴─────────┴──────────┘                                  │
│                                                                     │
│  Config: [Primus-Turbo MI325 Benchmark ▼]   📊 9434 data points   │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ Chart Type: [Line ▼]  Y-Axis: [value ▼]  Group By: [Op ▼]   │ │
│  │                                                               │ │
│  │  value (TFLOPS)                                               │ │
│  │  600 ┤                                    ╭─── Attention      │ │
│  │      │                              ╭────╯                    │ │
│  │  500 ┤                        ╭────╯     ╭─── GEMM           │ │
│  │      │                  ╭────╯     ╭────╯                    │ │
│  │  400 ┤            ╭────╯     ╭────╯                          │ │
│  │      │      ╭────╯     ╭────╯          ╭─── GroupedGEMM     │ │
│  │  300 ┤╭────╯     ╭────╯          ╭────╯                     │ │
│  │      ├╯     ╭────╯          ╭────╯                           │ │
│  │  200 ┤╭────╯          ╭────╯          ╭─── DeepEP           │ │
│  │      │           ╭────╯          ╭────╯                      │ │
│  │  100 ┤     ╭────╯          ╭────╯                            │ │
│  │      │────╯          ╭────╯                                  │ │
│  │    0 ┼───────────────┼───────────────┼──────────────────     │ │
│  │      Jan-30    Feb-10    Feb-20    Mar-01    Mar-10           │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                     │
│  Fields:  Dimensions: [Framework] [GPU] [Op✓] [Stage]              │
│           Metrics:    [value✓]                                      │
│                                                                     │
│  ▾ Raw Data (9434 rows)                                            │
│  ┌──────────┬─────┬────────────┬──────┬────────┐                   │
│  │ Framework│ GPU │ Op         │Stage │ value  │                   │
│  ├──────────┼─────┼────────────┼──────┼────────┤                   │
│  │ Turbo    │MI325│ DeepEP     │Comb  │ 255.68 │                   │
│  │ Turbo    │MI325│ DeepEP     │FP8   │ 227.61 │                   │
│  │ Turbo    │MI325│ Attention  │Fwd   │ 557.59 │                   │
│  │ ...      │     │            │      │        │                   │
│  └──────────┴─────┴────────────┴──────┴────────┘                   │
│  Showing 1-50 of 9434  [< 1 2 3 ... 189 >]                        │
└─────────────────────────────────────────────────────────────────────┘
```

### 样例图（group_mode = "metric"，Primus-lm Benchmark）

```
┌─────────────────────────────────────────────────────────────────────┐
│  Config: [MI325 Primus-lm Benchmark ▼]   📊 223 data points       │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ Chart Type: [Line ▼]  Filter: Model=[All ▼] Framework=[All ▼]│ │
│  │                                                               │ │
│  │  Tokens/s/GPU                                                 │ │
│  │  14000 ┤                                                      │ │
│  │        │     ╭── deepseek_v3_16b-BF16                        │ │
│  │  12000 ┤╭───╯                                                │ │
│  │        ├╯    ╭── qwen2.5_7B-BF16                             │ │
│  │  10000 ┤╭───╯                                                │ │
│  │        │     ╭── llama3.1_8B-BF16                            │ │
│  │   8000 ┤╭───╯                                                │ │
│  │        │                                                      │ │
│  │   6000 ┤╭─── mixtral_8x7B-BF16                              │ │
│  │        │                                                      │ │
│  │   2000 ┤                                                      │ │
│  │        │     ╭── llama3.1_70B-BF16                           │ │
│  │   1000 ┤────╯                                                │ │
│  │      0 ┼─────────┼─────────┼─────────┼──────────────────     │ │
│  │        Jan-19    Feb-01    Feb-15    Feb-28                   │ │
│  │                                                               │ │
│  │  ┌────────────────────────────────────────────────────┐       │ │
│  │  │ Y-Axis: ○Tokens/s/GPU ○TFLOP/s/GPU ○Step Time    │       │ │
│  │  │         ○Mem Usage                                 │       │ │
│  │  └────────────────────────────────────────────────────┘       │ │
│  └───────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

### 数据来源
- `GET /api/v1/github-workflow/repositories/:owner/:repo` → config 列表
- `GET /api/v1/github-workflow/collection-configs/:id/fields` → 字段 + display_settings
- `GET /api/v1/github-workflow/collection-configs/:id/metrics?limit=5000` → 指标数据

### 图表渲染逻辑

```javascript
// 1. 获取 fields + display_settings
const { fields, display_settings } = await api.getFields(configId)

// 2. 分类字段
const dimensions = fields.filter(f => f.hint === 'dimension')
const metrics = fields.filter(f => f.hint === 'metric')

// 3. 获取 metrics 数据
const { metrics: rows } = await api.getConfigMetrics(configId, { limit: 5000 })

// 4. 根据 display_settings 渲染
if (display_settings.default_chart_group_mode === 'dimension') {
  // 按维度分组: 每个 dimension 值一条线
  const groupBy = display_settings.default_chart_group_by || dimensions[0]?.name
  const metricField = metrics[0]?.name || 'value'
  const groups = [...new Set(rows.map(r => r.row_data[groupBy]))]
  
  series = groups.map(g => ({
    name: g,
    type: display_settings.default_chart_type || 'line',
    data: rows
      .filter(r => r.row_data[groupBy] === g)
      .map(r => [r.row_data['date'] || r.created_at, r.row_data[metricField]])
      .sort((a, b) => a[0] > b[0] ? 1 : -1)
  }))
} else if (display_settings.default_chart_group_mode === 'metric') {
  // 按指标分组: 每个 metric 字段一条线
  series = metrics.map(m => ({
    name: m.name,
    type: display_settings.default_chart_type || 'line',
    data: rows
      .filter(r => r.row_data[m.name] != null)
      .map(r => [r.row_data['date'] || r.created_at, r.row_data[m.name]])
      .sort((a, b) => a[0] > b[0] ? 1 : -1)
  }))
}
```

---

## 页面 4：仓库详情 — Settings Tab

### 样例图

```
┌─────────────────────────────────────────────────────────────────────┐
│  ┌─────────┬─────────┬──────────┐                                  │
│  │  Runs   │ Metrics │ Settings │                                  │
│  └─────────┴─────────┴──────────┘                                  │
│                                                                     │
│  Collection Configs                                                 │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ 📊 Primus-Turbo MI325 Benchmark                    [Edit] [✕]│ │
│  │                                                               │ │
│  │ File Patterns:                                                │ │
│  │   /wekafs/primus_turbo/benchmark/{yyyymmdd}/**/summary.csv   │ │
│  │   /wekafs/primus_turbo/benchmark/{yyyymmdd}/summary.csv      │ │
│  │                                                               │ │
│  │ Workflow Filter:  (none)                                      │ │
│  │ Branch Filter:    (none)                                      │ │
│  │                                                               │ │
│  │ Display Settings:                                             │ │
│  │   Chart Type:     line                                        │ │
│  │   Group Mode:     dimension                                   │ │
│  │   Group By:       Op                                          │ │
│  │   Show Raw Data:  ✅                                          │ │
│  │                                                               │ │
│  │ Metrics: 9434 data points │ Enabled: ✅                       │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                     │
│  [+ Add Collection Config]                                          │
└─────────────────────────────────────────────────────────────────────┘
```

### 数据来源
- `GET /api/v1/github-workflow/repositories/:owner/:repo` → config 列表
- `PUT /api/v1/github-workflow/collection-configs/:id` → 更新配置
- `POST /api/v1/github-workflow/collection-configs` → 创建配置
- `DELETE /api/v1/github-workflow/collection-configs/:id` → 删除配置

---

## 完整 API 清单（14 个端点）

| # | 方法 | 路径 | 用途 | 状态 |
|---|------|------|------|------|
| 1 | GET | `/github-workflow/repositories` | 仓库列表（聚合 runs + configs） | ✅ |
| 2 | GET | `/github-workflow/repositories/:owner/:repo` | 仓库详情（workflows + configs） | ✅ |
| 3 | GET | `/github-workflow/collection-configs` | 采集配置列表 | ✅ |
| 4 | POST | `/github-workflow/collection-configs` | 创建采集配置 | ✅ |
| 5 | PUT | `/github-workflow/collection-configs/:id` | 更新采集配置 | ✅ |
| 6 | DELETE | `/github-workflow/collection-configs/:id` | 删除采集配置 | ✅ |
| 7 | GET | `/github-workflow/collection-configs/:id/fields` | 字段列表 + display_settings | ✅ |
| 8 | GET | `/github-workflow/collection-configs/:id/metrics` | 指标数据（row_data） | ✅ |
| 9 | GET | `/github-workflow/runs` | Run 列表（支持 owner/repo/status 过滤） | ✅ |
| 10 | GET | `/github-workflow/runs/:id` | Run 详情 | ✅ |
| 11 | GET | `/github-workflow/runs/:id/jobs` | Jobs + Steps | ✅ |
| 12 | GET | `/github-workflow/runs/:id/metrics` | 按 run 查 metrics | ✅ |
| 13 | GET | `/github-workflow/commits/:sha` | Commit 详情 | ✅ |
| 14 | GET | `/github-workflow/stats` | 汇总统计 | ✅ |

## 前端技术栈

- **框架**: Vue 3 + Element Plus（与现有 SaFE 前端一致）
- **图表**: ECharts
- **路由**: 在 `/cicd` 页面内通过 Tab 切换，仓库详情通过 `:owner/:repo` 参数
- **样式**: 复用 Lens GithubWorkflow 的 CSS 变量和卡片样式
