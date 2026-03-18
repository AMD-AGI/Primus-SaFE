<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="(val: boolean) => $emit('update:visible', val)"
    title="Log Context"
    width="50%"
    top="5vh"
    :close-on-click-modal="false"
    @closed="onClosed"
    :draggable="true"
  >
    <el-table
      ref="ctxTableRef"
      :data="rows"
      v-loading="loading"
      :element-loading-text="$loadingText"
      :show-header="false"
      height="60vh"
      row-key="id"
    >
      <el-table-column>
        <template #default="{ row }">
          <!-- @vue-ignore -->
          <div
            :id="row.line === 0 ? 'ctx-target' : undefined"
            :class="row.line === 0 ? 'highlight-row' : ''"
          >
            <span class="mr-2">{{ row.line }}</span
            >{{ row.log }}
          </div>
        </template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>

<script lang="ts" setup>
import type { LogContextRow } from '@/composables/useLogTable'

defineProps<{
  visible: boolean
  loading: boolean
  rows: LogContextRow[]
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'closed'): void
}>()

const onClosed = () => {
  emit('closed')
}
</script>

<style scoped>
.highlight-row {
  font-weight: bold;
  background-color: #deffeb;
  color: #182b1c;
}
</style>
