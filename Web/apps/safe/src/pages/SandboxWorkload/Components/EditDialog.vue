<template>
  <el-dialog
    :model-value="visible"
    title="Edit SandBox Workload"
    width="480"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <el-form
      ref="formRef"
      :model="form"
      label-width="auto"
      class="p-x-4 p-y-2"
      v-loading="loading"
    >
      <el-form-item label="Priority">
        <el-select v-model="form.priority" class="w-full">
          <el-option
            v-for="(label, val) in PRIORITY_LABEL_MAP"
            :key="val"
            :label="label"
            :value="Number(val)"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Timeout">
        <el-input-number
          v-model="form.timeout"
          :min="0"
          :step="1"
          controls-position="right"
          class="w-full"
        />
        <div class="text-xs text-gray-400 mt-1">Duration in seconds. 0 means no timeout.</div>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="emit('update:visible', false)">Cancel</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">Save</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance } from 'element-plus'
import { editWorkload, getWorkloadDetail } from '@/services'
import { type PriorityValue, PRIORITY_LABEL_MAP } from '@/services/workload/type'

const props = defineProps<{
  visible: boolean
  workloadId: string
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: []
}>()

const formRef = ref<FormInstance>()
const loading = ref(false)
const submitting = ref(false)

const form = reactive({
  priority: 1 as number,
  timeout: 0 as number,
})

const loadWorkload = async () => {
  if (!props.workloadId) return
  loading.value = true
  try {
    const res = await getWorkloadDetail(props.workloadId)
    form.priority = (res as any)?.priority ?? 1
    form.timeout = (res as any)?.timeout ?? 0
  } catch {
    ElMessage.error('Failed to load workload details')
  } finally {
    loading.value = false
  }
}

const handleSubmit = async () => {
  submitting.value = true
  try {
    await editWorkload(props.workloadId, {
      priority: form.priority,
    })
    ElMessage.success('Workload updated')
    emit('success')
    emit('update:visible', false)
  } catch (err) {
    ElMessage.error((err as Error).message || 'Failed to update workload')
  } finally {
    submitting.value = false
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val && props.workloadId) loadWorkload()
  },
)
</script>
