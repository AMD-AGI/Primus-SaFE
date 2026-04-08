# RL Training UI 方案

## 概述

在 SaFE 平台 Model Square 中增加 RL（强化学习）训练入口，用户可对已有模型发起 GRPO/PPO 训练。整体交互模式复用 SFT 的 Dialog 表单模式，后端通过 RayJob 调度 verl 训练框架。

## 已验证的后端能力

| 场景 | 训练 | HF 导出 | val acc | 耗时 |
|------|------|---------|---------|------|
| FSDP2 单机 | ✅ | ✅ 16GB safetensors | ~32% | 30min |
| FSDP2 多机 | ✅ | ✅ 16GB safetensors | 32% | 4.5h |
| Megatron 单机 | ✅ | ✅ 16GB safetensors | — | ~1h |
| Megatron 多机 | ✅ | ✅ 16.4GB safetensors | 91.1% | 58min |

- **FSDP2**：适合 8B-32B 模型，单机效率高，多机通信开销大
- **Megatron**：适合 32B+ 模型，多机效率远高于 FSDP2（TP/PP 切分 vs 全模型 all-gather）

## 1. 文件结构

```
Web/apps/safe/src/
├── pages/ModelSquare/Components/
│   ├── CreateSftDialog.vue        # 已有 — SFT 训练表单
│   └── CreateRlDialog.vue         # 新增 — RL 训练表单
├── services/
│   ├── sft/                       # 已有
│   └── rl/                        # 新增
│       ├── index.ts               # getRlConfig / createRlJob API 封装
│       └── types.ts               # RlConfigResponse / CreateRlJobRequest 类型
```

## 2. API 端点

### 2.1 获取 RL 配置

```
GET /api/v1/playground/models/{modelId}/rl-config?workspace={workspaceId}&strategy={strategy}
```

**响应**（已在 `rl.go` 中实现）：

```json
{
  "supported": true,
  "defaults": {
    "image": "harbor.xxx/proxy/primussafe/verl:0.8.0.dev-fsdp-sglang-rocm700-mi35x",
    "nodeCount": 2,
    "gpuCount": 8,
    "cpu": "128",
    "memory": "2048Gi",
    "sharedMemory": "1Ti",
    "ephemeralStorage": "500Gi",
    "trainConfig": {
      "strategy": "fsdp2",
      "algorithm": "grpo",
      "rewardType": "math",
      "trainBatchSize": 128,
      "maxPromptLength": 512,
      "maxResponseLength": 512,
      "actorLr": 1e-6,
      "miniPatchSize": 64,
      "microBatchSizePerGpu": 4,
      "rolloutN": 5,
      "rolloutTpSize": 8,
      "rolloutGpuMemory": 0.4,
      "totalEpochs": 2,
      "saveFreq": 100,
      "testFreq": 5,
      "klLossCoef": 0.001,
      "gradClip": 1.0
    }
  },
  "options": {
    "strategyOptions": ["fsdp2", "megatron"],
    "algorithmOptions": ["grpo", "ppo"],
    "rewardTypeOptions": ["math"],
    "priorityOptions": [0, 1, 2]
  },
  "datasetFilter": {
    "datasetType": "rlhf",
    "workspace": "current-workspace"
  }
}
```

当 `strategy=megatron` 时，defaults 会自动切换为 Megatron 参数：

```json
{
  "trainConfig": {
    "strategy": "megatron",
    "megatronTpSize": 4,
    "megatronPpSize": 8,
    "megatronCpSize": 1,
    "paramOffload": true,
    "gradOffload": true,
    "rolloutGpuMemory": 0.85
  }
}
```

### 2.2 创建 RL 任务

```
POST /api/v1/rl/jobs
Content-Type: application/json
```

**请求体**：

```json
{
  "displayName": "qwen3-8b-rl-grpo",
  "modelId": "model-xxx",
  "datasetId": "dataset-yyy",
  "workspace": "control-plane-anthropic",
  "exportModel": true,
  "priority": 1,
  "timeout": 21600,
  "image": "",
  "nodeCount": 2,
  "gpuCount": 8,
  "cpu": "128",
  "memory": "2048Gi",
  "sharedMemory": "1Ti",
  "ephemeralStorage": "500Gi",
  "trainConfig": {
    "strategy": "fsdp2",
    "algorithm": "grpo",
    "rewardType": "math",
    "trainBatchSize": 256,
    "totalEpochs": 2,
    "actorLr": 1e-6
  }
}
```

**响应**：

```json
{
  "workloadId": "chenyi-rl-grpo-xxxxx",
  "message": "RL training job created"
}
```

### 2.3 数据集列表（复用）

```
GET /api/v1/datasets?datasetType=rlhf&workspace={workspaceId}
```

## 3. 类型定义

### `services/rl/types.ts`

```typescript
// RL 配置响应
export interface RlConfigResponse {
  supported: boolean
  reason?: string
  defaults: RlConfigDefaults
  options: RlConfigOptions
  datasetFilter: {
    datasetType: string
    workspace: string
  }
}

export interface RlConfigDefaults {
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  sharedMemory: string
  ephemeralStorage: string
  trainConfig: RlTrainConfig
}

export interface RlTrainConfig {
  strategy: 'fsdp2' | 'megatron'
  algorithm: 'grpo' | 'ppo'
  rewardType: 'math' | 'custom'
  trainBatchSize: number
  maxPromptLength: number
  maxResponseLength: number
  actorLr: number
  miniPatchSize: number
  microBatchSizePerGpu: number
  rolloutN: number
  rolloutTpSize: number
  rolloutGpuMemory: number
  totalEpochs: number
  saveFreq: number
  testFreq: number
  klLossCoef: number
  gradClip: number
  // FSDP2 specific
  paramOffload?: boolean
  optimizerOffload?: boolean
  gradientCheckpointing?: boolean
  useTorchCompile?: boolean
  // Megatron specific
  megatronTpSize?: number
  megatronPpSize?: number
  megatronCpSize?: number
  gradOffload?: boolean
}

export interface RlConfigOptions {
  strategyOptions: string[]
  algorithmOptions: string[]
  rewardTypeOptions: string[]
  priorityOptions: number[]
}

// 创建 RL 任务请求
export interface CreateRlJobRequest {
  displayName: string
  modelId: string
  datasetId: string
  workspace: string
  exportModel: boolean
  priority: number
  timeout: number
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  sharedMemory: string
  ephemeralStorage: string
  trainConfig: RlTrainConfig
}

// 创建 RL 任务响应
export interface CreateRlJobResponse {
  workloadId: string
  message: string
}
```

### `services/rl/index.ts`

```typescript
import { request } from '@/utils/request'
import type {
  RlConfigResponse,
  CreateRlJobRequest,
  CreateRlJobResponse,
} from './types'

export function getRlConfig(modelId: string, params: {
  workspace: string
  strategy?: string
}) {
  return request.get<RlConfigResponse>(
    `/playground/models/${modelId}/rl-config`,
    { params }
  )
}

export function createRlJob(data: CreateRlJobRequest) {
  return request.post<CreateRlJobResponse>('/rl/jobs', data, {
    timeout: 60000,
  })
}

export * from './types'
```

## 4. 表单布局

### 4.1 整体结构（参照 SFT Dialog）

```
┌─────────────────────────────────────────────────┐
│  RL Training                               [×]  │
├─────────────────────────────────────────────────┤
│                                                 │
│  📦 Base Model                                  │
│  ┌─────────────────────────────────────┐        │
│  │ [icon] Qwen3-8B                     │        │
│  │ ID: model-xxx  Status: Ready        │        │
│  └─────────────────────────────────────┘        │
│                                                 │
│  ⚙️ Training Strategy                           │
│  ○ FSDP2 (推荐 8B-32B)                          │
│  ○ Megatron (推荐 32B+, 多机效率更高)             │
│                                                 │
│  📊 Dataset                                     │
│  [ Select RLHF Dataset          ▼ ]             │
│                                                 │
│  🎯 Training Configuration                      │
│  ┌────────────────┬────────────────┐            │
│  │ Algorithm       │ Reward Type    │            │
│  │ [ GRPO    ▼ ]  │ [ Math    ▼ ]  │            │
│  ├────────────────┼────────────────┤            │
│  │ Batch Size      │ Epochs         │            │
│  │ [ 256      ]   │ [ 2        ]   │            │
│  ├────────────────┼────────────────┤            │
│  │ Learning Rate   │ Grad Clip      │            │
│  │ [ 1e-6     ]   │ [ 1.0      ]   │            │
│  ├────────────────┼────────────────┤            │
│  │ KL Loss Coef   │ Rollout N      │            │
│  │ [ 0.001    ]   │ [ 5        ]   │            │
│  └────────────────┴────────────────┘            │
│                                                 │
│  ▶ Megatron Settings (仅 strategy=megatron)     │
│  ┌────────────────┬──────────┬──────────┐       │
│  │ TP Size         │ PP Size  │ CP Size  │       │
│  │ [ 4        ]   │ [ 8    ] │ [ 1    ] │       │
│  ├────────────────┼──────────┴──────────┤       │
│  │ Param Offload   │ Grad Offload       │       │
│  │ [ ✓ ]          │ [ ✓ ]              │       │
│  └────────────────┴─────────────────────┘       │
│                                                 │
│  💻 Resources                                   │
│  ┌────────────────┬────────────────┐            │
│  │ Nodes           │ GPUs per Node  │            │
│  │ [ 2        ]   │ [ 8        ]   │            │
│  ├────────────────┼────────────────┤            │
│  │ CPU              │ Memory (Gi)   │            │
│  │ [ 128      ]   │ [ 2048     ]   │            │
│  └────────────────┴────────────────┘            │
│  Image: [auto-filled, editable]                 │
│                                                 │
│  📤 Output                                      │
│  Job Name: [ qwen3-8b-rl-grpo          ]       │
│  Export Model: [ ✓ ]                            │
│  ℹ️ 勾选后训练完成自动导出 HuggingFace 格式       │
│     并注册到模型广场                              │
│                                                 │
│  ▶ Advanced Settings                            │
│  ┌────────────────┬────────────────┐            │
│  │ Priority        │ Timeout (s)    │            │
│  │ [ Medium  ▼ ]  │ [ 21600    ]   │            │
│  ├────────────────┼────────────────┤            │
│  │ Save Freq       │ Test Freq      │            │
│  │ [ 100      ]   │ [ 5        ]   │            │
│  ├────────────────┼────────────────┤            │
│  │ Max Prompt Len  │ Max Response   │            │
│  │ [ 512      ]   │ [ 512      ]   │            │
│  ├────────────────┼────────────────┤            │
│  │ Micro Batch/GPU │ Rollout TP     │            │
│  │ [ 4        ]   │ [ 8        ]   │            │
│  └────────────────┴────────────────┘            │
│                                                 │
│              [ Cancel ]  [ Create RL Job ]       │
└─────────────────────────────────────────────────┘
```

### 4.2 Strategy 切换逻辑

用户切换 Strategy 时，联动以下字段：

| 字段 | FSDP2 默认 | Megatron 默认 |
|------|-----------|--------------|
| Image | `verl:fsdp-sglang-rocm700` | `verl:megatron-sglang-rocm700` |
| Rollout GPU Memory | 0.4 | 0.85 |
| Megatron TP/PP/CP | 隐藏 | 显示（4/8/1） |
| Param Offload | ✓ | ✓ |
| Grad Offload | — | ✓ |
| Optimizer Offload | ✓ | — |

### 4.3 Export Model 说明

| Export Model | 训练后行为 |
|-------------|----------|
| **勾选** | 训练 → 保存 checkpoint → **转换为 HuggingFace safetensors** → 注册模型广场 |
| **不勾选** | 训练 → 保存 checkpoint（原生格式，可用于恢复训练） |

转换方案：
- FSDP2：fake process group 加载 DTensor shards → merge → safetensors
- Megatron：`dcp_to_torch_save` 加载 DCP → 手动映射 Megatron keys → HF keys → safetensors

## 5. 关键交互逻辑

### 5.1 Dialog 生命周期

```
Dialog visible
  │
  ├─ loadRlConfig(modelId, workspace)
  │    ├─ supported=false → 显示不支持原因，禁用提交
  │    └─ supported=true
  │         ├─ 用 defaults 填充表单
  │         └─ loadDatasets(datasetFilter)
  │
  ├─ 用户切换 Strategy
  │    └─ 重新 loadRlConfig(modelId, workspace, strategy)
  │         └─ 更新 strategy-specific 字段（保留用户已改的通用字段）
  │
  ├─ 用户点击 Create
  │    ├─ 表单校验
  │    ├─ createRlJob(formData)
  │    ├─ 成功 → emit('success', workloadId) → 跳转训练详情
  │    └─ 失败 → 显示错误
  │
  └─ 用户点击 Cancel / 关闭
       └─ 重置表单和本地状态
```

### 5.2 表单校验规则

```typescript
const formRules = {
  displayName: [{ required: true, message: 'Job name is required' }],
  datasetId: [{ required: true, message: 'Dataset is required' }],
  'trainConfig.strategy': [{ required: true }],
  'trainConfig.algorithm': [{ required: true }],
  nodeCount: [{ required: true, type: 'number', min: 1 }],
  gpuCount: [{ required: true, type: 'number', min: 1 }],
}
```

## 6. 与 SFT 的对比

| 维度 | SFT | RL |
|------|-----|-----|
| 入口 | Model Square 模型卡片 "SFT" 按钮 | Model Square 模型卡片 **"RL"** 按钮 |
| 弹窗 | `CreateSftDialog.vue` | `CreateRlDialog.vue` |
| Workload 类型 | PyTorchJob | **RayJob** |
| 配置 API | `GET .../sft-config` | `GET .../rl-config` |
| 创建 API | `POST /sft/jobs` | `POST /rl/jobs` |
| Strategy | 无（统一 Megatron） | **FSDP2 / Megatron** 可选 |
| Dataset Type | `sft` | **`rlhf`** |
| 特有参数 | LoRA dim/alpha, peft | **Rollout N/TP, KL loss, Reward type, Algorithm** |
| 导出 | 固定导出 HF 模型 | **可选** 导出 HF 模型 |
| 镜像 | 固定 Primus 镜像 | **根据 Strategy 自动切换** verl 镜像 |

## 7. 开发计划

| 步骤 | 内容 | 估时 |
|------|------|------|
| 1 | `services/rl/types.ts` — 类型定义 | 0.5h |
| 2 | `services/rl/index.ts` — API 封装 | 0.5h |
| 3 | `CreateRlDialog.vue` — 复制 SFT Dialog 改造 | 3h |
| 4 | `ModelSquare/index.vue` — 加 RL 按钮 + `canRl()` | 0.5h |
| 5 | `ModelSquareDetail.vue` — 详情页加 RL 入口 | 0.5h |
| 6 | 联调测试 | 1h |
| **总计** | | **~6h** |

## 8. 后端已完成

- [x] `rl.go` — API handler（getRlConfig + createRlJob）
- [x] `rl_types.go` — 请求/响应类型定义
- [x] `rl_entrypoint_builder.go` — 动态生成训练脚本 + HF 导出
- [x] `routers.go` — 路由注册（`/rl/jobs`, `/rl-config`）
- [x] `dataset_constant.go` — RLHF 数据集类型定义
- [x] FSDP2 导出：fake process group → merge DTensor → safetensors
- [x] Megatron 导出：DCP → dcp_to_torch_save → 手动 key mapping → safetensors
