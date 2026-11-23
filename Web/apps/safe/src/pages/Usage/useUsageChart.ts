import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import * as echarts from 'echarts'

/** Theme colors shared by the usage charts. */
export const USAGE_COLORS = {
  allocated: '#409eff',
  busy: '#47b881',
  queuedQuota: '#e6a23c',
  queuedUserQos: '#8e6bd8',
  queuedExternal: '#0d9488',
  queuedOther: '#909399',
} as const

const isDarkMode = () => document.documentElement.classList.contains('dark')

/** Text / axis / grid colors that follow the light or dark theme. */
export const chartTheme = (dark: boolean) => ({
  text: dark ? '#E5EAF3' : '#303133',
  subtext: dark ? '#C8CDD5' : '#606266',
  axis: dark ? '#FFFFFF33' : '#00000026',
  grid: dark ? '#FFFFFF1F' : '#00000012',
})

/**
 * Owns the ECharts instance of one container: creates it on mount, re-renders when the option
 * or the theme changes, resizes with the container and disposes it on unmount.
 */
export function useUsageChart(
  el: Ref<HTMLElement | null>,
  buildOption: (dark: boolean) => echarts.EChartsOption,
) {
  const dark = ref(isDarkMode())
  let chart: echarts.ECharts | null = null
  let resizeObserver: ResizeObserver | null = null
  let themeObserver: MutationObserver | null = null

  const option = computed(() => buildOption(dark.value))

  const render = () => {
    if (!el.value) return
    if (!chart) chart = echarts.init(el.value)
    chart.setOption(option.value, true)
  }

  onMounted(() => {
    render()
    if (el.value) {
      resizeObserver = new ResizeObserver(() => chart?.resize())
      resizeObserver.observe(el.value)
    }
    themeObserver = new MutationObserver(() => {
      dark.value = isDarkMode()
    })
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  })

  watch(option, render)

  onBeforeUnmount(() => {
    resizeObserver?.disconnect()
    themeObserver?.disconnect()
    chart?.dispose()
    chart = null
  })
}
