<template>
  <el-dialog
    v-model="visible"
    title="Create Model"
    :close-on-click-modal="false"
    width="820"
    destroy-on-close
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="currentRules"
      label-width="auto"
      style="max-width: 800px"
      class="p-y-3 p-x-5"
    >
      <!-- Access Mode Tabs -->
      <el-segmented
        v-model="form.accessMode"
        :options="accessModeOptions"
        class="m-b-6 form-seg"
        @change="handleAccessModeChange"
      />

      <!-- ═══ Hugging Face ═══ -->
      <template v-if="form.accessMode === 'local'">
        <el-form-item label="Hugging Face URL" prop="sourceUrl">
          <el-input
            v-model="form.sourceUrl"
            placeholder="e.g. https://huggingface.co/Qwen/Qwen2.5-7B-Instruct"
          />
        </el-form-item>

        <el-form-item label="Workspace" prop="workspace">
          <el-select v-model="form.workspace" placeholder="Select workspace (optional)" clearable class="w-full">
            <el-option
              v-for="ws in workspaceStore.items"
              :key="ws.workspaceId"
              :label="ws.workspaceName"
              :value="ws.workspaceId"
            />
          </el-select>
        </el-form-item>

        <VolumeSelector
          v-if="form.workspace"
          v-model="form.targetVolume"
          :volumes="currentVolumes"
        />

        <el-form-item label="Token">
          <el-input
            v-model="form.sourceToken"
            type="password"
            placeholder="HF token (for private models)"
            show-password
          />
        </el-form-item>
      </template>

      <!-- ═══ Remote API ═══ -->
      <template v-if="form.accessMode === 'remote_api'">
        <el-form-item label="API Endpoint" prop="sourceUrl">
          <el-input v-model="form.sourceUrl" placeholder="e.g. https://api.deepseek.com/v1" />
        </el-form-item>

        <el-form-item label="Display Name" prop="displayName">
          <el-input v-model="form.displayName" placeholder="e.g. DeepSeek Chat" />
        </el-form-item>

        <el-form-item label="Description">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="Model description" />
        </el-form-item>

        <el-form-item label="Icon URL">
          <el-input v-model="form.icon" placeholder="e.g. https://example.com/icon.png" />
        </el-form-item>

        <el-form-item label="Label">
          <el-input v-model="form.label" placeholder="e.g. DeepSeek" />
        </el-form-item>

        <TagsInput v-model="form.tags" />

        <div class="section-divider">
          <div class="section-bar" />
          <span class="section-title">Remote API Configuration</span>
        </div>

        <el-form-item label="Model" prop="modelName">
          <el-input v-model="form.modelName" placeholder="Enter model name" />
        </el-form-item>

        <el-form-item label="API Key" prop="apiKey">
          <el-input v-model="form.apiKey" placeholder="Enter API Key" type="password" show-password />
        </el-form-item>
      </template>

      <!-- ═══ Existing Path (local_path) ═══ -->
      <template v-if="form.accessMode === 'local_path'">
        <el-form-item label="Display Name" prop="displayName">
          <el-input v-model="form.displayName" placeholder="e.g. my-custom-llm (lowercase, no spaces)" />
          <div class="el-form-item__tip">Used as K8s resource name prefix. Only lowercase letters, numbers, '-', '_', '.' allowed.</div>
        </el-form-item>

        <el-form-item label="Local Path" prop="localPath">
          <el-input v-model="form.localPath" placeholder="e.g. /shared_aig/models/my-custom-llm" />
        </el-form-item>

        <el-form-item label="Workspace">
          <el-select v-model="form.workspace" placeholder="Public (all workspaces)" clearable class="w-full">
            <el-option
              v-for="ws in workspaceStore.items"
              :key="ws.workspaceId"
              :label="ws.workspaceName"
              :value="ws.workspaceId"
            />
          </el-select>
        </el-form-item>

        <el-collapse v-model="showOptionalFields" class="m-t-2 m-b-4">
          <el-collapse-item title="Optional Metadata" name="optional">
            <el-form-item label="Model Name">
              <el-input v-model="form.modelName" placeholder="e.g. my-team/my-custom-llm" />
            </el-form-item>

            <el-form-item label="Source URL">
              <el-input v-model="form.sourceUrl" placeholder="e.g. https://huggingface.co/org/model (for reference)" />
            </el-form-item>

            <el-form-item label="Icon URL">
              <el-input v-model="form.icon" placeholder="e.g. https://example.com/icon.png" />
            </el-form-item>

            <el-form-item label="Label">
              <el-input v-model="form.label" placeholder="e.g. my-team" />
            </el-form-item>

            <TagsInput v-model="form.tags" />

            <el-form-item label="Max Tokens">
              <el-input-number v-model="form.maxTokens" :min="1" :max="1048576" style="width: 200px" />
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </template>

      <!-- ═══ Import from S3 (s3_sync) ═══ -->
      <template v-if="form.accessMode === 's3_sync'">
        <el-form-item label="Display Name" prop="displayName">
          <el-input v-model="form.displayName" placeholder="e.g. imported-model (lowercase, no spaces)" />
          <div class="el-form-item__tip">Used as K8s resource name prefix. Only lowercase letters, numbers, '-', '_', '.' allowed.</div>
        </el-form-item>

        <el-form-item label="S3 URI" prop="s3Uri">
          <el-input v-model="form.s3Uri" placeholder="s3://my-bucket/models/llm-prefix" />
          <div class="el-form-item__tip">Data will be downloaded directly from your S3 to workspace shared storage.</div>
        </el-form-item>

        <el-form-item label="Workspace" prop="workspace">
          <el-select v-model="form.workspace" placeholder="Select workspace" clearable class="w-full" @change="handleWorkspaceChange">
            <el-option
              v-for="ws in workspaceStore.items"
              :key="ws.workspaceId"
              :label="ws.workspaceName"
              :value="ws.workspaceId"
            />
          </el-select>
        </el-form-item>

        <VolumeSelector
          v-if="form.workspace"
          v-model="form.targetVolume"
          :volumes="currentVolumes"
        />

        <el-form-item label="Subpath">
          <el-input v-model="form.targetSubpath" placeholder="e.g. external/2026 (optional)" />
          <div class="el-form-item__tip">Final path: &lt;volume&gt;/[subpath/]models/&lt;name&gt;. Only A-Za-z0-9._-/ allowed, no '..' segments.</div>
        </el-form-item>

        <el-collapse v-model="showS3Credentials" class="m-t-2 m-b-4">
          <el-collapse-item title="S3 Credentials (optional)" name="s3cred">
            <div class="el-form-item__tip m-b-4">Leave empty to use platform S3 credentials for internal buckets.</div>

            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="Access Key ID">
                  <el-input v-model="form.s3AccessKeyId" placeholder="AKIA..." />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Secret Access Key">
                  <el-input v-model="form.s3SecretAccessKey" type="password" show-password placeholder="Secret..." />
                </el-form-item>
              </el-col>
            </el-row>

            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="Region">
                  <el-input v-model="form.s3Region" placeholder="e.g. us-west-2" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item :label="s3EndpointRequired ? 'Endpoint *' : 'Endpoint'" prop="s3Endpoint">
                  <el-input v-model="form.s3Endpoint" placeholder="e.g. https://s3.us-west-2.amazonaws.com" />
                </el-form-item>
              </el-col>
            </el-row>
          </el-collapse-item>
        </el-collapse>

        <el-collapse v-model="showOptionalFields" class="m-t-2 m-b-4">
          <el-collapse-item title="Optional Metadata" name="optional">
            <el-form-item label="Icon URL">
              <el-input v-model="form.icon" placeholder="e.g. https://example.com/icon.png" />
            </el-form-item>

            <el-form-item label="Label">
              <el-input v-model="form.label" placeholder="e.g. data-team" />
            </el-form-item>

            <TagsInput v-model="form.tags" />
          </el-collapse-item>
        </el-collapse>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="submitting">Create</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { createModel } from '@/services/playground'
import type { CreateModelPayload } from '@/services/playground'
import { useWorkspaceStore } from '@/stores/workspace'
import type { WorkspaceVolume } from '@/services/base/type'
import TagsInput from './TagsInput.vue'
import VolumeSelector from './VolumeSelector.vue'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits(['update:visible', 'success'])

const formRef = ref<FormInstance>()
const submitting = ref(false)
const workspaceStore = useWorkspaceStore()
const showOptionalFields = ref<string[]>([])
const showS3Credentials = ref<string[]>([])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

const accessModeOptions = [
  { label: 'Hugging Face', value: 'local' },
  { label: 'Remote API', value: 'remote_api' },
  { label: 'Existing Path', value: 'local_path' },
  { label: 'Import from S3', value: 's3_sync' },
]

const initialForm = () => ({
  accessMode: 'local' as string,
  sourceUrl: '',
  sourceToken: '',
  workspace: '',
  displayName: '',
  description: '',
  icon: '',
  label: '',
  tags: [] as string[],
  modelName: '',
  apiKey: '',
  localPath: '',
  maxTokens: undefined as number | undefined,
  // S3 fields
  s3Uri: '',
  s3AccessKeyId: '',
  s3SecretAccessKey: '',
  s3Region: '',
  s3Endpoint: '',
  // Target volume
  targetVolume: '',
  targetSubpath: '',
})

const form = reactive(initialForm())

// Whether S3 endpoint is required (when AK/SK is provided)
const s3EndpointRequired = computed(() =>
  !!(form.s3AccessKeyId || form.s3SecretAccessKey),
)

// Get volumes for selected workspace
const currentVolumes = computed<WorkspaceVolume[]>(() => {
  if (!form.workspace) return []
  const ws = workspaceStore.items.find((i) => i.workspaceId === form.workspace)
  return ws?.volumes || []
})

// ── Validation rules per mode ──

const k8sNamePattern = /^[a-z0-9][a-z0-9._-]*$/
const s3UriPattern = /^s3:\/\/[a-zA-Z0-9][a-zA-Z0-9._\-/]*$/
const subpathPattern = /^[A-Za-z0-9._\-/]*$/

const baseRules: FormRules = {}

const localRules: FormRules = {
  sourceUrl: [{ required: true, message: 'Please enter Hugging Face URL', trigger: 'blur' }],
}

const remoteApiRules: FormRules = {
  sourceUrl: [{ required: true, message: 'Please enter API endpoint', trigger: 'blur' }],
  displayName: [{ required: true, message: 'Please enter display name', trigger: 'blur' }],
  modelName: [{ required: true, message: 'Please enter model name', trigger: 'blur' }],
  apiKey: [{ required: true, message: 'Please enter API key', trigger: 'blur' }],
}

const localPathRules: FormRules = {
  displayName: [
    { required: true, message: 'Please enter display name', trigger: 'blur' },
    { pattern: k8sNamePattern, message: 'Only lowercase letters, numbers, -, _, . allowed (must start with letter/number)', trigger: 'blur' },
  ],
  localPath: [{ required: true, message: 'Please enter local path', trigger: 'blur' }],
}

const s3SyncRules: FormRules = {
  displayName: [
    { required: true, message: 'Please enter display name', trigger: 'blur' },
    { pattern: k8sNamePattern, message: 'Only lowercase letters, numbers, -, _, . allowed (must start with letter/number)', trigger: 'blur' },
  ],
  s3Uri: [
    { required: true, message: 'Please enter S3 URI', trigger: 'blur' },
    { pattern: s3UriPattern, message: 'Must start with s3:// and contain valid characters only', trigger: 'blur' },
  ],
  s3Endpoint: [
    {
      validator: (_rule, _value, callback) => {
        if (s3EndpointRequired.value && !form.s3Endpoint) {
          callback(new Error('Endpoint is required when AK/SK is provided'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
}

const currentRules = computed<FormRules>(() => {
  switch (form.accessMode) {
    case 'local': return localRules
    case 'remote_api': return remoteApiRules
    case 'local_path': return localPathRules
    case 's3_sync': return s3SyncRules
    default: return baseRules
  }
})

const handleAccessModeChange = () => {
  formRef.value?.clearValidate()
}

const handleWorkspaceChange = () => {
  form.targetVolume = ''
}

// ── Build payload per mode ──

function buildPayload(): CreateModelPayload {
  switch (form.accessMode) {
    case 'local':
      return {
        source: {
          accessMode: 'local',
          url: form.sourceUrl,
          ...(form.sourceToken ? { token: form.sourceToken } : {}),
        },
        ...(form.workspace ? { workspace: form.workspace } : {}),
        ...(form.targetVolume ? { target: { volume: form.targetVolume } } : {}),
      }

    case 'remote_api':
      return {
        displayName: form.displayName,
        description: form.description || undefined,
        icon: form.icon || undefined,
        label: form.label || undefined,
        tags: form.tags.length ? form.tags : undefined,
        source: {
          accessMode: 'remote_api',
          url: form.sourceUrl,
          modelName: form.modelName,
          apiKey: form.apiKey,
        },
      }

    case 'local_path':
      return {
        displayName: form.displayName,
        description: form.description || undefined,
        icon: form.icon || undefined,
        label: form.label || undefined,
        tags: form.tags.length ? form.tags : undefined,
        maxTokens: form.maxTokens || undefined,
        origin: 'external',
        ...(form.workspace ? { workspace: form.workspace } : {}),
        source: {
          accessMode: 'local_path',
          localPath: form.localPath,
          modelName: form.modelName || undefined,
          url: form.sourceUrl || undefined,
        },
      }

    case 's3_sync': {
      const hasCredentials = !!(form.s3AccessKeyId && form.s3SecretAccessKey)
      const payload: CreateModelPayload = {
        displayName: form.displayName,
        icon: form.icon || undefined,
        label: form.label || undefined,
        tags: form.tags.length ? form.tags : undefined,
        origin: 'external',
        ...(form.workspace ? { workspace: form.workspace } : {}),
        source: {
          accessMode: 's3_sync',
          modelName: form.displayName,
        },
        s3Source: {
          uri: form.s3Uri,
          ...(hasCredentials
            ? {
                accessKeyId: form.s3AccessKeyId,
                secretAccessKey: form.s3SecretAccessKey,
                endpoint: form.s3Endpoint,
              }
            : {}),
          ...(form.s3Region ? { region: form.s3Region } : {}),
        },
      }
      if (form.targetVolume || form.targetSubpath) {
        payload.target = {
          ...(form.targetVolume ? { volume: form.targetVolume } : {}),
          ...(form.targetSubpath ? { subpath: form.targetSubpath } : {}),
        }
      }
      return payload
    }

    default:
      throw new Error('Unknown access mode')
  }
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  // Additional cross-field validation for S3
  if (form.accessMode === 's3_sync') {
    if ((form.s3AccessKeyId || form.s3SecretAccessKey) && !(form.s3AccessKeyId && form.s3SecretAccessKey)) {
      ElMessage.error('Access Key ID and Secret Access Key must be provided together')
      return
    }
    if (form.targetSubpath && !subpathPattern.test(form.targetSubpath)) {
      ElMessage.error('Subpath can only contain A-Za-z0-9._-/ and must not include ".." segments')
      return
    }
    if (form.targetSubpath?.includes('..')) {
      ElMessage.error('Subpath must not include ".." segments')
      return
    }
  }

  submitting.value = true
  try {
    const payload = buildPayload()
    await createModel(payload)
    ElMessage.success('Model created successfully')
    emit('success')
    handleClose()
  } catch (error: any) {
    ElMessage.error(error?.message || 'Failed to create model')
  } finally {
    submitting.value = false
  }
}

const handleClose = () => {
  visible.value = false
  Object.assign(form, initialForm())
  formRef.value?.resetFields()
  showOptionalFields.value = []
  showS3Credentials.value = []
}
</script>

<style scoped lang="scss">
.section-divider {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 20px 0 16px;
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
.el-form-item__tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
  line-height: 1.4;
}
.w-full {
  width: 100%;
}
.m-b-6 {
  margin-bottom: 24px;
}
.m-b-4 {
  margin-bottom: 16px;
}
.m-t-2 {
  margin-top: 8px;
}
.p-y-3 {
  padding-top: 12px;
  padding-bottom: 12px;
}
.p-x-5 {
  padding-left: 20px;
  padding-right: 20px;
}
</style>
