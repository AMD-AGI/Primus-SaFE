<template>
  <el-dialog
    :model-value="visible"
    :title="`Create Datasync`"
    width="750"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 700px"
      class="p-5"
      :rules="rules"
    >
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Basic Information</span>
      </div>

      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" placeholder="Please enter datasync task name" />
      </el-form-item>

      <el-form-item label="Workspace" prop="workspace">
        <el-input v-model="form.workspace" disabled />
      </el-form-item>

      <el-form-item label="Secret" prop="secret">
        <div class="flex items-center gap-2 w-full">
          <el-select
            v-model="form.secret"
            placeholder="Select secret"
            filterable
            clearable
            class="flex-1"
          >
            <el-option
              v-for="item in secretOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
          <el-button :icon="Plus" @click="openCreateSecretDialog" text title="Create S3 secret" />
        </div>
      </el-form-item>

      <el-form-item label="Url" prop="endpoint">
        <el-input
          v-model="form.endpoint"
          type="textarea"
          :rows="3"
          placeholder="S3 URL format: endpoint/bucket[/path] (e.g., http://s3.example.com/bucket-name/file-name or http://s3.example.com/bucket-name/sub-dir/)"
        />
        <div class="text-[12px] text-gray-400 mt-1">
          <el-icon class="mr-1"><InfoFilled /></el-icon>
          Supports endpoint/bucket/dir/ or endpoint/bucket/file (must be S3 address)
        </div>
      </el-form-item>

      <el-form-item label="Destination Path" prop="destPath">
        <el-input v-model="form.destPath" placeholder="e.g., my_path" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>

  <!-- S3 Secret creation dialog -->
  <CreateS3SecretDialog
    v-model:visible="secretDialogVisible"
    :secret-name="form.name"
    :workspace="form.workspace"
    @success="onSecretCreated"
  />
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, ref } from 'vue'
import { addOpsjobs, getSecrets } from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { InfoFilled, Plus } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import CreateS3SecretDialog from './CreateS3SecretDialog.vue'

const wsStore = useWorkspaceStore()

defineProps<{
  visible: boolean
}>()
const emit = defineEmits(['update:visible', 'success'])

const secretOptions = ref<Array<{ label: string; value: string }>>([])

const initialForm = () => ({
  name: '',
  workspace: wsStore.currentWorkspaceId || '',
  secret: '',
  endpoint: '',
  destPath: '',
})

const form = reactive({ ...initialForm() })

const ruleFormRef = ref<FormInstance>()

// S3 URL validation rules
const validateS3Endpoint = (_rule: unknown, value: string, callback: (error?: Error) => void) => {
  if (!value) {
    return callback(new Error('Please input endpoint'))
  }

  // Simple URL format validation - supports endpoint/bucket, endpoint/bucket/dir/, endpoint/bucket/file
  const s3UrlPattern = /^https?:\/\/.+\/.+/
  if (!s3UrlPattern.test(value.trim())) {
    return callback(new Error('Invalid S3 URL format. Expected: endpoint/bucket[/path]'))
  }

  callback()
}

const rules = reactive<FormRules>({
  name: [{ required: true, message: 'Please input name', trigger: 'blur' }],
  workspace: [{ required: true, message: 'Workspace is required', trigger: 'blur' }],
  secret: [{ required: true, message: 'Please select secret', trigger: 'change' }],
  endpoint: [
    { required: true, message: 'Please input endpoint', trigger: 'blur' },
    { validator: validateS3Endpoint, trigger: 'blur' },
  ],
  destPath: [{ required: true, message: 'Please input destination path', trigger: 'blur' }],
})

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    const payload = {
      name: form.name,
      inputs: [
        {
          name: 'workspace',
          value: form.workspace,
        },
        {
          name: 'secret',
          value: form.secret,
        },
        {
          name: 'endpoint',
          value: form.endpoint,
        },
        {
          name: 'dest.path',
          value: form.destPath,
        },
      ],
      type: 'download',
      timeoutSecond: 7200,
    }

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    await addOpsjobs(payload as any)
    ElMessage.success('Create successful')

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey)
      ElMessage.error(firstMsg)
    }
  }
}

// Fetch secret list from backend, filter type=general with labels['secret.usage']=s3
const fetchSecrets = async () => {
  try {
    const res = await getSecrets({
      type: 'general',
      'labels[secret.usage]': 's3', // Use labels[key]=value format
    })
    secretOptions.value =
      res?.items?.map((item: { secretName?: string; name?: string }) => ({
        label: item.secretName || item.name || '',
        value: item.secretName || item.name || '',
      })) || []

    // Show a warning if no matching secrets are found
    if (secretOptions.value.length === 0) {
      ElMessage.warning('No S3 secrets found with label secret.usage=s3')
    }
  } catch (err) {
    console.error('Failed to fetch secrets:', err)
    // Use empty list on failure
    secretOptions.value = []
  }
}

const onOpen = async () => {
  await fetchSecrets()

  // Clear form
  Object.assign(form, initialForm())
  form.workspace = wsStore.currentWorkspaceId || ''

  // Clear validation errors in next event loop to ensure form validation system is fully initialized
  setTimeout(() => {
    ruleFormRef.value?.clearValidate()
  }, 0)
}

// ===== Secret creation feature =====
const secretDialogVisible = ref(false)

// Open create secret dialog
const openCreateSecretDialog = () => {
  if (!form.name) {
    ElMessage.warning('Please input name first')
    return
  }
  secretDialogVisible.value = true
}

// Secret creation success callback
const onSecretCreated = async () => {
  // Refresh secret list
  await fetchSecrets()

  // Auto-select newly created secret (name matches download name)
  const newSecret = secretOptions.value.find((s) => s.value === form.name)
  if (newSecret) {
    form.secret = newSecret.value
    ElMessage.success('Secret created and selected')
  }
}
</script>
