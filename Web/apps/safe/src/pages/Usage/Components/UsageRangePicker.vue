<template>
  <div class="usage-range-picker">
    <el-segmented v-model="model.key" :options="options" @change="onKeyChange" />
    <el-date-picker
      v-if="model.key === 'custom'"
      v-model="model.custom"
      type="datetimerange"
      range-separator="To"
      start-placeholder="Start time"
      end-placeholder="End time"
      :disabled-date="disableFuture"
      class="usage-range-picker__dates"
      @change="emit('change')"
    />
  </div>
</template>

<script setup lang="ts">
import type { UsageRangeValue } from '../usageUtils'

const model = defineModel<UsageRangeValue>({ required: true })
const emit = defineEmits<{ (e: 'change'): void }>()

const options = [
  { label: 'Past 1 Day', value: '1d' },
  { label: 'Past 7 Days', value: '7d' },
  { label: 'Past 30 Days', value: '30d' },
  { label: 'Custom', value: 'custom' },
]

const disableFuture = (d: Date) => d.getTime() > Date.now()

// A preset reloads at once; Custom waits until a date range has been picked.
const onKeyChange = (key: unknown) => {
  if (key !== 'custom' || model.value.custom) emit('change')
}
</script>

<style scoped>
.usage-range-picker {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
.usage-range-picker__dates {
  max-width: 420px;
}
</style>
