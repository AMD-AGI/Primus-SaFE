<template>
  <div v-if="detailData" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button
          v-if="showBackButton"
          @click="handleBack"
          :icon="ArrowLeft"
          text
          type="primary"
          class="mr-2 mt-1"
        >
          Back
        </el-button>
        <h1 class="w-name">{{ detailData.displayName || detailData.jobName }}</h1>

        <template v-if="detailData.phase === 'Pending'">
          <div class="flex items-center gap-2">
            <el-tooltip
              effect="dark"
              :content="detailData.message || '-'"
              placement="top"
              :disabled="!detailData.message"
            >
              <el-tag
                class="m-l-2 pulse"
                :type="WorkloadPhaseButtonType.Pending?.type || 'info'"
              >
                {{ detailData.phase }}
              </el-tag>
            </el-tooltip>
            <el-tooltip content="Pending Cause Analysis" placement="top">
              <el-icon
                class="pending-cause-icon pulse-scale"
                :size="18"
                @click.stop="navigateToPendingCause"
              >
                <InfoFilled />
              </el-icon>
            </el-tooltip>
          </div>
          <div class="text-sm m-l-2 text-gray-400" v-if="detailData.queuePosition">
            position in queue:{{ detailData.queuePosition }}
          </div>
        </template>
        <template v-else>
          <div class="flex items-center gap-2">
            <el-tag
              class="m-l-2"
              :type="
                (detailData.phase && WorkloadPhaseButtonType[detailData.phase]?.type) || 'info'
              "
              :effect="isDark ? 'plain' : 'light'"
            >
              {{ detailData.phase }}
            </el-tag>
            <el-tooltip
              v-if="
                detailData.phase === 'Failed' && detailData.groupVersionKind?.kind === 'PyTorchJob'
              "
              content="Root Cause Analysis"
              placement="top"
            >
              <el-icon
                class="root-cause-icon"
                :size="18"
                @click.stop="
                  router.push({
                    path: '/training/root-cause',
                    query: { id: detailData.workloadId || detailData.jobId },
                  })
                "
              >
                <WarningFilled />
              </el-icon>
            </el-tooltip>
          </div>
        </template>
      </div>

      <div class="w-actions">
        <slot name="extra-actions"></slot>

        <template v-if="!hideActions">
          <el-tooltip :content="editDisabled ? 'Not editable in current phase' : 'Edit'" placement="top">
            <el-button
              circle
              class="glass-btn glass-btn--edit"
              :disabled="editDisabled"
              @click="emit('edit')"
            >
              <el-icon><Edit /></el-icon>
            </el-button>
          </el-tooltip>

          <el-tooltip content="Clone" placement="top">
            <el-button circle class="glass-btn glass-btn--clone" @click="emit('clone')">
              <el-icon><CopyDocument /></el-icon>
            </el-button>
          </el-tooltip>

          <el-tooltip content="Delete" placement="top">
            <el-button circle class="glass-btn glass-btn--danger" @click="emit('delete')">
              <el-icon><Delete /></el-icon>
            </el-button>
          </el-tooltip>

          <el-tooltip
            v-if="showResumeButton"
            :content="detailData?.phase !== 'Stopped' ? 'Already running' : 'Resume'"
            placement="top"
          >
            <el-button
              circle
              class="glass-btn glass-btn--success"
              :disabled="!['Stopped','Failed','Succeeded'].includes(detailData?.phase)"
              @click="emit('resume')"
            >
              <el-icon><VideoPlay /></el-icon>
            </el-button>
          </el-tooltip>

          <el-tooltip
            :content="detailData?.phase === 'Stopped' ? 'Already stopped' : 'Stop'"
            placement="top"
          >
            <el-button
              circle
              class="glass-btn glass-btn--warning"
              :disabled="detailData?.phase === 'Stopped'"
              @click="emit('stop')"
            >
              <el-icon><CloseBold /></el-icon>
            </el-button>
          </el-tooltip>
        </template>
      </div>
    </div>

    <div class="w-meta">
      <slot name="meta-info">
        <span class="item">
          <span class="label">ID</span>
          <code class="code">{{ detailData.workloadId || detailData.jobId }}</code>
          <el-icon
            class="copy"
            size="12"
            style="color: var(--safe-primary)"
            @click="copyText(detailData.workloadId || detailData.jobId || '')"
          >
            <CopyDocument />
          </el-icon>
        </span>
        <span class="sep">•</span>
        <span class="item">
          <span class="label">creationTime</span>{{ formatTimeStr(detailData.creationTime) }}
        </span>
        <span class="sep">•</span>
        <span class="item">
          <span class="label">endTime</span>{{ formatTimeStr(detailData.endTime) || '-' }}
        </span>
        <span class="sep">•</span>
        <span class="item"><span class="label">user</span>{{ detailData.userName || '-' }}</span>
        <span class="sep">•</span>
        <span class="item" v-if="detailData.description !== undefined">
          <span class="label">description</span>
          <el-tooltip
            :content="detailData.description || '-'"
            placement="top"
            :disabled="!detailData.description || detailData.description.length <= 50"
          >
            <span class="truncate max-w-[42ch] cursor-help">
              {{ detailData.description || '-' }}
            </span>
          </el-tooltip>
        </span>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  CopyDocument,
  InfoFilled,
  CloseBold,
  Delete,
  ArrowLeft,
  WarningFilled,
  VideoPlay,
  Edit,
} from '@element-plus/icons-vue'
import { WorkloadPhaseButtonType } from '@/services'
import { copyText, formatTimeStr } from '@/utils'
import { useDark } from '@vueuse/core'
import { useRouter } from 'vue-router'
import { computed } from 'vue'

const isDark = useDark()
const router = useRouter()

interface WorkloadDetailData {
  displayName?: string
  jobName?: string
  phase: string
  message?: string
  workloadId?: string
  jobId?: string
  creationTime?: string
  endTime?: string
  userName?: string
  description?: string
  queuePosition?: number | string
  groupVersionKind?: {
    kind?: string
  }
}

const props = withDefaults(
  defineProps<{
    detailData: WorkloadDetailData
    showBackButton?: boolean
    hideActions?: boolean
    fallbackPath?: string
    editDisabled?: boolean
  }>(),
  {
    showBackButton: true,
    hideActions: false,
    editDisabled: false,
  },
)

const emit = defineEmits<{
  (e: 'clone'): void
  (e: 'edit'): void
  (e: 'delete'): void
  (e: 'stop'): void
  (e: 'resume'): void
  (e: 'back'): void
}>()

// Determine whether to show Resume button (only Infer / CICD / Authoring support, synced with list page actions)
const showResumeButton = computed(() => {
  const kind = props.detailData?.groupVersionKind?.kind
  // Training (PyTorchJob), TorchFT, RayJob do not support Resume
  const noResumeKinds = ['PyTorchJob', 'TorchFT', 'RayJob']
  return !noResumeKinds.includes(kind ?? '')
})

const handleBack = () => {
  emit('back')

  // Check if browser history is available for going back
  // window.history.length > 1 indicates history is available
  if (window.history.length > 1 && document.referrer) {
    router.back()
  } else {
    // If page was accessed directly, navigate to fallback path
    // Prefer the provided fallbackPath, otherwise infer from current route
    const fallback = props.fallbackPath || inferFallbackPath()
    router.push(fallback)
  }
}

const navigateToPendingCause = () => {
  router.push({
    path: '/workload/pending-cause',
    query: { id: props.detailData.workloadId || props.detailData.jobId },
  })
}

// Infer list page path from current route
// e.g.: /training/detail -> /training
const inferFallbackPath = () => {
  const currentPath = router.currentRoute.value.path
  const segments = currentPath.split('/').filter(Boolean)

  if (segments.length >= 2 && segments[segments.length - 1] === 'detail') {
    // Remove 'detail' segment, go back to parent path
    return '/' + segments.slice(0, -1).join('/')
  }

  // Default back to root path
  return '/'
}
</script>

<style scoped>
.pulse-scale {
  animation: pulse-scale 1.4s ease-in-out infinite;
}
@keyframes pulse-scale {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.2);
  }
}

.glass-btn {
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
  background: var(--button-bg-color);
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: var(--el-text-color-primary);
  transition:
    transform 0.2s ease,
    border-color 0.2s ease;
}

.glass-btn:hover {
  transform: scale(1.05);
  border-color: rgba(255, 255, 255, 0.35);
}

.glass-btn--export {
  color: var(--el-color-primary);
}

.glass-btn--edit {
  color: var(--el-color-primary);
}
.glass-btn--clone {
  color: var(--el-color-success);
}
.glass-btn--success {
  color: var(--el-color-primary);
}
.glass-btn--danger {
  color: var(--el-color-danger);
}
.glass-btn--warning {
  color: var(--el-color-warning);
}
.glass-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  transform: none;
}

/* Root Cause Analysis Icon */
.root-cause-icon {
  cursor: pointer;
  color: var(--el-color-danger);
  transition: all 0.2s ease;
}

.root-cause-icon:hover {
  color: var(--el-color-warning);
  transform: scale(1.2);
}

/* Pending Cause Analysis Icon */
.pending-cause-icon {
  cursor: pointer;
  color: var(--el-color-warning);
  transition: all 0.2s ease;
}

.pending-cause-icon:hover {
  color: var(--el-color-primary);
  transform: scale(1.2);
}
</style>
