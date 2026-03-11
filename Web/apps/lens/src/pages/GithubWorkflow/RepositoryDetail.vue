<template>
  <div class="repository-detail">
    <!-- Header -->
    <div class="page-header">
      <el-button @click="goBack" :icon="ArrowLeft">Back to Repositories</el-button>
      <h2 class="page-title" v-if="repoSummary">
        <el-icon><Link /></el-icon>
        {{ repoSummary.owner }}/{{ repoSummary.repo }}
        <el-link 
          :href="`https://github.com/${repoSummary.owner}/${repoSummary.repo}`" 
          target="_blank" 
          :underline="false"
          class="github-link"
        >
          <el-icon><TopRight /></el-icon>
        </el-link>
      </h2>
    </div>

    <!-- Running Workflow Banner -->
    <transition name="banner-fade">
      <div v-if="repoSummary && repoSummary.runningWorkflows > 0" class="running-banner">
        <div class="banner-content">
          <span class="pulse-dot"></span>
          <span class="banner-text">
            <strong>{{ repoSummary.runningWorkflows }}</strong> workflow{{ repoSummary.runningWorkflows > 1 ? 's' : '' }} currently running
          </span>
        </div>
      </div>
    </transition>

    <!-- Tabs -->
    <el-tabs v-model="activeTab" class="detail-tabs">
      <!-- Overview Tab -->
      <el-tab-pane label="Overview" name="overview">
        <template #label>
          <span class="tab-label">
            <el-icon><DataAnalysis /></el-icon>
            Overview
          </span>
        </template>

        <div class="tab-content">
          <!-- Stats Cards -->
          <div class="stats-cards">
            <el-card class="stat-card">
              <div class="stat-content">
                <div class="stat-icon stat-icon--primary">
                  <el-icon><Box /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label">Runner Sets</div>
                  <div class="stat-value stat-value--primary">{{ repoSummary?.runnerSetCount || 0 }}</div>
                </div>
              </div>
            </el-card>

            <el-card class="stat-card">
              <div class="stat-content">
                <div class="stat-icon stat-icon--success">
                  <el-icon><Check /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label">Active Runners</div>
                  <div class="stat-value stat-value--success">{{ repoSummary?.totalRunners || 0 }} / {{ repoSummary?.maxRunners || 0 }}</div>
                </div>
              </div>
            </el-card>

            <el-card class="stat-card">
              <div class="stat-content">
                <div class="stat-icon stat-icon--warning">
                  <el-icon><DataLine /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label">Total Runs</div>
                  <div class="stat-value stat-value--warning">{{ formatNumber(repoSummary?.totalRuns || 0) }}</div>
                </div>
              </div>
            </el-card>

            <el-card class="stat-card">
              <div class="stat-content">
                <div class="stat-icon stat-icon--info">
                  <el-icon><Setting /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label">Configured</div>
                  <div class="stat-value stat-value--info">{{ repoSummary?.configuredSets || 0 }}</div>
                </div>
              </div>
            </el-card>
          </div>

          <!-- Run Statistics -->
          <el-card class="overview-card">
            <template #header>
              <div class="card-header">
                <span>Run Statistics</span>
              </div>
            </template>
            <div class="run-stats-grid">
              <div class="run-stat-item">
                <div class="stat-icon success">
                  <el-icon><Check /></el-icon>
                </div>
                <div class="stat-details">
                  <div class="stat-value">{{ repoSummary?.completedRuns || 0 }}</div>
                  <div class="stat-label">Completed</div>
                </div>
              </div>
              <div class="run-stat-item">
                <div class="stat-icon warning">
                  <el-icon><Clock /></el-icon>
                </div>
                <div class="stat-details">
                  <div class="stat-value">{{ repoSummary?.pendingRuns || 0 }}</div>
                  <div class="stat-label">Pending</div>
                </div>
              </div>
              <div class="run-stat-item">
                <div class="stat-icon danger">
                  <el-icon><Close /></el-icon>
                </div>
                <div class="stat-details">
                  <div class="stat-value">{{ repoSummary?.failedRuns || 0 }}</div>
                  <div class="stat-label">Failed</div>
                </div>
              </div>
              <div class="run-stat-item">
                <div class="stat-icon info">
                  <el-icon><VideoPlay /></el-icon>
                </div>
                <div class="stat-details">
                  <div class="stat-value">{{ repoSummary?.runningWorkflows || 0 }}</div>
                  <div class="stat-label">Running</div>
                </div>
              </div>
            </div>
          </el-card>
        </div>
      </el-tab-pane>

      <!-- Runner Sets Tab -->
      <el-tab-pane label="Runner Sets" name="runner-sets">
        <template #label>
          <span class="tab-label">
            <el-icon><Box /></el-icon>
            Runner Sets
            <el-badge v-if="runnerSets.length > 0" :value="runnerSets.length" class="tab-badge" />
          </span>
        </template>

        <div class="tab-content">
          <el-card class="table-card">
            <el-table
              v-loading="runnerSetsLoading"
              :data="runnerSets"
              style="width: 100%"
            >
              <el-table-column label="Runner Set" min-width="280">
                <template #default="{ row }">
                  <div class="runner-set-cell">
                    <el-link 
                      type="primary" 
                      :underline="false" 
                      @click="goToRunnerSetDetail(row)"
                      class="workload-link"
                    >
                      {{ row.name }}
                    </el-link>
                    <div class="runner-namespace">{{ row.namespace }}</div>
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="Runners" width="140" align="center">
                <template #default="{ row }">
                  <div class="runners-cell">
                    <span class="current">{{ row.currentRunners }}</span>
                    <span class="separator">/</span>
                    <span class="max">{{ row.maxRunners }}</span>
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="Collection Config" min-width="180">
                <template #default="{ row }">
                  <div v-if="row.hasConfig" class="config-cell">
                    <el-tag type="success" size="small" effect="plain">
                      <el-icon class="config-icon"><Setting /></el-icon>
                      {{ row.configName || 'Configured' }}
                    </el-tag>
                  </div>
                  <span v-else class="text-muted">Not configured</span>
                </template>
              </el-table-column>

              <el-table-column label="Runs" width="180" align="center">
                <template #default="{ row }">
                  <div class="runs-stats-cell">
                    <el-tooltip content="Total Runs" placement="top">
                      <span class="run-stat total">{{ row.totalRuns || 0 }}</span>
                    </el-tooltip>
                    <span class="separator">/</span>
                    <el-tooltip content="Completed" placement="top">
                      <span class="run-stat completed">{{ row.completedRuns || 0 }}</span>
                    </el-tooltip>
                    <span class="separator">/</span>
                    <el-tooltip content="Failed" placement="top">
                      <span class="run-stat failed">{{ row.failedRuns || 0 }}</span>
                    </el-tooltip>
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="Status" width="120" align="center">
                <template #default="{ row }">
                  <el-tag
                    :type="row.status === 'active' ? 'success' : row.status === 'inactive' ? 'warning' : 'info'"
                    effect="light"
                  >
                    {{ row.status }}
                  </el-tag>
                </template>
              </el-table-column>
            </el-table>
          </el-card>
        </div>
      </el-tab-pane>

      <!-- Workflow Runs Tab (Run-level aggregation) -->
      <el-tab-pane label="Workflow Runs" name="workflow-runs">
        <template #label>
          <span class="tab-label">
            <el-icon><VideoPlay /></el-icon>
            Workflow Runs
            <el-badge v-if="runSummariesTotal > 0" :value="runSummariesTotal > 99 ? '99+' : runSummariesTotal" class="tab-badge" />
          </span>
        </template>

        <div class="tab-content">
          <!-- Filters -->
          <el-card class="filter-card" shadow="never">
            <div class="filter-row">
              <el-select v-model="runSummaryFilter.status" placeholder="Status" clearable style="width: 160px" @change="fetchRunSummaries">
                <el-option label="Queued" value="queued" />
                <el-option label="In Progress" value="in_progress" />
                <el-option label="Completed" value="completed" />
              </el-select>
              <el-select v-model="runSummaryFilter.conclusion" placeholder="Conclusion" clearable style="width: 160px" @change="fetchRunSummaries">
                <el-option label="Success" value="success" />
                <el-option label="Failure" value="failure" />
                <el-option label="Cancelled" value="cancelled" />
              </el-select>
              <el-input v-model="runSummaryFilter.headBranch" placeholder="Branch" clearable style="width: 180px" @clear="fetchRunSummaries" @keyup.enter="fetchRunSummaries" />
              <el-button @click="fetchRunSummaries" :icon="Search">Search</el-button>
            </div>
          </el-card>

          <!-- Run Summaries Table -->
          <el-card class="table-card">
            <el-table
              v-loading="runSummariesLoading"
              :data="runSummaries"
              style="width: 100%"
              @row-click="goToRunSummaryDetail"
              row-class-name="clickable-row"
            >
              <el-table-column label="Run" min-width="320">
                <template #default="{ row }">
                  <div class="run-info-cell">
                    <div class="run-title">
                      <StatusIcon :status="row.status" :conclusion="row.conclusion" />
                      <span class="workflow-name">{{ row.workflowName || 'Workflow' }}</span>
                      <span class="run-number">#{{ row.githubRunNumber }}</span>
                    </div>
                    <div class="run-meta">
                      <BranchIcon />
                      <span class="branch">{{ row.headBranch || '-' }}</span>
                      <span class="separator">·</span>
                      <span class="sha">{{ row.headSha?.substring(0, 7) || '-' }}</span>
                      <span class="separator">·</span>
                      <span class="event">{{ row.eventName || '-' }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="Jobs" width="200" align="center">
                <template #default="{ row }">
                  <div class="jobs-stats">
                    <el-tooltip content="Successful / Total">
                      <span class="job-stat success">{{ row.successfulJobs }}</span>
                    </el-tooltip>
                    <span class="separator">/</span>
                    <el-tooltip content="Failed">
                      <span class="job-stat failed">{{ row.failedJobs }}</span>
                    </el-tooltip>
                    <span class="separator">/</span>
                    <el-tooltip content="Total Jobs">
                      <span class="job-stat total">{{ row.totalJobs }}</span>
                    </el-tooltip>
                    <el-progress 
                      v-if="row.status === 'in_progress'" 
                      :percentage="row.progressPercent" 
                      :stroke-width="4"
                      :show-text="false"
                      style="width: 60px; margin-left: 8px;"
                    />
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="Status" width="130" align="center">
                <template #default="{ row }">
                  <StatusBadge :status="row.status" :conclusion="row.conclusion" />
                </template>
              </el-table-column>

              <el-table-column label="Triggered By" width="140">
                <template #default="{ row }">
                  <div class="actor-cell">
                    <span>{{ row.actor || row.triggeringActor || '-' }}</span>
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="Duration" width="120" align="center">
                <template #default="{ row }">
                  <span v-if="row.runStartedAt">
                    {{ formatDuration(row.runStartedAt, row.runCompletedAt) }}
                  </span>
                  <span v-else class="text-muted">-</span>
                </template>
              </el-table-column>

              <el-table-column label="Started" width="160" align="center">
                <template #default="{ row }">
                  <el-tooltip v-if="row.runStartedAt" :content="row.runStartedAt" placement="top">
                    <span>{{ formatRelativeTime(row.runStartedAt) }}</span>
                  </el-tooltip>
                  <span v-else class="text-muted">-</span>
                </template>
              </el-table-column>
            </el-table>

            <!-- Pagination -->
            <div class="table-pagination">
              <el-pagination
                v-model:current-page="runSummaryPagination.page"
                v-model:page-size="runSummaryPagination.pageSize"
                :total="runSummariesTotal"
                :page-sizes="[10, 20, 50]"
                layout="total, sizes, prev, pager, next"
                @size-change="fetchRunSummaries"
                @current-change="fetchRunSummaries"
              />
            </div>
          </el-card>
        </div>
      </el-tab-pane>

      <!-- Analytics Tab -->
      <el-tab-pane label="Analytics" name="analytics" v-if="hasConfiguredSets">
        <template #label>
          <span class="tab-label">
            <el-icon><TrendCharts /></el-icon>
            Analytics
          </span>
        </template>

        <div class="tab-content">
          <el-alert
            v-if="!metricsMetadata || configsWithData.length === 0"
            type="info"
            :closable="false"
            show-icon
          >
            No metrics data found. Configure collection for runner sets and wait for data to be collected.
          </el-alert>

          <template v-else>
            <!-- Query Builder -->
            <el-card class="analytics-card query-builder-card">
              <template #header>
                <div class="card-header">
                  <span>Metrics Explorer</span>
                </div>
              </template>

              <el-form :inline="true" class="query-form">
                <!-- Config Selector -->
                <el-form-item label="Configuration">
                  <el-select v-model="selectedConfigId" placeholder="Select config" style="width: 250px" @change="onConfigChange">
                    <el-option
                      v-for="config in configsWithData"
                      :key="config.configId"
                      :label="`${config.configName} (${config.recordCount} records)`"
                      :value="config.configId"
                    />
                  </el-select>
                </el-form-item>

                <!-- Time Range -->
                <el-form-item label="Time Range">
                  <el-date-picker
                    v-model="queryForm.timeRange"
                    type="daterange"
                    range-separator="to"
                    start-placeholder="Start"
                    end-placeholder="End"
                    value-format="YYYY-MM-DDTHH:mm:ssZ"
                    :shortcuts="dateShortcuts"
                    style="width: 300px"
                  />
                </el-form-item>

                <!-- Interval -->
                <el-form-item label="Interval">
                  <el-select v-model="queryForm.interval" style="width: 100px">
                    <el-option label="1 Hour" value="1h" />
                    <el-option label="6 Hours" value="6h" />
                    <el-option label="1 Day" value="1d" />
                    <el-option label="1 Week" value="1w" />
                  </el-select>
                </el-form-item>
              </el-form>

              <!-- Metrics Selection -->
              <div class="metrics-selection" v-if="selectedConfig">
                <div class="selection-label">Select Metrics:</div>
                <el-checkbox-group v-model="queryForm.selectedMetrics" class="metrics-checkboxes">
                  <el-checkbox 
                    v-for="metric in (selectedConfig.metricFields || [])" 
                    :key="metric" 
                    :value="metric"
                  >
                    {{ metric }}
                  </el-checkbox>
                </el-checkbox-group>
              </div>

              <!-- Query Button -->
              <div class="query-actions">
                <el-button 
                  type="primary" 
                  :icon="Search" 
                  :loading="querying"
                  :disabled="queryForm.selectedMetrics.length === 0"
                  @click="executeQuery"
                >
                  Query Metrics
                </el-button>
                <el-button @click="resetQuery">Reset</el-button>
              </div>
            </el-card>

            <!-- Chart Display - One chart per metric -->
            <template v-if="hasChartData">
              <el-card class="analytics-card chart-card" v-for="(metricSeries, idx) in chartSeriesGrouped" :key="metricSeries.field">
                <template #header>
                  <div class="card-header">
                    <span>{{ metricSeries.name || metricSeries.field }}</span>
                    <div class="chart-controls">
                      <el-radio-group v-model="chartType" size="small">
                        <el-radio-button value="line">Line</el-radio-button>
                        <el-radio-button value="bar">Bar</el-radio-button>
                      </el-radio-group>
                    </div>
                  </div>
                </template>
                <div :ref="el => setChartRef(el, idx)" class="chart-container" v-loading="querying"></div>
              </el-card>
            </template>

            <!-- No Data Message -->
            <el-card v-else-if="queryExecuted && !querying" class="analytics-card">
              <el-empty description="No data found for the selected criteria" />
            </el-card>

            <!-- Configs Overview -->
            <el-card class="analytics-card">
              <template #header>
                <div class="card-header">
                  <span>Available Configurations ({{ configsWithData.length }})</span>
                </div>
              </template>

              <el-table :data="configsWithData" style="width: 100%" size="small">
                <el-table-column prop="configName" label="Config Name" min-width="150" />
                <el-table-column prop="runnerSetName" label="Runner Set" min-width="120" />
                <el-table-column prop="recordCount" label="Records" width="100" align="right">
                  <template #default="{ row }">
                    <span class="record-count">{{ formatNumber(row.recordCount) }}</span>
                  </template>
                </el-table-column>
                <el-table-column label="Metrics" width="100" align="center">
                  <template #default="{ row }">
                    {{ (row.metricFields || []).length }}
                  </template>
                </el-table-column>
                <el-table-column label="Action" width="100" align="center">
                  <template #default="{ row }">
                    <el-button 
                      type="primary" 
                      link 
                      size="small"
                      @click="selectConfig(row.configId)"
                    >
                      Select
                    </el-button>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>
          </template>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, reactive, nextTick, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft, Link, TopRight, DataAnalysis, Box, Check, DataLine,
  Setting, Clock, Close, VideoPlay, TrendCharts, InfoFilled, Search
} from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import {
  getRepository,
  getRepositoryRunnerSets,
  getRepositoryMetricsMetadata,
  getRepositoryMetricsTrends,
  getRunSummaries,
  type RepositorySummary,
  type RunnerSetWithStats,
  type RepositoryMetricsMetadata,
  type TrendsResponse,
  type WorkflowRunSummary,
  type RunSummaryFilter
} from '@/services/workflow-metrics'
import { useClusterSync } from '@/composables/useClusterSync'
import StatusIcon from './components/StatusIcon.vue'
import StatusBadge from './components/StatusBadge.vue'
import BranchIcon from './components/BranchIcon.vue'

dayjs.extend(relativeTime)

const route = useRoute()
const router = useRouter()
const { selectedCluster } = useClusterSync()

// Props from route
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)

// State
const activeTab = ref('overview')
const repoSummary = ref<RepositorySummary | null>(null)
const runnerSets = ref<RunnerSetWithStats[]>([])
const runnerSetsLoading = ref(false)
const metricsMetadata = ref<RepositoryMetricsMetadata | null>(null)

// Run Summaries State
const runSummaries = ref<WorkflowRunSummary[]>([])
const runSummariesTotal = ref(0)
const runSummariesLoading = ref(false)
const runSummaryFilter = reactive<RunSummaryFilter>({
  status: undefined,
  conclusion: undefined,
  headBranch: undefined
})
const runSummaryPagination = reactive({
  page: 1,
  pageSize: 20
})
const selectedSchemaView = ref<'single' | 'common'>('single')
const selectedConfigId = ref<number | null>(null)

// Computed
const hasConfiguredSets = computed(() => {
  return (repoSummary.value?.configuredSets || 0) > 0
})

// Filter configs that have data (recordCount > 0)
const configsWithData = computed(() => {
  if (!metricsMetadata.value?.configs) return []
  return metricsMetadata.value.configs
    .filter(c => c.recordCount > 0)
    .sort((a, b) => b.recordCount - a.recordCount) // Sort by record count descending
})

// Check if there are common fields across all configs
const hasCommonFields = computed(() => {
  if (!metricsMetadata.value) return false
  const commonDims = metricsMetadata.value.commonDimensions || []
  const commonMetrics = metricsMetadata.value.commonMetrics || []
  return configsWithData.value.length > 1 && (commonDims.length > 0 || commonMetrics.length > 0)
})

// Get selected config details
const selectedConfig = computed(() => {
  if (!selectedConfigId.value || !metricsMetadata.value?.configs) return null
  return metricsMetadata.value.configs.find(c => c.configId === selectedConfigId.value) || null
})

// Select a config by ID
const selectConfig = (configId: number) => {
  selectedConfigId.value = configId
  // Reset query form when config changes
  queryForm.selectedMetrics = []
  queryExecuted.value = false
  trendsData.value = null
}

// Initialize selected config when metadata loads
const initializeSelectedConfig = () => {
  if (configsWithData.value.length > 0 && !selectedConfigId.value) {
    // Default to the config with most records
    selectedConfigId.value = configsWithData.value[0].configId
  }
}

// Query form
const queryForm = reactive({
  timeRange: [] as string[],
  selectedMetrics: [] as string[],
  interval: '1d'
})

// Chart state
const chartRefs = ref<(HTMLElement | null)[]>([])
const chartInstances: echarts.ECharts[] = []
const chartType = ref<'line' | 'bar'>('line')
const querying = ref(false)
const queryExecuted = ref(false)
const trendsData = ref<TrendsResponse | null>(null)

// Chart colors
const chartColors = [
  '#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de',
  '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc', '#48b8d0'
]

// Date shortcuts
const dateShortcuts = [
  { text: 'Last 7 days', value: () => [dayjs().subtract(7, 'day').toDate(), dayjs().toDate()] },
  { text: 'Last 30 days', value: () => [dayjs().subtract(30, 'day').toDate(), dayjs().toDate()] },
  { text: 'Last 90 days', value: () => [dayjs().subtract(90, 'day').toDate(), dayjs().toDate()] }
]

// Has chart data
const hasChartData = computed(() => {
  return trendsData.value && trendsData.value.series && trendsData.value.series.length > 0
})

// Group series by metric field - one chart per metric
const chartSeriesGrouped = computed(() => {
  if (!trendsData.value?.series) return []
  return trendsData.value.series
})

// Set chart ref
const setChartRef = (el: HTMLElement | null, idx: number) => {
  chartRefs.value[idx] = el
}

// On config change
const onConfigChange = () => {
  queryForm.selectedMetrics = []
  queryExecuted.value = false
  trendsData.value = null
}

// Execute query
const executeQuery = async () => {
  if (!selectedConfigId.value || queryForm.selectedMetrics.length === 0) {
    ElMessage.warning('Please select a configuration and at least one metric')
    return
  }

  querying.value = true
  queryExecuted.value = true

  try {
    const result = await getRepositoryMetricsTrends(owner.value, repo.value, {
      start: queryForm.timeRange[0] || dayjs().subtract(30, 'day').format('YYYY-MM-DDTHH:mm:ssZ'),
      end: queryForm.timeRange[1] || dayjs().format('YYYY-MM-DDTHH:mm:ssZ'),
      configIds: [selectedConfigId.value],
      metricFields: queryForm.selectedMetrics,
      interval: queryForm.interval
    })
    
    trendsData.value = result
    await nextTick()
    renderCharts()
  } catch (error) {
    console.error('Failed to query metrics:', error)
    ElMessage.error('Failed to query metrics')
  } finally {
    querying.value = false
  }
}

// Reset query
const resetQuery = () => {
  queryForm.timeRange = []
  queryForm.selectedMetrics = []
  queryForm.interval = '1d'
  queryExecuted.value = false
  trendsData.value = null
  chartInstances.forEach(instance => instance.dispose())
  chartInstances.length = 0
}

// Render all charts - one per metric
const renderCharts = () => {
  if (!trendsData.value) return

  const { timestamps, series } = trendsData.value
  if (!series || series.length === 0) return

  // Sort timestamps for proper x-axis display
  const sortedIndices = timestamps
    .map((t, i) => ({ t: dayjs(t).valueOf(), i }))
    .sort((a, b) => a.t - b.t)
    .map(item => item.i)
  
  const sortedTimestamps = sortedIndices.map(i => timestamps[i])

  // Check dark mode
  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const subtextColor = isDark ? '#A3A6AD' : '#606266'

  // Dispose old instances
  chartInstances.forEach(instance => instance.dispose())
  chartInstances.length = 0

  // Create chart for each series
  series.forEach((s, idx) => {
    const el = chartRefs.value[idx]
    if (!el) return

    const instance = echarts.init(el)
    chartInstances.push(instance)

    // Sort values according to sorted timestamps
    const sortedValues = sortedIndices.map(i => s.values[i])

    const option: echarts.EChartsOption = {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: chartType.value === 'line' ? 'line' : 'shadow' },
        valueFormatter: (value) => typeof value === 'number' ? value.toFixed(2) : String(value)
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '10%',
        top: '10%',
        containLabel: true
      },
      xAxis: {
        type: 'category',
        data: sortedTimestamps.map(t => dayjs(t).format('YYYY-MM-DD')),
        axisLabel: { rotate: 30, interval: 'auto', color: subtextColor },
        axisLine: { lineStyle: { color: subtextColor } }
      },
      yAxis: {
        type: 'value',
        name: s.field,
        nameTextStyle: { color: subtextColor },
        axisLabel: { color: subtextColor, formatter: (value: number) => value.toFixed(2) },
        axisLine: { lineStyle: { color: subtextColor } },
        splitLine: { lineStyle: { color: isDark ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.06)' } }
      },
      series: [{
        name: s.name || s.field,
        type: chartType.value,
        data: sortedValues,
        smooth: chartType.value === 'line',
        itemStyle: { color: chartColors[idx % chartColors.length] },
        lineStyle: chartType.value === 'line' ? { color: chartColors[idx % chartColors.length] } : undefined,
        areaStyle: chartType.value === 'line' ? { 
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: chartColors[idx % chartColors.length] + '40' },
            { offset: 1, color: chartColors[idx % chartColors.length] + '05' }
          ])
        } : undefined
      }]
    }

    instance.setOption(option, true)
  })
}

// Watch chart type change
watch(chartType, () => {
  if (hasChartData.value) {
    renderCharts()
  }
})

// Handle resize
const handleResize = () => {
  chartInstances.forEach(instance => instance.resize())
}

// Cleanup
onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chartInstances.forEach(instance => instance.dispose())
})

// Methods
const goBack = () => {
  router.push({
    path: '/github-workflow',
    query: selectedCluster.value ? { cluster: selectedCluster.value } : undefined
  })
}

const goToRunnerSetDetail = (row: RunnerSetWithStats) => {
  const cluster = selectedCluster.value
  router.push({
    path: `/github-workflow/runner-sets/${row.id}`,
    query: cluster ? { cluster } : undefined
  })
}

const fetchRepoSummary = async () => {
  try {
    repoSummary.value = await getRepository(owner.value, repo.value)
  } catch (error) {
    console.error('Failed to fetch repository summary:', error)
    ElMessage.error('Failed to load repository details')
  }
}

const fetchRunnerSets = async () => {
  runnerSetsLoading.value = true
  try {
    const res = await getRepositoryRunnerSets(owner.value, repo.value, true)
    runnerSets.value = res.runnerSets || []
  } catch (error) {
    console.error('Failed to fetch runner sets:', error)
    ElMessage.error('Failed to load runner sets')
  } finally {
    runnerSetsLoading.value = false
  }
}

const fetchMetricsMetadata = async () => {
  try {
    metricsMetadata.value = await getRepositoryMetricsMetadata(owner.value, repo.value)
    initializeSelectedConfig()
  } catch (error) {
    console.error('Failed to fetch metrics metadata:', error)
  }
}

const fetchRunSummaries = async () => {
  runSummariesLoading.value = true
  try {
    const params: RunSummaryFilter = {
      ...runSummaryFilter,
      offset: (runSummaryPagination.page - 1) * runSummaryPagination.pageSize,
      limit: runSummaryPagination.pageSize
    }
    // Remove undefined values
    Object.keys(params).forEach(key => {
      if (params[key as keyof RunSummaryFilter] === undefined || params[key as keyof RunSummaryFilter] === '') {
        delete params[key as keyof RunSummaryFilter]
      }
    })
    const res = await getRunSummaries(owner.value, repo.value, params)
    runSummaries.value = res.runSummaries || []
    runSummariesTotal.value = res.total || 0
  } catch (error) {
    console.error('Failed to fetch run summaries:', error)
    ElMessage.error('Failed to load workflow runs')
  } finally {
    runSummariesLoading.value = false
  }
}

const goToRunSummaryDetail = (row: WorkflowRunSummary) => {
  // Navigate to run detail page with the first job's run ID or create a new route
  const cluster = selectedCluster.value
  router.push({
    path: `/github-workflow/run-summary/${row.id}`,
    query: cluster ? { cluster } : undefined
  })
}

const formatDuration = (start: string, end?: string) => {
  const startTime = dayjs(start)
  const endTime = end ? dayjs(end) : dayjs()
  const duration = endTime.diff(startTime, 'second')
  
  if (duration < 60) return `${duration}s`
  if (duration < 3600) return `${Math.floor(duration / 60)}m ${duration % 60}s`
  return `${Math.floor(duration / 3600)}h ${Math.floor((duration % 3600) / 60)}m`
}

const formatRelativeTime = (time: string) => {
  return dayjs(time).fromNow()
}

const formatNumber = (num: number) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

// Lifecycle
onMounted(async () => {
  window.addEventListener('resize', handleResize)
  await Promise.all([
    fetchRepoSummary(),
    fetchRunnerSets(),
    fetchMetricsMetadata(),
    fetchRunSummaries()
  ])
})

// Watch for route changes
watch([owner, repo], async () => {
  await Promise.all([
    fetchRepoSummary(),
    fetchRunnerSets(),
    fetchMetricsMetadata(),
    fetchRunSummaries()
  ])
})

// Watch for cluster changes
watch(selectedCluster, async (newCluster, oldCluster) => {
  if (newCluster && newCluster !== oldCluster) {
    await Promise.all([
      fetchRepoSummary(),
      fetchRunnerSets(),
      fetchMetricsMetadata(),
      fetchRunSummaries()
    ])
  }
})

// Watch for tab changes to load data
watch(activeTab, async (newTab) => {
  if (newTab === 'workflow-runs' && runSummaries.value.length === 0) {
    await fetchRunSummaries()
  }
})
</script>

<style scoped lang="scss">
@import '@/styles/stats-layout.scss';

.repository-detail {
  padding: 20px;

  .page-header {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 20px;

    .page-title {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 20px;
      font-weight: 600;
      margin: 0;

      .github-link {
        color: var(--el-text-color-secondary);
        &:hover {
          color: var(--el-color-primary);
        }
      }
    }
  }

  .running-banner {
    background: linear-gradient(135deg, var(--el-color-warning-light-3), var(--el-color-warning-light-5));
    border-radius: 8px;
    padding: 12px 20px;
    margin-bottom: 20px;
    display: flex;
    align-items: center;
    justify-content: space-between;

    .banner-content {
      display: flex;
      align-items: center;
      gap: 12px;

      .pulse-dot {
        width: 10px;
        height: 10px;
        background: var(--el-color-warning);
        border-radius: 50%;
        animation: pulse 1.5s ease-in-out infinite;
      }

      .banner-text {
        color: var(--el-text-color-primary);
      }
    }
  }

  .detail-tabs {
    .tab-label {
      display: flex;
      align-items: center;
      gap: 6px;

      .tab-badge {
        margin-left: 4px;
      }
    }
  }

  .tab-content {
    padding: 16px 0;
  }

  .stats-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
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

          &.stat-icon--primary {
            background: var(--el-color-primary-light-9);
            color: var(--el-color-primary);
          }
          &.stat-icon--success {
            background: var(--el-color-success-light-9);
            color: var(--el-color-success);
          }
          &.stat-icon--warning {
            background: var(--el-color-warning-light-9);
            color: var(--el-color-warning);
          }
          &.stat-icon--info {
            background: var(--el-color-info-light-9);
            color: var(--el-color-info);
          }
        }

        .stat-info {
          .stat-label {
            font-size: 12px;
            color: var(--el-text-color-secondary);
          }
          .stat-value {
            font-size: 24px;
            font-weight: 600;
          }
        }
      }
    }
  }

  .overview-card {
    .run-stats-grid {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 20px;

      .run-stat-item {
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 16px;
        background: var(--el-fill-color-light);
        border-radius: 8px;

        .stat-icon {
          width: 40px;
          height: 40px;
          border-radius: 50%;
          display: flex;
          align-items: center;
          justify-content: center;
          font-size: 18px;

          &.success {
            background: var(--el-color-success-light-9);
            color: var(--el-color-success);
          }
          &.warning {
            background: var(--el-color-warning-light-9);
            color: var(--el-color-warning);
          }
          &.danger {
            background: var(--el-color-danger-light-9);
            color: var(--el-color-danger);
          }
          &.info {
            background: var(--el-color-info-light-9);
            color: var(--el-color-info);
          }
        }

        .stat-details {
          .stat-value {
            font-size: 24px;
            font-weight: 600;
          }
          .stat-label {
            font-size: 12px;
            color: var(--el-text-color-secondary);
          }
        }
      }
    }
  }

  .table-card {
    .runner-set-cell {
      .runner-namespace {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        font-family: monospace;
      }
    }

    .runners-cell {
      font-family: monospace;
      .current {
        color: var(--el-color-success);
        font-weight: 600;
      }
      .separator {
        color: var(--el-text-color-secondary);
        margin: 0 2px;
      }
      .max {
        color: var(--el-text-color-secondary);
      }
    }

    .config-cell {
      .config-icon {
        margin-right: 4px;
      }
    }

    .runs-stats-cell {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 2px;
      font-family: monospace;

      .run-stat {
        font-weight: 500;
        min-width: 24px;
        text-align: center;

        &.total {
          color: var(--el-text-color-primary);
        }
        &.completed {
          color: var(--el-color-success);
        }
        &.failed {
          color: var(--el-color-danger);
        }
      }

      .separator {
        color: var(--el-text-color-placeholder);
      }
    }
  }

  .analytics-card {
    margin-bottom: 20px;

    .card-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;

      .chart-controls {
        display: flex;
        align-items: center;
        gap: 12px;
      }
    }

    &.query-builder-card {
      .query-form {
        margin-bottom: 16px;
      }

      .metrics-selection {
        margin-bottom: 20px;
        padding: 16px;
        background: var(--el-fill-color-light);
        border-radius: 8px;

        .selection-label {
          font-size: 14px;
          font-weight: 500;
          margin-bottom: 12px;
          color: var(--el-text-color-primary);
        }

        .metrics-checkboxes {
          display: flex;
          flex-wrap: wrap;
          gap: 12px;
        }
      }

      .query-actions {
        display: flex;
        gap: 12px;
      }
    }

    &.chart-card {
      .chart-container {
        width: 100%;
        height: 400px;
      }
    }

    .schema-view {
      .config-selector {
        display: flex;
        align-items: center;
        gap: 12px;
        margin-bottom: 20px;

        .selector-label {
          font-size: 14px;
          color: var(--el-text-color-regular);
        }
      }

      .selected-config-details {
        .config-info {
          margin-bottom: 20px;
        }

        .fields-display {
          .field-section {
            margin-bottom: 20px;

            &:last-child {
              margin-bottom: 0;
            }

            h4 {
              font-size: 14px;
              color: var(--el-text-color-primary);
              margin: 0 0 12px 0;
              font-weight: 500;
            }

            .field-tags {
              display: flex;
              flex-wrap: wrap;
              gap: 8px;

              .el-tag {
                margin: 0;
              }
            }
          }
        }
      }
    }

    .fields-display {
      .field-section {
        margin-bottom: 20px;

        &:last-child {
          margin-bottom: 0;
        }

        h4 {
          font-size: 14px;
          color: var(--el-text-color-primary);
          margin: 0 0 12px 0;
          font-weight: 500;
        }

        .field-tags {
          display: flex;
          flex-wrap: wrap;
          gap: 8px;

          .el-tag {
            margin: 0;
          }
        }
      }
    }

    .record-count {
      font-family: monospace;
      font-weight: 500;
    }

    .configs-list {
      .config-item {
        padding: 16px;
        background: var(--el-fill-color-light);
        border-radius: 8px;
        margin-bottom: 12px;

        &:last-child {
          margin-bottom: 0;
        }

        .config-header {
          display: flex;
          align-items: center;
          gap: 12px;
          margin-bottom: 8px;

          .config-name {
            font-weight: 500;
            font-size: 14px;
          }
        }

        .config-meta {
          font-size: 12px;
          color: var(--el-text-color-secondary);
          margin-bottom: 12px;

          span {
            margin-right: 12px;
          }
        }

        .config-fields {
          .field-group {
            display: flex;
            flex-wrap: wrap;
            align-items: center;
            gap: 6px;
            margin-bottom: 8px;

            .field-label {
              font-size: 12px;
              color: var(--el-text-color-secondary);
              margin-right: 4px;
            }
          }
        }
      }
    }

    .common-fields {
      .field-section {
        margin-bottom: 16px;

        &:last-child {
          margin-bottom: 0;
        }

        h4 {
          font-size: 13px;
          color: var(--el-text-color-secondary);
          margin: 0 0 8px 0;
        }

        .el-tag {
          margin-right: 8px;
          margin-bottom: 4px;
        }
      }
    }
  }
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}

.banner-fade-enter-active,
.banner-fade-leave-active {
  transition: all 0.3s ease;
}

.banner-fade-enter-from,
.banner-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

// Workflow Runs Tab styles
.filter-card {
  margin-bottom: 16px;
  
  .filter-row {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
  }
}

.clickable-row {
  cursor: pointer;
  
  &:hover {
    background-color: var(--el-fill-color-light);
  }
}

.run-info-cell {
  .run-title {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
    
    .workflow-name {
      font-weight: 500;
      color: var(--el-text-color-primary);
    }
    
    .run-number {
      color: var(--el-text-color-secondary);
      font-size: 13px;
    }
  }
  
  .run-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    
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

.jobs-stats {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  
  .job-stat {
    font-weight: 500;
    
    &.success {
      color: var(--el-color-success);
    }
    
    &.failed {
      color: var(--el-color-danger);
    }
    
    &.total {
      color: var(--el-text-color-secondary);
    }
  }
  
  .separator {
    color: var(--el-text-color-placeholder);
  }
}

.actor-cell {
  font-size: 13px;
  color: var(--el-text-color-regular);
}

.table-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 16px 0 0;
}
</style>
