<template>
  <el-dialog
    :model-value="visible"
    width="780"
    class="util-dialog"
    :title="undefined"
    @close="emit('update:visible', false)"
  >
    <template #header>
      <div class="flex items-center justify-between pr-2">
        <div class="text-base font-600 leading-6 whitespace-nowrap">
          24h Utilization <span class="opacity-70">(Avg)</span>
        </div>
        <el-tooltip :content="title">
          <div class="opacity-70 truncate max-w-60">{{ title || '' }}</div>
        </el-tooltip>
      </div>
    </template>

    <div
      ref="el"
      v-loading="loading"
      element-loading-text="Loading..."
      style="width: 100%; height: 340px"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

const props = defineProps<{
  visible: boolean
  loading?: boolean
  series: { x: string[]; avg: (number | null)[] }
  title?: string
}>()
const emit = defineEmits(['update:visible'])

const el = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

const render = () => {
  if (!el.value) return
  if (!chart) {
    chart = echarts.init(el.value)
    window.addEventListener('resize', resize)
  }

  const n = props.series.avg?.length ?? 0
  const step = n <= 12 ? 1 : Math.ceil(n / 12)

  const isDark =
    document.documentElement.classList.contains('dark') ||
    (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)

  // Theme color palette (brighter text/lines work better in Element Plus dark mode)
  const colorText = isDark ? '#E5EAF3' : '#303133'
  const colorSubtext = isDark ? '#C8CDD5' : '#606266'
  const colorAxis = isDark ? '#FFFFFF33' : '#00000026'
  const colorGrid = isDark ? '#FFFFFF1F' : '#00000012'
  const labelBg = isDark ? 'rgba(255,255,255,0.16)' : 'rgba(0,0,0,0.35)'
  const labelBorder = isDark ? 'rgba(255,255,255,0.28)' : 'transparent'
  const tooltipBg = isDark ? 'rgba(17,24,39,0.95)' : '#fff'
  const tooltipBorder = isDark ? 'rgba(255,255,255,0.18)' : '#ebeef5'
  const pointerColor = isDark ? '#FFFFFF3D' : '#0000003d'

  chart.setOption({
    animation: false,
    grid: { left: 56, right: 28, top: 52, bottom: 40, containLabel: true },

    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      textStyle: { color: colorText },
      valueFormatter: (v: any) => (v == null ? '-' : `${(+v).toFixed(1)}%`),
      axisPointer: {
        type: 'line',
        lineStyle: { color: pointerColor, width: 1 },
        label: {
          show: true,
          color: colorText,
          backgroundColor: labelBg,
          borderColor: labelBorder,
          borderWidth: isDark ? 1 : 0,
        },
      },
    },

    xAxis: {
      type: 'category',
      data: props.series.x,
      axisTick: { show: false },
      axisLabel: {
        color: colorSubtext,
        interval: (idx: number) => (step === 1 ? 0 : idx % step === 0),
      },
      axisLine: { lineStyle: { color: colorAxis } },
    },

    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      axisLabel: { formatter: '{value}%', color: colorSubtext },
      axisLine: { lineStyle: { color: colorAxis } },
      splitLine: { lineStyle: { color: colorGrid } },
    },

    series: [
      {
        name: 'Avg',
        type: 'line',
        symbol: 'circle',
        symbolSize: step === 1 ? 4 : 3,
        showAllSymbol: step === 1,
        data: props.series.avg,
        lineStyle: { width: 1.8 },
        areaStyle: { opacity: isDark ? 0.1 : 0.08 },
        label: {
          show: true,
          position: 'top',
          color: colorText,
          backgroundColor: labelBg,
          borderColor: labelBorder,
          borderWidth: isDark ? 1 : 0,
          borderRadius: 4,
          padding: [2, 4],
          textBorderColor: isDark ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.35)', // Improve contrast
          textBorderWidth: 2,
          formatter: (p: any) => {
            const val = p.value
            if (val == null) return ''
            if (step !== 1 && p.dataIndex % step !== 0) return ''
            return `${(+val).toFixed(0)}%`
          },
        },
        emphasis: { focus: 'series' },
      },
    ],
  })
}

const resize = () => chart?.resize()

onMounted(render)
onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  chart?.dispose()
  chart = null
})
watch(() => props.series, render, { deep: true })
watch(
  () => props.visible,
  (v) => {
    if (v) setTimeout(render, 0)
  },
)
</script>

<style scoped>
.util-dialog :deep(.el-dialog__header) {
  padding: 12px 16px 4px;
}
.util-dialog :deep(.el-dialog__body) {
  padding: 8px 16px 16px;
}
.util-dialog :deep(.el-dialog__headerbtn .el-dialog__close) {
  transform: scale(0.9);
}
</style>
