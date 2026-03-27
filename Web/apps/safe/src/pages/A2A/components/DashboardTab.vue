<template>
  <!-- Stats Cards -->
  <div class="summary-grid mt-4">
    <div v-for="s in statsCards" :key="s.label" class="summary-card">
      <div class="summary-icon" :style="{ color: s.color }">
        <el-icon :size="18"><component :is="s.icon" /></el-icon>
      </div>
      <div class="summary-body">
        <div class="summary-value">{{ s.value }}</div>
        <div class="summary-label">{{ s.label }}</div>
      </div>
    </div>
  </div>

  <!-- Topology + Call Volume (24h) -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
    <!-- Topology Graph -->
    <el-card class="safe-card" shadow="never" v-loading="topoLoading">
      <template #header>
        <span class="font-500">Agent Topology</span>
      </template>
      <div ref="topoRef" style="width: 100%; height: 360px" />
      <div v-if="!topoLoading && !topoData.nodes.length" class="chart-empty">
        <el-empty description="No topology data" :image-size="60" />
      </div>
    </el-card>

    <!-- Call Volume (24h) / Status Distribution fallback -->
    <el-card class="safe-card" shadow="never" v-loading="loading">
      <template #header>
        <span class="font-500">{{ hasTimeLogs ? 'Call Volume (24h)' : 'Status Distribution' }}</span>
      </template>
      <div ref="volumeRef" style="width: 100%; height: 360px" />
      <div v-if="!loading && !callLogs.length" class="chart-empty">
        <el-empty description="No call data" :image-size="60" />
      </div>
    </el-card>
  </div>

  <!-- Skill Usage + Recent Invocations -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
    <!-- Skill Usage / Target Distribution fallback -->
    <el-card class="safe-card" shadow="never" v-loading="loading">
      <template #header>
        <span class="font-500">{{ hasSkillLogs ? 'Skill Usage' : 'Target Distribution' }}</span>
      </template>
      <div ref="skillRef" style="width: 100%; height: 320px" />
      <div v-if="!loading && !callLogs.length" class="chart-empty">
        <el-empty description="No call data" :image-size="60" />
      </div>
    </el-card>

    <!-- Recent Invocations -->
    <el-card class="safe-card" shadow="never" v-loading="loading">
      <template #header>
        <span class="font-500">Recent Invocations</span>
      </template>
      <div class="invocation-list">
        <div v-for="log in recentLogs" :key="log.id" class="invocation-item">
          <div class="invocation-flow">
            <span class="invocation-name">{{ log.callerServiceName }}</span>
            <el-icon :size="12" class="invocation-arrow"><Right /></el-icon>
            <span class="invocation-name">{{ log.targetServiceName }}</span>
          </div>
          <div class="invocation-meta">
            <span class="invocation-latency">{{ log.latencyMs }}ms</span>
            <span class="invocation-dot" :class="log.status === 'success' ? 'dot--ok' : 'dot--err'" />
            <span class="invocation-status" :class="log.status === 'success' ? 'text-green' : 'text-red'">
              {{ log.status }}
            </span>
          </div>
        </div>
        <el-empty v-if="!loading && !recentLogs.length" description="No invocations" :image-size="48" />
      </div>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { getA2ATopology } from '@/services'
import type { A2AService, A2ACallLog, A2ATopologyResponse, A2ATopologyNode, A2ATopologyEdge } from '@/services'
import { Connection, Timer, SuccessFilled, Monitor, Right } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts/core'
import { GraphChart, LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([GraphChart, LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{
  services: A2AService[]
  callLogs: A2ACallLog[]
  loading: boolean
}>()

const hasTimeLogs = computed(() => props.callLogs.some((l) => l.createdAt))
const hasSkillLogs = computed(() => props.callLogs.some((l) => l.skillId))

// ── Stats ──
const statsCards = computed(() => {
  const logs = props.callLogs
  const total = logs.length
  const successCount = logs.filter((l) => l.status === 'success').length
  const rate = total > 0 ? ((successCount / total) * 100).toFixed(1) + '%' : '-'
  const avgLatency =
    total > 0
      ? Math.round(logs.reduce((sum, l) => sum + (l.latencyMs || 0), 0) / total) + 'ms'
      : '-'
  const activeAgents = props.services.filter((s) => s.status === 'active').length

  return [
    { label: 'Request Count', value: String(total), icon: Connection, color: '#6b9eff' },
    { label: 'Success Rate', value: rate, icon: SuccessFilled, color: '#5ec6a0' },
    { label: 'Avg Latency', value: avgLatency, icon: Timer, color: '#e8b160' },
    { label: 'Active Agents', value: String(activeAgents), icon: Monitor, color: '#a78bdb' },
  ]
})

// ── Recent Logs ──
const recentLogs = computed(() => props.callLogs.slice(0, 10))

// ── Topology ──
const topoRef = ref<HTMLDivElement | null>(null)
const topoLoading = ref(false)
const topoData = ref<A2ATopologyResponse>({ nodes: [], edges: [] })
let topoChart: echarts.ECharts | null = null

const healthColor: Record<string, string> = {
  healthy: '#5ec6a0',
  unhealthy: '#e87272',
  unknown: '#9ca3af',
}

const fetchTopology = async () => {
  topoLoading.value = true
  try {
    topoData.value = await getA2ATopology()
    await nextTick()
    renderTopo()
  } catch {
    topoData.value = { nodes: [], edges: [] }
  } finally {
    topoLoading.value = false
  }
}

const renderTopo = () => {
  if (!topoRef.value || !topoData.value.nodes.length) return
  if (!topoChart) {
    topoChart = echarts.init(topoRef.value)
  }

  const isDark = document.documentElement.classList.contains('dark')

  const connectedNames = new Set<string>()
  topoData.value.edges.forEach((e: A2ATopologyEdge) => {
    connectedNames.add(e.caller)
    connectedNames.add(e.target)
  })

  const nodes = topoData.value.nodes.map((n: A2ATopologyNode) => ({
    name: n.displayName || n.serviceName,
    symbolSize: connectedNames.has(n.serviceName) ? 50 : 36,
    itemStyle: { color: healthColor[n.a2aHealth] || healthColor.unknown },
  }))

  const edges = topoData.value.edges.map((e: A2ATopologyEdge) => {
    const sourceNode = topoData.value.nodes.find((n: A2ATopologyNode) => n.serviceName === e.caller)
    const targetNode = topoData.value.nodes.find((n: A2ATopologyNode) => n.serviceName === e.target)
    return {
      source: sourceNode?.displayName || e.caller,
      target: targetNode?.displayName || e.target,
      label: {
        show: true,
        formatter: String(e.count),
        fontSize: 12,
        fontWeight: 700,
        color: '#fff',
        backgroundColor: isDark ? 'rgba(70,70,70,0.92)' : 'rgba(60,60,60,0.82)',
        borderRadius: 10,
        padding: [3, 8],
        shadowColor: 'rgba(0,0,0,0.15)',
        shadowBlur: 4,
      },
      lineStyle: {
        width: Math.min(1.5 + e.count / 5, 5),
        color: isDark ? 'rgba(160,160,160,0.35)' : 'rgba(160,160,160,0.5)',
        curveness: 0.2,
      },
    }
  })

  topoChart.setOption({
    tooltip: {
      trigger: 'item',
      formatter: (params: any) => {
        if (params.dataType === 'edge') {
          return `${params.data.source} → ${params.data.target}<br/>Calls: <b>${params.data.label.formatter}</b>`
        }
        if (params.dataType === 'node') {
          return `<b>${params.name}</b>`
        }
        return ''
      },
    },
    series: [
      {
        type: 'graph',
        layout: 'force',
        roam: true,
        draggable: true,
        force: {
          repulsion: 300,
          edgeLength: [120, 220],
          gravity: 0.1,
          layoutAnimation: true,
        },
        label: {
          show: true,
          position: 'bottom',
          fontSize: 12,
          fontWeight: 500,
          color: isDark ? '#ccc' : '#333',
          distance: 8,
        },
        edgeSymbol: ['none', 'arrow'],
        edgeSymbolSize: [0, 8],
        data: nodes,
        links: edges,
      },
    ],
  })
  topoChart.resize()
}

// ── Chart 1: Call Volume (24h) line chart, or Status Distribution bar chart fallback ──
const volumeRef = ref<HTMLDivElement | null>(null)
let volumeChart: echarts.ECharts | null = null

const renderVolumeChart = () => {
  if (!volumeRef.value || !props.callLogs.length) return
  if (!volumeChart) {
    volumeChart = echarts.init(volumeRef.value)
  }

  const isDark = document.documentElement.classList.contains('dark')
  const axisColor = isDark ? '#999' : '#888'
  const splitColor = isDark ? '#2a2a2a' : '#f0f0f0'

  if (hasTimeLogs.value) {
    const now = dayjs()
    const hours: string[] = []
    const successData: number[] = []
    const failureData: number[] = []

    for (let i = 23; i >= 0; i--) {
      const hourStart = now.subtract(i, 'hour').startOf('hour')
      const hourEnd = hourStart.add(1, 'hour')
      hours.push(hourStart.format('HH:00'))

      const hourLogs = props.callLogs.filter((l) => {
        if (!l.createdAt) return false
        const t = dayjs(l.createdAt)
        return t.isAfter(hourStart) && t.isBefore(hourEnd)
      })
      successData.push(hourLogs.filter((l) => l.status === 'success').length)
      failureData.push(hourLogs.filter((l) => l.status !== 'success').length)
    }

    volumeChart.setOption({
      tooltip: { trigger: 'axis' },
      legend: { data: ['Success', 'Failure'], textStyle: { color: axisColor, fontSize: 12 }, top: 0 },
      grid: { top: 36, right: 16, bottom: 32, left: 48, containLabel: false },
      xAxis: {
        type: 'category', data: hours, boundaryGap: false,
        axisLabel: { color: axisColor, fontSize: 11 },
        axisLine: { lineStyle: { color: isDark ? '#444' : '#ddd' } },
      },
      yAxis: {
        type: 'value', minInterval: 1,
        axisLabel: { color: axisColor, fontSize: 11 },
        splitLine: { lineStyle: { color: splitColor } },
      },
      series: [
        {
          name: 'Success', type: 'line', data: successData, smooth: true,
          symbol: 'circle', symbolSize: 4,
          lineStyle: { width: 2, color: '#7ecba1' }, itemStyle: { color: '#7ecba1' },
          areaStyle: { color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(126,203,161,0.25)' }, { offset: 1, color: 'rgba(126,203,161,0.02)' },
          ]) },
        },
        {
          name: 'Failure', type: 'line', data: failureData, smooth: true,
          symbol: 'circle', symbolSize: 4,
          lineStyle: { width: 2, color: '#e88e8e' }, itemStyle: { color: '#e88e8e' },
          areaStyle: { color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(232,142,142,0.20)' }, { offset: 1, color: 'rgba(232,142,142,0.02)' },
          ]) },
        },
      ],
    }, true)
  } else {
    const statusMap = new Map<string, number>()
    props.callLogs.forEach((l) => statusMap.set(l.status || 'unknown', (statusMap.get(l.status || 'unknown') || 0) + 1))
    const sorted = [...statusMap.entries()].sort((a, b) => b[1] - a[1])
    const colorMap: Record<string, string> = { success: '#7ecba1', error: '#e88e8e', timeout: '#e8b160', unknown: '#9ca3af' }

    volumeChart.setOption({
      tooltip: { trigger: 'axis' },
      grid: { top: 16, right: 16, bottom: 32, left: 48, containLabel: false },
      xAxis: {
        type: 'category', data: sorted.map((s) => s[0]),
        axisLabel: { color: axisColor, fontSize: 12 },
        axisLine: { lineStyle: { color: isDark ? '#444' : '#ddd' } },
      },
      yAxis: {
        type: 'value', minInterval: 1,
        axisLabel: { color: axisColor, fontSize: 11 },
        splitLine: { lineStyle: { color: splitColor } },
      },
      series: [{
        type: 'bar',
        data: sorted.map((s) => ({ value: s[1], itemStyle: { color: colorMap[s[0]] || '#7ea8e8' } })),
        barMaxWidth: 48, itemStyle: { borderRadius: [4, 4, 0, 0] },
      }],
    }, true)
  }
  volumeChart.resize()
}

// ── Chart 2: Skill Usage bar chart, or Target Distribution fallback ──
const skillRef = ref<HTMLDivElement | null>(null)
let skillChart: echarts.ECharts | null = null

const renderSkillChart = () => {
  if (!skillRef.value || !props.callLogs.length) return
  if (!skillChart) {
    skillChart = echarts.init(skillRef.value)
  }

  const isDark = document.documentElement.classList.contains('dark')
  const axisColor = isDark ? '#999' : '#888'
  const splitColor = isDark ? '#2a2a2a' : '#f0f0f0'

  const dataMap = new Map<string, number>()
  if (hasSkillLogs.value) {
    props.callLogs.forEach((l) => {
      if (l.skillId) dataMap.set(l.skillId, (dataMap.get(l.skillId) || 0) + 1)
    })
  } else {
    props.callLogs.forEach((l) => {
      if (l.targetServiceName) dataMap.set(l.targetServiceName, (dataMap.get(l.targetServiceName) || 0) + 1)
    })
  }

  const sorted = [...dataMap.entries()].sort((a, b) => b[1] - a[1]).slice(0, 10)

  skillChart.setOption({
    tooltip: { trigger: 'axis' },
    grid: { top: 8, right: 24, bottom: 32, left: 8, containLabel: true },
    xAxis: {
      type: 'value', minInterval: 1,
      axisLabel: { color: axisColor, fontSize: 11 },
      splitLine: { lineStyle: { color: splitColor } },
    },
    yAxis: {
      type: 'category',
      data: sorted.map((s) => s[0]).reverse(),
      axisLabel: { color: isDark ? '#bbb' : '#666', fontSize: 12 },
      axisLine: { show: false }, axisTick: { show: false },
    },
    series: [{
      type: 'bar',
      data: sorted.map((s) => s[1]).reverse(),
      itemStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 1, 0, [
          { offset: 0, color: '#7ea8e8' }, { offset: 1, color: '#a0c4f1' },
        ]),
        borderRadius: [0, 4, 4, 0],
      },
      barMaxWidth: 24,
    }],
  }, true)
  skillChart.resize()
}

const resizeAll = () => {
  topoChart?.resize()
  volumeChart?.resize()
  skillChart?.resize()
}

watch(
  () => [props.services, props.callLogs],
  async () => {
    await nextTick()
    renderVolumeChart()
    renderSkillChart()
  },
  { deep: true },
)

onMounted(() => {
  fetchTopology()
  window.addEventListener('resize', resizeAll)
})

onBeforeUnmount(() => {
  topoChart?.dispose()
  volumeChart?.dispose()
  skillChart?.dispose()
  window.removeEventListener('resize', resizeAll)
})
</script>

<style scoped>
.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.summary-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px;
  border-radius: 12px;
  border: 1px solid var(--el-border-color-lighter);
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.6), rgba(255, 255, 255, 0.15));
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.04);
  transition: transform 0.3s ease, box-shadow 0.3s ease;
}
.dark .summary-card {
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.06), rgba(255, 255, 255, 0.02));
}
.summary-card:hover {
  transform: perspective(600px) rotateX(-2deg) rotateY(3deg) translateY(-4px);
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.1);
}
.summary-icon {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, currentColor 12%, transparent);
}
.summary-body { min-width: 0; }
.summary-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}
.summary-label {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
@media (max-width: 768px) {
  .summary-grid { grid-template-columns: repeat(2, 1fr); }
}
.safe-card :deep(.el-card__body) {
  position: relative;
}
.chart-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.invocation-list {
  display: flex;
  flex-direction: column;
  max-height: 320px;
  overflow-y: auto;
}
.invocation-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 4px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
}
.invocation-item:last-child { border-bottom: none; }
.invocation-flow {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.invocation-name {
  font-weight: 500;
  color: var(--el-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 120px;
}
.invocation-arrow { color: var(--el-text-color-placeholder); flex-shrink: 0; }
.invocation-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  margin-left: 12px;
}
.invocation-latency {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
.invocation-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}
.dot--ok { background: #5ec6a0; }
.dot--err { background: #e88e8e; }
.invocation-status { font-size: 12px; }
.text-green { color: #5ec6a0; }
.text-red { color: #e88e8e; }
</style>
