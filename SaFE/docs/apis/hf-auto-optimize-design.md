# HuggingFace Top-N 模型自动优化方案

## 1. 目标

手动触发 CI，输入一个数字（如 10、20），自动完成：
1. 从 HuggingFace 拉取下载量 Top-N、参数量 ≥ 指定阈值的模型列表
2. 自动推断每个模型的 framework / precision / TP / concurrency
3. 在 SaFE 中注册模型（已有则跳过）
4. 批量提交 optimization task

---

## 2. 整体架构

```
GitHub Actions (Hyperloom repo)
  └── hf-top-models-optimize.yml       ← 手动触发，输入 model_count / min_params
        ├── prepare job
        │     └── hf_model_submit.py --discover   ← 查 HF + 查 SaFE，输出 matrix JSON
        └── optimize job (matrix, 并行)
              └── hf_model_submit.py --submit      ← 注册模型 + 提交任务 + 等待完成

SaFE API (Primus-SaFE repo)               ← 只提供 REST 接口，不做改动
  ├── POST /api/v1/playground/models       ← 注册/下载模型
  ├── GET  /api/v1/playground/models?sourceUrl=  ← 查重（需新增 sourceUrl filter）
  ├── GET  /api/v1/playground/models/:id   ← 轮询 phase=Ready
  └── POST /api/v1/optimization/tasks/batch ← 批量提交优化任务
```

---

## 3. Hyperloom 侧改动

### 3.1 新增 workflow：`.github/workflows/hf-top-models-optimize.yml`

```yaml
on:
  workflow_dispatch:
    inputs:
      model_count:
        description: 'Number of top HF models to optimize'
        required: true
        default: '10'
      min_params:
        description: 'Minimum model size in billions (e.g. 7)'
        required: false
        default: '7'
      dry_run:
        description: 'Dry run (discover only, do not submit)'
        required: false
        default: 'false'
```

两阶段：
- **prepare**：运行 `hf_model_submit.py --discover` 输出 GitHub Actions matrix
- **optimize**：matrix 并行，每个 job 运行 `hf_model_submit.py --submit <repo_id>`

### 3.2 新增脚本：`ci/hf_model_submit.py`

从 `SaFE/scripts/optimize_submit.py` 迁移核心逻辑，整合进 Hyperloom ci/ 风格：

| 模块 | 来源 | 说明 |
|---|---|---|
| HF 元数据拉取 | optimize_submit.py | fetch_hf_model_info / fetch_hf_config |
| framework 自动检测 | optimize_submit.py | SGLANG_ARCHS / VLLM_REQUIRED_ARCHS |
| TP / precision 检测 | optimize_submit.py | detect_tp / detect_precision |
| SaFE 模型注册 | optimize_submit.py | find_safe_model / register_model / wait_for_model_ready |
| 任务提交 | optimize_submit.py | submit_task → POST /optimization/tasks |
| 结果轮询 | **新增** | 轮询 task status，下载 ci_metrics.json，生成报告 |
| 报告生成 | 复用 report_generator.py | 和现有 CI 格式一致 |

Secrets 复用现有的：`CLAW_API_KEY`（即 SAFE_API_KEY）、`SANDBOX_WORKSPACE`、`HF_TOKEN`

---

## 4. SaFE 侧改动

### 4.1 已完成
- `gpuType` 从 workspace annotation 自动读取（`primus-safe.gpu.product`），不再 hardcode
- `DeepseekV32ForCausalLM` 加入 SGLANG_ARCHS（在 optimize_submit.py 中）

### 4.2 需要新增：`GET /api/v1/playground/models?sourceUrl=` 过滤

**现状**：`ListModelQuery` 没有 `sourceUrl` 字段，`hf_model_submit.py` 需要拉全量列表再客户端过滤，模型量大时不高效。

**改动**：在 `types.go` 的 `ListModelQuery` 中加 `SourceURL` 字段，在 `ListModels` handler 中加过滤逻辑。

```go
// types.go
type ListModelQuery struct {
    ...
    SourceURL string `form:"sourceUrl" binding:"omitempty"` // Filter by HF source URL
}
```

这样 `hf_model_submit.py` 可以直接：
```
GET /api/v1/playground/models?sourceUrl=https://huggingface.co/Qwen/Qwen3-8B
```
精确查重，不需要拉 200 条再遍历。

### 4.3 不需要改动
- `POST /api/v1/playground/models`：注册模型接口已有重复检测（`findModelBySourceURL`），重复注册会返回现有 model id 的错误信息，客户端解析即可
- `POST /api/v1/optimization/tasks/batch`：批量提交已支持
- `GET /api/v1/playground/models/:id`：轮询接口已支持

---

## 5. 数据流

```
用户触发 workflow_dispatch
  │  输入: model_count=10, min_params=7
  ▼
hf_model_submit.py --discover
  ├── 调 HF API 拉 Top-N（过滤 <7B）
  ├── 对每个 repo 调 fetch_hf_config → 检测 arch/framework/TP/precision
  ├── 调 GET /playground/models?sourceUrl= → 判断是否已在 SaFE
  └── 输出 matrix: [{repo, framework, tp, precision, model_id(或空)}]

并行 optimize jobs（每个 repo 一个 job）
  ├── 若 model_id 为空 → POST /playground/models 注册 → 轮询 phase=Ready
  ├── POST /optimization/tasks 提交任务
  ├── 轮询 task status 直到 Succeeded/Failed
  ├── 下载 ci_metrics.json
  └── 生成报告（复用 report_generator.py）
```

---

## 6. 工作量估算

| 任务 | 位置 | 工作量 |
|---|---|---|
| `hf-top-models-optimize.yml` | Hyperloom | 0.5d |
| `ci/hf_model_submit.py` | Hyperloom | 1d |
| `ListModels` 加 sourceUrl 过滤 | SaFE | 0.5d |
| 联调测试 | - | 0.5d |
| **合计** | | **~2.5d** |

---

## 7. 后续扩展（不在本期）

- 自动过滤已有优化结果的模型（避免重复跑）
- 按 gain 排序自动生成周报
- 接入 InferenceX baseline 对比
