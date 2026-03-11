<template>
  <el-dialog
    v-model="visible"
    title="Create Model"
    :close-on-click-modal="false"
    width="800"
    destroy-on-close
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      style="max-width: 800px"
      class="p-y-3 p-x-5"
    >
      <!-- Basic information -->
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Basic Information</span>
      </div>

      <el-form-item label="Access Mode" prop="accessMode">
        <el-segmented
          v-model="form.accessMode"
          :options="accessModeOptions"
          @change="handleAccessModeChange"
        />
      </el-form-item>

      <el-form-item
        :label="form.accessMode === 'local' ? 'Hugging Face URL' : 'API Endpoint'"
        prop="sourceUrl"
      >
        <el-input
          v-model="form.sourceUrl"
          :placeholder="
            form.accessMode === 'local'
              ? 'e.g. https://huggingface.co/Qwen/Qwen2.5-7B-Instruct'
              : 'e.g. https://api.deepseek.com/v1'
          "
        />
      </el-form-item>

      <el-form-item label="Workspace" prop="workspace" v-if="form.accessMode === 'local'">
        <el-select
          v-model="form.workspace"
          placeholder="Select workspace (optional)"
          clearable
          class="w-full"
        >
          <el-option
            v-for="ws in workspaceStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
      </el-form-item>

      <template v-if="form.accessMode === 'local'">
        <el-form-item label="Token">
          <el-input
            v-model="form.sourceToken"
            type="password"
            placeholder="e.g. sk-xxxxxxxxxxxxxxxxxxxxx"
            show-password
          />
        </el-form-item>
      </template>

      <!-- Remote API mode: API Token -->
      <template v-if="form.accessMode === 'remote_api'">
        <!-- Model display name -->
        <el-form-item label="Display Name" prop="displayName">
          <el-input v-model="form.displayName" placeholder="e.g. DeepSeek Chat" />
        </el-form-item>

        <!-- Model description -->
        <el-form-item label="Description" prop="description">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            placeholder="Model description"
          />
        </el-form-item>

        <!-- Icon URL -->
        <el-form-item label="Icon URL" prop="icon">
          <el-input v-model="form.icon" placeholder="e.g. https://example.com/icon.png" />
        </el-form-item>

        <!-- Author/Organization -->
        <el-form-item label="Label" prop="label">
          <el-input v-model="form.label" placeholder="e.g. DeepSeek" />
        </el-form-item>

        <!-- Tags -->
        <el-form-item label="Tags" prop="tags">
          <div class="flex gap-2 flex-wrap">
            <el-tag
              v-for="tag in form.tags"
              :key="tag"
              closable
              type="primary"
              effect="plain"
              :disable-transitions="false"
              @close="handleTagClose(tag)"
            >
              {{ tag }}
            </el-tag>
            <el-input
              v-if="tagInputVisible"
              ref="tagInputRef"
              v-model="tagInputValue"
              class="w-20"
              size="small"
              @keyup.enter="handleTagInputConfirm"
              @blur="handleTagInputConfirm"
            />
            <el-button v-else class="button-new-tag" size="small" @click="showTagInput">
              + New Tag
            </el-button>
          </div>
        </el-form-item>

        <!-- Remote API configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Remote API Configuration</span>
        </div>

        <el-form-item label="Model" prop="model" required>
          <el-input v-model="form.model" placeholder="Enter model name" />
        </el-form-item>

        <el-form-item label="API Key" prop="apiKey" required>
          <el-input
            v-model="form.apiKey"
            placeholder="Enter API Key"
            type="password"
            show-password
          />
        </el-form-item>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="submitting"> Create </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules, InputInstance } from 'element-plus'
import { createModel } from '@/services/playground'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits(['update:visible', 'success'])

const formRef = ref<FormInstance>()
const submitting = ref(false)
const workspaceStore = useWorkspaceStore()

// Tag input related
const tagInputVisible = ref(false)
const tagInputValue = ref('')
const tagInputRef = ref<InputInstance>()

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

const accessModeOptions = [
  { label: 'Hugging Face (Local)', value: 'local' },
  { label: 'Remote API', value: 'remote_api' },
]

const initialForm = () => ({
  accessMode: 'local',
  sourceUrl: '',
  sourceToken: '',
  workspace: '',
  displayName: '',
  description: '',
  icon: '',
  label: '',
  tags: [] as string[],
  model: '',
  apiKey: '',
})

const form = reactive(initialForm())

const rules: FormRules = {
  accessMode: [{ required: true, message: 'Please select access mode', trigger: 'change' }],
  sourceUrl: [{ required: true, message: 'Please enter source URL', trigger: 'blur' }],
  sourceToken: [
    {
      required: true,
      message: 'Please enter API token',
      trigger: 'blur',
      validator: (rule, value, callback) => {
        if (form.accessMode === 'remote_api' && !value) {
          callback(new Error('API token is required for Remote API mode'))
        } else {
          callback()
        }
      },
    },
  ],
  displayName: [
    {
      required: true,
      message: 'Please enter display name',
      trigger: 'blur',
      validator: (rule, value, callback) => {
        if (form.accessMode === 'remote_api' && !value) {
          callback(new Error('Display name is required for Remote API mode'))
        } else {
          callback()
        }
      },
    },
  ],
  model: [
    {
      required: true,
      message: 'Please enter model name',
      trigger: 'blur',
      validator: (rule, value, callback) => {
        if (form.accessMode === 'remote_api' && !value) {
          callback(new Error('Model name is required for Remote API mode'))
        } else {
          callback()
        }
      },
    },
  ],
  apiKey: [
    {
      required: true,
      message: 'Please enter API Key',
      trigger: 'blur',
      validator: (rule, value, callback) => {
        if (form.accessMode === 'remote_api' && !value) {
          callback(new Error('API Key is required for Remote API mode'))
        } else {
          callback()
        }
      },
    },
  ],
}

// Handle access mode change
const handleAccessModeChange = (value: string) => {
  // Clear some fields
  if (value === 'local') {
    form.sourceToken = ''
    form.workspace = ''
    form.displayName = ''
    form.description = ''
    form.icon = ''
    form.label = ''
    form.tags = []
    form.model = ''
    form.apiKey = ''
  }
}

// Tag-related methods
const handleTagClose = (tag: string) => {
  const index = form.tags.indexOf(tag)
  if (index > -1) {
    form.tags.splice(index, 1)
  }
}

const showTagInput = () => {
  tagInputVisible.value = true
  nextTick(() => {
    tagInputRef.value?.input?.focus()
  })
}

const handleTagInputConfirm = () => {
  if (tagInputValue.value && !form.tags.includes(tagInputValue.value)) {
    form.tags.push(tagInputValue.value)
  }
  tagInputVisible.value = false
  tagInputValue.value = ''
}

// Submit form
const handleSubmit = async () => {
  try {
    await formRef.value?.validate()

    submitting.value = true

    // Build request params
    const payload: any = {
      source: {
        url: form.sourceUrl,
        accessMode: form.accessMode,
        ...(form.sourceToken ? { token: form.sourceToken } : {}),
        modelName: form.model,
        apiKey: form.apiKey,
      },
      ...(form.workspace ? { workspace: form.workspace } : {}),
      ...(form.accessMode === 'remote_api'
        ? {
            displayName: form.displayName,
            description: form.description,
            icon: form.icon,
            label: form.label,
            tags: form.tags,
          }
        : {}),
    }

    await createModel(payload)

    ElMessage.success('Model created successfully')
    emit('success')
    handleClose()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(error?.message || 'Failed to add model')
    }
  } finally {
    submitting.value = false
  }
}

// Close dialog
const handleClose = () => {
  visible.value = false
  // Reset form
  Object.assign(form, initialForm())
  formRef.value?.resetFields()
  // Reset tag input state
  tagInputVisible.value = false
  tagInputValue.value = ''
}
</script>

<style scoped lang="scss">
.text-xs {
  font-size: 12px;
}

.text-gray-500 {
  color: #909399;
}

.mt-1 {
  margin-top: 4px;
}

.flex {
  display: flex;
}

.items-center {
  align-items: center;
}

.gap-2 {
  gap: 8px;
}

.flex-wrap {
  flex-wrap: wrap;
}

.button-new-tag {
  height: 24px;
}

.w-20 {
  width: 80px;
}

.w-full {
  width: 100%;
}

.m-b-4 {
  margin-bottom: 16px;
}

.m-t-6 {
  margin-top: 24px;
}

.p-y-3 {
  padding-top: 12px;
  padding-bottom: 12px;
}

.p-x-5 {
  padding-left: 20px;
  padding-right: 20px;
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
</style>
