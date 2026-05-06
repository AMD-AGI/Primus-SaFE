# Model Optimization API — 接口全量测试报告

| 项目 | 内容 |
|------|------|
| 环境 | `https://core42.primus-safe.amd.com` |
| 测试时间 | 2026-04-29 |
| 镜像版本 | `apiserver:202604291309`（commit `5d0529f0`） |
| 测试模型 | `qwen3-8b-4vdpm`（Qwen/Qwen3-8B，`/wekafs/models/Qwen-Qwen3-8B`） |
| Workspace | `core42-sandbox` |

---

## 全量路由测试结果

| # | 方法 | 路径 | HTTP 状态 | 结果 | 备注 |
|---|------|------|-----------|------|------|
| 1 | GET | `/tasks` | 200 | ✅ | `total=2`，workspace 过滤正常 |
| 2 | POST | `/tasks` | 200 | ✅ | 创建成功，返回 `id`+`clawSessionId`，Claw session 建立 |
| 3 | POST | `/tasks/batch` | 207 | ✅ | item-1 成功，item-2（无效 modelId）返回 `error` 字段；批量混合结果正确 |
| 4 | GET | `/tasks/:id`（真实） | 200 | ✅ | 返回完整任务详情，status=Running |
| 4b | GET | `/tasks/:id`（不存在） | 404 | ✅ | `task not found` |
| 5 | GET | `/tasks/:id/artifacts`（Running） | 200 | ✅ | `items: []`（任务运行中，无产物） |
| 5b | GET | `/tasks/:id/artifacts`（不存在） | 404 | ✅ | `task not found` |
| 6 | GET | `/tasks/:id/artifacts/download`（缺 path） | 400 | ✅ | `path query parameter is required` |
| 6b | GET | `/tasks/:id/artifacts/download?path=xxx`（文件不存在） | 500 | ⚠️ | `failed to download artifact`，应改为 404，见备注 |
| 7 | POST | `/tasks/:id/interrupt`（Running） | 200 | ✅ | `status: Interrupted`，Claw session 中止 |
| 7b | POST | `/tasks/:id/interrupt`（不存在） | 404 | ✅ | `task not found` |
| 8 | POST | `/tasks/:id/retry`（Interrupted） | 201 | ✅ | 生成新 task id + clawSessionId，Claw session 重建 |
| 8b | POST | `/tasks/:id/retry`（不存在） | 404 | ✅ | `task not found` |
| 9 | POST | `/tasks/:id/apply`（Running，无 image） | 500 | ⚠️ | `no image configured on task`，预期行为（task 未带 image 字段），见备注 |
| 9b | POST | `/tasks/:id/apply`（不存在） | 404 | ✅ | `task not found` |
| 10 | DELETE | `/tasks/:id`（不存在） | 404 | ✅ | `task not found` |
| 10b | DELETE | `/tasks/:id`（Running） | 204 | ✅ | 删除成功，无响应体 |
| 11 | GET | `/tasks/:id/events`（SSE） | 200 | ✅ | 事件流正常，keepalive 工作，Claw sandbox 事件推送 |

**✅ 9/11 完全符合预期，⚠️ 2 个 minor 问题（见下）**

---

## 详细结果

### POST /tasks — 创建 Hyperloom 任务
```json
请求: {"displayName":"api-test-qwen3-8b","modelId":"qwen3-8b-4vdpm","workspace":"core42-sandbox",
       "mode":"local","framework":"sglang","tp":1,"isl":1024,"osl":512,"concurrency":1}
响应: {"id":"opt-70b893bd-138d-460a-ade6-97d82c3b7eef","clawSessionId":"0822c2da-568b-4b03-9409-7b3b4d3443e3"}
```
Prompt 自动构建（节选）：
```
mode: local | Framework: sglang | ISL=1024, OSL=512, CONC=1 | TP=1 | GPU: MI355X
KERNEL_OPT_BACKENDS: claude
```

### POST /tasks/batch — 207 混合结果
```json
请求 items: [valid qwen3-8b, invalid modelId "INVALID_MODEL"]
响应: {
  "items": [
    {"id":"opt-20726d5d-...","clawSessionId":"e079f9f5-..."},   ← 成功
    {"error":"model \"INVALID_MODEL\" not found: record not found"}  ← 失败，不影响其他
  ]
}
```

### POST /tasks/:id/interrupt → POST /tasks/:id/retry
```
interrupt → status: Interrupted (HTTP 200)
retry     → {"id":"opt-6c4dd11f-...","clawSessionId":"a5fccc20-..."} (HTTP 201)
```
Retry 产生新 task id，旧 task 状态保持 Interrupted，新 task Running —— 符合设计。

### GET /tasks/:id/events — SSE 事件流
```
event: log  payload: {source:"claw", message: "sandbox phase=Creating"}
event: log  payload: {source:"claw", message: "Waiting in queue (queuePosition:0)"}
: keepalive  ← 心跳正常
```
Claw 沙箱排队等 GPU，属正常调度，非错误。

---

## ⚠️ 两个 Minor 问题

### 问题 1：`/artifacts/download` 文件不存在返回 500
- **现象**：`?path=nonexistent.txt` → HTTP 500 `failed to download artifact`
- **预期**：HTTP 404
- **影响**：低，任务未完成时前端不会调用此接口
- **修复方向**：`DownloadArtifact` handler 里 Claw 返回 404 时映射到 `commonerrors.NewNotFound`

### 问题 2：`/apply` 无 image 时返回 500
- **现象**：task 未设置 `image` 字段时 → HTTP 500 `no image configured on task`
- **预期**：HTTP 400（业务校验失败，不是 Internal Error）
- **影响**：低，正常使用需要传 `image` 参数
- **修复方向**：`ApplyTask` handler 里改为 `commonerrors.NewBadRequest`

---

## 最终任务列表

| task id | status | displayName |
|---------|--------|-------------|
| `opt-6c4dd11f` | Running | api-test-qwen3-8b（retry） |
| `opt-70b893bd` | Interrupted | api-test-qwen3-8b |
| `opt-9e4698fc` | Failed | cursor-hyperloom-default（旧版 401） |

---

## 结论

**核心链路 100% 打通**：创建 → Claw session → SSE 流 → interrupt → retry 全流程验证通过。旧版本的 `401 API key failed` 和 `unexpected EOF` 问题已消除。

**待观测**：Claw 沙箱就绪后 Hyperloom Phase 0-10 是否正常执行（当前等待 GPU 调度）。沙箱就绪后建议重新订阅 SSE，验证 `phase`/`benchmark`/`kernel` 结构化事件是否正常推送。
