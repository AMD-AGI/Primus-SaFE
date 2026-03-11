<template>
  <div class="date-range-filter">
    <el-date-picker
      v-model="dateRange"
      type="datetimerange"
      range-separator="To"
      start-placeholder="Start time"
      end-placeholder="End time"
      @change="onDateRangeChange"
    />
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue'

const emit = defineEmits<{
  (e: 'change', val: { since: string; until: string }): void
}>()

const dateRange = ref<[Date, Date] | null>(null)

const getDefaultRange = (): [Date, Date] => {
  const end = new Date()
  const start = new Date()
  start.setFullYear(start.getFullYear() - 1)
  return [start, end]
}

const emitChange = () => {
  if (dateRange.value) {
    emit('change', {
      since: dateRange.value[0].toISOString(),
      until: dateRange.value[1].toISOString(),
    })
  }
}

const onDateRangeChange = () => {
  if (dateRange.value) {
    emitChange()
  }
}

const refresh = () => {
  const current = dateRange.value
  dateRange.value = [current ? current[0] : getDefaultRange()[0], new Date()]
  emitChange()
}

onMounted(() => {
  dateRange.value = getDefaultRange()
  emitChange()
})

defineExpose({ refresh })
</script>

<style scoped>
.date-range-filter {
  width: 380px;
}
.date-range-filter :deep(.el-date-editor) {
  width: 100% !important;
}
</style>
