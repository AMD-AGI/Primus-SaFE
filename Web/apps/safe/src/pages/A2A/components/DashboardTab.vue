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

  <!-- Topology + Call Volume -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
    <!-- Topology Graph -->
    <el-card class="safe-card" shadow="never">
      <template #header>
        <span class="font-500">Agent Topology</span>
      </template>
      <div ref="topoRef" style="width: 100%; height: 360px" v-loading="topoLoading" />
      <el-empty v-if="!topoLoading && !topoData.nodes.length" description="No topology data" :image-size="60" />
    </el-card>

    <!-- Status Distribution -->
    <el-card class="safe-card" shadow="never">
      <template #header>
        <span class="font-500">Status Distribution</span>
      </template>
      <div ref="volumeRef" style="width: 100%; height: 360px" />
      <el-empty v-if="!callLogs.length" description="No call data" :image-size="60" />
    </el-card>
  </div>

  <!-- Target Distribution + Recent Invocations -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
    <!-- Target Distribution -->
    <el-card class="safe-card" shadow="never">
      <template #header>
        <span class="font-500">Target Distribution</span>
      </template>
      <div ref="targetRef" style="width: 100%; height: 320px" />
      <el-empty v-if="!callLogs.length" description="No call data" :image-size="60" />
    </el-card>

    <!-- Recent Invocations -->
    <el-card class="safe-card" shadow="never">
      <template #header>
        <span class="font-500">Recent Invocations</span>
      </template>
      <el-table :data="recentLogs" size="small" :show-header="true" max-height="320">
        <el-table-column prop="callerServiceName" label="Caller" min-width="100" show-overflow-tooltip />
        <el-table-column prop="targetServiceName" label="Target" min-width="100" show-overflow-tooltip />
        <el-table-column label="Latency" width="90" align="right">
          <template #default="{ row }">{{ row.latencyMs }}ms</template>
        </el-table-column>
        <el-table-column label="Status" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { getA2ATopology } from '@/services'
import type { A2AService, A2ACallLog, A2ATopologyResponse } from '@/services'
import { Connection, Timer, SuccessFilled, Monitor } from '@element-plus/icons-vue'
import * as echarts from 'echarts/core'
import { GraphChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([GraphChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{
  services: A2AService[]
  callLogs: A2ACallLog[]
}>()

// ── Stats ──
const statsCards = computed(() => {
  const total = props.callLogs.length
  const successCount = props.callLogs.filter((l) => l.status === 'success').length
  const rate = total > 0 ? ((successCount / total) * 100).toFixed(1) + '%' : '-'
  const avgLatency =
    total > 0
      ? Math.round(props.callLogs.reduce((sum, l) => sum + (l.latencyMs || 0), 0) / total) + 'ms'
      : '-'
  const activeAgents = props.services.filter((s) => s.status === 'active').length

  return [
    { label: 'Total Calls', value: String(total), icon: Connection, color: '#3b82f6' },
    { label: 'Success Rate', value: rate, icon: SuccessFilled, color: '#10b981' },
    { label: 'Avg Latency', value: avgLatency, icon: Timer, color: '#f59e0b' },
    { label: 'Active Agents', value: String(activeAgents), icon: Monitor, color: '#8b5cf6' },
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
  healthy: '#10b981',
  unhealthy: '#ef4444',
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

  const nodes = topoData.value.nodes.map((n, i) => ({
    name: n.displayName || n.serviceName,
    symbolSize: 50,
    itemStyle: { color: healthColor[n.a2aHealth] || healthColor.unknown },
    x: 150 + (i % 4) * 160,
    y: 80 + Math.floor(i / 4) * 140,
  }))

  const edges = topoData.value.edges.map((e) => {
    const sourceNode = topoData.value.nodes.find((n) => n.serviceName === e.caller)
    const targetNode = topoData.value.nodes.find((n) => n.serviceName === e.target)
    return {
      source: sourceNode?.displayName || e.caller,
      target: targetNode?.displayName || e.target,
      label: { show: true, formatter: String(e.count), fontSize: 11 },
      lineStyle: { width: Math.min(1 + e.count / 5, 6) },
    }
  })

  const isDark = document.documentElement.classList.contains('dark')

  topoChart.setOption({
    tooltip: { trigger: 'item' },
    series: [
      {
        type: 'graph',
        layout: 'none',
        roam: true,
        label: {
          show: true,
          position: 'bottom',
          fontSize: 12,
          color: isDark ? '#ccc' : '#333',
        },
        edgeSymbol: ['none', 'arrow'],
        edgeSymbolSize: [0, 10],
        data: nodes,
        links: edges,
      },
    ],
  })
  topoChart.resize()
}

// ── Status Distribution ──
const volumeRef = ref<HTMLDivElement | null>(null)
let volumeChart: echarts.ECharts | null = null

const renderStatusDist = () => {
  if (!volumeRef.value || !props.callLogs.length) return
  if (!volumeChart) {
    volumeChart = echarts.init(volumeRef.value)
  }

  const statusMap = new Map<string, number>()
  props.callLogs.forEach((l) => {
    const s = l.status || 'unknown'
    statusMap.set(s, (statusMap.get(s) || 0) + 1)
  })

  const statusColorMap: Record<string, string> = {
    success: '#10b981',
    error: '#ef4444',
    timeout: '#f59e0b',
    unknown: '#9ca3af',
  }

  const sorted = [...statusMap.entries()].sort((a, b) => b[1] - a[1])
  const isDark = document.documentElement.classList.contains('dark')

  volumeChart.setOption({
    tooltip: { trigger: 'axis' },
    grid: { top: 16, right: 16, bottom: 32, left: 48, containLabel: false },
    xAxis: {
      type: 'category',
      data: sorted.map((s) => s[0]),
      axisLabel: { color: isDark ? '#aaa' : '#666', fontSize: 12 },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: isDark ? '#aaa' : '#666', fontSize: 11 },
      splitLine: { lineStyle: { color: isDark ? '#333' : '#eee' } },
    },
    series: [
      {
        type: 'bar',
        data: sorted.map((s) => ({
          value: s[1],
          itemStyle: { color: statusColorMap[s[0]] || '#409eff' },
        })),
        barMaxWidth: 48,
        itemStyle: { borderRadius: [4, 4, 0, 0] },
      },
    ],
  })
  volumeChart.resize()
}

// ── Target Distribution ──
const targetRef = ref<HTMLDivElement | null>(null)
let targetChart: echarts.ECharts | null = null

const renderTargetDist = () => {
  if (!targetRef.value || !props.callLogs.length) return
  if (!targetChart) {
    targetChart = echarts.init(targetRef.value)
  }

  const targetMap = new Map<string, number>()
  props.callLogs.forEach((l) => {
    if (l.targetServiceName) {
      targetMap.set(l.targetServiceName, (targetMap.get(l.targetServiceName) || 0) + 1)
    }
  })

  const sorted = [...targetMap.entries()].sort((a, b) => b[1] - a[1]).slice(0, 10)
  const isDark = document.documentElement.classList.contains('dark')

  targetChart.setOption({
    tooltip: { trigger: 'axis' },
    grid: { top: 16, right: 16, bottom: 40, left: 120, containLabel: false },
    xAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: isDark ? '#aaa' : '#666', fontSize: 11 },
      splitLine: { lineStyle: { color: isDark ? '#333' : '#eee' } },
    },
    yAxis: {
      type: 'category',
      data: sorted.map((s) => s[0]).reverse(),
      axisLabel: { color: isDark ? '#aaa' : '#666', fontSize: 11 },
    },
    series: [
      {
        type: 'bar',
        data: sorted.map((s) => s[1]).reverse(),
        itemStyle: { color: '#409eff', borderRadius: [0, 4, 4, 0] },
        barMaxWidth: 28,
      },
    ],
  })
  targetChart.resize()
}

const resizeAll = () => {
  topoChart?.resize()
  volumeChart?.resize()
  targetChart?.resize()
}

watch(
  () => [props.services, props.callLogs],
  async () => {
    await nextTick()
    renderStatusDist()
    renderTargetDist()
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
  targetChart?.dispose()
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
</style>
