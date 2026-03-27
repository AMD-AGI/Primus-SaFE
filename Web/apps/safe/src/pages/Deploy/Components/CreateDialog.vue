<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    :title="action === 'Create' ? 'Create Deployment Request' : 'Rollback Deployment'"
    width="960px"
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      label-width="150px"
      :rules="rules"
      v-loading="loadingDetail"
    >
      <template v-if="form.type === 'safe'">
      <el-form-item v-if="loadingEnv || Object.keys(currentImageVersions).length > 0" label="Current Versions">
        <div v-if="loadingEnv && Object.keys(currentImageVersions).length === 0" class="w-full">
          <el-text type="info" size="small">
            <el-icon class="is-loading mr-1"><Loading /></el-icon>Loading versions...
          </el-text>
        </div>
        <div v-else class="w-full">
          <el-button link type="primary" size="small" @click="showVersions = !showVersions">
            <el-icon class="expand-icon" :class="{ 'is-expanded': showVersions }"><ArrowRight /></el-icon>
            {{ showVersions ? 'Hide' : 'Show' }} {{ Object.keys(currentImageVersions).length }} components
          </el-button>
          <div v-show="showVersions" class="version-card">
            <div v-for="(ver, comp) in currentImageVersions" :key="comp" class="version-row">
              <span class="version-comp">{{ comp }}</span>
              <code class="version-value">{{ ver }}</code>
            </div>
          </div>
        </div>
      </el-form-item>

      <el-form-item label="Image Versions">
        <div class="w-full">
          <div v-for="(item, index) in form.imageVersions" :key="index" class="flex gap-2 mb-2">
            <el-select
              v-model="item.component"
              placeholder="Select Component"
              class="flex-1"
              filterable
            >
              <el-option
                v-for="comp in availableComponents"
                :key="comp"
                :label="comp"
                :value="comp"
              />
            </el-select>
            <el-input v-model="item.version" placeholder="e.g., v1.2.3 or latest" class="flex-1" />
            <el-button
              :icon="Delete"
              circle
              @click="removeImageVersion(index)"
              :disabled="form.imageVersions.length === 1"
            />
          </div>
          <el-button :icon="Plus" @click="addImageVersion" size="small"> Add Component </el-button>
        </div>
      </el-form-item>

      <el-form-item label="Environment Config">
        <div class="w-full flex flex-col gap-2">
          <div class="flex gap-2">
            <el-button size="small" @click="loadCurrentEnv" :loading="loadingEnv">
              Load Current
            </el-button>
            <el-button size="small" @click="clearEnvConfig"> Clear </el-button>
          </div>
          <el-input
            v-model="form.envConfig"
            type="textarea"
            :rows="12"
            placeholder="# Environment variables configuration&#10;# Leave empty to keep current configuration&#10;&#10;KEY1=value1&#10;KEY2=value2"
          />
        </div>
      </el-form-item>
      </template>

      <template v-else>
        <el-form-item label="Branch" prop="branch">
          <el-input v-model="form.branch" placeholder="e.g., main" />
        </el-form-item>

        <el-form-item label="Control Plane Config" prop="controlPlaneConfig">
          <div class="w-full flex flex-col gap-2">
            <div class="flex gap-2">
              <el-button size="small" @click="loadCurrentEnv" :loading="loadingEnv">
                Load Current
              </el-button>
              <el-button size="small" @click="clearLensConfigs"> Clear </el-button>
            </div>
            <el-input
              v-model="form.controlPlaneConfig"
              type="textarea"
              :rows="10"
              placeholder="# Full Control Plane values.yaml"
            />
          </div>
        </el-form-item>

        <el-form-item label="Data Plane Config" prop="dataPlaneConfig">
          <el-input
            v-model="form.dataPlaneConfig"
            type="textarea"
            :rows="10"
            placeholder="# Full Data Plane values.yaml"
          />
        </el-form-item>

        <el-alert
          v-if="lensComponentsMessage"
          type="info"
          :closable="false"
          show-icon
          class="mb-4"
          :title="lensComponentsMessage"
        />
      </template>

      <el-form-item label="Description" prop="description">
        <el-input
          v-model="form.description"
          type="textarea"
          :rows="3"
          placeholder="Enter deployment description..."
          maxlength="500"
          show-word-limit
        />
      </el-form-item>

      <el-alert
        type="info"
        :closable="false"
        show-icon
        class="mb-4"
        title="Note: After creation, the deployment request requires admin approval before execution."
      />
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:visible', false)">Cancel</el-button>
      <el-button type="primary" @click="handleSubmit" :loading="loading">
        {{ action === 'Create' ? 'Create' : 'Create Rollback Request' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Delete, ArrowRight, Loading } from '@element-plus/icons-vue'
import {
  createDeployment,
  getEnvConfig,
  getComponents,
  getDeploymentDetail,
} from '@/services/deploy'
import type {
  DeploymentRequest,
  DeploymentType,
  EnvConfigResponse,
  ComponentsResponse,
  CreateDeploymentRequest,
} from '@/services/deploy/type'

interface Props {
  visible: boolean
  action: 'Create' | 'Rollback'
  rollbackData?: DeploymentRequest | null
  defaultType?: DeploymentType
}

interface ImageVersion {
  component: string
  version: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'success', type: DeploymentType): void
}>()

const formRef = ref<FormInstance>()
const loading = ref(false)
const loadingEnv = ref(false)
const loadingDetail = ref(false)
const availableComponents = ref<string[]>([])
const lensComponentsMessage = ref('')
const currentImageVersions = ref<Record<string, string>>({})
const showVersions = ref(false)

const form = reactive({
  type: 'safe' as DeploymentType,
  branch: 'main',
  controlPlaneConfig: '',
  dataPlaneConfig: '',
  description: '',
  imageVersions: [{ component: '', version: '' }] as ImageVersion[],
  envConfig: '',
})

const rules: FormRules = {
  description: [
    {
      required: true,
      trigger: 'blur',
      validator: (_rule, value, callback) => {
        if (!String(value || '').trim()) {
          callback(new Error('Description is required'))
        } else {
          callback()
        }
      },
    },
  ],
  branch: [
    {
      trigger: 'blur',
      validator: (_rule, value, callback) => {
        if (form.type === 'lens' && !String(value || '').trim()) {
          callback(new Error('Branch is required for Lens'))
        } else {
          callback()
        }
      },
    },
  ],
  controlPlaneConfig: [
    {
      trigger: 'blur',
      validator: (_rule, value, callback) => {
        if (form.type === 'lens' && !String(value || '').trim()) {
          callback(new Error('Control plane config is required for Lens'))
        } else {
          callback()
        }
      },
    },
  ],
  dataPlaneConfig: [
    {
      trigger: 'blur',
      validator: (_rule, value, callback) => {
        if (form.type === 'lens' && !String(value || '').trim()) {
          callback(new Error('Data plane config is required for Lens'))
        } else {
          callback()
        }
      },
    },
  ],
}

const addImageVersion = () => {
  form.imageVersions.push({ component: '', version: '' })
}

const removeImageVersion = (index: number) => {
  if (form.imageVersions.length > 1) {
    form.imageVersions.splice(index, 1)
  }
}

const loadCurrentEnv = async () => {
  try {
    loadingEnv.value = true
    const res: EnvConfigResponse = await getEnvConfig(form.type)

    if (form.type === 'safe') {
      form.envConfig = ('env_file_config' in res ? res.env_file_config : '') || ''
      const versions = 'image_versions' in res ? res.image_versions : undefined
      currentImageVersions.value = versions || {}
    } else {
      if ('control_plane_config' in res) {
        form.branch = res.branch || form.branch || 'main'
        form.controlPlaneConfig = res.control_plane_config || ''
        form.dataPlaneConfig = res.data_plane_config || ''
      }
    }

    ElMessage.success('Loaded current configuration')
  } catch (error) {
    console.error('Failed to load env config:', error)
  } finally {
    loadingEnv.value = false
  }
}

const clearEnvConfig = () => {
  form.envConfig = ''
}

const clearLensConfigs = () => {
  form.controlPlaneConfig = ''
  form.dataPlaneConfig = ''
}

const loadComponents = async () => {
  try {
    const res: ComponentsResponse = await getComponents(form.type)
    if (form.type === 'safe') {
      availableComponents.value = ('components' in res ? res.components : []) || []
      lensComponentsMessage.value = ''
    } else {
      availableComponents.value = []
      lensComponentsMessage.value = ('message' in res ? res.message : '') || ''
    }
  } catch (error) {
    console.error('Failed to load components:', error)
  }
}

const loadRollbackData = async (deploymentId: number) => {
  try {
    loadingDetail.value = true
    const detail = await getDeploymentDetail(deploymentId)

    // Load type (default safe for legacy data)
    form.type = ((detail as any).deploy_type || 'safe') as DeploymentType

    if (form.type === 'lens') {
      form.branch = detail.branch || 'main'
      form.controlPlaneConfig = detail.control_plane_config || ''
      form.dataPlaneConfig = detail.data_plane_config || ''
      // best-effort message for lens
      await loadComponents()
    } else {
    // Load image versions
    if (detail.image_versions && Object.keys(detail.image_versions).length > 0) {
      form.imageVersions = Object.entries(detail.image_versions).map(([component, version]) => ({
        component,
        version,
      }))
    } else {
      form.imageVersions = [{ component: '', version: '' }]
    }

    // Load environment config
    form.envConfig = detail.env_file_config || ''
    }

    // Load description
    form.description = detail.description || ''
  } catch (error) {
    console.error('Failed to load deployment detail:', error)
    ElMessage.error('Failed to load deployment detail')
  } finally {
    loadingDetail.value = false
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
    loading.value = true

    const payload: CreateDeploymentRequest = {
      type: form.type,
    }
    if (form.description.trim()) {
      payload.description = form.description.trim()
    }

    if (form.type === 'safe') {
      // Prepare image versions (safe)
      const imageVersions: Record<string, string> = {}
      form.imageVersions.forEach((item) => {
        if (item.component && item.version) {
          imageVersions[item.component] = item.version
        }
      })
      if (Object.keys(imageVersions).length > 0) {
        payload.image_versions = imageVersions
      }
      if (form.envConfig.trim()) {
        payload.env_file_config = form.envConfig.trim()
      }
    } else {
      // Lens full replacement configs
      payload.branch = form.branch.trim() || 'main'
      payload.control_plane_config = form.controlPlaneConfig.trim()
      payload.data_plane_config = form.dataPlaneConfig.trim()
    }

    if (props.action === 'Rollback' && props.rollbackData?.id) {
      // Add rollback_from_id for rollback action
      payload.rollback_from_id = props.rollbackData.id
    }

    await createDeployment(payload)

    ElMessage.success(
      props.action === 'Create'
        ? 'Deployment request created successfully'
        : 'Rollback request created successfully',
    )
    emit('update:visible', false)
    emit('success', form.type)
  } catch (error) {
    console.error('Failed to create deployment:', error)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      if (props.action === 'Rollback' && props.rollbackData?.id) {
        // Load data from rollback source via API
        loadRollbackData(props.rollbackData.id)
      } else {
        // Create mode: reset form and auto load current config
        form.type = props.defaultType || 'safe'
        form.branch = 'main'
        form.controlPlaneConfig = ''
        form.dataPlaneConfig = ''
        form.description = ''
        form.imageVersions = [{ component: '', version: '' }]
        form.envConfig = ''
        lensComponentsMessage.value = ''
        currentImageVersions.value = {}
        showVersions.value = false
        loadComponents()
        loadCurrentEnv()
      }
    }
  },
)

</script>

<style scoped>
.expand-icon {
  transition: transform 0.2s;
}
.expand-icon.is-expanded {
  transform: rotate(90deg);
}
.version-card {
  margin-top: 8px;
  background: var(--el-fill-color-light, #f5f7fa);
  border-radius: 6px;
  padding: 4px 0;
  display: grid;
  grid-template-columns: repeat(2, 1fr);
}
.version-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 12px;
}
.version-row:nth-child(odd) {
  border-right: 1px solid var(--el-border-color-extra-light, #f0f0f0);
}
.version-comp {
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
}
.version-value {
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  color: var(--el-text-color-primary, #303133);
  background: none;
  padding: 0;
}
</style>
