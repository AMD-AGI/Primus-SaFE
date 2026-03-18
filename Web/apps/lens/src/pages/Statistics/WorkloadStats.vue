<template>
  <div class="workload-stats">
    <div class="filter-section">
      <div class="filter-header">
        <h2 class="page-title">Workload GPU Statistics</h2>
        <div class="filters">
          <el-form :inline="true" :model="filters">
          <!-- Basic search -->
          <el-form-item>
            <el-date-picker
              v-model="timeRange"
              type="datetimerange"
              range-separator="to"
              start-placeholder="Start Time"
              end-placeholder="End Time"
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              size="default"
              class="time-picker"
              popper-class="custom-date-picker"
            />
          </el-form-item>
          
          <el-form-item>
            <el-input 
              v-model="filters.workloadName" 
              placeholder="Workload Name (optional)"
              clearable
              size="default"
              style="width: 200px"
            />
          </el-form-item>
          </el-form>
        </div>
      </div>
    </div>

    <!-- Statistics Cards - Only show in overview mode (no time range selected) -->
    <div class="stats-cards" v-if="isOverviewMode">
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--primary">
            <i i="ep-document" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Total Workloads
              <el-tooltip 
                content="Total number of unique workloads using GPU resources" 
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--primary">{{ totalWorkloads || '-' }}</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--success">
            <i i="ep-cpu" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Avg GPU Allocated
              <el-tooltip 
                content="Average number of GPUs allocated per workload" 
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--success">{{ avgGpuAllocated ? avgGpuAllocated.toFixed(2) : '-' }}</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--danger">
            <i i="ep-odometer" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Avg Utilization
              <el-tooltip 
                content="Average GPU utilization rate across all workloads" 
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--danger">{{ avgUtilization.toFixed(2) }}%</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--warning">
            <i i="ep-warning" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Low Utilization
              <el-tooltip 
                content="Number of workloads with GPU utilization below 30%" 
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--warning">{{ lowUtilizationCount }}</div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Data Table -->
    <el-card class="table-card">
      <div class="table-wrapper">
      <el-table 
        v-loading="loading"
        :data="statsData" 
        stripe 
        style="width: 100%"
        @sort-change="handleTableSortChange"
        @filter-change="handleFilterChange"
      >
        <el-table-column prop="workloadName" min-width="260" fixed>
          <template #header>
            <span class="table-header-with-tip">
              Workload
              <el-tooltip 
                content="Name of the workload (Deployment, Job, StatefulSet, etc.)" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <el-link 
              type="primary" 
              :underline="false" 
              @click="showWorkloadDetail(row)"
              class="workload-link"
            >
              {{ row.workloadName || 'N/A' }}
            </el-link>
          </template>
        </el-table-column>

        
        <el-table-column prop="namespace" min-width="180" column-key="namespace" :filters="namespaceFilters" :filtered-value="filters.namespace ? [filters.namespace] : []">
          <template #header>
            <span class="table-header-with-tip">
              Namespace
              <el-tooltip 
                content="Kubernetes namespace where the workload is deployed" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            {{ row.namespace || 'N/A' }}
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="workloadStatus" 
          min-width="120"
          column-key="workloadStatus"
          :filters="statusFilters"
          :filtered-value="filters.workloadStatus ? [filters.workloadStatus] : []"
        >
          <template #header>
            <span class="table-header-with-tip">
              Status
              <el-tooltip 
                content="Current status of the workload (Running or Done)" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.workloadStatus)" size="small">
              {{ row.workloadStatus || 'Unknown' }}
            </el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="statHour" min-width="200" sortable="custom">
          <template #header>
            <span class="table-header-with-tip">
              {{ isOverviewMode ? 'Start Time' : 'Stat Time' }}
              <el-tooltip 
                :content="isOverviewMode ? 'Time when the workload started running' : 'Statistical collection timestamp for this data point'" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            {{ row.statHour ? formatTime(row.statHour) : 'N/A' }}
          </template>
        </el-table-column>
        
        <el-table-column prop="allocatedGpuCount" min-width="160" sortable="custom">
          <template #header>
            <span class="table-header-with-tip">
              GPU Allocated
              <el-tooltip 
                content="Number of GPUs actually allocated to the workload (may differ from requested)" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <span :class="{ 'warning-text': row.allocatedGpuCount !== row.requestedGpuCount }">
              {{ row.allocatedGpuCount?.toFixed(2) ?? '0.00' }}
            </span>
            <span v-if="!isOverviewMode && row.requestedGpuCount !== row.allocatedGpuCount" class="requested-gpu">
              / {{ row.requestedGpuCount?.toFixed(2) ?? '0.00' }}
            </span>
          </template>
        </el-table-column>
        
        <!-- Overview mode utilization columns -->
        <el-table-column v-if="isOverviewMode" prop="avgGpuUsage" min-width="180">
          <template #header>
            <span class="table-header-with-tip">
              Avg Utilization (3h)
              <el-tooltip 
                content="Average GPU utilization over the past 3 hours (click to view detailed chart)" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <el-link 
              type="primary"
              :disabled="row.avgGpuUsage === -1 || row.avgGpuUsage == null"
              @click="row.avgGpuUsage !== -1 && row.avgGpuUsage != null && viewUtilizationChart(row)"
              >
              {{ row.avgGpuUsage === -1 || row.avgGpuUsage == null ? '-' : `${row.avgGpuUsage.toFixed(2)}%` }}
            </el-link>
          </template>
        </el-table-column>
        
        <!-- Additional utilization metrics (overview mode only) -->
        <el-table-column v-if="isOverviewMode" prop="instantGpuUtilization" min-width="120">
          <template #header>
            <span class="table-header-with-tip">
              Instant Util
              <el-tooltip 
                content="Current real-time GPU utilization percentage" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <span :class="getUtilizationClass(row.instantGpuUtilization)">
              {{ row.instantGpuUtilization == null ? '-' : `${row.instantGpuUtilization.toFixed(2)}%` }}
            </span>
          </template>
        </el-table-column>
        
        <el-table-column v-if="isOverviewMode" prop="p50GpuUtilization" min-width="100">
          <template #header>
            <span class="table-header-with-tip">
              P50 Util
              <el-tooltip 
                content="50th percentile (median) GPU utilization - 50% of time the usage is below this value" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <span :class="getUtilizationClass(row.p50GpuUtilization)">
              {{ row.p50GpuUtilization == null ? '-' : `${row.p50GpuUtilization.toFixed(2)}%` }}
            </span>
          </template>
        </el-table-column>
        
        <el-table-column v-if="isOverviewMode" prop="p90GpuUtilization" min-width="100">
          <template #header>
            <span class="table-header-with-tip">
              P90 Util
              <el-tooltip 
                content="90th percentile GPU utilization - 90% of time the usage is below this value" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <span :class="getUtilizationClass(row.p90GpuUtilization)">
              {{ row.p90GpuUtilization == null ? '-' : `${row.p90GpuUtilization.toFixed(2)}%` }}
            </span>
          </template>
        </el-table-column>
        
        <el-table-column v-if="isOverviewMode" prop="p95GpuUtilization" min-width="100">
          <template #header>
            <span class="table-header-with-tip">
              P95 Util
              <el-tooltip 
                content="95th percentile GPU utilization - 95% of time the usage is below this value" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <span :class="getUtilizationClass(row.p95GpuUtilization)">
              {{ row.p95GpuUtilization == null ? '-' : `${row.p95GpuUtilization.toFixed(2)}%` }}
            </span>
          </template>
        </el-table-column>
        
        <!-- Historical mode utilization column -->
        <el-table-column v-else prop="avgUtilization" min-width="220" sortable="custom">
          <template #header>
            <span class="table-header-with-tip">
              Avg Utilization
              <el-tooltip 
                content="Average GPU utilization percentage during the selected time period" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            <div class="utilization-cell">
              <el-progress 
                :percentage="row.avgUtilization ?? 0" 
                :color="getProgressColor(row.avgUtilization ?? 0)"
                :stroke-width="10"
                :format="(percentage: number) => percentage.toFixed(2) + '%'"
              />
            </div>
          </template>
        </el-table-column>
        
        <el-table-column v-if="!isOverviewMode" prop="p50Utilization" min-width="120">
          <template #header>
            <span class="table-header-with-tip">
              P50 Util
              <el-tooltip 
                content="50th percentile (median) GPU utilization during the time period" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            {{ row.p50Utilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>
        
        <el-table-column v-if="!isOverviewMode" prop="p95Utilization" min-width="120">
          <template #header>
            <span class="table-header-with-tip">
              P95 Util
              <el-tooltip 
                content="95th percentile GPU utilization during the time period" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            {{ row.p95Utilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>
        
        <el-table-column v-if="!isOverviewMode" prop="avgGpuMemoryUsed" min-width="200">
          <template #header>
            <span class="table-header-with-tip">
              GPU Mem Used
              <el-tooltip 
                content="Average GPU memory usage (Used / Total) in GB" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            {{ row.avgGpuMemoryUsed?.toFixed(2) ?? '0.00' }} / {{ row.avgGpuMemoryTotal?.toFixed(2) ?? '0.00' }} GB
          </template>
        </el-table-column>
        
        <el-table-column v-if="!isOverviewMode" prop="avgReplicaCount" min-width="160">
          <template #header>
            <span class="table-header-with-tip">
              Replicas
              <el-tooltip 
                content="Average number of pod replicas for the workload" 
                placement="top"
              >
                <el-icon class="header-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <template #default="{ row }">
            {{ row.avgReplicaCount ?? '0' }}
          </template>
        </el-table-column>
        
        <template #empty>
          <el-empty description="No Data" />
        </template>
      </el-table>
      
      <el-pagination
        v-if="pagination.total > 0"
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="handlePageChange"
        @size-change="handlePageChange"
        class="mt-4"
      />
      </div>
    </el-card>
    
    <!-- Utilization Chart Dialog -->
    <el-dialog
      v-model="utilizationDialog.visible"
      width="1000"
      class="utilization-dialog"
      @close="utilizationDialog.visible = false"
    >
      <template #header>
        <div class="flex items-center justify-between pr-2">
          <div class="text-base font-600 leading-6 whitespace-nowrap">
            24h Utilization <span class="opacity-70">(Avg)</span>
          </div>
          <el-tooltip :content="utilizationDialog.title">
            <div class="opacity-70 truncate max-w-60">{{ utilizationDialog.title || '' }}</div>
          </el-tooltip>
        </div>
      </template>
      <div
        ref="chartRef"
        v-loading="utilizationDialog.loading"
        element-loading-text="Loading..."
        class="utilization-chart"
      />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, onBeforeUnmount, nextTick } from 'vue'
import { useRouter, onBeforeRouteLeave } from 'vue-router'
import { ElMessage } from 'element-plus'
import { QuestionFilled } from '@element-plus/icons-vue'
import { getWorkloadHourlyStats, getNamespaces, WorkloadGpuHourlyStats } from '@/services/gpu-aggregation'
import { getWorkloadsList, WorkloadRowItem, getWorkloadStatistics, WorkloadStatistics, getWorkloadGpuUtilizationHistory } from '@/services/dashboard'
import { useClusterSync } from '@/composables/useClusterSync'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import * as echarts from 'echarts'

dayjs.extend(utc)

// Debounce function implementation
function debounce<T extends (...args: any[]) => any>(fn: T, delay: number) {
  let timeoutId: ReturnType<typeof setTimeout> | undefined
  
  return function (this: any, ...args: Parameters<T>) {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => {
      fn.apply(this, args)
    }, delay)
  } as T
}

// Get global cluster
const { selectedCluster } = useClusterSync()
const router = useRouter()

// Session storage key for state persistence
const STATE_STORAGE_KEY = 'workloadStats_searchState'

const loading = ref(false)
const namespaceLoading = ref(false)
const allStatsData = ref<WorkloadGpuHourlyStats[]>([]) // All data for statistics
const statsData = ref<WorkloadGpuHourlyStats[]>([]) // Data displayed on current page
const workloadStatistics = ref<WorkloadStatistics | null>(null) // Statistics data from backend
const totalFromApi = ref(0) // Store total from API response
const timeRange = ref<[string, string]>()
const namespaceOptions = ref<string[]>([])

// Overview mode data
const isOverviewMode = computed(() => !timeRange.value || timeRange.value?.length !== 2)
const overviewData = ref<WorkloadRowItem[]>([])
const overviewTotal = ref(0)


// Table header filter options
const namespaceFilters = computed(() => {
  if (isOverviewMode.value) {
    const uniqueNamespaces = [...new Set(overviewData.value.map(item => item.namespace).filter(Boolean))]
    return uniqueNamespaces.map(ns => ({ text: ns, value: ns }))
  }
  const uniqueNamespaces = [...new Set(allStatsData.value.map(item => item.namespace).filter(Boolean))]
  return uniqueNamespaces.map(ns => ({ text: ns, value: ns }))
})

const workloadTypeFilters = computed(() => {
  if (isOverviewMode.value) {
    const uniqueTypes = [...new Set(overviewData.value.map(item => item.kind).filter(Boolean))]
    return uniqueTypes.map(type => ({ text: type, value: type }))
  }
  const uniqueTypes = [...new Set(allStatsData.value.map(item => item.workloadType).filter(Boolean))]
  return uniqueTypes.map(type => ({ text: type, value: type }))
})

const statusFilters = computed(() => {
  if (isOverviewMode.value) {
    const uniqueStatuses = [...new Set(overviewData.value.map(item => item.workloadStatus || item.status).filter(Boolean))]
    return uniqueStatuses.map(status => ({ text: status, value: status }))
  }
  // In historical mode, we don't have status data
  return []
})

const filters = ref({
  namespace: '',
  workloadName: '',
  workloadType: '',
  workloadStatus: ''
})

// Pagination (frontend pagination)
const pagination = ref({
  page: 1,
  pageSize: 20,
  total: 0
})

// Sorting
const currentSortProp = ref<string>('statHour')
const currentSortOrder = ref<'ascending' | 'descending'>('descending')

// Utilization Dialog
const chartRef = ref<HTMLElement>()
let chartInstance: echarts.ECharts | null = null
const utilizationDialog = ref({
  visible: false,
  loading: false,
  title: '',
  series: { x: [] as string[], avg: [] as (number | null)[] }
})

// Calculate statistics (Prefer backend data, otherwise compute from all data)
const totalWorkloads = computed(() => {
  // If backend statistics data is available, use it first
  if (workloadStatistics.value?.runningWorkloadsCount != null) {
    return workloadStatistics.value.runningWorkloadsCount
  }
  
  if (isOverviewMode.value) {
    // In overview mode, return 0 or placeholder until we know what to show
    return overviewTotal.value || 0
  }
  return totalFromApi.value
})

const avgGpuAllocated = computed(() => {
  // If backend statistics data is available, use it first
  if (workloadStatistics.value?.avgGpuAllocated != null) {
    return workloadStatistics.value.avgGpuAllocated
  }
  
  if (isOverviewMode.value) {
    // In overview mode, calculate from overview data or return 0
    if (overviewData.value.length === 0) return 0
    const sum = overviewData.value.reduce((acc, item) => acc + item.gpuAllocated, 0)
    return sum / overviewData.value.length
  }
  if (allStatsData.value.length === 0) return 0
  const sum = allStatsData.value.reduce((acc, item) => acc + item.allocatedGpuCount, 0)
  return sum / allStatsData.value.length
})

const avgUtilization = computed(() => {
  // If backend statistics data is available, use it first
  if (workloadStatistics.value?.avgGpuUtilization != null) {
    return workloadStatistics.value.avgGpuUtilization
  }
  
  if (isOverviewMode.value) {
    // In overview mode, no utilization data available
    return 0
  }
  if (allStatsData.value.length === 0) return 0
  const sum = allStatsData.value.reduce((acc, item) => acc + item.avgUtilization, 0)
  return sum / allStatsData.value.length
})

const lowUtilizationCount = computed(() => {
  // If backend statistics data is available, use it first
  if (workloadStatistics.value?.lowUtilizationWorkloadsCount != null) {
    return workloadStatistics.value.lowUtilizationWorkloadsCount
  }
  
  if (isOverviewMode.value) {
    // In overview mode, no utilization data available
    return 0
  }
  return allStatsData.value.filter(item => item.avgUtilization < 30).length
})


// Frontend sorting and pagination handling
const sortAndPaginateData = () => {
  if (isOverviewMode.value) {
    // Overview mode uses server-side pagination, just refresh data
    fetchOverviewData()
    return
  }
  
  let filteredData = [...allStatsData.value]
  
  // Apply filters
  if (filters.value.namespace) {
    filteredData = filteredData.filter(item => item.namespace === filters.value.namespace)
  }
  if (filters.value.workloadType) {
    filteredData = filteredData.filter(item => item.workloadType === filters.value.workloadType)
  }
  if (filters.value.workloadName) {
    const searchTerm = filters.value.workloadName.toLowerCase()
    filteredData = filteredData.filter(item => 
      item.workloadName?.toLowerCase().includes(searchTerm)
    )
  }
  
  // Sort
  if (currentSortProp.value) {
    filteredData.sort((a, b) => {
      let valueA: any = a[currentSortProp.value as keyof typeof a]
      let valueB: any = b[currentSortProp.value as keyof typeof b]
      
      // Handle time fields
      if (currentSortProp.value === 'statHour') {
        valueA = new Date(valueA).getTime()
        valueB = new Date(valueB).getTime()
      }
      
      // Handle numeric fields
      if (typeof valueA === 'number' && typeof valueB === 'number') {
        return currentSortOrder.value === 'ascending' ? valueA - valueB : valueB - valueA
      }
      
      // Handle string fields
      if (typeof valueA === 'string' && typeof valueB === 'string') {
        return currentSortOrder.value === 'ascending' 
          ? valueA.localeCompare(valueB) 
          : valueB.localeCompare(valueA)
      }
      
      return 0
    })
  }
  
  // Update total count
  pagination.value.total = filteredData.length
  
  // Pagination
  const start = (pagination.value.page - 1) * pagination.value.pageSize
  const end = start + pagination.value.pageSize
  statsData.value = filteredData.slice(start, end)
}

// Handle table filter change
const handleFilterChange = (filterValues: Record<string, string[]>) => {
  // Reset all filters first
  Object.keys(filters.value).forEach(key => {
    if (key !== 'workloadName') {
      (filters.value as any)[key] = ''
    }
  })
  
  // Apply new filters
  if (filterValues.namespace && filterValues.namespace.length > 0) {
    filters.value.namespace = filterValues.namespace[0]
  }
  if (filterValues.workloadType && filterValues.workloadType.length > 0) {
    filters.value.workloadType = filterValues.workloadType[0]
  }
  if (filterValues.workloadStatus && filterValues.workloadStatus.length > 0) {
    filters.value.workloadStatus = filterValues.workloadStatus[0]
  }
  
  // In overview mode, trigger backend filtering
  if (isOverviewMode.value) {
    pagination.value.page = 1
    fetchOverviewData()
  } else {
    // In historical mode, use frontend filtering
    pagination.value.page = 1
    sortAndPaginateData()
  }
}

// Filter methods for table columns
const filterNamespace = (value: string, row: WorkloadGpuHourlyStats) => {
  return row.namespace === value
}

const filterWorkloadType = (value: string, row: WorkloadGpuHourlyStats) => {
  return row.workloadType === value
}

const filterStatus = (value: string, row: WorkloadGpuHourlyStats) => {
  return row.workloadStatus === value
}

// Handle table sort change
const handleTableSortChange = ({ prop, order }: { prop: string; order: string | null }) => {
  if (order) {
    currentSortProp.value = prop
    currentSortOrder.value = order as 'ascending' | 'descending'
  } else {
    // Cancel sorting, restore default sort (by time descending)
    currentSortProp.value = 'statHour'
    currentSortOrder.value = 'descending'
  }
  
  pagination.value.page = 1
  
  if (isOverviewMode.value) {
    // Overview mode uses backend sorting
    fetchOverviewData()
  } else {
    // Historical mode uses frontend sorting
    sortAndPaginateData()
  }
}

// Handle page change
const handlePageChange = () => {
  if (isOverviewMode.value) {
    // For overview mode, fetch new page from server
    fetchOverviewData()
  } else {
    // For historical mode, use frontend pagination
    sortAndPaginateData()
  }
}

// Fetch overview data (no time range)
const fetchOverviewData = async () => {
  if (!selectedCluster.value) {
    ElMessage.warning('Please select a cluster from the header')
    return
  }

  loading.value = true
  try {
    const params: any = {
      pageNum: pagination.value.page,
      pageSize: pagination.value.pageSize,
      order: currentSortProp.value === 'statHour' ? 'startAt' : currentSortProp.value,
      desc: currentSortOrder.value === 'descending'
    }
    
    // Only add filter params if they have values
    if (filters.value.workloadName) {
      params.name = filters.value.workloadName
    }
    if (filters.value.namespace) {
      params.namespace = filters.value.namespace
    }
    if (filters.value.workloadType) {
      params.kind = filters.value.workloadType
    }
    if (filters.value.workloadStatus) {
      params.status = filters.value.workloadStatus
    }
    
    // Convert order to snake_case for backend
    if (params.order === 'allocatedGpuCount') {
      params.order = 'allocated_gpu_count'
    } else if (params.order === 'startAt') {
      params.order = 'start_at'
    }
    
    const response = await getWorkloadsList(params)
    overviewData.value = response.data
    overviewTotal.value = response.total
    
    // Update pagination total
    pagination.value.total = response.total
    
    // For overview mode, we use the data directly without frontend sorting/pagination
    statsData.value = overviewData.value.map(item => ({
      // Map to WorkloadGpuHourlyStats structure for table compatibility
      id: 0, // Default value for required field
      clusterName: selectedCluster.value || '', // Use current cluster
      namespace: item.namespace,
      workloadName: item.name,
      workloadType: item.kind,
      allocatedGpuCount: item.gpuAllocated,
      requestedGpuCount: item.gpuAllocated, // Use allocated as requested
      workloadStatus: item.status,
      statHour: dayjs(item.startAt * 1000).format('YYYY-MM-DD HH:mm:ss'),
      avgUtilization: 0,
      maxUtilization: 0,
      minUtilization: 0,
      p50Utilization: 0,
      p95Utilization: 0,
      avgGpuMemoryUsed: 0,
      maxGpuMemoryUsed: 0,
      avgGpuMemoryTotal: 0,
      avgReplicaCount: 0,
      maxReplicaCount: 0,
      minReplicaCount: 0,
      sampleCount: 0,
      ownerUid: item.uid || '',
      ownerName: item.name,
      labels: {},
      annotations: {},
      createdAt: dayjs(item.startAt * 1000).toISOString(),
      updatedAt: dayjs().toISOString(),
      // Additional fields for overview mode
      uid: item.uid,
      avgGpuUsage: (item as any).avgGpuUtilization ?? (item as any).avgGpuUsage ?? -1,
      instantGpuUtilization: (item as any).instantGpuUtilization ?? (item as any).instant_gpu_utilization ?? null,
      p50GpuUtilization: (item as any).p50GpuUtilization ?? (item as any).p50_gpu_utilization ?? null,
      p90GpuUtilization: (item as any).p90GpuUtilization ?? (item as any).p90_gpu_utilization ?? null,
      p95GpuUtilization: (item as any).p95GpuUtilization ?? (item as any).p95_gpu_utilization ?? null,
      startAt: item.startAt, // Keep original timestamp for utilization chart
      endAt: item.endAt
    } as WorkloadGpuHourlyStats & { uid?: string; avgGpuUsage?: number; instantGpuUtilization?: number; p50GpuUtilization?: number; p90GpuUtilization?: number; p95GpuUtilization?: number; startAt?: number; endAt?: number }))
    
    // ElMessage.success(`Loaded ${response.data.length} workloads`) // Removed success message to avoid blocking the UI
  } catch (error: any) {
    ElMessage.error(error.message || 'Failed to load overview data')
    overviewData.value = []
  } finally {
    loading.value = false
  }
}

// Fetch historical data (with time range)
const fetchHistoricalData = async () => {
  if (!timeRange.value || timeRange.value.length !== 2) {
    ElMessage.warning('Please select time range')
    return
  }

  loading.value = true
  try {
    const params = {
      cluster: selectedCluster.value,
      namespace: filters.value.namespace || undefined,
      workloadName: filters.value.workloadName || undefined,
      workloadType: filters.value.workloadType || undefined,
      startTime: timeRange.value[0],
      endTime: timeRange.value[1],
      page: 1,
    }
    
    const response = await getWorkloadHourlyStats(params)
    allStatsData.value = response.data
    totalFromApi.value = response.total // Store the total from API
    
    // Reset pagination to first page
    pagination.value.page = 1
    
    // Frontend sorting and pagination
    sortAndPaginateData()
    
    // ElMessage.success(`Data loaded successfully (${allStatsData.value.length} records)`) // Removed success message to avoid blocking the UI
  } catch (error: any) {
    console.error('Failed to fetch workload stats:', error)
    ElMessage.error(error || 'Failed to load data')
  } finally {
    loading.value = false
  }
}

// Fetch statistics data from backend
const fetchStatistics = async () => {
  try {
    const stats = await getWorkloadStatistics()
    // Handle null or undefined from backend
    if (stats) {
      workloadStatistics.value = stats
    } else {
      // Use default values or keep existing computed values
      workloadStatistics.value = null
    }
  } catch (error: any) {
    // Print more detailed error info
    const errorMsg = error instanceof Error ? error.message : String(error)
    
    // Check if it's a specific error code
    if (errorMsg.includes('4004') || errorMsg.includes('404') || 
        errorMsg.includes('No data') || errorMsg.includes('not found') ||
        errorMsg.includes('Not Found')) {
      // These are expected "no data" errors, log as info only
    } else {
      // Only log other errors as warnings
      console.warn('Failed to fetch workload statistics:', errorMsg)
    }
    
    // If fetch fails, continue using computed values
    workloadStatistics.value = null
  }
}

// Main fetch data function that decides which API to call
const fetchData = async () => {
  if (isOverviewMode.value) {
    // Only fetch statistics data in overview mode (instant data)
    fetchStatistics()
    await fetchOverviewData()
  } else {
    await fetchHistoricalData()
  }
}

// Reset filters and auto search
const resetFilters = () => {
  filters.value.namespace = ''
  filters.value.workloadName = ''
  filters.value.workloadType = ''
  pagination.value.page = 1
  pagination.value.pageSize = 20
  currentSortProp.value = 'statHour'
  currentSortOrder.value = 'descending'
  allStatsData.value = []
  statsData.value = []
  overviewData.value = []
  pagination.value.total = 0
  // Clear time range to switch to overview mode
  timeRange.value = undefined
  
  // Auto search overview data
  if (selectedCluster.value) {
    fetchData()
  }
}

// Format time
const formatTime = (time: string) => {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

// Get progress bar color
const getProgressColor = (percentage: number) => {
  if (percentage < 30) return '#f56c6c'
  if (percentage < 50) return '#e6a23c'
  if (percentage < 80) return '#67c23a'
  return '#409eff'
}

// Get workload type tag
const getWorkloadTypeTag = (type: string) => {
  const typeMap: Record<string, 'primary' | 'success' | 'warning' | 'info'> = {
    'Job': 'primary',
    'Deployment': 'success',
    'StatefulSet': 'warning',
    'DaemonSet': 'info'
  }
  return typeMap[type] || 'info'
}

// Get status type
const getStatusType = (status: string) => {
  const statusMap: Record<string, 'success' | 'info' | 'danger' | 'warning'> = {
    'Running': 'success',
    'Completed': 'info',
    'Failed': 'danger',
    'Pending': 'warning'
  }
  return statusMap[status] || 'info'
}

// Get CSS class for utilization values
const getUtilizationClass = (utilization: number | null | undefined) => {
  if (utilization == null) return ''
  if (utilization < 30) return 'text-danger'
  if (utilization < 60) return 'text-warning'
  if (utilization < 80) return 'text-success'
  return 'text-primary'
}

// Save current search state to sessionStorage
const saveSearchState = () => {
  sessionStorage.setItem(STATE_STORAGE_KEY, JSON.stringify({
    filters: filters.value,
    timeRange: timeRange.value,
    pagination: {
      page: pagination.value.page,
      pageSize: pagination.value.pageSize
    },
    sortProp: currentSortProp.value,
    sortOrder: currentSortOrder.value,
    selectedCluster: selectedCluster.value,
    // Data cache
    allStatsData: allStatsData.value,
    overviewData: overviewData.value,
    overviewTotal: overviewTotal.value,
    totalFromApi: totalFromApi.value
  }))
}

// Restore search state from sessionStorage
const restoreSearchState = () => {
  const savedState = sessionStorage.getItem(STATE_STORAGE_KEY)
  if (!savedState) return false
  
  try {
    const state = JSON.parse(savedState)
    
    // Assign directly, using ?? or || to provide default values
    filters.value = state.filters || {}
    timeRange.value = state.timeRange ?? timeRange.value
    currentSortProp.value = state.sortProp ?? currentSortProp.value
    currentSortOrder.value = state.sortOrder ?? currentSortOrder.value
    
    // Pagination state
    pagination.value.page = state.pagination?.page || 1
    pagination.value.pageSize = state.pagination?.pageSize || 10
    
    // Data cache
    allStatsData.value = state.allStatsData || []
    overviewData.value = state.overviewData || []
    overviewTotal.value = state.overviewTotal ?? 0
    totalFromApi.value = state.totalFromApi ?? 0
    
    // Apply filters and pagination after restore
    if (allStatsData.value.length > 0 || overviewData.value.length > 0) {
      sortAndPaginateData()
    }
    
    // Clear the used state
    sessionStorage.removeItem(STATE_STORAGE_KEY)
    
    return true
  } catch (error) {
    console.error('Failed to restore search state:', error)
    sessionStorage.removeItem(STATE_STORAGE_KEY)
    return false
  }
}

// Show workload detail - navigate to detail page
const showWorkloadDetail = (workload: any) => {
  // Save current state before navigation
  saveSearchState()
  
  // Navigate to the workload detail page with kind, name, cluster, and status
  const query: any = {
    kind: workload.workloadType,
    name: workload.workloadName,
    cluster: selectedCluster.value
  }
  
  // Directly use status from data if available
  if (workload.workloadStatus || workload.status) {
    query.status = workload.workloadStatus || workload.status
  }
  
  router.push({
    name: 'WorkloadDetail',
    query
  })
}

// Fetch namespace list
const fetchNamespaces = async () => {
  if (!timeRange.value || timeRange.value.length !== 2) {
    return
  }

  namespaceLoading.value = true
  try {
    const params = {
      cluster: selectedCluster.value,
      startTime: timeRange.value[0],
      endTime: timeRange.value[1]
    }
    const data = await getNamespaces(params)
    namespaceOptions.value = data
  } catch (error: any) {
    console.error('Failed to fetch namespaces:', error)
    namespaceOptions.value = []
  } finally {
    namespaceLoading.value = false
  }
}
// Watch time range change to update namespace list
watch(timeRange, () => {
  fetchNamespaces()
})

// Watch for global cluster changes
watch(selectedCluster, (newCluster) => {
  if (newCluster && timeRange.value) {
    fetchNamespaces()
    fetchData()
  }
})

onMounted(() => {
  // Try to restore previous state first
  const stateRestored = restoreSearchState()
  
  if (!stateRestored) {
    // If no state to restore, proceed with normal initialization
    // Delay initial load to prevent blocking page transition
    nextTick(() => {
      resetFilters()
    })
  } else {
    // If state was restored, fetch namespace options if needed
    if (timeRange.value && timeRange.value.length === 2) {
      fetchNamespaces()
    }
  }
})

// Create debounced search function
const debouncedSearch = debounce(() => {
  pagination.value.page = 1
  
  // Decide how to handle search based on current mode
  if (isOverviewMode.value) {
    // Overview mode: call backend API to re-fetch data
    fetchOverviewData()
  } else if (timeRange.value && timeRange.value.length === 2) {
    // Historical mode: if time range exists, re-fetch historical data
    fetchHistoricalData()
  } else {
    // Local filter mode
    sortAndPaginateData()
  }
}, 300) // 300ms debounce delay

// Watch for workloadName changes to trigger filtering with debounce
watch(() => filters.value.workloadName, () => {
  debouncedSearch()
})

// Watch for time range changes
watch(timeRange, (newVal) => {
  if (!newVal || newVal.length !== 2) {
    // Time range cleared, switch to overview mode
    allStatsData.value = []
    statsData.value = []
    totalFromApi.value = 0
    pagination.value.page = 1
    if (selectedCluster.value) {
      fetchOverviewData()
    }
  } else if (selectedCluster.value) {
    // Time range set, fetch historical data
    overviewData.value = []
    overviewTotal.value = 0
    fetchHistoricalData()
  }
})

// Watch for cluster changes
watch(selectedCluster, () => {
  // Reset and fetch data when cluster changes
  if (selectedCluster.value) {
    fetchData()
  }
})

// View utilization chart for workload
const viewUtilizationChart = async (row: any) => {
  utilizationDialog.value.visible = true
  utilizationDialog.value.title = `${row.namespace}/${row.workloadName}`
  utilizationDialog.value.loading = true
  
  try {
    // Use start and end from row data
    // startAt is in Unix timestamp (seconds)
    const startTimestamp = row.startAt
    const endTimestamp = row.endAt || Math.floor(Date.now() / 1000)
    
    const params = {
      kind: row.workloadType || 'Workload',
      name: row.workloadName,
      start: startTimestamp,
      end: endTimestamp
    }
    
    const response = await getWorkloadGpuUtilizationHistory(params)

    const values = response?.series?.[0]?.values || []
    
    // Process data for chart
    const x = values.map((item: any) => dayjs.unix(item.timestamp).format('MM-DD HH:mm'))
    const avg = values.map((item: any) => {
      const v = item.value
      if (v == null) return null
      return Number(v.toFixed(2))
    })
    
    utilizationDialog.value.series = { x, avg }
    nextTick(() => renderChart())
  } catch (error: any) {
    ElMessage.error('Failed to load utilization data')
  } finally {
    utilizationDialog.value.loading = false
  }
}

// Render utilization chart
const renderChart = () => {
  if (!chartRef.value) return
  
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
    window.addEventListener('resize', resizeChart)
  }
  
  const n = utilizationDialog.value.series.avg?.length ?? 0
  // Optimize x-axis label interval: dynamically adjust label count by data points
  let step = 1
  if (n > 12 && n <= 24) {
    step = 2  // Show about 12 labels
  } else if (n > 24 && n <= 48) {
    step = 4  // Show about 6-12 labels
  } else if (n > 48 && n <= 96) {
    step = 8  // Show about 6-12 labels
  } else if (n > 96 && n <= 168) {
    step = 12  // Show about 8-14 labels (hours in a week)
  } else if (n > 168 && n <= 336) {
    step = 24  // Show about 7-14 labels (hours in two weeks)
  } else if (n > 336) {
    step = Math.ceil(n / 10)  // Show about 10 labels
  }
  const isDark = document.documentElement.classList.contains('dark')
  
  // Theme colors
  const colorText = isDark ? '#E5EAF3' : '#303133'
  const colorSubtext = isDark ? '#C8CDD5' : '#606266'
  const colorAxis = isDark ? '#FFFFFF33' : '#00000026'
  const colorGrid = isDark ? '#FFFFFF1F' : '#00000012'
  const tooltipBg = isDark ? 'rgba(17,24,39,0.95)' : '#fff'
  const tooltipBorder = isDark ? 'rgba(255,255,255,0.18)' : '#ebeef5'
  const pointerColor = isDark ? '#FFFFFF3D' : '#0000003d'
  
  const option = {
    animation: false,
    grid: { left: 60, right: 40, top: 60, bottom: 80, containLabel: true },
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      textStyle: { color: colorText },
      valueFormatter: (v: any) => (v == null ? '-' : `${(+v).toFixed(1)}%`),
      axisPointer: {
        type: 'line',
        lineStyle: { color: pointerColor, width: 1 }
      },
    },
    xAxis: {
      type: 'category',
      data: utilizationDialog.value.series.x,
      axisTick: { show: false },
      axisLabel: {
        color: colorSubtext,
        interval: (idx: number) => (step === 1 ? 0 : idx % step === 0),
        rotate: 45, // Rotate 45 degrees to prevent overlap
        align: 'right',
      },
      axisLine: { lineStyle: { color: colorAxis } },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      axisLabel: { formatter: '{value}%', color: colorSubtext },
      axisLine: { lineStyle: { color: colorAxis } },
      splitLine: { lineStyle: { color: colorGrid } },
    },
    series: [
      {
        name: 'Avg Utilization',
        type: 'line',
        symbol: 'circle',
        symbolSize: step === 1 ? 4 : 3,
        showAllSymbol: step === 1,
        data: utilizationDialog.value.series.avg,
        lineStyle: { width: 1.8, color: '#409eff' },
        areaStyle: { opacity: isDark ? 0.1 : 0.08, color: '#409eff' },
        label: {
          show: true,
          position: 'top',
          color: colorText,
          borderRadius: 4,
          padding: [2, 4],
          formatter: (p: any) => {
            const val = p.value
            if (val == null) return ''
            if (step !== 1 && p.dataIndex % step !== 0) return ''
            return `${(+val).toFixed(0)}%`
          },
        },
        emphasis: { focus: 'series' },
      },
    ],
  }
  
  chartInstance.setOption(option)
}

const resizeChart = () => chartInstance?.resize()

// Watch dialog visibility
watch(() => utilizationDialog.value.visible, (visible) => {
  if (visible) {
    nextTick(() => renderChart())
  } else {
    // Dispose chart when dialog closes
    if (chartInstance) {
      window.removeEventListener('resize', resizeChart)
      chartInstance.dispose()
      chartInstance = null
    }
  }
})

// Watch series data
watch(() => utilizationDialog.value.series, () => {
  if (utilizationDialog.value.visible) {
    renderChart()
  }
}, { deep: true })

// Save state when navigating away from this route
onBeforeRouteLeave(() => {
  // Only save if we have meaningful search state
  if ((allStatsData.value.length > 0 || overviewData.value.length > 0) && 
      (filters.value.workloadName || timeRange.value || pagination.value.page > 1)) {
    saveSearchState()
  }
})

onBeforeUnmount(() => {
  if (chartInstance) {
    window.removeEventListener('resize', resizeChart)
    chartInstance.dispose()
    chartInstance = null
  }
})
</script>

<style scoped lang="scss">
.workload-stats {
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;
  
  .filter-section {
    margin-bottom: 20px;
    
    .filter-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      flex-wrap: wrap;
      gap: 16px;
      width: 100%;
      box-sizing: border-box;
      
      @media (max-width: 768px) {
        gap: 12px;
        
        :deep(.el-form) {
          width: 100%;
          
          .el-form-item {
            margin-bottom: 12px;
            width: 100%;
            
            .el-input, .el-select {
              width: 100%;
            }
          }
        }
      }
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 12px 0;
      gap: 20px;
    }
    
    .page-title {
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin: 0;
      flex-shrink: 0;
      
      @media (min-width: 1920px) {
        font-size: 22px;
      }
    }
    
    .card-header {
      font-weight: 600;
      font-size: 18px;
    }
    
    .filters {
      display: flex;
      align-items: center;
      gap: 12px;
      
      :deep(.el-form-item) {
        align-items: center;
        margin-bottom: 0;
      }
      
      :deep(.el-form-item__label) {
        font-size: 14px;
        line-height: 32px;
        display: flex;
        align-items: center;
        height: 32px;
        
        @media (min-width: 1920px) {
          font-size: 16px;
        }
      }
      
      :deep(.el-form-item__content) {
        line-height: 32px;
        display: flex;
        align-items: center;
      }
    }
    
    .time-picker {
      width: 100%;
      max-width: 400px;
      
      @media (max-width: 768px) {
        max-width: 100%;
      }
    }
  }
  
  // All card containers should have proper box-sizing
  .filter-card, .stats-cards, .chart-card, .table-card {
    width: 100%;
    max-width: 100%;
    box-sizing: border-box;
  }
  
  // Ensure el-card doesn't overflow
  :deep(.el-card) {
    width: 100%;
    max-width: 100%;
    box-sizing: border-box;
  }
  
  .stats-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 16px;
    margin-bottom: 20px;
    
    @media (max-width: 1024px) {
      grid-template-columns: repeat(2, 1fr);
      gap: 12px;
    }
    
    @media (max-width: 600px) {
      grid-template-columns: 1fr;
      gap: 10px;
    }
    
    .stat-card {
      border-radius: 15px;
      transition: all 0.3s ease;
      position: relative;
      overflow: hidden;
      
      &::before {
        content: '';
        position: absolute;
        top: 0;
        left: 0;
        right: 0;
        height: 2px;
        background: linear-gradient(
          90deg,
          transparent 0%,
          rgba(64, 158, 255, 0.5) 20%,
          rgba(103, 194, 58, 0.5) 50%,
          rgba(245, 108, 108, 0.5) 80%,
          transparent 100%
        );
        opacity: 0.6;
      }
      
      &:hover {
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
        border-color: var(--el-border-color);
      }
      
      :deep(.el-card__body) {
        padding: 20px;
      }
      
      .stat-content {
        display: flex;
        align-items: center;
        gap: 16px;
        
        @media (max-width: 768px) {
          gap: 12px;
        }
        
        .stat-icon {
          width: 48px;
          height: 48px;
          border-radius: 12px;
          display: flex;
          align-items: center;
          justify-content: center;
          font-size: 24px;
          flex-shrink: 0;
          transition: all 0.3s ease;
          
          i {
            font-size: inherit;
          }
          
          &--primary {
            background: linear-gradient(135deg, rgba(64, 158, 255, 0.15), rgba(64, 158, 255, 0.05));
            color: #409eff;
          }
          
          &--success {
            background: linear-gradient(135deg, rgba(103, 194, 58, 0.15), rgba(103, 194, 58, 0.05));
            color: #67c23a;
          }
          
          &--info {
            background: linear-gradient(135deg, rgba(0, 177, 166, 0.15), rgba(0, 177, 166, 0.05));
            color: #00b1a6;
          }
          
          &--warning {
            background: linear-gradient(135deg, rgba(230, 162, 60, 0.15), rgba(230, 162, 60, 0.05));
            color: #e6a23c;
          }
          
          &--danger {
            background: linear-gradient(135deg, rgba(245, 108, 108, 0.15), rgba(245, 108, 108, 0.05));
            color: #f56c6c;
          }
          
          @media (max-width: 768px) {
            width: 40px;
            height: 40px;
            font-size: 20px;
          }
        }
        
        .stat-info {
          flex: 1;
          
          .stat-label {
            font-size: clamp(12px, 1vw + 10px, 14px);
            color: var(--el-text-color-secondary);
            margin-bottom: 4px;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            display: flex;
            align-items: center;
            gap: 4px;
            
            .stat-help-icon {
              font-size: 12px;
              color: var(--el-text-color-secondary);
              cursor: help;
              transition: all 0.3s ease;
              flex-shrink: 0;
              
              &:hover {
                color: var(--el-color-primary);
                transform: scale(1.1);
              }
            }
          }
          
          .stat-value {
            font-size: clamp(18px, 2vw + 14px, 24px);
            font-weight: 600;
            color: var(--el-text-color-primary);
            white-space: nowrap;
            transition: color 0.3s ease;
            
            &--primary {
              color: #409eff;
            }
            
            &--success {
              color: #67c23a;
            }
            
            &--info {
              color: #00b1a6;
            }
            
            &--warning {
              color: #e6a23c;
            }
            
            &--danger {
              color: #f56c6c;
            }
          }
        }
      }
      
      &:hover {
        .stat-icon {
          transform: scale(1.1) rotate(5deg);
        }
        
        .stat-value {
          &--primary {
            color: #66b1ff;
          }
          
          &--success {
            color: #85ce61;
          }
          
          &--info {
            color: #00c9bf;
          }
          
          &--warning {
            color: #ebb563;
          }
          
          &--danger {
            color: #fa8c8c;
          }
        }
      }
    }
  }
  
  .table-card {
    border-radius: 15px;
    overflow: hidden; // Ensure proper scroll container
    
    :deep(.el-card__body) {
      padding: 0;
    }
    
    // Wrapper for horizontal scroll
    .table-wrapper {
      overflow-x: auto;
      padding: 20px;
      
      @media (max-width: 768px) {
        padding: 12px;
      }
      
      :deep(.el-table) {
        min-width: 800px; // Ensure minimum width for readability
        
        @media (max-width: 768px) {
          font-size: 12px;
          
          th, td {
            padding: 8px 0;
          }
        }
      }
    }
    
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-weight: 600;
      font-size: 18px;
      
      .header-actions {
        display: flex;
        align-items: center;
      }
    }
    
    // Table overall font size
    :deep(.el-table) {
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
      
      // Table row height
      td {
        padding: 14px 0;
        
        @media (min-width: 1920px) {
          padding: 16px 0;
        }
      }
      
      th {
        font-size: 14px;
        font-weight: 600;
        padding: 14px 0;
        
        @media (min-width: 1920px) {
          font-size: 15px;
          padding: 16px 0;
        }
      }
      
      // Table cell padding
      .cell {
        padding-left: 12px;
        padding-right: 12px;
      }
    }
    
    .workload-link {
      font-weight: 500;
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
      
      &:hover {
        font-weight: 600;
      }
    }
    
    .warning-text {
      color: var(--el-color-warning);
      font-weight: 600;
    }
    
    // Utilization value color classes
    .text-danger {
      color: var(--el-color-danger);
      font-weight: 500;
    }
    
    .text-warning {
      color: var(--el-color-warning);
      font-weight: 500;
    }
    
    .text-success {
      color: var(--el-color-success);
      font-weight: 500;
    }
    
    .text-primary {
      color: var(--el-color-primary);
      font-weight: 500;
    }
    
    .requested-gpu {
      color: var(--el-text-color-secondary);
      font-size: 13px;
      
      @media (min-width: 1920px) {
        font-size: 14px;
      }
    }
    
    .replica-range {
      color: var(--el-text-color-secondary);
      font-size: 13px;
      
      @media (min-width: 1920px) {
        font-size: 14px;
      }
    }
    
    .utilization-cell {
      padding: 4px 0;
      
      :deep(.el-progress) {
        .el-progress__text {
          font-size: 13px !important;
          
          @media (min-width: 1920px) {
            font-size: 14px !important;
          }
        }
      }
    }
  }
}

.utilization-dialog {
  :deep(.el-dialog__header) {
    padding: 16px 20px 8px;
  }
  
  :deep(.el-dialog__body) {
    padding: 16px 24px 24px;
  }
  
  :deep(.el-dialog__headerbtn .el-dialog__close) {
    transform: scale(0.9);
  }
}

.utilization-chart {
  width: 100%;
  height: 500px;
}

// Table header help icon styles
.table-header-with-tip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  
  .header-help-icon {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    cursor: help;
    transition: all 0.3s ease;
    
    &:hover {
      color: var(--el-color-primary);
      transform: scale(1.1);
    }
  }
}
</style>

