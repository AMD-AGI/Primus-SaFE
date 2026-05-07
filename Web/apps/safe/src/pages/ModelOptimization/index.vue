<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Model Optimization</el-text>
    <div class="flex items-center m-t-4">
      <el-button
        type="primary"
        round
        :icon="Plus"
        class="text-black"
        @click="drawerVisible = true"
      >
        New Task
      </el-button>
      <el-input
        v-model="searchParams.search"
        placeholder="Search by name or model"
        clearable
        class="ml-auto"
        style="width: 240px"
        @input="handleSearchInput"
        @clear="onSearch({ resetPage: true })"
      />
    </div>
  </div>

  <!-- Table -->
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 245px)'"
      :data="tableData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @filter-change="handleFilterChange"
    >
      <el-table-column prop="displayName" label="Display Name" min-width="180">
        <template #default="{ row }">
          <el-link
            type="primary"
            v-route="{ path: `/model-optimization/${row.id}` }"
          >{{ row.displayName || row.id }}</el-link>
        </template>
      </el-table-column>
      <el-table-column prop="modelId" label="Model" min-width="200" />
      <el-table-column label="Config" width="160" align="center">
        <template #default="{ row }">
          <span class="config-tag">{{ row.framework || '-' }} · {{ row.precision || '-' }} · TP{{ row.tp ?? 1 }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="Status" width="130"
        column-key="statusFilter"
        :filters="statusFilters"
        :filter-multiple="false"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <TaskStatusTag :status="row.status" />
        </template>
      </el-table-column>
      <el-table-column label="Duration" width="130">
        <template #default="{ row }">
          {{ formatDuration(row.startedAt, row.finishedAt) }}
        </template>
      </el-table-column>
      <el-table-column prop="updatedAt" label="Updated" width="180" sortable>
        <template #default="{ row }">
          {{ formatTimeStr(row.updatedAt) }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="160" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Interrupt" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :icon="VideoPause"
              :disabled="row.status !== 'Running'"
              @click="handleInterrupt(row)"
            />
          </el-tooltip>
          <el-tooltip content="Retry" placement="top">
            <el-button
              circle
              size="default"
              class="btn-primary-plain"
              :icon="RefreshRight"
              :disabled="row.status !== 'Failed' && row.status !== 'Interrupted'"
              @click="handleRetry(row)"
            />
          </el-tooltip>
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="handleDelete(row)"
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

  <CreateTaskDrawer
    v-model:visible="drawerVisible"
    @success="onSearch({ resetPage: true })"
  />
</template>

<script lang="ts" setup>
import { ref, reactive, onMounted, h } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Delete, VideoPause, RefreshRight } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listOptimizationTasks,
  deleteOptimizationTask,
  interruptOptimizationTask,
  retryOptimizationTask,
} from '@/services/model-optimization'
import { OptimizationStatus } from '@/services/model-optimization/type'
import type { OptimizationTask } from '@/services/model-optimization/type'
import { formatTimeStr } from '@/utils'
import TaskStatusTag from './components/TaskStatusTag.vue'
import CreateTaskDrawer from './components/CreateTaskDrawer.vue'

const formatDuration = (start?: string, end?: string): string => {
  if (!start || !end) return '-'
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (ms < 0) return '-'
  const sec = Math.floor(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m ${sec % 60}s`
  const hr = Math.floor(min / 60)
  return `${hr}h ${min % 60}m`
}

defineOptions({ name: 'ModelOptimizationPage' })

const router = useRouter()

const loading = ref(false)
const tableData = ref<OptimizationTask[]>([])
const drawerVisible = ref(false)
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const searchParams = reactive({ search: '', status: '' })

const statusFilters = Object.values(OptimizationStatus).map((s) => ({ text: s, value: s }))
const passAll = () => true

let searchTimer: ReturnType<typeof setTimeout> | null = null
const handleSearchInput = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => onSearch({ resetPage: true }), 500)
}

const fetchData = async () => {
  loading.value = true
  try {
    const raw = await listOptimizationTasks({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      ...(searchParams.search?.trim() ? { search: searchParams.search.trim() } : {}),
      ...(searchParams.status ? { status: searchParams.status } : {}),
    })
    const res = (raw as any)?.data ?? raw
    tableData.value = res?.items || []
    pagination.total = res?.total ?? res?.totalCount ?? 0
  } catch (e: any) {
    ElMessage.error(e?.message || 'Failed to load tasks')
  } finally {
    loading.value = false
  }
}

const onSearch = (opts?: { resetPage?: boolean }) => {
  if (opts?.resetPage) pagination.page = 1
  fetchData()
}

const handlePageChange = (p: number) => { pagination.page = p; fetchData() }
const handlePageSizeChange = (s: number) => { pagination.pageSize = s; pagination.page = 1; fetchData() }

const handleFilterChange = (filters: Record<string, string[]>) => {
  if ('statusFilter' in filters) {
    searchParams.status = filters.statusFilter?.[0] || ''
  }
  onSearch({ resetPage: true })
}

const handleInterrupt = async (row: OptimizationTask) => {
  try {
    await ElMessageBox.confirm(
      `Interrupt optimization task "${row.displayName || row.id}"?`,
      'Interrupt',
      { confirmButtonText: 'Interrupt', type: 'warning' },
    )
    await interruptOptimizationTask(row.id)
    ElMessage.success('Task interrupted')
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e?.message || 'Failed to interrupt task')
  } finally {
    fetchData()
  }
}

const handleRetry = async (row: OptimizationTask) => {
  try {
    await ElMessageBox.confirm(
      `Retry optimization task "${row.displayName || row.id}"?`,
      'Retry',
      { confirmButtonText: 'Retry', type: 'info' },
    )
    await retryOptimizationTask(row.id)
    ElMessage.success('Task retried')
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e?.message || 'Failed to retry task')
  } finally {
    fetchData()
  }
}

const handleDelete = async (row: OptimizationTask) => {
  const msg = h('span', null, [
    'Delete optimization task: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.displayName || row.id),
    ' ?',
  ])
  try {
    await ElMessageBox.confirm(msg, 'Delete', {
      confirmButtonText: 'Delete',
      type: 'warning',
    })
    await deleteOptimizationTask(row.id)
    ElMessage.success('Deleted')
    fetchData()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
  }
}

onMounted(fetchData)
</script>

<style scoped>
.config-tag {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  font-family: monospace;
}
</style>
