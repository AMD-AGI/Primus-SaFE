<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Model Optimization</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="drawerVisible = true"
    >
      New Task
    </el-button>
  </div>

  <!-- Filters -->
  <el-row class="m-t-4" :gutter="20">
    <el-col :span="6">
      <el-input
        v-model="searchParams.search"
        placeholder="Search by name or model"
        clearable
        @input="handleSearchInput"
        @clear="onSearch({ resetPage: true })"
      />
    </el-col>
  </el-row>

  <!-- Table -->
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 295px)'"
      :data="tableData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
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
      <el-table-column prop="workspace" label="Workspace" width="160" />
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
      <el-table-column prop="currentPhaseName" label="Current Phase" width="160">
        <template #default="{ row }">
          {{ row.currentPhaseName || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="updatedAt" label="Updated" width="180" sortable>
        <template #default="{ row }">
          {{ formatTimeStr(row.updatedAt) }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="200" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="View detail" placement="top">
            <el-button
              circle
              size="default"
              class="btn-primary-plain"
              :icon="View"
              @click="router.push(`/model-optimization/${row.id}`)"
            />
          </el-tooltip>
          <el-tooltip v-if="row.status === 'Running'" content="Interrupt" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :icon="VideoPause"
              @click="handleInterrupt(row)"
            />
          </el-tooltip>
          <el-tooltip v-if="row.status === 'Failed' || row.status === 'Interrupted'" content="Retry" placement="top">
            <el-button
              circle
              size="default"
              class="btn-primary-plain"
              :icon="RefreshRight"
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
import { Plus, Delete, View, VideoPause, RefreshRight } from '@element-plus/icons-vue'
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
    const res = await listOptimizationTasks({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      ...(searchParams.search?.trim() ? { search: searchParams.search.trim() } : {}),
      ...(searchParams.status ? { status: searchParams.status } : {}),
    })
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
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
    fetchData()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
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
    fetchData()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
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
