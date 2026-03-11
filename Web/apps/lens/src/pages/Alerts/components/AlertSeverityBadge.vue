<script setup lang="ts">
import type { AlertSeverity } from '@/services/alerts/types'

defineProps<{
  severity: AlertSeverity
  size?: 'small' | 'default' | 'large'
}>()

const severityConfig = {
  critical: { color: '#f56c6c', bgColor: '#fef0f0', icon: 'CircleCloseFilled', label: 'Critical' },
  high: { color: '#e6a23c', bgColor: '#fdf6ec', icon: 'WarningFilled', label: 'High' },
  warning: { color: '#e6a23c', bgColor: '#fdf6ec', icon: 'Warning', label: 'Warning' },
  info: { color: '#409eff', bgColor: '#ecf5ff', icon: 'InfoFilled', label: 'Info' },
}
</script>

<template>
  <el-tag
    :type="severity === 'critical' ? 'danger' : severity === 'high' ? 'warning' : severity === 'warning' ? 'warning' : 'info'"
    :size="size || 'default'"
    effect="light"
    class="severity-badge"
    :class="[`severity-badge--${severity}`, size && `severity-badge--${size}`]"
  >
    <el-icon class="severity-icon">
      <component :is="severityConfig[severity]?.icon || 'InfoFilled'" />
    </el-icon>
    <span class="severity-label">{{ severityConfig[severity]?.label || severity }}</span>
  </el-tag>
</template>

<style lang="scss" scoped>
.severity-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;
  
  &--critical {
    --el-tag-bg-color: #fef0f0;
    --el-tag-border-color: #fbc4c4;
    --el-tag-text-color: #f56c6c;
  }
  
  &--high {
    --el-tag-bg-color: #fdf6ec;
    --el-tag-border-color: #f5dab1;
    --el-tag-text-color: #e6a23c;
  }
  
  &--warning {
    --el-tag-bg-color: #fdf6ec;
    --el-tag-border-color: #f5dab1;
    --el-tag-text-color: #e6a23c;
  }
  
  &--info {
    --el-tag-bg-color: #ecf5ff;
    --el-tag-border-color: #b3d8ff;
    --el-tag-text-color: #409eff;
  }
  
  &--small {
    font-size: 12px;
    .severity-icon {
      font-size: 12px;
    }
  }
  
  &--large {
    font-size: 14px;
    padding: 8px 12px;
    .severity-icon {
      font-size: 16px;
    }
  }
}

.severity-icon {
  font-size: 14px;
}

.severity-label {
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
</style>
