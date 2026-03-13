<template>
  <LogTerminal v-bind="$props" :select-first-n="1" />
</template>

<script lang="ts" setup>
import LogTerminal from '@/components/Workload/LogTerminal.vue'

defineProps<{
  wlid: string
  dispatchCount?: number
  nodes?: string[][]
  failedNodes?: string[]
  isDownload: boolean
}>()
</script>
