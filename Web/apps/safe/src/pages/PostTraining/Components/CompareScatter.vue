<template>
  <div>
    <div class="flex items-center justify-between mb-2">
      <span class="text-[13px] font-500">{{ xLabel }} vs. result</span>
      <el-select v-model="yKey" size="small" style="width: 200px">
        <el-option v-for="c in columns" :key="c.key" :label="c.label" :value="c.key" />
      </el-select>
    </div>
    <div ref="chartRef" style="height: 260px" />
    <div v-if="logX" class="text-[12px] text-[var(--safe-muted)]">
      {{ xLabel }} uses a log x axis.
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { ScatterChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useDark } from '@vueuse/core'

echarts.use([ScatterChart, GridComponent, TooltipComponent, CanvasRenderer])

/** One run of the scatter: its parameter value, results and whether it has ended. */
interface ScatterPoint {
  name: string
  color: string
  x: number
  ended: boolean
  results: Record<string, number | undefined>
}

const props = defineProps<{
  /** The field that differs, its label and whether it is a learning rate. */
  field: string
  xLabel: string
  points: ScatterPoint[]
  /** The result columns to choose from, the default first. */
  columns: { key: string; label: string }[]
}>()

const isDark = useDark()
const chartRef = ref<HTMLDivElement>()
const yKey = ref(props.columns[0]?.key ?? '')
let chart: echarts.ECharts | null = null

/** Learning rates, and values spanning ten times or more, are drawn on a log axis; zero or less never is. */
const logX = computed(() => {
  const xs = props.points.map((p) => p.x)
  if (!xs.length || Math.min(...xs) <= 0) return false
  return /lr$|Lr$/i.test(props.field) || Math.max(...xs) >= 10 * Math.min(...xs)
})

const render = () => {
  if (!chart) return
  const axisColor = isDark.value ? '#9aa7bd' : '#6b7280'
  const label = props.columns.find((c) => c.key === yKey.value)?.label ?? ''
  chart.setOption(
    {
      grid: { left: 56, right: 16, top: 16, bottom: 32 },
      tooltip: {
        trigger: 'item',
        formatter: (p: { data: { name: string; value: [number, number] } }) =>
          `${p.data.name}<br/>${props.xLabel}: ${p.data.value[0]}<br/>${label}: ${p.data.value[1]}`,
      },
      xAxis: { type: logX.value ? 'log' : 'value', scale: true, axisLabel: { color: axisColor } },
      yAxis: { type: 'value', scale: true, axisLabel: { color: axisColor } },
      series: [
        {
          type: 'scatter',
          symbolSize: 12,
          data: props.points
            .filter((p) => p.results[yKey.value] !== undefined)
            .map((p) => ({
              name: p.name,
              value: [p.x, p.results[yKey.value]],
              itemStyle: p.ended
                ? { color: p.color }
                : { color: 'transparent', borderColor: p.color, borderWidth: 2 },
            })),
        },
      ],
    },
    true,
  )
}

const resize = () => chart?.resize()

onMounted(() => {
  if (chartRef.value) chart = echarts.init(chartRef.value)
  render()
  window.addEventListener('resize', resize)
})

watch(() => [props.points, yKey.value, isDark.value], render, { deep: true })

onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  chart?.dispose()
  chart = null
})
</script>
