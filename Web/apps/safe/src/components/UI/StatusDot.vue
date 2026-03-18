<template>
  <div class="flex items-center gap-2">
    <span class="text-[14px]" v-if="isLabel">{{ label }}</span>
    <span class="inline-block w-2.5 h-2.5 rounded-full" :style="{ background: color }"></span>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    type?: 'success' | 'error' | 'warning' | 'info' | 'default'
    text?: string
    isLabel?: boolean
  }>(),
  {
    type: 'default',
    isLabel: true,
  },
)

const map = {
  success: { color: '#67c23a', label: 'Success' },
  error: { color: '#f56c6c', label: 'Error' },
  warning: { color: '#eab308', label: 'Warning' },
  info: { color: 'var(--safe-primary)', label: 'Info' },
  default: { color: '#d1d5db', label: '—' },
} as const

const color = map[props.type].color
const label = props.text ?? map[props.type].label
</script>
