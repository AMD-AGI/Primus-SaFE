# Training UI 前端开发文档

## 1. 交互设计

### 1.1 入口

Model Square 模型卡片上**一个按钮** "Train"（替代当前单独的 SFT 按钮），点击后弹出 Dialog。

```
模型卡片
┌──────────────────────┐
│ Qwen3-8B             │
│ Ready | local_path   │
│                      │
│   [Deploy] [Train]   │  ← 统一入口
└──────────────────────┘
```

### 1.2 Dialog 流程

```
点击 Train
  │
  ▼
┌─────────────────────────────────────────┐
│  Training                          [×]  │
│                                         │
│  📦 Base Model: Qwen3-8B (只读)        │
│                                         │
│  🎯 Training Type                       │
│  ┌─────────┐  ┌─────────┐              │
│  │   SFT   │  │   RL    │   ← Tab 切换 │
│  └─────────┘  └─────────┘              │
│                                         │
│  ⚙️ Strategy                            │
│  (SFT) [ Full ▼ ] [ LoRA ▼ ]           │
│  (RL)  [ FSDP2 ▼ ] [ Megatron ▼ ]      │
│         ↓ 切换时 GET config 刷新参数     │
│                                         │
│  📊 Dataset   [ Select... ▼ ]           │
│                                         │
│  🔧 Training Parameters                │
│  (根据 type + strategy 动态渲染)         │
│                                         │
│  💻 Resources                           │
│  📤 Output                              │
│  ▶ Advanced                             │
│                                         │
│          [Cancel]  [Create Job]          │
└─────────────────────────────────────────┘
```

### 1.3 关键交互

| 操作 | 触发 | 效果 |
|------|------|------|
| 切换 SFT ↔ RL | Tab click | 重新 `GET` 对应 config API，刷新整个表单 |
| SFT: 切换 Full ↔ LoRA | Select change | 重新 `GET sft-config?peft=xxx`，刷新 trainConfig |
| RL: 切换 FSDP2 ↔ Megatron | Select change | 重新 `GET rl-config?strategy=xxx`，刷新 trainConfig + image |
| 提交 | Button click | `POST /sft/jobs` 或 `POST /rl/jobs` |

## 2. 文件结构

```
Web/apps/safe/src/
├── pages/ModelSquare/Components/
│   ├── CreateSftDialog.vue           # 已有，保留不动
│   └── CreateTrainingDialog.vue      # 新增 — 统一训练入口
├── services/
│   ├── sft/
│   │   ├── index.ts                  # 已有
│   │   └── types.ts                  # 已有
│   └── rl/
│       ├── index.ts                  # 新增
│       └── types.ts                  # 新增
```

> **`CreateSftDialog.vue` 保留不删**，`CreateTrainingDialog.vue` 内部按 tab 选择后，
> SFT 分支可以直接复用 `CreateSftDialog` 的逻辑（或内联），RL 分支用新逻辑。

## 3. API 端点汇总

### 3.1 SFT（已有）

| 用途 | 方法 | 路径 | 切换时机 |
|------|------|------|---------|
| 获取配置 | GET | `/playground/models/{id}/sft-config?workspace=xx` | 选 SFT tab / 切 peft |
| 创建任务 | POST | `/sft/jobs` | 提交 |
| 数据集列表 | GET | `/datasets?datasetType=sft&workspace=xx` | 配置加载后 |

### 3.2 RL（新增）

| 用途 | 方法 | 路径 | 切换时机 |
|------|------|------|---------|
| 获取配置 | GET | `/playground/models/{id}/rl-config?workspace=xx&strategy=fsdp2` | 选 RL tab / 切 strategy |
| 创建任务 | POST | `/rl/jobs` | 提交 |
| 数据集列表 | GET | `/datasets?datasetType=rlhf&workspace=xx` | 配置加载后 |

## 4. 类型定义

### 4.1 `services/rl/types.ts`

```typescript
export interface RlConfigResponse {
  supported: boolean
  reason?: string
  model: {
    id: string
    displayName: string
    modelName: string
    accessMode: string
    phase: string
    workspace: string
  }
  datasetFilter: {
    datasetType: string
    workspace: string
    status: string
  }
  defaults?: RlConfigDefaults
  options: RlConfigOptions
}

export interface RlConfigDefaults {
  exportModel: boolean
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  sharedMemory: string
  ephemeralStorage: string
  priority: number
  trainConfig: RlTrainConfig
}

export interface RlTrainConfig {
  algorithm: string       // 'grpo' | 'ppo'
  strategy: string        // 'fsdp2' | 'megatron'
  rewardType: string      // 'math' | 'custom'
  // Data
  trainBatchSize: number
  maxPromptLength: number
  maxResponseLength: number
  // Actor
  actorLr: number
  miniPatchSize: number
  microBatchSizePerGpu: number
  gradClip: number
  // FSDP2
  paramOffload: boolean
  optimizerOffload: boolean
  gradientCheckpointing: boolean
  useTorchCompile: boolean
  // Megatron
  megatronTpSize: number
  megatronPpSize: number
  megatronCpSize: number
  megatronEpSize: number
  gradOffload: boolean
  // KL
  useKlLoss: boolean
  klLossCoef: number
  // Rollout
  rolloutN: number
  rolloutTpSize: number
  rolloutGpuMemory: number
  // Ref
  refParamOffload: boolean
  refReshardAfterForward: boolean
  // Schedule
  totalEpochs: number
  saveFreq: number
  testFreq: number
}

export interface RlConfigOptions {
  algorithmOptions: string[]
  strategyOptions: string[]
  rewardTypeOptions: string[]
  priorityOptions: Array<{ label: string; value: number }>
}

export interface CreateRlJobRequest {
  displayName: string
  modelId: string
  datasetId: string
  workspace: string
  exportModel: boolean
  image: string
  nodeCount: number
  gpuCount: number
  cpu: string
  memory: string
  sharedMemory: string
  ephemeralStorage: string
  priority: number
  timeout?: number
  trainConfig: RlTrainConfig
}

export interface CreateRlJobResponse {
  workloadId: string
}
```

### 4.2 `services/rl/index.ts`

```typescript
import request from '@/services/request'
import type { RlConfigResponse, CreateRlJobRequest, CreateRlJobResponse } from './types'

export * from './types'

export function getRlConfig(modelId: string, params: { workspace: string; strategy?: string }) {
  return request.get<RlConfigResponse>(`/playground/models/${modelId}/rl-config`, { params })
}

export function createRlJob(data: CreateRlJobRequest) {
  return request.post<CreateRlJobResponse>('/rl/jobs', data, { timeout: 60000 })
}
```

## 5. 核心组件 `CreateTrainingDialog.vue`

### 5.1 Props & Emits

```typescript
const props = defineProps<{
  visible: boolean
  model: PlaygroundModel | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'success', workloadId: string): void
}>()
```

### 5.2 核心状态

```typescript
// 训练类型：sft | rl
const trainingType = ref<'sft' | 'rl'>('sft')

// SFT 配置 & 表单
const sftConfig = ref<SftConfigResponse | null>(null)
const sftForm = reactive({ /* 同 CreateSftDialog 的 form */ })

// RL 配置 & 表单
const rlConfig = ref<RlConfigResponse | null>(null)
const rlForm = reactive({
  displayName: '',
  datasetId: '',
  exportModel: true,
  image: '',
  nodeCount: 2,
  gpuCount: 8,
  cpu: '128',
  memory: '2048',
  sharedMemory: '1024',
  ephemeralStorage: '500',
  priority: 1,
  trainConfig: { /* RlTrainConfig defaults */ },
})

// 公共
const datasets = ref<DatasetItem[]>([])
const configLoading = ref(false)
const submitting = ref(false)
```

### 5.3 Config 加载逻辑

```typescript
// Dialog 打开时
watch(() => props.visible, async (val) => {
  if (val && props.model) {
    await loadConfig()
  }
})

// 切换 SFT ↔ RL 时
watch(trainingType, async () => {
  await loadConfig()
})

// RL: 切换 strategy 时
watch(() => rlForm.trainConfig.strategy, async (newStrategy) => {
  if (trainingType.value === 'rl') {
    await loadRlConfig(newStrategy)
  }
})

// SFT: 切换 peft 时（如果后端支持按 peft 返回不同配置）
// 当前 SFT 的 peft 切换只是前端显隐 LoRA 字段，不重新 GET

async function loadConfig() {
  if (trainingType.value === 'sft') {
    await loadSftConfig()
  } else {
    await loadRlConfig()
  }
}

async function loadSftConfig() {
  configLoading.value = true
  try {
    const res = await getSftConfig(props.model!.id, workspace)
    sftConfig.value = res
    if (res.supported) {
      Object.assign(sftForm, res.defaults)        // 用 defaults 填表单
      await loadDatasets(res.datasetFilter)
    }
  } finally {
    configLoading.value = false
  }
}

async function loadRlConfig(strategy?: string) {
  configLoading.value = true
  try {
    const res = await getRlConfig(props.model!.id, {
      workspace,
      strategy: strategy || rlForm.trainConfig.strategy,
    })
    rlConfig.value = res
    if (res.supported && res.defaults) {
      // 保留用户已修改的 displayName/datasetId，刷新 strategy-specific 字段
      rlForm.image = res.defaults.image
      rlForm.nodeCount = res.defaults.nodeCount
      rlForm.gpuCount = res.defaults.gpuCount
      rlForm.cpu = res.defaults.cpu
      rlForm.memory = res.defaults.memory.replace(/Gi$/i, '')
      rlForm.sharedMemory = res.defaults.sharedMemory.replace(/[TGi]+$/i, '')
      rlForm.ephemeralStorage = res.defaults.ephemeralStorage.replace(/Gi$/i, '')
      rlForm.trainConfig = { ...res.defaults.trainConfig }
      await loadDatasets(res.datasetFilter)
    }
  } finally {
    configLoading.value = false
  }
}
```

### 5.4 表单模板结构

```html
<template>
  <el-dialog v-model="dialogVisible" title="Training" width="720px">
    <!-- Base Model (只读) -->
    <ModelCard :model="model" />

    <!-- Training Type Tab -->
    <el-radio-group v-model="trainingType" class="mb-4">
      <el-radio-button value="sft">SFT</el-radio-button>
      <el-radio-button value="rl">RL</el-radio-button>
    </el-radio-group>

    <!-- ============ SFT Form ============ -->
    <el-form v-if="trainingType === 'sft'" ref="sftFormRef" :model="sftForm" :rules="sftRules">

      <!-- Strategy: Full / LoRA -->
      <el-form-item label="PEFT">
        <el-select v-model="sftForm.trainConfig.peft">
          <el-option v-for="opt in sftConfig?.options.peftOptions" :key="opt" :label="opt" :value="opt" />
        </el-select>
      </el-form-item>

      <!-- Dataset -->
      <el-form-item label="Dataset">
        <el-select v-model="sftForm.datasetId" filterable>
          <el-option v-for="d in datasets" :key="d.id" :label="d.displayName" :value="d.id" />
        </el-select>
      </el-form-item>

      <!-- SFT Training Params -->
      <el-row :gutter="20">
        <el-col :span="12"><el-form-item label="Train Iters"><el-input-number v-model="sftForm.trainConfig.trainIters" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="Batch Size"><el-input-number v-model="sftForm.trainConfig.globalBatchSize" /></el-form-item></el-col>
      </el-row>
      <!-- ... 更多 SFT 参数 ... -->

      <!-- LoRA 特有参数 -->
      <template v-if="sftForm.trainConfig.peft === 'lora'">
        <el-row :gutter="20">
          <el-col :span="12"><el-form-item label="LoRA Dim"><el-input-number v-model="sftForm.trainConfig.peftDim" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="LoRA Alpha"><el-input-number v-model="sftForm.trainConfig.peftAlpha" /></el-form-item></el-col>
        </el-row>
      </template>

      <!-- Resources / Output / Advanced — 同现有 SFT -->
    </el-form>

    <!-- ============ RL Form ============ -->
    <el-form v-if="trainingType === 'rl'" ref="rlFormRef" :model="rlForm" :rules="rlRules">

      <!-- Strategy: FSDP2 / Megatron -->
      <el-form-item label="Strategy">
        <el-radio-group v-model="rlForm.trainConfig.strategy">
          <el-radio-button v-for="s in rlConfig?.options.strategyOptions" :key="s" :value="s">
            {{ s === 'fsdp2' ? 'FSDP2 (8B-32B)' : 'Megatron (32B+)' }}
          </el-radio-button>
        </el-radio-group>
      </el-form-item>

      <!-- Dataset -->
      <el-form-item label="Dataset">
        <el-select v-model="rlForm.datasetId" filterable>
          <el-option v-for="d in datasets" :key="d.id" :label="d.displayName" :value="d.id" />
        </el-select>
      </el-form-item>

      <!-- RL Training Params -->
      <el-row :gutter="20">
        <el-col :span="12"><el-form-item label="Algorithm">
          <el-select v-model="rlForm.trainConfig.algorithm">
            <el-option v-for="a in rlConfig?.options.algorithmOptions" :key="a" :label="a.toUpperCase()" :value="a" />
          </el-select>
        </el-form-item></el-col>
        <el-col :span="12"><el-form-item label="Reward Type">
          <el-select v-model="rlForm.trainConfig.rewardType">
            <el-option v-for="r in rlConfig?.options.rewardTypeOptions" :key="r" :label="r" :value="r" />
          </el-select>
        </el-form-item></el-col>
      </el-row>
      <el-row :gutter="20">
        <el-col :span="12"><el-form-item label="Batch Size"><el-input-number v-model="rlForm.trainConfig.trainBatchSize" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="Epochs"><el-input-number v-model="rlForm.trainConfig.totalEpochs" /></el-form-item></el-col>
      </el-row>
      <el-row :gutter="20">
        <el-col :span="12"><el-form-item label="Learning Rate"><el-input-number v-model="rlForm.trainConfig.actorLr" :precision="8" :step="0.000001" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="KL Loss Coef"><el-input-number v-model="rlForm.trainConfig.klLossCoef" :precision="4" :step="0.001" /></el-form-item></el-col>
      </el-row>

      <!-- Megatron 特有参数 -->
      <template v-if="rlForm.trainConfig.strategy === 'megatron'">
        <el-divider content-position="left">Megatron Settings</el-divider>
        <el-row :gutter="20">
          <el-col :span="8"><el-form-item label="TP Size"><el-input-number v-model="rlForm.trainConfig.megatronTpSize" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="PP Size"><el-input-number v-model="rlForm.trainConfig.megatronPpSize" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="CP Size"><el-input-number v-model="rlForm.trainConfig.megatronCpSize" /></el-form-item></el-col>
        </el-row>
      </template>

      <!-- Resources -->
      <el-divider content-position="left">Resources</el-divider>
      <el-row :gutter="20">
        <el-col :span="12"><el-form-item label="Nodes"><el-input-number v-model="rlForm.nodeCount" :min="1" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="GPUs/Node"><el-input-number v-model="rlForm.gpuCount" :min="1" /></el-form-item></el-col>
      </el-row>

      <!-- Output -->
      <el-divider content-position="left">Output</el-divider>
      <el-form-item label="Job Name"><el-input v-model="rlForm.displayName" /></el-form-item>
      <el-form-item label="Export Model">
        <el-switch v-model="rlForm.exportModel" />
        <span class="text-gray-400 ml-2 text-sm">导出 HuggingFace 格式并注册模型广场</span>
      </el-form-item>

      <!-- Advanced (collapsible) -->
      <el-collapse>
        <el-collapse-item title="Advanced Settings">
          <el-row :gutter="20">
            <el-col :span="12"><el-form-item label="Save Freq"><el-input-number v-model="rlForm.trainConfig.saveFreq" /></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="Test Freq"><el-input-number v-model="rlForm.trainConfig.testFreq" /></el-form-item></el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :span="12"><el-form-item label="Rollout N"><el-input-number v-model="rlForm.trainConfig.rolloutN" /></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="Rollout TP"><el-input-number v-model="rlForm.trainConfig.rolloutTpSize" /></el-form-item></el-col>
          </el-row>
          <el-form-item label="Image"><el-input v-model="rlForm.image" /></el-form-item>
        </el-collapse-item>
      </el-collapse>
    </el-form>

    <!-- Footer -->
    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        {{ trainingType === 'sft' ? 'Create SFT Job' : 'Create RL Job' }}
      </el-button>
    </template>
  </el-dialog>
</template>
```

### 5.5 提交逻辑

```typescript
async function handleSubmit() {
  if (trainingType.value === 'sft') {
    await submitSftJob()
  } else {
    await submitRlJob()
  }
}

async function submitSftJob() {
  await sftFormRef.value?.validate()
  submitting.value = true
  try {
    const res = await createSftJob({
      displayName: sftForm.displayName,
      modelId: props.model!.id,
      datasetId: sftForm.datasetId,
      workspace: wsStore.currentWorkspaceId || '',
      exportModel: sftForm.exportModel,
      image: sftForm.image,
      nodeCount: sftForm.nodeCount,
      gpuCount: sftForm.gpuCount,
      cpu: sftForm.cpu,
      memory: `${sftForm.memory}Gi`,
      ephemeralStorage: `${sftForm.ephemeralStorage}Gi`,
      priority: sftForm.priority,
      trainConfig: { ...sftForm.trainConfig },
    })
    emit('success', (res as any).workloadId)
    handleClose()
  } finally {
    submitting.value = false
  }
}

async function submitRlJob() {
  await rlFormRef.value?.validate()
  submitting.value = true
  try {
    const res = await createRlJob({
      displayName: rlForm.displayName,
      modelId: props.model!.id,
      datasetId: rlForm.datasetId,
      workspace: wsStore.currentWorkspaceId || '',
      exportModel: rlForm.exportModel,
      image: rlForm.image,
      nodeCount: rlForm.nodeCount,
      gpuCount: rlForm.gpuCount,
      cpu: rlForm.cpu,
      memory: `${rlForm.memory}Gi`,
      sharedMemory: `${rlForm.sharedMemory}Gi`,
      ephemeralStorage: `${rlForm.ephemeralStorage}Gi`,
      priority: rlForm.priority,
      trainConfig: { ...rlForm.trainConfig },
    })
    emit('success', (res as any).workloadId)
    handleClose()
  } finally {
    submitting.value = false
  }
}
```

## 6. 父组件改动 `ModelSquare/index.vue`

### 6.1 替换 SFT 按钮

```diff
- <!-- SFT button -->
- <el-tooltip v-if="canSft(model)" content="SFT" placement="top">
-   <el-button size="small" @click="handleCommand('sft', model)" circle class="btn-sft">
-     <el-icon><MagicStick /></el-icon>
-   </el-button>
- </el-tooltip>
+ <!-- Train button (SFT + RL) -->
+ <el-tooltip v-if="canTrain(model)" content="Train" placement="top">
+   <el-button size="small" @click="handleCommand('train', model)" circle class="btn-train">
+     <el-icon><MagicStick /></el-icon>
+   </el-button>
+ </el-tooltip>
```

### 6.2 Dialog 替换

```diff
- <CreateSftDialog
-   v-model:visible="showSftDialog"
-   :model="currentSftModel"
-   @success="handleSftSuccess"
- />
+ <CreateTrainingDialog
+   v-model:visible="showTrainDialog"
+   :model="currentTrainModel"
+   @success="handleTrainSuccess"
+ />
```

### 6.3 逻辑

```typescript
// canTrain = canSft (同样条件: local + Ready)
const canTrain = (model: PlaygroundModel) =>
  model.source?.accessMode === 'local_path' && model.phase === 'Ready'

const showTrainDialog = ref(false)
const currentTrainModel = ref<PlaygroundModel | null>(null)

case 'train':
  currentTrainModel.value = model
  showTrainDialog.value = true
  break

const handleTrainSuccess = (workloadId: string) => {
  showTrainDialog.value = false
  currentTrainModel.value = null
  router.push({ path: '/training/detail', query: { id: workloadId } })
}
```

## 7. Entrypoint 说明

后端 `rl_entrypoint_builder.go` 的 `BuildRlTrainScript()` 生成完整的 shell 脚本内容（几百行），通过 `BuildRlContainerEntrypoint()` 写入 pod 的 entry_points：

```go
trainScript := BuildRlTrainScript(cfg)           // 完整 shell 内容
headEntrypoint := BuildRlContainerEntrypoint(trainScript, true)   // 写到 /tmp/rl_train.sh
workerEntrypoint := BuildRlContainerEntrypoint(trainScript, false) // worker 不写脚本
```

最终 entrypoint 是 **base64 编码的完整 shell 内容**（不是文件路径），通过 `entry_points[]` 数组传给 workload_create API。`RAY_JOB_ENTRYPOINT` 则是 `bash /tmp/rl_train.sh`。

前端**不需要关心** entrypoint 的具体内容，只需提交 `CreateRlJobRequest` JSON，后端自动生成。

## 8. 数据集类型对照

| Training Type | Dataset Type | 数据格式 |
|--------------|-------------|---------|
| SFT | `sft` | instruction/output（Alpaca 格式） |
| RL | `rlhf` | question/answer（GRPO 规则奖励） |

## 9. Config API 触发时机总结

```
Dialog 打开
  └─ GET sft-config (默认 SFT)
      └─ 填充 SFT 表单 + 加载 sft 数据集

用户切换到 RL tab
  └─ GET rl-config?strategy=fsdp2 (默认 FSDP2)
      └─ 填充 RL 表单 + 加载 rlhf 数据集

用户切换 RL strategy 到 Megatron
  └─ GET rl-config?strategy=megatron
      └─ 刷新 trainConfig + image（保留 displayName/datasetId）

用户切回 SFT tab
  └─ 使用缓存的 sftConfig（或重新 GET）

提交
  └─ SFT: POST /sft/jobs
  └─ RL:  POST /rl/jobs
```

## 10. 模型广场筛选 & 标签改动

### 10.1 Origin 值对照表

| origin 值 | 来源 | 卡片标签 | 筛选归类 |
|-----------|------|---------|---------|
| `external` | 用户导入 | 无 | **Imported** |
| `fine_tuned` | SFT 训练产出 | **SFT**（绿色） | **Custom** |
| `rl_trained` | RL 训练产出 | **RL**（紫色） | **Custom** |

### 10.2 筛选下拉改动 `ModelSquare/index.vue`

```diff
 // Origin filter options
 const originOptions = [
   { label: 'All', value: '' },
   { label: 'Imported', value: 'external' },
-  { label: 'SFT', value: 'fine_tuned' },
+  { label: 'Custom', value: 'custom' },
 ]
```

> `custom` 是虚拟筛选值，后端 `matchModelOrigin()` 会匹配所有非 `external` 的模型（包括 `fine_tuned` 和 `rl_trained`）。

### 10.3 卡片标签改动 `ModelSquare/index.vue`

```diff
 <!-- Origin tag on model card -->
-<el-tag
-  v-if="model.origin === 'fine_tuned'"
-  size="small"
-  type="success"
->
-  SFT
-</el-tag>
+<el-tag
+  v-if="model.origin === 'fine_tuned'"
+  size="small"
+  type="success"
+>
+  SFT
+</el-tag>
+<el-tag
+  v-if="model.origin === 'rl_trained'"
+  size="small"
+  type="warning"
+>
+  RL
+</el-tag>
```

### 10.4 卡片元信息改动 `ModelSquare/index.vue`

```diff
 <!-- Model metadata (base model, user) -->
-<div v-if="model.origin === 'fine_tuned'" class="text-xs text-gray-400 mt-1">
+<div v-if="model.origin === 'fine_tuned' || model.origin === 'rl_trained'" class="text-xs text-gray-400 mt-1">
   <span v-if="model.userName">By {{ model.userName }}</span>
   <span v-if="model.userName && model.baseModel"> · </span>
   <span v-if="model.baseModel">Base: {{ model.baseModel }}</span>
 </div>
```

### 10.5 详情页改动 `ModelSquareDetail.vue`

```diff
 <!-- Origin display -->
-<el-tag size="small" :type="detailData.origin === 'fine_tuned' ? 'success' : 'info'">
-  {{ detailData.origin === 'fine_tuned' ? 'SFT' : 'External' }}
-</el-tag>
+<el-tag
+  size="small"
+  :type="detailData.origin === 'fine_tuned' ? 'success' : detailData.origin === 'rl_trained' ? 'warning' : 'info'"
+>
+  {{ detailData.origin === 'fine_tuned' ? 'SFT' : detailData.origin === 'rl_trained' ? 'RL' : 'External' }}
+</el-tag>

 <!-- SFT/RL job link -->
-<div v-if="detailData.origin === 'fine_tuned'">
+<div v-if="detailData.origin === 'fine_tuned' || detailData.origin === 'rl_trained'">
   <router-link :to="{ path: '/training/detail', query: { id: detailData.sftJobId } }">
     View Training Job
   </router-link>
 </div>
```

### 10.6 后端已完成的配套改动

| 文件 | 改动 |
|------|------|
| `models.go` | 新增 `matchModelOrigin()`：`"custom"` 匹配所有非 external |
| `models.go` | K8s 列表路径：用 `matchModelOrigin` 替代精确匹配 |
| `models.go` | DB 列表路径：`"custom"` 时不传给 SQL，内存过滤 |
| `types.go` | `ListModelQuery.Origin` 注释更新 |
| `rl_entrypoint_builder.go` | RL 模型注册时 `origin: "rl_trained"` |

## 11. 开发计划

| 步骤 | 内容 | 估时 |
|------|------|------|
| 1 | `services/rl/types.ts` + `index.ts` | 0.5h |
| 2 | `CreateTrainingDialog.vue` 框架 + Tab 切换 | 1h |
| 3 | SFT 表单（从 CreateSftDialog 迁移） | 1.5h |
| 4 | RL 表单（strategy 切换 + 动态参数） | 2h |
| 5 | `ModelSquare/index.vue` 改 Train 按钮 + 筛选 + 标签 | 1h |
| 6 | `ModelSquareDetail.vue` 同步改造 | 0.5h |
| 7 | 联调测试 | 1h |
| **总计** | | **~7.5h** |
