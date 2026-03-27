<template>
  <div class="model-square-container">
    <!-- Page title -->
    <div class="page-header">
      <el-text class="block textx-18 font-500" tag="b">Model Square</el-text>
      <div class="subtitle text-gray-500 text-sm mt-1">
        Browse and manage available AI models for inference
      </div>
    </div>

    <!-- Action bar -->
    <div class="flex flex-wrap items-center mt-4">
      <!-- Left side actions -->
      <div class="flex flex-wrap items-center gap-2">
        <el-button
          type="primary"
          round
          :icon="Plus"
          @click="showAddDialog = true"
          class="mb-2 text-black"
        >
          Create Model
        </el-button>
      </div>

      <!-- Right side filters -->
      <div class="flex flex-wrap items-center mt-2 mb-2 sm:mt-0 ml-auto gap-4">
        <!-- Origin filter -->
        <el-segmented
          v-model="filters.origin"
          :options="originOptions"
          @change="handleFilterChange"
          class="mb-2"
        />
        <!-- Access mode filter -->
        <el-select
          v-model="filters.modelType"
          placeholder="All Types"
          clearable
          @change="handleFilterChange"
          style="width: 150px"
          class="mb-2"
        >
          <el-option label="All Types" value="" />
          <el-option label="Local" value="local" />
          <el-option label="Local Path" value="local_path" />
          <el-option label="Remote API" value="remote_api" />
        </el-select>
      </div>
    </div>

    <!-- Model card grid -->
    <div v-if="!loading && models.length > 0" class="model-grid">
      <el-card
        v-for="model in models"
        :key="model.id"
        class="model-card"
        shadow="never"
        :body-style="{ padding: '0' }"
      >
        <div class="card-content">
          <!-- Model info -->
          <div class="model-info">
            <div class="model-title-row">
              <!-- Icon -->
              <div class="model-icon">
                <div
                  v-if="!model.icon || (model as any)._iconLoadFailed"
                  class="model-icon-fallback"
                >
                  <svg
                    class="w-8 h-8 text-gray-500 dark:text-gray-400"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    xmlns="http://www.w3.org/2000/svg"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="1.5"
                      d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
                    />
                  </svg>
                </div>
                <img
                  v-else-if="model.icon && !(model as any)._iconLoadFailed"
                  :src="model.icon"
                  alt="Model Icon"
                  class="model-icon-img"
                  @error="handleIconError($event, model)"
                />
              </div>
              <!-- Name and tag group -->
              <div class="model-info-text">
                <h3 class="model-name">{{ model.displayName || model.id }}</h3>
                <div class="model-type">
                  <el-tag
                    :type="getStatusType(model.phase)"
                    size="small"
                    :effect="isDark ? 'dark' : 'light'"
                  >
                    {{ model.phase }}
                  </el-tag>
                  <el-tag
                    size="small"
                    :type="isDeployableLocalModel(model) ? 'primary' : 'warning'"
                    :effect="isDark ? 'dark' : 'plain'"
                  >
                    {{ model.accessMode || 'Unknown' }}
                  </el-tag>
                  <el-tag
                    v-if="model.origin === 'fine_tuned'"
                    size="small"
                    type="success"
                    :effect="isDark ? 'dark' : 'plain'"
                  >
                    SFT
                  </el-tag>
                </div>
              </div>
            </div>
            <p class="model-description">
              {{ model.description || 'No description available' }}
            </p>
            <!-- SFT model metadata -->
            <div
              v-if="model.origin === 'fine_tuned'"
              class="text-xs text-gray-400 mt-1"
            >
              <span v-if="model.userName">By {{ model.userName }}</span>
              <span v-if="model.userName && model.baseModel"> · </span>
              <span v-if="model.baseModel">Base: {{ model.baseModel }}</span>
            </div>

            <!-- Resource info -->
            <div v-if="model.cpu || model.gpu || model.memory" class="model-resources">
              <span v-if="model.gpu && model.gpu !== '0'" class="resource-item">
                <i class="i-ep-cpu text-xs"></i> GPU: {{ model.gpu }}
              </span>
              <span v-if="model.cpu && model.cpu !== '0'" class="resource-item">
                <i class="i-ep-cpu text-xs"></i> CPU: {{ model.cpu }}
              </span>
              <span v-if="model.memory && model.memory !== '0Gi'" class="resource-item">
                <i class="i-ep-coin text-xs"></i> {{ model.memory }}
              </span>
            </div>

            <!-- Tags -->
            <div
              v-if="model.categorizedTags && model.categorizedTags.length > 0"
              class="model-tags"
            >
              <el-tag
                v-for="(tag, index) in model.categorizedTags.slice(0, 5)"
                :key="index"
                size="small"
                :type="getTagColorType(tag.color)"
                effect="plain"
                class="mr-1"
              >
                {{ tag.value }}
              </el-tag>
              <span v-if="model.categorizedTags.length > 5" class="more-tags">
                +{{ model.categorizedTags.length - 5 }}
              </span>
            </div>

            <!-- Status message -->
            <div v-if="model.message" class="model-message">
              <el-tooltip :content="model.message" placement="top">
                <span class="text-xs text-gray-500 truncate block">{{ model.message }}</span>
              </el-tooltip>
            </div>

            <!-- Time info -->
            <div class="model-time">
              <span class="time-label">Updated:</span>
              <span class="time-value">{{
                formatTimeStr(model.updatedAt || model.createdAt)
              }}</span>
            </div>
          </div>

          <!-- Action button area -->
          <div class="model-actions">
            <el-button
              size="small"
              type="primary"
              @click="openChat(model)"
              :disabled="model.phase !== 'Ready'"
            >
              <el-icon><ChatLineSquare /></el-icon>
              Chat
            </el-button>
            <div class="action-buttons">
              <el-tooltip content="View Details" placement="top">
                <el-button
                  size="small"
                  @click="handleCommand('detail', model)"
                  circle
                  class="btn-icon btn-detail"
                >
                  <el-icon><View /></el-icon>
                </el-button>
              </el-tooltip>
              <!-- Retry button for Failed state -->
              <el-tooltip v-if="model.phase === 'Failed'" content="Retry" placement="top">
                <el-button
                  size="small"
                  @click="handleCommand('retry', model)"
                  circle
                  class="btn-icon btn-retry"
                >
                  <el-icon><RefreshRight /></el-icon>
                </el-button>
              </el-tooltip>
              <!-- Start/Stop buttons (shown for deployable local models) -->
              <el-tooltip
                v-else-if="isDeployableLocalModel(model)"
                :content="!model.serviceID ? 'Start Service' : 'Stop Service'"
                placement="top"
              >
                <el-button
                  v-if="!model.serviceID"
                  size="small"
                  @click="handleCommand('start', model)"
                  circle
                  class="btn-icon btn-start"
                  :disabled="model.phase !== 'Ready'"
                >
                  <el-icon><VideoPlay /></el-icon>
                </el-button>
                <el-button
                  v-else
                  size="small"
                  @click="handleCommand('stop', model)"
                  circle
                  class="btn-icon btn-stop"
                  :disabled="model.phase !== 'Ready'"
                >
                  <el-icon><VideoPause /></el-icon>
                </el-button>
              </el-tooltip>
              <!-- SFT button -->
              <el-tooltip
                v-if="canSft(model)"
                content="SFT"
                placement="top"
              >
                <el-button
                  size="small"
                  @click="handleCommand('sft', model)"
                  circle
                  class="btn-icon btn-sft"
                >
                  <el-icon><MagicStick /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="Delete Model" placement="top">
                <el-button
                  size="small"
                  @click="handleCommand('delete', model)"
                  circle
                  class="btn-icon btn-delete"
                >
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Empty state -->
    <el-empty v-else-if="!loading" description="No models found" :image-size="200">
      <template #image>
        <el-icon :size="100" color="#C0C4CC">
          <Box />
        </el-icon>
      </template>
    </el-empty>

    <!-- Loading state -->
    <div v-if="loading" class="loading-container">
      <el-skeleton :rows="6" animated />
    </div>

    <!-- Add model dialog -->
    <AddModelDialog v-model:visible="showAddDialog" @success="handleAddSuccess" />

    <!-- Stop service dialog -->
    <ToggleServiceDialog
      v-model:visible="showToggleDialog"
      :model="currentToggleModel"
      @success="handleToggleSuccess"
    />

    <!-- Infer create dialog -->
    <InferAddDialog
      v-model:visible="showInferDialog"
      :wlid="currentInferWlid"
      :action="inferAction"
      :prefill-data="inferPrefillData"
      @success="handleInferSuccess"
    />

    <!-- Select Infer dialog (Local type) -->
    <SelectInferDialog
      v-model:visible="showSelectInferDialog"
      :model-id="currentSelectModel?.id || ''"
      @confirm="handleInferSelected"
    />

    <!-- Create SFT dialog -->
    <CreateSftDialog
      v-model:visible="showSftDialog"
      :model="currentSftModel"
      @success="handleSftSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus,
  ChatLineSquare,
  View,
  Delete,
  VideoPlay,
  VideoPause,
  Box,
  RefreshRight,
  MagicStick,
} from '@element-plus/icons-vue'
import { useDark } from '@vueuse/core'
import { formatTimeStr } from '@/utils'
import {
  getModelsList,
  deleteModel,
  retryModel,
  getModelWorkloadConfig,
  isDeployableLocalModel,
  canSft,
  type PlaygroundModel,
  type ModelsListParams,
  type ModelsListResp,
} from '@/services/playground'
import AddModelDialog from './Components/AddModelDialog.vue'
import ToggleServiceDialog from './Components/ToggleServiceDialog.vue'
import CreateSftDialog from './Components/CreateSftDialog.vue'
import InferAddDialog from '@/pages/Infer/Components/AddDialog.vue'
import SelectInferDialog from './Components/SelectInferDialog.vue'
import { useWorkspaceStore } from '@/stores/workspace'

const router = useRouter()
const isDark = useDark()
const wsStore = useWorkspaceStore()

// State
const loading = ref(false)
const models = ref<PlaygroundModel[]>([])
const total = ref(0)
const showAddDialog = ref(false)
const showToggleDialog = ref(false)
const currentToggleModel = ref<PlaygroundModel | null>(null)
const showInferDialog = ref(false)
const currentInferWlid = ref<string>('')
const inferAction = ref('Create')
const inferPrefillData = ref<Record<string, unknown>>({})
const showSelectInferDialog = ref(false)
const currentSelectModel = ref<PlaygroundModel | null>(null)
const showSftDialog = ref(false)
const currentSftModel = ref<PlaygroundModel | null>(null)

// Filter criteria
const filters = reactive({
  modelType: '',
  origin: '',
  search: '',
})

// Get status type
const getStatusType = (phase: string) => {
  const statusMap: Record<string, string> = {
    Ready: 'success',
    Running: 'success',
    Stopped: 'info',
    Pending: 'warning',
    Failed: 'danger',
  }
  return statusMap[phase] || 'info'
}

// Map backend-returned color to Element Plus tag type
const getTagColorType = (color: string) => {
  const colorMap: Record<string, string> = {
    blue: 'primary',
    green: 'success',
    purple: 'purple', // Custom purple type
    orange: 'warning',
    gray: 'info',
    red: 'danger',
  }
  return colorMap[color.toLowerCase()] || 'info'
}

// Origin filter options
const originOptions = [
  { label: 'All', value: '' },
  { label: 'Imported', value: 'external' },
  { label: 'SFT', value: 'fine_tuned' },
]

// Handle image load error
const handleIconError = (event: Event, model: PlaygroundModel & { _iconLoadFailed?: boolean }) => {
  const target = event.target as HTMLImageElement
  // Remove src to show fallback icon
  target.style.display = 'none'
  // Set a flag to avoid retrying
  model._iconLoadFailed = true
}

// Fetch model list
const fetchModels = async () => {
  loading.value = true
  try {
    const params: ModelsListParams = {}

    if (filters.modelType) params.accessMode = filters.modelType
    if (filters.origin) params.origin = filters.origin
    if (wsStore.currentWorkspaceId) params.workspace = wsStore.currentWorkspaceId

    const res = (await getModelsList(params)) as unknown as ModelsListResp
    models.value = res.items || []
    total.value = res.total || 0
  } catch (_error) {
    ElMessage.error('Failed to load models')
  } finally {
    loading.value = false
  }
}

// Handle filter change
const handleFilterChange = () => {
  fetchModels()
}

// Open chat
const openChat = (model: PlaygroundModel) => {
  // Remote API type: navigate directly
  if (model.accessMode === 'remote_api') {
    if (model.phase !== 'Ready') {
      ElMessage.warning('Model is not ready yet.')
      return
    }

    router.push({
      path: '/playground-agent',
      query: {
        modelId: model.id,
        serviceId: model.id, // Remote API uses modelId as serviceId
        modelIcon: model.icon || '',
      },
    })
    return
  }

  // Local type: show select Infer dialog
  currentSelectModel.value = model
  showSelectInferDialog.value = true
}

// Handle Infer selection
const handleInferSelected = (
  inferId: string,
  service: { id: string; displayName: string } | undefined,
) => {
  if (!currentSelectModel.value) return

  router.push({
    path: '/playground-agent',
    query: {
      modelId: currentSelectModel.value.id,
      serviceId: inferId,
      modelName: service?.displayName || '',
      modelIcon: currentSelectModel.value.icon || '',
    },
  })
}

// Handle dropdown command
const handleCommand = async (command: string, model: PlaygroundModel) => {
  switch (command) {
    case 'detail':
      router.push(`/model-square/detail/${model.id}`)
      break
    case 'start':
      await handleStartModel(model)
      break
    case 'stop':
      await handleStopModel(model)
      break
    case 'retry':
      await handleRetryModel(model)
      break
    case 'delete':
      await handleDeleteModel(model)
      break
    case 'sft':
      currentSftModel.value = model
      showSftDialog.value = true
      break
  }
}

// Start model service (only for Local type)
const handleStartModel = async (model: PlaygroundModel) => {
  // Check if current workspace exists
  if (!wsStore.currentWorkspaceId) {
    ElMessage.warning('Please select a workspace first')
    return
  }

  try {
    // Get workload config
    const config = (await getModelWorkloadConfig(
      model.id,
      wsStore.currentWorkspaceId,
    )) as unknown as {
      displayName: string
      description: string
      entryPoint: string
      image: string
      env?: Record<string, string>
      labels?: Record<string, string>
      cpu: string
      memory: string
      gpu: string
      replica: number
      ephemeralStorage: string
      service?: {
        protocol: string
        port: number
        targetPort: number
        serviceType: string
        nodePort?: number
      }
    }

    // Map data to Infer AddDialog
    inferPrefillData.value = {
      displayName: config.displayName,
      description: config.description,
      entryPoint: config.entryPoint,
      image: config.image,
      env: config.env || {},
      labels: config.labels || {},
      cpu: config.cpu,
      memory: config.memory,
      gpu: config.gpu,
      replica: config.replica,
      ephemeralStorage: config.ephemeralStorage,
      service: config.service || {},
    }

    currentInferWlid.value = ''
    inferAction.value = 'Create'
    showInferDialog.value = true
  } catch (_error) {
    const error = _error as { message?: string }
    ElMessage.error(error?.message || 'Failed to get workload config')
  }
}

// Stop model service (only for Local type)
const handleStopModel = async (model: PlaygroundModel) => {
  currentToggleModel.value = model
  showToggleDialog.value = true
}

// Retry model
const handleRetryModel = async (model: PlaygroundModel) => {
  try {
    await ElMessageBox.confirm(
      `Retry model "${model.displayName}"? This will attempt to restart the failed model.`,
      'Retry Model',
      {
        confirmButtonText: 'Retry',
        cancelButtonText: 'Cancel',
        type: 'warning',
      },
    )

    loading.value = true
    await retryModel(model.id)
    ElMessage.success('Model retry initiated successfully')
    await fetchModels()
  } catch (_error) {
    if (_error !== 'cancel') {
      console.error('Failed to retry model:', _error)
      ElMessage.error('Failed to retry model')
    }
  } finally {
    loading.value = false
  }
}

// Handle service toggle success
const handleToggleSuccess = () => {
  showToggleDialog.value = false
  currentToggleModel.value = null
  fetchModels()
}

// Handle Infer creation success
const handleInferSuccess = () => {
  showInferDialog.value = false
  currentInferWlid.value = ''
  inferPrefillData.value = {}
  fetchModels()
  // Navigate to Infer list page and show my workloads
  router.push({
    path: '/infer',
    query: {
      onlyMyself: 'My Workloads',
    },
  })
}

// Handle SFT job creation success
const handleSftSuccess = (workloadId: string) => {
  showSftDialog.value = false
  currentSftModel.value = null
  router.push({
    path: '/training/detail',
    query: { id: workloadId },
  })
}

// Delete model
const handleDeleteModel = async (model: PlaygroundModel) => {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to delete "${model.displayName}"? This action cannot be undone.`,
      'Delete Model',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      },
    )

    await deleteModel(model.id)
    ElMessage.success('Model deleted successfully')
    fetchModels()
  } catch (_error) {
    if (_error !== 'cancel') {
      ElMessage.error('Failed to delete model')
    }
  }
}

// Handle add success
const handleAddSuccess = () => {
  showAddDialog.value = false
  fetchModels()
}

// Initialize
onMounted(() => {
  fetchModels()
})

// Watch for workspace changes, auto refresh list
watch(
  () => wsStore.currentWorkspaceId,
  (newWorkspaceId, oldWorkspaceId) => {
    if (newWorkspaceId !== oldWorkspaceId) {
      fetchModels()
    }
  },
)
</script>

<style scoped lang="scss">
.text-black {
  color: #333 !important;
}

.mb-2 {
  margin-bottom: 8px;
}

.ml-auto {
  margin-left: auto;
}

// Purple tag custom styles
:deep(.el-tag--purple) {
  background: rgba(155, 81, 224, 0.1);
  border: 1px solid rgba(155, 81, 224, 0.2);
  color: #9b51e0;

  &.is-plain {
    background: rgba(155, 81, 224, 0.1);
    border-color: rgba(155, 81, 224, 0.3);
  }
}

.model-square-container {
  padding: 0;

  .page-header {
    margin-bottom: 20px;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 6px;
    display: inline-block;

    &.ready,
    &.running {
      background-color: #67c23a;
    }
    &.stopped {
      background-color: #909399;
    }
    &.pending {
      background-color: #e6a23c;
    }
    &.failed {
      background-color: #f56c6c;
    }
  }

  .model-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
    gap: 20px;
    margin-top: 20px;
    grid-auto-rows: 1fr;

    // Ensure el-card fills grid item height
    :deep(.el-card) {
      height: 100%;
      display: flex;
      flex-direction: column;

      .el-card__body {
        flex: 1;
        display: flex;
        flex-direction: column;
        padding: 0;
      }
    }

    // Add subtle animation delay to different cards
    .model-card {
      animation: float 6s ease-in-out infinite;

      &:nth-child(1) {
        animation-delay: 0s;
      }
      &:nth-child(2) {
        animation-delay: 0.2s;
      }
      &:nth-child(3) {
        animation-delay: 0.4s;
      }
      &:nth-child(4) {
        animation-delay: 0.6s;
      }
      &:nth-child(5) {
        animation-delay: 0.8s;
      }
      &:nth-child(6) {
        animation-delay: 1s;
      }
      &:nth-child(7) {
        animation-delay: 1.2s;
      }
      &:nth-child(8) {
        animation-delay: 1.4s;
      }
      &:nth-child(9) {
        animation-delay: 1.6s;
      }
      &:nth-child(10) {
        animation-delay: 1.8s;
      }
      &:nth-child(11) {
        animation-delay: 2s;
      }
      &:nth-child(12) {
        animation-delay: 2.2s;
      }
    }
  }

  @keyframes float {
    0%,
    100% {
      transform: translateY(0);
    }
    50% {
      transform: translateY(-3px);
    }
  }

  .model-card {
    transition: all 0.3s ease;
    border-radius: 16px;
    overflow: hidden;
    background: linear-gradient(
      135deg,
      rgba(255, 255, 255, 0.1) 0%,
      rgba(255, 255, 255, 0.05) 100%
    );
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border: 1px solid rgba(255, 255, 255, 0.18);
    box-shadow:
      0 8px 32px 0 rgba(31, 38, 135, 0.12),
      inset 0 1px 0 0 rgba(255, 255, 255, 0.2);
    position: relative;
    display: flex;
    flex-direction: column;
    height: 100%;

    // Top glow effect
    &::after {
      content: '';
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      height: 100px;
      background: radial-gradient(ellipse at 50% 0%, rgba(255, 255, 255, 0.12) 0%, transparent 70%);
      opacity: 0;
      transition: opacity 0.3s;
      pointer-events: none;
    }

    &:hover {
      animation-play-state: paused; // Pause float animation on hover
      transform: translateY(-8px) scale(1.02);
      box-shadow:
        0 16px 48px 0 rgba(31, 38, 135, 0.3),
        inset 0 1px 0 0 rgba(255, 255, 255, 0.3),
        0 0 20px rgba(102, 126, 234, 0.2);
      background: linear-gradient(
        135deg,
        rgba(255, 255, 255, 0.15) 0%,
        rgba(255, 255, 255, 0.08) 100%
      );
      border: 1px solid rgba(255, 255, 255, 0.25);

      &::after {
        opacity: 1;
      }
    }

    .card-content {
      padding: 0;
      position: relative;
      z-index: 1;
      display: flex;
      flex-direction: column;
      height: 100%;
    }

    .model-info {
      padding: 20px;
      flex: 1;

      .model-title-row {
        display: flex;
        gap: 12px;
        margin-bottom: 12px;

        .model-icon {
          width: 48px;
          height: 48px;
          display: flex;
          align-items: center;
          justify-content: center;
          border-radius: 8px;
          flex-shrink: 0;
          overflow: hidden;

          .model-icon-img {
            width: 100%;
            height: 100%;
            object-fit: cover;
            border-radius: 8px;
          }

          .model-icon-fallback {
            display: flex;
            align-items: center;
            justify-content: center;
          }
        }

        .model-info-text {
          flex: 1;
          display: flex;
          flex-direction: column;
          justify-content: center;
          gap: 6px;
          min-height: 48px;

          .model-name {
            font-size: 16px;
            font-weight: 600;
            margin: 0;
            color: var(--el-text-color-primary);
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
            line-height: 1.3;
          }

          .model-type {
            gap: 6px;
            display: flex;
            flex-wrap: wrap;
            align-items: center;
          }
        }
      }

      .model-description {
        font-size: 14px;
        color: var(--el-text-color-regular);
        line-height: 1.5;
        margin: 12px 0;
        height: 42px;
        overflow: hidden;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        -webkit-box-orient: vertical;
      }

      .model-resources {
        display: flex;
        gap: 12px;
        margin: 8px 0;
        font-size: 12px;
        color: var(--el-text-color-secondary);

        .resource-item {
          display: flex;
          align-items: center;
          gap: 4px;
        }
      }

      .model-tags {
        margin: 12px 0;
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 6px;

        :deep(.el-tag) {
          // background: rgba(102, 126, 234, 0.1);
          // border: 1px solid rgba(102, 126, 234, 0.2);
          // color: #667eea;
          border-radius: 6px;
          padding: 2px 8px;

          &.el-tag--info {
            background: rgba(144, 147, 153, 0.1);
            border: 1px solid rgba(144, 147, 153, 0.2);
            color: #909399;
          }
        }

        .more-tags {
          font-size: 12px;
          color: var(--el-text-color-secondary);
        }
      }

      .model-message {
        margin: 8px 0;
        font-size: 12px;
        color: var(--el-text-color-secondary);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .model-time {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        margin-top: 12px;

        .time-label {
          margin-right: 4px;
        }
      }
    }

    .model-actions {
      padding: 12px 16px;
      border-top: 1px solid rgba(255, 255, 255, 0.08);
      background: rgba(255, 255, 255, 0.02);
      backdrop-filter: blur(8px);
      position: relative;
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-top: auto;

      .action-buttons {
        display: flex;
        gap: 6px;
        align-items: center;

        .btn-icon {
          width: 32px;
          height: 32px;
          padding: 0;
          background: rgba(255, 255, 255, 0.05);
          border: 1px solid rgba(255, 255, 255, 0.1);

          .el-icon {
            font-size: 16px;
          }

          &:hover {
            transform: scale(1.1);
            background: rgba(255, 255, 255, 0.1);
          }

          &.btn-detail {
            .el-icon {
              color: #409eff;
            }
            &:hover {
              border-color: #409eff;
              background: rgba(64, 158, 255, 0.1);
            }
          }

          &.btn-start {
            .el-icon {
              color: #67c23a;
            }
            &:hover {
              border-color: #67c23a;
              background: rgba(103, 194, 58, 0.1);
            }
          }

          &.btn-stop {
            .el-icon {
              color: #e6a23c;
            }
            &:hover {
              border-color: #e6a23c;
              background: rgba(230, 162, 60, 0.1);
            }
          }

          &.btn-retry {
            .el-icon {
              color: #409eff;
            }
            &:hover {
              border-color: #409eff;
              background: rgba(64, 158, 255, 0.1);
            }
          }

          &.btn-delete {
            .el-icon {
              color: #f56c6c;
            }
            &:hover {
              border-color: #f56c6c;
              background: rgba(245, 108, 108, 0.1);
            }
          }
        }
      }
    }
  }

  .loading-container {
    padding: 40px;
  }
}

// Dark mode adaptation
.dark {
  // Purple tag dark mode adaptation
  :deep(.el-tag--purple) {
    background: rgba(155, 81, 224, 0.15);
    border-color: rgba(155, 81, 224, 0.35);
    color: #bb86fc;

    &.is-plain {
      background: rgba(155, 81, 224, 0.15);
      border-color: rgba(155, 81, 224, 0.4);
    }
  }

  .model-card {
    background: linear-gradient(
      135deg,
      rgba(255, 255, 255, 0.08) 0%,
      rgba(255, 255, 255, 0.03) 100%
    );
    border: 1px solid rgba(255, 255, 255, 0.12);
    box-shadow:
      0 8px 32px 0 rgba(0, 0, 0, 0.5),
      inset 0 1px 0 0 rgba(255, 255, 255, 0.1);

    &::after {
      background: radial-gradient(ellipse at 50% 0%, rgba(255, 255, 255, 0.08) 0%, transparent 70%);
      opacity: 0;
    }

    &:hover {
      animation-play-state: paused;
      transform: translateY(-8px) scale(1.02);
      box-shadow:
        0 16px 48px 0 rgba(0, 0, 0, 0.7),
        inset 0 1px 0 0 rgba(255, 255, 255, 0.15),
        0 0 20px rgba(102, 126, 234, 0.15);
      background: linear-gradient(
        135deg,
        rgba(255, 255, 255, 0.1) 0%,
        rgba(255, 255, 255, 0.05) 100%
      );
      border: 1px solid rgba(255, 255, 255, 0.18);

      &::after {
        opacity: 1;
      }
    }

    .model-actions {
      background: rgba(20, 20, 20, 0.5);
      border-top: 1px solid rgba(255, 255, 255, 0.05);
    }
  }
}
</style>
