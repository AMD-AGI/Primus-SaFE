<template>
  <el-dialog
    :model-value="visible"
    title="Import Progress"
    width="1150px"
    @close="onClose"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onRefresh"
  >
    <div class="dialog-body">
      <div class="mb-4 flex items-center" style="font-size: large">
        image tag:
        <span style="color: var(--el-color-primary); font-weight: 600; margin-left: 6px">{{
          tag
        }}</span>
      </div>

      <el-tabs v-model="activeTab">
        <!-- Progress Tab -->
        <el-tab-pane label="Progress" name="progress">
          <div class="mb-2 flex justify-end">
            <el-button @click="onRefresh" :loading="loading" size="small"> Refresh </el-button>
          </div>
          <el-table
            :data="parsedData"
            style="width: 100%"
            row-key="digest"
            height="620"
            :show-header="false"
            v-loading="loading"
          >
            <el-table-column prop="digest" label="Digest" width="600">
              <template #default="{ row }">
                <el-tooltip :content="row.digest">
                  <span
                    style="
                      display: inline-block;
                      max-width: 560px;
                      overflow: hidden;
                      text-overflow: ellipsis;
                      white-space: nowrap;
                      font-family:
                        ui-monospace, SFMono-Regular, Menlo, Monaco, 'Roboto Mono', 'Noto Mono',
                        monospace;
                    "
                  >
                    {{ row.digest }}
                  </span>
                </el-tooltip>
              </template>
            </el-table-column>

            <el-table-column label="Progress" width="490" align="right">
              <template #default="{ row }">
                <div
                  style="display: flex; align-items: center; justify-content: flex-end; gap: 12px"
                >
                  <el-progress
                    :percentage="computePercentage(row)"
                    stroke-width="12"
                    style="width: 310px"
                  />
                  <div style="width: 170px; text-align: right; font-size: 12px; color: #909399">
                    {{ formatOffsetAndSize(row) }}
                  </div>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <!-- Logs Tab -->
        <el-tab-pane label="Logs" name="logs">
          <div class="flex items-center justify-between mb-2">
            <el-space wrap>
              <el-select v-model="logOrder" style="width: 100px" size="small">
                <el-option label="desc" value="desc" />
                <el-option label="asc" value="asc" />
              </el-select>
            </el-space>

            <el-button :icon="Refresh" size="small" @click="manualLogRefresh"> Refresh </el-button>
          </div>

          <el-table
            :data="logRows"
            v-loading="logLoading"
            style="height: 560px"
            row-key="id"
          >
            <el-table-column>
              <template #default="{ row }">
                <div v-if="row._kind === 'header'" class="group-header">
                  <span class="pill">host: {{ row.host }}</span>
                  <span class="pill ml-2">pod: {{ row.pod_name }}</span>
                </div>
                <div v-else class="log-row">
                  <div class="log-time">{{ row.timeMessage }}</div>
                  <div class="log-message">{{ row.log }}</div>
                </div>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!logLoading && !logRows.length" description="No logs available" />
          <el-pagination
            v-if="logRows.length"
            class="mt-2"
            :current-page="logPagination.page"
            :page-size="logPagination.pageSize"
            :total="logPagination.total"
            @current-change="handleLogPageChange"
            @size-change="handleLogPageSizeChange"
            layout="total, sizes, prev, pager, next"
            :page-sizes="[200, 500, 800]"
          />
        </el-tab-pane>
      </el-tabs>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { getImportDetail, getImportLogs } from '@/services'
import { Refresh } from '@element-plus/icons-vue'
import { fmtTs } from '@/utils'

const props = defineProps<{
  id: number
  visible: boolean
  tag: string
}>()
const emit = defineEmits(['update:visible'])

const activeTab = ref('progress')

// ===== Progress tab =====
const loading = ref(false)
const parsedData = ref<any[]>([])

const loadDetail = async (id: number) => {
  loading.value = true
  try {
    const res = await getImportDetail(id)
    parsedData.value = Object.entries(res?.layersDetail?.data ?? {}).map(([digest, item]) => ({
      digest,
      ...(item as Record<string, any>),
    }))
  } catch (err) {
    console.error('loadDetail error', err)
  } finally {
    loading.value = false
  }
}

const onRefresh = async () => {
  await loadDetail(props.id)
}

function computePercentage(item: any) {
  if (!item || !item.Artifact) return 0
  const size = Number(item.Artifact.Size) || 0
  const offset = Number(item.Offset) || 0
  if (size <= 0) return 0
  const p = Math.round((offset / size) * 100)
  return p > 100 ? 100 : p
}

function formatOffsetAndSize(item: any) {
  if (!item || !item.Artifact) return '--'
  const size = Number(item.Artifact.Size) || 0
  const offset = Number(item.Offset) || 0
  if (size <= 0) return `— / ${size}`
  return `${offset} / ${size}`
}

// ===== Logs tab =====
type LogRowData =
  | { _kind: 'header'; id: string; pod_name: string; host: string }
  | { _kind: 'row'; id: string; log: string; pod_name: string; host: string; timeMessage: string }

const logLoading = ref(false)
const logRows = ref<LogRowData[]>([])
const logOrder = ref<'asc' | 'desc'>('desc')
const logPagination = reactive({
  page: 1,
  pageSize: 200,
  total: 0,
})


// Build grouped rows (group by pod+host, like LogTable)
function buildGroupedRows(
  rows: Array<{ id: string; timestamp: string; message: string; pod_name: string; host: string }>,
): LogRowData[] {
  const groups = new Map<
    string,
    { pod_name: string; host: string; items: typeof rows }
  >()
  for (const r of rows) {
    const key = `${r.pod_name}@@${r.host}`
    if (!groups.has(key)) groups.set(key, { pod_name: r.pod_name, host: r.host, items: [] })
    if (r.message) {
      groups.get(key)!.items.push(r)
    }
  }

  const out: LogRowData[] = []
  for (const [key, g] of groups) {
    out.push({
      _kind: 'header',
      id: `header-${key}`,
      pod_name: g.pod_name,
      host: g.host,
    })
    for (const r of g.items) {
      out.push({
        _kind: 'row',
        id: r.id,
        log: r.message,
        pod_name: r.pod_name,
        host: r.host,
        timeMessage: fmtTs(r.timestamp),
      })
    }
  }
  return out
}

const fetchLogs = async () => {
  if (!props.id) return
  logLoading.value = true
  try {
    const offset = (logPagination.page - 1) * logPagination.pageSize
    const res = await getImportLogs(props.id, {
      offset,
      limit: logPagination.pageSize,
      order: logOrder.value,
    })
    logRows.value = buildGroupedRows(res?.rows ?? [])
    logPagination.total = res?.total ?? 0
  } catch (err) {
    console.error('fetchLogs error', err)
  } finally {
    logLoading.value = false
  }
}

const manualLogRefresh = async () => {
  await fetchLogs()
}

const handleLogPageChange = (newPage: number) => {
  logPagination.page = newPage
  fetchLogs()
}

const handleLogPageSizeChange = (newSize: number) => {
  logPagination.pageSize = newSize
  logPagination.page = 1
  fetchLogs()
}

// When switching to logs tab, fetch logs
watch(activeTab, (tab) => {
  if (tab === 'logs') {
    logPagination.page = 1
    fetchLogs()
  }
})

// When order changes, re-fetch
watch(logOrder, () => {
  logPagination.page = 1
  fetchLogs()
})

// When dialog opens/closes
watch(
  () => props.visible,
  (v) => {
    if (v) {
      activeTab.value = 'progress'
    } else {
      logRows.value = []
    }
  },
)

const onClose = () => {
  emit('update:visible', false)
}
</script>

<style scoped>
.dialog-body {
  padding: 15px;
}

.group-header {
  font-weight: 600;
  color: #628ed0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-row {
  display: flex;
  align-items: flex-start;
  font-family: monospace;
  white-space: pre-wrap;
  line-height: 1.4;
  padding: 2px 0;
}
.log-time {
  flex-shrink: 0;
  width: 200px;
  color: var(--log-time-color);
}
.log-message {
  flex: 1;
  color: var(--log-message-color);
  overflow-wrap: anywhere;
}
</style>
