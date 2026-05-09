# Model Optimization API — core42 接口测试记录

| 项目 | 内容 |
|------|------|
| 环境 | `https://core42.primus-safe.amd.com` |
| 测试时间 | 2026-04-21（自动化请求） |
| 鉴权 | `Authorization: Bearer <API Key>`（密钥由测试方提供，本文不写入完整 Key） |
| Workspace | `core42-sandbox` |
| 抽样模型 | `GET /api/v1/playground/models?workspace=core42-sandbox` → 选用 **`minimax-m2-5-nvfp4-gjcv5`**（`phase=Ready`，`localPaths` 含 `core42-sandbox`） |

基路径：`/api/v1/optimization`（与 `routers.go` 中 `PrimusRouterCustomRootPath + "/optimization"` 一致）。

---

## 路由清单（对照 `SaFE/apiserver/pkg/handlers/optimization/routers.go`）

说明：

- **✅**：本次请求得到预期行为（含列表 200、或对不存在任务返回 **404** 且非 Gin `NoRoute`，说明路由已挂上并由 Handler 处理）。
- **❌**：本次请求失败或返回非预期错误（见备注）。
- **`:id`**：以下使用占位任务 ID `00000000-0000-0000-0000-000000000001` 探测路由是否注册（预期 **404**「任务不存在」类响应，而非整站 404 页面）。

| # | 方法 | 路径 | 结果 | 备注 |
|---|------|------|------|------|
| 1 | `GET` | `/tasks` | ✅ | `GET .../optimization/tasks?workspace=core42-sandbox` → **HTTP 200**，`total=0` |
| 2 | `POST` | `/tasks` | ❌ | **HTTP 500**，`errorMessage`: `Claw base URL not configured; Model Optimization disabled`（创建任务依赖 Claw 基址；需在 apiserver 配置/Secret 中配置 `claw_base_url` 等） |
| 3 | `POST` | `/tasks/batch` | ❌ | 同上 **HTTP 500**，原因与单条创建相同 |
| 4 | `GET` | `/tasks/:id` | ✅ | 占位 id → **HTTP 404**（路由可达） |
| 5 | `GET` | `/tasks/:id/artifacts` | ✅ | 占位 id → **HTTP 404** |
| 6 | `GET` | `/tasks/:id/artifacts/download` | ✅ | 占位 id（无 `path` 或带假 `path`）→ **HTTP 404**（任务不存在时先于 `path` 校验返回） |
| 7 | `POST` | `/tasks/:id/interrupt` | ✅ | 占位 id → **HTTP 404** |
| 8 | `POST` | `/tasks/:id/retry` | ✅ | 占位 id → **HTTP 404** |
| 9 | `POST` | `/tasks/:id/apply` | ✅ | 占位 id → **HTTP 404** |
| 10 | `DELETE` | `/tasks/:id` | ✅ | 占位 id → **HTTP 404** |
| 11 | `GET` | `/tasks/:id/events`（SSE） | ✅ | 占位 id → **HTTP 404**（`Authorize` 组，无 `Preprocess`/`Audit`） |

---

## 结论摘要

1. **路由与鉴权**：在提供有效 API Key 时，上述 **11 条** 优化相关路径均可访问，**未出现**未注册路由导致的 **Gin `NoRoute` / 前端统一 404**（对占位 `id` 统一为 **404**，符合「任务不存在」预期）。
2. **创建类接口**：`POST /tasks` 与 `POST /tasks/batch` 当前返回 **500**，根因为服务端 **`Claw base URL not configured`**，需在 **core42 管理面** 为 apiserver 配置模型优化所需的 **PrimusClaw 地址**（及密钥等），与 ConfigMap / Secret 中 `model_optimization` 段一致后重测。
3. **完整链路（创建 → 事件流 → 工件下载等）**：依赖成功创建任务及 Claw 会话，**本次未执行**（因创建失败且无历史任务 `total=0`）。配置 Claw 后建议用同表逐条复测并可将 **❌** 更新为 **✅**。

---

## 参考命令（勿将真实 Key 提交到仓库）

```bash
export BASE=https://core42.primus-safe.amd.com/api/v1
export HDR="Authorization: Bearer <YOUR_API_KEY>"

curl -sS -H "$HDR" "$BASE/playground/models?workspace=core42-sandbox" | head

curl -sS -H "$HDR" "$BASE/optimization/tasks?workspace=core42-sandbox"
curl -sS -H "$HDR" -H "Content-Type: application/json" \
  -d '{"displayName":"test","modelId":"minimax-m2-5-nvfp4-gjcv5","workspace":"core42-sandbox"}' \
  "$BASE/optimization/tasks"
```
