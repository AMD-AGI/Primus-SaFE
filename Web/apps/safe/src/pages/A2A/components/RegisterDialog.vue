<template>
  <el-dialog
    :model-value="visible"
    title="Register Agent"
    width="600"
    @close="$emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
    >
      <el-form-item label="Service Name" prop="serviceName">
        <el-input v-model="form.serviceName" clearable placeholder="e.g. my-agent" />
      </el-form-item>
      <el-form-item label="Display Name" prop="displayName">
        <el-input v-model="form.displayName" clearable placeholder="e.g. My Agent" />
      </el-form-item>
      <el-form-item label="Endpoint" prop="endpoint">
        <el-input v-model="form.endpoint" clearable placeholder="e.g. http://my-agent:8080" />
      </el-form-item>
      <el-form-item label="A2A Path Prefix" prop="a2aPathPrefix">
        <el-input v-model="form.a2aPathPrefix" clearable placeholder="/a2a" />
      </el-form-item>
      <el-form-item label="Description">
        <el-input
          v-model="form.description"
          type="textarea"
          :rows="3"
          placeholder="Optional description"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="$emit('update:visible', false)" :disabled="submitLoading">
          Cancel
        </el-button>
        <el-button type="primary" :loading="submitLoading" @click="onSubmit">
          Register
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, reactive, computed, nextTick } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { registerA2AService } from '@/services'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: []
}>()

const formRef = ref<FormInstance>()
const submitLoading = ref(false)

const initialForm = () => ({
  serviceName: '',
  displayName: '',
  endpoint: '',
  a2aPathPrefix: '/a2a',
  description: '',
})

const form = reactive({ ...initialForm() })

const rules = computed<FormRules>(() => ({
  serviceName: [{ required: true, message: 'Please input service name', trigger: 'blur' }],
  displayName: [{ required: true, message: 'Please input display name', trigger: 'blur' }],
  endpoint: [{ required: true, message: 'Please input endpoint', trigger: 'blur' }],
  a2aPathPrefix: [{ required: true, message: 'Please input A2A path prefix', trigger: 'blur' }],
}))

const onOpen = async () => {
  formRef.value?.resetFields()
  Object.assign(form, initialForm())
  await nextTick()
}

const onSubmit = async () => {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
    submitLoading.value = true
    await registerA2AService(form)
    ElMessage.success('Agent registered successfully')
    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formRef.value?.scrollToField?.(firstKey as any)
      ElMessage.error(firstMsg)
    }
  } finally {
    submitLoading.value = false
  }
}
</script>
