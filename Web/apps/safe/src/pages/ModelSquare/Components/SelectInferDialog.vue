<template>
  <el-dialog
    v-model="visible"
    title="Select Inference Service"
    width="500px"
    :close-on-click-modal="false"
  >
    <el-form label-width="150px">
      <el-form-item label="Inference Service">
        <el-select
          v-model="selectedInferId"
          placeholder="Please select inference service"
          class="w-full"
          :loading="loading"
        >
          <el-option
            v-for="service in services"
            :key="service.workloadId"
            :label="service.displayName"
            :value="service.workloadId"
          >
            <div class="flex items-center justify-between">
              <span>{{ service.displayName }}</span>
              <el-tag v-if="service.phase" size="small" :type="getPhaseType(service.phase)">
                {{ service.phase }}
              </el-tag>
            </div>
          </el-option>
        </el-select>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" @click="handleConfirm" :disabled="!selectedInferId">
        Confirm
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getModelWorkloads,
  type ModelWorkload,
  type ModelWorkloadsResp,
} from '@/services/playground'

const props = defineProps<{
  visible: boolean
  modelId?: string
}>()

const emit = defineEmits(['update:visible', 'confirm'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

const loading = ref(false)
const services = ref<ModelWorkload[]>([])
const selectedInferId = ref('')

const getPhaseType = (phase: string) => {
  const typeMap: Record<string, string> = {
    Running: 'success',
    Pending: 'warning',
    Failed: 'danger',
    Succeeded: 'success',
  }
  return typeMap[phase] || 'info'
}

const fetchServices = async () => {
  if (!props.modelId) {
    ElMessage.warning('Please select a model first')
    return
  }

  loading.value = true
  try {
    const res = (await getModelWorkloads(props.modelId)) as unknown as ModelWorkloadsResp
    services.value = res?.items || []
  } catch (_error) {
    const error = _error as { message?: string }
    ElMessage.error(error?.message || 'Failed to load inference services')
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  visible.value = false
  selectedInferId.value = ''
}

const handleConfirm = () => {
  if (!selectedInferId.value) {
    ElMessage.warning('Please select an inference service')
    return
  }

  const selectedService = services.value.find((w) => w.workloadId === selectedInferId.value)
  emit('confirm', selectedInferId.value, selectedService)
  handleClose()
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      fetchServices()
    }
  },
)
</script>

<style scoped>
.w-full {
  width: 100%;
}

.flex {
  display: flex;
}

.items-center {
  align-items: center;
}

.justify-between {
  justify-content: space-between;
}
</style>
