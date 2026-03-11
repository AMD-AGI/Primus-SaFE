<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Evaluation</el-text>
    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center">
        <el-button
          type="primary"
          round
          :icon="Plus"
          @click="showCreateDialog = true"
          class="text-black"
        >
          Create Evaluation Task
        </el-button>
      </div>
      <div class="flex flex-wrap items-center">
        <el-input
          v-model="serviceId"
          placeholder="Search by service ID"
          clearable
          :prefix-icon="Search"
          style="max-width: 300px"
          @input="debouncedSearch"
          @clear="handleFilterChange"
        />
      </div>
    </div>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 260px)'"
      :data="items"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @filter-change="handleTableFilterChange"
    >
      <el-table-column prop="taskName" label="Name/ID" min-width="240" :fixed="true" align="left">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" @click="router.push(`/evaluation/${row.taskId}`)">{{
              row.taskName
            }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.taskId }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.taskId)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="serviceName" label="Service" min-width="260">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <span>{{ row.serviceName || '-' }}</span>
            <span class="text-[12px] text-gray-400">{{ row.serviceId || '-' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        prop="status"
        label="Status"
        min-width="140"
        column-key="status"
        :filters="statusFilters"
        :filtered-value="statusSelectedIds"
        :filter-multiple="true"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag
            v-if="row.status"
            :type="STATUS_META[row.status as EvaluationStatus]?.type || 'info'"
            :effect="isDark ? 'plain' : 'light'"
          >
            {{ STATUS_META[row.status as EvaluationStatus]?.label || row.status }}
          </el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="userName" label="User" min-width="140">
        <template #default="{ row }">
          {{ row.userName || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" width="180">
        <template #default="{ row }">
          {{ row.creationTime ? dayjs(row.creationTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="startTime" label="Start Time" width="180">
        <template #default="{ row }">
          {{ row.startTime ? dayjs(row.startTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="endTime" label="End Time" width="180">
        <template #default="{ row }">
          {{ row.endTime ? dayjs(row.endTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="duration" label="Duration" width="120">
        <template #default="{ row }">
          {{ calculateDuration(row.startTime, row.endTime) }}
        </template>
      </el-table-column>
      <el-table-column prop="datasetName" label="Dataset" min-width="180">
        <template #default="{ row }">
          <div v-if="row.benchmarks && row.benchmarks.length" class="flex flex-wrap gap-1">
            <el-tag
              v-for="(benchmark, index) in row.benchmarks"
              :key="index"
              size="small"
              type="info"
              :effect="isDark ? 'plain' : 'light'"
            >
              {{ benchmark.datasetName }}
            </el-tag>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="evaluationType" label="Evaluation Type" min-width="140">
        <template #default="{ row }">
          <el-tag
            v-if="row.evaluationType"
            :type="row.evaluationType === 'normal' ? 'warning' : 'success'"
            :effect="isDark ? 'plain' : 'light'"
            size="small"
          >
            {{ row.evaluationType === 'normal' ? 'Normal' : 'Judge' }}
          </el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="160" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Clone" placement="top">
            <el-button
              circle
              size="default"
              class="btn-success-plain"
              :icon="DocumentCopy"
              @click="onClone(row.taskId)"
            />
          </el-tooltip>
          <el-tooltip content="Stop" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :disabled="row.status !== 'Running' && row.status !== 'Pending'"
              :icon="Close"
              @click="onStop(row.taskId, row.taskName)"
            />
          </el-tooltip>
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="onDelete(row.taskId, row.taskName)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
  <el-pagination
    v-model:current-page="currentPage"
    v-model:page-size="pageSize"
    :total="totalCount"
    :page-sizes="[10, 20, 50, 100]"
    layout="total, sizes, prev, pager, next, jumper"
    class="m-t-4"
    @size-change="handlePageChange"
    @current-change="handlePageChange"
  />

  <CreateDialog
    v-model:visible="showCreateDialog"
    :clone-task-id="cloneTaskId"
    @success="handleCreateSuccess"
    @close="handleDialogClose"
  />
</template>

<script lang="ts" setup>
import { ref, onMounted, h, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import dayjs from 'dayjs'
import { debounce } from 'lodash'
import { CopyDocument, Delete, Search, Plus, DocumentCopy, Close } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { copyText } from '@/utils'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  deleteEvaluationTask,
  getEvaluationTasks,
  stopEvaluationTask,
} from '@/services/evaluations'
import type { EvaluationStatus, EvaluationTaskItem } from '@/services/evaluations/type'
import { useDark } from '@vueuse/core'
import CreateDialog from '@/pages/Evaluation/Components/CreateDialog.vue'

const router = useRouter()
const route = useRoute()

const workspaceStore = useWorkspaceStore()
const isDark = useDark()
const loading = ref(false)
const items = ref<EvaluationTaskItem[]>([])
const totalCount = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const serviceId = ref('')
const showCreateDialog = ref(false)
const cloneTaskId = ref('')
const statusSelectedIds = ref<EvaluationStatus[]>([])

const STATUS_META: Record<string, { label: string; type: string }> = {
  Pending: { label: 'Pending', type: 'info' },
  Running: { label: 'Running', type: 'primary' },
  Succeeded: { label: 'Completed', type: 'success' },
  Failed: { label: 'Failed', type: 'danger' },
}

const statusOptions = (Object.keys(STATUS_META) as EvaluationStatus[]).map((value) => ({
  label: STATUS_META[value].label,
  value,
}))

const statusFilters = statusOptions.map((option) => ({
  text: option.label,
  value: option.value,
}))

const passAll = () => true

const calculateDuration = (startTime?: string, endTime?: string) => {
  if (!startTime || !endTime) return '-'
  const start = dayjs(startTime)
  const end = dayjs(endTime)
  const diffSeconds = end.diff(start, 'second')

  if (diffSeconds < 60) return `${diffSeconds}s`
  if (diffSeconds < 3600) {
    const minutes = Math.floor(diffSeconds / 60)
    const seconds = diffSeconds % 60
    return `${minutes}m ${seconds}s`
  }
  const hours = Math.floor(diffSeconds / 3600)
  const minutes = Math.floor((diffSeconds % 3600) / 60)
  return `${hours}h ${minutes}m`
}

const fetchData = async () => {
  try {
    loading.value = true
    const res = await getEvaluationTasks({
      workspace: workspaceStore.currentWorkspaceId || undefined,
      status: statusSelectedIds.value[0] || undefined,
      serviceId: serviceId.value || undefined,
      limit: pageSize.value,
      offset: (currentPage.value - 1) * pageSize.value,
    })
    items.value = res.items || []
    totalCount.value = res.totalCount || 0
  } catch (error) {
    console.error('Failed to fetch evaluation tasks:', error)
    ElMessage.error('Failed to load evaluation tasks')
  } finally {
    loading.value = false
  }
}

const handleFilterChange = () => {
  currentPage.value = 1
  fetchData()
}

const handleTableFilterChange = (filters: Record<string, EvaluationStatus[]>) => {
  if (Object.prototype.hasOwnProperty.call(filters, 'status')) {
    statusSelectedIds.value = filters.status || []
  }
  currentPage.value = 1
  fetchData()
}

const debouncedSearch = debounce(() => {
  handleFilterChange()
}, 300)

const handlePageChange = () => {
  fetchData()
}

const onDelete = (taskId: string, taskName?: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete evaluation task: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, taskName || taskId),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete evaluation task', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteEvaluationTask(taskId)
      ElMessage.success('Delete completed')
      fetchData()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const handleCreateSuccess = () => {
  currentPage.value = 1
  fetchData()
}

const onClone = (taskId: string) => {
  cloneTaskId.value = taskId
  showCreateDialog.value = true
}

const handleDialogClose = () => {
  cloneTaskId.value = ''
}

const onStop = async (taskId: string, taskName?: string) => {
  const msg = h('span', null, [
    'Are you sure you want to stop evaluation task: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, taskName || taskId),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, 'Stop evaluation task', {
      confirmButtonText: 'Stop',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await stopEvaluationTask(taskId)
    ElMessage.success('Stop complete')
    fetchData()
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error('Failed to stop task')
    }
  }
}

onMounted(async () => {
  try {
    await workspaceStore.fetchWorkspace()
  } catch (error) {
    console.error('Failed to load workspace list:', error)
  }
  fetchData()

  // Check for clone query parameter
  const cloneId = route.query.clone as string | undefined
  if (cloneId) {
    cloneTaskId.value = cloneId
    showCreateDialog.value = true
    // Clear query parameter
    router.replace({ query: {} })
  }
})

watch(
  () => workspaceStore.currentWorkspaceId,
  () => {
    currentPage.value = 1
    fetchData()
  },
)

defineOptions({
  name: 'EvaluationPage',
})
</script>

<style></style>
