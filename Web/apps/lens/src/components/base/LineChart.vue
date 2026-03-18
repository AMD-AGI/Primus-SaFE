<template>
    <div :style="{ height }" ref="chartRef" />
  </template>
  
  <script setup lang="ts">
  import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
  import * as echarts from 'echarts'
  import { isDark } from '@/composables/dark'
  
  const props = withDefaults(defineProps<{
    labels: string[]
    series: { name: string; data: number[] }[]
    unit?: string
    height?: string
    loading?: boolean
    title?: string
    colors?: string[]
  }>(), {
    height: '300px',
    loading: false
  })
  
  const chartRef = ref<HTMLElement | null>(null)
  let chartInstance: echarts.ECharts | null = null
  
  const renderChart = () => {
    if (!chartInstance || props.loading) return

    const width = chartRef.value?.clientWidth
    const isWide = width ? width >= 1200 : false

    // Optimize x-axis label interval: dynamically adjust label count by data points
    const n = props.labels.length
    let step = 1
    if (n > 12 && n <= 24) {
      step = 2  // show ~12 labels
    } else if (n > 24 && n <= 48) {
      step = 4  // show ~6-12 labels
    } else if (n > 48 && n <= 96) {
      step = 8  // show ~6-12 labels
    } else if (n > 96 && n <= 168) {
      step = 12  // show ~8-14 labels (hours in a week)
    } else if (n > 168 && n <= 336) {
      step = 24  // show ~7-14 labels (hours in two weeks)
    } else if (n > 336) {
      step = Math.ceil(n / 10)  // show ~10 labels
    }

    const dark = isDark.value
    const textColor = dark ? '#E5EAF3' : '#303133'
    const borderColor = dark ? '#FFFFFF1A' : '#00000012'

    const option = {
      title: props.title
      ? { text: props.unit ? `${props.title} (${props.unit})` : props.title, textStyle: { color: textColor, textShadowColor: 'rgba(0,0,0,0.3)', textShadowBlur: 1, } }
        : undefined,
        tooltip: {
          trigger: 'axis',
          appendToBody: true,
          position: (point, _params, dom) => {
            const x = point[0] + 20
            const y = Math.max(0, point[1] - dom.offsetHeight - 10) 
            return [x, y]
          },
          valueFormatter: (value: number) =>
            props.unit ? `${value} ${props.unit}` : String(value)
        },
      legend: {
        data: props.series.map(s => s.name),
        textStyle: { color: textColor },
        top: props.title ? 40 : 10,
        type: isWide ? 'plain' : 'scroll',
      },
      xAxis: {
        type: 'category',
        data: props.labels,
        axisLabel: {
          interval: (idx: number) => (step === 1 ? true : idx % step === 0),
          formatter: val => val.replace(' ', '\n'),
          color: textColor,
          rotate: step > 4 ? 45 : 0,
          align: step > 4 ? 'right' : 'center'
        },
        axisLine: { lineStyle: { color: borderColor } }
      },
      yAxis: {
        type: 'value',
        name: props.series?.length ? props.unit : '',
        axisLabel: { color: textColor },
        axisLine: { lineStyle: { color: borderColor } },
        splitLine: { lineStyle: { color: borderColor } }
      },
      grid: {
        left: 40,
        right: 20,
        bottom: 40,
        top: props.title ? 100 : 70,
      },
      series: props.series.map((s, i) => ({
        name: s.name,
        type: 'line',
        smooth: true,
        data: s.data,
        areaStyle: {},
        ...(props.colors?.[i] ? { color: props.colors[i] } : {})
      }))
    }
  
    chartInstance.setOption(option)
  }
  
  onMounted(() => {
    nextTick(() => {
      if (chartRef.value) {
        chartInstance = echarts.init(chartRef.value)
        renderChart()
      }
    })
  })
  
  watch(() => [props.labels, props.series], renderChart, { deep: true })
  watch(isDark, renderChart)
  
  onBeforeUnmount(() => {
    chartInstance?.dispose()
  })
  </script>
  