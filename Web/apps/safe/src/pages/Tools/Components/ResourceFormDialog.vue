<template>
  <el-dialog
    :model-value="visible"
    :title="isEdit ? 'Edit Resource' : 'Create Resource'"
    width="720px"
    @close="handleClose"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" class="resource-form">
      <!-- === Basic Information === -->
      <div class="section-card">
        <div class="section-header">
          <div class="section-bar"></div>
          <div>
            <div class="section-title">Basic Information</div>
            <div class="section-subtitle">Name, type, image and version</div>
          </div>
        </div>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="name" prop="name">
              <el-input v-model="form.name" placeholder="e.g., gpu-a100" :disabled="isEdit" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="type" prop="type">
              <el-select v-model="form.type" style="width: 100%">
                <el-option label="GPU" value="gpu" />
                <el-option label="CPU" value="cpu" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="image">
              <el-input v-model="form.image" placeholder="container image" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="version">
              <el-input v-model="form.version" placeholder="1.0.0" />
            </el-form-item>
          </el-col>
        </el-row>
      </div>

      <!-- === Resource Limits === -->
      <div class="section-card">
        <div class="section-header">
          <div class="section-bar"></div>
          <div>
            <div class="section-title">Resource Limits</div>
            <div class="section-subtitle">Allocate compute resources and set timeout</div>
          </div>
        </div>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="cpu">
              <el-input v-model="form.resources.cpu" placeholder="1" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="gpu">
              <el-input v-model="form.resources.gpu" placeholder="0" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="memory">
              <el-input v-model="form.resources.memory" placeholder="1">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="ephemeral">
              <el-input v-model="form.resources.ephemeralStorage" placeholder="10">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="timeout">
              <el-input-number
                v-model.number="form.timeout"
                :min="0"
                :step="1"
                class="w-full"
              />
              <el-text size="small" type="info" class="mt-1">
                <el-icon class="mr-1"><InfoFilled /></el-icon>
                timeout duration in seconds
              </el-text>
            </el-form-item>
          </el-col>
        </el-row>
      </div>

      <!-- === Environment Variables === -->
      <div class="section-card">
        <div class="section-header">
          <div class="section-bar"></div>
          <div>
            <div class="section-title">Environment Variables</div>
            <div class="section-subtitle">Key-value pairs injected into the container</div>
          </div>
        </div>

        <div class="env-list">
          <el-row v-for="(e, i) in form.env" :key="i" :gutter="16" class="env-row">
            <el-col :span="10">
              <el-input v-model="e.key" placeholder="KEY" />
            </el-col>
            <el-col :span="12">
              <el-input v-model="e.val" placeholder="value" />
            </el-col>
            <el-col :span="2" class="flex items-center">
              <el-button text type="danger" :icon="Delete" @click="form.env.splice(i, 1)" />
            </el-col>
          </el-row>
          <div
            class="add-btn"
            @click="form.env.push({ key: '', val: '' })"
          >
            <el-icon><Plus /></el-icon> Add Variable
          </div>
        </div>
      </div>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" :loading="saving" @click="handleSave">
        {{ isEdit ? 'Save' : 'Create' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Delete, InfoFilled } from '@element-plus/icons-vue'
import { upsertResource, updateResource, getResource } from '@/services/tools'

const props = defineProps<{
  visible: boolean
  resourceId?: number
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
  success: []
  close: []
}>()

const isEdit = computed(() => !!props.resourceId)
const formRef = ref<FormInstance>()
const saving = ref(false)

const form = reactive({
  name: '',
  type: 'gpu' as 'gpu' | 'cpu',
  image: '',
  version: '1.0.0',
  timeout: 300,
  resources: {
    gpu: '',
    cpu: '',
    memory: '',
    ephemeralStorage: '',
  },
  env: [] as { key: string; val: string }[],
})

const rules: FormRules = {
  name: [{ required: true, message: 'Name is required', trigger: 'blur' }],
  type: [{ required: true, message: 'Type is required', trigger: 'change' }],
}

const resetForm = () => {
  form.name = ''
  form.type = 'gpu'
  form.image = ''
  form.version = '1.0.0'
  form.timeout = 300
  form.resources = { gpu: '', cpu: '', memory: '', ephemeralStorage: '' }
  form.env = []
}

watch(() => props.visible, async (v) => {
  if (!v) return
  if (props.resourceId) {
    try {
      saving.value = true
      const r = await getResource(props.resourceId)
      form.name = r.name
      form.type = r.type
      form.image = r.image
      form.version = r.version
      form.timeout = r.timeout
      form.resources = {
        gpu: r.resources?.gpu || '',
        cpu: r.resources?.cpu || '',
        memory: r.resources?.memory || '',
        ephemeralStorage: r.resources?.ephemeralStorage || '',
      }
      form.env = (r.env || []).map(e => ({ key: e.key, val: e.val }))
    } catch {
      ElMessage.error('Failed to load resource')
      handleClose()
    } finally {
      saving.value = false
    }
  } else {
    resetForm()
  }
})

const handleSave = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  saving.value = true
  try {
    const envPayload = form.env.filter(e => e.key.trim())
    const payload = {
      name: form.name,
      type: form.type,
      image: form.image,
      version: form.version,
      timeout: form.timeout,
      resources: form.resources,
      env: envPayload,
    }
    if (isEdit.value && props.resourceId) {
      await updateResource(props.resourceId, payload)
      ElMessage.success('Resource updated')
    } else {
      await upsertResource(payload)
      ElMessage.success('Resource created')
    }
    emit('success')
    handleClose()
  } catch (e) {
    console.error('Save resource failed:', e)
  } finally {
    saving.value = false
  }
}

const handleClose = () => {
  emit('update:visible', false)
  emit('close')
  setTimeout(resetForm, 300)
}
</script>

<style scoped lang="scss">
.resource-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.section-card {
  padding: 16px 20px;
  border-radius: 10px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
}

.section-header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 14px;
}

.section-bar {
  width: 4px;
  height: 18px;
  border-radius: 999px;
  margin-top: 2px;
  background-color: var(--safe-primary, var(--el-color-primary));
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.2;
}

.section-subtitle {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.env-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.env-row {
  margin-bottom: 0 !important;
}

.add-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
  border: 1px dashed var(--el-border-color);
  border-radius: 6px;
  cursor: pointer;
  transition: color 0.2s, border-color 0.2s;
  width: fit-content;

  &:hover {
    color: var(--el-color-primary);
    border-color: var(--el-color-primary);
  }
}
</style>
