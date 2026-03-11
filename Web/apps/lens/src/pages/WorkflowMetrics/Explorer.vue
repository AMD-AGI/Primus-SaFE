<template>
  <div class="explorer-page">
    <!-- Query Builder -->
    <el-card class="query-card">
      <template #header>
        <div class="card-header">
          <span class="card-title">Query Builder</span>
        </div>
      </template>

      <el-form :model="queryForm" label-width="120px" label-position="right">
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Config">
              <el-select
                v-model="queryForm.configId"
                placeholder="Select Config"
                filterable
                class="w-full"
                @change="onConfigChange"
              >
                <el-option
                  v-for="config in configOptions"
                  :key="config.id"
                  :label="config.name"
                  :value="config.id"
                />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Time Range">
              <el-date-picker
                v-model="queryForm.timeRange"
                type="datetimerange"
                range-separator="to"
                start-placeholder="Start"
                end-placeholder="End"
                value-format="YYYY-MM-DDTHH:mm:ssZ"
                class="w-full"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Dimension Filters -->
        <el-form-item label="Dimensions" v-if="availableDimensions.length > 0">
          <div class="dimension-filters">
            <div v-for="dim in availableDimensions" :key="dim" class="dimension-row">
              <span class="dim-label">{{ dim }}:</span>
              <el-select
                v-model="queryForm.dimensions[dim]"
                multiple
                collapse-tags
                collapse-tags-tooltip
                :max-collapse-tags="2"
                placeholder="All"
                clearable
                class="dim-select"
              >
                <el-option
                  v-for="val in dimensionValues[dim] || []"
                  :key="val"
                  :label="val"
                  :value="val"
                />
              </el-select>
            </div>
          </div>
        </el-form-item>

        <!-- Metric Selection -->
        <el-form-item label="Metrics" v-if="availableMetrics.length > 0">
          <el-checkbox-group v-model="queryForm.selectedMetrics">
            <el-checkbox v-for="metric in availableMetrics" :key="metric" :label="metric">
              {{ metric }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>

        <!-- Interval for trends -->
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item label="Interval">
              <el-select v-model="queryForm.interval" class="w-full">
                <el-option label="1 Hour" value="1h" />
                <el-option label="6 Hours" value="6h" />
                <el-option label="1 Day" value="1d" />
                <el-option label="1 Week" value="1w" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item>
          <el-button type="primary" :loading="querying" :icon="Search" @click="executeQuery">
            Query
          </el-button>
          <el-button @click="resetQuery">Clear</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Results -->
    <template v-if="hasResults">
      <!-- Trends Chart -->
      <el-card class="chart-card">
        <template #header>
          <div class="card-header">
            <span class="card-title">Trends</span>
            <div class="card-actions">
              <el-button-group size="small">
                <el-button 
                  :type="chartType === 'line' ? 'primary' : 'default'"
                  @click="chartType = 'line'"
                >
                  Line
                </el-button>
                <el-button 
                  :type="chartType === 'bar' ? 'primary' : 'default'"
                  @click="chartType = 'bar'"
                >
                  Bar
                </el-button>
              </el-button-group>
            </div>
          </div>
        </template>

        <div ref="chartRef" class="chart-container" v-loading="querying"></div>
      </el-card>

      <!-- Dimension Groups Table (for controlling chart series visibility) -->
      <el-card class="groups-card">
        <template #header>
          <div class="card-header">
            <span class="card-title">Dimension Groups ({{ dimensionGroups.length }})</span>
            <div class="card-actions">
              <el-button size="small" @click="toggleAllSeries(true)">Show All</el-button>
              <el-button size="small" @click="toggleAllSeries(false)">Hide All</el-button>
            </div>
          </div>
        </template>

        <el-table :data="dimensionGroups" size="small" max-height="300" border>
          <el-table-column width="60" align="center">
            <template #header>
              <el-checkbox 
                :model-value="allSeriesVisible" 
                :indeterminate="someSeriesVisible"
                @change="toggleAllSeries($event as boolean)"
              />
            </template>
            <template #default="{ row }">
              <el-checkbox 
                :model-value="visibleSeries.has(row.key)" 
                @change="toggleSeries(row.key)"
              />
            </template>
          </el-table-column>
          <el-table-column label="Color" width="60" align="center">
            <template #default="{ row }">
              <div 
                class="color-dot" 
                :style="{ backgroundColor: seriesColors[row.key] || '#999' }"
              ></div>
            </template>
          </el-table-column>
          <el-table-column 
            v-for="dim in availableDimensions" 
            :key="dim"
            :prop="dim"
            :label="dim"
            min-width="150"
            show-overflow-tooltip
          />
          <el-table-column 
            v-for="metric in queryForm.selectedMetrics" 
            :key="`stat-${metric}`"
            :label="`${metric} (avg)`"
            align="right"
            min-width="100"
          >
            <template #default="{ row }">
              {{ formatNumber(row.stats?.[metric]?.avg) }}
            </template>
          </el-table-column>
          <el-table-column prop="count" label="Count" width="80" align="right" />
        </el-table>
      </el-card>

      <!-- Raw Data Table -->
      <el-card class="data-card">
        <template #header>
          <div class="card-header">
            <span class="card-title">Raw Data ({{ totalRawRecords }} records)</span>
            <div class="card-actions">
              <el-button :icon="Download" size="small" @click="exportCSV">Export CSV</el-button>
            </div>
          </div>
        </template>

        <el-table :data="rawResults" size="small" max-height="400" border stripe>
          <el-table-column
            v-for="dim in availableDimensions"
            :key="`dim-${dim}`"
            :label="dim"
            min-width="120"
          >
            <template #default="{ row }">{{ row.dimensions?.[dim] || '-' }}</template>
          </el-table-column>
          <el-table-column
            v-for="metric in availableMetrics"
            :key="`metric-${metric}`"
            :label="metric"
            align="right"
            min-width="100"
          >
            <template #default="{ row }">{{ formatNumber(row.metrics?.[metric]) }}</template>
          </el-table-column>
          <el-table-column prop="sourceFile" label="Source File" min-width="200" show-overflow-tooltip />
          <el-table-column label="Collected At" width="160">
            <template #default="{ row }">{{ formatDate(row.collectedAt) }}</template>
          </el-table-column>
        </el-table>

        <!-- Pagination -->
        <div v-if="totalRawRecords > queryForm.limit" class="pagination-container">
          <el-pagination
            v-model:current-page="currentPage"
            :page-size="queryForm.limit"
            :total="totalRawRecords"
            layout="total, prev, pager, next"
            @current-change="onPageChange"
          />
        </div>
      </el-card>
    </template>

    <!-- Empty State -->
    <el-card v-else-if="!querying" class="empty-card">
      <el-empty description="Select a config and run a query to see results">
        <template #image>
          <el-icon :size="64" color="#c0c4cc"><DataAnalysis /></el-icon>
        </template>
      </el-empty>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search, Download, DataAnalysis } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import dayjs from 'dayjs'
import {
  getConfigs,
  getDimensions,
  getMetricFields,
  getMetricsTrends,
  queryMetrics,
  type WorkflowConfig,
  type MetricRecord,
  type TrendsResponse
} from '@/services/workflow-metrics'

const route = useRoute()

// Chart colors palette
const COLORS = [
  '#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de',
  '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc', '#48b8d0',
  '#ff9f7f', '#87cefa', '#da70d6', '#32cd32', '#6495ed',
  '#ff69b4', '#ba55d3', '#cd5c5c', '#ffa500', '#40e0d0'
]

// State
const configOptions = ref<WorkflowConfig[]>([])
const availableDimensions = ref<string[]>([])
const availableMetrics = ref<string[]>([])
const dimensionValues = ref<Record<string, string[]>>({})
const querying = ref(false)

// Query form
const queryForm = reactive({
  configId: undefined as number | undefined,
  timeRange: [] as string[],
  dimensions: {} as Record<string, string[]>,
  selectedMetrics: [] as string[],
  interval: '1d',
  limit: 100
})

// Results
const rawResults = ref<MetricRecord[]>([])
const totalRawRecords = ref(0)
const currentPage = ref(1)
const trendsTimestamps = ref<string[]>([])

// Trends data grouped by dimension combination
interface TrendDataPoint {
  timestamp: string
  value: number
}

interface DimensionGroup {
  key: string  // Unique key for this dimension combination
  dimensions: Record<string, string>  // The actual dimension values
  stats: Record<string, { avg: number; sum: number; min: number; max: number }>
  count: number
  data: Record<string, TrendDataPoint[]>  // metric -> data points
  [key: string]: any  // For dynamic dimension columns
}

const dimensionGroups = ref<DimensionGroup[]>([])
const visibleSeries = ref<Set<string>>(new Set())
const seriesColors = ref<Record<string, string>>({})
const chartType = ref<'line' | 'bar'>('line')

const hasResults = computed(() => 
  rawResults.value.length > 0 || dimensionGroups.value.length > 0
)

const allSeriesVisible = computed(() => 
  dimensionGroups.value.length > 0 && 
  dimensionGroups.value.every(g => visibleSeries.value.has(g.key))
)

const someSeriesVisible = computed(() => 
  !allSeriesVisible.value && 
  dimensionGroups.value.some(g => visibleSeries.value.has(g.key))
)

// Chart
const chartRef = ref<HTMLElement>()
let chartInstance: echarts.ECharts | null = null

// Initialize
onMounted(async () => {
  // Load configs
  try {
    const res = await getConfigs({ limit: 100 })
    configOptions.value = res.configs || []
  } catch (error) {
    console.error('Failed to load configs:', error)
  }

  // Check for configId in query
  if (route.query.configId) {
    queryForm.configId = Number(route.query.configId)
    await onConfigChange()
  }

  // Set default time range (last 30 days)
  const now = dayjs()
  queryForm.timeRange = [
    now.subtract(30, 'day').format('YYYY-MM-DDTHH:mm:ssZ'),
    now.format('YYYY-MM-DDTHH:mm:ssZ')
  ]

  // Resize chart on window resize
  window.addEventListener('resize', () => chartInstance?.resize())
})

// Watch chart type changes
watch(chartType, () => {
  if (hasResults.value) {
    renderChart()
  }
})

// Watch visible series changes
watch(visibleSeries, () => {
  if (hasResults.value) {
    renderChart()
  }
}, { deep: true })

// Methods
const onConfigChange = async () => {
  if (!queryForm.configId) {
    availableDimensions.value = []
    availableMetrics.value = []
    dimensionValues.value = {}
    return
  }

  try {
    const [dimsRes, fieldsRes] = await Promise.all([
      getDimensions(queryForm.configId),
      getMetricFields(queryForm.configId)
    ])
    
    availableDimensions.value = dimsRes.dimensions || []
    dimensionValues.value = dimsRes.values || {}
    availableMetrics.value = fieldsRes.metricFields || []
    
    // Reset selections
    queryForm.dimensions = {}
    queryForm.selectedMetrics = availableMetrics.value.slice(0, 1)
  } catch (error) {
    console.error('Failed to load config metadata:', error)
    ElMessage.error('Failed to load config metadata')
  }
}

const executeQuery = async () => {
  if (!queryForm.configId) {
    ElMessage.warning('Please select a config')
    return
  }

  if (!queryForm.selectedMetrics.length) {
    ElMessage.warning('Please select at least one metric')
    return
  }

  querying.value = true
  rawResults.value = []
  dimensionGroups.value = []
  totalRawRecords.value = 0
  currentPage.value = 1
  visibleSeries.value = new Set()
  seriesColors.value = {}
  trendsTimestamps.value = []

  try {
    // Build dimension filters
    const dimensions: Record<string, any> = {}
    for (const [key, values] of Object.entries(queryForm.dimensions)) {
      if (values && values.length > 0) {
        dimensions[key] = values
      }
    }

    // Fetch both raw data and trends in parallel
    const [rawRes, trendsRes] = await Promise.all([
      queryMetrics(queryForm.configId, {
        start: queryForm.timeRange[0],
        end: queryForm.timeRange[1],
        dimensions,
        offset: 0,
        limit: queryForm.limit
      }),
      getMetricsTrends(queryForm.configId, {
        start: queryForm.timeRange[0],
        end: queryForm.timeRange[1],
        dimensions,
        metricFields: queryForm.selectedMetrics,
        interval: queryForm.interval,
        groupBy: availableDimensions.value  // Group by all dimensions
      })
    ])

    // Process raw data
    rawResults.value = rawRes.metrics || []
    totalRawRecords.value = rawRes.total || 0

    // Process trends - group by dimension combination
    const groupsMap = new Map<string, DimensionGroup>()
    
    // Store timestamps for chart rendering
    const timestamps = trendsRes.timestamps || []
    trendsTimestamps.value = timestamps

    // Process series from API response
    // Note: 'date' dimension needs special handling - it represents the time axis, not a grouping dimension
    if (trendsRes.series && trendsRes.series.length > 0) {
      for (const series of trendsRes.series) {
        const allDims = (series.dimensions || {}) as Record<string, string>
        
        // Separate 'date' from other dimensions
        // 'date' is used to position data on the time axis
        const dateValue = allDims.date
        const dims: Record<string, string> = {}
        for (const [k, v] of Object.entries(allDims)) {
          if (k !== 'date') {
            dims[k] = v
          }
        }
        
        // Create key from non-date dimensions
        const key = createDimensionKey(dims)
        
        if (!groupsMap.has(key)) {
          const group: DimensionGroup = {
            key,
            dimensions: dims,
            stats: {},
            count: 0,
            data: {},
            ...dims // Spread dimensions for table columns
          }
          groupsMap.set(key, group)
        }

        const group = groupsMap.get(key)!
        const metric = series.field || series.name
        
        // Initialize data array for this metric if not exists
        if (!group.data[metric]) {
          group.data[metric] = []
        }
        
        // If series has a 'date' dimension, use it to position data points
        // Otherwise use the timestamps array from API
        const values = series.values || []
        
        if (dateValue) {
          // This series represents a single date point
          // Find which timestamp index matches this date
          // Use the average of values (they should all be the same for a single date)
          const avgValue = values.length > 0 
            ? values.reduce((a: number, b: number) => a + b, 0) / values.length 
            : 0
          
          // Find matching timestamp or use the date directly
          let matchedTimestamp = timestamps.find(t => t.includes(dateValue))
          if (!matchedTimestamp) {
            // Try to construct a timestamp from the date
            matchedTimestamp = `${dateValue}T00:00:00Z`
          }
          
          // Check if we already have a data point for this timestamp
          const existingPoint = group.data[metric].find(p => p.timestamp === matchedTimestamp)
          if (!existingPoint) {
            group.data[metric].push({
              timestamp: matchedTimestamp,
              value: avgValue
            })
          }
        } else {
          // No date dimension - use timestamps array directly
          for (let i = 0; i < Math.min(timestamps.length, values.length); i++) {
            if (values[i] !== null && values[i] !== undefined) {
              group.data[metric].push({
                timestamp: timestamps[i],
                value: values[i]
              })
            }
          }
        }
        
        // Update stats
        const allValues = group.data[metric].map(p => p.value).filter(v => v !== null && v !== undefined)
        if (allValues.length > 0) {
          group.stats[metric] = {
            avg: allValues.reduce((a, b) => a + b, 0) / allValues.length,
            sum: allValues.reduce((a, b) => a + b, 0),
            min: Math.min(...allValues),
            max: Math.max(...allValues)
          }
        }
        
        // Update count from series counts
        const seriesTotal = series.counts?.reduce((a, b) => a + b, 0) || allValues.length
        group.count = Math.max(group.count, seriesTotal)
      }
      
      // Sort data points by timestamp for each group
      for (const group of groupsMap.values()) {
        for (const metric of Object.keys(group.data)) {
          group.data[metric].sort((a, b) => 
            new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
          )
        }
      }
    }
    
    // Fallback: if no dimension grouping from API, aggregate from raw data
    if (groupsMap.size === 0 && rawResults.value.length > 0) {
      for (const record of rawResults.value) {
        const dims = (record.dimensions || {}) as Record<string, string>
        const key = createDimensionKey(dims)
        
        if (!groupsMap.has(key)) {
          const group: DimensionGroup = {
            key,
            dimensions: dims,
            stats: {},
            count: 0,
            data: {},
            ...dims
          }
          groupsMap.set(key, group)
        }

        const group = groupsMap.get(key)!
        group.count++

        // Accumulate metric values for stats
        for (const metric of queryForm.selectedMetrics) {
          const value = record.metrics?.[metric]
          if (value !== undefined && value !== null) {
            if (!group.stats[metric]) {
              group.stats[metric] = { avg: 0, sum: 0, min: Infinity, max: -Infinity }
            }
            group.stats[metric].sum += value
            group.stats[metric].min = Math.min(group.stats[metric].min, value)
            group.stats[metric].max = Math.max(group.stats[metric].max, value)
          }
        }
      }

      // Calculate averages
      for (const group of groupsMap.values()) {
        for (const metric of queryForm.selectedMetrics) {
          if (group.stats[metric]) {
            group.stats[metric].avg = group.stats[metric].sum / group.count
          }
        }
      }
    }

    dimensionGroups.value = Array.from(groupsMap.values())

    // Assign colors and make all visible by default
    dimensionGroups.value.forEach((group, i) => {
      seriesColors.value[group.key] = COLORS[i % COLORS.length]
      visibleSeries.value.add(group.key)
    })

    // Render chart
    await nextTick()
    renderChart()

  } catch (error) {
    console.error('Failed to execute query:', error)
    ElMessage.error('Failed to execute query')
  } finally {
    querying.value = false
  }
}

const createDimensionKey = (dims: Record<string, string>): string => {
  // Create a stable key from dimension values
  const entries = Object.entries(dims).sort((a, b) => a[0].localeCompare(b[0]))
  return entries.map(([k, v]) => `${k}=${v}`).join('|')
}

const onPageChange = async (page: number) => {
  if (!queryForm.configId) return
  
  querying.value = true
  try {
    const dimensions: Record<string, any> = {}
    for (const [key, values] of Object.entries(queryForm.dimensions)) {
      if (values && values.length > 0) {
        dimensions[key] = values
      }
    }

    const res = await queryMetrics(queryForm.configId, {
      start: queryForm.timeRange[0],
      end: queryForm.timeRange[1],
      dimensions,
      offset: (page - 1) * queryForm.limit,
      limit: queryForm.limit
    })
    rawResults.value = res.metrics || []
  } catch (error) {
    console.error('Failed to load page:', error)
    ElMessage.error('Failed to load page')
  } finally {
    querying.value = false
  }
}

const toggleSeries = (key: string) => {
  const newSet = new Set(visibleSeries.value)
  if (newSet.has(key)) {
    newSet.delete(key)
  } else {
    newSet.add(key)
  }
  visibleSeries.value = newSet
}

const toggleAllSeries = (visible: boolean) => {
  if (visible) {
    visibleSeries.value = new Set(dimensionGroups.value.map(g => g.key))
  } else {
    visibleSeries.value = new Set()
  }
}

const renderChart = () => {
  if (!chartRef.value) return
  
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }

  // Get visible groups
  const visibleGroups = dimensionGroups.value.filter(g => visibleSeries.value.has(g.key))
  
  if (visibleGroups.length === 0) {
    chartInstance.clear()
    return
  }

  // Collect all timestamps
  const timestampSet = new Set<string>()
  for (const group of visibleGroups) {
    for (const metric of queryForm.selectedMetrics) {
      const data = group.data[metric] || []
      for (const point of data) {
        timestampSet.add(point.timestamp)
      }
    }
  }
  
  const timestamps = Array.from(timestampSet).sort()
  
  // Build series
  const series: echarts.SeriesOption[] = []
  
  for (const group of visibleGroups) {
    for (const metric of queryForm.selectedMetrics) {
      const data = group.data[metric] || []
      const dataMap = new Map(data.map(p => [p.timestamp, p.value]))
      
      // Create series name from dimensions
      const dimLabel = Object.entries(group.dimensions)
        .map(([k, v]) => `${v}`)
        .join(', ')
      
      const seriesName = queryForm.selectedMetrics.length > 1 
        ? `${dimLabel} - ${metric}`
        : dimLabel

      series.push({
        name: seriesName,
        type: chartType.value,
        data: timestamps.map(t => dataMap.get(t) ?? null),
        smooth: chartType.value === 'line',
        itemStyle: {
          color: seriesColors.value[group.key]
        },
        lineStyle: chartType.value === 'line' ? {
          color: seriesColors.value[group.key]
        } : undefined
      })
    }
  }

  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const borderColor = isDark ? '#FFFFFF1A' : '#00000012'

  const option: echarts.EChartsOption = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: chartType.value === 'line' ? 'line' : 'shadow' },
      formatter: (params: any) => {
        if (!Array.isArray(params)) params = [params]
        const time = params[0]?.axisValue || ''
        let html = `<div style="font-weight:600;margin-bottom:4px">${time}</div>`
        for (const p of params) {
          if (p.value !== null && p.value !== undefined) {
            html += `<div style="display:flex;align-items:center;gap:4px">
              <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:${p.color}"></span>
              <span>${p.seriesName}:</span>
              <span style="font-weight:600">${formatNumber(p.value)}</span>
            </div>`
          }
        }
        return html
      }
    },
    legend: {
      type: 'scroll',
      bottom: 0,
      data: series.map(s => s.name as string),
      textStyle: { color: textColor }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: timestamps.map(t => dayjs(t).format('YYYY-MM-DD')),
      axisLabel: {
        rotate: 30,
        interval: 'auto',
        color: textColor
      },
      axisLine: { lineStyle: { color: borderColor } }
    },
    yAxis: {
      type: 'value',
      name: queryForm.selectedMetrics.join(', '),
      axisLabel: { color: textColor },
      axisLine: { lineStyle: { color: borderColor } },
      splitLine: { lineStyle: { color: borderColor } }
    },
    series
  }

  chartInstance.setOption(option, true)
}

const resetQuery = () => {
  queryForm.dimensions = {}
  queryForm.selectedMetrics = availableMetrics.value.slice(0, 1)
  queryForm.interval = '1d'
  queryForm.limit = 100
  rawResults.value = []
  dimensionGroups.value = []
  totalRawRecords.value = 0
  currentPage.value = 1
  visibleSeries.value = new Set()
  seriesColors.value = {}
  chartType.value = 'line'
  
  if (chartInstance) {
    chartInstance.clear()
  }
}

const formatDate = (date: string) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm')
}

const formatNumber = (num: number) => {
  if (num === undefined || num === null) return '-'
  if (typeof num !== 'number') return String(num)
  return num.toLocaleString(undefined, { maximumFractionDigits: 4 })
}

const exportCSV = () => {
  const data = rawResults.value
  if (!data.length) return

  const firstRow = data[0]
  const dimKeys = Object.keys(firstRow.dimensions || {})
  const metricKeys = Object.keys(firstRow.metrics || {})
  const headers = [...dimKeys, ...metricKeys, 'sourceFile', 'collectedAt']
  const rows = data.map(row => [
    ...dimKeys.map(k => row.dimensions?.[k] || ''),
    ...metricKeys.map(k => row.metrics?.[k] || ''),
    row.sourceFile || '',
    row.collectedAt || ''
  ].join(','))

  const csv = [headers.join(','), ...rows].join('\n')

  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `metrics-${dayjs().format('YYYYMMDD-HHmmss')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<style scoped lang="scss">
.explorer-page {
  .query-card {
    margin-bottom: 20px;
    border-radius: 12px;

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .card-title {
        font-size: 16px;
        font-weight: 600;
      }
    }

    .dimension-filters {
      display: flex;
      flex-wrap: wrap;
      gap: 16px;

      .dimension-row {
        display: flex;
        align-items: center;
        gap: 8px;

        .dim-label {
          font-size: 13px;
          color: var(--el-text-color-secondary);
          min-width: 80px;
        }

        .dim-select {
          width: 200px;
        }
      }
    }
  }

  .chart-card {
    margin-bottom: 20px;
    border-radius: 12px;

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .card-title {
        font-size: 16px;
        font-weight: 600;
      }

      .card-actions {
        display: flex;
        align-items: center;
        gap: 12px;
      }
    }

    .chart-container {
      height: 400px;
      width: 100%;
    }
  }

  .groups-card {
    margin-bottom: 20px;
    border-radius: 12px;

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .card-title {
        font-size: 16px;
        font-weight: 600;
      }

      .card-actions {
        display: flex;
        align-items: center;
        gap: 8px;
      }
    }

    .color-dot {
      width: 12px;
      height: 12px;
      border-radius: 50%;
      display: inline-block;
    }
  }

  .data-card {
    margin-bottom: 20px;
    border-radius: 12px;

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .card-title {
        font-size: 16px;
        font-weight: 600;
      }

      .card-actions {
        display: flex;
        align-items: center;
        gap: 12px;
      }
    }

    .pagination-container {
      display: flex;
      justify-content: flex-end;
      margin-top: 16px;
      padding-top: 16px;
      border-top: 1px solid var(--el-border-color-lighter);
    }
  }

  .empty-card {
    border-radius: 12px;
    min-height: 300px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .w-full {
    width: 100%;
  }
}
</style>
