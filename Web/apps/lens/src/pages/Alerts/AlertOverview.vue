<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { 
  alertEventsApi, 
  alertSilencesApi,
  type AlertEvent, 
  type AlertStatistics, 
  type AlertTrendPoint,
  type TopAlertSource,
  type ClusterAlertCount,
  type AlertSilence
} from '@/services/alerts'
import AlertSeverityBadge from './components/AlertSeverityBadge.vue'
import AlertStatusTag from './components/AlertStatusTag.vue'
import { useClusterStore } from '@/stores/cluster'

const router = useRouter()
const clusterStore = useClusterStore()

// State
const loading = ref(false)
const statistics = ref<AlertStatistics>({
  critical: { count: 0, change: 0 },
  high: { count: 0, change: 0 },
  warning: { count: 0, change: 0 },
  info: { count: 0, change: 0 },
})
const trendData = ref<AlertTrendPoint[]>([])
const topSources = ref<TopAlertSource[]>([])
const clusterCounts = ref<ClusterAlertCount[]>([])
const recentAlerts = ref<AlertEvent[]>([])
const activeSilences = ref<AlertSilence[]>([])

const autoRefresh = ref(true)
const refreshInterval = ref(30)
const timeRange = ref<'1h' | '6h' | '24h' | '7d'>('24h')
let refreshTimer: number | null = null

// Chart refs
const trendChartRef = ref<HTMLElement | null>(null)
const clusterChartRef = ref<HTMLElement | null>(null)
let trendChart: echarts.ECharts | null = null
let clusterChart: echarts.ECharts | null = null

const selectedCluster = computed(() => clusterStore.currentCluster || 'all')

// Fetch data
async function fetchData() {
  loading.value = true
  try {
    const clusterParam = selectedCluster.value === 'all' ? undefined : selectedCluster.value
    
    const [statsRes, trendRes, sourcesRes, clustersRes, alertsRes, silencesRes] = await Promise.all([
      alertEventsApi.getStatistics({ clusterName: clusterParam }),
      alertEventsApi.getTrend({ clusterName: clusterParam, groupBy: 'hour' }),
      alertEventsApi.getTopSources({ cluster: clusterParam, limit: 5 }),
      alertEventsApi.getByCluster({ cluster: clusterParam }),
      alertEventsApi.list({ 
        cluster: clusterParam, 
        severity: 'critical', 
        limit: 5,
        status: 'firing'
      }),
      alertSilencesApi.list({ cluster: clusterParam, enabled: true, limit: 5 })
    ])
    
    statistics.value = statsRes || statistics.value
    trendData.value = trendRes || []
    topSources.value = sourcesRes || []
    clusterCounts.value = clustersRes || []
    recentAlerts.value = alertsRes?.data || []
    activeSilences.value = silencesRes?.data || []
    
    updateCharts()
  } catch (error) {
    console.error('Failed to fetch alert overview data:', error)
  } finally {
    loading.value = false
  }
}

// Initialize charts
function initCharts() {
  if (trendChartRef.value) {
    trendChart = echarts.init(trendChartRef.value)
  }
  if (clusterChartRef.value) {
    clusterChart = echarts.init(clusterChartRef.value)
  }
  updateCharts()
}

function updateCharts() {
  updateTrendChart()
  updateClusterChart()
}

function updateTrendChart() {
  if (!trendChart || trendData.value.length === 0) return
  
  const times = trendData.value.map(p => {
    const date = new Date(p.timestamp)
    return `${date.getHours()}:00`
  })
  
  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const borderColor = isDark ? '#FFFFFF1A' : '#00000012'

  const option: echarts.EChartsOption = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' }
    },
    legend: {
      data: ['Critical', 'High', 'Warning', 'Info'],
      bottom: 0,
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
      data: times,
      axisLabel: {
        rotate: 45,
        fontSize: 10,
        color: textColor
      },
      axisLine: { lineStyle: { color: borderColor } }
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: textColor },
      axisLine: { lineStyle: { color: borderColor } },
      splitLine: { lineStyle: { color: borderColor } }
    },
    series: [
      {
        name: 'Critical',
        type: 'line',
        stack: 'Total',
        smooth: true,
        areaStyle: { opacity: 0.3 },
        lineStyle: { color: '#f56c6c' },
        itemStyle: { color: '#f56c6c' },
        data: trendData.value.map(p => p.critical)
      },
      {
        name: 'High',
        type: 'line',
        stack: 'Total',
        smooth: true,
        areaStyle: { opacity: 0.3 },
        lineStyle: { color: '#e6a23c' },
        itemStyle: { color: '#e6a23c' },
        data: trendData.value.map(p => p.high)
      },
      {
        name: 'Warning',
        type: 'line',
        stack: 'Total',
        smooth: true,
        areaStyle: { opacity: 0.3 },
        lineStyle: { color: '#f2c97d' },
        itemStyle: { color: '#f2c97d' },
        data: trendData.value.map(p => p.warning)
      },
      {
        name: 'Info',
        type: 'line',
        stack: 'Total',
        smooth: true,
        areaStyle: { opacity: 0.3 },
        lineStyle: { color: '#409eff' },
        itemStyle: { color: '#409eff' },
        data: trendData.value.map(p => p.info)
      }
    ]
  }
  
  trendChart.setOption(option)
}

function updateClusterChart() {
  if (!clusterChart || clusterCounts.value.length === 0) return
  
  const option: echarts.EChartsOption = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'value'
    },
    yAxis: {
      type: 'category',
      data: clusterCounts.value.map(c => c.clusterName)
    },
    series: [
      {
        type: 'bar',
        data: clusterCounts.value.map((c, i) => ({
          value: c.count,
          itemStyle: {
            color: ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399'][i % 5]
          }
        })),
        barWidth: '60%'
      }
    ]
  }
  
  clusterChart.setOption(option)
}

// Auto refresh
function startAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
  if (autoRefresh.value) {
    refreshTimer = window.setInterval(() => {
      fetchData()
    }, refreshInterval.value * 1000)
  }
}

function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

// Navigation
function goToAlertDetail(id: string) {
  router.push(`/alerts/events/${id}`)
}

function goToAllEvents() {
  router.push('/alerts/events')
}

function formatRelativeTime(timestamp: string) {
  const now = new Date()
  const time = new Date(timestamp)
  const diff = Math.floor((now.getTime() - time.getTime()) / 1000)
  
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

function formatSilenceRemaining(endsAt?: string) {
  if (!endsAt) return 'Permanent'
  const now = new Date()
  const end = new Date(endsAt)
  const diff = Math.floor((end.getTime() - now.getTime()) / 1000)
  
  if (diff <= 0) return 'Expired'
  if (diff < 3600) return `${Math.floor(diff / 60)}m remaining`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m remaining`
  return `${Math.floor(diff / 86400)}d remaining`
}

// Lifecycle
watch(autoRefresh, (val) => {
  if (val) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
})

watch(() => clusterStore.currentCluster, () => {
  fetchData()
})

onMounted(() => {
  fetchData()
  initCharts()
  startAutoRefresh()
  
  window.addEventListener('resize', () => {
    trendChart?.resize()
    clusterChart?.resize()
  })
})

onUnmounted(() => {
  stopAutoRefresh()
  trendChart?.dispose()
  clusterChart?.dispose()
})
</script>

<template>
  <div class="alert-overview" v-loading="loading">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">
          <el-icon class="title-icon"><Alarm /></el-icon>
          Alert Overview
        </h1>
      </div>
      <div class="header-right">
        <el-select v-model="timeRange" size="default" style="width: 120px">
          <el-option label="Last 1 Hour" value="1h" />
          <el-option label="Last 6 Hours" value="6h" />
          <el-option label="Last 24 Hours" value="24h" />
          <el-option label="Last 7 Days" value="7d" />
        </el-select>
        <el-switch 
          v-model="autoRefresh" 
          active-text="Auto" 
          inactive-text="" 
          style="margin-left: 12px"
        />
        <el-button :icon="Refresh" circle @click="fetchData" style="margin-left: 8px" />
      </div>
    </div>

    <!-- Statistics Cards -->
    <div class="stats-cards">
      <el-card class="stat-card stat-card--critical" shadow="hover">
        <div class="stat-content">
          <div class="stat-icon">
            <el-icon><CircleCloseFilled /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Critical</div>
            <div class="stat-value">{{ statistics.critical.count }}</div>
            <div class="stat-change" :class="{ 'is-up': statistics.critical.change > 0, 'is-down': statistics.critical.change < 0 }">
              <el-icon v-if="statistics.critical.change > 0"><Top /></el-icon>
              <el-icon v-else-if="statistics.critical.change < 0"><Bottom /></el-icon>
              <span>{{ Math.abs(statistics.critical.change) }} from 1h ago</span>
            </div>
          </div>
        </div>
      </el-card>

      <el-card class="stat-card stat-card--high" shadow="hover">
        <div class="stat-content">
          <div class="stat-icon">
            <el-icon><WarningFilled /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">High</div>
            <div class="stat-value">{{ statistics.high.count }}</div>
            <div class="stat-change" :class="{ 'is-up': statistics.high.change > 0, 'is-down': statistics.high.change < 0 }">
              <el-icon v-if="statistics.high.change > 0"><Top /></el-icon>
              <el-icon v-else-if="statistics.high.change < 0"><Bottom /></el-icon>
              <span>{{ Math.abs(statistics.high.change) }} from 1h ago</span>
            </div>
          </div>
        </div>
      </el-card>

      <el-card class="stat-card stat-card--warning" shadow="hover">
        <div class="stat-content">
          <div class="stat-icon">
            <el-icon><Warning /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Warning</div>
            <div class="stat-value">{{ statistics.warning.count }}</div>
            <div class="stat-change" :class="{ 'is-up': statistics.warning.change > 0, 'is-down': statistics.warning.change < 0 }">
              <el-icon v-if="statistics.warning.change > 0"><Top /></el-icon>
              <el-icon v-else-if="statistics.warning.change < 0"><Bottom /></el-icon>
              <span>{{ Math.abs(statistics.warning.change) }} from 1h ago</span>
            </div>
          </div>
        </div>
      </el-card>

      <el-card class="stat-card stat-card--info" shadow="hover">
        <div class="stat-content">
          <div class="stat-icon">
            <el-icon><InfoFilled /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Info</div>
            <div class="stat-value">{{ statistics.info.count }}</div>
            <div class="stat-change" :class="{ 'is-up': statistics.info.change > 0, 'is-down': statistics.info.change < 0 }">
              <el-icon v-if="statistics.info.change > 0"><Top /></el-icon>
              <el-icon v-else-if="statistics.info.change < 0"><Bottom /></el-icon>
              <span>{{ Math.abs(statistics.info.change) }} from 1h ago</span>
            </div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Charts Row -->
    <div class="charts-row">
      <el-card class="chart-card trend-chart" shadow="hover">
        <template #header>
          <span class="card-title">Alert Trend (Last 24 Hours)</span>
        </template>
        <div ref="trendChartRef" class="chart-container"></div>
      </el-card>

      <el-card class="chart-card top-sources" shadow="hover">
        <template #header>
          <span class="card-title">Top Alert Sources</span>
        </template>
        <div class="sources-list">
          <div 
            v-for="(source, index) in topSources" 
            :key="source.alertName"
            class="source-item"
          >
            <span class="source-rank">{{ index + 1 }}.</span>
            <span class="source-name">{{ source.alertName }}</span>
            <span class="source-count">({{ source.count }})</span>
          </div>
          <el-empty v-if="topSources.length === 0" description="No alerts" :image-size="60" />
        </div>
      </el-card>
    </div>

    <!-- Recent Alerts -->
    <el-card class="recent-alerts-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <span class="card-title">Recent Critical & High Alerts</span>
          <el-button type="primary" text @click="goToAllEvents">
            View All
            <el-icon><ArrowRight /></el-icon>
          </el-button>
        </div>
      </template>
      <el-table :data="recentAlerts" stripe>
        <el-table-column prop="severity" label="Severity" width="120">
          <template #default="{ row }">
            <AlertSeverityBadge :severity="row.severity" size="small" />
          </template>
        </el-table-column>
        <el-table-column prop="alertName" label="Alert Name" min-width="200">
          <template #default="{ row }">
            <div class="alert-name-cell">
              <span class="alert-name">{{ row.alertName }}</span>
              <span class="alert-summary">{{ row.annotations?.summary }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="Status" width="120">
          <template #default="{ row }">
            <AlertStatusTag :status="row.status" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="Resource" min-width="180">
          <template #default="{ row }">
            <div class="resource-cell">
              <span>{{ row.podName || row.nodeName || '-' }}</span>
              <span class="cluster-name">{{ row.clusterName }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="startsAt" label="Time" width="120">
          <template #default="{ row }">
            {{ formatRelativeTime(row.startsAt) }}
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="100" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" text size="small" @click="goToAlertDetail(row.id)">
              Details
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="recentAlerts.length === 0" description="No recent alerts" />
    </el-card>

    <!-- Bottom Row -->
    <div class="bottom-row">
      <el-card class="cluster-chart-card" shadow="hover">
        <template #header>
          <span class="card-title">Alerts by Cluster</span>
        </template>
        <div ref="clusterChartRef" class="chart-container"></div>
      </el-card>

      <el-card class="silences-card" shadow="hover">
        <template #header>
          <div class="card-header">
            <span class="card-title">Active Silences</span>
            <el-button type="primary" text @click="$router.push('/alerts/silences')">
              Manage
              <el-icon><ArrowRight /></el-icon>
            </el-button>
          </div>
        </template>
        <div class="silences-list">
          <div 
            v-for="silence in activeSilences" 
            :key="silence.id"
            class="silence-item"
          >
            <div class="silence-header">
              <el-icon class="silence-icon"><MuteNotification /></el-icon>
              <span class="silence-name">{{ silence.name }}</span>
            </div>
            <div class="silence-details">
              <span class="silence-cluster">{{ silence.clusterName || 'All Clusters' }}</span>
              <span class="silence-remaining">{{ formatSilenceRemaining(silence.endsAt) }}</span>
            </div>
            <div class="silence-reason">{{ silence.reason }}</div>
          </div>
          <el-empty v-if="activeSilences.length === 0" description="No active silences" :image-size="60" />
        </div>
      </el-card>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.alert-overview {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header-left {
  display: flex;
  align-items: center;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  
  .title-icon {
    color: var(--el-color-warning);
  }
}

.header-right {
  display: flex;
  align-items: center;
}

// Statistics Cards
.stats-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
  
  @media (max-width: 1200px) {
    grid-template-columns: repeat(2, 1fr);
  }
  
  @media (max-width: 768px) {
    grid-template-columns: 1fr;
  }
}

.stat-card {
  border-radius: 12px;
  overflow: hidden;
  
  &--critical {
    border-left: 4px solid #f56c6c;
    .stat-icon { color: #f56c6c; background: rgba(245, 108, 108, 0.1); }
    .stat-value { color: #f56c6c; }
  }
  
  &--high {
    border-left: 4px solid #e6a23c;
    .stat-icon { color: #e6a23c; background: rgba(230, 162, 60, 0.1); }
    .stat-value { color: #e6a23c; }
  }
  
  &--warning {
    border-left: 4px solid #f2c97d;
    .stat-icon { color: #f2c97d; background: rgba(242, 201, 125, 0.1); }
    .stat-value { color: #f2c97d; }
  }
  
  &--info {
    border-left: 4px solid #409eff;
    .stat-icon { color: #409eff; background: rgba(64, 158, 255, 0.1); }
    .stat-value { color: #409eff; }
  }
}

.stat-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-icon {
  width: 56px;
  height: 56px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
}

.stat-info {
  flex: 1;
}

.stat-label {
  font-size: 14px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  line-height: 1.2;
}

.stat-change {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  
  &.is-up {
    color: #f56c6c;
  }
  
  &.is-down {
    color: #67c23a;
  }
}

// Charts Row
.charts-row {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 16px;
  margin-bottom: 24px;
  
  @media (max-width: 1024px) {
    grid-template-columns: 1fr;
  }
}

.chart-card {
  border-radius: 12px;
}

.card-title {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.chart-container {
  height: 280px;
}

.sources-list {
  padding: 8px 0;
}

.source-item {
  display: flex;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  
  &:last-child {
    border-bottom: none;
  }
}

.source-rank {
  width: 24px;
  color: var(--el-text-color-secondary);
  font-weight: 600;
}

.source-name {
  flex: 1;
  font-weight: 500;
}

.source-count {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

// Recent Alerts
.recent-alerts-card {
  margin-bottom: 24px;
  border-radius: 12px;
}

.alert-name-cell {
  display: flex;
  flex-direction: column;
  
  .alert-name {
    font-weight: 500;
    color: var(--el-text-color-primary);
  }
  
  .alert-summary {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 2px;
  }
}

.resource-cell {
  display: flex;
  flex-direction: column;
  
  .cluster-name {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }
}

// Bottom Row
.bottom-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  
  @media (max-width: 1024px) {
    grid-template-columns: 1fr;
  }
}

.cluster-chart-card,
.silences-card {
  border-radius: 12px;
}

.silences-list {
  padding: 8px 0;
}

.silence-item {
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  
  &:last-child {
    border-bottom: none;
  }
}

.silence-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.silence-icon {
  color: var(--el-color-info);
}

.silence-name {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.silence-details {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}

.silence-remaining {
  color: var(--el-color-warning);
}

.silence-reason {
  font-size: 13px;
  color: var(--el-text-color-regular);
}
</style>
