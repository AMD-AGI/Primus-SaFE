<template>
  <el-dialog
    v-model="visible"
    title="Create Fine-Tuning Job"
    :close-on-click-modal="false"
    width="860"
    destroy-on-close
    @close="handleClose"
    class="sft-dialog"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="formRules"
      label-width="auto"
      class="p-y-3 p-x-5"
      :disabled="configLoading"
    >
      <!-- Section 1: Base Model (read-only) -->
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Base Model</span>
      </div>

      <div class="model-info-card m-b-4">
        <div class="flex items-center gap-3">
          <div v-if="props.model?.icon" class="model-icon">
            <img :src="props.model.icon" alt="" class="w-10 h-10 rounded" />
          </div>
          <div>
            <div class="font-medium">{{ props.model?.displayName || props.model?.id }}</div>
            <div class="text-xs text-gray-500 mt-1">
              <el-tag size="small" type="success">{{ props.model?.phase }}</el-tag>
              <el-tag size="small" type="primary" class="ml-1">{{ props.model?.accessMode }}</el-tag>
            </div>
          </div>
        </div>
      </div>

      <!-- Unsupported warning -->
      <el-alert
        v-if="sftConfig && !sftConfig.supported"
        :title="sftConfig.reason || 'This model is not supported for fine-tuning'"
        type="warning"
        show-icon
        :closable="false"
        class="m-b-4"
      />

      <!-- Config loading -->
      <div v-if="configLoading" class="text-center p-y-8">
        <el-icon class="is-loading" :size="24"><Loading /></el-icon>
        <div class="text-gray-500 mt-2">Loading configuration...</div>
      </div>

      <template v-if="sftConfig?.supported && !configLoading">
        <!-- Section 2: Dataset -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Dataset</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Job Name" prop="displayName">
              <el-input v-model="form.displayName" placeholder="e.g. sft-qwen3-8b-alpaca" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Dataset" prop="datasetId">
              <el-select
                v-model="form.datasetId"
                placeholder="Select SFT dataset"
                filterable
                class="w-full"
                :loading="datasetsLoading"
              >
                <el-option
                  v-for="ds in datasets"
                  :key="ds.datasetId"
                  :label="ds.displayName"
                  :value="ds.datasetId"
                >
                  <div class="flex justify-between items-center">
                    <span>{{ ds.displayName }}</span>
                    <span class="text-xs text-gray-400">{{ ds.totalSizeStr || '' }}</span>
                  </div>
                </el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Section 3: Training Configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Training Configuration</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="PEFT" prop="trainConfig.peft">
              <el-select v-model="form.trainConfig.peft" class="w-full">
                <el-option
                  v-for="opt in sftConfig.options.peftOptions"
                  :key="opt"
                  :label="opt"
                  :value="opt"
                />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Dataset Format" prop="trainConfig.datasetFormat">
              <el-select v-model="form.trainConfig.datasetFormat" class="w-full">
                <el-option
                  v-for="opt in sftConfig.options.datasetFormatOptions"
                  :key="opt"
                  :label="opt"
                  :value="opt"
                />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Train Iters" prop="trainConfig.trainIters">
              <el-input-number
                v-model="form.trainConfig.trainIters"
                :min="1"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Global Batch Size" prop="trainConfig.globalBatchSize">
              <el-input-number
                v-model="form.trainConfig.globalBatchSize"
                :min="1"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Micro Batch Size" prop="trainConfig.microBatchSize">
              <el-input-number
                v-model="form.trainConfig.microBatchSize"
                :min="1"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Seq Length" prop="trainConfig.seqLength">
              <el-input-number
                v-model="form.trainConfig.seqLength"
                :min="1"
                :step="256"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Learning Rate" prop="trainConfig.finetuneLr">
              <el-input-number
                v-model="form.trainConfig.finetuneLr"
                :min="0"
                :step="0.00001"
                :precision="6"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Advanced Training Settings (collapsed) -->
        <el-collapse class="m-b-4">
          <el-collapse-item title="Advanced Training Settings" name="advanced-train">
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Min LR">
                  <el-input-number
                    v-model="form.trainConfig.minLr"
                    :min="0"
                    :step="0.00001"
                    :precision="6"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="LR Warmup Iters">
                  <el-input-number
                    v-model="form.trainConfig.lrWarmupIters"
                    :min="0"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
            </el-row>

            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Eval Interval">
                  <el-input-number
                    v-model="form.trainConfig.evalInterval"
                    :min="1"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Save Interval">
                  <el-input-number
                    v-model="form.trainConfig.saveInterval"
                    :min="1"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
            </el-row>

            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Precision">
                  <el-input v-model="form.trainConfig.precisionConfig" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Packed Sequence">
                  <el-switch v-model="form.trainConfig.packedSequence" />
                </el-form-item>
              </el-col>
            </el-row>

            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="TP Size">
                  <el-input-number
                    v-model="form.trainConfig.tensorModelParallelSize"
                    :min="1"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="PP Size">
                  <el-input-number
                    v-model="form.trainConfig.pipelineModelParallelSize"
                    :min="1"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
            </el-row>

            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="CP Size">
                  <el-input-number
                    v-model="form.trainConfig.contextParallelSize"
                    :min="1"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Sequence Parallel">
                  <el-switch v-model="form.trainConfig.sequenceParallel" />
                </el-form-item>
              </el-col>
            </el-row>

            <el-row :gutter="20" v-if="form.trainConfig.peft === 'lora'">
              <el-col :span="12">
                <el-form-item label="LoRA Dim">
                  <el-input-number
                    v-model="form.trainConfig.peftDim"
                    :min="0"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="LoRA Alpha">
                  <el-input-number
                    v-model="form.trainConfig.peftAlpha"
                    :min="0"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
            </el-row>
          </el-collapse-item>
        </el-collapse>

        <!-- Section 4: Resource Configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Resource Configuration</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Image" prop="image">
              <el-input v-model="form.image" placeholder="Training image" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Priority" prop="priority">
              <el-select v-model="form.priority" class="w-full">
                <el-option
                  v-for="opt in sftConfig.options.priorityOptions"
                  :key="opt.value"
                  :label="opt.label"
                  :value="opt.value"
                />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Nodes" prop="nodeCount">
              <el-input-number
                v-model="form.nodeCount"
                :min="1"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="GPUs" prop="gpuCount">
              <el-input-number
                v-model="form.gpuCount"
                :min="1"
                controls-position="right"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="CPU" prop="cpu">
              <el-input v-model="form.cpu" placeholder="e.g. 128" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Memory" prop="memory">
              <el-input v-model="form.memory" placeholder="e.g. 1024">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Ephemeral Storage">
              <el-input v-model="form.ephemeralStorage" placeholder="e.g. 300">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Section 5: Output / Export -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Output / Export</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Export model after training">
              <el-switch v-model="form.exportModel" />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="text-xs text-gray-400 m-b-4" style="padding-left: 2px;">
          When enabled, training output will be exported to PFS and registered in Model Square
        </div>

        <!-- Section 6: Advanced Settings (collapsed) -->
        <el-collapse class="m-b-4">
          <el-collapse-item title="Advanced Settings" name="advanced">
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Timeout (seconds)">
                  <el-input-number
                    v-model="form.timeout"
                    :min="0"
                    controls-position="right"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Force Host Network">
                  <el-switch v-model="form.forceHostNetwork" />
                </el-form-item>
              </el-col>
            </el-row>
          </el-collapse-item>
        </el-collapse>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button
        type="primary"
        @click="handleSubmit"
        :loading="submitting"
        :disabled="configLoading || !sftConfig?.supported"
      >
        Create Job
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import type { FormInstance, FormRules } from 'element-plus'
import { getSftConfig, createSftJob } from '@/services/sft'
import type { SftConfigResponse, SftTrainConfig } from '@/services/sft'
import { getDatasets } from '@/services/dataset'
import type { DatasetItem } from '@/services/dataset/type'
import type { PlaygroundModel } from '@/services/playground'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  visible: boolean
  model: PlaygroundModel | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: [workloadId: string]
}>()

const wsStore = useWorkspaceStore()

const dialogVisible = computed({
  get: () => props.visible,
  set: (val: boolean) => emit('update:visible', val),
})

// eslint-disable-next-line vue/no-dupe-keys
const visible = dialogVisible

const formRef = ref<FormInstance>()
const configLoading = ref(false)
const datasetsLoading = ref(false)
const submitting = ref(false)
const sftConfig = ref<SftConfigResponse | null>(null)
const datasets = ref<DatasetItem[]>([])

const defaultTrainConfig: SftTrainConfig = {
  peft: 'none',
  datasetFormat: 'alpaca',
  trainIters: 1000,
  globalBatchSize: 128,
  microBatchSize: 1,
  seqLength: 2048,
  finetuneLr: 0.0001,
  minLr: 0,
  lrWarmupIters: 50,
  evalInterval: 30,
  saveInterval: 50,
  precisionConfig: 'bf16_mixed',
  tensorModelParallelSize: 1,
  pipelineModelParallelSize: 1,
  contextParallelSize: 1,
  sequenceParallel: false,
  peftDim: 0,
  peftAlpha: 0,
  packedSequence: false,
}

const form = reactive({
  displayName: '',
  datasetId: '',
  exportModel: true,
  image: '',
  nodeCount: 1,
  gpuCount: 8,
  cpu: '128',
  memory: '1024',
  ephemeralStorage: '300',
  priority: 1,
  trainConfig: { ...defaultTrainConfig },
  timeout: 0,
  forceHostNetwork: false,
})

const formRules: FormRules = {
  displayName: [{ required: true, message: 'Please enter a job name', trigger: 'blur' }],
  datasetId: [{ required: true, message: 'Please select a dataset', trigger: 'change' }],
  image: [{ required: true, message: 'Image is required', trigger: 'blur' }],
  nodeCount: [{ required: true, message: 'Required', trigger: 'change' }],
  gpuCount: [{ required: true, message: 'Required', trigger: 'change' }],
  cpu: [{ required: true, message: 'Required', trigger: 'blur' }],
  memory: [{ required: true, message: 'Required', trigger: 'blur' }],
}

const loadConfig = async () => {
  if (!props.model?.id) return

  configLoading.value = true
  try {
    const res = await getSftConfig(props.model.id, wsStore.currentWorkspaceId || '')
    sftConfig.value = res as unknown as SftConfigResponse

    if (sftConfig.value.supported) {
      const d = sftConfig.value.defaults
      form.exportModel = d.exportModel
      form.image = d.image
      form.nodeCount = d.nodeCount
      form.gpuCount = d.gpuCount
      form.cpu = d.cpu
      form.memory = d.memory.replace(/Gi$/i, '')
      form.ephemeralStorage = d.ephemeralStorage.replace(/Gi$/i, '')
      form.priority = d.priority
      form.trainConfig = { ...d.trainConfig }
    }
  } catch (error) {
    ElMessage.error('Failed to load SFT configuration')
    console.error('Failed to load SFT config:', error)
  } finally {
    configLoading.value = false
  }
}

const loadDatasets = async () => {
  if (!sftConfig.value?.supported) return

  datasetsLoading.value = true
  try {
    const filter = sftConfig.value.datasetFilter
    const res = await getDatasets({
      datasetType: filter.datasetType,
      workspace: filter.workspace || wsStore.currentWorkspaceId,
    })
    datasets.value = (res as unknown as { items: DatasetItem[] }).items?.filter(
      (d) => d.status === 'Ready',
    ) || []
  } catch (error) {
    console.error('Failed to load datasets:', error)
    ElMessage.error('Failed to load datasets')
  } finally {
    datasetsLoading.value = false
  }
}

const handleSubmit = async () => {
  if (!formRef.value || !props.model) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    submitting.value = true
    try {
      const res = await createSftJob({
        displayName: form.displayName,
        modelId: props.model!.id,
        datasetId: form.datasetId,
        workspace: wsStore.currentWorkspaceId || '',
        exportModel: form.exportModel,
        image: form.image,
        nodeCount: form.nodeCount,
        gpuCount: form.gpuCount,
        cpu: form.cpu,
        memory: `${form.memory}Gi`,
        ephemeralStorage: `${form.ephemeralStorage}Gi`,
        priority: form.priority,
        trainConfig: { ...form.trainConfig },
        timeout: form.timeout || undefined,
        forceHostNetwork: form.forceHostNetwork || undefined,
      })

      const result = res as unknown as { workloadId: string }
      ElMessage.success(`SFT job created successfully`)
      emit('success', result.workloadId)
      handleClose()
    } catch (error) {
      console.error('Failed to create SFT job:', error)
      ElMessage.error((error as Error).message || 'Failed to create SFT job')
    } finally {
      submitting.value = false
    }
  })
}

const handleClose = () => {
  formRef.value?.resetFields()
  sftConfig.value = null
  datasets.value = []
  form.displayName = ''
  form.datasetId = ''
  emit('update:visible', false)
}

watch(
  () => props.visible,
  async (val) => {
    if (val && props.model) {
      await loadConfig()
      if (sftConfig.value?.supported) {
        await loadDatasets()
      }
    }
  },
)
</script>

<style scoped lang="scss">
.model-info-card {
  padding: 12px 16px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
  border: 1px solid var(--el-border-color-lighter);
}

.sft-dialog :deep(.el-dialog__body) {
  max-height: 65vh;
  overflow-y: auto;
}

:deep(.el-input-number) {
  width: 100%;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}
</style>
