<template>
  <div class="header-row">
    <h3 class="chart-title">Usage breakdown</h3>
    <div class="header-links">
      <!-- Hyperloom entry - shown only in OCI environment -->
      <el-button v-if="isOci" link @click="goToHyperloom" class="lens-link">
        Go to Hyperloom
        <el-icon class="ml-1"><Right /></el-icon>
      </el-button>
      <!-- Lens entry - shown only in production -->
      <el-button v-if="isProd" link @click="goToLens" class="lens-link">
        Go to Lens
        <el-icon class="ml-1"><Right /></el-icon>
      </el-button>
    </div>
  </div>

  <!-- Template -->
  <div class="stat-grid mb-6">
    <el-card
      v-for="(s, i) in statCards"
      :key="s.key"
      shadow="never"
      class="stat-card"
      :class="{ 'stat-card--tall': i === 0 }"
      :style="{
        '--accent': PIE_COLORS[0] || '#16a34a',
        '--accent-bad': PIE_COLORS[1] || '#ef4444',
        '--accent-used': PIE_COLORS[2] || '#3b82f6',
      }"
    >
      <div class="stat-header">
        <div class="stat-title">{{ s.title }}</div>
        <div class="stat-total">
          <span class="stat-total__num">{{ s.total }}</span>
        </div>
      </div>

      <div class="stat-bottom">
        <div class="stat-badges">
          <span class="badge badge--bling badge--ok" v-if="s.avail !== undefined">
            <i class="dot" :style="{ background: PIE_COLORS[0] || '' }"></i>
            <span class="badge-text">Available {{ s.avail }}</span>
          </span>
          <span class="badge badge--bling badge--bad">
            <i class="dot" :style="{ background: PIE_COLORS[1] || '' }"></i>
            <span class="badge-text">Abnormal {{ s.abnormal }}</span>
          </span>
          <span class="badge badge--bling badge--used" v-if="s.used !== undefined">
            <i class="dot" :style="{ background: PIE_COLORS[2] || '' }"></i>
            <span class="badge-text">Used {{ s.used }}</span>
          </span>
        </div>

        <!-- Only add a small pie chart to Nodes card -->
        <div v-if="i === 0" class="small-pie-box" :ref="(el) => (nodePieRef = el as any)" />
      </div>
    </el-card>
  </div>

  <!-- GPU resource utilization line chart -->
  <div class="gpu-chart-section">
    <div class="header-row mb-4">
      <h3 class="chart-title">GPU Utilization & Allocation</h3>
      <div class="chart-filters">
        <el-radio-group v-model="quickDateRange" @change="onQuickDateChange">
          <el-radio-button :value="1">Past 1 Day</el-radio-button>
          <el-radio-button :value="7">Past 7 Days</el-radio-button>
          <el-radio-button :value="30">Past 30 Days</el-radio-button>
          <el-radio-button :value="0">Custom</el-radio-button>
        </el-radio-group>
        <el-date-picker
          v-model="dateRange"
          type="datetimerange"
          range-separator="To"
          start-placeholder="Start time"
          end-placeholder="End time"
          :disabled="quickDateRange !== 0"
          @change="onDateRangeChange"
          class="ml-3"
        />
      </div>
    </div>

    <div class="gpu-layout">
      <!-- Left side line chart -->
      <el-card shadow="never" class="gpu-chart-card" v-loading="gpuLoading">
        <div class="gpu-chart-box" ref="gpuChartRef" />
      </el-card>

      <!-- Right side statistics -->
      <div class="gpu-stats-panel">
        <el-card shadow="never" class="gpu-stat-card">
          <div class="stat-icon stat-icon--info">
            <el-icon><List /></el-icon>
          </div>
          <div class="stat-content">
            <div class="stat-label">Total Workloads</div>
            <div class="stat-value stat-value--info">{{ gpuStats.totalWorkloads }}</div>
          </div>
        </el-card>
        <el-card shadow="never" class="gpu-stat-card">
          <div class="stat-icon stat-icon--success">
            <el-icon><TrendCharts /></el-icon>
          </div>
          <div class="stat-content">
            <div class="stat-label">Avg Allocation</div>
            <div class="stat-value stat-value--success">
              {{ gpuStats.avgAllocation }}
              <span class="stat-unit">%</span>
            </div>
          </div>
        </el-card>
        <el-card shadow="never" class="gpu-stat-card">
          <div class="stat-icon stat-icon--primary">
            <el-icon><Odometer /></el-icon>
          </div>
          <div class="stat-content">
            <div class="stat-label">Avg Utilization</div>
            <div class="stat-value stat-value--primary">
              {{ gpuStats.avgUtilization }}
              <span class="stat-unit">%</span>
            </div>
          </div>
        </el-card>
        <el-card shadow="never" class="gpu-stat-card">
          <div class="stat-icon stat-icon--warning">
            <el-icon><WarningFilled /></el-icon>
          </div>
          <div class="stat-content">
            <div class="stat-label">Low Utilization</div>
            <div class="stat-value stat-value--warning">{{ gpuStats.lowUtilization }}</div>
          </div>
        </el-card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount, onMounted } from 'vue'
import * as echarts from 'echarts'
import { Right, Odometer, TrendCharts, List, WarningFilled } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { getWorkspaceDetail } from '@/services/workspace/index'
import { getGPUAggregation } from '@/services/workload/index'
import { byte2Gi } from '@/utils/index'
import { useClusterStore } from '@/stores/cluster'

// Check if in production environment
const isProd = import.meta.env.PROD
const isOci = window.location.origin === 'https://oci-slc.primus-safe.amd.com'

// Navigate to Lens system
const goToLens = () => {
  window.open(`${location.origin}/lens`, '_blank')
}

const goToHyperloom = () => {
  window.open('https://oci-slc.primus-safe.amd.com/hyperloom/', '_blank')
}

const store = useWorkspaceStore()
const clusterStore = useClusterStore()
const PIE_COLORS = ['#67C23A', '#F56C6C', '#00e5e5'] as const

const detailData = ref<any>(null)
const RES_KEYS = ['amd.com/gpu', 'rdma/hca', 'cpu', 'memory'] as const
const unitOf = (k: string) => (k === 'ephemeral-storage' || k === 'memory' ? 'Gi' : '')
const normalized = computed(() => {
  const d = detailData.value || {}
  const pick = (group: any) =>
    RES_KEYS.map((k) => {
      const raw = group?.[k] ?? 0
      return k === 'memory' ? byte2Gi(raw, 0, false) : raw
    })
  return { total: pick(d.totalQuota), avail: pick(d.availQuota), abnormal: pick(d.abnormalQuota) }
})
const usedNormal = computed(() =>
  normalized.value.total.map((t, i) =>
    Math.max(t - (normalized.value.avail[i] ?? 0) - (normalized.value.abnormal[i] ?? 0), 0),
  ),
)

const legends = ['Available', 'Abnormal', 'Used']

// Nodes pie chart option
const nodeNumbers = computed(() => {
  const total = Number(detailData.value?.currentNodeCount ?? 0)
  const used = Number(detailData.value?.usedNodeCount ?? 0)
  const abnormal = Number(detailData.value?.abnormalNodeCount ?? 0)
  const avail = Math.max(total - used - abnormal, 0)
  return { total, avail, abnormal, used }
})

const fmtPct = (x: number | undefined) =>
  `${Number.isFinite(x as number) ? (x as number).toFixed(1) : '0.0'}%`

const buildNodePieOption = (): echarts.EChartsOption => {
  const n = nodeNumbers.value
  return {
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
          { name: legends[0], value: n.avail },
          { name: legends[1], value: n.abnormal },
          { name: legends[2], value: n.used },
        ],
      },
    ],
  }
}

const nodePieRef = ref<HTMLElement | null>(null)
let nodePieChart: echarts.ECharts | null = null

function renderAllCharts() {
  // Nodes pie chart
  if (!nodePieChart && nodePieRef.value) {
    nodePieChart = echarts.init(nodePieRef.value)
  }
  nodePieChart?.setOption(buildNodePieOption(), true)
}

type StatCard = {
  key: string
  title: string
  total: string
  avail?: string
  abnormal?: string
  used?: string
}
const fmt = (v: number, u: string) => (u ? `${v} ${u}` : String(v))
const statCards = computed<StatCard[]>(() => {
  const res = RES_KEYS.map<StatCard>((k, i) => {
    const unit = unitOf(k)
    return {
      key: k as string,
      title: `${k}${unit ? ` (${unit})` : ''}`,
      total: fmt(normalized.value.total[i] ?? 0, unit),
      avail: fmt(normalized.value.avail[i] ?? 0, unit),
      abnormal: fmt(normalized.value.abnormal[i] ?? 0, unit),
      used: fmt(usedNormal.value[i] ?? 0, unit),
    }
  })

  res.unshift({
    key: 'nodes',
    title: 'Nodes',
    avail: String(
      detailData.value?.currentNodeCount
        ? detailData.value?.currentNodeCount -
            detailData.value?.usedNodeCount -
            detailData.value?.abnormalNodeCount
        : '0',
    ),
    total: String(detailData.value?.currentNodeCount ?? 0),
    abnormal: String(detailData.value?.abnormalNodeCount ?? 0),
    used: String(detailData.value?.usedNodeCount ?? 0),
  })

  return res
})

async function getDetail() {
  if (!store.currentWorkspaceId) return
  detailData.value = await getWorkspaceDetail(store.currentWorkspaceId)
}

watch(
  () => store.currentWorkspaceId,
  (id) => {
    if (id) getDetail()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (nodePieChart) {
    nodePieChart.dispose()
    nodePieChart = null
  }
  if (gpuChart) {
    gpuChart.dispose()
    gpuChart = null
  }
})

watch(
  () => detailData.value,
  async (v) => {
    if (!v) return
    await nextTick()
    renderAllCharts()
  },
)

// ========== GPU resource utilization line chart related ==========
interface GPUAggregationItem {
  id: number
  cluster_name: string
  namespace: string
  stat_hour: string
  avg_utilization: number
  allocation_rate: number
  allocated_gpu_count: number
  active_workload_count: number
  total_gpu_capacity: number
  created_at: string
  updated_at: string
}

const gpuChartRef = ref<HTMLElement | null>(null)
let gpuChart: echarts.ECharts | null = null
const gpuLoading = ref(false)
const quickDateRange = ref(7) // Default to past 7 days
const dateRange = ref<[Date, Date] | null>(null)
const gpuData = ref<GPUAggregationItem[]>([])

// GPU statistics
const gpuStats = computed(() => {
  if (!gpuData.value || gpuData.value.length === 0) {
    return {
      avgUtilization: '0.0',
      avgAllocation: '0.0',
      totalWorkloads: 0,
      lowUtilization: 0,
    }
  }

  // Data is in descending order, so the first item is the latest
  const latest = gpuData.value[0]
  const avgUtil =
    gpuData.value.reduce((sum, item) => sum + (item.avg_utilization || 0), 0) / gpuData.value.length
  const avgAlloc =
    gpuData.value.reduce((sum, item) => sum + (item.allocation_rate || 0), 0) / gpuData.value.length

  // Calculate Total Workloads - use the latest active_workload_count
  const totalWorkloads = latest.active_workload_count || 0

  // Calculate Low Utilization - count data points with utilization below 30%
  const lowUtilization = gpuData.value.filter((item) => item.avg_utilization < 30).length

  return {
    avgUtilization: avgUtil.toFixed(1),
    avgAllocation: avgAlloc.toFixed(1),
    totalWorkloads: totalWorkloads,
    lowUtilization: lowUtilization,
  }
})

// Calculate date range
const getDateRange = (days: number): [Date, Date] => {
  const end = new Date()
  const start = new Date()
  start.setDate(start.getDate() - days)
  return [start, end]
}

// Initialize date range
const initDateRange = () => {
  if (quickDateRange.value > 0) {
    dateRange.value = getDateRange(quickDateRange.value)
  }
}

// Quick date range switch
const onQuickDateChange = (val: number) => {
  if (val > 0) {
    dateRange.value = getDateRange(val)
    fetchGPUData()
  }
}

// Custom date range switch
const onDateRangeChange = () => {
  if (dateRange.value) {
    fetchGPUData()
  }
}

// Format time with timezone: 2025-11-01T00:00:00+08:00
const formatDateWithTimezone = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')

  // Get timezone offset (in minutes)
  const timezoneOffset = -date.getTimezoneOffset()
  const offsetHours = String(Math.floor(Math.abs(timezoneOffset) / 60)).padStart(2, '0')
  const offsetMinutes = String(Math.abs(timezoneOffset) % 60).padStart(2, '0')
  const offsetSign = timezoneOffset >= 0 ? '+' : '-'

  return `${year}-${month}-${day}T${hours}:${minutes}:${seconds}${offsetSign}${offsetHours}:${offsetMinutes}`
}

// Fetch GPU data
const fetchGPUData = async () => {
  if (!dateRange.value || !store.currentWorkspaceId) return

  gpuLoading.value = true
  try {
    const [start, end] = dateRange.value
    const response = await getGPUAggregation({
      cluster: clusterStore.currentClusterId ?? '',
      namespace: store.currentWorkspaceId,
      start_time: formatDateWithTimezone(start),
      end_time: formatDateWithTimezone(end),
      page: 1,
      page_size: 20,
      order_by: 'time',
      order_direction: 'desc',
    })

    gpuData.value = response?.data?.data || []
    renderGPUChart()
  } catch (error) {
    console.error('Failed to fetch GPU data:', error)
  } finally {
    gpuLoading.value = false
  }
}

// Render GPU line chart
const renderGPUChart = async () => {
  await nextTick()
  if (!gpuChartRef.value) return

  if (!gpuChart) {
    gpuChart = echarts.init(gpuChartRef.value)
  }

  if (gpuData.value.length === 0) {
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
    gpuChart.setOption(emptyOption, true)
    return
  }

  // Process data - descending order needs to be reversed
  const sortedData = [...gpuData.value].reverse()

  const times = sortedData.map((item) => {
    const date = new Date(item.stat_hour)
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  })

  const avgUtilization = sortedData.map((item) =>
    item.avg_utilization ? Number(item.avg_utilization.toFixed(2)) : 0,
  )

  const allocationRate = sortedData.map((item) =>
    item.allocation_rate ? Number(item.allocation_rate.toFixed(2)) : 0,
  )

  // Calculate appropriate X-axis label interval to avoid overcrowding
  const calculateInterval = (dataLength: number): number => {
    if (dataLength <= 24) return 0 // 24 data points or fewer — show all
    if (dataLength <= 48) return 1 // Show every other one
    if (dataLength <= 168) return Math.floor(dataLength / 12) // Show about 12 labels
    return Math.floor(dataLength / 10) // Show about 10 labels
  }

  // Detect dark mode
  const isDark =
    document.documentElement.classList.contains('dark') ||
    (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)

  // Theme color palette
  const colorText = isDark ? '#E5EAF3' : '#303133'
  const colorSubtext = isDark ? '#C8CDD5' : '#606266'
  const colorAxis = isDark ? '#FFFFFF33' : '#00000026'
  const colorGrid = isDark ? '#FFFFFF1F' : '#00000012'
  const tooltipBg = isDark ? 'rgba(17,24,39,0.95)' : '#fff'
  const tooltipBorder = isDark ? 'rgba(255,255,255,0.18)' : '#ebeef5'
  const pointerColor = isDark ? '#FFFFFF3D' : '#0000003d'

  const option: echarts.EChartsOption = {
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      padding: 12,
      textStyle: {
        color: colorText,
        fontSize: 13,
      },
      axisPointer: {
        type: 'line',
        lineStyle: {
          color: pointerColor,
          width: 2,
          type: 'solid',
        },
      },
      formatter: (params: unknown) => {
        const items = params as Array<{
          axisValue: string
          seriesName: string
          value: number
          color: string
        }>
        let result = `<div style="padding: 4px 0;">
          <div style="font-weight: 600; margin-bottom: 8px; font-size: 14px;">${items[0].axisValue}</div>`
        items.forEach((item) => {
          result += `
            <div style="margin: 6px 0; display: flex; align-items: center; justify-content: space-between; gap: 16px;">
              <span style="display: flex; align-items: center;">
                <span style="display: inline-block; width: 12px; height: 12px; border-radius: 50%; background: ${item.color}; margin-right: 8px; box-shadow: 0 0 4px ${item.color}50;"></span>
                <span style="opacity: 0.9;">${item.seriesName}</span>
              </span>
              <span style="font-weight: 600; font-size: 14px;">${item.value}%</span>
            </div>
          `
        })
        result += '</div>'
        return result
      },
    },
    legend: {
      data: ['Avg Utilization', 'Allocation Rate'],
      top: 12,
      textStyle: {
        color: colorText,
        fontSize: 13,
        padding: [0, 8, 0, 0],
      },
      itemWidth: 28,
      itemHeight: 14,
      itemGap: 20,
    },
    grid: {
      left: '2%',
      right: '1%',
      bottom: '8%',
      top: '16%',
      containLabel: true,
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: times,
      axisTick: {
        show: false,
      },
      axisLine: {
        lineStyle: {
          color: colorAxis,
          width: 1.5,
        },
      },
      axisLabel: {
        color: colorSubtext,
        fontSize: 12,
        rotate: 45,
        interval: calculateInterval(times.length),
        margin: 12,
        fontWeight: 500,
      },
    },
    yAxis: {
      type: 'value',
      name: 'Percentage (%)',
      nameTextStyle: {
        color: colorText,
        fontSize: 13,
        fontWeight: 600,
        padding: [0, 0, 8, 0],
      },
      min: 0,
      max: 100,
      axisLine: {
        show: true,
        lineStyle: {
          color: colorAxis,
          width: 1.5,
        },
      },
      axisLabel: {
        color: colorSubtext,
        fontSize: 12,
        formatter: '{value}%',
        fontWeight: 500,
      },
      splitLine: {
        lineStyle: {
          color: colorGrid,
          width: 1,
        },
      },
    },
    series: [
      {
        name: 'Avg Utilization',
        type: 'line',
        data: avgUtilization,
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        showSymbol: false,
        lineStyle: {
          width: 2.5,
          color: '#409EFF',
        },
        itemStyle: {
          color: '#409EFF',
          borderColor: '#fff',
          borderWidth: 2,
        },
        emphasis: {
          scale: true,
          focus: 'series',
        },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(64, 158, 255, 0.4)' },
            { offset: 0.5, color: 'rgba(64, 158, 255, 0.2)' },
            { offset: 1, color: 'rgba(64, 158, 255, 0.05)' },
          ]),
        },
      },
      {
        name: 'Allocation Rate',
        type: 'line',
        data: allocationRate,
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        showSymbol: false,
        lineStyle: {
          width: 2.5,
          color: '#67C23A',
        },
        itemStyle: {
          color: '#67C23A',
          borderColor: '#fff',
          borderWidth: 2,
        },
        emphasis: {
          scale: true,
          focus: 'series',
        },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(103, 194, 58, 0.4)' },
            { offset: 0.5, color: 'rgba(103, 194, 58, 0.2)' },
            { offset: 1, color: 'rgba(103, 194, 58, 0.05)' },
          ]),
        },
      },
    ],
  }

  gpuChart.setOption(option, true)
}

// Watch for workspace changes
watch(
  () => store.currentWorkspaceId,
  (id) => {
    if (id) {
      fetchGPUData()
    }
  },
)

// Initialize
onMounted(() => {
  initDateRange()
  if (store.currentWorkspaceId) {
    fetchGPUData()
  }
})
</script>

<style scoped>
/* Title row: title and link side by side */
.header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: clamp(10px, 1vw, 16px) 0;
  flex-wrap: wrap;
  gap: 12px;
}

.header-links {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Lens link style */
.lens-link {
  font-size: 14px;
  color: var(--el-color-primary);
  padding: 4px 8px;
  transition: all 0.2s ease;
}

.lens-link:hover {
  color: var(--el-color-primary-light-3);
}

/* GPU chart section */
.gpu-chart-section {
  margin-top: 32px;
  margin-bottom: 24px;
}

.gpu-layout {
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: 16px;
  align-items: stretch;
}

@media (max-width: 1280px) {
  .gpu-layout {
    grid-template-columns: 1fr;
  }

  .gpu-stats-panel {
    display: grid !important;
    grid-template-columns: repeat(2, 1fr) !important;
    grid-template-rows: repeat(2, 1fr) !important;
    gap: 12px !important;
    flex-direction: unset !important;
  }

  .gpu-stat-card {
    flex: unset !important;
  }
}

.chart-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 14px;
}

.chart-filters :deep(.el-radio-group) {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  border-radius: 8px;
  overflow: hidden;
}

.chart-filters :deep(.el-radio-button__inner) {
  border: none;
  border-radius: 0;
  padding: 10px 18px;
  font-weight: 500;
  transition: all 0.3s ease;
  background: var(--el-fill-color-blank);
  color: var(--el-text-color-regular);
}

.chart-filters :deep(.el-radio-button__inner:hover) {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.chart-filters :deep(.el-radio-button:first-child .el-radio-button__inner) {
  border-radius: 8px 0 0 8px;
}

.chart-filters :deep(.el-radio-button:last-child .el-radio-button__inner) {
  border-radius: 0 8px 8px 0;
}

.chart-filters :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: var(--el-color-primary);
  color: #fff;
  box-shadow: 0 2px 6px var(--el-color-primary-light-5);
}

.chart-filters :deep(.el-date-editor) {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  border-radius: 8px;
  border: 1px solid var(--el-border-color-light);
  transition: all 0.3s ease;
}

.chart-filters :deep(.el-date-editor:hover) {
  border-color: var(--el-color-primary);
  box-shadow: 0 2px 12px rgba(64, 158, 255, 0.15);
}

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
  height: 100%;
  display: flex;
  flex-direction: column;
}

.gpu-chart-card::before {
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

.gpu-chart-card:hover {
  box-shadow:
    0 8px 24px rgba(0, 0, 0, 0.08),
    0 2px 6px rgba(0, 0, 0, 0.04),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
  transform: translateY(-2px);
}

.gpu-chart-card :deep(.el-card__body) {
  padding: clamp(16px, 1.5vw, 24px);
  background: transparent;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.gpu-chart-box {
  width: 100%;
  flex: 1;
  min-height: 320px;
  position: relative;
}

/* Right side stats panel */
.gpu-stats-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
}

.gpu-stat-card {
  flex: 1;
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.08) 0%, rgba(255, 255, 255, 0.02) 100%)
    var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  transition: all 0.3s ease;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.gpu-stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  border-color: var(--el-border-color);
}

.gpu-stat-card :deep(.el-card__body) {
  padding: 20px;
  flex: 1;
  display: flex;
  align-items: center;
  gap: 16px;
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
}

.stat-icon--primary {
  background: linear-gradient(135deg, rgba(64, 158, 255, 0.15), rgba(64, 158, 255, 0.05));
  color: #409eff;
}

.stat-icon--success {
  background: linear-gradient(135deg, rgba(103, 194, 58, 0.15), rgba(103, 194, 58, 0.05));
  color: #67c23a;
}

.stat-icon--info {
  background: linear-gradient(135deg, rgba(0, 177, 166, 0.15), rgba(0, 177, 166, 0.05));
  color: #00b1a6;
}

.stat-icon--warning {
  background: linear-gradient(135deg, rgba(230, 162, 60, 0.15), rgba(230, 162, 60, 0.05));
  color: #e6a23c;
}

.gpu-stat-card:hover .stat-icon {
  transform: scale(1.1) rotate(5deg);
}

.stat-content {
  flex: 1;
  min-width: 0;
}

.stat-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
  font-weight: 500;
}

.stat-value {
  font-size: 32px;
  font-weight: 800;
  line-height: 1;
  background-clip: text;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.stat-value--primary {
  background-image: linear-gradient(135deg, #409eff 0%, #79bbff 100%);
}

.stat-value--success {
  background-image: linear-gradient(135deg, #67c23a 0%, #95d475 100%);
}

.stat-value--info {
  background-image: linear-gradient(135deg, #00b1a6 0%, #00e5e5 100%);
}

.stat-value--warning {
  background-image: linear-gradient(135deg, #e6a23c 0%, #f3d19e 100%);
}

.stat-unit {
  font-size: 18px;
  margin-left: 2px;
}

/* Small pie chart (inside Nodes card) */
.small-pie-box {
  flex: 0 0 280px;
  max-width: 360px;
  margin-left: auto;
  aspect-ratio: 1 / 1;
  min-height: 200px;
  max-height: 280px;
}

.chart-title {
  font-weight: 600;
  color: var(--el-text-color-primary);
  font-size: calc(clamp(16px, 0.9vw + 12px, 24px) * var(--scale));
  margin: 0;
}

/* Card group styles */
/* ===== Grid container: three columns ===== */
.stat-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  grid-auto-rows: 200px;
}

/* Small screen responsive: two columns / one column */
@media (max-width: 1024px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    grid-auto-rows: 180px;
  }

  /* First card does not span rows on small screens */
  .stat-card--tall {
    grid-row: span 1;
  }

  /* Adjust first card number size */
  .stat-card--tall .stat-total__num {
    font-size: 2rem;
  }

  /* Adjust small pie chart size */
  .small-pie-box {
    flex: 0 0 140px;
    max-width: 160px;
    min-height: 120px;
    max-height: 140px;
  }
}
@media (max-width: 640px) {
  .stat-grid {
    grid-template-columns: 1fr;
    grid-auto-rows: auto;
  }

  .small-pie-box {
    display: none;
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
}

/* First card spans two rows vertically */
.stat-card--tall {
  grid-row: span 2;
  justify-content: space-between;
}
.stat-card--tall .stat-total__num {
  font-size: 2.8rem;
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
  padding: clamp(18px, 1.5vw, 32px);
}

/* ===== Header: title + large number in top right ===== */
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

  font-size: calc(clamp(15px, 0.7vw + 11px, 18px) * var(--scale));
  font-weight: 700;
  line-height: 1.4;
  color: var(--el-text-color-primary);
  letter-spacing: 0.2px;
}
.stat-total {
  white-space: nowrap;
  line-height: 1;
}
/* Prominent total in top right (gradient text) */
.stat-total__num {
  font-weight: 800;
  font-size: calc(clamp(18px, 0.7vw + 12px, 24px) * var(--scale));
  line-height: 1;

  background-image: linear-gradient(
    180deg,
    color-mix(in oklab, #00b1a6 92%, #c2edfd 8%),
    color-mix(in oklab, #00b1a6 62%, #003932 12%)
  );
  -webkit-background-clip: text;
  background-clip: text;

  -webkit-text-fill-color: transparent;
  /* Keep a very light shadow for depth effect */
  text-shadow: 0 1px 0 rgba(0, 0, 0, 0.06);
}
@container (max-width: 420px) {
  .stat-total__num {
    font-size: calc(16px * var(--scale));
  }
  .stat-title {
    font-size: calc(14px * var(--scale));
  }
}

.stat-bottom {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-top: 10px;
}
.stat-badges {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  flex: 1 1 auto;
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: calc(13px * var(--scale));
  color: var(--el-text-color-secondary);
  padding: 5px 10px;
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
  width: 11px;
  height: 11px;
  border-radius: 999px;
  display: inline-block;
}

/* ---------- Bling text ---------- */
/* Three states base colors */
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

/* Hover: switch text to solid primary color, no gradient, clearer and bolder */
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

/* Extra flair: a sweep-light effect passes through on hover */
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

/* With reduced motion preference, keep only color transitions */
@media (prefers-reduced-motion: reduce) {
  .badge--bling::after {
    transition: none;
    display: none;
  }
  .badge--bling .badge-text {
    transition: none;
  }
}
</style>
