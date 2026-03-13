<template>
  <LogTerminal v-bind="$props" />
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
