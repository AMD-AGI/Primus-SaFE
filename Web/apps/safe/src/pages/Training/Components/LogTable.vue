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
          @selection-change="onNodeSelectionChange"
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
            v-model="searchParams.order"
            placeholder="order"
            style="width: 100%"
            size="default"
          >
            <el-option v-for="item in ['desc', 'asc']" :key="item" :label="item" :value="item" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <el-button :icon="Search" size="default" type="primary" @click="onSearch"></el-button>
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
      <el-card class="mt-4 safe-card" shadow="never">
        <el-table
          :data="rowData"
          v-loading="loading"
          :element-loading-text="$loadingText"
          style="height: calc(100vh - 360px)"
          row-key="id"
          v-if="rowData?.length"
        >
          <el-table-column>
            <template #default="{ row }">
              <div v-if="row._kind === 'header'" class="group-header">
                <span class="pill">host: {{ row.host }}</span>
                <span class="pill ml-2">pod: {{ row.pod_name }}</span>
                <span class="pill ml-2">dispatchCount: {{ row.count }}</span>
              </div>
              <div v-else class="log-row" @click="onFetchContext(row.id, row.ts, row.log)">
                <div class="log-time">{{ row.timeMessage }}</div>
                <div class="log-message">{{ row.log }}</div>
              </div>
            </template>
          </el-table-column>
        </el-table>
        <el-empty
          style="height: calc(100vh - 320px)"
          v-loading="loading"
          :element-loading-text="$loadingText"
          v-else
          description="Click the Search button to view results"
        />
        <el-pagination
          v-if="rowData?.length"
          class="m-t-2"
          :current-page="pagination.page"
          :page-size="pagination.pageSize"
          :total="pagination.total"
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
          layout="total, sizes, prev, pager, next"
          :page-sizes="[200, 500, 800]"
        />
      </el-card>
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
import { computed, ref, toRef, watch } from 'vue'
import { InfoFilled, Search, Download } from '@element-plus/icons-vue'
import { useLogTable } from '@/composables/useLogTable'
import LogContextDialog from '@/components/Workload/LogContextDialog.vue'

const props = defineProps<{
  wlid: string
  dispatchCount?: number
  nodes?: string[][]
  ranks?: string[][]
  failedNodes?: string[]
  isDownload: boolean
}>()

const hasRank = (r: unknown) => r !== null && r !== undefined && r !== ''

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
  handlePageSizeChange,
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
)

watch(
  () => props.dispatchCount,
  (v) => {
    selectedGroup.value = v ?? 0
  },
  { immediate: true },
)
</script>

<style scoped>
:deep(.el-table-v2__row) {
  height: auto !important;
}
:deep(.group-header) {
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
  cursor: pointer;
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
