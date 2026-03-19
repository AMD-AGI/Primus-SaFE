<template>
  <iframe
    :src="src"
    :style="`width: 100%; height: ${height}; border: 0`"
    loading="lazy"
    referrerpolicy="no-referrer"
    allow="fullscreen; clipboard-read; clipboard-write"
  />
</template>
<script setup lang="ts">
import { computed } from 'vue'
import dayjs from 'dayjs'
import { useDark } from '@vueuse/core'

const isDark = useDark()

const props = defineProps<{
  path: string
  orgId?: number | string
  datasource?: string
  /** Variable key */
  varKey?: string
  /** Variable value */
  varValue?: string
  time?: [Date, Date | 'now'] | null
  refresh?: string
  theme?: 'light' | 'dark'
  kiosk?: boolean
  height?: string
  cluster?: string
}>()

const src = computed(() => {
  const p = new URLSearchParams()
  if (props.orgId != null) p.set('orgId', String(props.orgId))
  p.set('timezone', 'browser')
  if (props.datasource) p.set('var-Datasource', props.datasource)
  if (props.varKey && props.varValue) {
    p.set(props.varKey, props.varValue)
  }
  if (props.refresh) p.set('refresh', props.refresh)
  if (props.theme) p.set('theme', props.theme)

  if (props.cluster) p.set('var-cluster', props.cluster) // workload-detail - additional parameter

  p.set('theme', isDark.value ? 'dark' : 'light')

  if (props.time && props.time[0] && props.time[1]) {
    p.set('from', String(dayjs(props.time[0]).valueOf()))
    p.set('to', props.time[1] === 'now' ? 'now' : String(dayjs(props.time[1]).valueOf()))
  } else {
    p.set('from', 'now-12h')
    p.set('to', 'now')
  }

  let url = `${props.path}?${p.toString()}`
  if (props.kiosk) {
    url += '&kiosk'
  }
  return url
})

const height = computed(() => props.height || '600px')
</script>
