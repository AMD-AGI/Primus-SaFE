<template>
  <div v-if="detailData" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
          Back
        </el-button>
        <h1 class="w-name">{{ detailData.displayName }}</h1>

        <div class="flex items-center gap-2 ml-4">
          <el-tag :type="getStatusType(detailData.phase)" :effect="isDark ? 'plain' : 'light'">
            {{ detailData.phase }}
          </el-tag>
          <el-tag
            size="small"
            :type="isDeployable ? 'primary' : 'warning'"
            :effect="isDark ? 'dark' : 'plain'"
          >
            {{ detailData.accessMode || 'Unknown' }}
          </el-tag>
          <el-tag
            v-if="detailData.origin === 'fine_tuned'"
            size="small"
            type="success"
            :effect="isDark ? 'dark' : 'plain'"
          >
            Fine-tuned
          </el-tag>
          <el-tag
            v-else-if="detailData.accessMode === 'local'"
            size="small"
            type="info"
            :effect="isDark ? 'dark' : 'plain'"
          >
            Base
          </el-tag>
          <el-tag
            v-if="detailData.origin === 'rl_trained'"
            size="small"
            type="warning"
            :effect="isDark ? 'dark' : 'plain'"
          >
            RL
          </el-tag>
        </div>
      </div>

      <div class="w-actions">
        <el-tooltip content="Chat" placement="top">
          <el-button circle class="glass-btn glass-btn--primary" @click="openChat">
            <el-icon><ChatDotRound /></el-icon>
          </el-button>
        </el-tooltip>

        <el-tooltip
          v-if="canTrainModel"
          content="Train"
          placement="top"
        >
          <el-button circle class="glass-btn glass-btn--success" @click="showTrainDialog = true">
            <el-icon><MagicStick /></el-icon>
          </el-button>
        </el-tooltip>

        <el-tooltip :content="canStartService ? 'Start Service' : 'Stop Service'" placement="top">
          <el-button
            circle
            :class="
              canStartService ? 'glass-btn glass-btn--success' : 'glass-btn glass-btn--warning'
            "
            @click="handleToggleService"
          >
            <el-icon v-if="canStartService"><VideoPlay /></el-icon>
            <el-icon v-else><VideoPause /></el-icon>
          </el-button>
        </el-tooltip>

        <el-tooltip content="Delete" placement="top">
          <el-button circle class="glass-btn glass-btn--danger" @click="onDelete">
            <el-icon><Delete /></el-icon>
          </el-button>
        </el-tooltip>
      </div>
    </div>

    <div class="w-meta">
      <span class="item">
        <span class="label">ID</span>
        <code class="code">{{ detailData.id }}</code>
        <el-icon
          class="copy"
          size="12"
          style="color: var(--safe-primary)"
          @click="copyText(detailData.id)"
        >
          <CopyDocument />
        </el-icon>
      </span>
      <span class="sep">•</span>
      <span class="item">
        <span class="label">Created</span>{{ formatTimeStr(detailData.createdAt) }}
      </span>
      <span class="sep">•</span>
      <span class="item">
        <span class="label">Updated</span>{{ formatTimeStr(detailData.updatedAt) || '-' }}
      </span>
      <span class="sep" v-if="detailData.label">•</span>
      <span class="item" v-if="detailData.label">
        <span class="label">Label</span>{{ detailData.label }}
      </span>
      <span class="sep" v-if="detailData.description">•</span>
      <span class="item" v-if="detailData.description">
        <span class="label">Description</span>
        <el-tooltip
          :content="detailData.description"
          placement="top"
          :disabled="!detailData.description || detailData.description.length <= 50"
        >
          <span class="truncate max-w-[42ch] cursor-help">
            {{ detailData.description }}
          </span>
        </el-tooltip>
      </span>
    </div>
  </div>

  <!-- <el-tabs v-model="activeTab" class="mt-4">
    <el-tab-pane label="Overview" name="overview"> -->
  <!-- Model Information -->
  <el-card class="mt-6 safe-card" shadow="never">
    <div class="flex items-center">
      <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
      <span class="textx-15 font-medium">Model Information</span>
    </div>

    <el-descriptions v-if="detailData" class="m-t-4" border :column="4" direction="vertical">
      <el-descriptions-item label="Display Name">{{ detailData.displayName }}</el-descriptions-item>
      <el-descriptions-item label="Access Mode">{{ detailData.accessMode }}</el-descriptions-item>
      <el-descriptions-item label="Phase">{{ detailData.phase }}</el-descriptions-item>
      <el-descriptions-item label="Version">{{ detailData.version || '-' }}</el-descriptions-item>
      <el-descriptions-item v-if="detailData.origin" label="Origin">
        <el-tag
          size="small"
          :type="detailData.origin === 'fine_tuned' ? 'success' : detailData.origin === 'rl_trained' ? 'warning' : 'info'"
        >
          {{ detailData.origin === 'fine_tuned' ? 'Fine-tuned' : detailData.origin === 'rl_trained' ? 'RL' : 'Base' }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item v-if="detailData.userName" label="Owner">
        {{ detailData.userName }}
      </el-descriptions-item>
      <el-descriptions-item v-if="detailData.baseModel" label="Base Model">
        {{ detailData.baseModel }}
      </el-descriptions-item>
      <el-descriptions-item v-if="detailData.sftJobId" label="Training Job">
        <el-link type="primary" :underline="false" @click="goToSftJob">
          {{ detailData.sftJobId }}
          <el-icon class="ml-1"><Right /></el-icon>
        </el-link>
      </el-descriptions-item>
      <el-descriptions-item v-if="detailData.workspace" label="Workspace">
        {{ detailData.workspace }}
      </el-descriptions-item>
    </el-descriptions>
  </el-card>

  <!-- Resource Configuration -->
  <el-card
    class="mt-4 safe-card"
    shadow="never"
    v-if="detailData?.cpu || detailData?.gpu || detailData?.memory"
  >
    <div class="flex items-center">
      <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
      <span class="textx-15 font-medium">Resource Configuration</span>
    </div>

    <el-descriptions class="m-t-4" border :column="3" direction="vertical">
      <el-descriptions-item label="CPU" v-if="detailData?.cpu && detailData?.cpu !== '0'">
        {{ detailData?.cpu }}
      </el-descriptions-item>
      <el-descriptions-item label="GPU" v-if="detailData?.gpu && detailData?.gpu !== '0'">
        {{ detailData?.gpu }}
      </el-descriptions-item>
      <el-descriptions-item
        label="Memory"
        v-if="detailData?.memory && detailData?.memory !== '0Gi'"
      >
        {{ detailData?.memory }}
      </el-descriptions-item>
    </el-descriptions>
  </el-card>

  <!-- Source Configuration -->
  <el-card class="mt-4 safe-card" shadow="never" v-if="detailData?.sourceURL">
    <div class="flex items-center">
      <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
      <span class="textx-15 font-medium">Source Configuration</span>
    </div>

    <el-descriptions class="m-t-4" border :column="2" direction="vertical">
      <el-descriptions-item label="Source URL" :span="2">
        <el-link :href="detailData?.sourceURL" target="_blank" type="primary">
          {{ detailData?.sourceURL }}
        </el-link>
      </el-descriptions-item>
      <el-descriptions-item label="Download Type" v-if="detailData?.downloadType">
        {{ detailData?.downloadType }}
      </el-descriptions-item>
      <el-descriptions-item label="Local Path" v-if="detailData?.localPath">
        {{ detailData?.localPath }}
      </el-descriptions-item>
    </el-descriptions>
  </el-card>

  <!-- Inference Service -->
  <el-card class="mt-4 safe-card" shadow="never" v-if="detailData?.workloadID">
    <div class="flex items-center">
      <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
      <span class="textx-15 font-medium">Inference Service</span>
    </div>

    <el-descriptions class="m-t-4" border :column="2" direction="vertical">
      <el-descriptions-item label="Workload ID">
        <el-link type="primary" :underline="false" @click="goToInferDetail" style="cursor: pointer">
          {{ detailData?.workloadID }}
          <el-icon class="ml-1"><Right /></el-icon>
        </el-link>
      </el-descriptions-item>
      <el-descriptions-item label="Inference Phase">
        <el-tag size="small" :type="detailData?.inferencePhase === 'Running' ? 'success' : 'info'">
          {{ detailData?.inferencePhase }}
        </el-tag>
      </el-descriptions-item>
    </el-descriptions>
  </el-card>

  <!-- Tags -->
  <el-card
    class="mt-4 safe-card"
    shadow="never"
    v-if="detailData?.categorizedTags && detailData.categorizedTags.length > 0"
  >
    <div class="flex items-center">
      <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
      <span class="textx-15 font-medium">Tags</span>
    </div>

    <div class="m-t-4 flex flex-wrap gap-2">
      <el-tag
        v-for="(tag, index) in detailData.categorizedTags"
        :key="index"
        :type="getTagColorType(tag.color)"
        effect="plain"
      >
        {{ tag.value }}
      </el-tag>
    </div>
  </el-card>
  <!-- </el-tab-pane>
  </el-tabs> -->

  <!-- Toggle Service Dialog -->
  <ToggleServiceDialog
    v-if="toggleDialogVisible"
    :visible="toggleDialogVisible"
    @update:visible="toggleDialogVisible = $event"
    :model="detailData"
    :is-starting="canStartService"
    @success="handleToggleSuccess"
  />

  <!-- Create Training Dialog -->
  <CreateTrainingDialog
    v-model:visible="showTrainDialog"
    :model="detailData"
    @success="handleTrainSuccess"
  />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDark } from '@vueuse/core'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  CopyDocument,
  Delete,
  ChatDotRound,
  VideoPlay,
  VideoPause,
  Right,
  ArrowLeft,
  MagicStick,
} from '@element-plus/icons-vue'
import { getModelDetail, deleteModel, isDeployableLocalModel, canTrain } from '@/services/playground'
import { copyText, formatTimeStr } from '@/utils/index'
import ToggleServiceDialog from './Components/ToggleServiceDialog.vue'
import CreateTrainingDialog from './Components/CreateTrainingDialog.vue'

const route = useRoute()
const router = useRouter()
const isDark = useDark()

const modelId = computed(() => route.params.id as string)
const detailData = ref<any>(null)
const toggleDialogVisible = ref(false)
const showTrainDialog = ref(false)

const isDeployable = computed(() => {
  if (!detailData.value) return false
  return isDeployableLocalModel(detailData.value)
})

const canTrainModel = computed(() => {
  if (!detailData.value) return false
  return canTrain(detailData.value)
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

// Map backend color to Element Plus tag type
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

// Determine if service can be started
const canStartService = computed(() => {
  return !detailData.value?.serviceID || detailData.value?.inferencePhase !== 'Running'
})

// Fetch details
const getDetail = async () => {
  try {
    const res = await getModelDetail(modelId.value)
    detailData.value = res
  } catch (error) {
    console.error('Failed to fetch model detail:', error)
    ElMessage.error('Failed to load model details')
  }
}

// Open chat
const openChat = () => {
  const modelName = detailData.value?.displayName || detailData.value?.id
  const modelIcon = detailData.value?.icon || ''
  router.push({
    path: '/playground-agent',
    query: {
      modelId: modelId.value,
      modelName,
      modelIcon,
    },
  })
}

// Navigate to inference detail page
const goToInferDetail = () => {
  if (detailData.value?.workloadID) {
    router.push({
      path: '/infer/detail',
      query: {
        id: detailData.value.workloadID,
      },
    })
  }
}

// Toggle service
const handleToggleService = () => {
  toggleDialogVisible.value = true
}

const handleToggleSuccess = () => {
  getDetail()
}

// Navigate to SFT job detail
const goToSftJob = () => {
  if (detailData.value?.sftJobId) {
    router.push({
      path: '/training/detail',
      query: { id: detailData.value.sftJobId },
    })
  }
}

// Handle training success
const handleTrainSuccess = (workloadId: string) => {
  showTrainDialog.value = false
  router.push({
    path: '/training/detail',
    query: { id: workloadId },
  })
}

// Delete model
const onDelete = () => {
  const msg = h('span', null, [
    'Are you sure you want to delete model: ',
    h(
      'span',
      { style: 'color: var(--el-color-primary); font-weight: 600' },
      detailData.value.displayName,
    ),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete Model', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteModel(modelId.value)
      ElMessage.success('Model deleted successfully')
      router.push('/model-square')
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

onMounted(() => {
  getDetail()
})
</script>

<style scoped lang="scss">
/* Detail page styles inherited from global styles */

// Custom purple tag style
:deep(.el-tag--purple) {
  background: rgba(155, 81, 224, 0.1);
  border: 1px solid rgba(155, 81, 224, 0.2);
  color: #9b51e0;

  &.is-plain {
    background: rgba(155, 81, 224, 0.1);
    border-color: rgba(155, 81, 224, 0.3);
  }
}

// Dark mode adaptation
.dark {
  :deep(.el-tag--purple) {
    background: rgba(155, 81, 224, 0.15);
    border-color: rgba(155, 81, 224, 0.35);
    color: #bb86fc;

    &.is-plain {
      background: rgba(155, 81, 224, 0.15);
      border-color: rgba(155, 81, 224, 0.4);
    }
  }
}
</style>
