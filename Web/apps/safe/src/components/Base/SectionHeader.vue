<template>
  <div
    class="section-header"
    :class="{ 'section-header--clickable': clickable }"
    @click="clickable && emit('click')"
  >
    <div class="section-bar"></div>
    <div class="flex-1">
      <div class="section-title">{{ title }}</div>
      <div v-if="subtitle" class="section-subtitle">{{ subtitle }}</div>
    </div>
    <slot name="actions">
      <el-icon v-if="clickable" class="section-chevron" :class="{ 'is-open': expanded }">
        <ArrowRight />
      </el-icon>
    </slot>
  </div>
</template>

<script setup lang="ts">
import { ArrowRight } from '@element-plus/icons-vue'

defineProps<{
  title: string
  subtitle?: string
  clickable?: boolean
  expanded?: boolean
}>()

const emit = defineEmits<{ (e: 'click'): void }>()
</script>

<style scoped>
.section-header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 10px;
}
.section-header--clickable {
  cursor: pointer;
}
.section-bar {
  width: 4px;
  height: 18px;
  border-radius: 999px;
  margin-top: 2px;
  background-color: var(--safe-primary);
  flex-shrink: 0;
}
.section-title {
  font-size: var(--fs-subtitle);
  font-weight: 600;
  line-height: 1.2;
}
.section-subtitle {
  margin-top: 2px;
  font-size: var(--fs-caption);
  color: var(--el-text-color-secondary);
}
.section-chevron {
  margin-top: 2px;
  color: var(--el-text-color-secondary);
  transition: transform 0.2s ease;
}
.section-chevron.is-open {
  transform: rotate(90deg);
}
</style>
