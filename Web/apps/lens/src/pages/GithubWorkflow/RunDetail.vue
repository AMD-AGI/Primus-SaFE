<template>
  <div class="run-detail-page">
    <!-- Header -->
    <div class="page-header">
      <el-button @click="goBack" :icon="ArrowLeft">Back</el-button>
      <div class="header-content">
        <h2 class="page-title">
          <span class="workflow-name">{{ run?.workflowName || 'Workflow Run' }}</span>
          <el-tag v-if="run?.githubRunNumber" type="info" effect="plain" class="run-number">
            #{{ run.githubRunNumber }}
          </el-tag>
        </h2>
        <div class="header-status">
          <StatusBadge :status="liveState?.workflowStatus || run?.status" :conclusion="liveState?.workflowConclusion" size="large" />
          <span v-if="isRunning" class="live-indicator">
            <span class="pulse-dot"></span>
            Live
          </span>
        </div>
      </div>
      <div class="header-actions">
        <el-button v-if="hasGithubInfo" type="primary" link @click="openGithubRun">
          <el-icon><Link /></el-icon>
          View on GitHub
        </el-button>
      </div>
    </div>

    <!-- Progress Bar (when running) -->
    <div v-if="isRunning" class="progress-section">
      <el-progress 
        :percentage="liveState?.progressPercent || 0" 
        :stroke-width="8"
        :show-text="false"
        :status="progressStatus"
        class="main-progress"
      />
      <div class="progress-info">
        <span class="current-task">
          <template v-if="liveState?.currentJobName">
            {{ liveState.currentJobName }}
            <template v-if="liveState?.currentStepName">
              → {{ liveState.currentStepName }}
            </template>
          </template>
          <template v-else>Initializing...</template>
        </span>
        <span class="progress-percent">{{ liveState?.progressPercent || 0 }}%</span>
      </div>
    </div>

    <div class="content-grid" v-loading="loading">
      <!-- Left Column: Info Cards -->
      <div class="info-column">
        <!-- Basic Info Card -->
        <el-card class="info-card glass-card">
          <template #header>
            <div class="card-header">
              <el-icon><InfoFilled /></el-icon>
              <span>Basic Information</span>
            </div>
          </template>
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="Workload">
              <div class="workload-info">
                <span class="name">{{ run?.workloadName }}</span>
                <span class="namespace">{{ run?.workloadNamespace }}</span>
              </div>
            </el-descriptions-item>
            <el-descriptions-item label="Branch">
              <el-tag v-if="run?.headBranch" type="info" effect="plain" size="small">
                <el-icon><BranchIcon /></el-icon>
                {{ run.headBranch }}
              </el-tag>
              <span v-else class="text-muted">-</span>
            </el-descriptions-item>
            <el-descriptions-item label="Commit">
              <el-link v-if="run?.headSha && hasGithubInfo" :href="commitUrl" target="_blank" type="primary">
                {{ run.headSha.substring(0, 7) }}
              </el-link>
              <span v-else class="text-muted">-</span>
            </el-descriptions-item>
            <el-descriptions-item label="Trigger">
              <el-tag :type="triggerTagType" effect="plain" size="small">
                {{ run?.triggerSource || '-' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Started">
              {{ formatDate(run?.workloadStartedAt) || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Duration">
              <span v-if="duration" class="duration">{{ duration }}</span>
              <span v-else class="text-muted">-</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>

        <!-- Collection Status Card -->
        <el-card class="info-card glass-card">
          <template #header>
            <div class="card-header">
              <el-icon><DataAnalysis /></el-icon>
              <span>Collection Status</span>
            </div>
          </template>
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="Status">
              <StatusBadge :status="run?.status" size="small" />
            </el-descriptions-item>
            <el-descriptions-item label="Files Found">
              {{ run?.filesFound || 0 }}
            </el-descriptions-item>
            <el-descriptions-item label="Files Processed">
              {{ run?.filesProcessed || 0 }}
            </el-descriptions-item>
            <el-descriptions-item label="Metrics Collected">
              <el-tag type="success" effect="plain" size="small">
                {{ run?.metricsCount || 0 }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item v-if="run?.errorMessage" label="Error">
              <el-text type="danger" class="error-message">{{ run.errorMessage }}</el-text>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>

        <!-- AI Analysis Tasks Card -->
        <AnalysisTasksCard 
          v-if="run?.id" 
          :run-id="run.id" 
          class="info-card"
          @view-report="handleViewReport"
        />
      </div>

      <!-- Right Column: Workflow Topology -->
      <div class="topology-column">
        <el-card class="topology-card glass-card">
          <template #header>
            <div class="card-header">
              <el-icon><VideoPlay /></el-icon>
              <span>Workflow Jobs</span>
              <span class="job-count" v-if="jobs.length">{{ jobs.length }} jobs</span>
            </div>
          </template>
          
          <div v-if="jobs.length === 0 && !loading" class="no-jobs">
            <el-empty description="No job information available" :image-size="100" />
          </div>

          <div v-else class="jobs-list">
            <div 
              v-for="job in jobs" 
              :key="job.id || job.githubJobId" 
              class="job-item"
              :class="{ 'expanded': expandedJobs.has(job.githubJobId), 'running': job.status === 'in_progress' }"
            >
              <!-- Job Header -->
              <div class="job-header" @click="toggleJob(job.githubJobId)">
                <div class="job-status-icon">
                  <StatusIcon :status="job.status" :conclusion="job.conclusion" />
                </div>
                <div class="job-info">
                  <span class="job-name">{{ job.name }}</span>
                  <span class="job-meta">
                    <template v-if="job.runnerName">on {{ job.runnerName }}</template>
                    <template v-if="job.durationSeconds"> · {{ formatDuration(job.durationSeconds) }}</template>
                  </span>
                </div>
                <el-icon class="expand-icon" :class="{ 'is-expanded': expandedJobs.has(job.githubJobId) }">
                  <ArrowRight />
                </el-icon>
              </div>

              <!-- Job Steps (Expandable) -->
              <el-collapse-transition>
                <div v-show="expandedJobs.has(job.githubJobId)" class="job-steps">
                  <div 
                    v-for="step in job.steps || []" 
                    :key="step.number" 
                    class="step-item clickable"
                    :class="{ 'running': step.status === 'in_progress' }"
                    @click.stop="showStepLogs(job, step)"
                  >
                    <div class="step-status-icon">
                      <StatusIcon :status="step.status" :conclusion="step.conclusion" size="small" />
                    </div>
                    <span class="step-name">{{ step.name }}</span>
                    <span class="step-duration" v-if="step.durationSeconds">
                      {{ formatDuration(step.durationSeconds) }}
                    </span>
                    <el-icon class="logs-icon"><Document /></el-icon>
                  </div>
                  <div v-if="!job.steps?.length" class="no-steps">
                    <span class="text-muted">No step details available</span>
                  </div>
                </div>
              </el-collapse-transition>
            </div>
          </div>
        </el-card>
      </div>
    </div>

    <!-- Step Logs Drawer -->
    <el-drawer
      v-model="logsDrawerVisible"
      :title="logsDrawerTitle"
      direction="rtl"
      size="50%"
      :destroy-on-close="true"
    >
      <div class="logs-drawer-content">
        <div class="logs-header">
          <div class="logs-meta">
            <el-tag v-if="currentStep" :type="getStepTagType(currentStep)" effect="plain" size="small">
              {{ currentStep?.conclusion || currentStep?.status }}
            </el-tag>
            <el-tag v-if="logsStatus && logsStatus !== 'available'" :type="getLogsStatusTagType(logsStatus)" effect="plain" size="small">
              {{ getLogsStatusLabel(logsStatus) }}
            </el-tag>
          </div>
          <el-button-group v-if="stepLogs">
            <el-button size="small" @click="copyLogs" :icon="CopyDocument">Copy</el-button>
            <el-button size="small" @click="downloadLogs" :icon="Download">Download</el-button>
          </el-button-group>
          <el-button v-if="logsStatus === 'pending' || logsStatus === 'not_collected'" 
                     size="small" type="primary" @click="refreshLogs" :icon="Refresh">
            Refresh
          </el-button>
        </div>
        <div v-loading="logsLoading" class="logs-container">
          <!-- Show logs content -->
          <pre v-if="stepLogs" class="logs-content">{{ stepLogs }}</pre>
          
          <!-- Show status message when no logs -->
          <div v-else-if="!logsLoading && logsStatus" class="logs-status-message">
            <el-icon v-if="logsStatus === 'pending'" class="status-icon pending"><Loading /></el-icon>
            <el-icon v-else-if="logsStatus === 'not_collected'" class="status-icon info"><InfoFilled /></el-icon>
            <el-icon v-else-if="logsStatus === 'failed'" class="status-icon error"><CircleCloseFilled /></el-icon>
            <p class="status-text">{{ logsMessage || 'No logs available' }}</p>
            <p v-if="logsStatus === 'pending'" class="status-hint">Logs are being collected. Click Refresh to check again.</p>
            <p v-if="logsStatus === 'not_collected'" class="status-hint">Logs will be available once the job completes.</p>
          </div>
          
          <el-empty v-else-if="!logsLoading" description="No logs available" :image-size="80" />
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft, Link, InfoFilled, DataAnalysis, VideoPlay, ArrowRight,
  Document, CopyDocument, Download, Refresh, Loading, CircleCloseFilled
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  getRun,
  getRunLiveState,
  getRunJobs,
  getStepLogs as fetchStepLogs,
  createRunLiveStream,
  type WorkflowRun,
  type WorkflowLiveState,
  type WorkflowJob,
  type WorkflowStep,
  getRunnerSetById
} from '@/services/workflow-metrics'
import { useClusterSync } from '@/composables/useClusterSync'
import StatusBadge from './components/StatusBadge.vue'
import StatusIcon from './components/StatusIcon.vue'
import BranchIcon from './components/BranchIcon.vue'
import AnalysisTasksCard from '@/components/analysis-tasks/AnalysisTasksCard.vue'
import type { AnalysisTask } from '@/services/analysis-tasks'

const route = useRoute()
const router = useRouter()
const { selectedCluster, navigateWithCluster } = useClusterSync()

// State
const loading = ref(true)
const run = ref<WorkflowRun | null>(null)
const liveState = ref<WorkflowLiveState | null>(null)
const jobs = ref<WorkflowJob[]>([])
const expandedJobs = ref<Set<number>>(new Set())
const githubInfo = ref<{ owner: string; repo: string } | null>(null)

// SSE connection
let eventSource: EventSource | null = null

// Logs drawer state
const logsDrawerVisible = ref(false)
const logsLoading = ref(false)
const stepLogs = ref<string>('')
const logsSource = ref<'cache' | 'github' | null>(null)
const currentJob = ref<WorkflowJob | null>(null)
const currentStep = ref<WorkflowStep | null>(null)

// Computed
const runId = computed(() => Number(route.params.runId))

const isRunning = computed(() => {
  const status = liveState.value?.workflowStatus || run.value?.status
  return status === 'in_progress' || status === 'queued' || status === 'collecting' || status === 'pending'
})

const hasGithubInfo = computed(() => {
  return !!(githubInfo.value?.owner && githubInfo.value?.repo && run.value?.githubRunId)
})

const githubRunUrl = computed(() => {
  if (!hasGithubInfo.value) return '#'
  return `https://github.com/${githubInfo.value!.owner}/${githubInfo.value!.repo}/actions/runs/${run.value!.githubRunId}`
})

const commitUrl = computed(() => {
  if (!githubInfo.value?.owner || !githubInfo.value?.repo || !run.value?.headSha) return '#'
  return `https://github.com/${githubInfo.value.owner}/${githubInfo.value.repo}/commit/${run.value.headSha}`
})

const triggerTagType = computed(() => {
  switch (run.value?.triggerSource) {
    case 'realtime': return 'success'
    case 'backfill': return 'warning'
    case 'manual': return 'info'
    default: return 'info'
  }
})

const progressStatus = computed(() => {
  const conclusion = liveState.value?.workflowConclusion
  if (conclusion === 'failure') return 'exception'
  if (conclusion === 'success') return 'success'
  return undefined
})

const duration = computed(() => {
  if (!run.value?.workloadStartedAt) return null
  const start = dayjs(run.value.workloadStartedAt)
  const end = run.value.workloadCompletedAt ? dayjs(run.value.workloadCompletedAt) : dayjs()
  const seconds = end.diff(start, 'second')
  return formatDuration(seconds)
})

const logsDrawerTitle = computed(() => {
  if (currentJob.value && currentStep.value) {
    return `${currentJob.value.name} / Step ${currentStep.value.number}: ${currentStep.value.name}`
  }
  return 'Step Logs'
})

// Methods
const goBack = () => {
  router.back()
}

const openGithubRun = () => {
  window.open(githubRunUrl.value, '_blank')
}

const toggleJob = (jobId: number) => {
  if (expandedJobs.value.has(jobId)) {
    expandedJobs.value.delete(jobId)
  } else {
    expandedJobs.value.add(jobId)
  }
}

const formatDate = (date?: string) => {
  if (!date) return null
  return dayjs(date).format('YYYY-MM-DD HH:mm:ss')
}

const formatDuration = (seconds: number) => {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  const hours = Math.floor(seconds / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  return `${hours}h ${mins}m`
}

const getStepTagType = (step: WorkflowStep) => {
  if (step.conclusion === 'success') return 'success'
  if (step.conclusion === 'failure') return 'danger'
  if (step.conclusion === 'skipped') return 'info'
  if (step.status === 'in_progress') return 'warning'
  return 'info'
}

// Logs status state
const logsStatus = ref<string>('')
const logsMessage = ref<string>('')

const showStepLogs = async (job: WorkflowJob, step: WorkflowStep) => {
  currentJob.value = job
  currentStep.value = step
  stepLogs.value = ''
  logsSource.value = null
  logsStatus.value = ''
  logsMessage.value = ''
  logsDrawerVisible.value = true
  logsLoading.value = true

  try {
    const response = await fetchStepLogs(runId.value, job.githubJobId, step.number) as any
    stepLogs.value = response.logs || ''
    logsStatus.value = response.status || 'available'
    logsMessage.value = response.message || ''
    logsSource.value = response.source
  } catch (error) {
    console.error('Failed to fetch step logs:', error)
    ElMessage.error('Failed to load step logs')
    stepLogs.value = ''
    logsStatus.value = 'error'
    logsMessage.value = 'Failed to load logs'
  } finally {
    logsLoading.value = false
  }
}

const copyLogs = async () => {
  if (!stepLogs.value) return
  try {
    await navigator.clipboard.writeText(stepLogs.value)
    ElMessage.success('Logs copied to clipboard')
  } catch (error) {
    ElMessage.error('Failed to copy logs')
  }
}

const downloadLogs = () => {
  if (!stepLogs.value || !currentStep.value) return
  const blob = new Blob([stepLogs.value], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `step-${currentStep.value.number}-${currentStep.value.name.replace(/[^a-zA-Z0-9]/g, '_')}.log`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const refreshLogs = () => {
  if (currentJob.value && currentStep.value) {
    showStepLogs(currentJob.value, currentStep.value)
  }
}

const getLogsStatusTagType = (status: string) => {
  switch (status) {
    case 'pending': return 'warning'
    case 'not_collected': return 'info'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

const getLogsStatusLabel = (status: string) => {
  switch (status) {
    case 'pending': return 'Collecting...'
    case 'not_collected': return 'Not Collected'
    case 'failed': return 'Collection Failed'
    default: return status
  }
}

const handleViewReport = (task: AnalysisTask) => {
  if (task.result?.reportUrl) {
    window.open(task.result.reportUrl, '_blank')
  }
}

const fetchRunData = async () => {
  loading.value = true
  try {
    const [runData, liveData, jobsData] = await Promise.all([
      getRun(runId.value),
      getRunLiveState(runId.value).catch(() => null),
      getRunJobs(runId.value).catch(() => ({ jobs: [] }))
    ])

    run.value = runData
    liveState.value = liveData
    jobs.value = liveData?.jobs || jobsData.jobs || []

    // Auto-expand running jobs
    jobs.value.forEach(job => {
      if (job.status === 'in_progress') {
        expandedJobs.value.add(job.githubJobId)
      }
    })

    // Try to get GitHub info from runner set
    if (runData.runnerSetId) {
      try {
        const runnerSet = await getRunnerSetById(runData.runnerSetId as number)
        if (runnerSet.githubOwner && runnerSet.githubRepo) {
          githubInfo.value = { owner: runnerSet.githubOwner, repo: runnerSet.githubRepo }
        }
      } catch (e) {
        // Ignore
      }
    }
  } catch (error) {
    console.error('Failed to fetch run data:', error)
    ElMessage.error('Failed to load run details')
  } finally {
    loading.value = false
  }
}

const connectLiveStream = () => {
  if (eventSource) {
    eventSource.close()
  }

  try {
    eventSource = createRunLiveStream(runId.value, selectedCluster.value)

    eventSource.addEventListener('state', (event) => {
      try {
        const data = JSON.parse(event.data)
        liveState.value = data
        if (data.jobs) {
          jobs.value = data.jobs
          // Auto-expand newly running jobs
          data.jobs.forEach((job: WorkflowJob) => {
            if (job.status === 'in_progress') {
              expandedJobs.value.add(job.githubJobId)
            }
          })
        }
      } catch (e) {
        console.error('Failed to parse SSE state:', e)
      }
    })

    eventSource.addEventListener('complete', () => {
      eventSource?.close()
      eventSource = null
      // Refresh data one more time
      fetchRunData()
    })

    eventSource.onerror = (error) => {
      console.error('SSE error:', error)
      // Will auto-reconnect
    }
  } catch (e) {
    console.error('Failed to connect SSE:', e)
  }
}

// Lifecycle
onMounted(async () => {
  await fetchRunData()
  if (isRunning.value) {
    connectLiveStream()
  }
})

onBeforeUnmount(() => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
})

// Watch for running state changes
watch(isRunning, (running) => {
  if (running && !eventSource) {
    connectLiveStream()
  } else if (!running && eventSource) {
    eventSource.close()
    eventSource = null
  }
})
</script>

<style scoped lang="scss">
.run-detail-page {
  padding: 20px;
  min-height: 100vh;
  position: relative;
  
  // Decorative background
  &::before {
    content: '';
    position: absolute;
    top: -50px;
    right: 10%;
    width: 500px;
    height: 500px;
    background: radial-gradient(circle, rgba(64, 158, 255, 0.08) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }
}

.page-header {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-bottom: 20px;
  position: relative;
  z-index: 1;
  
  .header-content {
    flex: 1;
    
    .page-title {
      margin: 0 0 4px 0;
      font-size: 22px;
      font-weight: 600;
      display: flex;
      align-items: center;
      gap: 12px;
      
      .workflow-name {
        background: linear-gradient(135deg, var(--el-text-color-primary) 0%, var(--el-text-color-regular) 100%);
        -webkit-background-clip: text;
        background-clip: text;
      }
      
      .run-number {
        font-size: 14px;
        font-weight: 500;
      }
    }
    
    .header-status {
      display: flex;
      align-items: center;
      gap: 12px;
      
      .live-indicator {
        display: flex;
        align-items: center;
        gap: 6px;
        color: var(--el-color-success);
        font-size: 13px;
        font-weight: 500;
        
        .pulse-dot {
          width: 8px;
          height: 8px;
          background: var(--el-color-success);
          border-radius: 50%;
          animation: pulse 1.5s ease-in-out infinite;
        }
      }
    }
  }
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(1.2); }
}

.progress-section {
  margin-bottom: 24px;
  padding: 16px 20px;
  background: linear-gradient(135deg, rgba(64, 158, 255, 0.08) 0%, rgba(103, 194, 58, 0.08) 100%);
  border-radius: 12px;
  border: 1px solid rgba(64, 158, 255, 0.2);
  position: relative;
  z-index: 1;
  
  .main-progress {
    margin-bottom: 8px;
    
    :deep(.el-progress-bar__outer) {
      background: rgba(0, 0, 0, 0.06);
    }
    
    :deep(.el-progress-bar__inner) {
      background: linear-gradient(90deg, var(--el-color-primary), var(--el-color-success));
      transition: width 0.5s ease;
    }
  }
  
  .progress-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    
    .current-task {
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }
    
    .progress-percent {
      font-size: 14px;
      font-weight: 600;
      color: var(--el-color-primary);
    }
  }
}

.content-grid {
  display: grid;
  grid-template-columns: 380px 1fr;
  gap: 24px;
  position: relative;
  z-index: 1;
  
  @media (max-width: 1200px) {
    grid-template-columns: 1fr;
  }
}

.glass-card {
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0.05) 100%);
  backdrop-filter: blur(16px) saturate(180%);
  -webkit-backdrop-filter: blur(16px) saturate(180%);
  border-radius: 16px !important;
  border: 1px solid rgba(255, 255, 255, 0.18);
  box-shadow: 0 8px 32px rgba(17, 24, 39, 0.06);
  overflow: hidden;
  
  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: linear-gradient(90deg, transparent 0%, rgba(64, 158, 255, 0.5) 50%, transparent 100%);
    opacity: 0.6;
  }
  
  :deep(.el-card__header) {
    padding: 16px 20px;
    border-bottom: 1px solid var(--el-border-color-lighter);
    background: rgba(0, 0, 0, 0.02);
  }
  
  :deep(.el-card__body) {
    padding: 20px;
  }
}

.card-header {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  
  .el-icon {
    font-size: 18px;
    color: var(--el-color-primary);
  }
  
  .job-count, .coming-soon {
    margin-left: auto;
    font-weight: 400;
    font-size: 12px;
  }
}

.info-column {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.info-card {
  .workload-info {
    display: flex;
    flex-direction: column;
    
    .name {
      font-weight: 500;
      color: var(--el-text-color-primary);
    }
    
    .namespace {
      font-size: 12px;
      color: var(--el-text-color-secondary);
      font-family: monospace;
    }
  }
  
  .text-muted {
    color: var(--el-text-color-placeholder);
  }
  
  .duration {
    font-family: 'SF Mono', Monaco, monospace;
    font-weight: 500;
  }
  
  .error-message {
    font-size: 12px;
    word-break: break-all;
  }
}

.analysis-card {
  .analysis-placeholder {
    padding: 20px 0;
  }
}

.topology-column {
  min-width: 0;
}

.topology-card {
  height: fit-content;
  
  .no-jobs {
    padding: 40px 20px;
  }
}

.jobs-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.job-item {
  border-radius: 12px;
  border: 1px solid var(--el-border-color-lighter);
  overflow: hidden;
  transition: all 0.2s ease;
  
  &:hover {
    border-color: var(--el-color-primary-light-5);
  }
  
  &.running {
    border-color: var(--el-color-warning);
    background: rgba(230, 162, 60, 0.04);
  }
  
  &.expanded {
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.06);
  }
}

.job-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  cursor: pointer;
  transition: background 0.2s;
  
  &:hover {
    background: rgba(0, 0, 0, 0.02);
  }
  
  .job-status-icon {
    flex-shrink: 0;
  }
  
  .job-info {
    flex: 1;
    min-width: 0;
    
    .job-name {
      display: block;
      font-weight: 500;
      color: var(--el-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    
    .job-meta {
      display: block;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      margin-top: 2px;
    }
  }
  
  .expand-icon {
    color: var(--el-text-color-secondary);
    transition: transform 0.2s;
    
    &.is-expanded {
      transform: rotate(90deg);
    }
  }
}

.job-steps {
  padding: 0 16px 16px 52px;
  border-top: 1px dashed var(--el-border-color-lighter);
  margin-top: -1px;
  padding-top: 12px;
}

.step-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  margin: 4px 0;
  border-radius: 8px;
  transition: background 0.2s;
  
  &:hover {
    background: rgba(0, 0, 0, 0.02);
  }
  
  &.running {
    background: rgba(230, 162, 60, 0.08);
  }
  
  .step-status-icon {
    flex-shrink: 0;
  }
  
  .step-name {
    flex: 1;
    font-size: 13px;
    color: var(--el-text-color-regular);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .step-duration {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    font-family: 'SF Mono', Monaco, monospace;
  }
}

.no-steps {
  padding: 8px 12px;
  
  .text-muted {
    font-size: 13px;
    color: var(--el-text-color-placeholder);
  }
}

.step-item.clickable {
  cursor: pointer;
  position: relative;
  
  .logs-icon {
    opacity: 0;
    color: var(--el-text-color-secondary);
    font-size: 14px;
    transition: opacity 0.2s;
    margin-left: 8px;
  }
  
  &:hover {
    background: rgba(64, 158, 255, 0.08);
    
    .logs-icon {
      opacity: 1;
    }
  }
}

// Logs Drawer Styles
.logs-drawer-content {
  display: flex;
  flex-direction: column;
  height: 100%;
  
  .logs-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0 0 16px 0;
    border-bottom: 1px solid var(--el-border-color-lighter);
    margin-bottom: 16px;
    
    .logs-meta {
      display: flex;
      align-items: center;
      gap: 12px;
      
      .logs-source {
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }
  
  .logs-container {
    flex: 1;
    overflow: auto;
    background: var(--el-bg-color-page);
    border-radius: 8px;
    border: 1px solid var(--el-border-color-lighter);
  }
  
  .logs-content {
    margin: 0;
    padding: 16px;
    font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-all;
    color: var(--el-text-color-regular);
  }
  
  .logs-status-message {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 60px 20px;
    text-align: center;
    
    .status-icon {
      font-size: 48px;
      margin-bottom: 16px;
      
      &.pending {
        color: var(--el-color-warning);
        animation: spin 1.5s linear infinite;
      }
      
      &.info {
        color: var(--el-color-info);
      }
      
      &.error {
        color: var(--el-color-danger);
      }
    }
    
    .status-text {
      font-size: 14px;
      color: var(--el-text-color-primary);
      margin: 0 0 8px 0;
    }
    
    .status-hint {
      font-size: 12px;
      color: var(--el-text-color-secondary);
      margin: 0;
    }
  }
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

// Dark mode support
html.dark {
  .glass-card {
    background: linear-gradient(135deg, rgba(30, 30, 30, 0.6) 0%, rgba(20, 20, 20, 0.4) 100%);
    border-color: rgba(255, 255, 255, 0.1);
  }
  
  .progress-section {
    background: linear-gradient(135deg, rgba(64, 158, 255, 0.12) 0%, rgba(103, 194, 58, 0.12) 100%);
  }
  
  .job-item {
    background: rgba(255, 255, 255, 0.02);
    
    &.running {
      background: rgba(230, 162, 60, 0.08);
    }
  }
  
  .job-header:hover,
  .step-item:hover {
    background: rgba(255, 255, 255, 0.04);
  }
}
</style>
