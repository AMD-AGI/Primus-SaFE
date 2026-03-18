<template>
  <div class="tracelens-loading-page">
    <el-card class="loading-card">
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
        <p class="loading-message">{{ statusMessage }}</p>
        <p class="loading-estimate" v-if="estimatedTime">
          Estimated time: {{ estimatedTime }} seconds
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
        <div class="session-loading-info" v-if="fileName || currentSession">
          <div class="info-item" v-if="fileName">
            <label>File:</label>
            <span class="file-name">{{ fileName }}</span>
          </div>
          <div class="info-item">
            <label>Cluster:</label>
            <span>{{ selectedCluster }}</span>
          </div>
          <div class="info-item" v-if="currentSession?.resourceProfile">
            <label>Resource:</label>
            <span>{{ getResourceProfileLabel(currentSession.resourceProfile) }}</span>
          </div>
          <div class="info-item" v-if="currentSession?.sessionId">
            <label>Session:</label>
            <span class="session-id">{{ currentSession.sessionId }}</span>
          </div>
        </div>

        <!-- Actions -->
        <div class="loading-actions">
          <el-button @click="handleCancel">Cancel</el-button>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Loading, CircleCheck } from '@element-plus/icons-vue'
import { getSession, deleteSession, getResourceProfiles, DEFAULT_RESOURCE_PROFILES } from '@/services/tracelens'
import type { ResourceProfile } from '@/services/tracelens'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// Get cluster from global state
const { selectedCluster } = useGlobalCluster()

// Resource profiles loaded from backend
const resourceProfiles = ref<ResourceProfile[]>(DEFAULT_RESOURCE_PROFILES)

// Props & Emits
const props = defineProps<{
  sessionId: string
  fileName?: string
}>()

const emit = defineEmits<{
  'ready': [session: any]
  'cancel': []
}>()

// Status steps configuration
const statusSteps = [
  { status: 'pending', label: 'Scheduling' },
  { status: 'creating', label: 'Creating' },
  { status: 'initializing', label: 'Initializing' },
  { status: 'ready', label: 'Ready' }
]

// Data
const currentStatus = ref('pending')
const statusMessage = ref('Starting analysis environment...')
const estimatedTime = ref(30)
const attempts = ref(0)
const maxAttempts = 40 // 2 minutes max (3 seconds * 40)
const currentSession = ref<any>(null)
let pollInterval: any = null

// Computed status message based on current status
const computedStatusMessage = computed(() => {
  switch (currentStatus.value) {
    case 'pending':
      return 'Waiting for scheduling...'
    case 'creating':
      return 'Creating analysis environment...'
    case 'initializing':
      return 'Initializing environment...'
    case 'ready':
      return 'Ready, redirecting...'
    case 'failed':
      return 'Failed to start'
    default:
      return 'Starting analysis environment...'
  }
})

// Get step state (pending/active/completed)
const getStepState = (status: string) => {
  const statusOrder = ['pending', 'creating', 'initializing', 'ready']
  const currentIndex = statusOrder.indexOf(currentStatus.value)
  const stepIndex = statusOrder.indexOf(status)
  
  if (stepIndex < currentIndex) return 'completed'
  if (stepIndex === currentIndex) return 'active'
  return 'pending'
}

// Get step class
const getStepClass = (status: string) => {
  const state = getStepState(status)
  return {
    'status-step': true,
    'is-completed': state === 'completed',
    'is-active': state === 'active',
    'is-pending': state === 'pending'
  }
}

// Get resource profile label
const getResourceProfileLabel = (profile: string) => {
  const config = resourceProfiles.value.find(p => p.value === profile)
  return config?.label || profile
}

// Poll session status
const pollSessionStatus = async () => {
  try {
    const session = await getSession(props.sessionId, selectedCluster.value)
    
    // Update status
    currentStatus.value = session.status
    
    // Use actual statusMessage from backend if available, otherwise use computed message
    if (session.statusMessage) {
      statusMessage.value = session.statusMessage
    } else {
      statusMessage.value = computedStatusMessage.value
    }
    
    // Store session for displaying additional info
    currentSession.value = session
    
    // Update estimated time based on attempts
    // Decrease estimated time as we progress
    estimatedTime.value = Math.max(5, 30 - attempts.value * 2)
    
    // Check status
    if (session.status === 'ready') {
      // Session is ready
      clearInterval(pollInterval)
      ElMessage.success('Analysis environment is ready')
      setTimeout(() => {
        emit('ready', session)
      }, 500)
      return
    }
    
    if (session.status === 'failed') {
      // Session failed
      clearInterval(pollInterval)
      ElMessage.error(session.statusMessage || 'Failed to create session')
      handleCancel()
      return
    }
    
    // Continue polling
    attempts.value++
    
    if (attempts.value >= maxAttempts) {
      // Timeout
      clearInterval(pollInterval)
      ElMessage.error('Startup timeout, please try again later')
      handleCancel()
    }
  } catch (error) {
    console.error('Failed to poll session status:', error)
    clearInterval(pollInterval)
    ElMessage.error('Failed to get session status')
    handleCancel()
  }
}

// Handle cancel
const handleCancel = async () => {
  const confirmed = await ElMessageBox.confirm(
    'Are you sure you want to cancel? Created resources will be released.',
    'Cancel Startup',
    {
      confirmButtonText: 'Confirm',
      cancelButtonText: 'Continue waiting',
      type: 'warning'
    }
  ).catch(() => false)
  
  if (confirmed) {
    clearInterval(pollInterval)
    
    // Try to delete the session
    try {
      await deleteSession(props.sessionId, selectedCluster.value)
      ElMessage.info('Startup cancelled')
    } catch (error) {
      console.error('Failed to delete session:', error)
    }
    
    emit('cancel')
  }
}

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

// Start polling when component is mounted
onMounted(() => {
  // Load resource profiles
  loadResourceProfiles()
  
  // Start polling immediately
  pollSessionStatus()
  
  // Set up interval for subsequent polls
  pollInterval = setInterval(() => {
    pollSessionStatus()
  }, 3000) // Poll every 3 seconds
})

// Clean up on unmount
onUnmounted(() => {
  if (pollInterval) {
    clearInterval(pollInterval)
  }
})
</script>

<style scoped lang="scss">
// Import shared loading styles
@import '@/styles/tracelens-loading.scss';

.tracelens-loading-page {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
  
  .loading-card {
    width: 600px;
    max-width: 90%;
  }
}
</style>
