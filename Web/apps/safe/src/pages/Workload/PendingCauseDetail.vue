<template>
  <!-- Header -->
  <div class="flex items-center justify-between mb-4">
    <div class="flex items-center gap-3">
      <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
        Back
      </el-button>
      <el-text class="text-xl font-600" tag="b">Pending Cause Analysis</el-text>
      <el-tag
        v-if="analysisResult"
        :type="getStatusType(analysisResult.status)"
        :effect="isDark ? 'plain' : 'light'"
      >
        {{ analysisResult.status.toUpperCase() }}
      </el-tag>
    </div>
    <el-button
      v-if="analysisResult?.result?.report && !isLegacyFormat"
      type="primary"
      :icon="CopyDocument"
      @click="copyReport"
      class="btn-primary-plain"
    >
      Copy Report
    </el-button>
  </div>

  <!-- Analysis Overview Card -->
  <el-card class="mt-4 safe-card" shadow="never" v-loading="loading">
    <div class="flex items-center mb-4">
      <div class="w-1 h-4 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
      <span class="text-base font-medium">Pending Cause Analysis Overview</span>
    </div>

    <el-descriptions v-if="analysisResult" border :column="4" direction="vertical">
      <el-descriptions-item label="Workload Id" :span="2">
        {{ analysisResult.workload_id }}
      </el-descriptions-item>
      <el-descriptions-item label="Job ID" :span="2">
        <div class="flex items-center gap-2">
          <code class="font-mono text-sm">{{ analysisResult.job_id }}</code>
          <el-button size="small" text @click="copyText(analysisResult.job_id)" title="Copy">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </div>
      </el-descriptions-item>
      <el-descriptions-item label="Creation Time" v-if="analysisResult.created_at">
        {{ formatTimeStr(analysisResult.created_at) }}
      </el-descriptions-item>
      <el-descriptions-item label="Started At" v-if="analysisResult.started_at">
        {{ formatTimeStr(analysisResult.started_at) }}
      </el-descriptions-item>
      <el-descriptions-item label="Completed At" v-if="analysisResult.completed_at">
        {{ formatTimeStr(analysisResult.completed_at) }}
      </el-descriptions-item>

      <el-descriptions-item label="Analysis Status">
        <el-tag
          :type="getStatusType(analysisResult.status)"
          :effect="isDark ? 'plain' : 'light'"
          size="large"
        >
          {{ analysisResult.status.toUpperCase() }}
        </el-tag>
      </el-descriptions-item>

      <el-descriptions-item label="Progress">
        <div class="flex items-center gap-2">
          <el-progress
            :percentage="analysisResult.progress"
            :status="analysisResult.status === 'completed' ? 'success' : undefined"
            :stroke-width="8"
            style="width: 180px"
          />
          <el-icon
            v-if="isPolling"
            class="is-loading ml-2"
            color="var(--el-color-primary)"
            :size="20"
          >
            <Loading />
          </el-icon>
        </div>
      </el-descriptions-item>

      <el-descriptions-item label="Current Step" :span="2">
        <el-tag type="info" size="large">{{ analysisResult.current_step }}</el-tag>
      </el-descriptions-item>

      <el-descriptions-item
        label="Total Analysis Time"
        :span="2"
        v-if="analysisResult.result?.total_analysis_time"
      >
        <span class="font-semibold text-lg">
          {{ analysisResult.result.total_analysis_time.toFixed(2) }}
        </span>
        <span class="ml-1 text-sm">seconds</span>
      </el-descriptions-item>

      <el-descriptions-item
        label="Problem Description"
        :span="4"
        v-if="analysisResult.problem_description"
      >
        {{ analysisResult.problem_description }}
      </el-descriptions-item>
    </el-descriptions>

    <el-empty v-else description="No analysis data available" :image-size="120" class="py-12" />
  </el-card>

  <el-tabs v-model="activeTab" class="mt-4">
    <!-- Legacy Format: Simple Report Tab -->
    <el-tab-pane v-if="isLegacyFormat" label="Report" name="report">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="analysisResult?.result?.report">
          <div class="legacy-report-content">
            <el-alert
              :title="analysisResult.result.report"
              type="warning"
              :closable="false"
              show-icon
            />
          </div>
        </div>
        <el-empty
          v-else
          description="No report available"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Analysis Steps Tab -->
    <el-tab-pane v-if="!isLegacyFormat" label="Analysis Steps" name="steps">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="steps-container">
          <el-timeline v-if="analysisResult?.steps?.length">
            <el-timeline-item
              v-for="(step, index) in analysisResult.steps"
              :key="step.step_name"
              :type="getStepType(step.status)"
              :hollow="step.status === 'pending'"
              :timestamp="formatTimestamp(step)"
              placement="top"
            >
              <el-card class="step-card" shadow="hover">
                <div class="flex items-start justify-between gap-4">
                  <div class="flex-1">
                    <div class="flex items-center gap-3 mb-3">
                      <span class="text-lg font-semibold">
                        {{ index + 1 }}. {{ step.step_name }}
                      </span>
                      <el-tag
                        :type="getStepType(step.status)"
                        size="default"
                        :effect="isDark ? 'plain' : 'light'"
                      >
                        {{ step.status.toUpperCase() }}
                      </el-tag>
                    </div>

                    <el-descriptions :column="3" size="default" class="mt-2" border>
                      <el-descriptions-item label="Started At" v-if="step.started_at">
                        {{ formatTimeStr(step.started_at) }}
                      </el-descriptions-item>
                      <el-descriptions-item label="Completed At" v-if="step.completed_at">
                        {{ formatTimeStr(step.completed_at) }}
                      </el-descriptions-item>
                      <el-descriptions-item label="Duration" v-if="step.duration">
                        <span class="font-mono font-semibold">{{ step.duration.toFixed(2) }}s</span>
                      </el-descriptions-item>
                    </el-descriptions>
                  </div>

                  <div class="step-progress">
                    <el-progress
                      type="circle"
                      :percentage="step.progress"
                      :status="step.status === 'completed' ? 'success' : undefined"
                      :width="90"
                    />
                  </div>
                </div>
              </el-card>
            </el-timeline-item>
          </el-timeline>

          <el-empty v-else description="No steps data available" :image-size="120" class="py-12" />
        </div>
      </el-card>
    </el-tab-pane>

    <!-- Pending Cause Tab (New Format Only) -->
    <el-tab-pane v-if="!isLegacyFormat" label="Pending Cause" name="pendingcause">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="reportSections.pendingCause">
          <div class="markdown-body" v-html="reportSections.pendingCause"></div>
        </div>
        <el-empty
          v-else
          description="Pending cause analysis not available yet"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Key Evidence Tab (New Format Only) -->
    <el-tab-pane v-if="!isLegacyFormat" label="Key Evidence" name="keyevidence">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="reportSections.keyEvidence">
          <div class="markdown-body" v-html="reportSections.keyEvidence"></div>
        </div>
        <el-empty
          v-else
          description="Key evidence not available yet"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Recommended Action Tab (New Format Only) -->
    <el-tab-pane v-if="!isLegacyFormat" label="Recommended Action" name="recommendedaction">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="reportSections.recommendedAction">
          <div class="markdown-body" v-html="reportSections.recommendedAction"></div>
        </div>
        <el-empty
          v-else
          description="Recommended action not available yet"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Full Report Tab (New Format Only) -->
    <el-tab-pane v-if="!isLegacyFormat" label="Full Report" name="fullreport">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="analysisResult?.result?.report">
          <div class="markdown-body" v-html="renderedReport"></div>

          <!-- Event Collection Section (appended below full report) -->
          <div v-if="eventCollection.length > 0" class="mt-8">
            <div class="flex items-center mb-4">
              <div class="w-1 h-4 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
              <span class="text-base font-medium">Event Collection (Top 5)</span>
            </div>
            <el-timeline>
              <el-timeline-item
                v-for="(event, index) in eventCollection"
                :key="index"
                :timestamp="formatEventTime(event.lastTimestamp || event.firstTimestamp)"
                placement="top"
                :type="getEventType(event.type)"
              >
                <el-card class="event-card" shadow="hover">
                  <div class="flex items-start gap-3">
                    <el-tag :type="getEventType(event.type)" size="large">
                      {{ event.type }}
                    </el-tag>
                    <div class="flex-1">
                      <div class="font-semibold text-base mb-2">{{ event.reason }}</div>
                      <div class="text-sm text-gray-600 dark:text-gray-400">{{ event.message }}</div>
                      <div class="mt-2 text-xs text-gray-500">
                        <span v-if="event.count > 1" class="mr-3">Count: {{ event.count }}</span>
                        <span v-if="event.source?.component">Component: {{ event.source.component }}</span>
                      </div>
                    </div>
                  </div>
                </el-card>
              </el-timeline-item>
            </el-timeline>
          </div>
        </div>
        <el-empty
          v-else
          description="Analysis report not available yet"
          :image-size="150"
          class="py-16"
        >
          <template #description>
            <p class="text-base text-gray-500 mb-2">
              {{
                analysisResult?.status === 'running' || analysisResult?.status === 'pending'
                  ? 'Analysis is in progress. The report will be available once completed.'
                  : 'No analysis report available.'
              }}
            </p>
          </template>
        </el-empty>
      </el-card>
    </el-tab-pane>
  </el-tabs>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import { createPendingCauseJob, getPendingCauseJob } from '@/services'
import type { PendingCauseAnalysisResult } from '@/services/workload/type'
import { ElMessage } from 'element-plus'
import { marked } from 'marked'
import { useDark } from '@vueuse/core'
import { copyText, formatTimeStr } from '@/utils/index'
import { ArrowLeft, CopyDocument, Loading } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()
const isDark = useDark()

const workloadId = computed(() => route.query.id as string)
const loading = ref(false)
const analysisResult = ref<PendingCauseAnalysisResult | null>(null)
const pollingTimer = ref<number | null>(null)
const activeTab = ref('steps')
const isPolling = ref(false)
const jobId = ref<string>('')

// Storage key for persisting job ID
const STORAGE_KEY_PREFIX = 'pending-cause-job-'
const getStorageKey = (wlId?: string) => {
  const id = wlId || workloadId.value
  return `${STORAGE_KEY_PREFIX}${id}`
}

// Check if the report is legacy format (simple string) or new format (markdown with sections)
const isLegacyFormat = computed(() => {
  const report = analysisResult.value?.result?.report
  if (!report) return false
  // Legacy format is a simple string without markdown headers
  return !report.includes('#') && !report.includes('##')
})

// Parse event collection from task_outputs (only first 5 events)
const eventCollection = computed(() => {
  const taskOutputs = analysisResult.value?.result?.task_outputs

  if (!taskOutputs || !taskOutputs.event_collection) return []

  try {
    const events = JSON.parse(taskOutputs.event_collection)

    // Handle different response formats
    if (Array.isArray(events)) {
      // If it's directly an array
      return events.slice(0, 5)
    } else if (events && Array.isArray(events.events)) {
      // If it's an object with an events array
      return events.events.slice(0, 5)
    }
    return []
  } catch (error) {
    console.error('Failed to parse event collection:', error)
    return []
  }
})

const renderedReport = computed(() => {
  if (!analysisResult.value?.result?.report) return ''
  return marked.parse(analysisResult.value.result.report) as string
})

// Parse different sections of the report (only for new format)
const reportSections = computed(() => {
  const report = analysisResult.value?.result?.report
  if (!report || isLegacyFormat.value) {
    return {
      pendingCause: '',
      keyEvidence: '',
      recommendedAction: '',
    }
  }

  // Extract different sections
  const sections = {
    pendingCause: '',
    keyEvidence: '',
    recommendedAction: '',
  }

  // Extract Pending Cause section
  const pendingCauseMatch = report.match(/## Pending Cause\s*\n([\s\S]*?)(?=\n## |$)/i)
  if (pendingCauseMatch) {
    sections.pendingCause = marked.parse('## Pending Cause\n' + pendingCauseMatch[1]) as string
  }

  // Extract Key Evidence section
  const keyEvidenceMatch = report.match(/## Key Evidence\s*\n([\s\S]*?)(?=\n## |$)/i)
  if (keyEvidenceMatch) {
    sections.keyEvidence = marked.parse('## Key Evidence\n' + keyEvidenceMatch[1]) as string
  }

  // Extract Recommended Action section
  const recommendedActionMatch = report.match(/## Recommended Action\s*\n([\s\S]*?)(?=\n## |$)/i)
  if (recommendedActionMatch) {
    sections.recommendedAction = marked.parse('## Recommended Action\n' + recommendedActionMatch[1]) as string
  }

  return sections
})

const getStatusType = (status: string) => {
  const types: Record<string, 'success' | 'danger' | 'warning' | 'info'> = {
    completed: 'success',
    failed: 'danger',
    running: 'warning',
    pending: 'info',
  }
  return types[status] || 'info'
}

const getStepType = (status: string) => {
  const types: Record<string, 'success' | 'danger' | 'warning' | 'info' | 'primary'> = {
    completed: 'success',
    failed: 'danger',
    running: 'primary',
    pending: 'info',
  }
  return types[status] || 'info'
}

const getEventType = (type: string) => {
  const types: Record<string, 'success' | 'danger' | 'warning' | 'info'> = {
    Normal: 'success',
    Warning: 'warning',
    Error: 'danger',
  }
  return types[type] || 'info'
}

const formatEventTime = (timeStr: string) => {
  if (!timeStr) return '-'

  // Handle Go time format: "2026-02-10 00:23:57 +0000 UTC"
  // Check if it's the zero time
  if (timeStr.startsWith('0001-01-01')) return '-'

  try {
    // Parse the time string
    // Format: "YYYY-MM-DD HH:mm:ss +0000 UTC"
    const parts = timeStr.split(' ')
    if (parts.length >= 2) {
      const dateTime = `${parts[0]}T${parts[1]}Z`
      return formatTimeStr(dateTime)
    }
    return timeStr
  } catch (error) {
    console.error('Failed to format event time:', error)
    return timeStr
  }
}

const formatTimestamp = (step: { started_at?: string; completed_at?: string }) => {
  if (step.completed_at) return formatTimeStr(step.completed_at)
  if (step.started_at) return formatTimeStr(step.started_at)
  return ''
}

const copyReport = () => {
  if (!analysisResult.value?.result?.report) return
  copyText(analysisResult.value.result.report)
  ElMessage.success('Report copied to clipboard')
}

const stopPolling = () => {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
    isPolling.value = false
  }
}

const saveJobIdToStorage = (id: string, wlId?: string) => {
  try {
    const key = getStorageKey(wlId)
    sessionStorage.setItem(key, id)
  } catch (error) {
    console.error('Failed to save job ID to storage:', error)
  }
}

const getJobIdFromStorage = (wlId?: string): string | null => {
  try {
    const key = getStorageKey(wlId)
    const jobId = sessionStorage.getItem(key)
    return jobId
  } catch (error) {
    console.error('Failed to get job ID from storage:', error)
    return null
  }
}

const clearJobIdFromStorage = (wlId?: string) => {
  try {
    const key = getStorageKey(wlId)
    sessionStorage.removeItem(key)
  } catch (error) {
    console.error('Failed to clear job ID from storage:', error)
  }
}

const fetchAnalysis = async (showLoading = true) => {
  if (!jobId.value) {
    return
  }

  if (showLoading) {
    loading.value = true
  }

  try {
    const result = await getPendingCauseJob(jobId.value)
    analysisResult.value = result

    // Set default active tab based on whether result is available
    if (result.result && activeTab.value === 'steps') {
      // For legacy format, go to report tab; for new format, go to pendingcause tab
      const isLegacy = result.result.report && !result.result.report.includes('#') && !result.result.report.includes('##')
      activeTab.value = isLegacy ? 'report' : 'pendingcause'
    }

    // If status is pending or running, continue polling
    if (result.status === 'pending' || result.status === 'running') {
      if (!pollingTimer.value) {
        isPolling.value = true
        pollingTimer.value = window.setInterval(() => {
          fetchAnalysis(false)
        }, 3000) // Poll every 3 seconds
      }
    } else {
      stopPolling()
    }
  } catch (error) {
    console.error('Failed to fetch pending cause analysis:', error)
    if (showLoading) {
      ElMessage.error('Failed to fetch analysis result')
    }
    // Clear invalid job from storage
    clearJobIdFromStorage(workloadId.value)
    stopPolling()
  } finally {
    if (showLoading) {
      loading.value = false
    }
  }
}

const createJob = async () => {
  if (!workloadId.value) {
    ElMessage.error('Workload ID is required')
    return
  }

  loading.value = true
  try {
    const response = await createPendingCauseJob({ workload_id: workloadId.value })
    jobId.value = response.job_id
    // Save job ID immediately after creation
    saveJobIdToStorage(response.job_id, workloadId.value)
    ElMessage.success('Analysis job created successfully')
    // Start polling for the job result
    await fetchAnalysis()
  } catch (error) {
    console.error('Failed to create pending cause analysis job:', error)
    ElMessage.error('Failed to create analysis job')
    loading.value = false
  }
}

const initializeJob = async () => {
  if (!workloadId.value) {
    ElMessage.error('Workload ID is required')
    return
  }

  const existingJobId = getJobIdFromStorage(workloadId.value)

  if (existingJobId) {
    jobId.value = existingJobId
    ElMessage.info('Resuming existing analysis...')
    await fetchAnalysis()
  } else {
    await createJob()
  }
}

onMounted(() => {
  initializeJob()
})

onBeforeUnmount(() => {
  stopPolling()
})

// Clear storage when leaving the route (not on refresh)
onBeforeRouteLeave(() => {
  clearJobIdFromStorage(workloadId.value)
  return true
})

defineOptions({
  name: 'PendingCauseDetail',
})
</script>

<style scoped>
.step-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  backdrop-filter: blur(10px);
  transition: all 0.3s ease;
}

.step-card:hover {
  border-color: var(--el-border-color);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.dark .step-card {
  background: rgba(255, 255, 255, 0.03);
}

.step-progress {
  flex-shrink: 0;
}

.steps-container {
  padding: 20px 0;
  width: 85%;
}

.report-container {
  padding: 0 24px;
  min-height: 400px;
}

.markdown-body {
  line-height: 1.8;
  font-size: 15px;
  color: var(--el-text-color-primary);
}

.markdown-body :deep(h1) {
  font-size: 32px;
  font-weight: 700;
  margin-top: 32px;
  margin-bottom: 20px;
  border-bottom: 2px solid var(--el-border-color);
  padding-bottom: 12px;
  color: var(--el-text-color-primary);
}

.markdown-body :deep(h2) {
  font-size: 24px;
  font-weight: 600;
  margin-top: 28px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--el-border-color-light);
  padding-bottom: 8px;
  color: var(--el-text-color-primary);
}

.markdown-body :deep(h3) {
  font-size: 20px;
  font-weight: 600;
  margin-top: 20px;
  margin-bottom: 12px;
  color: var(--el-text-color-primary);
}

.markdown-body :deep(p) {
  margin-bottom: 16px;
  line-height: 1.8;
  font-size: 15px;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin-bottom: 16px;
  padding-left: 32px;
}

.markdown-body :deep(li) {
  margin-bottom: 8px;
  line-height: 1.7;
  font-size: 15px;
}

.markdown-body :deep(code) {
  background-color: var(--el-fill-color-light);
  padding: 3px 8px;
  border-radius: 4px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 14px;
  color: var(--el-color-danger);
}

.markdown-body :deep(pre) {
  background-color: var(--el-fill-color);
  padding: 16px;
  border-radius: 8px;
  overflow-x: auto;
  margin-bottom: 16px;
  border: 1px solid var(--el-border-color-light);
}

.markdown-body :deep(pre code) {
  background-color: transparent;
  padding: 0;
  color: var(--el-text-color-primary);
  font-size: 14px;
}

.markdown-body :deep(blockquote) {
  border-left: 4px solid var(--el-color-primary);
  padding-left: 20px;
  margin: 16px 0;
  color: var(--el-text-color-secondary);
  font-style: italic;
  background-color: var(--el-fill-color-lighter);
  padding: 16px 20px;
  border-radius: 6px;
}

.markdown-body :deep(hr) {
  border: none;
  border-top: 2px solid var(--el-border-color);
  margin: 32px 0;
}

.markdown-body :deep(strong) {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.markdown-body :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 16px;
}

.markdown-body :deep(table th),
.markdown-body :deep(table td) {
  border: 1px solid var(--el-border-color);
  padding: 10px 14px;
  font-size: 14px;
}

.markdown-body :deep(table th) {
  background-color: var(--el-fill-color);
  font-weight: 600;
}

.markdown-body :deep(a) {
  color: var(--el-color-primary);
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

.legacy-report-content {
  padding: 20px 0;
}

.legacy-report-content :deep(.el-alert__title) {
  font-size: 16px;
  line-height: 1.8;
  white-space: pre-wrap;
  word-break: break-word;
}

.event-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  transition: all 0.3s ease;
}

.event-card:hover {
  border-color: var(--el-border-color);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.dark .event-card {
  background: rgba(255, 255, 255, 0.02);
}
</style>
