<template>
  <div class="perfetto-viewer-page" v-loading="loading">
    <!-- Page Header -->
    <div class="page-header">
      <div class="header-left">
        <el-button :icon="ArrowLeft" @click="handleBack" link>
          Back to Workload
        </el-button>
        <h2 class="page-title">Perfetto Trace Viewer</h2>
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
                  <el-dropdown-item :command="15">Extend 15 minutes</el-dropdown-item>
                  <el-dropdown-item :command="30">Extend 30 minutes</el-dropdown-item>
                  <el-dropdown-item :command="60">Extend 1 hour</el-dropdown-item>
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
      description="This viewer session has expired. Please create a new session."
      show-icon
      :closable="false"
      class="expired-alert"
    >
      <template #default>
        <div class="alert-content">
          <p>This viewer session has expired. You can create a new session or go back.</p>
          <el-space>
            <el-button type="primary" size="small" @click="handleRecreate" :loading="loading">
              Re-open Viewer
            </el-button>
            <el-button size="small" @click="handleBack">
              Back
            </el-button>
          </el-space>
        </div>
      </template>
    </el-alert>

    <!-- Perfetto UI iframe -->
    <div class="perfetto-container" v-if="sessionData?.status === 'ready' && !remainingTime?.expired">
      <iframe 
        ref="perfettoIframe"
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
        :sub-title="sessionData.statusMessage || (sessionData.status === 'failed' ? 'The viewer session failed to start.' : 'The viewer session has been deleted.')"
      >
        <template #extra>
          <el-space>
            <el-button type="primary" @click="handleRecreate" :loading="loading">
              Re-open Viewer
            </el-button>
            <el-button @click="handleBack">
              Back
            </el-button>
          </el-space>
        </template>
      </el-result>
    </el-card>

    <!-- Loading State -->
    <el-card v-else-if="sessionData && ['pending', 'creating', 'initializing'].includes(sessionData.status)" class="status-card">
      <div class="loading-container">
        <el-progress 
          type="circle" 
          :percentage="loadingProgress" 
          :status="sessionData.status === 'failed' ? 'exception' : undefined"
        />
        <h3>{{ loadingTitle }}</h3>
        <p class="loading-subtitle">{{ sessionData.statusMessage || 'Preparing Perfetto viewer...' }}</p>
        <p class="loading-hint">Estimated time: ~15 seconds</p>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, ArrowDown, Refresh, TopRight } from '@element-plus/icons-vue'
import { 
  createSession, 
  getSession, 
  extendSession, 
  deleteSession, 
  getUIUrl, 
  calculateRemainingTime,
  SESSION_STATUS,
  type PerfettoSession,
  type SessionStatus
} from '@/services/perfetto'
import { useClusterSync } from '@/composables/useClusterSync'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(relativeTime)

const route = useRoute()
const router = useRouter()
const { selectedCluster: globalCluster } = useClusterSync()

// Route params
const workloadUid = computed(() => route.params.workloadUid as string)
const fileId = computed(() => parseInt(route.params.fileId as string))
const workloadName = computed(() => route.query.workloadName as string || workloadUid.value)
const fileName = computed(() => route.query.fileName as string || `File ${fileId.value}`)
const selectedCluster = computed(() => (route.query.cluster as string) || globalCluster.value || '')

// State
const loading = ref(true)
const deleting = ref(false)
const sessionData = ref<PerfettoSession | null>(null)
const perfettoIframe = ref<HTMLIFrameElement | null>(null)
const iframeSrc = ref('')

// Polling interval
let pollInterval: ReturnType<typeof setInterval> | null = null
let expiryCheckInterval: ReturnType<typeof setInterval> | null = null

// Computed
const remainingTime = computed(() => {
  if (!sessionData.value?.expiresAt) return null
  return calculateRemainingTime(sessionData.value.expiresAt)
})

const loadingProgress = computed(() => {
  if (!sessionData.value) return 0
  switch (sessionData.value.status) {
    case 'pending': return 15
    case 'creating': return 45
    case 'initializing': return 75
    case 'ready': return 100
    default: return 0
  }
})

const loadingTitle = computed(() => {
  if (!sessionData.value) return 'Loading...'
  switch (sessionData.value.status) {
    case 'pending': return 'Starting session...'
    case 'creating': return 'Creating viewer pod...'
    case 'initializing': return 'Loading trace file...'
    case 'ready': return 'Ready'
    default: return 'Loading...'
  }
})

// Methods
function getStatusType(status: SessionStatus): 'success' | 'warning' | 'danger' | 'info' {
  return SESSION_STATUS[status]?.color || 'info'
}

function getStatusIcon(status: SessionStatus): string {
  return SESSION_STATUS[status]?.icon || '?'
}

function getStatusLabel(status: SessionStatus): string {
  return SESSION_STATUS[status]?.label || status
}

function formatRelativeTime(time: string): string {
  return dayjs(time).fromNow()
}

function formatFullTime(time: string): string {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

async function loadOrCreateSession() {
  loading.value = true
  
  try {
    // Try to create a session (API will reuse if exists)
    const session = await createSession({
      workloadUid: workloadUid.value,
      profilerFileId: fileId.value,
      ttlMinutes: 30
    }, selectedCluster.value)
    
    sessionData.value = session
    
    // If ready, set iframe source
    if (session.status === 'ready') {
      setIframeSource()
      stopPolling()
    } else if (['pending', 'creating', 'initializing'].includes(session.status)) {
      startPolling()
    }
  } catch (error: any) {
    ElMessage.error(`Failed to create session: ${error.message}`)
    console.error('Failed to create session:', error)
  } finally {
    loading.value = false
  }
}

async function refreshSession() {
  if (!sessionData.value) return
  
  try {
    const session = await getSession(sessionData.value.sessionId, selectedCluster.value)
    sessionData.value = session
    
    // If ready, set iframe source and stop polling
    if (session.status === 'ready') {
      setIframeSource()
      stopPolling()
    } else if (session.status === 'failed') {
      stopPolling()
    }
  } catch (error: any) {
    console.error('Failed to refresh session:', error)
  }
}

function setIframeSource() {
  if (!sessionData.value) return
  
  // Use UI URL with trace file path
  const baseUrl = getUIUrl(sessionData.value.sessionId, selectedCluster.value)
  // Perfetto loads trace from /trace.json when opened
  iframeSrc.value = `${baseUrl}#!/viewer?url=/trace.json`
}

function startPolling() {
  stopPolling()
  pollInterval = setInterval(refreshSession, 2000)
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

function startExpiryCheck() {
  expiryCheckInterval = setInterval(() => {
    // Force reactivity update for remaining time
    if (sessionData.value) {
      sessionData.value = { ...sessionData.value }
    }
  }, 30000)
}

async function handleExtend(minutes: number) {
  if (!sessionData.value) return
  
  try {
    const session = await extendSession(sessionData.value.sessionId, selectedCluster.value, minutes)
    sessionData.value = session
    ElMessage.success(`Session extended by ${minutes} minutes`)
  } catch (error: any) {
    ElMessage.error(`Failed to extend session: ${error.message}`)
  }
}

async function handleDelete() {
  if (!sessionData.value) return
  
  try {
    await ElMessageBox.confirm(
      'Are you sure you want to delete this viewer session?',
      'Delete Session',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    
    deleting.value = true
    await deleteSession(sessionData.value.sessionId, selectedCluster.value)
    ElMessage.success('Session deleted')
    handleBack()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(`Failed to delete session: ${error.message}`)
    }
  } finally {
    deleting.value = false
  }
}

async function handleRecreate() {
  sessionData.value = null
  await loadOrCreateSession()
}

function handleRefresh() {
  refreshSession()
}

function handleBack() {
  router.push({
    name: 'Workload',
    params: { workloadUid: workloadUid.value },
    query: { cluster: selectedCluster.value }
  })
}

function handleOpenInNewTab() {
  if (!sessionData.value || sessionData.value.status !== 'ready') {
    ElMessage.warning('Session is not ready yet')
    return
  }
  window.open(iframeSrc.value, '_blank')
}

function handleIframeLoad() {
}

function handleIframeError() {
  ElMessage.error('Failed to load Perfetto UI')
}

onMounted(() => {
  loadOrCreateSession()
  startExpiryCheck()
})

onUnmounted(() => {
  stopPolling()
  if (expiryCheckInterval) {
    clearInterval(expiryCheckInterval)
  }
})
</script>

<style scoped lang="scss">
.perfetto-viewer-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 20px;
  box-sizing: border-box;
  background: var(--el-bg-color-page);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  
  .header-left {
    display: flex;
    align-items: center;
    gap: 16px;
    
    .page-title {
      margin: 0;
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }
  }
}

.session-info-card {
  margin-bottom: 16px;
  
  .session-info {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 24px;
    
    .info-section, .status-section {
      display: flex;
      flex-wrap: wrap;
      gap: 16px;
    }
    
    .info-item, .status-item {
      display: flex;
      align-items: center;
      gap: 8px;
      
      label {
        color: var(--el-text-color-secondary);
        font-size: 13px;
      }
      
      span, .el-tag {
        font-size: 13px;
      }
      
      .file-name {
        max-width: 200px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      
      .status-icon {
        margin-right: 4px;
      }
    }
    
    .action-section {
      margin-left: auto;
    }
  }
}

.expired-alert {
  margin-bottom: 16px;
  
  .alert-content {
    display: flex;
    flex-direction: column;
    gap: 12px;
    
    p {
      margin: 0;
    }
  }
}

.perfetto-container {
  flex: 1;
  min-height: 500px;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: var(--el-box-shadow-light);
  background: #fff;
  
  iframe {
    display: block;
    width: 100%;
    height: 100%;
    border: none;
  }
}

.status-card {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  
  .loading-container {
    text-align: center;
    padding: 40px;
    
    h3 {
      margin: 24px 0 8px 0;
      font-size: 18px;
      color: var(--el-text-color-primary);
    }
    
    .loading-subtitle {
      color: var(--el-text-color-secondary);
      margin: 0 0 8px 0;
    }
    
    .loading-hint {
      color: var(--el-text-color-placeholder);
      font-size: 12px;
      margin: 0;
    }
  }
}
</style>

