<template>
  <div class="tracelens-analysis-page" v-loading="loading">
    <!-- Page Header -->
    <div class="page-header">
      <div class="header-left">
        <el-button :icon="ArrowLeft" @click="handleBack" link>
          Back to Workload
        </el-button>
        <h2 class="page-title">TraceLens Performance Analysis</h2>
      </div>
      <div class="header-right">
        <el-space>
          <el-button @click="handleRefresh" :icon="Refresh">Refresh</el-button>
          <el-button @click="handleOpenInNewTab" :icon="TopRight">Open in New Tab</el-button>
        </el-space>
      </div>
    </div>

    <!-- Session Info Bar -->
    <el-card class="session-info-card" v-if="sessionData">
      <div class="session-info">
        <!-- Workload Info -->
        <div class="info-section">
          <div class="info-item">
            <label>Workload:</label>
            <span>{{ workloadName }}</span>
          </div>
          <div class="info-item">
            <label>Cluster:</label>
            <span>{{ selectedCluster }}</span>
          </div>
          <div class="info-item">
            <label>File:</label>
            <el-tooltip :content="fileName" placement="top">
              <span class="file-name">{{ fileName }}</span>
            </el-tooltip>
          </div>
        </div>

        <!-- Session Status -->
        <div class="status-section">
          <div class="status-item">
            <label>Status:</label>
            <el-tag :type="getStatusType(sessionData.status)">
              <span class="status-icon">{{ getStatusIcon(sessionData.status) }}</span>
              {{ getStatusLabel(sessionData.status) }}
            </el-tag>
          </div>
          <div class="status-item">
            <label>Created:</label>
            <el-tooltip :content="formatFullTime(sessionData.createdAt)">
              <span>{{ formatRelativeTime(sessionData.createdAt) }}</span>
            </el-tooltip>
          </div>
          <div class="status-item" v-if="remainingTime">
            <label>Expires:</label>
            <el-tag 
              :type="remainingTime.isExpiring ? 'warning' : 'info'"
              effect="plain"
            >
              {{ remainingTime.text }}
            </el-tag>
          </div>
          <div class="status-item">
            <label>Resources:</label>
            <span>{{ getResourceLabel(sessionData.resourceProfile) }}</span>
          </div>
        </div>

        <!-- Actions -->
        <div class="action-section">
          <el-space>
            <el-dropdown 
              @command="handleExtend" 
              v-if="sessionData.status === 'ready' && !remainingTime?.expired"
            >
              <el-button type="primary">
                Extend Time <el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item :command="30">Extend 30 minutes</el-dropdown-item>
                  <el-dropdown-item :command="60">Extend 1 hour</el-dropdown-item>
                  <el-dropdown-item :command="120">Extend 2 hours</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button 
              type="danger" 
              plain
              @click="handleDelete"
              :loading="deleting"
            >
              Delete Session
            </el-button>
          </el-space>
        </div>
      </div>
    </el-card>

    <!-- Expired Alert -->
    <el-alert
      v-if="remainingTime?.expired"
      title="Session Expired"
      type="error"
      description="This analysis session has expired. Please create a new session."
      show-icon
      :closable="false"
      class="expired-alert"
    >
      <template #default>
        <div class="alert-content">
          <p>This analysis session has expired. You can create a new session or go back.</p>
          <el-space>
            <el-button type="primary" size="small" @click="handleRecreate" :loading="loading">
              Re-analyze
            </el-button>
            <el-button size="small" @click="handleBack">
              Back
            </el-button>
          </el-space>
        </div>
      </template>
    </el-alert>

    <!-- TraceLens UI iframe -->
    <div class="tracelens-container" v-if="sessionData?.status === 'ready' && !remainingTime?.expired">
      <iframe 
        ref="traceLensIframe"
        :src="iframeSrc"
        frameborder="0"
        width="100%"
        height="100%"
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-modals allow-downloads allow-presentation"
        @load="handleIframeLoad"
        @error="handleIframeError"
      ></iframe>
    </div>

    <!-- Failed or Deleted State -->
    <el-card v-else-if="sessionData && (sessionData.status === 'failed' || sessionData.status === 'deleted')" class="status-card">
      <el-result
        :icon="sessionData.status === 'failed' ? 'error' : 'warning'"
        :title="`Session ${sessionData.status === 'failed' ? 'Failed' : 'Deleted'}`"
        :sub-title="sessionData.statusMessage || (sessionData.status === 'failed' ? 'The analysis session failed to start.' : 'The analysis session has been deleted.')"
      >
        <template #extra>
          <el-space>
            <el-button type="primary" @click="handleRecreate" :loading="loading">
              Re-analyze
            </el-button>
            <el-button @click="handleBack">
              Back
            </el-button>
          </el-space>
        </template>
      </el-result>
    </el-card>
    
    <!-- Loading State for other non-ready sessions -->
    <el-card v-else-if="sessionData && sessionData.status !== 'ready' && !remainingTime?.expired" class="status-card loading-status-card">
      <div class="tracelens-loading-content">
        <!-- Loading Animation -->
        <div class="loading-animation">
          <el-icon class="loading-spinner" :size="64">
            <Loading />
          </el-icon>
        </div>

        <!-- Loading Title -->
        <h2 class="loading-title">TraceLens Analysis Loading</h2>

        <!-- Loading Message -->
        <p class="loading-message">{{ sessionData.statusMessage || getStatusLabel(sessionData.status) }}</p>
        <p class="loading-estimate" v-if="pollingAttempts > 0">
          Estimated time: {{ Math.max(5, 30 - pollingAttempts * 2) }} seconds
        </p>

        <!-- Status Progress -->
        <div class="status-progress">
          <div class="status-step" 
               v-for="step in statusSteps" 
               :key="step.status"
               :class="getStepClass(step.status)"
          >
            <div class="step-icon">
              <el-icon v-if="getStepState(step.status) === 'completed'">
                <CircleCheck />
              </el-icon>
              <el-icon v-else-if="getStepState(step.status) === 'active'">
                <Loading />
              </el-icon>
              <el-icon v-else>
                <span class="empty-circle"></span>
              </el-icon>
            </div>
            <div class="step-label">{{ step.label }}</div>
          </div>
        </div>

        <!-- Session Info -->
        <div class="session-loading-info">
          <div class="info-item" v-if="fileName">
            <label>File:</label>
            <span class="file-name">{{ fileName }}</span>
          </div>
          <div class="info-item">
            <label>Cluster:</label>
            <span>{{ selectedCluster }}</span>
          </div>
          <div class="info-item" v-if="sessionData?.resourceProfile">
            <label>Resource:</label>
            <span>{{ getResourceProfileLabel(sessionData.resourceProfile) }}</span>
          </div>
          <div class="info-item" v-if="sessionData?.sessionId">
            <label>Session:</label>
            <span class="session-id">{{ sessionData.sessionId }}</span>
          </div>
        </div>

        <!-- Actions -->
        <div class="loading-actions">
          <el-button type="primary" @click="loadSession">Refresh Status</el-button>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  ArrowLeft, 
  Refresh, 
  TopRight, 
  ArrowDown,
  Loading,
  CircleCheck
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import { getSession, createSession, extendSession, deleteSession, getUIUrl, calculateRemainingTime, formatRelativeTime, getResourceProfiles, DEFAULT_RESOURCE_PROFILES, SESSION_STATUS } from '@/services/tracelens'
import type { ResourceProfile } from '@/services/tracelens'
import { preAuthCheck } from '@/services/tracelens/auth'
import { useClusterSync } from '@/composables/useClusterSync'

// Initialize dayjs plugins
dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

// Router & Route
const route = useRoute()
const router = useRouter()

// Get cluster from global state
const { selectedCluster } = useClusterSync()

// Route params
const workloadUid = computed(() => route.params.workloadUid as string)
const sessionId = computed(() => route.params.sessionId as string)

// Data
const loading = ref(false)
const deleting = ref(false)
const sessionData = ref<any>(null)
const workloadName = ref('')
const fileName = ref('')
const iframeLoaded = ref(false)
const remainingTime = ref<any>(null)
const pollingAttempts = ref(0)
const resourceProfiles = ref<ResourceProfile[]>(DEFAULT_RESOURCE_PROFILES)

// Status steps configuration
const statusSteps = [
  { status: 'pending', label: 'Scheduling' },
  { status: 'creating', label: 'Creating' },
  { status: 'initializing', label: 'Initializing' },
  { status: 'ready', label: 'Ready' }
]

// Computed iframe source
const iframeSrc = computed(() => {
  if (!sessionData.value || sessionData.value.status !== 'ready') {
    return ''
  }
  return getUIUrl(sessionId.value, selectedCluster.value)
})

// Load resource profiles from backend
const loadResourceProfiles = async () => {
  try {
    const profiles = await getResourceProfiles()
    resourceProfiles.value = profiles
  } catch (error) {
    console.warn('Failed to load resource profiles, using defaults:', error)
    resourceProfiles.value = DEFAULT_RESOURCE_PROFILES
  }
}

// Load session data
const loadSession = async () => {
  loading.value = true
  try {
    // Load resource profiles in parallel with session data
    const [session] = await Promise.all([
      getSession(sessionId.value, selectedCluster.value),
      loadResourceProfiles()
    ])
    sessionData.value = session
    
    // If session is ready, do auth pre-check
    if (session.status === 'ready') {
      await preAuthCheck(sessionId.value)
    }
    
    // Update remaining time
    if (session.expiresAt) {
      remainingTime.value = calculateRemainingTime(session.expiresAt)
    }
    
    // Extract workload name and file name from session data
    // These might need to be fetched from another API
    workloadName.value = session.workloadName || workloadUid.value
    fileName.value = session.fileName || `File #${session.profilerFileId}`
    
    // If session is not ready, start polling
    if (session.status !== 'ready' && session.status !== 'failed' && session.status !== 'expired') {
      startPolling()
    }
  } catch (error) {
    console.error('Failed to load session:', error)
    ElMessage.error('Failed to load session information')
  } finally {
    loading.value = false
  }
}

// Polling for session status
let pollInterval: any = null

const startPolling = () => {
  if (pollInterval) {
    clearInterval(pollInterval)
  }
  
  // Reset polling attempts
  pollingAttempts.value = 0
  
  pollInterval = setInterval(async () => {
    try {
      pollingAttempts.value++
      const session = await getSession(sessionId.value, selectedCluster.value)
      sessionData.value = session
      
      // Update remaining time
      if (session.expiresAt) {
        remainingTime.value = calculateRemainingTime(session.expiresAt)
      }
      
      // Stop polling if ready, failed, or expired
      if (session.status === 'ready' || session.status === 'failed' || session.status === 'expired') {
        clearInterval(pollInterval)
        
        if (session.status === 'ready') {
          ElMessage.success('Analysis environment is ready')
        }
      }
    } catch (error) {
      console.error('Polling error:', error)
    }
  }, 3000) // Poll every 3 seconds
}

// Update remaining time every minute
let timeUpdateInterval: any = null

const startTimeUpdate = () => {
  if (timeUpdateInterval) {
    clearInterval(timeUpdateInterval)
  }
  
  timeUpdateInterval = setInterval(() => {
    if (sessionData.value?.expiresAt) {
      remainingTime.value = calculateRemainingTime(sessionData.value.expiresAt)
      
      // Show warning if expiring soon
      if (remainingTime.value?.isExpiring && remainingTime.value.minutes === 5) {
        ElMessage.warning('Session will expire in 5 minutes, please extend time')
      }
    }
  }, 60000) // Update every minute
}

// Handle back to workload
const handleBack = () => {
  router.push({
    path: '/workload/detail',
    query: {
      kind: route.query.kind,
      name: route.query.name,
      cluster: selectedCluster.value
    }
  })
}

// Template ref for iframe
const traceLensIframe = ref<HTMLIFrameElement>()

// Handle refresh
const handleRefresh = () => {
  if (iframeLoaded.value && traceLensIframe.value) {
    traceLensIframe.value.contentWindow?.location.reload()
  }
  loadSession()
}

// Handle open in new tab
const handleOpenInNewTab = () => {
  window.open(iframeSrc.value, '_blank')
}

// Handle extend session
const handleExtend = async (minutes: number) => {
  try {
    const result = await extendSession(sessionId.value, selectedCluster.value, minutes)
    sessionData.value.expiresAt = result.expiresAt
    remainingTime.value = calculateRemainingTime(result.expiresAt)
    ElMessage.success(`Session extended by ${minutes} minutes`)
  } catch (error) {
    console.error('Failed to extend session:', error)
    ElMessage.error('Failed to extend session')
  }
}

// Handle delete session
const handleDelete = async () => {
  const confirmed = await ElMessageBox.confirm(
    'Are you sure you want to delete this analysis session? You will need to restart after deletion.',
    'Delete Session',
    {
      confirmButtonText: 'Confirm',
      cancelButtonText: 'Cancel',
      type: 'warning'
    }
  ).catch(() => false)
  
  if (confirmed) {
    deleting.value = true
    try {
      await deleteSession(sessionId.value, selectedCluster.value)
      ElMessage.success('Session deleted')
      handleBack()
    } catch (error) {
      console.error('Failed to delete session:', error)
      ElMessage.error('Failed to delete session')
    } finally {
      deleting.value = false
    }
  }
}

// Handle recreate session - create a new session with same parameters
const handleRecreate = async () => {
  if (!sessionData.value) {
    ElMessage.error('Session information not available')
    return
  }

  try {
    loading.value = true
    const newSession = await createSession(
      {
        workloadUid: sessionData.value.workloadUid,
        profilerFileId: sessionData.value.profilerFileId,
        resourceProfile: sessionData.value.resourceProfile || 'medium',
        ttlMinutes: sessionData.value.ttlMinutes || 60
      },
      selectedCluster.value
    )
    
    ElMessage.success('New session created, redirecting...')
    
    // Navigate to the new session
    router.replace({
      name: 'TraceLensAnalysis',
      params: {
        workloadUid: newSession.workloadUid,
        sessionId: newSession.sessionId
      },
      query: {
        kind: route.query.kind,
        name: route.query.name
      }
    })
    
    // Reload the session data
    await loadSession()
  } catch (error) {
    console.error('Failed to recreate session:', error)
    ElMessage.error('Failed to recreate session')
  } finally {
    loading.value = false
  }
}

// Handle iframe load
const handleIframeLoad = () => {
  iframeLoaded.value = true
}

// Handle iframe error
const handleIframeError = (event: Event) => {
  console.error('iframe load error:', event)
  ElMessage.error('TraceLens UI failed to load, please refresh the page')
}

// Format functions
// formatRelativeTime is imported from services/tracelens

const formatFullTime = (dateStr: string) => {
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm:ss')
}

const getStatusType = (status: string): 'success' | 'warning' | 'danger' | 'info' => {
  const config = SESSION_STATUS[status as keyof typeof SESSION_STATUS]
  if (!config) return 'info'
  
  const typeMap: Record<string, 'success' | 'warning' | 'danger' | 'info'> = {
    success: 'success',
    warning: 'warning',
    danger: 'danger',
    info: 'info'
  }
  
  return typeMap[config.color] || 'info'
}

const getStatusIcon = (status: string) => {
  return SESSION_STATUS[status as keyof typeof SESSION_STATUS]?.icon || '-'
}

const getStatusLabel = (status: string) => {
  return SESSION_STATUS[status as keyof typeof SESSION_STATUS]?.label || status
}

const getResourceLabel = (profile: string) => {
  const config = resourceProfiles.value.find(p => p.value === profile)
  return config?.label || profile
}

// Get resource profile label for loading display
const getResourceProfileLabel = (profile: string) => {
  const config = resourceProfiles.value.find(p => p.value === profile)
  return config?.label || profile
}

// Get step state (pending/active/completed) for loading animation
const getStepState = (status: string) => {
  const statusOrder = ['pending', 'creating', 'initializing', 'ready']
  const currentIndex = statusOrder.indexOf(sessionData.value?.status || 'pending')
  const stepIndex = statusOrder.indexOf(status)
  
  if (stepIndex < currentIndex) return 'completed'
  if (stepIndex === currentIndex) return 'active'
  return 'pending'
}

// Get step class for loading animation
const getStepClass = (status: string) => {
  const state = getStepState(status)
  return {
    'status-step': true,
    'is-completed': state === 'completed',
    'is-active': state === 'active',
    'is-pending': state === 'pending'
  }
}

// Lifecycle hooks
onMounted(() => {
  loadSession()
  startTimeUpdate()
})

onUnmounted(() => {
  if (pollInterval) {
    clearInterval(pollInterval)
  }
  if (timeUpdateInterval) {
    clearInterval(timeUpdateInterval)
  }
})
</script>

<style scoped lang="scss">
// Import shared loading styles
@import '@/styles/tracelens-loading.scss';

.tracelens-analysis-page {
  padding: 20px;
  height: 100vh;
  display: flex;
  flex-direction: column;
  
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
    
    .header-left {
      display: flex;
      align-items: center;
      gap: 16px;
      
      .page-title {
        margin: 0;
        font-size: 20px;
        font-weight: 500;
        color: #303133;
      }
    }
  }
  
  .session-info-card {
    margin-bottom: 20px;
    
    .session-info {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 24px;
      
      .info-section,
      .status-section {
        display: flex;
        flex-wrap: wrap;
        gap: 16px;
        
        .info-item,
        .status-item {
          display: flex;
          align-items: center;
          gap: 4px;
          
          label {
            color: #606266;
            font-size: 14px;
          }
          
          span {
            color: #303133;
            font-size: 14px;
          }
          
          .file-name {
            max-width: 200px;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
            font-family: 'Courier New', monospace;
            font-size: 13px;
          }
          
          .status-icon {
            margin-right: 4px;
          }
        }
      }
      
      .action-section {
        flex-shrink: 0;
      }
    }
  }
  
  .expired-alert {
    margin-bottom: 20px;
    
    .alert-content {
      p {
        margin: 0 0 12px 0;
      }
    }
  }
  
  .tracelens-container {
    flex: 1;
    background: #fff;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
    min-height: 600px;
    
    iframe {
      border: none;
      display: block;
    }
  }
  
  .status-card {
    min-height: 400px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
}

// Dark theme support
.dark {
  .tracelens-analysis-page {
    .page-header {
      .page-title {
        color: var(--el-text-color-primary);
      }
    }
    
    .session-info-card {
      .session-info {
        .info-item,
        .status-item {
          label {
            color: var(--el-text-color-regular);
          }
          
          span {
            color: var(--el-text-color-primary);
          }
        }
      }
    }
    
    .tracelens-container {
      background: var(--el-bg-color);
    }
  }
}

// Responsive design
@media (max-width: 1200px) {
  .tracelens-analysis-page {
    .session-info-card {
      .session-info {
        flex-direction: column;
        align-items: flex-start;
        
        .action-section {
          width: 100%;
          margin-top: 12px;
        }
      }
    }
  }
}

@media (max-width: 768px) {
  .tracelens-analysis-page {
    padding: 12px;
    
    .page-header {
      flex-direction: column;
      align-items: flex-start;
      gap: 12px;
      
      .header-right {
        width: 100%;
      }
    }
  }
}
</style>
