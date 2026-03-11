<template>
  <!-- Header -->
  <div class="flex items-center justify-between mb-4">
    <div class="flex items-center gap-3">
      <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
        Back
      </el-button>
      <el-text class="text-xl font-600" tag="b">Root Cause Analysis</el-text>
      <el-tag
        v-if="analysisResult"
        :type="getStatusType(analysisResult.status)"
        :effect="isDark ? 'plain' : 'light'"
      >
        {{ analysisResult.status.toUpperCase() }}
      </el-tag>
    </div>
    <el-button
      v-if="analysisResult?.result?.report"
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
      <span class="text-base font-medium">Root Cause Analysis Overview</span>
    </div>

    <el-descriptions v-if="analysisResult" border :column="4" direction="vertical">
      <el-descriptions-item label="Workload Id" :span="2">
        {{ analysisResult.workload_name }}
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
    <!-- Analysis Steps Tab -->
    <el-tab-pane label="Analysis Steps" name="steps">
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

    <!-- Root Cause Tab -->
    <el-tab-pane label="Root Cause" name="rootcause">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="reportSections.rootCause">
          <div class="markdown-body" v-html="reportSections.rootCause"></div>
        </div>
        <el-empty
          v-else
          description="Root cause analysis not available yet"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Problem Description Tab -->
    <el-tab-pane label="Problem Description" name="problem">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="reportSections.problemDescription">
          <div class="markdown-body" v-html="reportSections.problemDescription"></div>
        </div>
        <el-empty
          v-else
          description="Problem description not available yet"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Analysis Findings Tab -->
    <el-tab-pane label="Analysis Findings" name="findings">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="reportSections.analysisFindings">
          <div class="markdown-body" v-html="reportSections.analysisFindings"></div>
        </div>
        <el-empty
          v-else
          description="Analysis findings not available yet"
          :image-size="150"
          class="py-16"
        />
      </el-card>
    </el-tab-pane>

    <!-- Full Report Tab -->
    <el-tab-pane label="Full Report" name="fullreport">
      <el-card class="mt-2 safe-card" shadow="never" v-loading="loading">
        <div class="report-container" v-if="analysisResult?.result?.report">
          <div class="markdown-body" v-html="renderedReport"></div>
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
import { useRoute, useRouter } from 'vue-router'
import { getRootCauseAnalysis } from '@/services'
import type { RootCauseAnalysisResult } from '@/services/workload/type'
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
const analysisResult = ref<RootCauseAnalysisResult | null>(null)
const pollingTimer = ref<number | null>(null)
const activeTab = ref('rootcause')
const isPolling = ref(false)

const renderedReport = computed(() => {
  if (!analysisResult.value?.result?.report) return ''
  return marked.parse(analysisResult.value.result.report) as string
})

// Parse different sections of the report
const reportSections = computed(() => {
  const report = analysisResult.value?.result?.report
  if (!report) {
    return {
      rootCause: '',
      problemDescription: '',
      analysisFindings: '',
    }
  }

  // Extract different sections
  const sections = {
    rootCause: '',
    problemDescription: '',
    analysisFindings: '',
  }

  // Extract ROOT CAUSE IDENTIFICATION section
  const rootCauseMatch = report.match(/# ROOT CAUSE IDENTIFICATION([\s\S]*?)(?=\n# |$)/i)
  if (rootCauseMatch) {
    sections.rootCause = marked.parse('# ROOT CAUSE IDENTIFICATION' + rootCauseMatch[1]) as string
  }

  // Extract PROBLEM DESCRIPTION section
  const problemMatch = report.match(/# PROBLEM DESCRIPTION([\s\S]*?)(?=\n# |$)/i)
  if (problemMatch) {
    sections.problemDescription = marked.parse('# PROBLEM DESCRIPTION' + problemMatch[1]) as string
  }

  // Extract ANALYSIS FINDINGS section
  const findingsMatch = report.match(/# ANALYSIS FINDINGS([\s\S]*?)(?=\n# |$)/i)
  if (findingsMatch) {
    sections.analysisFindings = marked.parse('# ANALYSIS FINDINGS' + findingsMatch[1]) as string
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

const fetchAnalysis = async (showLoading = true) => {
  if (!workloadId.value) return

  if (showLoading) {
    loading.value = true
  }

  try {
    const result = await getRootCauseAnalysis(workloadId.value)
    analysisResult.value = result

    // If status is pending or running, continue polling
    if (result.status === 'pending' || result.status === 'running') {
      if (!pollingTimer.value) {
        isPolling.value = true
        pollingTimer.value = window.setInterval(() => {
          fetchAnalysis(false)
        }, 3000) // Poll every 3 seconds
      }
    } else {
      // Analysis completed or failed, stop polling
      stopPolling()
    }
  } catch (error) {
    console.error('Failed to fetch root cause analysis:', error)
    if (showLoading) {
      ElMessage.error('Failed to fetch analysis result')
    }
    stopPolling()
  } finally {
    if (showLoading) {
      loading.value = false
    }
  }
}

onMounted(() => {
  fetchAnalysis()
})

onBeforeUnmount(() => {
  stopPolling()
})

defineOptions({
  name: 'TrainingRootCauseDetail',
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
</style>
