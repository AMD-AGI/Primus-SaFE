<template>
  <div :class="['status-icon', `status-${effectiveStatus}`, `size-${size}`]">
    <el-icon v-if="effectiveStatus === 'success'" :size="iconSize"><CircleCheckFilled /></el-icon>
    <el-icon v-else-if="effectiveStatus === 'failure'" :size="iconSize"><CircleCloseFilled /></el-icon>
    <el-icon v-else-if="effectiveStatus === 'cancelled'" :size="iconSize"><RemoveFilled /></el-icon>
    <el-icon v-else-if="effectiveStatus === 'skipped'" :size="iconSize"><Remove /></el-icon>
    <div v-else-if="effectiveStatus === 'in_progress'" class="spinner">
      <svg viewBox="0 0 24 24" :width="iconSize" :height="iconSize">
        <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" fill="none" stroke-dasharray="40 60" />
      </svg>
    </div>
    <el-icon v-else-if="effectiveStatus === 'queued'" :size="iconSize"><Clock /></el-icon>
    <el-icon v-else :size="iconSize"><MoreFilled /></el-icon>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheckFilled, CircleCloseFilled, RemoveFilled, Remove, Clock, MoreFilled } from '@element-plus/icons-vue'

const props = withDefaults(defineProps<{
  status?: string
  conclusion?: string
  size?: 'small' | 'default' | 'large'
}>(), {
  size: 'default'
})

const effectiveStatus = computed(() => {
  if (props.status === 'completed' && props.conclusion) {
    return props.conclusion
  }
  return props.status || 'pending'
})

const iconSize = computed(() => {
  switch (props.size) {
    case 'small': return 16
    case 'large': return 24
    default: return 20
  }
})
</script>

<style scoped lang="scss">
.status-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  
  &.status-success {
    color: var(--el-color-success);
  }
  
  &.status-failure {
    color: var(--el-color-danger);
  }
  
  &.status-cancelled, &.status-skipped {
    color: var(--el-text-color-secondary);
  }
  
  &.status-in_progress, &.status-collecting {
    color: var(--el-color-warning);
  }
  
  &.status-queued, &.status-pending {
    color: var(--el-text-color-placeholder);
  }
  
  .spinner {
    animation: spin 1s linear infinite;
    
    svg {
      display: block;
    }
  }
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
