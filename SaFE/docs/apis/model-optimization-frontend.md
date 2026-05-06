# Model Optimization 前端实现说明

这份文档给 `Web/apps/safe` 前端同事使用，用于在 SaFE 中新增 `Model Optimization` 模块的页面与交互实现。

后端 API 文档见：

- `SaFE/docs/apis/model-optimization.md`

## 1. 目标

在 SaFE 中新增一个 `Model Optimization` 模块，让用户可以对 **已经下载完成的模型** 发起 Hyperloom 优化任务，并在 SaFE 内完成：

- 创建优化任务
- 查看实时进度
- 查看 benchmark / kernel / 日志
- 下载优化产物
- 中断 / 重试
- （可选）一键应用为新的 Infer Workload：走 `POST …/optimization/tasks/:id/apply`；若产品不要求「优化 → 一键部署」，可改为引导用户使用既有 **`POST /api/v1/workloads`** 自行起推理

前端 **不需要理解 Hyperloom 内部的 DFS 或 Claw/Claude 原始事件格式**。后端已经把运行时事件翻译为结构化的 SSE 数据，前端只需要消费这些 API。

**创建任务成功判定**：`POST /tasks` 返回 **HTTP 201** 且 body 含 **`id`**、**`clawSessionId`** 表示 SaFE 已落库且已向 Claw 建会话并下发首条 prompt。若返回 5xx 或任务很快变为 **Failed**，查看 **`GET /tasks/:id` 的 `message`**（例如 Claw 侧校验 Key 时无法访问 SaFE `auth/verify` 等部署问题，非前端表单错误）。

## 2. 建议放置位置

建议基于现有 `Web/apps/safe` 结构新增：

```text
Web/apps/safe/src/pages/ModelOptimization/
├── index.vue
├── Detail.vue
├── components/
│   ├── CreateTaskDrawer.vue
│   ├── TaskTable.vue
│   ├── TaskStatusTag.vue
│   ├── PhaseTimeline.vue
│   ├── BenchmarkPanel.vue
│   ├── KernelPanel.vue
│   ├── LogPanel.vue
│   ├── ArtifactPanel.vue
│   └── ApplyDrawer.vue
└── composables/
    ├── useOptimizationEvents.ts
    └── useOptimizationTaskForm.ts

Web/apps/safe/src/services/model-optimization/
├── index.ts
└── type.ts
```

如果前端想先做 MVP，可以最少保留：

- `index.vue`
- `Detail.vue`
- `services/model-optimization/index.ts`
- `services/model-optimization/type.ts`

## 3. 可复用文件

可以参考这些现有实现：

- 路由：`Web/apps/safe/src/router/index.ts`
- 左侧菜单：`Web/apps/safe/src/components/layout/BaseMenu.vue`
- 图标：`Web/apps/safe/src/components/layout/menuIcons.ts`
- 请求封装：`Web/apps/safe/src/services/request.ts`
- Model Square：`Web/apps/safe/src/pages/ModelSquare/index.vue`
- Infer Drawer：`Web/apps/safe/src/pages/Infer/Components/AddDialog.vue`

## 4. Router 与菜单

### 4.1 Router

建议在 `Web/apps/safe/src/router/index.ts` 中增加两条路由：

- `/model-optimization`：任务列表页
- `/model-optimization/:id`：任务详情页

不建议单独做 `/create` 页面。创建任务更适合用 Drawer/Dialog。

### 4.2 菜单

建议在 `Web/apps/safe/src/components/layout/BaseMenu.vue` 的 `Model Lab` 分组里新增：

- `Model Optimization`

和这些模块同一级：

- `Model Square`
- `Playground`
- `Evaluation`

## 5. 页面职责

### 5.1 列表页 `index.vue`

功能：

- 查看任务列表
- 按 `workspace / status / modelId / search` 过滤
- 新建任务
- 跳转详情
- 对失败任务执行 `retry`
- 对运行中任务执行 `interrupt`
- 对成功任务执行 `apply`

建议列表列：

- `Display Name`
- `Model`
- `Workspace`
- `Status`
- `Current Phase`
- `UpdatedAt`
- `Actions`

### 5.2 创建任务抽屉 `CreateTaskDrawer.vue`

表单字段尽量和 Hyperloom Web 保持一致。

#### HTTP 最小必填（与后端 `binding:"required"` 一致）

仅以下两项在 **不传即 400** 的意义上为必填：

- `modelId`
- `workspace`

其余字段若省略，后端会按 `model-optimization.md` 所述与 **Hyperloom-Web `useInferOptTemplate.ts`** 相同的默认值补全（如 `mode` 默认 `local`、`kernelBackends` 默认含 **Claude Code**、镜像默认等）。

#### 建议表单仍展示/可编辑的字段（与 Hyperloom 对齐，便于用户控制）

- `displayName`
- `mode`（`local` | `claw`）
- `framework`、`precision`、`tp`、`ep`、`gpuType`、`isl`、`osl`、`concurrency`
- `kernelBackends`、`geakStepLimit`
- `image`（**强烈建议**允许按环境覆盖默认 Harbor 前缀，避免与集群镜像仓库不一致）
- `inferencexPath`、`resultsPath`
- `rayReplica`、`rayGpu`、`rayCpu`、`rayMemory`（主要在 `mode=claw` 的 prompt 块中使用）
- `targetGpu`、`baselineCSV`、`baselineCount`

#### 模型来源

模型下拉框直接来自已下载模型列表，只展示：

- `phase === Ready`
- `accessMode === local` 或 `accessMode === local_path`

不建议让用户手填 `modelPath`，后端会根据 `modelId + workspace` 自动解析。

### 5.3 详情页 `Detail.vue`

建议分 4 块：

1. 基本信息
2. 实时进度
3. 结构化事件
4. 产物与 apply

## 6. 后端 API

Base Path:

```text
/api/v1/optimization
```

### 6.1 任务

- `POST /tasks`
- `POST /tasks/batch`
- `GET /tasks`
- `GET /tasks/:id`
- `DELETE /tasks/:id`

### 6.2 实时事件

- `GET /tasks/:id/events`

这是 SSE。

### 6.3 生命周期

- `POST /tasks/:id/interrupt`
- `POST /tasks/:id/retry`

### 6.4 产物

- `GET /tasks/:id/artifacts`
- `GET /tasks/:id/artifacts/download?path=...`

### 6.5 一键部署

- `POST /tasks/:id/apply`

## 7. 前端 service 建议

建议在 `Web/apps/safe/src/services/model-optimization/index.ts` 至少封装这些方法：

```ts
listTasks(params)
getTask(id)
createTask(payload)
batchCreateTasks(payload)
interruptTask(id)
retryTask(id)
deleteTask(id)
listArtifacts(id)
downloadArtifact(id, path) // GET …/artifacts/download?path=…
applyTask(id, payload)
subscribeTaskEvents(id, afterEventId?)
```

## 8. SSE 事件说明

前端 **不要解析 Claw 原始消息**。后端已经统一成结构化事件。

### 8.1 SSE event name

可能收到：

- `phase`
- `benchmark`
- `kernel`
- `log`
- `status`（类型已定义；payload 为任务级状态过渡，与 `types.go` 中 `StatusEventPayload` 一致：`status`、`message`）
- `done`

### 8.2 data 格式

统一 envelope：

```json
{
  "id": "opt-xxxx-12",
  "taskId": "opt-xxxx",
  "type": "phase",
  "timestamp": 1710000000000,
  "payload": {}
}
```

### 8.3 phase payload

```json
{
  "phase": 2,
  "phaseName": "Baseline",
  "status": "started",
  "message": ""
}
```

### 8.4 benchmark payload

后端可能带上更多数值字段（均为可选），前端宜做兼容展示：

```json
{
  "round": 1,
  "label": "baseline benchmark",
  "inputTokensPerSec": 0,
  "outputTokensPerSec": 571.3,
  "totalTokensPerSec": 0,
  "tpotMs": 6.78,
  "ttftMs": 44.12,
  "concurrency": 64,
  "isl": 1024,
  "osl": 256,
  "framework": "sglang"
}
```

### 8.5 kernel payload

```json
{
  "name": "triton_tem_fused_mm_0",
  "backend": "GEAK",
  "status": "patched",
  "source": "inductor",
  "gpuPercent": 0,
  "baselineUs": 120.5,
  "optimizedUs": 88.1
}
```

`source`、`gpuPercent` 可能为空，按可选字段处理。

### 8.6 log payload

```json
{
  "level": "info",
  "source": "hyperloom",
  "message": "## Phase 2: Baseline Benchmark ..."
}
```

### 8.7 status payload（`type: "status"`）

与 `StatusEventPayload` 一致，用于任务级状态提示（是否下发依运行时版本而定，前端应容错忽略未知 `type`）：

```json
{
  "status": "Running",
  "message": ""
}
```

### 8.8 done payload

```json
{
  "status": "Succeeded",
  "message": "completed"
}
```

## 9. SSE 接入注意事项

### 9.1 推荐方式

优先用同源 `EventSource`：

```ts
new EventSource(`/api/v1/optimization/tasks/${id}/events?after_event_id=${lastEventId}`)
```

### 9.2 原因

浏览器原生 `EventSource` 不支持自定义 `Authorization` header。  
如果当前 SaFE 页面和 apiserver 是 **同源且依赖浏览器 Cookie（Token）** 登录，浏览器会自动带 cookie，就没有问题。

### 9.3 如果当前鉴权不是 cookie（例如仅用 SSO API Key `ak-…`）

必须使用能带 **`Authorization: Bearer …`** 的 SSE 客户端，**不要**用原生 `EventSource`，例如：

- `@microsoft/fetch-event-source`

否则订阅 `/tasks/:id/events` 会得到 **401**，与 REST 能否调通无关。

### 9.4 重连策略

前端要维护 `lastEventId`，断线重连时带上：

```text
GET /api/v1/optimization/tasks/:id/events?after_event_id=<lastEventId>
```

这样不会丢事件。

## 10. TUI 示例图

### 10.1 任务列表页

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Model Optimization                                              [New Task]  │
├──────────────────────────────────────────────────────────────────────────────┤
│ Filters: [Workspace ▼] [Status ▼] [Search.....................] [Refresh]   │
├──────────────────────────────────────────────────────────────────────────────┤
│ Display Name         Model         Workspace        Status      Phase        │
│──────────────────────────────────────────────────────────────────────────────│
│ qwen3-opt-1          Qwen3-30B     cp-sandbox       Running     Profile      │
│ dsr1-opt             DeepSeek-R1   cp-sandbox       Succeeded   Report       │
│ kimi-int4-opt        Kimi-K2.5     team-a           Failed      Kernel Opt   │
│──────────────────────────────────────────────────────────────────────────────│
│ Actions: [Detail] [Retry] [Interrupt] [Apply] [Delete]                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 10.2 创建任务抽屉

```text
┌──────────────────────────── Create Optimization Task ───────────────────────┐
│ Model              [ Qwen3-30B-A3B                            ▼ ]           │
│ Workspace          [ control-plane-sandbox                    ▼ ]           │
│ Display Name       [ qwen3-opt-1                               ]           │
│                                                                            │
│ Framework          (•) sglang   ( ) vllm                                     │
│ Precision          [ FP4                                         ]         │
│ GPU Type           [ MI355X                                      ]         │
│ TP                 [ 1 ]   EP [ 1 ]   CONC [ 64 ]                           │
│ ISL                [ 1024 ]      OSL [ 1024 ]                               │
│                                                                            │
│ Kernel Backends    [ ] GEAK   [ ] Codex   [x] Claude Code   （默认与后端一致）│
│ GEAK Step Limit    [ 100 ]                                                │
│ Image              [ harbor/.../sglang:latest                       ]      │
│ Results Path       [ /workspace/hyperloom/                          ]      │
│                                                                            │
│                                     [Cancel]   [Create Task]               │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 10.3 任务详情页

```text
┌──────────────────────────── Optimization Detail ────────────────────────────┐
│ qwen3-opt-1                                       Status: Running Phase: 3  │
│ Model: Qwen3-30B-A3B   Workspace: control-plane-sandbox                     │
├──────────────────────────────────────────────────────────────────────────────┤
│ Phase Timeline                                                         3/10 │
│ [Classify]──[Baseline]──[Profile]──[TraceLens]──[Kernel Opt]──[...]        │
│    done        done        doing                                             │
├──────────────────────────────────────────────────────────────────────────────┤
│ Benchmark Summary                                                           │
│ Baseline tok/s: 571.3     Current best tok/s: 653.3     TPOT: 5.90ms       │
├──────────────────────────────────────────────────────────────────────────────┤
│ Kernel Events                                                               │
│ - RMSNorm                     GEAK       patched       120.5us -> 88.1us    │
│ - triton_tem_fused_mm_0       Codex      optimizing                          │
├──────────────────────────────────────────────────────────────────────────────┤
│ Logs                                                                        │
│ [12:00:01] ## Phase 2: Baseline Benchmark                                   │
│ [12:01:12] baseline benchmark done, 571.3 tok/s                             │
│ [12:03:40] ## Phase 3: Profile                                              │
│ [12:05:05] submitting kernel RMSNorm to GEAK                                │
├──────────────────────────────────────────────────────────────────────────────┤
│ Artifacts                                                                   │
│ - claw-1/optimization_report.md                              [Download]      │
│ - claw-1/results/.../baseline.json                           [Download]      │
│ - claw-1/results/sweep/results.tsv                            [Download]      │
├──────────────────────────────────────────────────────────────────────────────┤
│ Actions: [Interrupt] [Retry] [Apply to Infer]                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 10.4 Apply 抽屉

```text
┌──────────────────────────── Apply Optimized Result ─────────────────────────┐
│ Task: qwen3-opt-1                                                           │
│ Report: claw-1/optimization_report.md                                       │
│                                                                            │
│ Recommended launch command:                                                │
│ python3 -m sglang.launch_server --model-path ... --tp 1 --port 8888 ...    │
│                                                                            │
│ Display Name       [ qwen3-optimized-infer ]                               │
│ Workspace          [ control-plane-sandbox                      ▼ ]         │
│ Image              [ harbor/.../sglang:latest ]                            │
│ CPU                [ 16 ]   Memory [ 64Gi ]   GPU [ 1 ]   Replica [ 1 ]    │
│ Port               [ 8888 ]                                                │
│                                                                            │
│                                    [Cancel]   [Create Workload]            │
└──────────────────────────────────────────────────────────────────────────────┘
```

## 11. 推荐交互行为

### 创建后

- 创建成功后直接跳转到详情页
- 详情页立刻建立 SSE
- 如果 SSE 失败，页面至少还能通过 `GET /tasks/:id` 保持任务状态可见

### 详情页刷新

- 先拉 `GET /tasks/:id`
- 再拉 `GET /tasks/:id/artifacts`
- 最后建 SSE 订阅

### 产物展示

建议按类别分组：

- Report
- Benchmark JSON / TSV
- Kernel files
- Logs

### Apply 成功后

- 提示 workloadId
- 提供跳转到 `Infer` 或 workload 详情页的入口

## 12. MVP 实现优先级

如果前端时间紧，建议按下面顺序做：

### P0

- 列表页
- 创建任务弹窗
- 详情页基础信息
- Phase timeline
- Log panel
- Artifact list

### P1

- Benchmark panel
- Kernel panel
- Retry / Interrupt / Apply 按钮

### P2

- 更细的图表
- 更强的筛选与统计
- 批量创建交互

## 13. 备注

- 当前后端已经支持 `batch / artifacts / interrupt / retry / apply`
- 前端不需要自己拼 prompt，也不需要理解 Hyperloom 的 phase 内部细节
- 后端已经负责把 Claw/Claude 的原始流转成前端可直接消费的结构化事件
- **Infer Workload 通用创建**仍在 **`POST /api/v1/workloads`**（`resources` 路由）；`playground/models/:id/workload-config` 等接口用于辅助拼单，与优化模块无重复路由

---

## 14. 后端本次变更 — 前端需要关注的地方（2026-04-28）

### 14.1 Breaking Change：`POST /tasks/batch` 响应结构

**之前**：任意一个 item 失败 → 整个请求返回 4xx，无 `items` 数组。

**现在**：HTTP 状态码改为 `207 Multi-Status`，每个 item 单独返回，失败的 item 有 `error` 字段。

```typescript
// 新增类型，需要加到 services/type.ts
interface BatchCreateTaskResponseItem {
  id?: string            // 成功时有值
  clawSessionId?: string // 成功时有值
  error?: string         // 失败时非空，成功时缺省
}

interface BatchCreateTasksResponse {
  items: BatchCreateTaskResponseItem[]  // 原来是 CreateTaskResponse[]
}
```

`services/index.ts` 中 `batchCreateOptimizationTasks` 的返回类型从 `Promise<any>` 改为 `Promise<BatchCreateTasksResponse>`，调用处检查每个 `item.error`。

---

### 14.2 新增输入校验（400 Bad Request）

`POST /tasks` 现在对以下字段做范围校验，前端表单建议同步添加校验规则：

| 字段 | 合法范围 |
|---|---|
| `tp` / `ep` | 0 – 256 |
| `isl` / `osl` | 0 – 1,000,000 |
| `concurrency` | 0 – 10,000 |
| `geakStepLimit` | 0 – 10,000 |
| `mode` | `"local"` 或 `"claw"` |
| `framework` | `"sglang"` 或 `"vllm"` |
| `resultsPath` | 不能包含 `..` |

---

### 14.3 前端自身 Bug — 需要修复（按优先级）

#### P0-1：创建任务失败无提示（`CreateTaskDrawer.vue`）

`handleSubmit` 的 `try` 块缺 `catch`，API 报错时 loading 消失但用户看不到任何提示。

```typescript
// 加上 catch
} catch (e: any) {
  ElMessage.error(e?.message || 'Failed to create task')
} finally {
  submitting.value = false
}
```

#### P0-2：模型列表加载失败无提示（`CreateTaskDrawer.vue`）

`loadModels` 同样缺 `catch`，失败时下拉框空白、没有提示。

```typescript
} catch {
  ElMessage.error('Failed to load models')
} finally {
  modelsLoading.value = false
}
```

#### P0-3：SSE 断线无任何提示（`useOptimizationEvents.ts` + `Detail.vue`）

`sseError` 设了但从未在模板中展示，长任务（几小时）断网后用户看不到任何异常。

`useOptimizationEvents.ts` 的 `onerror` 加自动重连：

```typescript
es.onerror = () => {
  sseError.value = true
  close()
  setTimeout(() => { if (!isDone.value) connect() }, 5000)
}
```

`Detail.vue` 模板里展示断线提示：

```vue
<el-alert v-if="sseError" type="warning" title="实时连接已断开" :closable="false">
  <el-button link @click="connect()">重新连接</el-button>
</el-alert>
```

#### P1：Interrupt / Retry / Delete 操作失败无提示（`index.vue`）

三个操作的 `await` 调用都缺 `catch`，API 失败时会显示"操作成功"但实际未生效。统一加：

```typescript
try {
  await interruptOptimizationTask(row.id)   // 或 retry / delete
  ElMessage.success('...')
} catch (e: any) {
  ElMessage.error(e?.message || '操作失败')
} finally {
  fetchData()
}
```

#### P1：无效的 `EventSource` 选项（`services/index.ts`）

`new EventSource(url, { withCredentials: true })` — `withCredentials` 不是 EventSource 的标准选项，删掉即可：

```typescript
return new EventSource(url)
```

#### P1：`OptimizationTask` 索引签名破坏类型安全（`services/type.ts`）

删掉 `[key: string]: unknown` 这一行，它会让所有字段访问绕过类型检查。
