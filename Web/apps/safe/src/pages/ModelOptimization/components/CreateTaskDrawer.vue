<template>
  <el-drawer
    :model-value="visible"
    title="Create Optimization Task"
    direction="rtl"
    size="780px"
    destroy-on-close
    append-to-body
    :before-close="handleClose"
    :z-index="2001"
  >
    <div class="drawer-body">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="auto" label-position="top">
        <!-- Basic -->
        <div class="section-card">
          <div class="section-header">
            <span class="section-bar" />
            <span class="section-title">Basic</span>
          </div>

          <el-form-item label="Model" prop="modelId">
            <el-select
              v-model="form.modelId"
              filterable
              placeholder="Select a model"
              style="width: 100%"
              :loading="modelsLoading"
            >
              <el-option
                v-for="m in readyModels"
                :key="m.modelId"
                :label="m.modelName"
                :value="m.modelId"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="Workspace" prop="workspace">
            <el-input v-model="form.workspace" disabled />
          </el-form-item>

          <el-form-item label="Display Name" prop="displayName">
            <el-input v-model="form.displayName" placeholder="Optional display name" />
          </el-form-item>

          <el-form-item label="Mode">
            <el-radio-group v-model="form.mode">
              <el-radio value="local">Local</el-radio>
              <el-radio value="claw">Claw</el-radio>
            </el-radio-group>
          </el-form-item>
        </div>

        <!-- Inference -->
        <div class="section-card">
          <div class="section-header">
            <span class="section-bar" />
            <span class="section-title">Inference Configuration</span>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Framework">
                <el-select v-model="form.framework" style="width: 100%">
                  <el-option v-for="f in FRAMEWORK_OPTIONS" :key="f" :label="f" :value="f" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Precision">
                <el-select v-model="form.precision" style="width: 100%">
                  <el-option v-for="p in PRECISION_OPTIONS" :key="p" :label="p" :value="p" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="16">
            <el-col :span="8">
              <el-form-item label="GPU Type">
                <el-select v-model="form.gpuType" style="width: 100%">
                  <el-option
                    v-for="g in GPU_TYPE_OPTIONS"
                    :key="g.value"
                    :label="g.label"
                    :value="g.value"
                    :disabled="g.disabled"
                  />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="TP">
                <el-input-number v-model="form.tp" :min="1" :max="8" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="EP">
                <el-input-number v-model="form.ep" :min="1" :max="8" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="16">
            <el-col :span="8">
              <el-form-item label="ISL">
                <el-input-number v-model="form.isl" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="OSL">
                <el-input-number v-model="form.osl" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="Concurrency">
                <el-input-number v-model="form.concurrency" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- Kernel Optimization -->
        <div class="section-card">
          <div class="section-header">
            <span class="section-bar" />
            <span class="section-title">Kernel Optimization</span>
          </div>

          <el-form-item label="Kernel Backends">
            <el-checkbox-group v-model="form.kernelBackends">
              <el-checkbox
                v-for="b in KERNEL_BACKEND_OPTIONS"
                :key="b"
                :value="b"
                :label="b"
              />
            </el-checkbox-group>
          </el-form-item>

          <el-form-item label="GEAK Step Limit">
            <el-input-number v-model="form.geakStepLimit" :min="1" :max="1000" style="width: 200px" />
          </el-form-item>
        </div>

        <!-- Advanced -->
        <div class="section-card">
          <div class="section-header">
            <span class="section-bar" />
            <span class="section-title">Advanced</span>
          </div>

          <el-form-item label="Image">
            <el-input v-model="form.image" placeholder="Leave empty for default" />
          </el-form-item>

          <el-form-item label="Results Path">
            <el-input v-model="form.resultsPath" placeholder="/workspace/hyperloom/" />
          </el-form-item>

          <el-row :gutter="16" v-if="form.mode === 'claw'">
            <el-col :span="6">
              <el-form-item label="Ray Replica">
                <el-input-number v-model="form.rayReplica" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="Ray GPU">
                <el-input-number v-model="form.rayGpu" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="Ray CPU">
                <el-input-number v-model="form.rayCpu" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="Ray Memory (Gi)">
                <el-input-number v-model="form.rayMemory" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </el-form>
    </div>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        Create Task
      </el-button>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import { ref, reactive, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createOptimizationTask } from '@/services/model-optimization'
import type { CreateOptimizationTaskPayload } from '@/services/model-optimization/type'
import { useWorkspaceStore } from '@/stores/workspace'
import { useRouter } from 'vue-router'
import request from '@/services/request'

const FRAMEWORK_OPTIONS = ['sglang', 'vllm']
const PRECISION_OPTIONS = ['FP4', 'FP8']
const KERNEL_BACKEND_OPTIONS = ['GEAK', 'Claude Code', 'Codex']
const GPU_TYPE_OPTIONS = [
  { value: 'MI300X', label: 'MI300X', disabled: true },
  { value: 'MI325X', label: 'MI325X', disabled: true },
  { value: 'MI355X', label: 'MI355X', disabled: false },
]

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  'update:visible': [val: boolean]
  success: []
}>()

const wsStore = useWorkspaceStore()
const router = useRouter()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const modelsLoading = ref(false)
const readyModels = ref<Array<{ modelId: string; modelName: string }>>([])

const defaultForm = (): CreateOptimizationTaskPayload => ({
  modelId: '',
  workspace: wsStore.currentWorkspaceId || '',
  displayName: '',
  mode: 'local',
  framework: 'sglang',
  precision: 'FP4',
  tp: 1,
  ep: 1,
  gpuType: 'MI355X',
  isl: 1024,
  osl: 1024,
  concurrency: 64,
  kernelBackends: ['Claude Code'],
  geakStepLimit: 100,
  image: '',
  resultsPath: '/workspace/hyperloom/',
  rayReplica: 1,
  rayGpu: 1,
  rayCpu: 32,
  rayMemory: 128,
})

const form = reactive(defaultForm())

const rules: FormRules = {
  modelId: [{ required: true, message: 'Please select a model', trigger: 'change' }],
  workspace: [{ required: true, message: 'Workspace is required', trigger: 'change' }],
}

watch(() => props.visible, (v) => {
  if (v) {
    Object.assign(form, defaultForm())
    loadModels()
  }
})

const loadModels = async () => {
  modelsLoading.value = true
  try {
    const res = await request.get('/models', {
      params: { workspace: wsStore.currentWorkspaceId },
    })
    const items = (res as any)?.items || res || []
    readyModels.value = items.filter(
      (m: any) =>
        m.phase === 'Ready' &&
        (m.accessMode === 'local' || m.accessMode === 'local_path'),
    )
  } finally {
    modelsLoading.value = false
  }
}

const handleClose = async () => {
  try {
    await ElMessageBox.confirm('Discard unsaved changes?', 'Confirm', {
      confirmButtonText: 'Discard',
      cancelButtonText: 'Keep Editing',
      type: 'warning',
    })
    emit('update:visible', false)
  } catch {
    // keep editing
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const payload: CreateOptimizationTaskPayload = { ...form }
    if (!payload.image) delete payload.image
    if (!payload.displayName) delete payload.displayName

    const task = await createOptimizationTask(payload)
    ElMessage.success('Task created')
    emit('update:visible', false)
    emit('success')
    router.push(`/model-optimization/${task.id}`)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.drawer-body {
  padding: 0 16px 16px;
  overflow-y: auto;
  max-height: calc(100vh - 140px);
}
.section-card {
  padding: 16px;
  margin-bottom: 16px;
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
}
.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}
.section-bar {
  width: 3px;
  height: 16px;
  border-radius: 2px;
  background: var(--safe-primary, var(--el-color-primary));
}
.section-title {
  font-weight: 600;
  font-size: 14px;
}
</style>
