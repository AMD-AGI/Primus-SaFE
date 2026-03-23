<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Faults</el-text>
  </div>
  <el-row class="m-t-4" :gutter="20">
    <el-col :span="6">
      <el-input
        v-model="searchParams.nodeId"
        style="width: 100%"
        size="default"
        placeholder="Node ID"
        clearable
        @keyup.enter="onSearch({ resetPage: true })"
        @clear="onSearch({ resetPage: true })"
      />
    </el-col>
    <el-col :span="6">
      <el-input
        v-model="searchParams.monitorId"
        style="width: 100%"
        size="default"
        placeholder="Error Id"
        clearable
        @keyup.enter="onSearch({ resetPage: true })"
        @clear="onSearch({ resetPage: true })"
      />
    </el-col>
    <el-col :span="6">
      <el-space>
        <el-checkbox
          v-model="searchParams.onlyOpen"
          label="Only Open"
          size="large"
          @change="filterByOpen"
        />
        <el-button
          :icon="Search"
          size="default"
          type="primary"
          @click="onSearch({ resetPage: true })"
        ></el-button>
        <el-tooltip content="Reset filters" placement="top">
          <el-button
            :icon="ResetIcon"
            size="default"
            @click="
            () => {
              Object.assign(searchParams, initialSearchParams)
              pagination.page = 1
              fetchData()
            }
            "
          ></el-button>
        </el-tooltip>
        <el-tooltip content="Refresh" placement="top">
          <el-button
            :icon="Refresh"
            size="default"
            @click="onSearch({ resetPage: false })"
          ></el-button>
        </el-tooltip>
      </el-space>
    </el-col>
  </el-row>

  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      ref="tableRef"
      height="calc(100vh - 245px)"
      :data="tableData"
      size="large"
      border
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @sort-change="onSortChange"
      :span-method="objectSpanMethod"
      :default-sort="{ prop: 'nodeId', order: 'descending' }"
      :sort-orders="['descending', 'ascending']"
    >
      <el-table-column
        prop="nodeId"
        label="Node ID"
        min-width="100"
        :fixed="true"
        sortable="custom"
      >
        <template #default="{ row }">
          {{ row.nodeId || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="id" label="ID" min-width="120" :fixed="true" />
      <el-table-column prop="monitorId" label="Error ID" min-width="120" :fixed="true">
        <template #default="{ row }">
          <div class="flex items-center">
            {{ row.monitorId || '-' }}
            <el-tooltip :content="row.message || '-'">
              <el-icon class="color-[var(--safe-primary)] ml-1 cursor-pointer"><Warning /></el-icon>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="action" label="Action" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.action || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" min-width="180">
        <template #default="{ row }">
          {{ formatTimeStr(row.creationTime) }}
        </template>
      </el-table-column>
      <el-table-column prop="deletionTime" label="Deletion Time" min-width="180">
        <template #default="{ row }">
          {{ formatTimeStr(row.deletionTime) }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="onDelete(row.id)"
            />
          </el-tooltip>
          <el-tooltip :content="!!row.deletionTime ? 'Has been stopped' : 'Stop'" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :icon="Close"
              @click="onStop(row.id)"
              :disabled="!!row.deletionTime"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      class="m-t-2"
      :current-page="pagination.page"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      @current-change="handlePageChange"
      @size-change="handlePageSizeChange"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50, 100]"
    />
  </el-card>
</template>
<script lang="ts" setup>
import { ref, reactive, onMounted, h, watch } from 'vue'
import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import { Search, Refresh, Delete, Close, Warning } from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getFaultsList, type FaultsData, deleteFault, stopFault } from '@/services'
import { formatTimeStr } from '@/utils'
import type { TableInstance } from 'element-plus'

const tableRef = ref<TableInstance>()

const clusterStore = useClusterStore()
const userStore = useUserStore()

const initialSearchParams = {
  nodeId: '',
  monitorId: '',
  onlyOpen: true,
}
const searchParams = reactive({ ...initialSearchParams })
const sortState = reactive<{ sortBy?: string; order?: 'asc' | 'desc' }>({
  sortBy: 'node',
  order: 'desc',
})

const loading = ref(false)
const tableData = ref([] as FaultsData[])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})

const nodeSpans = ref<number[]>([])
function computeSpans(data: any[], key: string) {
  const spans: number[] = new Array(data.length).fill(1)
  let i = 0
  while (i < data.length) {
    const cur = data[i]?.[key]
    let j = i + 1
    while (j < data.length && data[j]?.[key] === cur) j++
    const len = j - i
    spans[i] = len // First row of the group spans len rows
    for (let k = i + 1; k < j; k++) spans[k] = 0 // Other rows in the group are hidden
    i = j
  }
  return spans
}
watch(
  tableData,
  () => {
    nodeSpans.value = computeSpans(tableData.value, 'nodeId')
  },
  { deep: true },
)
const objectSpanMethod = ({ rowIndex, column }: { rowIndex: number; column: any }) => {
  if (column?.property !== 'nodeId') return { rowspan: 1, colspan: 1 }
  return { rowspan: nodeSpans.value[rowIndex] ?? 1, colspan: 1 }
}
const fetchData = async (params?: any) => {
  try {
    loading.value = true
    if (!clusterStore.currentClusterId) return

    const res = await getFaultsList({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      sortBy: sortState.sortBy,
      order: sortState.order,
      ...params,
    })
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}

const handlePageChange = (newPage: number) => {
  pagination.page = newPage
  onSearch({ resetPage: false })
}

const handlePageSizeChange = (newSize: number) => {
  pagination.pageSize = newSize
  pagination.page = 1
  onSearch({ resetPage: false })
}

const onSortChange = (e: {
  column: any
  prop: string
  order: 'ascending' | 'descending' | null
}) => {
  if (e.prop !== 'nodeId' || !e.order) {
    // Force reset to nodeId descending (keep UI and state in sync)
    sortState.sortBy = 'node'
    sortState.order = 'desc'
    tableRef.value?.sort?.('nodeId', 'descending')
  } else {
    sortState.sortBy = 'node'
    sortState.order = e.order === 'ascending' ? 'asc' : 'desc'
  }

  onSearch({ resetPage: true })
}

const onSearch = (options?: { resetPage?: boolean }) => {
  const reset = options?.resetPage ?? true
  if (reset) pagination.page = 1
  return fetchData(searchParams)
}
const filterByOpen = (val: boolean) => {
  searchParams.onlyOpen = val
  onSearch({ resetPage: true })
}

const onDelete = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete fault: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete fault', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteFault(id)
    ElMessage({
      type: 'success',
      message: 'Delete completed',
    })
    onSearch({ resetPage: false })
  })
}
const onStop = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to stop fault: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Stop fault', {
    confirmButtonText: 'Stop',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await stopFault(id)
    ElMessage({
      type: 'success',
      message: 'Stop completed',
    })
    onSearch({ resetPage: false })
  })
}

onMounted(() => {
  tableRef.value?.sort?.('nodeId', 'descending')
  onSearch({ resetPage: true })
})

defineOptions({
  name: 'faultPage',
})
</script>
<style scoped></style>
