<template>
  <el-dialog
    :model-value="visible"
    :title="cloneTaskId ? 'Clone Evaluation Task' : 'Create Evaluation Task'"
    width="650"
    @close="handleClose"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="110px"
      class="p-x-2"
      :rules="rules"
      v-loading="loading"
    >
      <!-- Basic Information -->
      <div class="section-header">
        <div class="section-title">Basic Information</div>
        <div class="section-subtitle">Configure task name and inference service</div>
      </div>

      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" placeholder="Enter task name" />
      </el-form-item>

      <el-form-item label="Service" prop="serviceId">
        <el-select
          v-model="form.serviceId"
          placeholder="Select inference service"
          filterable
          class="w-full"
          @change="onServiceChange"
        >
          <el-option
            v-for="item in services"
            :key="item.serviceId"
            :label="item.displayName"
            :value="item.serviceId"
          >
            <div class="flex flex-col">
              <span>{{ item.displayName }}</span>
              <span class="text-xs text-gray-400">{{ item.modelName }}</span>
            </div>
          </el-option>
        </el-select>
      </el-form-item>

      <!-- Benchmark Configuration -->
      <div class="section-header">
        <div class="section-title">Benchmarks</div>
        <div class="section-subtitle">Select evaluation datasets and sample limits</div>
      </div>

      <el-form-item prop="benchmarks" required label-width="0">
        <div class="w-full">
          <div
            v-for="(benchmark, index) in form.benchmarks"
            :key="benchmark.uid"
            class="benchmark-item"
          >
            <div class="benchmark-header">
              <span class="benchmark-index">#{{ index + 1 }}</span>
              <el-button
                type="danger"
                link
                size="small"
                @click="removeBenchmark(index)"
                v-if="form.benchmarks.length > 1"
              >
                Remove
              </el-button>
            </div>

            <el-form-item
              :prop="`benchmarks.${index}.datasetId`"
              :rules="[{ required: true, message: 'Dataset is required', trigger: 'change' }]"
              label="Dataset"
              label-width="110px"
              class="m-b-2"
            >
              <el-select
                v-model="benchmark.datasetId"
                placeholder="Select dataset"
                filterable
                class="w-full"
              >
                <el-option
                  v-for="dataset in datasets"
                  :key="dataset.datasetId"
                  :label="dataset.displayName"
                  :value="dataset.datasetId"
                >
                  <div class="flex flex-col">
                    <span>{{ dataset.displayName }}</span>
                    <span class="text-xs text-gray-400">{{ dataset.description }}</span>
                  </div>
                </el-option>
              </el-select>
            </el-form-item>

            <el-form-item label="Limit" label-width="110px" class="m-b-0">
              <el-input-number
                v-model="benchmark.limit"
                :min="1"
                :max="10000"
                placeholder="Optional"
                style="width: 160px"
              />
              <span class="text-xs text-gray-400 ml-2">Leave empty to use all samples</span>
            </el-form-item>
          </div>

          <el-button @click="addBenchmark" class="w-full m-t-2" size="small">
            + Add Dataset
          </el-button>
        </div>
      </el-form-item>

      <!-- Judge Configuration (Optional) -->
      <div class="section-header">
        <div class="section-title">
          Judge Model
          <el-tag size="small" type="info" class="ml-2">Optional</el-tag>
        </div>
        <div class="section-subtitle">Configure external judge model for evaluation</div>
      </div>

      <el-form-item label-width="0">
        <el-checkbox v-model="enableJudge">Enable judge model</el-checkbox>
      </el-form-item>

      <template v-if="enableJudge">
        <el-form-item label="Judge Service" prop="judgeServiceId">
          <el-select
            v-model="form.judgeServiceId"
            placeholder="Select judge service"
            filterable
            class="w-full"
            @change="onJudgeServiceChange"
          >
            <el-option
              v-for="item in availableJudgeServices"
              :key="item.serviceId"
              :label="item.displayName"
              :value="item.serviceId"
            >
              <div class="flex flex-col">
                <span>{{ item.displayName }}</span>
                <span class="text-xs text-gray-400">{{ item.modelName }}</span>
              </div>
            </el-option>
          </el-select>
        </el-form-item>
      </template>

      <!-- Advanced Settings -->
      <div class="section-header">
        <div class="section-title">Advanced</div>
        <div class="section-subtitle">Timeout, concurrency and other configurations</div>
      </div>

      <el-form-item label="Timeout" prop="timeoutSecond">
        <el-input-number v-model="form.timeoutSecond" :min="60" :max="86400" style="width: 160px" />
        <span class="text-xs text-gray-400 ml-2">seconds</span>
      </el-form-item>

      <el-form-item label="Concurrency" prop="concurrency">
        <el-input-number v-model="form.concurrency" :min="1" :max="128" style="width: 160px" />
        <span class="text-xs text-gray-400 ml-2">parallel requests</span>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">Cancel</el-button>
        <el-button type="primary" @click="onSubmit" :loading="submitting">
          {{ cloneTaskId ? 'Clone' : 'Create' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { getAvailableServices, getEvaluationTaskDetail } from '@/services/evaluations'
import { getDatasets } from '@/services/dataset'
import { addOpsjobs } from '@/services/opsjobs'
import type { AvailableService } from '@/services/evaluations/type'
import type { DatasetItem } from '@/services/dataset/type'

const props = defineProps<{
  visible: boolean
  cloneTaskId?: string
}>()

const emit = defineEmits(['update:visible', 'success', 'close'])

const workspaceStore = useWorkspaceStore()
const ruleFormRef = ref<FormInstance>()
const loading = ref(false)
const submitting = ref(false)
const services = ref<AvailableService[]>([])
const datasets = ref<DatasetItem[]>([])
const enableJudge = ref(false)

interface BenchmarkItem {
  uid: string
  datasetId: string
  limit?: number
}

const initialForm = {
  name: '',
  serviceId: '',
  serviceType: '',
  benchmarks: [] as BenchmarkItem[],
  judgeServiceId: '',
  judgeServiceType: '',
  timeoutSecond: 7200,
  concurrency: 32,
}

const form = reactive({ ...initialForm })

const rules = reactive<FormRules>({
  name: [{ required: true, message: 'Please enter task name', trigger: 'blur' }],
  serviceId: [{ required: true, message: 'Please select a service', trigger: 'change' }],
  benchmarks: [
    {
      required: true,
      message: 'At least one dataset is required',
      trigger: 'change',
      validator: (_rule, _value, callback) => {
        if (form.benchmarks.length === 0) {
          callback(new Error('At least one dataset is required'))
        } else {
          callback()
        }
      },
    },
  ],
  judgeServiceId: [
    {
      validator: (_rule, _value, callback) => {
        if (enableJudge.value && !form.judgeServiceId) {
          callback(new Error('Judge service is required'))
        } else {
          callback()
        }
      },
      trigger: 'change',
    },
  ],
})

const addBenchmark = () => {
  form.benchmarks.push({
    uid: Date.now().toString() + Math.random(),
    datasetId: '',
    limit: undefined,
  })
}

const removeBenchmark = (index: number) => {
  form.benchmarks.splice(index, 1)
}

const onServiceChange = () => {
  const service = services.value.find((s) => s.serviceId === form.serviceId)
  if (service) {
    form.serviceType = service.serviceType
  }
  // Clear judge service if it's the same as inference service
  if (form.judgeServiceId === form.serviceId) {
    form.judgeServiceId = ''
    form.judgeServiceType = ''
  }
}

const onJudgeServiceChange = () => {
  const service = services.value.find((s) => s.serviceId === form.judgeServiceId)
  if (service) {
    form.judgeServiceType = service.serviceType
  }
}

// Filter out the selected inference service from judge service options
const availableJudgeServices = computed(() => {
  return services.value.filter((s) => s.serviceId !== form.serviceId)
})

const fetchServices = async () => {
  try {
    const res = await getAvailableServices({
      workspace: workspaceStore.currentWorkspaceId,
    })
    services.value = res.items || []
  } catch (error) {
    console.error('Failed to fetch available services:', error)
    ElMessage.error('Failed to load available services')
  }
}

const fetchDatasets = async () => {
  try {
    const res = await getDatasets({
      datasetType: 'evaluation',
      workspace: workspaceStore.currentWorkspaceId,
    })
    datasets.value = res.items || []
  } catch (error) {
    console.error('Failed to fetch datasets:', error)
    ElMessage.error('Failed to load datasets')
  }
}

const loadCloneData = async () => {
  if (!props.cloneTaskId) return

  try {
    const detail = await getEvaluationTaskDetail(props.cloneTaskId)

    // Prefill form with cloned data
    form.name = detail.taskName || ''
    form.serviceId = detail.serviceId || ''
    form.serviceType = detail.serviceType || ''

    // Prefill benchmarks
    if (detail.benchmarks && detail.benchmarks.length > 0) {
      form.benchmarks = detail.benchmarks.map((b) => ({
        uid: Date.now().toString() + Math.random(),
        datasetId: b.datasetId,
        limit: b.limit,
      }))
    } else {
      addBenchmark()
    }

    // Prefill judge configuration if exists
    if (detail.judgeServiceId) {
      enableJudge.value = true
      form.judgeServiceId = detail.judgeServiceId
      form.judgeServiceType = detail.judgeServiceType || ''
    }

    form.concurrency = detail.concurrency || 32
    form.timeoutSecond = detail.timeout || 7200
  } catch (error) {
    console.error('Failed to load task for cloning:', error)
    ElMessage.error('Failed to load task data')
    addBenchmark()
  }
}

const onOpen = async () => {
  loading.value = true
  try {
    // Reset form
    Object.assign(form, initialForm)
    form.benchmarks = []
    enableJudge.value = false
    ruleFormRef.value?.clearValidate()

    // Fetch data
    await Promise.all([fetchServices(), fetchDatasets()])

    // Load clone data if cloneTaskId is provided
    if (props.cloneTaskId) {
      await loadCloneData()
    } else {
      // Add first benchmark by default for new tasks
      addBenchmark()
    }
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  emit('update:visible', false)
  emit('close')
}

const onSubmit = async () => {
  if (!ruleFormRef.value) return

  try {
    await ruleFormRef.value.validate()

    submitting.value = true

    // Prepare benchmarks data
    const benchmarksData = form.benchmarks
      .filter((b) => b.datasetId)
      .map((b) => ({
        datasetId: b.datasetId,
        ...(b.limit ? { limit: b.limit } : {}),
      }))

    // Prepare inputs for OpsJob
    const inputs = [
      { name: 'eval.service.id', value: form.serviceId },
      { name: 'eval.service.type', value: form.serviceType },
      { name: 'eval.benchmarks', value: JSON.stringify(benchmarksData) },
      { name: 'eval.concurrency', value: String(form.concurrency) },
      { name: 'workspace', value: workspaceStore.currentWorkspaceId || '' },
    ]

    // Add judge configuration if enabled
    if (enableJudge.value && form.judgeServiceId) {
      inputs.push({
        name: 'eval.judge',
        value: JSON.stringify({
          serviceId: form.judgeServiceId,
          serviceType: form.judgeServiceType,
        }),
      })
    }

    // Create evaluation task via OpsJob
    await addOpsjobs({
      name: form.name,
      type: 'evaluation',
      inputs,
      timeoutSecond: form.timeoutSecond,
    })

    const successMsg = props.cloneTaskId
      ? 'Evaluation task cloned successfully'
      : 'Evaluation task created successfully'
    ElMessage.success(successMsg)
    handleClose()
    emit('success')
  } catch (error: unknown) {
    if (error && typeof error === 'object' && !(error instanceof Error)) {
      const fields = error as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      ElMessage.error(firstMsg)
    }
  } finally {
    submitting.value = false
  }
}

// Watch workspace change
watch(
  () => workspaceStore.currentWorkspaceId,
  () => {
    if (props.visible) {
      fetchServices()
      fetchDatasets()
    }
  },
)
</script>

<style scoped>
.section-header {
  margin-bottom: 20px;
  margin-top: 24px;
}

.section-header:first-child {
  margin-top: 0;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  display: flex;
  align-items: center;
}

.section-subtitle {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.benchmark-item {
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 16px;
  margin-bottom: 12px;
  transition: all 0.3s;
}

.benchmark-item:hover {
  border-color: var(--el-border-color);
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.04);
}

.benchmark-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.benchmark-index {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}

:deep(.el-select-dropdown__item) {
  height: auto;
  padding: 8px 20px;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}

:deep(.benchmark-item .el-form-item) {
  margin-bottom: 12px;
}

:deep(.benchmark-item .el-form-item:last-child) {
  margin-bottom: 0;
}
</style>
