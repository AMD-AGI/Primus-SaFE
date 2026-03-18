<template>
  <el-dialog
    :model-value="visible"
    title="Create S3 Secret"
    width="500"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      class="px-5"
    >
      <el-alert
        type="info"
        :closable="false"
        class="mb-4"
        show-icon
      >
        <template #title>
          Creating secret: <strong>{{ secretName }}</strong>
        </template>
      </el-alert>

      <el-form-item label="Access Key" prop="accessKey">
        <el-input
          v-model="form.accessKey"
          placeholder="Enter access key"
          clearable
        />
      </el-form-item>

      <el-form-item label="Secret Key" prop="secretKey">
        <el-input
          v-model="form.secretKey"
          type="password"
          placeholder="Enter secret key"
          show-password
          clearable
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" @click="onSubmit" :loading="submitting">
          Create
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { reactive, ref, watch } from 'vue'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { addSecret } from '@/services'
import { encodeToBase64String } from '@/utils'

const props = defineProps<{
  visible: boolean
  secretName: string
  workspace: string
}>()

const emit = defineEmits(['update:visible', 'success'])

const ruleFormRef = ref<FormInstance>()
const submitting = ref(false)

const form = reactive({
  accessKey: '',
  secretKey: '',
})

const rules: FormRules = {
  accessKey: [{ required: true, message: 'Please input access key', trigger: 'blur' }],
  secretKey: [{ required: true, message: 'Please input secret key', trigger: 'blur' }],
}

const onSubmit = async () => {
  if (!ruleFormRef.value) return

  try {
    await ruleFormRef.value.validate()

    submitting.value = true

    // Build payload
    const payload = {
      name: props.secretName,
      type: 'general',
      workspaceIds: [props.workspace],
      bindAllWorkspaces: false,
      params: [
        {
          access_key: form.accessKey ? encodeToBase64String(form.accessKey) : '',
          secret_key: form.secretKey ? encodeToBase64String(form.secretKey) : '',
        },
      ],
      labels: {
        'secret.usage': 's3',
      },
    }

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    await addSecret(payload as any)
    ElMessage.success('Secret created successfully')

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      ruleFormRef.value?.scrollToField?.(firstKey)
      ElMessage.error(firstMsg)
    }
  } finally {
    submitting.value = false
  }
}

// Watch dialog open and reset form
watch(
  () => props.visible,
  (val) => {
    if (val) {
      form.accessKey = ''
      form.secretKey = ''
      setTimeout(() => {
        ruleFormRef.value?.clearValidate()
      }, 0)
    }
  },
)
</script>

