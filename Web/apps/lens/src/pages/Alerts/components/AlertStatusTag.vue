<script setup lang="ts">
import type { AlertStatus } from '@/services/alerts/types'

defineProps<{
  status: AlertStatus
  size?: 'small' | 'default' | 'large'
}>()

const statusConfig = {
  firing: { type: 'danger' as const, icon: 'Alarm', label: 'Firing' },
  resolved: { type: 'success' as const, icon: 'CircleCheck', label: 'Resolved' },
  silenced: { type: 'info' as const, icon: 'MuteNotification', label: 'Silenced' },
}
</script>

<template>
  <el-tag
    :type="statusConfig[status]?.type || 'info'"
    :size="size || 'default'"
    effect="light"
    class="status-tag"
    :class="[`status-tag--${status}`, size && `status-tag--${size}`]"
  >
    <el-icon class="status-icon">
      <component :is="statusConfig[status]?.icon || 'QuestionFilled'" />
    </el-icon>
    <span class="status-label">{{ statusConfig[status]?.label || status }}</span>
  </el-tag>
</template>

<style lang="scss" scoped>
.status-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;
  
  &--firing {
    animation: pulse 2s infinite;
  }
  
  &--small {
    font-size: 12px;
    .status-icon {
      font-size: 12px;
    }
  }
  
  &--large {
    font-size: 14px;
    padding: 8px 12px;
    .status-icon {
      font-size: 16px;
    }
  }
}

.status-icon {
  font-size: 14px;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}
</style>
