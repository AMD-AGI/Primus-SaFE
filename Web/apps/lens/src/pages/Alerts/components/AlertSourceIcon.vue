<script setup lang="ts">
import type { AlertSource } from '@/services/alerts/types'

defineProps<{
  source: AlertSource
  showLabel?: boolean
  size?: 'small' | 'default' | 'large'
}>()

const sourceConfig = {
  metric: { icon: 'DataLine', color: '#409eff', label: 'Metric' },
  log: { icon: 'Document', color: '#67c23a', label: 'Log' },
  trace: { icon: 'Connection', color: '#e6a23c', label: 'Trace' },
}
</script>

<template>
  <span 
    class="alert-source" 
    :class="[`alert-source--${source}`, size && `alert-source--${size}`]"
  >
    <el-icon 
      class="source-icon"
      :style="{ color: sourceConfig[source]?.color }"
    >
      <component :is="sourceConfig[source]?.icon || 'QuestionFilled'" />
    </el-icon>
    <span v-if="showLabel" class="source-label">{{ sourceConfig[source]?.label || source }}</span>
  </span>
</template>

<style lang="scss" scoped>
.alert-source {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  
  &--small {
    .source-icon {
      font-size: 14px;
    }
    .source-label {
      font-size: 12px;
    }
  }
  
  &--large {
    .source-icon {
      font-size: 20px;
    }
    .source-label {
      font-size: 14px;
    }
  }
}

.source-icon {
  font-size: 16px;
}

.source-label {
  color: var(--el-text-color-regular);
  font-size: 13px;
}
</style>
