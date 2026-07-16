<template>
  <div class="section-card">
    <SectionHeader
      v-if="title"
      :title="title"
      :subtitle="subtitle"
      :clickable="clickable"
      :expanded="expanded"
      @click="emit('header-click')"
    >
      <template v-if="$slots.actions" #actions>
        <slot name="actions" />
      </template>
    </SectionHeader>
    <slot />
  </div>
</template>

<script setup lang="ts">
import SectionHeader from './SectionHeader.vue'

defineProps<{
  title?: string
  subtitle?: string
  clickable?: boolean
  expanded?: boolean
}>()

const emit = defineEmits<{ (e: 'header-click'): void }>()
</script>

<style scoped>
.section-card {
  background: var(--el-bg-color-overlay);
  border-radius: 10px;
  padding: 14px 16px 10px;
  margin-bottom: 20px;
  border: 1px solid var(--el-border-color-lighter);
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.08),
    0 1px 3px rgba(0, 0, 0, 0.04);
  transition:
    box-shadow 0.16s ease-out,
    transform 0.16s ease-out;
}
html.dark .section-card {
  border: 1px solid rgba(255, 255, 255, 0.03);
  box-shadow:
    0 12px 35px rgba(0, 0, 0, 0.55),
    0 0 0 1px rgba(0, 0, 0, 0.7);
}
.section-card:hover {
  box-shadow:
    0 4px 12px rgba(0, 0, 0, 0.12),
    0 2px 6px rgba(0, 0, 0, 0.06);
  transform: translateY(-1px);
}
html.dark .section-card:hover {
  box-shadow:
    0 14px 40px rgba(0, 0, 0, 0.55),
    0 0 1px rgba(0, 0, 0, 0.9);
}
</style>
