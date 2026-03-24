<template>
  <el-row :gutter="20">
    <el-col :span="6">
      <div style="align-items: center; display: flex">
        <el-select v-model="selectedGroup" size="default" class="w-40">
          <el-option
            v-for="opt in groupOptions"
            :key="opt.value"
            :label="opt.label"
            :value="opt.value"
          />
        </el-select>
        <el-tooltip content="dispatch count" placement="top">
          <el-icon class="ml-2"><InfoFilled /></el-icon>
        </el-tooltip>
      </div>
      <el-card class="mt-4 safe-card" style="height: calc(100vh - 238px)" shadow="never">
        <el-table
          ref="nodeTableRef"
          :data="tableRows"
          row-key="node"
          stripe
          border
          v-if="tableRows?.length"
          height="calc(100vh - 280px)"
          reserve-selection
          @selection-change="doNodeSelectionChange"
        >
          <el-table-column type="selection" width="40" />
          <el-table-column prop="node" label="Node">
            <template #default="{ row }">
              <el-tooltip v-if="hasRank(row.rank)" :content="`Rank: ${row.rank}`" placement="top">
                {{ row.node }}
              </el-tooltip>
              <div v-else>{{ row.node }}</div>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else description="no data" />
      </el-card>
    </el-col>
    <el-col :span="18">
      <el-row :gutter="20">
        <el-col :span="12">
          <el-date-picker
            v-model="searchParams.dateRange"
            size="default"
            type="datetimerange"
            range-separator="To"
            start-placeholder="Start date"
            end-placeholder="End date"
            style="width: 100%"
          />
        </el-col>
        <el-col :span="6">
          <el-select
            v-model="displayOrder"
            placeholder="order"
            style="width: 100%"
            size="default"
          >
            <el-option v-for="item in ['asc', 'desc']" :key="item" :label="item" :value="item" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <el-button :icon="Search" size="default" type="primary" @click="doSearch"></el-button>
          <el-tooltip
            content="Download all logs for this workload (not just the search results)."
            placement="top"
          >
            <el-button
              v-if="isDownload"
              :icon="Download"
              :loading="downloadLoading"
              size="default"
              @click="downloadLogs"
            ></el-button>
          </el-tooltip>
        </el-col>
        <el-col :span="12" class="mt-2">
          <div class="flex items-center gap-2">
            <el-select
              v-model="searchParams.keywords"
              multiple
              clearable
              filterable
              allow-create
              default-first-option
              collapse-tags
              collapse-tags-tooltip
              :max-collapse-tags="3"
              :reserve-keyword="false"
              placeholder="Enter a single keyword (Enter to add)"
              style="width: 100%"
              size="default"
              @change="dedupeKeywords"
            >
            </el-select>
            <el-tooltip content="Multiple keywords must all match" placement="top">
              <el-icon><InfoFilled /></el-icon>
            </el-tooltip>
          </div>
        </el-col>
      </el-row>
      <div class="log-terminal-wrapper mt-4">
        <div class="log-terminal-toolbar">
          <el-switch v-model="wordWrap" size="small" active-text="Wrap" />
          <span v-if="pagination.total" class="term-stats">
            {{ loadedCount }} / {{ pagination.total }}
          </span>
        </div>
        <div
          ref="terminalRef"
          class="log-terminal"
          :class="{ 'log-terminal--nowrap': !wordWrap }"
          v-if="scrollRows.length"
          @scroll="onTerminalScroll"
        >
          <div v-if="!hasMore && lastLoadedPage > 0" class="term-end-mark">
            {{ displayOrder === 'asc' ? '── top ──' : '── end ──' }}
          </div>
          <div v-if="loadingMore" class="term-loading-bar">Loading...</div>
          <template v-for="row in scrollRows" :key="row.id">
            <div v-if="row._kind === 'header'" class="term-group-header">
              ━━━ host: {{ row.host }} │ pod: {{ row.pod_name }} │ dispatch: {{ row.count }} ━━━
            </div>
            <div
              v-else
              class="term-line"
              :class="getLogLevelClass(row.log)"
              @click="onFetchContext(row.id, row.ts, row.log)"
            >
              <span class="term-ts">{{ row.timeMessage }}</span>
              <span class="term-msg" v-html="highlightLog(row.log)"></span>
            </div>
          </template>
        </div>
        <div
          class="log-terminal log-terminal--empty"
          v-loading="initialLoading"
          :element-loading-text="$loadingText"
          element-loading-background="rgba(10,12,16,0.8)"
          v-else
        >
          <span style="color: #585b70">Click the Search button to view results</span>
        </div>
      </div>
    </el-col>
  </el-row>

  <LogContextDialog
    :visible="ctxVisible"
    :loading="ctxLoading"
    :rows="ctxRows"
    @update:visible="(val) => (ctxVisible = val)"
    @closed="onCtxClosed"
  />
</template>

<script lang="ts" setup>
import { computed, nextTick, ref, toRef, watch } from 'vue'
import { InfoFilled, Search, Download } from '@element-plus/icons-vue'
import { useLogTable, type RowData } from '@/composables/useLogTable'
import LogContextDialog from '@/components/Workload/LogContextDialog.vue'

const props = defineProps<{
  wlid: string
  dispatchCount?: number
  nodes?: string[][]
  ranks?: string[][]
  failedNodes?: string[]
  isDownload: boolean
  selectFirstN?: number
}>()

const hasRank = (r: unknown) => r !== null && r !== undefined && r !== ''
const wordWrap = ref(true)

const selectedGroup = ref<number>(props.dispatchCount ?? 0)

const tableRows = computed(() => {
  if (!props.nodes || selectedGroup.value === 0) return []
  const i = selectedGroup.value - 1
  const nodes = props.nodes[i] ?? []
  const ranks = props.ranks?.[i] ?? []
  return nodes.map((node, idx) => ({ node, rank: ranks[idx] }))
})

const groupOptions = computed(() =>
  Array.from({ length: props.dispatchCount ?? 0 }, (_, i) => {
    const idx = i + 1
    return { label: String(idx), value: idx }
  }),
)

const {
  loading,
  rowData,
  searchParams,
  pagination,
  handlePageChange,
  onSearch,
  dedupeKeywords,
  nodeTableRef,
  onNodeSelectionChange,
  downloadLoading,
  downloadLogs,
  ctxVisible,
  ctxLoading,
  ctxRows,
  onFetchContext,
  onCtxClosed,
} = useLogTable(
  props.wlid,
  selectedGroup,
  toRef(props, 'dispatchCount'),
  tableRows,
  toRef(props, 'failedNodes'),
  props.selectFirstN,
)

searchParams.order = 'desc'
const displayOrder = ref<'asc' | 'desc'>('asc')

// ── Infinite scroll state ──
const scrollRows = ref<RowData[]>([])
const terminalRef = ref<HTMLElement>()
const lastLoadedPage = ref(0)
let pendingReset = true

const totalPages = computed(() =>
  Math.max(1, Math.ceil(pagination.total / pagination.pageSize)),
)
const hasMore = computed(() =>
  lastLoadedPage.value > 0 && lastLoadedPage.value < totalPages.value,
)
const loadedCount = computed(() =>
  scrollRows.value.filter((r) => r._kind !== 'header').length,
)
const initialLoading = computed(() => loading.value && !scrollRows.value.length)
const loadingMore = computed(() => loading.value && scrollRows.value.length > 0)

const reverseWithinGroups = (rows: RowData[]): RowData[] => {
  const result: RowData[] = []
  let buffer: RowData[] = []
  for (const row of rows) {
    if (row._kind === 'header') {
      if (buffer.length) result.push(...buffer.reverse())
      buffer = []
      result.push(row)
    } else {
      buffer.push(row)
    }
  }
  if (buffer.length) result.push(...buffer.reverse())
  return result
}

const scrollToBottom = () => {
  const el = terminalRef.value
  if (el) el.scrollTop = el.scrollHeight
}

watch(rowData, (rows) => {
  if (!rows?.length) {
    if (pendingReset) {
      scrollRows.value = []
      lastLoadedPage.value = 0
    }
    pendingReset = false
    return
  }

  const page = pagination.page
  const isAsc = displayOrder.value === 'asc'
  const processed = isAsc ? reverseWithinGroups(rows) : [...rows]

  if (pendingReset || lastLoadedPage.value === 0) {
    pendingReset = false
    scrollRows.value = processed
    lastLoadedPage.value = page
    if (isAsc) nextTick(scrollToBottom)
    return
  }

  if (page > lastLoadedPage.value) {
    if (isAsc) {
      const el = terminalRef.value
      const prevHeight = el?.scrollHeight ?? 0
      scrollRows.value = [...processed, ...scrollRows.value]
      lastLoadedPage.value = page
      nextTick(() => {
        if (el) el.scrollTop += el.scrollHeight - prevHeight
      })
    } else {
      scrollRows.value = [...scrollRows.value, ...processed]
      lastLoadedPage.value = page
    }
  }
})

const onTerminalScroll = () => {
  const el = terminalRef.value
  if (!el || loading.value || !hasMore.value) return

  if (displayOrder.value === 'asc' && el.scrollTop < 80) {
    handlePageChange(lastLoadedPage.value + 1)
  } else if (
    displayOrder.value !== 'asc' &&
    el.scrollHeight - el.scrollTop - el.clientHeight < 80
  ) {
    handlePageChange(lastLoadedPage.value + 1)
  }
}

const resetScroll = () => {
  pendingReset = true
  scrollRows.value = []
  lastLoadedPage.value = 0
}

const doSearch = () => {
  resetScroll()
  onSearch()
}

const doNodeSelectionChange = (rows: { node: string; rank?: string | number }[]) => {
  resetScroll()
  onNodeSelectionChange(rows)
}

watch(displayOrder, () => doSearch())

const RE_ANSI = new RegExp(String.fromCharCode(0x1b) + '\\[[0-9;]*[A-Za-z]', 'g')
const stripAnsi = (s: string) => s.replace(RE_ANSI, '')

const getLogLevelClass = (log: string): string => {
  const clean = stripAnsi(log)
  if (/error|fatal|exception|traceback|panic|NCCL WARN/i.test(clean)) return 'term-error'
  if (/warn(?:ing)?/i.test(clean)) return 'term-warn'
  if (/training.?progress|throughput|metric/i.test(clean)) return 'term-metric'
  if (/cache.?flush|debug/i.test(clean)) return 'term-debug'
  return ''
}

const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
const escapeRegExp = (s: string) =>
  s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

const highlightLog = (log: string): string => {
  const html = escapeHtml(stripAnsi(log))
  const kws = searchParams.keywords
  if (!kws.length) return html
  const pattern = kws.map(escapeRegExp).join('|')
  return html.replace(new RegExp(`(${pattern})`, 'gi'), '<mark class="term-kw-hit">$1</mark>')
}

watch(
  () => props.dispatchCount,
  (v) => {
    selectedGroup.value = v ?? 0
  },
  { immediate: true },
)
</script>

<style scoped>
/* ── Terminal wrapper ── */
.log-terminal-wrapper {
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid #2a2e3a;
  background: #1a1d27;
}

.log-terminal-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 14px;
  background: #14161e;
  border-bottom: 1px solid #2a2e3a;
}

/* ── Terminal body ── */
.log-terminal {
  height: calc(100vh - 316px);
  overflow: auto;
  padding: 0;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 20px;
  background: #1a1d27;
  color: #cdd6f4;
}
.log-terminal--empty {
  display: flex;
  align-items: center;
  justify-content: center;
}
.log-terminal--nowrap {
  white-space: nowrap;
}
.log-terminal:not(.log-terminal--nowrap) {
  white-space: pre-wrap;
}

/* ── Group header (pod separator) ── */
.term-group-header {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 6px 14px;
  color: #89b4fa;
  font-weight: 600;
  background: #1e2030;
  border-top: 1px solid #2a2e3a;
  border-bottom: 1px solid #2a2e3a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  user-select: none;
}
.term-group-header:first-child {
  border-top: none;
}

/* ── Log line ── */
.term-line {
  display: flex;
  align-items: flex-start;
  padding: 1px 14px 1px 14px;
  cursor: pointer;
  transition: background 0.1s;
  border-left: 3px solid transparent;
}
.term-line:hover {
  background: rgba(205, 214, 244, 0.06);
}

/* ── Timestamp ── */
.term-ts {
  flex-shrink: 0;
  width: 200px;
  color: #7f849c;
}

/* ── Message ── */
.term-msg {
  flex: 1;
  color: #bac2de;
  overflow-wrap: anywhere;
}

/* ── Log level colors ── */
.term-error {
  border-left-color: #f38ba8;
}
.term-error .term-msg {
  color: #f38ba8;
}
.term-warn {
  border-left-color: #fab387;
}
.term-warn .term-msg {
  color: #fab387;
}
.term-metric {
  border-left-color: #89b4fa;
}
.term-metric .term-msg {
  color: #89dceb;
}
.term-debug .term-msg {
  color: #585b70;
}

/* ── Keyword highlight ── */
:deep(.term-kw-hit) {
  background: rgba(249, 226, 175, 0.3);
  color: #f9e2af;
  border-radius: 2px;
  padding: 0 2px;
}

/* ── Scrollbar ── */
.log-terminal::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.log-terminal::-webkit-scrollbar-track {
  background: transparent;
}
.log-terminal::-webkit-scrollbar-thumb {
  background: #45475a;
  border-radius: 4px;
}
.log-terminal::-webkit-scrollbar-thumb:hover {
  background: #585b70;
}

/* ── Loading / end marks ── */
.term-loading-bar {
  padding: 6px 14px;
  color: #585b70;
  text-align: center;
  font-size: 12px;
}
.term-end-mark {
  padding: 4px 14px;
  color: #45475a;
  text-align: center;
  font-size: 12px;
}
.term-stats {
  color: #7f849c;
  font-size: 12px;
  margin-left: auto;
}

/* ── Override el-switch in dark terminal toolbar ── */
.log-terminal-toolbar :deep(.el-switch__label) {
  color: #7f849c;
}
.log-terminal-toolbar :deep(.el-switch__label.is-active) {
  color: #cdd6f4;
}
</style>
