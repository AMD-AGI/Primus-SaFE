<template>
  <div ref="chartRef" class="usage-queued-chart" :style="{ height: `${height}px` }" />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { UsagePoint } from '@/services/usage/type'
import { fmtAxisTime } from '../usageUtils'
import { USAGE_COLORS, chartTheme, timeAxis, useUsageChart } from '../useUsageChart'

const props = withDefaults(defineProps<{ series: UsagePoint[]; height?: number }>(), {
  height: 200,
})

const chartRef = ref<HTMLElement | null>(null)

const REASONS = [
  { key: 'queuedQuota', label: 'Workspace quota', color: USAGE_COLORS.queuedQuota },
  { key: 'queuedUserQos', label: 'User QoS', color: USAGE_COLORS.queuedUserQos },
  { key: 'queuedExternal', label: 'External capacity', color: USAGE_COLORS.queuedExternal },
  { key: 'queuedOther', label: 'Other', color: USAGE_COLORS.queuedOther },
] as const

useUsageChart(chartRef, (dark) => {
  const theme = chartTheme(dark)
  const s = props.series
  const span =
    s.length > 1
      ? (new Date(s[s.length - 1].time).getTime() - new Date(s[0].time).getTime()) / 3600e3
      : 0
  return {
    animation: false,
    tooltip: { trigger: 'axis', confine: true },
    legend: { data: REASONS.map((r) => r.label), top: 0, textStyle: { color: theme.text } },
    grid: { left: '2%', right: '2%', top: 32, bottom: 8, containLabel: true },
    xAxis: timeAxis(
      s.map((p) => fmtAxisTime(p.time, span)),
      theme,
    ),
    yAxis: {
      type: 'value',
      min: 0,
      name: 'GPUs',
      nameTextStyle: { color: theme.text },
      axisLabel: { color: theme.subtext },
      splitLine: { lineStyle: { color: theme.grid } },
    },
    series: REASONS.map((r) => ({
      name: r.label,
      type: 'line' as const,
      stack: 'queued',
      showSymbol: false,
      data: s.map((p) => p[r.key] ?? 0),
      lineStyle: { width: 1, color: r.color },
      itemStyle: { color: r.color },
      areaStyle: { color: r.color, opacity: 0.7 },
    })),
  }
})
</script>

<style scoped>
.usage-queued-chart {
  width: 100%;
}
</style>
