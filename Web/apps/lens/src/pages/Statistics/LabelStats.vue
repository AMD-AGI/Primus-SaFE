<template>
  <div class="label-stats">
    <div class="filter-section">
      <div class="filter-header">
        <h2 class="page-title">Label/Annotation GPU Statistics</h2>
        <div class="filters">
          <el-form :inline="true" :model="filters">
          <!-- Basic search -->
          <el-form-item>
            <el-select 
              v-model="filters.dimensionType" 
              placeholder="Dimension Type"
              size="default"
              style="width: 150px"
              @change="handleDimensionTypeChange"
            >
              <el-option label="Label" value="label" />
              <el-option label="Annotation" value="annotation" />
            </el-select>
          </el-form-item>
          
          <el-form-item>
            <el-select 
              v-model="filters.dimensionKey" 
              placeholder="Dimension Key"
              clearable
              size="default"
              style="width: 200px"
              :loading="dimensionKeyLoading"
            >
              <el-option
                v-for="key in dimensionKeyOptions"
                :key="key"
                :label="key"
                :value="key"
              />
            </el-select>
          </el-form-item>
          
          <!-- Advanced search collapse button -->
          <el-form-item>
            <el-button 
              @click="showAdvanced = !showAdvanced" 
              :class="['advanced-toggle', { active: showAdvanced }]"
              size="default"
            >
              <i v-if="showAdvanced" i="ep-arrow-up" class="toggle-icon" />
              <i v-else i="ep-arrow-down" class="toggle-icon" />
              Advanced Filters
            </el-button>
          </el-form-item>
          </el-form>
        </div>
      </div>
      
      <!-- Advanced search area -->
      <transition name="advanced-slide">
        <div v-show="showAdvanced" class="advanced-filters">
          <el-form :inline="true" :model="filters">
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
            
            <el-form-item>
              <el-input 
                v-model="filters.dimensionValue" 
                placeholder="Dimension Value (optional)"
                clearable
                size="default"
                style="width: 250px"
                @input="handleDimensionValueChange"
                @clear="handleDimensionValueChange"
              />
            </el-form-item>
          </el-form>
        </div>
      </transition>
    </div>

    <!-- Dimension Value Statistics -->
    <div class="dimension-stats">
      <template v-if="loading">
        <el-card class="stats-card">
          <el-skeleton :rows="4" animated />
        </el-card>
      </template>
      <template v-else-if="dimensionValues.length > 0">
        <el-card class="stats-card">
          <template #header>
            <div class="card-header">
              <span>Dimension Value Distribution</span>
            </div>
          </template>
          <div class="dimension-list">
            <div 
              v-for="item in dimensionValues" 
              :key="item.value"
              class="dimension-item"
              @click="showTrendChart(item.value)"
            >
              <div class="dimension-info">
                <el-tag type="primary" size="large">{{ item.value }}</el-tag>
                <span class="dimension-count">{{ item.count }} records</span>
              </div>
              <div class="dimension-metrics">
                <div class="metric-item">
                  <span class="metric-label">Avg GPUs:</span>
                  <span class="metric-value">{{ item.avgGPU.toFixed(2) }}</span>
                </div>
                <div class="metric-item">
                  <span class="metric-label">Avg Utilization:</span>
                  <span class="metric-value">{{ item.avgUtilization.toFixed(2) }}%</span>
                </div>
              </div>
            </div>
          </div>
        </el-card>
      </template>
      <template v-else>
        <el-card class="stats-card">
          <el-empty 
            description="No label/annotation data available"
            :image-size="150"
          >
            <template #description>
              <div class="empty-description">
                <p>No statistics found for the selected filters</p>
                <p class="empty-hint">Try selecting different dimension keys or adjusting the time range</p>
              </div>
            </template>
          </el-empty>
        </el-card>
      </template>
    </div>

    <!-- Trend Chart Dialog -->
    <el-dialog
      v-model="chartDialogVisible"
      :title="`${selectedDimensionValue} - Trend Analysis`"
      width="80%"
      top="5vh"
    >
      <div ref="chartContainer" style="width: 100%; height: 500px;"></div>
    </el-dialog>

    <!-- Data Table -->
    <el-card class="table-card">
      <el-table 
        v-loading="loading"
        :data="statsData" 
        stripe 
        style="width: 100%"
        @sort-change="handleTableSortChange"
      >
        <el-table-column prop="dimensionType" label="Dimension Type" min-width="160">
          <template #default="{ row }">
            <el-tag :type="row.dimensionType === 'label' ? 'success' : 'warning'" size="small">
              {{ row.dimensionType === 'label' ? 'Label' : 'Annotation' }}
            </el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="dimensionKey" label="Dimension Key" min-width="200" />
        
        <el-table-column prop="dimensionValue" label="Dimension Value" min-width="250" />
        
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
        
        <el-table-column prop="activeWorkloadCount" label="Active Workloads" min-width="160">
          <template #default="{ row }">
            {{ row.activeWorkloadCount ?? 0 }}
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
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { getLabelHourlyStats, getDimensionKeys, LabelGpuHourlyStats } from '@/services/gpu-aggregation'
import { useClusterSync } from '@/composables/useClusterSync'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import * as echarts from 'echarts'

dayjs.extend(utc)

// Get global cluster
const { selectedCluster } = useClusterSync()

const loading = ref(false)
const dimensionKeyLoading = ref(false)
const showAdvanced = ref(false)
const allStatsData = ref<LabelGpuHourlyStats[]>([]) // All data
const statsData = ref<LabelGpuHourlyStats[]>([]) // Current page data
const timeRange = ref<[string, string]>()
const dimensionKeyOptions = ref<string[]>([])
const filters = ref({
  dimensionType: 'annotation' as 'label' | 'annotation',
  dimensionKey: '',
  dimensionValue: ''
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

// Chart related
const chartDialogVisible = ref(false)
const selectedDimensionValue = ref('')
const chartContainer = ref<HTMLElement>()
let chartInstance: echarts.ECharts | null = null

// Calculate dimension value distribution (based on all data)
const dimensionValues = computed(() => {
  const valueMap = new Map<string, { count: number; totalGPU: number; totalUtilization: number }>()
  
  allStatsData.value.forEach(item => {
    const existing = valueMap.get(item.dimensionValue) || { count: 0, totalGPU: 0, totalUtilization: 0 }
    existing.count++
    existing.totalGPU += item.allocatedGpuCount
    existing.totalUtilization += item.avgUtilization
    valueMap.set(item.dimensionValue, existing)
  })
  
  return Array.from(valueMap.entries()).map(([value, data]) => ({
    value,
    count: data.count,
    avgGPU: data.totalGPU / data.count,
    avgUtilization: data.totalUtilization / data.count
  })).sort((a, b) => b.avgGPU - a.avgGPU)
})

// Handle dimension value change
const handleDimensionValueChange = () => {
  // Auto-fetch data when dimension value changes
  if (selectedCluster.value && timeRange.value && timeRange.value.length === 2 && filters.value.dimensionKey) {
    fetchData()
  }
}

// Fetch data
const fetchData = async () => {
  if (!selectedCluster.value) {
    ElMessage.warning('Please select a cluster from the header')
    return
  }
  
  if (!timeRange.value || timeRange.value.length !== 2) {
    ElMessage.warning('Please select time range')
    return
  }
  
  if (!filters.value.dimensionKey) {
    ElMessage.warning('Please enter dimension key')
    return
  }

  loading.value = true
  try {
    // Convert sort prop to API format
    let orderBy: 'time' | 'utilization' = 'time'
    if (currentSortProp.value === 'avgUtilization' || currentSortProp.value === 'allocatedGpuCount') {
      orderBy = 'utilization'
    }
    
    const params = {
      cluster: selectedCluster.value,
      dimensionType: filters.value.dimensionType,
      dimensionKey: filters.value.dimensionKey,
      dimensionValue: filters.value.dimensionValue || undefined,
      startTime: timeRange.value[0],
      endTime: timeRange.value[1],
      order_by: orderBy,
      order_direction: (currentSortOrder.value === 'ascending' ? 'asc' : 'desc') as 'asc' | 'desc'
    }
    
    const response = await getLabelHourlyStats(params)
    allStatsData.value = response.data
    
    // Reset pagination to first page
    pagination.value.page = 1
    
    // Frontend sorting and pagination
    sortAndPaginateData()
    
    // ElMessage.success(`Loaded ${response.data.length} records successfully`) // Removed success message to avoid blocking the UI
  } catch (error: any) {
    console.error('Failed to fetch label stats:', error)
    ElMessage.error(error || 'Failed to load data')
  } finally {
    loading.value = false
  }
}

// Handle dimension type change
const handleDimensionTypeChange = async () => {
  // Clear dimension key when type changes
  filters.value.dimensionKey = ''
  
  // Fetch new dimension keys
  await fetchDimensionKeys()
  
  // Auto-select first key if available
  if (dimensionKeyOptions.value.length > 0) {
    filters.value.dimensionKey = dimensionKeyOptions.value[0]
  }
}

// Handle table sort change
const handleTableSortChange = ({ prop, order }: { prop: string; order: string | null }) => {
  if (order) {
    currentSortProp.value = prop
    currentSortOrder.value = order as 'ascending' | 'descending'
  } else {
    currentSortProp.value = 'statHour'
    currentSortOrder.value = 'descending'
  }
  pagination.value.page = 1
  sortAndPaginateData()
}

// Handle page change
const handlePageChange = () => {
  sortAndPaginateData()
}

// Initialize filters with defaults
const initializeFilters = () => {
  filters.value.dimensionType = 'annotation'
  filters.value.dimensionKey = ''
  filters.value.dimensionValue = ''
  pagination.value.page = 1
  pagination.value.pageSize = 20
  currentSortProp.value = 'statHour'
  currentSortOrder.value = 'descending'
  // Default to last 24 hours
  const endTime = dayjs().utc()
  const startTime = endTime.subtract(24, 'hour')
  timeRange.value = [
    startTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z',
    endTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z'
  ]
  // No need to clear data here as watch will handle it
  
  // Auto load dimension keys and trigger initial search
  loadInitialData()
}

// Load initial data
const loadInitialData = async () => {
  try {
    // Fetch dimension keys first
    await fetchDimensionKeys()
    
    // If we have keys, select the first one and fetch data
    if (dimensionKeyOptions.value.length > 0) {
      filters.value.dimensionKey = dimensionKeyOptions.value[0]
      await fetchData()
    }
  } catch (error) {
    console.error('Failed to load initial data:', error)
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

// Fetch dimension keys
const fetchDimensionKeys = async () => {
  if (!timeRange.value || timeRange.value.length !== 2) {
    return
  }

  dimensionKeyLoading.value = true
  try {
    const params = {
      cluster: selectedCluster.value,
      dimensionType: filters.value.dimensionType,
      startTime: timeRange.value[0],
      endTime: timeRange.value[1]
    }
    const data = await getDimensionKeys(params)
    dimensionKeyOptions.value = data
  } catch (error: any) {
    console.error('Failed to fetch dimension keys:', error)
    dimensionKeyOptions.value = []
  } finally {
    dimensionKeyLoading.value = false
  }
}

// Watch cluster change to update dimension keys
watch(() => selectedCluster.value, () => {
  filters.value.dimensionKey = '' // Clear dimension key selection when cluster changes
  fetchDimensionKeys()
})

// Watch time range change to update dimension keys and fetch data
watch(timeRange, (newTimeRange) => {
  fetchDimensionKeys()
  if (newTimeRange && newTimeRange.length === 2 && filters.value.dimensionKey) {
    fetchData()
  }
})

// Watch dimension type change
watch(() => filters.value.dimensionType, () => {
  filters.value.dimensionKey = '' // Clear dimension key when type changes
  fetchDimensionKeys()
})

// Watch dimension key change
watch(() => filters.value.dimensionKey, (newKey) => {
  if (newKey && timeRange.value && timeRange.value.length === 2) {
    fetchData()
  }
})

// Frontend sorting and pagination handling
const sortAndPaginateData = () => {
  let sortedData = [...allStatsData.value]
  
  // Sort
  if (currentSortProp.value) {
    sortedData.sort((a, b) => {
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
  pagination.value.total = sortedData.length
  
  // Pagination
  const start = (pagination.value.page - 1) * pagination.value.pageSize
  const end = start + pagination.value.pageSize
  statsData.value = sortedData.slice(start, end)
}

// Show trend chart for a specific dimension value
const showTrendChart = async (dimensionValue: string) => {
  selectedDimensionValue.value = dimensionValue
  chartDialogVisible.value = true
  
  await nextTick()

  if (!chartContainer.value) return
  
  // Dispose existing chart instance
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
  
  // Filter data for the selected dimension value
  // Use allStatsData instead of statsData to get all records for this dimension value
  const filteredData = allStatsData.value
    .filter(item => item.dimensionValue === dimensionValue)
    .sort((a, b) => new Date(a.statHour).getTime() - new Date(b.statHour).getTime())
  
  if (filteredData.length === 0) {
    ElMessage.warning('No data available for this dimension value')
    return
  }
  
  // Prepare chart data
  const timeLabels = filteredData.map(item => formatTime(item.statHour))
  const gpuData = filteredData.map(item => item.allocatedGpuCount.toFixed(2))
  const utilizationData = filteredData.map(item => item.avgUtilization.toFixed(2))
  
  // Initialize chart
  chartInstance = echarts.init(chartContainer.value)
  
  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const borderColor = isDark ? '#FFFFFF1A' : '#00000012'

  const option: echarts.EChartsOption = {
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
      data: ['GPU Count', 'GPU Utilization'],
      top: 10,
      textStyle: { color: textColor }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: timeLabels,
      axisPointer: {
        type: 'shadow'
      },
      axisLabel: {
        rotate: 45,
        interval: Math.floor(timeLabels.length / 10) || 0,
        color: textColor
      },
      axisLine: { lineStyle: { color: borderColor } }
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
        axisLine: { show: true, lineStyle: { color: '#409EFF' } },
        splitLine: { lineStyle: { color: borderColor } }
      },
      {
        type: 'value',
        name: 'GPU Utilization (%)',
        position: 'right',
        min: 0,
        max: 100,
        axisLabel: {
          formatter: '{value}%',
          color: textColor
        },
        axisLine: { show: true, lineStyle: { color: '#67C23A' } },
        splitLine: { show: false }
      }
    ],
    series: [
      {
        name: 'GPU Count',
        type: 'line',
        data: gpuData,
        smooth: true,
        yAxisIndex: 0,
        itemStyle: {
          color: '#409EFF'
        },
        lineStyle: {
          width: 2
        }
      },
      {
        name: 'GPU Utilization',
        type: 'line',
        data: utilizationData,
        smooth: true,
        yAxisIndex: 1,
        itemStyle: {
          color: '#67C23A'
        },
        lineStyle: {
          width: 2
        }
      }
    ]
  }
  
  chartInstance.setOption(option)
  
  // Handle window resize
  window.addEventListener('resize', handleResize)
}

// Handle chart resize
const handleResize = () => {
  if (chartInstance) {
    chartInstance.resize()
  }
}

// Watch dialog close to cleanup
watch(chartDialogVisible, (newVal) => {
  if (!newVal) {
    window.removeEventListener('resize', handleResize)
    if (chartInstance) {
      chartInstance.dispose()
      chartInstance = null
    }
  }
})

// Watch for global cluster changes
watch(selectedCluster, (newCluster) => {
  if (newCluster && timeRange.value) {
    fetchDimensionKeys()
    fetchData()
  }
})

onMounted(() => {
  // Delay initial load to prevent blocking page transition
  nextTick(() => {
    initializeFilters()
  })
})
</script>

<style scoped lang="scss">
.label-stats {
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;
  padding: 0 20px;
  
  @media (max-width: 768px) {
    padding: 0 12px;
  }
  
  
  .filter-section {
    margin-bottom: 20px;
    
    .filter-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      padding: 12px 0;
      gap: 20px;
      flex-wrap: wrap;
      
      @media (max-width: 1024px) {
        flex-direction: column;
        align-items: stretch;
        gap: 16px;
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
      flex-wrap: wrap; // Allow wrapping
      
      @media (max-width: 1200px) {
        gap: 8px;
      }
      
      :deep(.el-form) {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
        align-items: center;
        width: 100%;
        
        @media (max-width: 768px) {
          flex-direction: column;
          align-items: stretch;
        }
      }
      
      :deep(.el-form-item) {
        align-items: center;
        margin-bottom: 0;
        
        @media (max-width: 768px) {
          display: flex;
          flex-direction: column;
          align-items: stretch;
          margin-bottom: 12px;
          width: 100%;
        }
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
        
        @media (max-width: 768px) {
          line-height: 1.5;
          height: auto;
          margin-bottom: 4px;
        }
      }
      
      :deep(.el-form-item__content) {
        line-height: 32px;
        display: flex;
        align-items: center;
        
        @media (max-width: 768px) {
          width: 100%;
          
          .el-select,
          .el-input {
            width: 100% !important;
          }
        }
      }
      
      // Button group
      .button-group {
        @media (max-width: 768px) {
          width: 100%;
          
          :deep(.el-form-item__content) {
            display: flex;
            gap: 8px;
            
            .el-button {
              flex: 1;
            }
          }
        }
      }
      
      // Advanced filter button
      .advanced-toggle {
        background: linear-gradient(135deg, var(--el-color-primary) 0%, var(--el-color-primary-light-3) 100%);
        color: white;
        border: none;
        transition: all 0.3s ease;
        font-weight: 500;
        
        &:hover {
          transform: translateY(-2px);
          box-shadow: 0 4px 12px rgba(64, 158, 255, 0.4);
        }
        
        &.active {
          background: linear-gradient(135deg, var(--el-color-success) 0%, var(--el-color-success-light-3) 100%);
        }
        
        .toggle-icon {
          margin-right: 6px;
          transition: transform 0.3s ease;
        }
      }
      
      // Query button enhancement
      .query-btn {
        background: linear-gradient(135deg, var(--el-color-primary) 0%, var(--el-color-primary-light-3) 100%);
        border: none;
        
        &:hover {
          transform: translateY(-2px);
          box-shadow: 0 4px 12px rgba(64, 158, 255, 0.4);
        }
      }
      
      // Advanced search area with glassmorphism
      .advanced-filters {
        background: linear-gradient(135deg, rgba(64, 158, 255, 0.08) 0%, rgba(64, 158, 255, 0.04) 100%);
        backdrop-filter: blur(16px) saturate(180%);
        -webkit-backdrop-filter: blur(16px) saturate(180%);
        border-radius: 12px;
        padding: 16px;
        margin-top: 12px;
        border: 1px solid rgba(64, 158, 255, 0.2);
        box-shadow: 0 8px 32px rgba(64, 158, 255, 0.12);
        transition: all 0.3s ease;
        
        @media (max-width: 768px) {
          padding: 12px;
          
          :deep(.el-form) {
            display: flex;
            flex-direction: column;
            gap: 12px;
          }
          
          :deep(.el-form-item) {
            width: 100%;
            margin-bottom: 0;
          }
          
          :deep(.el-date-picker),
          :deep(.el-input) {
            width: 100% !important;
          }
        }
        
        &:hover {
          box-shadow: 0 12px 40px rgba(64, 158, 255, 0.18);
          border-color: rgba(64, 158, 255, 0.35);
        }
        
        :deep(.el-form) {
          margin-bottom: 0;
        }
        
        :deep(.el-form-item) {
          margin-bottom: 16px;
        }
      }
    }
    
    // Advanced search expand animation
    .advanced-slide-enter-active,
    .advanced-slide-leave-active {
      transition: all 0.3s ease;
    }
    
    .advanced-slide-enter-from,
    .advanced-slide-leave-to {
      opacity: 0;
      transform: translateY(-10px);
    }
    
    .advanced-slide-enter-to,
    .advanced-slide-leave-from {
      opacity: 1;
      transform: translateY(0);
    }
  }
  
  .dimension-stats {
    margin-bottom: 20px;
    
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
    
    .stats-card {
      border-radius: 15px;
      
      .card-header {
        font-weight: 600;
        font-size: 16px;
      }
      
      .dimension-list {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
        gap: 16px;
        padding: 10px 0;
        
        .dimension-item {
          padding: 16px;
          border: 1px solid var(--el-border-color);
          border-radius: 8px;
          display: flex;
          justify-content: space-between;
          align-items: center;
          transition: all 0.3s;
          cursor: pointer;
          
          &:hover {
            background-color: var(--el-fill-color-light);
            border-color: var(--el-color-primary);
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
          }
          
          .dimension-info {
            display: flex;
            flex-direction: column;
            gap: 8px;
            flex: 1;
            
            .dimension-count {
              font-size: 12px;
              color: var(--el-text-color-secondary);
            }
          }
          
          .dimension-metrics {
            display: flex;
            flex-direction: column;
            gap: 8px;
            text-align: right;
            
            .metric-item {
              display: flex;
              flex-direction: column;
              gap: 2px;
              
              .metric-label {
                font-size: 12px;
                color: var(--el-text-color-secondary);
              }
              
              .metric-value {
                font-size: 16px;
                font-weight: 600;
                color: var(--el-color-primary);
              }
            }
          }
        }
      }
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
    
    // Progress bar text size
    :deep(.el-progress) {
      .el-progress__text {
        font-size: 13px;
        
        @media (min-width: 1920px) {
          font-size: 14px;
        }
      }
    }
  }
}
</style>

