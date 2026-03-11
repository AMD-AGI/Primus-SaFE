<template>
  <el-card class="sampling-config stat-card">
    <template #header>
      <span>Sampling Configuration</span>
    </template>

    <el-form
      ref="formRef"
      :model="config"
      :rules="rules"
      label-position="top"
      size="default"
    >
      <el-form-item label="Duration (seconds)" prop="duration">
        <el-input-number
          v-model="config.duration"
          :min="1"
          :max="300"
          :step="1"
          :disabled="disabled"
          style="width: 100%"
        />
        <span class="form-help">1-300 seconds</span>
      </el-form-item>

      <el-form-item label="Sample Rate (Hz)" prop="rate">
        <el-input-number
          v-model="config.rate"
          :min="1"
          :max="1000"
          :step="10"
          :disabled="disabled"
          style="width: 100%"
        />
        <span class="form-help">1-1000 Hz</span>
      </el-form-item>

      <el-form-item label="Output Format" prop="format">
        <el-select
          v-model="config.format"
          :disabled="disabled"
          style="width: 100%"
        >
          <el-option label="Flamegraph (SVG)" value="flamegraph" />
          <el-option label="Speedscope (JSON)" value="speedscope" />
          <el-option label="Raw (TXT)" value="raw" />
        </el-select>
      </el-form-item>

      <el-form-item>
        <el-checkbox v-model="config.native" :disabled="disabled">
          Include Native Stacks
        </el-checkbox>
      </el-form-item>

      <el-form-item>
        <el-checkbox v-model="config.subprocesses" :disabled="disabled">
          Profile Subprocesses
        </el-checkbox>
      </el-form-item>

      <el-form-item>
        <el-button
          type="primary"
          :disabled="disabled || !selectedProcess"
          :loading="loading"
          @click="handleStart"
          style="width: 100%"
        >
          <el-icon v-if="!loading"><VideoPlay /></el-icon>
          Start Profiling
        </el-button>
      </el-form-item>

      <el-alert
        v-if="!selectedProcess"
        type="info"
        :closable="false"
        show-icon
      >
        <template #title>
          Select a Python process to start profiling
        </template>
      </el-alert>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { VideoPlay } from '@element-plus/icons-vue'
import type { FormInstance, FormRules } from 'element-plus'
import type { NormalizedProcessInfo } from '@/services/pyspy'

interface SamplingConfig {
  duration: number
  rate: number
  format: 'flamegraph' | 'speedscope' | 'raw'
  native: boolean
  subprocesses: boolean
}

interface Props {
  selectedProcess: NormalizedProcessInfo | null
  disabled?: boolean
  loading?: boolean
}

defineProps<Props>()
const emit = defineEmits<{
  start: [config: SamplingConfig]
}>()

const formRef = ref<FormInstance>()
const config = reactive<SamplingConfig>({
  duration: 30,
  rate: 100,
  format: 'flamegraph',
  native: false,
  subprocesses: false
})

const rules: FormRules = {
  duration: [
    { required: true, message: 'Duration is required', trigger: 'blur' },
    { type: 'number', min: 1, max: 300, message: 'Duration must be between 1 and 300', trigger: 'blur' }
  ],
  rate: [
    { required: true, message: 'Rate is required', trigger: 'blur' },
    { type: 'number', min: 1, max: 1000, message: 'Rate must be between 1 and 1000', trigger: 'blur' }
  ]
}

const handleStart = async () => {
  if (!formRef.value) return
  
  await formRef.value.validate((valid) => {
    if (valid) {
      emit('start', { ...config })
    }
  })
}
</script>

<style scoped lang="scss">
@import '@/styles/stats-layout.scss';

.sampling-config {
  height: 100%;

  .form-help {
    display: block;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 4px;
  }

  :deep(.el-form-item) {
    margin-bottom: 18px;
  }

  :deep(.el-input-number) {
    width: 100%;
  }
}
</style>
