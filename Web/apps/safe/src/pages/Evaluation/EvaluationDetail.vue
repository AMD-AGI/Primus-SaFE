<template>
  <div v-loading="loading">
    <div v-if="detail" class="mb-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <el-button :icon="ArrowLeft" @click="handleBack" class="mr-0">Back</el-button>
          <div>
            <div class="flex items-center gap-3">
              <h2 class="text-xl font-semibold">{{ detail.taskName }}</h2>
              <el-tag
                v-if="detail.status"
                :type="STATUS_META[detail.status]?.type || 'info'"
                :effect="isDark ? 'plain' : 'light'"
              >
                {{ STATUS_META[detail.status]?.label || detail.status }}
              </el-tag>
            </div>
            <div class="text-sm text-gray-500 mt-1 flex items-center gap-2">
              <span>{{ detail.taskId }}</span>
              <span>•</span>
              <span>Created by {{ detail.userName || 'Unknown' }}</span>
              <span>•</span>
              <span>{{ formatTime(detail.creationTime) }}</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <el-tooltip content="Clone" placement="top">
            <el-button circle class="glass-btn glass-btn--clone" @click="onClone">
              <el-icon><DocumentCopy /></el-icon>
            </el-button>
          </el-tooltip>

          <el-tooltip content="Delete" placement="top">
            <el-button circle class="glass-btn glass-btn--danger" @click="onDelete">
              <el-icon><Delete /></el-icon>
            </el-button>
          </el-tooltip>

          <el-tooltip
            :content="isTaskCompleted(detail.status) ? 'Already completed' : 'Stop'"
            placement="top"
          >
            <el-button
              circle
              class="glass-btn glass-btn--warning"
              :disabled="isTaskCompleted(detail.status)"
              @click="onStop"
            >
              <el-icon><CloseBold /></el-icon>
            </el-button>
          </el-tooltip>
        </div>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="mt-4" v-if="detail">
      <el-tab-pane label="Overview" name="overview">
        <el-card class="mt-2 safe-card" shadow="never">
          <!-- Basic Information -->
          <div class="flex items-center mb-4">
            <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
            <span class="textx-15 font-medium">Basic Information</span>
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Task Name">
              {{ detail.taskName || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Task ID">
              {{ detail.taskId || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Status">
              <el-tag
                v-if="detail.status"
                :type="STATUS_META[detail.status]?.type || 'info'"
                :effect="isDark ? 'plain' : 'light'"
              >
                {{ STATUS_META[detail.status]?.label || detail.status }}
              </el-tag>
              <span v-else>-</span>
            </el-descriptions-item>
            <el-descriptions-item label="Service Name">
              {{ detail.serviceName || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Service ID">
              {{ detail.serviceId || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Evaluation Type">
              <el-tag
                v-if="detail.evaluationType"
                :type="detail.evaluationType === 'normal' ? 'warning' : 'success'"
                :effect="isDark ? 'plain' : 'light'"
                size="small"
              >
                {{ detail.evaluationType === 'normal' ? 'Normal' : 'Judge' }}
              </el-tag>
              <span v-else>-</span>
            </el-descriptions-item>
            <el-descriptions-item label="Judge Service" v-if="detail.evaluationType === 'judge'">
              <div class="flex flex-col gap-1">
                <span>{{ detail.judgeServiceName || '-' }}</span>
                <span class="text-xs text-gray-400">{{ detail.judgeServiceId || '-' }}</span>
              </div>
            </el-descriptions-item>
            <el-descriptions-item label="Workspace">
              {{ detail.workspace || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="User">
              {{ detail.userName || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Creation Time">
              {{ formatTime(detail.creationTime) }}
            </el-descriptions-item>
            <el-descriptions-item label="Start Time">
              {{ formatTime(detail.startTime) }}
            </el-descriptions-item>
            <el-descriptions-item label="End Time">
              {{ formatTime(detail.endTime) }}
            </el-descriptions-item>
            <el-descriptions-item label="Duration">
              {{ calculateDuration(detail.startTime, detail.endTime) }}
            </el-descriptions-item>
            <el-descriptions-item label="Ops Job ID">
              {{ detail.opsJobId || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Description" :span="2">
              {{ detail.description || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Concurrency">
              {{ detail.concurrency || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Timeout">
              {{ detail.timeout || '-' }}
            </el-descriptions-item>
          </el-descriptions>

          <!-- Benchmarks -->
          <div class="flex items-center mb-4 mt-6">
            <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
            <span class="textx-15 font-medium">Benchmarks</span>
            <span class="text-gray-400 text-[12px] ml-2"
              >({{ detail?.benchmarks?.length || 0 }} items)</span
            >
          </div>
          <el-table :data="detail?.benchmarks || []" border>
            <el-table-column prop="datasetName" label="Dataset Name" min-width="180" />
            <el-table-column
              prop="datasetId"
              label="Dataset ID"
              min-width="200"
              show-overflow-tooltip
            />
            <el-table-column prop="limit" label="Limit" width="120">
              <template #default="{ row }">
                {{ row.limit ?? '-' }}
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="Report" name="report" lazy>
        <el-card class="mt-2 safe-card" shadow="never" v-loading="reportLoading">
          <template v-if="report?.results">
            <!-- Single Dataset Report -->
            <template v-if="report.results.dataset_name && !report.results.datasets">
              <div class="flex items-center mb-4">
                <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
                <span class="textx-15 font-medium">{{ report.results.dataset_pretty_name }}</span>
              </div>

              <el-descriptions :column="2" border class="mb-4">
                <el-descriptions-item label="Dataset">
                  {{ report.results.dataset_pretty_name || report.results.dataset_name }}
                </el-descriptions-item>
                <el-descriptions-item label="Model">
                  {{ report.results.model_name || '-' }}
                </el-descriptions-item>
                <el-descriptions-item label="Overall Score">
                  <el-tag type="success" size="large">{{
                    formatScore(report.results.score)
                  }}</el-tag>
                </el-descriptions-item>
                <el-descriptions-item label="Duration">
                  {{ report.duration || '-' }}
                </el-descriptions-item>
                <el-descriptions-item label="Description" :span="2">
                  {{ report.results.dataset_description || '-' }}
                </el-descriptions-item>
              </el-descriptions>

              <!-- Metrics -->
              <div class="flex items-center mb-4 mt-6">
                <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
                <span class="textx-15 font-medium">Metrics</span>
              </div>
              <el-table
                :data="report.results.metrics || []"
                border
                style="width: 100%"
                row-key="name"
                :default-expand-all="true"
              >
                <el-table-column type="expand">
                  <template #default="{ row }">
                    <div
                      v-if="row.categories && row.categories.length"
                      class="px-8 py-4 bg-gray-50"
                    >
                      <div class="text-sm font-semibold mb-3 text-gray-700">Categories</div>
                      <div class="space-y-3">
                        <div
                          v-for="(cat, idx) in row.categories"
                          :key="idx"
                          class="bg-white rounded-lg p-4 border border-gray-200"
                        >
                          <div class="flex items-center gap-4 mb-3">
                            <div class="flex-1">
                              <span class="text-sm font-medium text-gray-600">Category:</span>
                              <span class="ml-2 font-semibold">{{
                                cat.name?.join(', ') || 'default'
                              }}</span>
                            </div>
                            <el-tag type="success">Score: {{ formatScore(cat.score) }}</el-tag>
                            <el-tag type="info">Macro: {{ formatScore(cat.macro_score) }}</el-tag>
                            <el-tag type="warning">Samples: {{ cat.num }}</el-tag>
                          </div>
                          <div v-if="cat.subsets && cat.subsets.length" class="mt-2">
                            <div class="text-xs text-gray-500 mb-2">Subsets:</div>
                            <div class="flex flex-wrap gap-2">
                              <div
                                v-for="(subset, sidx) in cat.subsets"
                                :key="sidx"
                                class="inline-flex items-center gap-2 bg-blue-50 px-3 py-1 rounded text-sm border border-blue-200"
                              >
                                <span class="font-medium text-blue-900">{{ subset.name }}</span>
                                <span class="text-blue-700">{{ formatScore(subset.score) }}</span>
                                <span class="text-blue-600 text-xs">({{ subset.num }})</span>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                    <div v-else class="px-8 py-4 bg-gray-50 text-gray-400 text-sm text-center">
                      No categories data
                    </div>
                  </template>
                </el-table-column>
                <el-table-column prop="name" label="Metric Name" min-width="150" />
                <el-table-column prop="score" label="Score" width="120">
                  <template #default="{ row }">
                    <el-tag type="success">{{ formatScore(row.score) }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="macro_score" label="Macro Score" width="140">
                  <template #default="{ row }">
                    {{ formatScore(row.macro_score) }}
                  </template>
                </el-table-column>
                <el-table-column prop="num" label="Samples" width="120" />
                <el-table-column label="Categories" width="120" align="center">
                  <template #default="{ row }">
                    <el-tag
                      v-if="row.categories && row.categories.length"
                      size="small"
                      type="primary"
                    >
                      {{ row.categories.length }} items
                    </el-tag>
                    <span v-else class="text-gray-400">-</span>
                  </template>
                </el-table-column>
              </el-table>
            </template>

            <!-- Multi Dataset Report -->
            <template v-else-if="report.results?.datasets">
              <!-- Summary -->
              <div class="flex items-center mb-4">
                <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
                <span class="textx-15 font-medium">Summary</span>
              </div>
              <el-descriptions :column="2" border class="mb-6">
                <el-descriptions-item label="Average Score">
                  <el-tag type="success" size="large">
                    {{ formatScore(report.results.summary?.average_score) }}
                  </el-tag>
                </el-descriptions-item>
                <el-descriptions-item label="Total Datasets">
                  {{ report.results.summary?.total_datasets || report.results.datasets.length }}
                </el-descriptions-item>
                <el-descriptions-item label="Duration">
                  {{ report.duration || '-' }}
                </el-descriptions-item>
              </el-descriptions>

              <!-- Datasets Results -->
              <div class="flex items-center mb-4">
                <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
                <span class="textx-15 font-medium">Dataset Results</span>
              </div>
              <el-collapse accordion>
                <el-collapse-item
                  v-for="(dataset, idx) in report.results.datasets"
                  :key="idx"
                  :name="idx"
                >
                  <template #title>
                    <div class="flex items-center justify-between w-full pr-4">
                      <span class="font-medium">
                        {{ dataset.dataset_pretty_name || dataset.dataset_name }}
                      </span>
                      <el-tag type="success">Score: {{ formatScore(dataset.score) }}</el-tag>
                    </div>
                  </template>

                  <el-descriptions :column="2" border class="mb-4">
                    <el-descriptions-item label="Dataset">
                      {{ dataset.dataset_pretty_name || dataset.dataset_name }}
                    </el-descriptions-item>
                    <el-descriptions-item label="Model">
                      {{ dataset.model_name || '-' }}
                    </el-descriptions-item>
                    <el-descriptions-item label="Score">
                      {{ formatScore(dataset.score) }}
                    </el-descriptions-item>
                    <el-descriptions-item label="Description" :span="2">
                      {{ dataset.dataset_description || '-' }}
                    </el-descriptions-item>
                  </el-descriptions>

                  <div class="text-sm font-medium mb-2 mt-4">Metrics</div>
                  <el-table
                    :data="dataset.metrics || []"
                    border
                    size="small"
                    style="width: 100%"
                    row-key="name"
                    :default-expand-all="true"
                  >
                    <el-table-column type="expand">
                      <template #default="{ row }">
                        <div
                          v-if="row.categories && row.categories.length"
                          class="px-6 py-3 bg-gray-50"
                        >
                          <div class="text-xs font-semibold mb-2 text-gray-700">Categories</div>
                          <div class="space-y-2">
                            <div
                              v-for="(cat, idx) in row.categories"
                              :key="idx"
                              class="bg-white rounded p-3 border border-gray-200"
                            >
                              <div class="flex items-center gap-3 mb-2 flex-wrap">
                                <div class="flex-1 min-w-[120px]">
                                  <span class="text-xs text-gray-600">Category:</span>
                                  <span class="ml-1 text-sm font-medium">{{
                                    cat.name?.join(', ') || 'default'
                                  }}</span>
                                </div>
                                <el-tag size="small" type="success">{{
                                  formatScore(cat.score)
                                }}</el-tag>
                                <el-tag size="small" type="info"
                                  >Macro: {{ formatScore(cat.macro_score) }}</el-tag
                                >
                                <el-tag size="small" type="warning">{{ cat.num }}</el-tag>
                              </div>
                              <div v-if="cat.subsets && cat.subsets.length" class="mt-2">
                                <div class="text-xs text-gray-500 mb-1">Subsets:</div>
                                <div class="flex flex-wrap gap-1">
                                  <span
                                    v-for="(subset, sidx) in cat.subsets"
                                    :key="sidx"
                                    class="inline-flex items-center gap-1 bg-blue-50 px-2 py-0.5 rounded text-xs border border-blue-200"
                                  >
                                    <span class="font-medium text-blue-900">{{ subset.name }}</span>
                                    <span class="text-blue-700">{{
                                      formatScore(subset.score)
                                    }}</span>
                                    <span class="text-blue-600">({{ subset.num }})</span>
                                  </span>
                                </div>
                              </div>
                            </div>
                          </div>
                        </div>
                        <div v-else class="px-6 py-3 bg-gray-50 text-gray-400 text-xs text-center">
                          No categories data
                        </div>
                      </template>
                    </el-table-column>
                    <el-table-column prop="name" label="Metric" min-width="120" />
                    <el-table-column prop="score" label="Score" width="100">
                      <template #default="{ row }">
                        <el-tag size="small" type="success">{{ formatScore(row.score) }}</el-tag>
                      </template>
                    </el-table-column>
                    <el-table-column prop="macro_score" label="Macro Score" width="120">
                      <template #default="{ row }">
                        {{ formatScore(row.macro_score) }}
                      </template>
                    </el-table-column>
                    <el-table-column prop="num" label="Samples" width="90" />
                    <el-table-column label="Categories" width="100" align="center">
                      <template #default="{ row }">
                        <el-tag
                          v-if="row.categories && row.categories.length"
                          size="small"
                          type="primary"
                        >
                          {{ row.categories.length }}
                        </el-tag>
                        <span v-else class="text-gray-400">-</span>
                      </template>
                    </el-table-column>
                  </el-table>
                </el-collapse-item>
              </el-collapse>
            </template>
          </template>

          <el-empty v-else description="No report available" />
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="Logs" name="logs" lazy>
        <!-- v-if="detail?.opsJobId && userStore.envs?.enableLog" -->
        <LogTable
          v-if="workloadDetail && detail?.opsJobId"
          :wlid="detail.opsJobId"
          :dispatchCount="workloadDetail.dispatchCount"
          :nodes="workloadDetail.nodes"
          :ranks="workloadDetail.ranks"
          :failedNodes="workloadDetail.failedNodes"
          :isDownload="userStore.envs?.enableLogDownload || false"
        />
        <el-card v-else v-loading="workloadLoading" class="mt-2 safe-card" shadow="never">
          <el-empty description="Loading workload information..." />
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ArrowLeft, DocumentCopy, Delete, CloseBold } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDark } from '@vueuse/core'
import {
  getEvaluationTaskDetail,
  getEvaluationReport,
  stopEvaluationTask,
  deleteEvaluationTask,
} from '@/services/evaluations'
import { getWorkloadDetail } from '@/services/workload'
import type { EvaluationTaskDetail } from '@/services/evaluations/type'
import { useUserStore } from '@/stores/user'
import LogTable from '@/pages/CICD/Components/LogTable.vue'

const router = useRouter()
const route = useRoute()
const isDark = useDark()
const userStore = useUserStore()

interface WorkloadDetail {
  dispatchCount?: number
  nodes?: string[][]
  ranks?: string[][]
  failedNodes?: string[]
}

interface EvaluationReport {
  taskId: string
  taskName: string
  serviceName: string
  status: string
  results?: {
    dataset_name?: string
    dataset_pretty_name?: string
    dataset_description?: string
    model_name?: string
    score?: number
    metrics?: Array<{
      name: string
      score?: number
      macro_score?: number
      num?: number
      categories?: Array<{
        name?: string[]
        score?: number
        num?: number
      }>
    }>
    datasets?: Array<{
      dataset_name: string
      dataset_pretty_name?: string
      dataset_description?: string
      model_name?: string
      score?: number
      metrics?: Array<{
        name: string
        score?: number
        macro_score?: number
        num?: number
      }>
    }>
    summary?: {
      average_score?: number
      total_datasets?: number
    }
  }
  startTime?: string
  endTime?: string
  duration?: string
}

const activeTab = ref('overview')
const loading = ref(false)
const reportLoading = ref(false)
const workloadLoading = ref(false)
const detail = ref<EvaluationTaskDetail | null>(null)
const report = ref<EvaluationReport | null>(null)
const workloadDetail = ref<WorkloadDetail | null>(null)

const STATUS_META: Record<string, { label: string; type: string }> = {
  Pending: { label: 'Pending', type: 'info' },
  Running: { label: 'Running', type: 'primary' },
  Succeeded: { label: 'Completed', type: 'success' },
  Failed: { label: 'Failed', type: 'danger' },
}

const isTaskCompleted = (status?: string) => {
  return status === 'Succeeded' || status === 'Failed'
}

const formatTime = (time?: string) => {
  if (!time) return '-'
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

const calculateDuration = (startTime?: string, endTime?: string) => {
  if (!startTime || !endTime) return '-'
  const start = dayjs(startTime)
  const end = dayjs(endTime)
  const diffSeconds = end.diff(start, 'second')

  if (diffSeconds < 60) return `${diffSeconds}s`
  if (diffSeconds < 3600) {
    const minutes = Math.floor(diffSeconds / 60)
    const seconds = diffSeconds % 60
    return `${minutes}m ${seconds}s`
  }
  const hours = Math.floor(diffSeconds / 3600)
  const minutes = Math.floor((diffSeconds % 3600) / 60)
  return `${hours}h ${minutes}m`
}

const formatScore = (score?: number) => {
  if (score == null || Number.isNaN(score)) return '-'
  return Number(score).toFixed(4)
}

const fetchDetail = async () => {
  const taskId = route.params.taskId as string
  if (!taskId) {
    ElMessage.error('Missing task ID')
    router.push('/evaluation')
    return
  }

  try {
    loading.value = true
    detail.value = await getEvaluationTaskDetail(taskId)
  } catch (error) {
    console.error('Failed to fetch evaluation task detail:', error)
    ElMessage.error('Failed to fetch evaluation task detail')
  } finally {
    loading.value = false
  }
}

const fetchReport = async () => {
  const taskId = route.params.taskId as string
  if (!taskId) return

  try {
    reportLoading.value = true
    report.value = await getEvaluationReport(taskId)
  } catch (error) {
    console.error('Failed to fetch evaluation report:', error)
    // Don't show error message as report might not be available yet
  } finally {
    reportLoading.value = false
  }
}

const fetchWorkloadDetail = async () => {
  if (!detail.value?.opsJobId) return

  try {
    workloadLoading.value = true
    workloadDetail.value = await getWorkloadDetail(detail.value.opsJobId)
  } catch (error) {
    console.error('Failed to fetch workload detail:', error)
    ElMessage.error('Failed to fetch workload detail')
  } finally {
    workloadLoading.value = false
  }
}

const handleBack = () => {
  router.push('/evaluation')
}

const onClone = () => {
  router.push({
    path: '/evaluation',
    query: { clone: detail.value?.taskId },
  })
}

const onDelete = async () => {
  if (!detail.value) return

  const msg = h('span', null, [
    'Are you sure you want to delete evaluation task: ',
    h(
      'span',
      { style: 'color: var(--el-color-primary); font-weight: 600' },
      detail.value.taskName || detail.value.taskId,
    ),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, 'Delete evaluation task', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await deleteEvaluationTask(detail.value.taskId)
    ElMessage.success('Delete completed')
    router.push('/evaluation')
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error('Failed to delete task')
    }
  }
}

const onStop = async () => {
  if (!detail.value) return

  const msg = h('span', null, [
    'Are you sure you want to stop evaluation task: ',
    h(
      'span',
      { style: 'color: var(--el-color-primary); font-weight: 600' },
      detail.value.taskName || detail.value.taskId,
    ),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, 'Stop evaluation task', {
      confirmButtonText: 'Stop',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await stopEvaluationTask(detail.value.taskId)
    ElMessage.success('Stop complete')
    fetchDetail()
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error('Failed to stop task')
    }
  }
}

// Watch detail to fetch workload detail when available
watch(
  () => detail.value?.opsJobId,
  (opsJobId) => {
    if (opsJobId) {
      fetchWorkloadDetail()
    }
  },
  { immediate: true },
)

onMounted(() => {
  fetchDetail()
  fetchReport()
  userStore.fetchEnvs()
})
</script>

<style scoped>
.params-textarea :deep(.el-textarea__inner) {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
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

.glass-btn--clone {
  color: var(--el-color-success);
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

/* Dark mode support for metrics categories */
html.dark .bg-gray-50 {
  background-color: rgba(255, 255, 255, 0.05) !important;
}

html.dark .bg-white {
  background-color: var(--el-bg-color-overlay) !important;
}

html.dark .border-gray-200 {
  border-color: var(--el-border-color) !important;
}

html.dark .text-gray-700 {
  color: var(--el-text-color-primary) !important;
}

html.dark .text-gray-600 {
  color: var(--el-text-color-regular) !important;
}

html.dark .text-gray-500 {
  color: var(--el-text-color-secondary) !important;
}

html.dark .bg-blue-50 {
  background-color: rgba(64, 158, 255, 0.1) !important;
}

html.dark .border-blue-200 {
  border-color: rgba(64, 158, 255, 0.3) !important;
}

html.dark .text-blue-900 {
  color: #a0cfff !important;
}

html.dark .text-blue-700 {
  color: #79bbff !important;
}

html.dark .text-blue-600 {
  color: #79bbff !important;
}
</style>
