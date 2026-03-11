<template>
  <div class="run-summary-detail">
    <!-- Header -->
    <div class="page-header">
      <el-button @click="goBack" :icon="ArrowLeft">Back</el-button>
      <div v-if="summary" class="header-content">
        <div class="run-title">
          <StatusIcon :status="summary.status" :conclusion="summary.conclusion" size="large" />
          <h2>{{ summary.workflowName || 'Workflow Run' }}</h2>
          <span class="run-number">#{{ summary.githubRunNumber }}</span>
          <StatusBadge :status="summary.status" :conclusion="summary.conclusion" />
        </div>
        <div class="run-meta">
          <span class="repo">
            <el-icon><Link /></el-icon>
            {{ summary.owner }}/{{ summary.repo }}
          </span>
          <span class="separator">·</span>
          <BranchIcon />
          <span class="branch">{{ summary.headBranch || '-' }}</span>
          <span class="separator">·</span>
          <span class="sha">{{ summary.headSha?.substring(0, 7) || '-' }}</span>
          <span class="separator">·</span>
          <span class="event">{{ summary.eventName || '-' }}</span>
          <span class="separator">·</span>
          <span class="actor">{{ summary.actor || summary.triggeringActor || '-' }}</span>
        </div>
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading-container">
      <el-skeleton :rows="10" animated />
    </div>

    <!-- Content -->
    <template v-else-if="summary">
      <!-- Summary Stats -->
      <div class="stats-row">
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-icon success">
              <el-icon><Check /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ summary.successfulJobs }}</div>
              <div class="stat-label">Successful</div>
            </div>
          </div>
        </el-card>
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-icon danger">
              <el-icon><Close /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ summary.failedJobs }}</div>
              <div class="stat-label">Failed</div>
            </div>
          </div>
        </el-card>
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-icon warning">
              <el-icon><VideoPlay /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ summary.inProgressJobs }}</div>
              <div class="stat-label">In Progress</div>
            </div>
          </div>
        </el-card>
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-icon info">
              <el-icon><Clock /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ summary.queuedJobs }}</div>
              <div class="stat-label">Queued</div>
            </div>
          </div>
        </el-card>
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-icon primary">
              <el-icon><DataLine /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ summary.totalJobs }}</div>
              <div class="stat-label">Total Jobs</div>
            </div>
          </div>
        </el-card>
      </div>

      <!-- Progress bar for in-progress runs -->
      <el-card v-if="summary.status === 'in_progress'" class="progress-card">
        <div class="progress-content">
          <div class="progress-label">
            <span>Progress</span>
            <span>{{ summary.progressPercent }}%</span>
          </div>
          <el-progress :percentage="summary.progressPercent" :stroke-width="12" />
          <div v-if="summary.currentJobName" class="current-job">
            Currently running: <strong>{{ summary.currentJobName }}</strong>
            <span v-if="summary.currentStepName"> - {{ summary.currentStepName }}</span>
          </div>
        </div>
      </el-card>

      <!-- Run Info -->
      <el-card class="info-card">
        <template #header>
          <span>Run Information</span>
        </template>
        <el-descriptions :column="3" border>
          <el-descriptions-item label="GitHub Run ID">
            <a :href="`https://github.com/${summary.owner}/${summary.repo}/actions/runs/${summary.githubRunId}`" target="_blank" class="github-link">
              {{ summary.githubRunId }}
              <el-icon><TopRight /></el-icon>
            </a>
          </el-descriptions-item>
          <el-descriptions-item label="Run Number">#{{ summary.githubRunNumber }}</el-descriptions-item>
          <el-descriptions-item label="Attempt">{{ summary.githubRunAttempt }}</el-descriptions-item>
          <el-descriptions-item label="Started">{{ summary.runStartedAt ? formatTime(summary.runStartedAt) : '-' }}</el-descriptions-item>
          <el-descriptions-item label="Completed">{{ summary.runCompletedAt ? formatTime(summary.runCompletedAt) : '-' }}</el-descriptions-item>
          <el-descriptions-item label="Duration">{{ formatDuration(summary.runStartedAt, summary.runCompletedAt) }}</el-descriptions-item>
          <el-descriptions-item label="Collection Status">
            <el-tag :type="getCollectionStatusType(summary.collectionStatus)" size="small">
              {{ summary.collectionStatus || 'N/A' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Metrics Collected">{{ summary.totalMetricsCount }}</el-descriptions-item>
          <el-descriptions-item label="Files Processed">{{ summary.totalFilesProcessed }}</el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- Workflow Graph -->
      <el-card class="graph-card">
        <template #header>
          <div class="card-header">
            <span>Workflow Graph</span>
            <el-button size="small" @click="fetchGraph" :loading="graphLoading">
              <el-icon><Refresh /></el-icon>
              Refresh
            </el-button>
          </div>
        </template>
        <WorkflowDAG 
          v-loading="graphLoading" 
          :jobs="graphJobs" 
          @job-click="handleGraphJobClick"
        />
      </el-card>

      <!-- Jobs List -->
      <el-card class="jobs-card">
        <template #header>
          <div class="card-header">
            <span>Jobs ({{ jobs.length }})</span>
            <el-button size="small" @click="fetchJobs" :loading="jobsLoading">
              <el-icon><Refresh /></el-icon>
              Refresh
            </el-button>
          </div>
        </template>
        <el-table v-loading="jobsLoading" :data="jobsWithGithubName" style="width: 100%" @row-click="goToJobDetail">
          <el-table-column label="Job" min-width="280">
            <template #default="{ row }">
              <div class="job-cell">
                <StatusIcon :status="row.workflowStatus" :conclusion="row.workflowConclusion" />
                <div class="job-names">
                  <span class="job-name">{{ row.githubJobName || row.workloadName }}</span>
                  <span v-if="row.githubJobName && row.githubJobName !== row.workloadName" class="workload-name">
                    {{ row.workloadName }}
                  </span>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="Status" width="140" align="center">
            <template #default="{ row }">
              <StatusBadge :status="row.workflowStatus" :conclusion="row.workflowConclusion" />
            </template>
          </el-table-column>
          <el-table-column label="Collection" width="140" align="center">
            <template #default="{ row }">
              <el-tag :type="getCollectionStatusType(row.collectionStatus)" size="small">
                {{ row.collectionStatus || 'N/A' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Metrics" width="100" align="center">
            <template #default="{ row }">
              {{ row.metricsCount || 0 }}
            </template>
          </el-table-column>
          <el-table-column label="Started" width="160">
            <template #default="{ row }">
              {{ row.workloadStartedAt ? formatRelativeTime(row.workloadStartedAt) : '-' }}
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <!-- Not Found -->
    <el-empty v-else description="Run summary not found" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft, Link, TopRight, Check, Close, VideoPlay, Clock, DataLine, Refresh
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import {
  getRunSummary,
  getRunSummaryJobs,
  getRunSummaryGraph,
  type WorkflowRunSummary,
  type WorkflowRun,
  type GithubJobNode
} from '@/services/workflow-metrics'
import { useClusterSync } from '@/composables/useClusterSync'
import StatusIcon from './components/StatusIcon.vue'
import StatusBadge from './components/StatusBadge.vue'
import BranchIcon from './components/BranchIcon.vue'
import WorkflowDAG from './components/WorkflowDAG.vue'

dayjs.extend(relativeTime)

const route = useRoute()
const router = useRouter()
const { selectedCluster } = useClusterSync()

// State
const loading = ref(true)
const summary = ref<WorkflowRunSummary | null>(null)
const jobs = ref<WorkflowRun[]>([])
const jobsLoading = ref(false)
const graphJobs = ref<GithubJobNode[]>([])
const graphLoading = ref(false)

// Computed
const summaryId = computed(() => Number(route.params.id))

// Build a map from github_job_id to github job name for enriching jobs table
const githubJobNameMap = computed(() => {
  const map = new Map<number, string>()
  graphJobs.value.forEach(job => {
    map.set(job.githubJobId, job.name)
  })
  return map
})

// Jobs with GitHub job name added
const jobsWithGithubName = computed(() => {
  return jobs.value.map(job => ({
    ...job,
    githubJobName: job.githubJobId ? githubJobNameMap.value.get(job.githubJobId) : undefined
  }))
})

// Methods
const goBack = () => {
  router.back()
}

const fetchSummary = async () => {
  loading.value = true
  try {
    summary.value = await getRunSummary(summaryId.value)
  } catch (error) {
    console.error('Failed to fetch run summary:', error)
    ElMessage.error('Failed to load run summary')
  } finally {
    loading.value = false
  }
}

const fetchJobs = async () => {
  jobsLoading.value = true
  try {
    const res = await getRunSummaryJobs(summaryId.value)
    jobs.value = res.jobs || []
  } catch (error) {
    console.error('Failed to fetch jobs:', error)
    ElMessage.error('Failed to load jobs')
  } finally {
    jobsLoading.value = false
  }
}

const fetchGraph = async () => {
  graphLoading.value = true
  try {
    const res = await getRunSummaryGraph(summaryId.value)
    graphJobs.value = res.jobs || []
  } catch (error) {
    console.error('Failed to fetch graph:', error)
    // Silent fail - graph is optional
  } finally {
    graphLoading.value = false
  }
}

const handleGraphJobClick = (job: GithubJobNode) => {
  if (job.htmlUrl) {
    window.open(job.htmlUrl, '_blank')
  }
}

const goToJobDetail = (row: WorkflowRun) => {
  router.push({
    path: `/github-workflow/runs/${row.id}`,
    query: selectedCluster.value ? { cluster: selectedCluster.value } : undefined
  })
}

const formatTime = (time: string) => {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

const formatRelativeTime = (time: string) => {
  return dayjs(time).fromNow()
}

const formatDuration = (start?: string, end?: string) => {
  if (!start) return '-'
  const startTime = dayjs(start)
  const endTime = end ? dayjs(end) : dayjs()
  const duration = endTime.diff(startTime, 'second')
  
  if (duration < 60) return `${duration}s`
  if (duration < 3600) return `${Math.floor(duration / 60)}m ${duration % 60}s`
  return `${Math.floor(duration / 3600)}h ${Math.floor((duration % 3600) / 60)}m`
}

const getCollectionStatusType = (status?: string) => {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'partial': return 'warning'
    case 'pending': return 'info'
    default: return 'info'
  }
}

// Lifecycle
onMounted(async () => {
  await fetchSummary()
  // Fetch graph and jobs in parallel
  await Promise.all([
    fetchGraph(),
    fetchJobs()
  ])
})
</script>

<style scoped lang="scss">
.run-summary-detail {
  padding: 20px;
  
  .page-header {
    display: flex;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 24px;
    
    .header-content {
      .run-title {
        display: flex;
        align-items: center;
        gap: 12px;
        margin-bottom: 8px;
        
        h2 {
          margin: 0;
          font-size: 20px;
          font-weight: 600;
        }
        
        .run-number {
          color: var(--el-text-color-secondary);
          font-size: 16px;
        }
      }
      
      .run-meta {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 14px;
        color: var(--el-text-color-secondary);
        
        .repo {
          display: flex;
          align-items: center;
          gap: 4px;
        }
        
        .separator {
          color: var(--el-text-color-placeholder);
        }
        
        .branch {
          color: var(--el-color-primary);
        }
        
        .sha {
          font-family: monospace;
        }
      }
    }
  }
  
  .loading-container {
    padding: 40px;
  }
  
  .stats-row {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 16px;
    margin-bottom: 20px;
    
    .stat-card {
      .stat-content {
        display: flex;
        align-items: center;
        gap: 16px;
        
        .stat-icon {
          width: 48px;
          height: 48px;
          border-radius: 12px;
          display: flex;
          align-items: center;
          justify-content: center;
          font-size: 24px;
          
          &.success {
            background: var(--el-color-success-light-9);
            color: var(--el-color-success);
          }
          
          &.danger {
            background: var(--el-color-danger-light-9);
            color: var(--el-color-danger);
          }
          
          &.warning {
            background: var(--el-color-warning-light-9);
            color: var(--el-color-warning);
          }
          
          &.info {
            background: var(--el-color-info-light-9);
            color: var(--el-color-info);
          }
          
          &.primary {
            background: var(--el-color-primary-light-9);
            color: var(--el-color-primary);
          }
        }
        
        .stat-info {
          .stat-value {
            font-size: 24px;
            font-weight: 600;
            color: var(--el-text-color-primary);
          }
          
          .stat-label {
            font-size: 13px;
            color: var(--el-text-color-secondary);
          }
        }
      }
    }
  }
  
  .progress-card {
    margin-bottom: 20px;
    
    .progress-content {
      .progress-label {
        display: flex;
        justify-content: space-between;
        margin-bottom: 8px;
        font-size: 14px;
        color: var(--el-text-color-secondary);
      }
      
      .current-job {
        margin-top: 12px;
        font-size: 13px;
        color: var(--el-text-color-secondary);
        
        strong {
          color: var(--el-text-color-primary);
        }
      }
    }
  }
  
  .info-card {
    margin-bottom: 20px;
    
    .github-link {
      color: var(--el-color-primary);
      text-decoration: none;
      display: inline-flex;
      align-items: center;
      gap: 4px;
      
      &:hover {
        text-decoration: underline;
      }
    }
  }
  
  .graph-card {
    margin-bottom: 20px;
    
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
  }
  
  .jobs-card {
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    
    .job-cell {
      display: flex;
      align-items: center;
      gap: 8px;
      
      .job-names {
        display: flex;
        flex-direction: column;
        gap: 2px;
        
        .job-name {
          font-weight: 500;
        }
        
        .workload-name {
          font-size: 12px;
          color: var(--el-text-color-secondary);
        }
      }
    }
  }
}
</style>
