<template>
  <div class="runs-page">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">Workflow Run Records</h2>
    </div>

    <!-- Filters -->
    <div class="filter-section">
      <el-select
        v-model="filters.configId"
        placeholder="Select Config"
        clearable
        filterable
        class="filter-select-wide"
        @change="onSearch"
      >
        <el-option
          v-for="config in configOptions"
          :key="config.id"
          :label="config.name"
          :value="config.id"
        />
      </el-select>
      <el-select
        v-model="filters.status"
        placeholder="Status"
        clearable
        class="filter-select"
        @change="onSearch"
      >
        <el-option label="Pending" value="pending" />
        <el-option label="Collecting" value="collecting" />
        <el-option label="Extracting" value="extracting" />
        <el-option label="Completed" value="completed" />
        <el-option label="Failed" value="failed" />
      </el-select>
      <el-select
        v-model="filters.triggerSource"
        placeholder="Source"
        clearable
        class="filter-select"
        @change="onSearch"
      >
        <el-option label="Realtime" value="realtime" />
        <el-option label="Backfill" value="backfill" />
        <el-option label="Manual" value="manual" />
      </el-select>
      <el-date-picker
        v-model="filters.dateRange"
        type="daterange"
        range-separator="to"
        start-placeholder="Start"
        end-placeholder="End"
        value-format="YYYY-MM-DD"
        class="date-picker"
        @change="onSearch"
      />
      <el-button :icon="Refresh" @click="resetFilters">Reset</el-button>
    </div>

    <!-- Table -->
    <el-card class="table-card">
      <el-table
        v-loading="loading"
        :data="tableData"
        style="width: 100%"
        @row-click="goToDetail"
      >
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="Config" min-width="150">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click.stop="goToConfig(row.configId)">
              {{ getConfigName(row.configId) }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="workloadName" label="Workload" min-width="220">
          <template #default="{ row }">
            <el-tooltip :content="row.workloadName" placement="top">
              <span class="workload-text">{{ row.workloadName }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="filesProcessed" label="Files" width="80" align="center">
          <template #default="{ row }">
            {{ row.filesProcessed }}/{{ row.filesFound }}
          </template>
        </el-table-column>
        <el-table-column prop="metricsCount" label="Metrics" width="100" align="center" />
        <el-table-column prop="status" label="Status" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              <el-icon v-if="isProcessing(row.status)" class="is-loading">
                <Loading />
              </el-icon>
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="triggerSource" label="Source" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="getSourceType(row.triggerSource)" size="small" effect="plain">
              {{ row.triggerSource }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="retryCount" label="Retries" width="80" align="center">
          <template #default="{ row }">
            <span v-if="row.retryCount > 0" class="retry-count">{{ row.retryCount }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="Created" width="180">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="Duration" width="100">
          <template #default="{ row }">{{ getDuration(row) }}</template>
        </el-table-column>
        <el-table-column label="Actions" width="80" fixed="right" align="center">
          <template #default="{ row }">
            <el-tooltip v-if="row.status === 'failed'" content="Retry">
              <el-button link :icon="Refresh" @click.stop="retryRun(row)" />
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-if="pagination.total > 0"
        v-model:current-page="pagination.pageNum"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        @current-change="fetchData"
        @size-change="fetchData"
        class="mt-4"
      />
    </el-card>

    <!-- Run Detail Drawer -->
    <el-drawer
      v-model="showDetailDrawer"
      :title="`Run #${selectedRun?.id}`"
      size="50%"
    >
      <template v-if="selectedRun">
        <div class="run-detail">
          <!-- Status Timeline -->
          <div class="timeline-section">
            <h4>Status Timeline</h4>
            <el-steps :active="getStepActive(selectedRun.status)" finish-status="success">
              <el-step title="Pending" :description="formatDate(selectedRun.createdAt)" />
              <el-step title="Collecting" :description="selectedRun.collectionStartedAt ? formatDate(selectedRun.collectionStartedAt) : '-'" />
              <el-step title="Extracting" description="" />
              <el-step 
                :title="selectedRun.status === 'failed' ? 'Failed' : 'Completed'" 
                :description="selectedRun.collectionCompletedAt ? formatDate(selectedRun.collectionCompletedAt) : '-'"
                :status="selectedRun.status === 'failed' ? 'error' : undefined"
              />
            </el-steps>
          </div>

          <!-- Error Message -->
          <el-alert
            v-if="selectedRun.errorMessage"
            :title="selectedRun.errorMessage"
            type="error"
            show-icon
            :closable="false"
            class="error-alert"
          />

          <!-- Run Info -->
          <el-descriptions :column="2" border class="run-info">
            <el-descriptions-item label="Workload">{{ selectedRun.workloadName }}</el-descriptions-item>
            <el-descriptions-item label="Namespace">{{ selectedRun.workloadNamespace }}</el-descriptions-item>
            <el-descriptions-item label="Files Found">{{ selectedRun.filesFound }}</el-descriptions-item>
            <el-descriptions-item label="Files Processed">{{ selectedRun.filesProcessed }}</el-descriptions-item>
            <el-descriptions-item label="Metrics Count">{{ selectedRun.metricsCount }}</el-descriptions-item>
            <el-descriptions-item label="Retry Count">{{ selectedRun.retryCount }}</el-descriptions-item>
            <el-descriptions-item label="Trigger Source">{{ selectedRun.triggerSource }}</el-descriptions-item>
            <el-descriptions-item label="Branch">{{ selectedRun.headBranch || '-' }}</el-descriptions-item>
          </el-descriptions>

          <!-- Metrics Preview -->
          <div class="metrics-section" v-if="selectedRun.metricsCount > 0">
            <div class="section-header">
              <h4>Metrics Preview</h4>
              <el-button link type="primary" @click="viewInExplorer">View in Explorer</el-button>
            </div>
            <el-table :data="runMetrics" size="small" max-height="300" v-loading="loadingMetrics">
              <el-table-column type="index" width="50" />
              <el-table-column label="Dimensions" min-width="200">
                <template #default="{ row }">
                  <div class="dimensions-cell">
                    <el-tag 
                      v-for="(value, key) in row.dimensions" 
                      :key="key" 
                      size="small"
                      class="dim-tag"
                    >
                      {{ key }}: {{ value }}
                    </el-tag>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="Metrics" min-width="150">
                <template #default="{ row }">
                  <div class="metrics-cell">
                    <span v-for="(value, key) in row.metrics" :key="key" class="metric-item">
                      {{ key }}: <strong>{{ formatMetricValue(value) }}</strong>
                    </span>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Loading } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  getConfigs,
  getRunsByConfig,
  getRun,
  getRunMetrics,
  type WorkflowConfig,
  type WorkflowRun,
  type MetricRecord
} from '@/services/workflow-metrics'

const route = useRoute()
const router = useRouter()

// State
const loading = ref(false)
const tableData = ref<WorkflowRun[]>([])
const configOptions = ref<WorkflowConfig[]>([])
const pagination = reactive({
  pageNum: 1,
  pageSize: 20,
  total: 0
})
const filters = reactive({
  configId: undefined as number | undefined,
  status: '',
  triggerSource: '',
  dateRange: [] as string[]
})

// Detail drawer
const showDetailDrawer = ref(false)
const selectedRun = ref<WorkflowRun | null>(null)
const runMetrics = ref<MetricRecord[]>([])
const loadingMetrics = ref(false)

// Initialize from route query
onMounted(async () => {
  // Load config options
  try {
    const res = await getConfigs({ limit: 100 })
    configOptions.value = res.configs || []
  } catch (error) {
    console.error('Failed to load configs:', error)
  }

  // Check for configId in query
  if (route.query.configId) {
    filters.configId = Number(route.query.configId)
  }

  fetchData()
})

// Watch for route query changes
watch(() => route.query.configId, (newVal) => {
  if (newVal) {
    filters.configId = Number(newVal)
    fetchData()
  }
})

// Methods
const fetchData = async () => {
  if (!filters.configId) {
    tableData.value = []
    pagination.total = 0
    return
  }

  loading.value = true
  try {
    const res = await getRunsByConfig(filters.configId, {
      offset: (pagination.pageNum - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      status: filters.status || undefined,
      triggerSource: filters.triggerSource || undefined
    })
    tableData.value = res.runs || []
    pagination.total = res.total || 0
  } catch (error) {
    console.error('Failed to fetch runs:', error)
    ElMessage.error('Failed to fetch runs')
  } finally {
    loading.value = false
  }
}

const onSearch = () => {
  pagination.pageNum = 1
  fetchData()
}

const resetFilters = () => {
  filters.configId = undefined
  filters.status = ''
  filters.triggerSource = ''
  filters.dateRange = []
  pagination.pageNum = 1
  fetchData()
}

const getConfigName = (configId: number) => {
  return configOptions.value.find(c => c.id === configId)?.name || `Config #${configId}`
}

const formatDate = (date: string) => {
  if (!date) return '-'
  return dayjs(date).format('YYYY-MM-DD HH:mm:ss')
}

const getDuration = (run: WorkflowRun) => {
  if (!run.collectionStartedAt || !run.collectionCompletedAt) return '-'
  const start = dayjs(run.collectionStartedAt)
  const end = dayjs(run.collectionCompletedAt)
  const diff = end.diff(start, 'second')
  if (diff < 60) return `${diff}s`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`
  return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`
}

const getStatusType = (status: string) => {
  const types: Record<string, string> = {
    pending: 'info',
    collecting: 'primary',
    extracting: 'warning',
    completed: 'success',
    failed: 'danger'
  }
  return types[status] || 'info'
}

const getSourceType = (source: string) => {
  const types: Record<string, string> = {
    realtime: 'success',
    backfill: 'warning',
    manual: 'info'
  }
  return types[source] || 'info'
}

const isProcessing = (status: string) => {
  return status === 'collecting' || status === 'extracting'
}

const getStepActive = (status: string) => {
  const steps: Record<string, number> = {
    pending: 0,
    collecting: 1,
    extracting: 2,
    completed: 4,
    failed: 3
  }
  return steps[status] || 0
}

const goToDetail = async (row: WorkflowRun) => {
  selectedRun.value = row
  showDetailDrawer.value = true
  
  // Load metrics
  if (row.metricsCount > 0) {
    loadingMetrics.value = true
    try {
      const res = await getRunMetrics(row.id, { limit: 20 })
      runMetrics.value = res.metrics || []
    } catch (error) {
      console.error('Failed to load metrics:', error)
    } finally {
      loadingMetrics.value = false
    }
  }
}

const goToConfig = (configId: number) => {
  router.push(`/workflow-metrics/configs/${configId}`)
}

const retryRun = (run: WorkflowRun) => {
  ElMessage.info('Retry functionality coming soon')
}

const viewInExplorer = () => {
  if (selectedRun.value) {
    router.push(`/workflow-metrics/explorer?configId=${selectedRun.value.configId}`)
    showDetailDrawer.value = false
  }
}

const formatMetricValue = (value: number) => {
  if (typeof value !== 'number') return value
  return value.toFixed(2)
}
</script>

<style scoped lang="scss">
.runs-page {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;

    .page-title {
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin: 0;
    }
  }

  .filter-section {
    display: flex;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;

    .filter-select {
      width: 140px;
    }

    .filter-select-wide {
      width: 200px;
    }

    .date-picker {
      width: 280px;
    }
  }

  .table-card {
    border-radius: 12px;

    :deep(.el-card__body) {
      padding: 20px;
    }

    .workload-text {
      display: inline-block;
      max-width: 200px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .retry-count {
      color: var(--el-color-warning);
      font-weight: 500;
    }
  }
}

// Drawer styles
.run-detail {
  .timeline-section {
    margin-bottom: 24px;

    h4 {
      margin: 0 0 16px;
      font-size: 15px;
      font-weight: 600;
    }
  }

  .error-alert {
    margin-bottom: 24px;
  }

  .run-info {
    margin-bottom: 24px;
  }

  .metrics-section {
    .section-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 12px;

      h4 {
        margin: 0;
        font-size: 15px;
        font-weight: 600;
      }
    }

    .dimensions-cell {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;

      .dim-tag {
        font-size: 11px;
      }
    }

    .metrics-cell {
      .metric-item {
        display: block;
        font-size: 12px;
        color: var(--el-text-color-secondary);

        strong {
          color: var(--el-text-color-primary);
        }
      }
    }
  }
}
</style>

