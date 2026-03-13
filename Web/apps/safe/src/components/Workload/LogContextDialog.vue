<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="(val: boolean) => $emit('update:visible', val)"
    title="Log Context"
    width="60%"
    top="5vh"
    :close-on-click-modal="false"
    @closed="onClosed"
    :draggable="true"
    class="ctx-dialog"
  >
    <div
      class="ctx-terminal"
      v-loading="loading"
      :element-loading-text="$loadingText"
      element-loading-background="rgba(10,12,16,0.8)"
    >
      <div
        v-for="row in rows"
        :key="row.id"
        :id="row.line === 0 ? 'ctx-target' : undefined"
        class="ctx-line"
        :class="[
          row.line === 0 ? 'ctx-highlight' : '',
          getLogLevelClass(row.log),
        ]"
      >
        <span class="ctx-lineno">{{ row.line }}</span>
        <span class="ctx-ts">{{ row.timeMessage }}</span>
        <span class="ctx-msg" v-html="cleanLog(row.log)"></span>
      </div>
    </div>
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

const RE_ANSI = new RegExp(String.fromCharCode(0x1b) + '\\[[0-9;]*[A-Za-z]', 'g')
const stripAnsi = (s: string) => s.replace(RE_ANSI, '')
const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
const cleanLog = (log: string) => escapeHtml(stripAnsi(log))

const getLogLevelClass = (log: string): string => {
  const clean = stripAnsi(log)
  if (/error|fatal|exception|traceback|panic|NCCL WARN/i.test(clean)) return 'ctx-error'
  if (/warn(?:ing)?/i.test(clean)) return 'ctx-warn'
  if (/training.?progress|throughput|metric/i.test(clean)) return 'ctx-metric'
  if (/cache.?flush|debug/i.test(clean)) return 'ctx-debug'
  return ''
}
</script>

<style scoped>
/* ── Dialog override ── */
:deep(.el-dialog) {
  background: #1a1d27;
  border: 1px solid #2a2e3a;
  border-radius: 8px;
}
:deep(.el-dialog__header) {
  background: #14161e;
  border-bottom: 1px solid #2a2e3a;
  padding: 12px 20px;
  margin-right: 0;
}
:deep(.el-dialog__title) {
  color: #cdd6f4;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  font-size: 14px;
}
:deep(.el-dialog__headerbtn .el-dialog__close) {
  color: #7f849c;
}
:deep(.el-dialog__body) {
  padding: 0;
}

/* ── Terminal body ── */
.ctx-terminal {
  height: 60vh;
  overflow: auto;
  padding: 4px 0;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 20px;
  background: #1a1d27;
  color: #cdd6f4;
  white-space: pre-wrap;
}

/* ── Log line ── */
.ctx-line {
  display: flex;
  align-items: flex-start;
  padding: 1px 14px;
  border-left: 3px solid transparent;
}
.ctx-line:hover {
  background: rgba(205, 214, 244, 0.06);
}

/* ── Highlighted target row ── */
.ctx-highlight {
  background: rgba(166, 227, 161, 0.1);
  border-left-color: #a6e3a1 !important;
}

/* ── Line number ── */
.ctx-lineno {
  flex-shrink: 0;
  width: 40px;
  text-align: right;
  padding-right: 12px;
  color: #45475a;
  user-select: none;
}

/* ── Timestamp ── */
.ctx-ts {
  flex-shrink: 0;
  width: 200px;
  color: #7f849c;
}

/* ── Message ── */
.ctx-msg {
  flex: 1;
  color: #bac2de;
  overflow-wrap: anywhere;
}

/* ── Log level colors ── */
.ctx-error {
  border-left-color: #f38ba8;
}
.ctx-error .ctx-msg {
  color: #f38ba8;
}
.ctx-warn {
  border-left-color: #fab387;
}
.ctx-warn .ctx-msg {
  color: #fab387;
}
.ctx-metric {
  border-left-color: #89b4fa;
}
.ctx-metric .ctx-msg {
  color: #89dceb;
}
.ctx-debug .ctx-msg {
  color: #585b70;
}

/* ── Scrollbar ── */
.ctx-terminal::-webkit-scrollbar {
  width: 8px;
}
.ctx-terminal::-webkit-scrollbar-track {
  background: transparent;
}
.ctx-terminal::-webkit-scrollbar-thumb {
  background: #45475a;
  border-radius: 4px;
}
.ctx-terminal::-webkit-scrollbar-thumb:hover {
  background: #585b70;
}
</style>
