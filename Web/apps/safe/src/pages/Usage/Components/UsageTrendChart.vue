<template>
  <div ref="chartRef" class="usage-trend-chart" :style="{ height: `${height}px` }" />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { UsagePoint } from '@/services/usage/type'
import { fmtAxisTime } from '../usageUtils'
import { USAGE_COLORS, chartTheme, useUsageChart } from '../useUsageChart'

const props = withDefaults(
  defineProps<{
    series: UsagePoint[]
    height?: number
    /** Sparkline mode: no axes, legend or tooltip. */
    compact?: boolean
  }>(),
  { height: 260, compact: false },
)

const chartRef = ref<HTMLElement | null>(null)

const spanHours = () => {
  const s = props.series
  if (s.length < 2) return 0
  return (new Date(s[s.length - 1].time).getTime() - new Date(s[0].time).getTime()) / 3600e3
}

useUsageChart(chartRef, (dark) => {
  const theme = chartTheme(dark)
  const span = spanHours()
  const times = props.series.map((p) => fmtAxisTime(p.time, span))
  const line = (name: string, color: string, data: Array<number | null>, extra = {}) => ({
    name,
    type: 'line' as const,
    showSymbol: false,
    data,
    lineStyle: { width: props.compact ? 1.5 : 2, color },
    itemStyle: { color },
    ...extra,
  })
  // A null busy value (no GPU metrics) is kept as null so the line breaks instead of dropping to 0.
  const series = [
    line(
      'Allocated',
      USAGE_COLORS.allocated,
      props.series.map((p) => p.allocated),
      {
        areaStyle: { color: USAGE_COLORS.allocated, opacity: 0.14 },
      },
    ),
    line(
      'Busy',
      USAGE_COLORS.busy,
      props.series.map((p) => p.busy),
    ),
    line(
      'Quota',
      theme.subtext,
      props.series.map((p) => p.quota),
      {
        lineStyle: { width: 1.2, color: theme.subtext, type: 'dashed' },
      },
    ),
  ]

  if (props.compact) {
    return {
      animation: false,
      grid: { left: 0, right: 0, top: 4, bottom: 0 },
      xAxis: { type: 'category', show: false, boundaryGap: false, data: times },
      yAxis: { type: 'value', show: false, min: 0 },
      series: series.filter((s) => s.name !== 'Quota'),
    }
  }

  return {
    animation: false,
    tooltip: { trigger: 'axis', confine: true },
    legend: { data: ['Quota', 'Allocated', 'Busy'], top: 0, textStyle: { color: theme.text } },
    grid: { left: '2%', right: '2%', top: 32, bottom: 8, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: times,
      axisTick: { show: false },
      axisLine: { lineStyle: { color: theme.axis } },
      axisLabel: { color: theme.subtext, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      min: 0,
      name: 'GPUs',
      nameTextStyle: { color: theme.text },
      axisLabel: { color: theme.subtext },
      splitLine: { lineStyle: { color: theme.grid } },
    },
    series,
  }
})
</script>

<style scoped>
.usage-trend-chart {
  width: 100%;
}
</style>
