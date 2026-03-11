<template>
  <el-tag :type="tagType" :effect="effect" :size="size" :class="['status-badge', { 'is-running': isRunning }]">
    <span v-if="isRunning" class="running-dot"></span>
    {{ displayText }}
  </el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status?: string
  conclusion?: string
  size?: 'small' | 'default' | 'large'
  effect?: 'light' | 'dark' | 'plain'
}>()

const isRunning = computed(() => 
  props.status === 'in_progress' || props.status === 'queued' || 
  props.status === 'collecting' || props.status === 'pending'
)

const tagType = computed(() => {
  // If completed, use conclusion
  if (props.status === 'completed' && props.conclusion) {
    switch (props.conclusion) {
      case 'success': return 'success'
      case 'failure': return 'danger'
      case 'cancelled': return 'info'
      case 'skipped': return 'info'
      default: return 'info'
    }
  }
  
  switch (props.status) {
    case 'completed': return 'success'
    case 'success': return 'success'
    case 'in_progress': return 'warning'
    case 'queued': return 'info'
    case 'collecting': return 'primary'
    case 'pending': return 'info'
    case 'failed': return 'danger'
    case 'failure': return 'danger'
    case 'cancelled': return 'info'
    default: return 'info'
  }
})

const displayText = computed(() => {
  if (props.status === 'completed' && props.conclusion) {
    return props.conclusion
  }
  return props.status?.replace(/_/g, ' ') || 'unknown'
})
</script>

<style scoped lang="scss">
.status-badge {
  text-transform: capitalize;
  
  &.is-running {
    .running-dot {
      display: inline-block;
      width: 6px;
      height: 6px;
      background: currentColor;
      border-radius: 50%;
      margin-right: 6px;
      animation: blink 1s ease-in-out infinite;
    }
  }
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
