<template>
  <div class="pt-panel">
    <div class="flex items-center justify-between">
      <span class="text-[13px] font-500">{{ title }}</span>
      <span v-if="nonFiniteCount" class="text-[12px] text-[var(--el-color-danger)]">
        ▲ {{ nonFiniteCount }} NaN/Inf
      </span>
    </div>
    <div ref="chartRef" :style="{ height: `${height}px` }" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  MarkPointComponent,
  TooltipComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useDark } from '@vueuse/core'
import { smoothPoints, type MetricMarkers, type MetricSeries } from '@/services/postTraining'

echarts.use([
  LineChart,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  MarkPointComponent,
  TooltipComponent,
  CanvasRenderer,
])

const props = withDefaults(
  defineProps<{
    title: string
    series: MetricSeries[]
    markers?: MetricMarkers
    /** The smoothing weight, 0 for none. */
    smoothing?: number
    height?: number
    /** One color per series instead of the single-run palette, for comparing runs. */
    colors?: string[]
    /** Draw the values on a logarithmic axis. */
    logY?: boolean
    /** A light band between a low and a high series, such as the min and max of a value. */
    band?: [MetricSeries | undefined, MetricSeries | undefined]
    /** Stack the series as areas; with rest, the part of rest they leave is stacked on top as Other. */
    stack?: boolean
    rest?: MetricSeries
    /** Divides every value, such as counts into shares of the prompts of a rollout. */
    divisor?: number
  }>(),
  {
    smoothing: 0.6,
    height: 220,
    markers: undefined,
    colors: undefined,
    logY: false,
    band: undefined,
    stack: false,
    rest: undefined,
    divisor: undefined,
  },
)

const isDark = useDark()
const chartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

const nonFiniteCount = computed(() =>
  props.series.reduce((n, s) => n + (s.nonFinite?.length ?? 0), 0),
)

/** The single-run palette: primary first, indigo second, amber for validation and evaluation series. */
const colorOf = (s: MetricSeries, i: number) => {
  if (props.colors) return props.colors[i % props.colors.length]
  if (s.name.startsWith('eval/')) return isDark.value ? '#fbbf24' : '#f59e0b'
  if (i === 0) return isDark.value ? '#00e5e5' : '#0d9488'
  return isDark.value ? '#818cf8' : '#6366f1'
}

/** The checkpoint, resume and failure lines drawn on the first series. */
const markLines = () => {
  const m = props.markers
  if (!m) return []
  const muted = isDark.value ? '#9aa7bd' : '#6b7280'
  const lines: Record<string, unknown>[] = m.checkpoints.map((cp) => ({
    xAxis: cp.step,
    lineStyle: { type: 'dashed', color: muted, opacity: cp.deleted ? 0.3 : 0.7 },
    label: { show: false },
  }))
  for (const r of m.resumes) {
    lines.push({
      xAxis: r.step,
      lineStyle: { type: 'solid', color: '#6366f1' },
      label: { formatter: `Resumed from step ${r.step}`, fontSize: 11 },
    })
  }
  if (m.failure) {
    lines.push({
      xAxis: m.failure.step,
      lineStyle: { type: 'solid', color: '#f56c6c' },
      label: { formatter: 'Failed' },
    })
  }
  return lines
}

/** The points of a series, divided by the divisor when there is one. */
const pointsOf = (s: MetricSeries): [number, number][] =>
  props.divisor ? s.points.map(([x, y]) => [x, y / props.divisor!]) : s.points

/** The band series: the low line, invisible, and the gap to the high line stacked on it as a light area. */
const bandSeries = () => {
  const [low, high] = props.band ?? []
  if (!low || !high) return []
  const highAt = new Map(pointsOf(high).map(([x, y]) => [x, y]))
  const lows = pointsOf(low).filter(([x]) => highAt.has(x))
  const color = isDark.value ? '#00e5e5' : '#0d9488'
  return [
    {
      name: low.name,
      type: 'line',
      data: lows,
      stack: 'band',
      showSymbol: false,
      lineStyle: { opacity: 0 },
    },
    {
      name: `${low.name} – ${high.name}`,
      type: 'line',
      data: lows.map(([x, y]) => [x, highAt.get(x)! - y]),
      stack: 'band',
      showSymbol: false,
      lineStyle: { opacity: 0 },
      areaStyle: { color, opacity: 0.15 },
    },
  ]
}

/** The stacked series, with Other: the part of rest the stacked series leave at each step. */
const stackSeries = () => {
  const out: Record<string, unknown>[] = props.series.map((s, i) => ({
    name: s.name,
    type: 'line',
    data: pointsOf(s),
    stack: 'total',
    showSymbol: false,
    lineStyle: { width: 1, color: colorOf(s, i) },
    itemStyle: { color: colorOf(s, i) },
    areaStyle: { opacity: 0.5 },
  }))
  if (props.rest) {
    const sums = new Map<number, number>()
    for (const s of props.series)
      for (const [x, y] of pointsOf(s)) sums.set(x, (sums.get(x) ?? 0) + y)
    out.push({
      name: 'Other',
      type: 'line',
      data: pointsOf(props.rest).map(([x, y]) => [x, Math.max(0, y - (sums.get(x) ?? 0))]),
      stack: 'total',
      showSymbol: false,
      lineStyle: { width: 1, color: '#9aa7bd' },
      itemStyle: { color: '#9aa7bd' },
      areaStyle: { opacity: 0.3 },
    })
  }
  return out
}

const render = () => {
  if (!chart) return
  const out: Record<string, unknown>[] = props.stack ? stackSeries() : bandSeries()
  ;(props.stack ? [] : props.series).forEach((s, i) => {
    const color = colorOf(s, i)
    const smoothed = props.smoothing > 0 && s.points.length > 2
    out.push({
      name: s.name,
      type: 'line',
      data: pointsOf(s),
      showSymbol: s.points.length < 3,
      lineStyle: { width: 1, color, opacity: smoothed ? 0.28 : 1 },
      itemStyle: { color },
      silent: smoothed,
      markLine: i === 0 ? { symbol: 'none', data: markLines(), animation: false } : undefined,
      markPoint:
        i === 0 && props.markers?.bestCheckpoint
          ? {
              symbol: 'pin',
              symbolSize: 28,
              itemStyle: { color: '#f59e0b' },
              data: [{ name: 'Best', coord: bestCoord(s), value: '★' }],
            }
          : undefined,
    })
    if (smoothed) {
      out.push({
        name: `${s.name} (smoothed)`,
        type: 'line',
        data: smoothPoints(pointsOf(s), props.smoothing),
        showSymbol: false,
        lineStyle: { width: 2, color },
        itemStyle: { color },
      })
    }
  })
  const axisColor = isDark.value ? '#9aa7bd' : '#6b7280'
  chart.setOption(
    {
      grid: { left: 48, right: 16, top: 24, bottom: 28 },
      tooltip: { trigger: 'axis' },
      legend: {
        show: props.series.length > 1 || !!props.rest,
        top: 0,
        textStyle: { color: axisColor, fontSize: 11 },
      },
      xAxis: { type: 'value', axisLabel: { color: axisColor }, splitLine: { show: false } },
      yAxis: props.logY
        ? { type: 'log', axisLabel: { color: axisColor } }
        : { type: 'value', scale: true, axisLabel: { color: axisColor } },
      series: out,
    },
    true,
  )
}

/** Where the best checkpoint sits on a series: its step and the series value there. */
const bestCoord = (s: MetricSeries): [number, number] => {
  const step = props.markers?.bestCheckpoint?.step ?? 0
  const near = s.points.reduce<[number, number] | undefined>(
    (best, p) => (!best || Math.abs(p[0] - step) < Math.abs(best[0] - step) ? p : best),
    undefined,
  )
  return near ? [step, near[1]] : [step, 0]
}

const resize = () => chart?.resize()

onMounted(() => {
  if (chartRef.value) chart = echarts.init(chartRef.value)
  render()
  window.addEventListener('resize', resize)
})

watch(
  () => [
    props.series,
    props.markers,
    props.smoothing,
    props.logY,
    props.band,
    props.rest,
    props.divisor,
    isDark.value,
  ],
  render,
  { deep: true },
)

onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.pt-panel {
  padding: 8px 12px;
  border: 1px solid var(--safe-border);
  border-radius: 8px;
}
</style>
