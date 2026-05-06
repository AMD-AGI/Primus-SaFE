# Model Square 前端实现说明

本文档说明 **Web/apps/safe** 中「模型广场」应如何对接新增/既有能力：**登记已有共享路径**（`local_path`）、**从用户 S3 导入**（`s3_sync`），以及现有 **Hugging Face**、**Remote API**。  
**后端实现位置**：分支/工作树 `feature/chenyyan/model-square-s3-pfs`（建议在本 worktree 上开发前端联调分支）。

---

## 1. 创建模型：四种模式对照

| 模式 | `source.accessMode` | 必填与说明 |
|------|---------------------|------------|
| Hugging Face 下载 | `local` | `source.url` 为 HF 或 `org/model`；元数据由后端从 HF 拉取。 |
| Remote API | `remote_api` | `displayName`、`source.modelName`、（建议）`apiKey` 等。 |
| 已有共享盘路径 | `local_path` | `displayName`、`source.localPath`；**可传** `icon`、`label`、`tags`、`maxTokens`、`source.url`（留档）、`origin`（如 `external` / `fine_tuned`）。**不传 `origin` 时**：若同时传 `sftJobId` 则视为 `fine_tuned`，否则默认 `external`（用户手工登记）。 |
| 从用户 S3 拉取到平台 | `s3_sync` | 见下节；集群内仍表现为「本地部署」，但列表 `accessMode` 为 `s3_sync`。 |

**统一入口**：`POST /api/v1/playground/models`（与现有「Create Model」一致）。

---

## 2. `local_path`（登记已有 PFS/NFS 路径）

用户模型已在共享存储上时，不触发下载，**立即 Ready**。

**请求体示例**：

```json
{
  "displayName": "my-custom-llm",
  "description": "optional",
  "icon": "https://example.com/icon.png",
  "label": "my-team",
  "tags": ["text-generation"],
  "maxTokens": 8192,
  "source": {
    "accessMode": "local_path",
    "localPath": "/shared_aig/models/my-custom-llm",
    "modelName": "my-team/my-custom-llm",
    "url": "https://huggingface.co/optional/reference"
  },
  "workspace": "",
  "origin": "external"
}
```

**前端建议**：

- 在「Create Model」中增加独立入口，例如 **「Use existing shared path / 使用已有共享路径」**，与 HF 的 `local` 表单**用 Tab 切换**，不要让用户在同一表单里既填 HF URL 又填本地路径。
- 校验：`localPath` 非空；`displayName` 非空。
- **`displayName` 用于生成 K8s 资源名**（前缀），实际命名建议**全小写、不含 `/` 和空格**（例如 `my-custom-llm`，避免 `My Custom/LLM`），否则会被后端拒绝。
- 展示与过滤：列表项 `accessMode` 为 `local_path`；`phase` 一般为 `Ready`。

---

## 3. `s3_sync`（一键：用户 S3 → 平台桶 → 再落 PFS）

**语义**：从用户提供的 `s3://bucket/prefix` 将对象 **同步到平台配置桶** 中该模型的前缀，之后与现有 **local** 模型相同，走 **Uploading → Downloading → Ready** 状态机；推理仍读 PFS 上的文件。

**请求体示例（带源站凭证）**：

```json
{
  "displayName": "imported-model",
  "description": "optional",
  "icon": "https://example.com/icon.png",
  "label": "data-team",
  "tags": ["llm"],
  "source": {
    "accessMode": "s3_sync",
    "modelName": "imported-model"
  },
  "s3Source": {
    "uri": "s3://my-bucket/models/llm-prefix",
    "accessKeyId": "AKIA...",
    "secretAccessKey": "....",
    "region": "us-west-2",
    "endpoint": "https://s3.us-west-2.amazonaws.com"
  },
  "workspace": "",
  "origin": "external"
}
```

**规则**：

- `s3Source.uri`：**必须**以 `s3://` 开头并含非空 bucket，例如 `s3://my-bucket/prefix`。
- `accessKeyId` / `secretAccessKey`：**要么成对出现，要么都省略**（省略时由 Job 使用**平台 S3 凭据**去拉源；仅当源对平台角色可读或公开读时才能成功，否则须填用户密钥）。
- `modelName`：可省略，后端会由 `displayName` 归一化生成。
- 列表/详情中 **`accessMode` 字段返回 `s3_sync`**，前端按字符串识别即可。

**列表筛选**：

- `GET /api/v1/playground/models?accessMode=<value>`，`<value>` 取值：`local | remote_api | local_path | s3_sync`（不传 = 全部）。
- `s3_sync` 与 `local`（HF 下载）在结果中是**互斥两组**，前端按字符串等值过滤即可，无需关心后端如何在 K8s 标签上区分。

**阶段提示**：与 HF 的 `local` 一样，可能经历 `Pending` / `Uploading` / `Downloading` / `Ready` / `Failed`；可复用现有轮询与重试（`POST .../models/:id/retry`）交互。

---

## 4. UI/UX 建议（由产品定稿）

1. **分步或 Tab**：`Hugging Face` | `Remote API` | `Existing path` | `Import from S3`（文案可英中双语）。
2. **S3 表单**：`uri`、可选 AK/SK/Region/Endpoint；敏感字段密码框、禁止写入 localStorage 明文。
3. **帮助文案**：说明数据会进入**平台桶**并再下载到工作区共享存储，与 [playground-models.md](./playground-models.md) 中「local 模型」部署一致。
4. **错误提示**：直接展示后端 `error` 字符串（如重复 `s3` 源、凭据只填一半等）。

---

## 5. 与现有文件的关系

- 请求封装：继续用 `@/services/playground` 中 `createModel` 或等价方法，仅扩展 `payload` 结构。
- 参考页面：`Web/apps/safe/src/pages/ModelSquare/Components/AddModelDialog.vue`（**请在本 worktree/分支外由前端改 Vue**，本需求仅要求行为对齐本文与 OpenAPI/后端）。

---

## 5.1 已知限制

- **`PATCH /api/v1/playground/models/:id`** 当前只支持更新 `displayName`、`description`、`modelName`；**不支持**修改 `icon` / `label` / `tags` / `maxTokens`。如需修改这些字段，需删除后重建模型；如必要，可单独提需求扩展 `PatchModelRequest`。

---

## 6. 联调检查清单

- [ ] `local_path` 创建后卡片展示 `icon` / `label` / `tags` / `maxTokens` 与预期一致；`origin` 不传时默认 `external`。
- [ ] `s3_sync` 创建后 `accessMode` 为 `s3_sync`，阶段从 Pending → Uploading → Downloading → Ready（依赖集群 S3/Job 配置）。
- [ ] `s3_sync` 处于 Failed 时，`POST /models/:id/retry` 能正常重试（与 HF `local` 行为一致）。
- [ ] 列表 `accessMode=s3_sync` 与 `local` / `local_path` / `remote_api` 互不重叠。
- [ ] 删除 S3 导入模型后，K8s 中的 user-S3 source Secret 与平台桶中的 prefix 都被清理（由后端处理；前端只调删除 API）。

---

## 7. 相关文档

- 通用 API：[playground-models.md](./playground-models.md)  
- 运营侧登记与路径说明：团队内 `model-square-ops` skill / 运维文档（若有）。
