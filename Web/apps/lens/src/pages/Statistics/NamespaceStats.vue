<template>
  <div class="namespace-stats">
    <div class="filter-section">
      <div class="filter-header">
        <h2 class="page-title">Namespace GPU Statistics</h2>
        <div class="filters">
          <el-form :inline="true" :model="filters">
          <el-form-item>
            <el-select 
              v-model="filters.namespace" 
              placeholder="Namespaces"
              clearable
              size="default"
              style="width: 180px"
              :loading="namespaceLoading"
              @change="handleNamespaceChange"
              @clear="handleNamespaceChange"
            >
              <el-option
                v-for="namespace in namespaceOptions"
                :key="namespace"
                :label="namespace"
                :value="namespace"
              />
            </el-select>
          </el-form-item>
          
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
              style="width: 400px"
              popper-class="custom-date-picker"
            />
          </el-form-item>
          
          </el-form>
        </div>
      </div>
    </div>

    <!-- Namespace Cards -->
    <div class="namespace-cards">
      <!-- Loading skeleton for cards -->
      <template v-if="loading && !filters.namespace">
        <el-card v-for="i in 4" :key="`skeleton-${i}`" class="namespace-card">
          <el-skeleton :rows="2" animated />
        </el-card>
      </template>
      
      <!-- Empty state -->
      <template v-else-if="!loading && namespaceStatsList.length === 0">
        <el-empty 
          description="No namespace data available"
          :image-size="200"
          class="namespace-empty"
        >
          <template #description>
            <div class="empty-description">
              <p>No namespace statistics found for the selected cluster</p>
              <p class="empty-hint">Try selecting a different time range or cluster</p>
            </div>
          </template>
        </el-empty>
      </template>
      
      <!-- Actual cards -->
      <template v-else>
        <el-card
          v-for="item in namespaceStatsList" 
          :key="item.namespace"
          class="namespace-card"
          :class="{ 'selected': selectedNamespace === item.namespace }"
          @click="selectNamespace(item.namespace)"
        >
          <div class="namespace-content">
            <div class="namespace-header">
              <div class="namespace-name">{{ item.namespace }}</div>
              <el-tag v-if="selectedNamespace === item.namespace" type="primary" size="small">
                Selected
              </el-tag>
            </div>
            <div class="namespace-stats">
              <div class="stat-item">
                <div class="stat-label">
                  <i i="ep-cpu" class="mr-1" />
                  GPUs Used
                  <el-tooltip 
                    content="Number of GPU resources allocated to this namespace" 
                    placement="top"
                  >
                    <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
                <div class="stat-value">{{ item.allocatedGpuCount.toFixed(2) }}</div>
              </div>
              <div class="stat-item">
                <div class="stat-label">
                  <i i="ep-odometer" class="mr-1" />
                  Avg Utilization
                  <el-tooltip 
                    content="Average GPU utilization percentage for this namespace" 
                    placement="top"
                  >
                    <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
                <div class="stat-value">{{ item.avgUtilization.toFixed(2) }}%</div>
              </div>
            </div>
            <div class="utilization-bar">
              <el-progress 
                :percentage="item.avgUtilization" 
                :color="getProgressColor(item.avgUtilization)"
                :stroke-width="8"
                :show-text="false"
              />
            </div>
          </div>
        </el-card>
      </template>
    </div>
    
    <!-- Loading State for table view -->
    <div v-if="loading && filters.namespace" class="loading-container">
      <el-card>
        <div style="text-align: center; padding: 40px;">
          <i i="ep-loading" class="is-loading" style="font-size: 36px; color: var(--el-color-primary);"></i>
          <p style="margin-top: 16px; color: var(--el-text-color-secondary);">Loading data...</p>
        </div>
      </el-card>
    </div>
    
    <!-- Workspace Pie Charts Loading -->
    <div v-if="!filters.namespace && pieLoading" class="loading-container">
      <el-card>
        <div style="text-align: center; padding: 40px;">
          <i i="ep-loading" class="is-loading" style="font-size: 36px; color: var(--el-color-primary);"></i>
          <p style="margin-top: 16px; color: var(--el-text-color-secondary);">Loading workspace data...</p>
        </div>
      </el-card>
    </div>
    
    <!-- Workspace Pie Charts (show when no namespace filter is applied) -->
    <div class="stat-grid" v-if="!filters.namespace && !pieLoading && workspaceData.length > 0">
      <el-card
        v-for="(workspace, i) in workspaceData"
        :key="workspace.workspaceId"
        shadow="never"
        class="stat-card stat-card--tall"
        :style="{
          '--accent': PIE_COLORS[0],
          '--accent-bad': PIE_COLORS[1],
          '--accent-used': PIE_COLORS[2],
        }"
      >
        <div class="stat-header">
          <div class="stat-title">
            {{ workspace.workspaceName }}
            <el-tooltip 
              content="Workspace GPU resource allocation and usage statistics" 
              placement="top"
            >
              <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="stat-total">
            <span class="stat-total__num">{{ workspace.currentNodeCount || 0 }}</span>
            <el-tooltip 
              content="Total number of GPU nodes in this workspace" 
              placement="top"
            >
              <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
        </div>
        <div class="stat-bottom">
          <div class="stat-badges">
            <span class="badge badge--bling badge--ok">
              <i class="dot" :style="{ background: PIE_COLORS[0] }"></i>
              <span class="badge-text">Available {{ Math.max((workspace.currentNodeCount || 0) - (workspace.usedNodeCount || 0) - (workspace.abnormalNodeCount || 0), 0) }}</span>
            </span>
            <span class="badge badge--bling badge--bad">
              <i class="dot" :style="{ background: PIE_COLORS[1] }"></i>
              <span class="badge-text">Abnormal {{ workspace.abnormalNodeCount || 0 }}</span>
            </span>
            <span class="badge badge--bling badge--used">
              <i class="dot" :style="{ background: PIE_COLORS[2] }"></i>
              <span class="badge-text">Used {{ workspace.usedNodeCount || 0 }}</span>
            </span>
          </div>
          <div class="small-pie-box" :ref="(el: any) => setPieChartRef(el, workspace.workspaceId)" />
        </div>
      </el-card>
    </div>
    
    <!-- GPU Trend Charts (show when no namespace filter is applied) -->
    <div class="gpu-charts-grid" v-if="!filters.namespace && workspaceData.length > 0">
      <el-card 
        v-for="workspace in workspaceData" 
        :key="workspace.workspaceId"
        shadow="never" 
        class="gpu-chart-card" 
        v-loading="gpuLoadingMap[workspace.workspaceId]"
      >
        <template #header>
          <div class="card-header">
            <span>{{ workspace.workspaceName }} - GPU Trends</span>
          </div>
        </template>
        <div class="gpu-chart-box" :ref="(el: any) => setGpuChartRef(el, workspace.workspaceId)" />
      </el-card>
    </div>

    <!-- Selected Namespace Trend Chart -->
    <el-card class="chart-card" v-if="selectedNamespace && selectedNamespaceData.length > 0">
      <template #header>
        <div class="card-header">
          <span>{{ selectedNamespace }} - Utilization & Allocation Trends</span>
          <el-button size="small" @click="selectedNamespace = ''">
            <i i="ep-close" class="mr-1" />
            Close
          </el-button>
        </div>
      </template>
      
      <div ref="trendChartRef" style="height: 400px;" />
    </el-card>

    <!-- Data Table (show only when namespace is filtered) -->
    <el-card class="table-card" v-if="!loading && filters.namespace">
      <el-table 
        v-loading="loading"
        :data="statsData" 
        stripe 
        style="width: 100%"
        @sort-change="handleTableSortChange"
      >
        <el-table-column prop="namespace" label="Namespace" width="200" fixed />
        
        <el-table-column prop="statHour" label="Stat Time" min-width="200" sortable="custom">
          <template #default="{ row }">
            {{ formatTime(row.statHour) }}
          </template>
        </el-table-column>
        
        <el-table-column prop="allocatedGpuCount" label="Allocated GPU" min-width="160" sortable="custom">
          <template #default="{ row }">
            {{ row.allocatedGpuCount?.toFixed(2) ?? '0.00' }}
          </template>
        </el-table-column>
        
        <el-table-column prop="avgUtilization" label="Avg Utilization" min-width="220" sortable="custom">
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
        
        <el-table-column prop="maxUtilization" label="Max Utilization" min-width="160">
          <template #default="{ row }">
            {{ row.maxUtilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>
        
        <el-table-column prop="minUtilization" label="Min Utilization" min-width="160">
          <template #default="{ row }">
            {{ row.minUtilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>
        
        <el-table-column prop="activeWorkloadCount" label="Active Workloads" min-width="160" />
        
        <template #empty>
          <el-empty description="No Data" />
        </template>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { QuestionFilled } from '@element-plus/icons-vue'
import { getNamespaceHourlyStats, getNamespaces, NamespaceGpuHourlyStats } from '@/services/gpu-aggregation'
import { useClusterSync } from '@/composables/useClusterSync'
import { getWorkspaceDetail, type WorkspaceDetail } from '@/services/safe-api'
import * as echarts from 'echarts'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'

dayjs.extend(utc)

// Get global cluster
const { selectedCluster } = useClusterSync()

// Constants for pie chart colors
const PIE_COLORS = ['#67C23A', '#F56C6C', '#00e5e5'] as const

const loading = ref(false)
const pieLoading = ref(false)
const namespaceLoading = ref(false)
const statsData = ref<NamespaceGpuHourlyStats[]>([])
const timeRange = ref<[string, string]>()
const namespaceOptions = ref<string[]>([])
const selectedNamespace = ref('')
const trendChartRef = ref<HTMLElement | null>(null)
let trendChartInstance: echarts.ECharts | null = null
const filters = ref({
  namespace: ''
})

// Workspace data for pie charts
const workspaceData = ref<WorkspaceDetail[]>([])
const pieChartInstances = new Map<string, echarts.ECharts>()
const pieChartRefs = new Map<string, HTMLElement>()

// GPU trend chart data
const gpuChartRefs = new Map<string, HTMLElement>()
const gpuChartInstances = new Map<string, echarts.ECharts>()
const gpuDataMap = ref<Map<string, NamespaceGpuHourlyStats[]>>(new Map())
const gpuLoadingMap = ref<Record<string, boolean>>({})

// Sorting
const currentSortProp = ref<string>('statHour')
const currentSortOrder = ref<'ascending' | 'descending'>('descending')

// Calculate namespace stats list (latest stats for each namespace)
const namespaceStatsList = computed(() => {
  const map = new Map<string, NamespaceGpuHourlyStats>()
  statsData.value.forEach(item => {
    const existing = map.get(item.namespace)
    if (!existing || new Date(item.statHour) > new Date(existing.statHour)) {
      map.set(item.namespace, item)
    }
  })
  return Array.from(map.values())
    .filter(item => item.namespace !== 'default') // Filter out default namespace
})

// Get data for selected namespace
const selectedNamespaceData = computed(() => {
  if (!selectedNamespace.value) return []
  return statsData.value
    .filter(item => item.namespace === selectedNamespace.value)
    .sort((a, b) => new Date(a.statHour).getTime() - new Date(b.statHour).getTime())
})

// Chart labels for selected namespace
const selectedChartLabels = computed(() => {
  return selectedNamespaceData.value.map(item => formatTime(item.statHour))
})

// Render trend chart
const renderTrendChart = () => {
  if (!trendChartInstance || selectedNamespaceData.value.length === 0) return

  // Optimize x-axis label interval: dynamically adjust label count by data points
  const n = selectedChartLabels.value.length
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
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const borderColor = isDark ? '#FFFFFF1A' : '#00000012'

  const option = {
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
        crossStyle: {
          color: textColor
        }
      }
    },
    legend: {
      data: ['Avg Utilization', 'GPUs Used'],
      textStyle: { color: textColor },
      top: 10
    },
    grid: {
      left: 60,
      right: 60,
      bottom: 40,
      top: 60
    },
    xAxis: {
      type: 'category',
      data: selectedChartLabels.value,
      axisPointer: {
        type: 'shadow'
      },
      axisLabel: {
        interval: (idx: number) => (step === 1 ? true : idx % step === 0),
        formatter: (val: string) => val.replace(' ', '\n'),
        color: textColor,
        rotate: step > 4 ? 45 : 0,
        align: step > 4 ? 'right' : 'center'
      },
      axisLine: { lineStyle: { color: borderColor } }
    },
    yAxis: [
      {
        type: 'value',
        name: 'Utilization (%)',
        min: 0,
        max: 100,
        position: 'left',
        axisLabel: {
          formatter: '{value}%',
          color: textColor
        },
        axisLine: {
          show: true,
          lineStyle: { color: '#409eff' }
        },
        splitLine: { lineStyle: { color: borderColor } }
      },
      {
        type: 'value',
        name: 'GPUs Used',
        position: 'right',
        axisLabel: {
          formatter: '{value}',
          color: textColor
        },
        axisLine: {
          show: true,
          lineStyle: { color: '#67c23a' }
        },
        splitLine: { show: false }
      }
    ],
    series: [
      {
        name: 'Avg Utilization',
        type: 'line',
        yAxisIndex: 0,
        smooth: true,
        data: selectedNamespaceData.value.map(item => Number(item.avgUtilization.toFixed(2))),
        itemStyle: { color: '#409eff' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(64, 158, 255, 0.3)' },
            { offset: 1, color: 'rgba(64, 158, 255, 0.05)' }
          ])
        }
      },
      {
        name: 'GPUs Used',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: selectedNamespaceData.value.map(item => Number(item.allocatedGpuCount.toFixed(2))),
        itemStyle: { color: '#67c23a' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(103, 194, 58, 0.3)' },
            { offset: 1, color: 'rgba(103, 194, 58, 0.05)' }
          ])
        }
      }
    ]
  }

  trendChartInstance.setOption(option)
}

// Initialize trend chart
const initTrendChart = () => {
  nextTick(() => {
    if (trendChartRef.value && !trendChartInstance) {
      trendChartInstance = echarts.init(trendChartRef.value)
      renderTrendChart()
    }
  })
}

// Dispose trend chart
const disposeTrendChart = () => {
  if (trendChartInstance) {
    trendChartInstance.dispose()
    trendChartInstance = null
  }
}

// Select namespace
const selectNamespace = (namespace: string) => {
  if (selectedNamespace.value === namespace) {
    selectedNamespace.value = ''
    disposeTrendChart()
  } else {
    selectedNamespace.value = namespace
    disposeTrendChart()
    initTrendChart()
  }
}

// Fetch data
// Handle table sort change
const handleTableSortChange = ({ prop, order }: { prop: string; order: string | null }) => {
  if (order) {
    currentSortProp.value = prop
    currentSortOrder.value = order as 'ascending' | 'descending'
  } else {
    currentSortProp.value = 'statHour'
    currentSortOrder.value = 'descending'
  }
  fetchData()
}

// Handle namespace change - auto fetch data
const handleNamespaceChange = () => {
  // Don't change view immediately, wait for data
  fetchData()
}

const fetchData = async () => {
  if (!selectedCluster.value) {
    ElMessage.warning('Please select a cluster from the header')
    return
  }
  
  if (!timeRange.value || timeRange.value.length !== 2) {
    ElMessage.warning('Please select time range')
    return
  }

  loading.value = true
  try {
    // Convert sort prop to API format
    let orderBy: 'time' | 'utilization' = 'time'
    if (currentSortProp.value === 'avgUtilization') {
      orderBy = 'utilization'
    }
    
    const params = {
      cluster: selectedCluster.value,
      namespace: filters.value.namespace || undefined,
      startTime: timeRange.value[0],
      endTime: timeRange.value[1],
      order_by: orderBy,
      order_direction: (currentSortOrder.value === 'ascending' ? 'asc' : 'desc') as 'asc' | 'desc'
    }
    
    const response = await getNamespaceHourlyStats(params)
    statsData.value = response.data
    
    // Clear selected namespace when filter changes
    selectedNamespace.value = ''
    
    // Only show success message when namespace is filtered (table view)
    if (filters.value.namespace) {
      // ElMessage.success(`Loaded ${response.data.length} records successfully`) // Removed success message to avoid blocking the UI
      // Clear workspace data when namespace is filtered
      workspaceData.value = []
    } else {
      // Fetch workspace data for pie charts if no namespace filter
      // Don't await here to make it parallel
      workspaceData.value = []
      fetchWorkspaceData()
      // Also fetch GPU trend data
      fetchGPUData()
    }
  } catch (error: any) {
    console.error('Failed to fetch namespace stats:', error)
    ElMessage.error(error || 'Failed to load data')
  } finally {
    loading.value = false
  }
}

// Reset filters and auto search
const resetFilters = () => {
  filters.value.namespace = ''
  selectedNamespace.value = ''
  currentSortProp.value = 'statHour'
  currentSortOrder.value = 'descending'
  // Default to last 24 hours
  const endTime = dayjs().utc()
  const startTime = endTime.subtract(24, 'hour')
  timeRange.value = [
    startTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z',
    endTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z'
  ]
  
  // Auto search with default time range
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
  if (percentage < 50) return '#67c23a'
  if (percentage < 80) return '#e6a23c'
  return '#f56c6c'
}

// Get workspace status tag type
const getWorkspaceTagType = (phase: string) => {
  const phaseMap: Record<string, any> = {
    'Running': 'success',
    'Pending': 'warning',
    'Failed': 'danger',
    'Unknown': 'info'
  }
  return phaseMap[phase] || 'info'
}

// Set pie chart ref and render chart
const setPieChartRef = (el: HTMLElement | null, workspaceId: string) => {
  if (el) {
    pieChartRefs.set(workspaceId, el)
    // Find the workspace data and render chart immediately
    nextTick(() => {
      const workspace = workspaceData.value.find(w => w.workspaceId === workspaceId)
      if (workspace) {
        renderPieChart(workspace)
      }
    })
  }
}

// Format percentage for pie chart labels
const fmtPct = (x: number | undefined) =>
  `${Number.isFinite(x as number) ? (x as number).toFixed(1) : '0.0'}%`

// Render pie chart for workspace
const renderPieChart = (workspace: WorkspaceDetail) => {
  const chartDom = pieChartRefs.get(workspace.workspaceId)
  if (!chartDom) return
  
  // Initialize or get existing chart instance
  let chart = pieChartInstances.get(workspace.workspaceId)
  if (!chart) {
    chart = echarts.init(chartDom)
    pieChartInstances.set(workspace.workspaceId, chart)
  }
  
  // Calculate node numbers based on the provided pattern
  const total = Number(workspace.currentNodeCount ?? 0)
  const used = Number(workspace.usedNodeCount ?? 0)
  const abnormal = Number(workspace.abnormalNodeCount ?? 0)
  const avail = Math.max(total - used - abnormal, 0)
  
  const option = {
    color: [...PIE_COLORS],
    tooltip: {
      trigger: 'item',
      appendToBody: true,
      formatter: (p: any) => `Nodes<br/>${p.name}: ${p.value} (${p.percent}%)`,
    },
    series: [
      {
        type: 'pie',
        radius: ['45%', '90%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: true,
        minShowLabelAngle: 5,
        stillShowZeroSum: true,
        label: {
          position: 'inside',
          formatter: (p: any) => (p.value > 0 ? fmtPct(p.percent) : ''),
          fontSize: 16,
          fontWeight: 700,
          color: '#fff',
          textBorderColor: 'rgba(0,0,0,.35)',
          textBorderWidth: 2,
          textShadowColor: 'rgba(0,0,0,.25)',
          textShadowBlur: 2,
        },
        labelLine: { show: false },
        data: [
          { name: 'Available', value: avail },
          { name: 'Abnormal', value: abnormal },
          { name: 'Used', value: used }
        ],
      },
    ],
  }
  
  chart.setOption(option)
}

// Set GPU chart ref
const setGpuChartRef = (el: HTMLElement | null, workspaceId: string) => {
  if (el) {
    gpuChartRefs.set(workspaceId, el)
    // Render chart if data already exists
    const data = gpuDataMap.value.get(workspaceId)
    if (data && data.length > 0) {
      nextTick(() => renderGPUChart(workspaceId))
    }
  } else {
    gpuChartRefs.delete(workspaceId)
    const chart = gpuChartInstances.get(workspaceId)
    if (chart) {
      chart.dispose()
      gpuChartInstances.delete(workspaceId)
    }
  }
}

// Fetch GPU trend data for all workspaces
const fetchGPUData = async () => {
  if (!selectedCluster.value || filters.value.namespace || workspaceData.value.length === 0) return
  
  // Use the same time range as main data
  if (!timeRange.value || timeRange.value.length !== 2) return
  
  // Clear existing data
  gpuDataMap.value.clear()
  
  // Fetch data for each workspace
  const promises = workspaceData.value.map(async (workspace) => {
    const workspaceId = workspace.workspaceId
    gpuLoadingMap.value[workspaceId] = true
    
    try {
      const params = {
        cluster: selectedCluster.value,
        namespace: workspaceId, // Use workspace ID as namespace
        startTime: timeRange.value![0],
        endTime: timeRange.value![1],
        page: 1,
        pageSize: 1000,
        orderBy: 'time',
        orderDirection: 'asc' as const
      }
      
      const response = await getNamespaceHourlyStats(params)
      gpuDataMap.value.set(workspaceId, response.data || [])
      nextTick(() => renderGPUChart(workspaceId))
    } catch (error: any) {
      console.error(`Failed to fetch GPU data for ${workspaceId}:`, error)
      gpuDataMap.value.set(workspaceId, [])
    } finally {
      gpuLoadingMap.value[workspaceId] = false
    }
  })
  
  await Promise.all(promises)
}

// Render GPU trend chart for a specific workspace
const renderGPUChart = async (workspaceId: string) => {
  await nextTick()
  const chartDom = gpuChartRefs.get(workspaceId)
  if (!chartDom) return
  
  let chart = gpuChartInstances.get(workspaceId)
  if (!chart) {
    chart = echarts.init(chartDom)
    gpuChartInstances.set(workspaceId, chart)
  }
  
  const gpuData = gpuDataMap.value.get(workspaceId) || []
  
  if (gpuData.length === 0) {
    const emptyOption: echarts.EChartsOption = {
      title: {
        show: true,
        text: 'No Data',
        left: 'center',
        top: 'center',
        textStyle: {
          color: '#999',
          fontSize: 18,
        },
      },
    }
    chart.setOption(emptyOption, true)
    return
  }
  
  // Sort data by time
  let sortedData = [...gpuData].sort((a, b) => 
    new Date(a.statHour).getTime() - new Date(b.statHour).getTime()
  )
  
  // Sample data if too many points (keep max 50 points for better visualization)
  if (sortedData.length > 50) {
    const step = Math.ceil(sortedData.length / 50)
    sortedData = sortedData.filter((_, index) => index % step === 0)
  }
  
  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const borderColor = isDark ? '#FFFFFF1A' : '#00000012'
  
  const option: echarts.EChartsOption = {
    animation: true,
    grid: {
      top: 40,
      left: 50,
      right: 50,
      bottom: 40,
      containLabel: true
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
        label: {
          backgroundColor: isDark ? '#6a7985' : '#6a7985'
        }
      }
    },
    legend: {
      data: ['Allocated GPU', 'Avg Utilization'],
      top: 10,
      textStyle: { color: textColor }
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: sortedData.map(item => dayjs(item.statHour).format('MM-DD HH:mm')),
      axisLine: { lineStyle: { color: borderColor } },
      axisLabel: { 
        color: textColor,
        interval: Math.ceil(sortedData.length / 12), // Show about 12 labels
        rotate: 0 // No rotation
      }
    },
    yAxis: [
      {
        type: 'value',
        name: 'GPU Count',
        position: 'left',
        axisLabel: {
          formatter: '{value}',
          color: textColor
        },
        axisLine: {
          show: true,
          lineStyle: { color: '#67c23a' }
        },
        splitLine: { lineStyle: { color: borderColor } }
      },
      {
        type: 'value',
        name: 'Utilization (%)',
        position: 'right',
        min: 0,
        max: 100,
        axisLabel: {
          formatter: '{value}%',
          color: textColor
        },
        axisLine: {
          show: true,
          lineStyle: { color: '#409eff' }
        },
        splitLine: { show: false }
      }
    ],
    series: [
      {
        name: 'Allocated GPU',
        type: 'line',
        yAxisIndex: 0,
        smooth: true,
        symbol: 'circle',
        symbolSize: 4,
        sampling: 'lttb', // Down-sampling for better performance
        data: sortedData.map(item => item.allocatedGpuCount),
        lineStyle: {
          width: 2,
          color: '#67c23a'
        },
        itemStyle: { color: '#67c23a' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(103, 194, 58, 0.3)' },
            { offset: 1, color: 'rgba(103, 194, 58, 0.05)' }
          ])
        }
      },
      {
        name: 'Avg Utilization',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        symbol: 'circle',
        symbolSize: 4,
        sampling: 'lttb', // Down-sampling for better performance
        data: sortedData.map(item => item.avgUtilization),
        lineStyle: {
          width: 2,
          color: '#409eff'
        },
        itemStyle: { color: '#409eff' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(64, 158, 255, 0.3)' },
            { offset: 1, color: 'rgba(64, 158, 255, 0.05)' }
          ])
        }
      }
    ]
  }
  
  chart.setOption(option, true)
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

// Fetch workspace data for pie charts
const fetchWorkspaceData = async () => {
  if (filters.value.namespace) {
    // Clear workspace data when a namespace filter is applied
    workspaceData.value = []
    return
  }
  
  pieLoading.value = true
  try {
    // Get unique namespaces from current data
    const uniqueNamespaces = [...new Set(statsData.value.map(item => item.namespace).filter(Boolean))]
    
    // Fetch workspace details for each namespace
    const workspacePromises = uniqueNamespaces.map(async (namespace) => {
      try {
        // Use namespace directly as workspace ID
        const workspaceId = namespace
        
        const data = await getWorkspaceDetail(workspaceId)
        return data
      } catch (error) {
        console.warn(`Failed to fetch workspace ${namespace}:`, error)
        return null
      }
    })
    
    const results = await Promise.all(workspacePromises)
    workspaceData.value = results.filter(Boolean) as WorkspaceDetail[]
    
    // Render pie charts after data is loaded
    await nextTick()
    // Clear and re-initialize all charts
    pieChartInstances.forEach((chart) => {
      chart.dispose()
    })
    pieChartInstances.clear()
    
    // Render new charts
    workspaceData.value.forEach(workspace => {
      renderPieChart(workspace)
    })
    
    // Also fetch GPU trend data for each workspace
    fetchGPUData()
  } catch (error) {
    console.error('Failed to fetch workspace data:', error)
  } finally {
    pieLoading.value = false
  }
}

// Watch cluster change to update namespace list
watch(selectedCluster, () => {
  filters.value.namespace = '' // Clear namespace selection when cluster changes
  fetchNamespaces()
  // Also re-fetch GPU data for new cluster
  if (!filters.value.namespace) {
    fetchGPUData()
  }
})

// Watch time range change to update namespace list
watch(timeRange, () => {
  fetchNamespaces()
})

// Watch selected namespace data change to re-render chart
watch(selectedNamespaceData, () => {
  if (selectedNamespace.value && selectedNamespaceData.value.length > 0) {
    renderTrendChart()
  }
}, { deep: true })

// Watch workspace data changes to render pie charts
watch(workspaceData, (newData) => {
  if (newData && newData.length > 0) {
    nextTick(() => {
      newData.forEach(workspace => {
        renderPieChart(workspace)
      })
    })
  }
}, { deep: true })

// Watch for global cluster changes
watch(selectedCluster, (newCluster) => {
  if (newCluster && timeRange.value) {
    fetchNamespaces()
    fetchData()
  }
})

onMounted(() => {
  // Delay initial load to prevent blocking page transition
  nextTick(() => {
    resetFilters()
  })
})

// Handle window resize
const handleResize = () => {
  pieChartInstances.forEach((chart) => {
    chart.resize()
  })
  if (trendChartInstance) {
    trendChartInstance.resize()
  }
  gpuChartInstances.forEach((chart) => {
    chart.resize()
  })
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  
  // Set initial time range and fetch data
  const endTime = dayjs().utc()
  const startTime = endTime.subtract(24, 'hour')
  timeRange.value = [
    startTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z',
    endTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z'
  ]
  fetchData()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  disposeTrendChart()
  
  // Dispose all pie charts
  pieChartInstances.forEach((chart) => {
    chart.dispose()
  })
  pieChartInstances.clear()
  pieChartRefs.clear()
  
  // Dispose all GPU charts
  gpuChartInstances.forEach((chart) => {
    chart.dispose()
  })
  gpuChartInstances.clear()
  gpuChartRefs.clear()
})
</script>

<style scoped lang="scss">
.namespace-stats {
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
      padding: 12px 0;
      gap: 20px;
      flex-wrap: wrap;
      
      @media (max-width: 768px) {
        gap: 12px;
      }
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
  }
  
  .namespace-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: 16px;
    margin-bottom: 20px;
    
    .namespace-empty {
      grid-column: 1 / -1;
      padding: 60px 20px;
      background: var(--el-bg-color);
      border-radius: 8px;
      
      .empty-description {
        text-align: center;
        
        p {
          margin: 8px 0;
          color: var(--el-text-color-primary);
          font-size: 14px;
          
          &.empty-hint {
            color: var(--el-text-color-secondary);
            font-size: 12px;
            margin-top: 12px;
          }
        }
      }
    }
    
    .namespace-card {
      border-radius: 12px;
      cursor: pointer;
      transition: all 0.3s ease;
      border: 2px solid transparent;
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
        transform: translateY(-4px);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
      }
      
      &.selected {
        border-color: var(--el-color-primary);
        box-shadow: 0 4px 12px rgba(64, 158, 255, 0.3);
      }
      
      .namespace-content {
        padding: 8px;
        
        .namespace-header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 16px;
          
          .namespace-name {
            font-size: 16px;
            font-weight: 600;
            color: var(--el-text-color-primary);
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
          }
        }
        
        .namespace-stats {
          display: grid;
          grid-template-columns: 1fr 1fr;
          gap: 12px;
          margin-bottom: 12px;
          
          .stat-item {
            .stat-label {
              font-size: 12px;
              color: var(--el-text-color-secondary);
              margin-bottom: 4px;
              display: flex;
              align-items: center;
              gap: 4px;
              
              .stat-help-icon {
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
            
            .stat-value {
              font-size: 20px;
              font-weight: 600;
              color: var(--el-color-primary);
            }
          }
        }
        
        .utilization-bar {
          margin-top: 8px;
        }
      }
    }
  }
  
  .chart-card {
    border-radius: 15px;
    margin-bottom: 20px;
    
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-weight: 600;
      font-size: 18px;
    }
  }
  
  .table-card {
    border-radius: 15px;
    
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
    
    .utilization-cell {
      padding: 4px 0;
    }
  }
  
  /* ===== Grid container: three columns ===== */
  .stat-grid {
    display: grid;
    gap: 16px;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    grid-auto-rows: 320px;
    margin-bottom: 20px;
  }

  /* Responsive for small screens: two columns / one column */
  @media (max-width: 1024px) {
    .stat-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (max-width: 768px) {
    .stat-grid {
      grid-template-columns: 1fr;
      grid-auto-rows: auto;
      gap: 12px;
    }
    
    .stat-card {
      .stat-header {
        .stat-title {
          font-size: 14px;
        }
        .stat-total__num {
          font-size: 1.8rem !important;
        }
      }
      
      .stat-bottom {
        flex-direction: column;
        align-items: stretch;
        gap: 16px;
      }
      
      .stat-badges {
        order: 2;
      }
      
      .small-pie-box {
        order: 1;
        flex: none;
        max-width: 100%;
        width: 100%;
        height: 200px;
        margin: 0;
      }
    }
  }

  /* ===== Card body ===== */
  .stat-card {
    position: relative;
    border-radius: 12px;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.06), rgba(255, 255, 255, 0.02))
      var(--el-bg-color);
    border: 1px solid var(--el-border-color-lighter);
    box-shadow: 0 6px 24px rgba(17, 24, 39, 0.06);
    overflow: hidden;
    transition:
      transform 0.25s ease,
      box-shadow 0.25s ease,
      border-color 0.25s ease;
    display: flex;
    flex-direction: column;
    
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
      z-index: 1;
    }
  }

  /* All cards use tall card style */
  .stat-card--tall {
    justify-content: space-between;
  }

  .stat-card--tall .stat-total__num {
    font-size: 2.4rem;
    line-height: 1;
  }

  .stat-card:hover {
    transform: translateY(-2px);
    box-shadow: 0 10px 32px rgba(17, 24, 39, 0.12);
  }

  /* el-card inner layer */
  .stat-card :deep(.el-card__body) {
    display: flex;
    flex-direction: column;
    padding: clamp(14px, 1.2vw, 28px);
    height: 100%;
  }

  /* ===== Header: title + top-right large number ===== */
  .stat-header {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: start;
    gap: 10px;
    padding-bottom: 6px;
  }

  .stat-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: calc(clamp(14px, 0.6vw + 10px, 16px));
    font-weight: 700;
    line-height: 1.4;
    color: var(--el-text-color-primary);
    display: flex;
    align-items: center;
    gap: 4px;
    
    .stat-help-icon {
      font-size: 14px;
      color: var(--el-text-color-secondary);
      cursor: help;
      transition: all 0.3s ease;
      flex-shrink: 0;
      
      &:hover {
        color: var(--el-color-primary);
        transform: scale(1.1);
      }
    }
    letter-spacing: 0.2px;
  }

  .stat-total {
    white-space: nowrap;
    line-height: 1;
    display: flex;
    align-items: center;
    gap: 4px;
    
    .stat-help-icon {
      font-size: 14px;
      color: var(--el-text-color-secondary);
      cursor: help;
      transition: all 0.3s ease;
      
      &:hover {
        color: var(--el-color-primary);
        transform: scale(1.1);
      }
    }
  }

  /* Top-right emphasized total (gradient text) */
  .stat-total__num {
    font-weight: 800;
    font-size: calc(clamp(16px, 0.6vw + 10px, 20px));
    line-height: 1;
    background-image: linear-gradient(
      180deg,
      color-mix(in oklab, #00b1a6 92%, #c2edfd 8%),
      color-mix(in oklab, #00b1a6 62%, #003932 12%)
    );
    -webkit-background-clip: text;
    background-clip: text;
    -webkit-text-fill-color: transparent;
    text-shadow: 0 1px 0 rgba(0, 0, 0, 0.06);
  }

  .stat-bottom {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 10px;
    flex: 1;
  }

  .stat-badges {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    flex: 1 1 auto;
    align-content: center;
  }

  /* Small pie chart */
  .small-pie-box {
    flex: 0 0 200px;
    max-width: 240px;
    margin-left: auto;
    aspect-ratio: 1 / 1;
    min-height: 160px;
    max-height: 200px;
  }

  .badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: calc(12px);
    color: var(--el-text-color-secondary);
    padding: 4px 8px;
    border-radius: 999px;
    background: color-mix(in oklab, var(--el-fill-color-lighter) 80%, white 20%);
    box-shadow: inset 0 0 0 1px var(--el-border-color-lighter);
    white-space: nowrap;
    min-width: 0;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dot {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    display: inline-block;
    flex-shrink: 0;
  }

  /* ---------- Bling text ---------- */
  /* Three state "base colors" */
  .badge--ok {
    --c: #67c23a;
  }

  .badge--bad {
    --c: #f56c6c;
  }

  .badge--used {
    --c: #00e5e5;
  }

  /* Default state: brighter gradient + slight glow */
  .badge--bling .badge-text {
    display: inline-block;
    font-weight: 700;
    /* Gradient fill: slightly brighter than before */
    --g1: color-mix(in oklab, var(--c) 96%, white 4%);
    --g2: color-mix(in oklab, var(--c) 70%, #003932 10%);
    background-image: linear-gradient(180deg, var(--g1), var(--g2));
    background-clip: text;
    -webkit-background-clip: text;
    color: transparent;
    -webkit-text-fill-color: transparent;
    /* Glow convergence: smaller radius, lower opacity */
    text-shadow:
      0 1px 0 rgba(0, 0, 0, 0.08),
      0 0 6px color-mix(in oklab, var(--c) 28%, transparent 72%);
    transition:
      filter 0.2s ease,
      color 0.2s ease,
      -webkit-text-fill-color 0.2s ease,
      background 0.2s ease;
  }

  /* Hover: text switches to primary color, no gradient, sharper */
  .badge--bling:hover .badge-text {
    background: none;
    color: color-mix(in oklab, var(--c) 94%, white 6%) !important;
    -webkit-text-fill-color: color-mix(in oklab, var(--c) 94%, white 6%) !important;
    text-shadow:
      0 1px 0 rgba(0, 0, 0, 0.1),
      0 0 8px color-mix(in oklab, var(--c) 35%, transparent 65%);
    filter: brightness(1.05);
  }

  /* Badge outline: light by default, slightly brighter on hover */
  .badge--bling {
    position: relative; /* For sweep light effect */
    box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--c) 30%, var(--el-border-color-lighter) 70%);
    overflow: hidden; /* Prevent sweep light overflow */
  }

  .badge--bling:hover {
    box-shadow:
      inset 0 0 0 1px color-mix(in oklab, var(--c) 55%, var(--el-border-color-lighter) 45%),
      0 0 10px -4px color-mix(in oklab, var(--c) 32%, transparent 68%);
  }

  /* Extra flair: sweep light effect on hover */
  .badge--bling::after {
    content: '';
    position: absolute;
    top: -40%;
    bottom: -40%;
    left: -60%;
    width: 60px;
    background: linear-gradient(
      120deg,
      transparent 0%,
      rgba(255, 255, 255, 0.1) 18%,
      rgba(255, 255, 255, 0.28) 35%,
      rgba(255, 255, 255, 0.1) 52%,
      transparent 70%
    );
    transform: translateX(-120%) rotate(20deg);
    transition: transform 0.6s ease;
    pointer-events: none;
  }

  .badge--bling:hover::after {
    transform: translateX(140%) rotate(20deg);
  }
}

// Date picker OK button styling
:deep(.el-date-editor) {
  .el-range-input {
    font-size: 13px;
  }
}

// Style for the date picker panel (appears in body)
:global(.el-picker-panel) {
  .el-picker-panel__footer {
    .el-picker-panel__link-btn {
      &:last-child {
        background: var(--el-color-primary);
        color: white;
        padding: 4px 12px;
        border-radius: 4px;
        font-weight: 500;
        
        &:hover {
          background: var(--el-color-primary-light-3);
          color: white;
        }
      }
    }
  }
  
  .loading-container {
    margin: 20px 0;
  }
}

// GPU Charts Grid
.gpu-charts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
  gap: 16px;
  margin-top: 20px;
  
  @media (max-width: 1200px) {
    grid-template-columns: 1fr;
  }
}

// GPU Chart Card Styles
.gpu-chart-card {
  border-radius: 16px;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.08) 0%, rgba(255, 255, 255, 0.02) 100%)
    var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  box-shadow:
    0 4px 16px rgba(0, 0, 0, 0.04),
    0 1px 3px rgba(0, 0, 0, 0.02),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
  backdrop-filter: blur(10px);
  transition: all 0.3s ease;
  overflow: hidden;
  position: relative;
  
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
    box-shadow:
      0 8px 24px rgba(0, 0, 0, 0.08),
      0 2px 6px rgba(0, 0, 0, 0.04),
      inset 0 1px 0 rgba(255, 255, 255, 0.08);
    transform: translateY(-2px);
  }
  
  :deep(.el-card__header) {
    border-bottom: 1px solid var(--el-border-color-lighter);
    padding: 10px 16px;
    font-size: 14px;
  }
  
  :deep(.el-card__body) {
    padding: 12px;
    background: transparent;
  }
}

.gpu-chart-box {
  width: 100%;
  height: 280px; // Optimized height
  position: relative;
}
</style>

