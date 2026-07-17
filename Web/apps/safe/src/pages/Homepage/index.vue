<template>
  <div class="home-layout">
    <div class="home-main">
      <div class="header-row">
    <h3 class="chart-title">Usage breakdown</h3>
    <div class="header-links">
      <!-- Hyperloom entry - shown only in production (same gating as Lens) -->
      <el-button v-if="isProd" link @click="goToHyperloom" class="lens-link">
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

  <div class="usage-dashboard">
    <div class="stat-grid">
      <el-card
        v-for="(s, i) in statCards"
        :key="s.key"
        shadow="never"
        class="stat-card safe-card"
        :class="[{ 'stat-card--tall': i === 0 }, `stat-card--tone-${s.tone}`]"
        :style="{
          '--accent': PIE_COLORS[0] || '#47b881',
          '--accent-bad': PIE_COLORS[1] || '#d65f68',
          '--accent-used': PIE_COLORS[2] || '#0d9488',
        }"
      >
        <div class="stat-header">
          <div>
            <div class="stat-kicker">{{ s.kicker }}</div>
            <div class="stat-title">{{ s.title }}</div>
          </div>
          <div class="stat-total">
            <span class="stat-total__num">{{ s.total }}</span>
          </div>
        </div>

        <div class="stat-bottom">
          <div class="stat-details">
            <div class="stat-badges">
              <span class="badge badge--state badge--ok" v-if="s.avail !== undefined">
                <i class="dot" :style="{ background: PIE_COLORS[0] || '' }"></i>
                <span class="badge-text">Available {{ s.avail }}</span>
              </span>
              <span class="badge badge--state badge--bad">
                <i class="dot" :style="{ background: PIE_COLORS[1] || '' }"></i>
                <span class="badge-text">Abnormal {{ s.abnormal }}</span>
              </span>
              <span class="badge badge--state badge--used" v-if="s.used !== undefined">
                <i class="dot" :style="{ background: PIE_COLORS[2] || '' }"></i>
                <span class="badge-text">Used {{ s.used }}</span>
              </span>
            </div>

            <div class="stat-meter" aria-hidden="true">
              <span class="stat-meter__ok" :style="{ width: s.availPercent }"></span>
              <span class="stat-meter__bad" :style="{ width: s.abnormalPercent }"></span>
              <span class="stat-meter__used" :style="{ width: s.usedPercent }"></span>
            </div>
            <div class="stat-caption" v-if="i === 0">{{ s.caption }}</div>
          </div>

          <div v-if="i === 0" class="small-pie-box" :ref="(el) => (nodePieRef = el as any)" />
        </div>
      </el-card>
    </div>
  </div>

  <section class="gpu-chart-section">
    <div class="header-row">
      <h3 class="chart-title">GPU Utilization & Allocation</h3>
      <div class="chart-filters">
        <el-segmented
          v-model="quickDateRange"
          :options="[
            { label: 'Past 1 Day', value: 1 },
            { label: 'Past 7 Days', value: 7 },
            { label: 'Past 30 Days', value: 30 },
            { label: 'Custom', value: 0 },
          ]"
          @change="onQuickDateChange"
        />
        <el-date-picker
          v-model="dateRange"
          type="datetimerange"
          range-separator="To"
          start-placeholder="Start time"
          end-placeholder="End time"
          :disabled="quickDateRange !== 0"
          class="gpu-date-picker"
          @change="onDateRangeChange"
        />
      </div>
    </div>

    <div class="gpu-layout">
      <el-card shadow="never" class="gpu-chart-card safe-card" v-loading="gpuLoading">
        <div ref="gpuChartRef" class="gpu-chart-box" />
      </el-card>

      <div class="gpu-stats-panel">
        <el-card shadow="never" class="gpu-stat-card safe-card">
          <div class="gpu-stat-icon gpu-stat-icon--info">
            <el-icon><List /></el-icon>
          </div>
          <div class="gpu-stat-copy">
            <div class="gpu-stat-label">Total Workloads</div>
            <div class="gpu-stat-value">{{ gpuStats.totalWorkloads }}</div>
          </div>
        </el-card>
        <el-card shadow="never" class="gpu-stat-card safe-card">
          <div class="gpu-stat-icon gpu-stat-icon--success">
            <el-icon><TrendCharts /></el-icon>
          </div>
          <div class="gpu-stat-copy">
            <div class="gpu-stat-label">Avg Allocation</div>
            <div class="gpu-stat-value">{{ gpuStats.avgAllocation }}<span>%</span></div>
          </div>
        </el-card>
        <el-card shadow="never" class="gpu-stat-card safe-card">
          <div class="gpu-stat-icon gpu-stat-icon--primary">
            <el-icon><Odometer /></el-icon>
          </div>
          <div class="gpu-stat-copy">
            <div class="gpu-stat-label">Avg Utilization</div>
            <div class="gpu-stat-value">{{ gpuStats.avgUtilization }}<span>%</span></div>
          </div>
        </el-card>
        <el-card shadow="never" class="gpu-stat-card safe-card">
          <div class="gpu-stat-icon gpu-stat-icon--warning">
            <el-icon><WarningFilled /></el-icon>
          </div>
          <div class="gpu-stat-copy">
            <div class="gpu-stat-label">Low Utilization</div>
            <div class="gpu-stat-value">{{ gpuStats.lowUtilization }}</div>
          </div>
        </el-card>
      </div>
    </div>
  </section>
    </div>

    <aside class="home-rail">
      <el-card shadow="never" class="rail-card safe-card">
        <div class="rail-header">
          <h3 class="chart-title">
            My Workloads
            <span v-if="myWlTotal" class="rail-count">{{ myWlTotal }}</span>
          </h3>
          <el-link
            v-if="canManageWorkloads"
            type="primary"
            class="rail-viewall"
            @click="goToWorkloads"
          >
            View all<el-icon class="ml-1"><Right /></el-icon>
          </el-link>
        </div>
        <div v-loading="myWlLoading" class="rail-list">
          <template v-if="myWorkloads.length">
            <div
              v-for="row in myWorkloads"
              :key="row.workloadId"
              class="rail-item"
              @click="goToWlDetail(row)"
            >
              <div class="rail-item__main">
                <span class="rail-item__name">{{ row.displayName || row.workloadId }}</span>
                <span class="rail-item__kind">{{ getRowKind(row) }}</span>
              </div>
              <div class="rail-item__meta">
                <el-tag size="small" :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'">
                  {{ row.phase }}
                </el-tag>
                <span
                  v-if="row.phase === 'Pending' && row.queuePosition"
                  class="rail-item__queue"
                >#{{ row.queuePosition }}</span>
              </div>
            </div>
          </template>
          <el-empty
            v-else-if="!myWlLoading"
            description="No running or pending workloads"
            :image-size="70"
          />
        </div>
        <div
          v-if="canManageWorkloads && myWlTotal > myWorkloads.length"
          class="rail-more"
          @click="goToWorkloads"
        >
          +{{ myWlTotal - myWorkloads.length }} more · View all
        </div>
      </el-card>

      <el-card v-if="quickActions.length" shadow="never" class="quick-card safe-card">
        <h3 class="chart-title quick-title">Quick Actions</h3>
        <div class="quick-actions">
          <button
            v-for="qa in quickActions"
            :key="qa.path"
            type="button"
            class="quick-action"
            @click="goToCreate(qa.path)"
          >
            <el-icon class="quick-action__icon"><component :is="qa.icon" /></el-icon>
            <span class="quick-action__label">{{ qa.label }}</span>
          </button>
        </div>
      </el-card>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount, onMounted, type Component } from 'vue'
import * as echarts from 'echarts'
import {
  List,
  Odometer,
  Right,
  TrendCharts,
  WarningFilled,
  Cpu,
  Promotion,
  Notebook,
} from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useClusterStore } from '@/stores/cluster'
import { getWorkspaceDetail } from '@/services/workspace/index'
import { getGPUAggregation, getWorkloadsList } from '@/services/workload/index'
import { WorkloadPhaseButtonType, KindPathMap, WorkloadKind } from '@/services/workload/type'
import { useUserStore } from '@/stores/user'
import { useRouter } from 'vue-router'
import type { ScopesKeys } from '@/services/base/type'
import { byte2Gi } from '@/utils/index'
import {
  buildGpuStats,
  buildGpuUsageSeries,
  formatDateWithTimezone,
  getDateRange,
  unwrapGpuAggregationRows,
  type GPUAggregationItem,
} from './gpuUsage'

// Check if in production environment
const isProd = import.meta.env.PROD

// Navigate to Lens system
const goToLens = () => {
  window.open(`${location.origin}/lens`, '_blank', 'noopener,noreferrer')
}

const goToHyperloom = () => {
  window.open(`${location.origin}/hyperloom/`, '_blank', 'noopener,noreferrer')
}

const store = useWorkspaceStore()
const clusterStore = useClusterStore()
const PIE_COLORS = ['#47b881', '#d65f68', '#0d9488'] as const

// ── Right rail: my running / pending workloads ──
const router = useRouter()
const userStore = useUserStore()
const myWorkloads = ref<any[]>([])
const myWlTotal = ref(0)
const myWlLoading = ref(false)
// Admins get a short teaser (rest is one click away via "View all"); regular
// users have no full workload page, so show more and let the list scroll.
const MY_WL_TEASER = 8
const MY_WL_MAX = 50
const getRowKind = (row: any): string => row.groupVersionKind?.kind || row.kind || ''
const goToWlDetail = (row: any) => {
  const base = KindPathMap[getRowKind(row) as WorkloadKind]
  if (base) router.push({ path: `${base}/detail`, query: { id: row.workloadId } })
}
// The unified /workload-manage page is workspace-admin only (route guard
// redirects regular users to /403), so only expose "View all" to those users.
const canManageWorkloads = computed(
  () => userStore.hasManagerAccess || store.isCurrentWorkspaceAdmin(),
)
// Carry the current user into the full list so "View all" stays scoped to me.
// Pass userName so it lands in the visible User filter box on the target page.
const goToWorkloads = () =>
  router.push({
    path: '/workload-manage',
    query: userStore.profile?.name ? { userName: userStore.profile.name } : {},
  })

// ── Right rail: quick create shortcuts ──
// `?action=create` is consumed by each list page (see useRouteAction) to open
// its create dialog on mount. Each action is gated by the current workspace
// scope, mirroring how the sidebar hides workloads the user can't access.
const quickActionDefs: Array<{ label: string; path: string; icon: Component; scope: ScopesKeys }> = [
  { label: 'New Training', path: '/training', icon: Cpu, scope: 'Train' },
  { label: 'New Inference', path: '/infer', icon: Promotion, scope: 'Infer' },
  { label: 'New Authoring', path: '/authoring', icon: Notebook, scope: 'Authoring' },
]
const quickActions = computed(() =>
  quickActionDefs.filter((a) => (store.currentScopes ?? []).includes(a.scope)),
)
const goToCreate = (path: string) => router.push({ path, query: { action: 'create' } })
const fetchMyWorkloads = async () => {
  try {
    myWlLoading.value = true
    const res: any = await getWorkloadsList({
      userId: userStore.userId,
      phase: ['Running', 'Pending'],
      offset: 0,
      limit: canManageWorkloads.value ? MY_WL_TEASER : MY_WL_MAX,
      sortBy: 'createdAt',
      order: 'desc',
    })
    myWorkloads.value = res?.items || []
    myWlTotal.value = res?.totalCount || 0
  } catch {
    myWorkloads.value = []
    myWlTotal.value = 0
  } finally {
    myWlLoading.value = false
  }
}
onMounted(fetchMyWorkloads)

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
const gpuChartRef = ref<HTMLElement | null>(null)
let gpuChart: echarts.ECharts | null = null
let chartResizeObserver: ResizeObserver | null = null
const observedChartContainers = new Set<HTMLElement>()
const gpuLoading = ref(false)
const quickDateRange = ref(7)
const dateRange = ref<[Date, Date] | null>(null)
const gpuData = ref<GPUAggregationItem[]>([])

function resizeCharts() {
  nodePieChart?.resize()
  gpuChart?.resize()
}

function observeChartContainers() {
  if (!chartResizeObserver) {
    chartResizeObserver = new ResizeObserver(() => {
      resizeCharts()
    })
  }

  ;[nodePieRef.value, gpuChartRef.value].forEach((el) => {
    if (!el || observedChartContainers.has(el)) return
    chartResizeObserver?.observe(el)
    observedChartContainers.add(el)
  })
}

function renderAllCharts() {
  // Nodes pie chart
  if (!nodePieChart && nodePieRef.value) {
    nodePieChart = echarts.init(nodePieRef.value)
  }
  nodePieChart?.setOption(buildNodePieOption(), true)
  observeChartContainers()
}

const calculateAxisInterval = (dataLength: number) => {
  if (dataLength <= 24) return 0
  if (dataLength <= 48) return 1
  if (dataLength <= 168) return Math.floor(dataLength / 12)
  return Math.floor(dataLength / 10)
}

async function renderGpuChart() {
  await nextTick()
  if (!gpuChartRef.value) return

  if (!gpuChart) {
    gpuChart = echarts.init(gpuChartRef.value)
  }
  observeChartContainers()

  if (!gpuData.value.length) {
    gpuChart.setOption(
      {
        title: {
          show: true,
          text: 'No Data',
          left: 'center',
          top: 'center',
          textStyle: { color: '#999', fontSize: 18 },
        },
      },
      true,
    )
    return
  }

  const series = buildGpuUsageSeries(gpuData.value)
  const isDark =
    document.documentElement.classList.contains('dark') ||
    (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)
  const colorText = isDark ? '#E5EAF3' : '#303133'
  const colorSubtext = isDark ? '#C8CDD5' : '#606266'
  const colorAxis = isDark ? '#FFFFFF33' : '#00000026'
  const colorGrid = isDark ? '#FFFFFF1F' : '#00000012'

  const option: echarts.EChartsOption = {
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      valueFormatter: (value) => `${value}%`,
    },
    legend: {
      data: ['Avg Utilization', 'Allocation Rate'],
      top: 12,
      textStyle: { color: colorText, fontSize: 13 },
    },
    grid: {
      left: '2%',
      right: '2%',
      bottom: '8%',
      top: '16%',
      containLabel: true,
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: series.times,
      axisTick: { show: false },
      axisLine: { lineStyle: { color: colorAxis } },
      axisLabel: {
        color: colorSubtext,
        fontSize: 12,
        rotate: 45,
        interval: calculateAxisInterval(series.times.length),
      },
    },
    yAxis: {
      type: 'value',
      name: 'Percentage (%)',
      min: 0,
      max: 100,
      nameTextStyle: { color: colorText, fontSize: 13, fontWeight: 600 },
      axisLabel: { color: colorSubtext, formatter: '{value}%' },
      splitLine: { lineStyle: { color: colorGrid } },
    },
    series: [
      {
        name: 'Avg Utilization',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: series.utilization,
        lineStyle: { width: 3, color: '#0d9488' },
        areaStyle: { color: 'rgba(13, 148, 136, 0.12)' },
      },
      {
        name: 'Allocation Rate',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: series.allocation,
        lineStyle: { width: 3, color: '#47b881' },
        areaStyle: { color: 'rgba(71, 184, 129, 0.12)' },
      },
    ],
  }

  gpuChart.setOption(option, true)
}

type StatCard = {
  key: string
  tone: string
  kicker: string
  title: string
  total: string
  avail?: string
  abnormal?: string
  used?: string
  availPercent: string
  abnormalPercent: string
  usedPercent: string
  caption: string
}
const fmt = (v: number, u: string) => (u ? `${v} ${u}` : String(v))
const toPercent = (value: number, total: number) =>
  total > 0 ? `${Math.min(Math.max((value / total) * 100, 0), 100).toFixed(2)}%` : '0%'
const toneOf = (key: string) => {
  if (key === 'amd.com/gpu') return 'compute'
  if (key === 'rdma/hca') return 'network'
  if (key === 'memory') return 'memory'
  return 'compute'
}
const statCards = computed<StatCard[]>(() => {
  const res = RES_KEYS.map<StatCard>((k, i) => {
    const unit = unitOf(k)
    const total = normalized.value.total[i] ?? 0
    const avail = normalized.value.avail[i] ?? 0
    const abnormal = normalized.value.abnormal[i] ?? 0
    const used = usedNormal.value[i] ?? 0
    return {
      key: k as string,
      tone: toneOf(k),
      kicker: 'Resource Pool',
      title: `${k}${unit ? ` (${unit})` : ''}`,
      total: fmt(total, unit),
      avail: fmt(avail, unit),
      abnormal: fmt(abnormal, unit),
      used: fmt(used, unit),
      availPercent: toPercent(avail, total),
      abnormalPercent: toPercent(abnormal, total),
      usedPercent: toPercent(used, total),
      caption: total > 0 ? 'Capacity split' : 'No capacity yet',
    }
  })

  const nodes = nodeNumbers.value
  res.unshift({
    key: 'nodes',
    tone: 'nodes',
    kicker: 'Cluster Health',
    title: 'Nodes',
    avail: String(nodes.avail),
    total: String(nodes.total),
    abnormal: String(nodes.abnormal),
    used: String(nodes.used),
    availPercent: toPercent(nodes.avail, nodes.total),
    abnormalPercent: toPercent(nodes.abnormal, nodes.total),
    usedPercent: toPercent(nodes.used, nodes.total),
    caption: nodes.total > 0 ? `${nodes.avail} nodes ready` : 'No node capacity reported',
  })

  return res
})
const gpuStats = computed(() => buildGpuStats(gpuData.value))

async function getDetail() {
  if (!store.currentWorkspaceId) return
  detailData.value = await getWorkspaceDetail(store.currentWorkspaceId)
}

async function fetchGPUData() {
  if (!dateRange.value || !store.currentWorkspaceId || !clusterStore.currentClusterId) return

  gpuLoading.value = true
  try {
    const [start, end] = dateRange.value
    const res = await getGPUAggregation({
      cluster: clusterStore.currentClusterId,
      namespace: store.currentWorkspaceId,
      start_time: formatDateWithTimezone(start),
      end_time: formatDateWithTimezone(end),
      page: 1,
      page_size: 20,
      order_by: 'time',
      order_direction: 'desc',
    })
    gpuData.value = unwrapGpuAggregationRows(res)
    await renderGpuChart()
  } finally {
    gpuLoading.value = false
  }
}

const initDateRange = () => {
  if (quickDateRange.value > 0) {
    dateRange.value = getDateRange(quickDateRange.value)
  }
}

const onQuickDateChange = (value: number | string | boolean | undefined) => {
  const days = Number(value)
  if (days > 0) {
    dateRange.value = getDateRange(days)
    fetchGPUData()
  }
}

const onDateRangeChange = () => {
  if (dateRange.value) fetchGPUData()
}

watch(
  () => store.currentWorkspaceId,
  (id) => {
    if (!id) return
    getDetail()
    fetchGPUData()
  },
  { immediate: true },
)

watch(
  () => clusterStore.currentClusterId,
  () => {
    fetchGPUData()
  },
)

onMounted(() => {
  initDateRange()
  fetchGPUData()
})

onBeforeUnmount(() => {
  if (nodePieChart) {
    nodePieChart.dispose()
    nodePieChart = null
  }
  if (gpuChart) {
    gpuChart.dispose()
    gpuChart = null
  }
  chartResizeObserver?.disconnect()
  chartResizeObserver = null
  observedChartContainers.clear()
})

watch(
  () => detailData.value,
  async (v) => {
    if (!v) return
    await nextTick()
    renderAllCharts()
  },
)
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

/* ===== Page layout: main column + right rail (large screens) ===== */
.home-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: 20px;
  align-items: start;
}
.home-main {
  min-width: 0;
}
.home-rail {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.rail-card,
.quick-card {
  display: flex;
  flex-direction: column;
}
.rail-card :deep(.el-card__body),
.quick-card :deep(.el-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.rail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.rail-viewall {
  font-size: calc(12px * var(--scale));
}
.rail-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 120px;
  flex: 1 1 auto;
  overflow-y: auto;
  /* Buffer so the first/last item's hover lift + border isn't clipped by the
     scroll container (overflow-y also clips overflow-x). */
  padding: 2px;
}
.rail-list :deep(.el-empty) {
  margin: auto 0;
}

/* ── Quick Actions card ── */
.quick-title {
  margin-bottom: 12px;
}
.quick-actions {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(80px, 1fr));
  gap: 10px;
  flex: 1;
  align-content: center;
}
.quick-action {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 14px 8px;
  border: none;
  border-radius: 10px;
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
  color: var(--el-text-color-primary);
  cursor: pointer;
  transition: color 0.15s, background 0.15s, box-shadow 0.15s;
}
.quick-action:hover {
  color: var(--safe-primary);
  background: var(--safe-primary-plain-bg);
  box-shadow: inset 0 0 0 1px var(--safe-primary-plain-border);
}
.quick-action__icon {
  font-size: calc(20px * var(--scale));
  color: var(--safe-primary);
}
.quick-action__label {
  font-size: calc(12px * var(--scale));
  font-weight: 600;
  text-align: center;
  line-height: 1.2;
}
.rail-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
  cursor: pointer;
  transition:
    box-shadow 0.2s ease,
    transform 0.2s ease;
}
.rail-item:hover {
  transform: translateY(-1px);
  box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--safe-border) 40%, var(--safe-primary) 60%);
}
.rail-item__main {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 2px;
}
.rail-item__name {
  font-weight: 600;
  font-size: calc(13px * var(--scale));
  color: var(--el-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 190px;
}
.rail-item__kind {
  font-size: calc(11px * var(--scale));
  color: var(--safe-muted);
}
.rail-item__meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.rail-item__queue {
  font-size: calc(11px * var(--scale));
  color: var(--safe-muted);
}
.rail-count {
  display: inline-block;
  margin-left: 6px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: calc(11px * var(--scale));
  font-weight: 700;
  color: var(--safe-primary);
  background: var(--safe-primary-plain-bg);
  box-shadow: inset 0 0 0 1px var(--safe-primary-plain-border);
  vertical-align: middle;
}
.rail-more {
  margin-top: 10px;
  text-align: center;
  font-size: calc(12px * var(--scale));
  color: var(--safe-primary);
  cursor: pointer;
}
.rail-more:hover {
  text-decoration: underline;
}

.gpu-chart-section {
  margin-top: 20px;
}

.chart-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.gpu-date-picker {
  max-width: 420px;
}

.gpu-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 280px;
  gap: 16px;
  align-items: stretch;
}

.gpu-chart-card {
  min-height: 390px;
}

.gpu-chart-card :deep(.el-card__body) {
  height: 100%;
  box-sizing: border-box;
  padding: 18px;
}

.gpu-chart-box {
  width: 100%;
  height: 350px;
}

.gpu-stats-panel {
  display: grid;
  grid-template-rows: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.gpu-stat-card {
  min-height: 84px;
}

.gpu-stat-card :deep(.el-card__body) {
  display: flex;
  height: 100%;
  box-sizing: border-box;
  align-items: center;
  gap: 14px;
  padding: 16px;
}

.gpu-stat-icon {
  display: inline-flex;
  width: 42px;
  height: 42px;
  flex: 0 0 42px;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  color: var(--safe-primary);
  background: var(--safe-primary-plain-bg);
  box-shadow: inset 0 0 0 1px var(--safe-primary-plain-border);
  font-size: 22px;
}

.gpu-stat-icon--success {
  color: #47b881;
}

.gpu-stat-icon--warning {
  color: #e6a23c;
}

.gpu-stat-copy {
  min-width: 0;
}

.gpu-stat-label {
  margin-bottom: 6px;
  color: var(--safe-muted);
  font-size: calc(12px * var(--scale));
  font-weight: 700;
}

.gpu-stat-value {
  color: var(--safe-primary);
  font-size: calc(26px * var(--scale));
  font-weight: 800;
  line-height: 1;
}

.gpu-stat-value span {
  margin-left: 2px;
  font-size: calc(14px * var(--scale));
}

.usage-dashboard {
  display: grid;
  gap: 16px;
}

/* Small pie chart (inside Nodes card) */
.small-pie-box {
  flex: 0 0 min(38%, 220px);
  max-width: 260px;
  margin-left: auto;
  aspect-ratio: 1 / 1;
  min-height: 160px;
  max-height: 220px;
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
  grid-auto-rows: 232px;
}

/* Wide screens: right rail fills the row height (bottom aligns with the GPU
   section) and splits My Workloads / Quick Actions in an 8:2 ratio. */
@media (min-width: 1440px) {
  .home-rail {
    align-self: stretch;
  }
  .rail-card {
    flex: 8 1 0;
    min-height: 0;
  }
  .quick-card {
    flex: 2 1 0;
    min-height: 0;
  }
}

/* Zoom tier (<1440px): compact vertical rhythm so the dashboard fits without heavy scrolling */
@media (max-width: 1439px) {
  .home-layout {
    grid-template-columns: 1fr;
  }
  .rail-card {
    position: static;
  }
  .stat-grid {
    grid-auto-rows: 188px;
  }
  .gpu-chart-card {
    min-height: 300px;
  }
  .gpu-chart-box {
    height: 260px;
  }
}

/* Small screen responsive: two columns / one column */
@media (max-width: 1024px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    grid-auto-rows: 180px;
  }

  .gpu-layout {
    grid-template-columns: 1fr;
  }

  .gpu-stats-panel {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    grid-template-rows: auto;
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

  .chart-filters {
    align-items: stretch;
    flex-direction: column;
  }

  .gpu-date-picker {
    width: 100%;
  }

  .gpu-stats-panel {
    grid-template-columns: 1fr;
  }

  .small-pie-box {
    display: none;
  }
}

/* ===== Card body ===== */
.stat-card {
  --tone: var(--safe-primary);
  position: relative;
  min-width: 0;
  border-radius: var(--safe-radius-xl);
  background: var(--safe-card);
  border: 1px solid var(--safe-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
  overflow: hidden;
  transition:
    transform 0.25s ease,
    box-shadow 0.25s ease,
    border-color 0.25s ease;

  display: flex;
  flex-direction: column;
}

.stat-card::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: linear-gradient(
    180deg,
    color-mix(in oklab, var(--safe-primary) 7%, transparent),
    transparent 54%
  );
  pointer-events: none;
}

.stat-card::after {
  display: none;
}

.stat-card--tone-nodes {
  --tone: var(--safe-primary);
}

.stat-card--tone-compute,
.stat-card--tone-network,
.stat-card--tone-memory {
  --tone: var(--safe-primary);
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
/* Fill the tall Nodes card's empty middle: vertically center an enlarged donut */
.stat-card--tall .stat-bottom {
  margin-top: 0;
  flex: 1 1 auto;
  align-items: center;
}
.stat-card--tall .small-pie-box {
  flex: 0 0 46%;
  max-width: 300px;
  min-height: 190px;
  max-height: 300px;
}

.stat-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in oklab, var(--safe-border) 55%, var(--safe-primary) 45%);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

/* el-card inner layer */
.stat-card :deep(.el-card__body) {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  overflow: hidden !important;
  padding: 18px;
}

/* ===== Header: title + large number in top right ===== */
.stat-header {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: start;
  gap: 14px;
  padding-bottom: 10px;
}

.stat-kicker {
  margin-bottom: 6px;
  color: var(--safe-muted);
  font-size: calc(10px * var(--scale));
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.stat-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  font-size: calc(clamp(14px, 0.55vw + 11px, 17px) * var(--scale));
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
  color: var(--safe-primary);
  font-weight: 800;
  font-size: calc(clamp(22px, 0.9vw + 14px, 30px) * var(--scale));
  line-height: 1;

  background-image: none;
  -webkit-background-clip: text;
  background-clip: text;

  -webkit-text-fill-color: currentColor;
  text-shadow: none;
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
  margin-top: auto;
}

.stat-details {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  flex-direction: column;
  gap: 12px;
}

.stat-badges {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  flex: 1 1 auto;
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: calc(11px * var(--scale));
  color: var(--el-text-color-secondary);
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
  white-space: nowrap;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dot {
  width: 9px;
  height: 9px;
  border-radius: 999px;
  display: inline-block;
}

.stat-meter {
  display: flex;
  width: 100%;
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
}

.stat-meter span {
  transition: width 0.25s ease;
}

.stat-meter__ok {
  background: #47b881;
}

.stat-meter__bad {
  background: #d65f68;
}

.stat-meter__used {
  background: var(--safe-primary);
}

.stat-caption {
  color: var(--el-text-color-secondary);
  font-size: calc(12px * var(--scale));
  line-height: 1.45;
}

.badge--ok {
  --c: #47b881;
}
.badge--bad {
  --c: #d65f68;
}
.badge--used {
  --c: var(--safe-primary);
}

.badge--state .badge-text {
  display: inline-block;
  font-weight: 700;
  color: var(--c);
  -webkit-text-fill-color: currentColor;
  text-shadow: none;

  transition:
    color 0.2s ease,
    opacity 0.2s ease;
}

.badge--state:hover .badge-text {
  opacity: 0.86;
}

.badge--state {
  position: relative;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
  overflow: hidden;
}
.badge--state:hover {
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
}

.badge--state::after {
  display: none;
}

/* With reduced motion preference, keep only color transitions */
@media (prefers-reduced-motion: reduce) {
  .badge--state::after {
    transition: none;
    display: none;
  }
  .badge--state .badge-text {
    transition: none;
  }
}
</style>
