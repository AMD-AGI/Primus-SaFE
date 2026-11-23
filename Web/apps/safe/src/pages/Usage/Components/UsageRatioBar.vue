<template>
  <div class="usage-ratio">
    <span class="usage-ratio__text">{{ fmtRatio(value) }}</span>
    <el-progress
      :percentage="percent"
      :show-text="false"
      :stroke-width="6"
      :status="status"
      class="usage-ratio__bar"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { fmtRatio, type UsageTone } from '../usageUtils'

const props = defineProps<{ value: number | null; tone?: UsageTone }>()

const percent = computed(() =>
  props.value == null ? 0 : Math.min(100, Math.max(0, props.value * 100)),
)
const status = computed(() => {
  if (props.tone === 'danger') return 'exception'
  if (props.tone === 'warning') return 'warning'
  if (props.tone === 'success') return 'success'
  return undefined
})
</script>

<style scoped>
.usage-ratio {
  display: flex;
  align-items: center;
  gap: 8px;
}
.usage-ratio__text {
  min-width: 48px;
  text-align: right;
}
.usage-ratio__bar {
  flex: 1;
}
</style>
