<template>
  <div class="safe-kpi">
    <div class="min-w-0">
      <div class="safe-kpi__label">{{ label }}</div>
      <div class="safe-kpi__value">
        <slot>{{ value }}</slot>
        <span v-if="suffix" class="safe-kpi__suffix">{{ suffix }}</span>
      </div>
    </div>
    <div class="safe-kpi__icon" v-if="icon">
      <el-icon><component :is="icon" /></el-icon>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
defineProps<{
  label: string
  value?: string | number
  suffix?: string
  icon?: Component
}>()
</script>

<style scoped>
.safe-kpi {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-radius: 12px;
  background: var(--safe-card);
  border: 1px solid var(--safe-border);
  /* box-shadow:
    0 0 0 1px rgba(253, 253, 253, 0.04) inset,
    0 8px 24px rgba(208, 207, 207, 0.45); */
  min-height: 64px;
}

.safe-kpi:hover {
  transform: translateY(-2px);
  box-shadow: 0 10px 32px rgba(42, 71, 138, 0.12);
}
/* .dark .safe-kpi {
  box-shadow:
    0 0 0 1px rgba(236, 232, 232, 0.03) inset,
    0 8px 32px rgba(56, 56, 56, 0.6);
} */
.safe-kpi__label {
  color: #409eff;
  font-size: 12px;
  font-weight: 600;
  margin-bottom:6px;
}
.safe-kpi__value {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.15;
}
.safe-kpi__suffix {
  font-size: 14px;
  font-weight: 500;
  margin-left: 4px;
  color: var(--safe-muted);
}
.safe-kpi__icon {
  opacity: 0.28;
  display: flex;
  align-items: center;
}
.safe-kpi__icon .el-icon {
  font-size: 18px;
}
</style>
