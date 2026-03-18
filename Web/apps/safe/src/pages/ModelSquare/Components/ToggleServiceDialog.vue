<template>
  <el-dialog v-model="visible" title="Stop Inference Service" width="500">
    <!-- StopServiceConfirm -->
    <div class="stop-confirm">
      <p>Are you sure you want to stop the inference service for this model?</p>
      <p class="text-gray-500 text-sm mt-2">
        This will terminate the running inference pod and free up the allocated resources.
      </p>
    </div>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="submitting">
        Stop Service
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { toggleModel, type PlaygroundModel } from '@/services/playground'

const props = defineProps<{
  visible: boolean
  model: PlaygroundModel | null
}>()

const emit = defineEmits(['update:visible', 'success'])

const submitting = ref(false)

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

// Submit form - only handles StopService
const handleSubmit = async () => {
  if (!props.model) return

  submitting.value = true
  try {
    await toggleModel(props.model.id, { enabled: false })
    ElMessage.success('Inference service stopped successfully')
    emit('success')
    handleClose()
  } catch (_error) {
    const error = _error as { message?: string }
    ElMessage.error(error?.message || 'Failed to stop inference service')
  } finally {
    submitting.value = false
  }
}

// Close dialog
const handleClose = () => {
  visible.value = false
}
</script>

<style scoped lang="scss">
.dialog-form {
  padding: 20px;
}

.start-confirm,
.stop-confirm {
  padding: 30px;
}

.flex {
  display: flex;
}

.items-center {
  align-items: center;
}

.m-b-4 {
  margin-bottom: 16px;
}

.m-t-6 {
  margin-top: 24px;
}

.w-1 {
  width: 4px;
}

.hx-16 {
  height: 16px;
}

.mr-2 {
  margin-right: 8px;
}

.rounded-sm {
  border-radius: 2px;
}

.textx-15 {
  font-size: 15px;
}

.font-medium {
  font-weight: 500;
}

.text-gray-500 {
  color: #909399;
}

.text-sm {
  font-size: 14px;
}

.mt-2 {
  margin-top: 8px;
}
</style>
