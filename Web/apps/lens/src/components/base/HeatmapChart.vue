<template>
    <div :style="{ height }" ref="chartRef" />
  </template>
  
  <script setup lang="ts">
  import { ref, onMounted, onBeforeUnmount, watch, nextTick, computed } from 'vue'
  import * as echarts from 'echarts'
  import { isDark } from '@/composables/dark'
  
  type AnyRec = Record<string, any>
  
  const props = withDefaults(defineProps<{
    rawData: AnyRec[]
  
    xKey: string
    yKey: string
    valueKey: string
  
    title?: string
    unit?: string
  
    height?: string
    loading?: boolean
  
    min?: number | null
    max?: number | null
  
    colorRange?: string[]
  
    autoCompleteGrid?: boolean
  
    // default true, string needs to be false
    yIsNumber?: boolean

    xLabelRotate?: number
  
    showCellBorder?: boolean
  }>(), {
    height: '420px',
    loading: false,
    min: null,
    max: null,
    colorRange: () => ['#50a3ba', '#eac763', '#d94e5d'],
    autoCompleteGrid: true,
    yIsNumber: true,
    xLabelRotate: 0,
    showCellBorder: true,
  })
  
  const chartRef = ref<HTMLElement | null>(null)
  let chartInstance: echarts.ECharts | null = null
  
  const xCats = computed(() => {
    const set = new Set<any>()
    props.rawData?.forEach(d => set.add(d?.[props.xKey]))

    return Array.from(set).map(String).sort()
  })
  
  const yCats = computed(() => {
    const set = new Set<any>()
    props.rawData?.forEach(d => set.add(d?.[props.yKey]))
    const arr = Array.from(set)
    if (props.yIsNumber) {
      return arr
        .map(v => (typeof v === 'number' ? v : Number(v)))
        .sort((a, b) => (isNaN(a) && isNaN(b) ? 0 : isNaN(a) ? 1 : isNaN(b) ? -1 : a - b))
        .map(v => (isNaN(v as number) ? String(v) : String(v)))
    }
    return arr.map(String).sort()
  })
  
  function buildTriples() {
    const xIndex = new Map(xCats.value.map((x, i) => [x, i]))
    const yIndex = new Map(yCats.value.map((y, i) => [y, i]))
  
    const table = new Map<string, number | null>()
    for (const d of props.rawData || []) {
      const x = String(d?.[props.xKey])
      const y = String(d?.[props.yKey])
      const key = `${x}@@${y}`
      let v = d?.[props.valueKey]
      v = typeof v === 'number' ? v : (v == null ? null : Number(v))
      table.set(key, isNaN(v as number) ? null : (v as number))
    }
  
    const triples: [number, number, number | null][] = []
    if (props.autoCompleteGrid) {
      xCats.value.forEach((x, xi) => {
        yCats.value.forEach((y, yi) => {
          const key = `${x}@@${y}`
          const v = table.has(key) ? (table.get(key) ?? null) : null
          triples.push([xi, yi, v])
        })
      })
    } else {
      for (const [key, v] of table) {
        const [x, y] = key.split('@@')
        const xi = xIndex.get(x)
        const yi = yIndex.get(y)
        if (xi != null && yi != null) triples.push([xi, yi, v])
      }
    }
    return triples
  }
  
  function calcMinMax(data: [number, number, number | null][]) {
    const vals = data.map(d => d[2]).filter(v => typeof v === 'number') as number[]
    if (!vals.length) return { min: 0, max: 1 }
    let min = Math.min(...vals)
    let max = Math.max(...vals)
    if (min === max) { min -= 1; max += 1 }
    return { min, max }
  }

  function shortenName(name: string) {
    return name.length > 12 ? name.slice(0, 6) + '…' + name.slice(-4) : name;
 }
  
  function makeOption() {
    const triples = buildTriples()
    const dyn = calcMinMax(triples)
    const min = props.min ?? dyn.min
    const max = props.max ?? dyn.max
  
    const dark = isDark.value
    const textColor = dark ? '#E5EAF3' : '#303133'
    const borderColor = dark ? '#FFFFFF1A' : '#00000012'

    const titleObj = props.title
      ? { text: props.unit ? `${props.title} (${props.unit})` : props.title, textStyle: { color: textColor, textShadowColor: 'rgba(0,0,0,0.3)', textShadowBlur: 1, } }
      : undefined

    const width = chartRef.value?.clientWidth
    const isWide = width ? width >= 1200 : false
  
    return {
      backgroundColor: 'transparent',
      title: titleObj,
      tooltip: {
        trigger: 'item',
        position: 'top',
        appendToBody: true,
        formatter: (p: any) => {
          const [xi, yi, v] = p.data as [number, number, number | null]
          const xv = xCats.value[xi]
          const yv = yCats.value[yi]
          const unit = props.unit ? ` ${props.unit}` : ''
          return `node: ${xv}<br/>GPU: ${yv}<br/>value: ${v ?? '-'}${v == null ? '' : unit}`
        }
      },
      grid: { left: 60, right: 20, top: props.title ? 60 : 30, bottom: 40 },
      xAxis: {
        type: 'category',
        data: isWide ? xCats.value : xCats.value.map(shortenName),
        axisLabel: { rotate: props.xLabelRotate, overflow: 'truncate', color: textColor },
        axisLine: { lineStyle: { color: borderColor } },
        splitArea: { show: true }
      },
      yAxis: {
        type: 'category',
        data: yCats.value,
        name: props.unit ? ` ${props.unit}` : '',
        axisLabel: { color: textColor },
        axisLine: { lineStyle: { color: borderColor } },
        splitArea: { show: true }
      },
      visualMap: {
        min, max,
        calculable: true,
        orient: 'vertical',
        right: 10, top: 'middle',
        inRange: { color: props.colorRange }
      },
      series: [{
        type: 'heatmap',
        name: props.title || 'heatmap',
        data: triples,
        label: { show: false },
        itemStyle: props.showCellBorder
          ? { borderColor: 'black', borderWidth: 1 }
          : undefined,
        emphasis: { itemStyle: { borderColor: '#333', borderWidth: 1 } }
      }]
    } as echarts.EChartsCoreOption
  }
  
  function render() {
    if (!chartInstance || props.loading) return
    chartInstance.setOption(makeOption(), true)
    chartInstance.resize()
  }
  
  onMounted(() => {
    nextTick(() => {
      if (chartRef.value) {
        chartInstance = echarts.init(chartRef.value)
        render()
        const onResize = () => chartInstance && chartInstance.resize()
        window.addEventListener('resize', onResize)
        onBeforeUnmount(() => window.removeEventListener('resize', onResize))
      }
    })
  })
  
  watch(
    () => [props.rawData, props.xKey, props.yKey, props.valueKey, props.min, props.max, props.colorRange, props.loading],
    render,
    { deep: true }
  )
  watch(isDark, render)
  
  onBeforeUnmount(() => {
    chartInstance?.dispose()
  })
  </script>
  