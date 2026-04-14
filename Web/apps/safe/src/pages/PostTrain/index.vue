<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <el-text class="block textx-18 font-500" tag="b">Post Train</el-text>

  <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        @click="showCreateDialog = true"
        class="text-black"
      >
        Create Training
      </el-button>
    </div>

    <!-- Filters -->
    <div class="flex flex-wrap items-center gap-2">
      <el-input v-model="filters.baseModel" placeholder="Base Model" clearable style="width: 140px" @keyup.enter="onSearch({ resetPage: true })" @clear="onSearch({ resetPage: true })" />
      <el-input v-model="filters.owner" placeholder="Owner" clearable style="width: 120px" @keyup.enter="onSearch({ resetPage: true })" @clear="onSearch({ resetPage: true })" />
      <el-date-picker
        v-model="filters.dateRange"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start"
        end-placeholder="End"
        clearable
        @change="onSearch({ resetPage: true })"
      />
      <el-button :icon="Search" type="primary" @click="onSearch({ resetPage: true })" />
      <el-tooltip content="Reset" placement="top">
        <el-button :icon="RefreshIcon" @click="resetFilters" />
      </el-tooltip>
      <el-tooltip content="Refresh" placement="top">
        <el-button :icon="Refresh" @click="onSearch({ resetPage: false })" />
      </el-tooltip>
    </div>
  </div>

  <!-- Table -->
  <el-card class="mt-4 safe-card" shadow="never">
    <el-table
      ref="tableRef"
      :data="tableData"
      :height="tableHeight"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"

      @filter-change="handleFilterChange"
    >
      <el-table-column prop="displayName" label="Run Name / ID" min-width="260" :fixed="true">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" @click.stop="goDetail(row)">{{ row.displayName }}</el-link>
            <span class="text-[13px] text-gray-400">{{ row.runId }}</span>
          </div>
        </template>
      </el-table-column>

      <el-table-column
        prop="trainType"
        label="Type"
        width="100"
        column-key="trainType"
        :filters="trainTypeFilters"
        :filter-multiple="false"
        :filtered-value="filters.trainType ? [filters.trainType] : []"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag size="small" :type="row.trainType === 'sft' ? 'success' : 'warning'" effect="plain">
            {{ row.trainType?.toUpperCase() }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column
        prop="strategy"
        label="Strategy"
        width="130"
        column-key="strategy"
        :filters="strategyFilters"
        :filter-multiple="false"
        :filtered-value="filters.strategy ? [filters.strategy] : []"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">{{ row.strategy || '-' }}</template>
      </el-table-column>

      <el-table-column prop="baseModelName" label="Base Model" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ row.baseModelName || '-' }}</template>
      </el-table-column>

      <el-table-column prop="datasetName" label="Dataset" min-width="150" show-overflow-tooltip>
        <template #default="{ row }">{{ row.datasetName || '-' }}</template>
      </el-table-column>

      <el-table-column
        prop="workspace"
        label="Workspace"
        min-width="180"
        show-overflow-tooltip
        column-key="workspace"
        :filters="wsFilters"
        :filter-multiple="false"
        :filtered-value="filters.workspace ? [filters.workspace] : []"
        filter-placement="bottom-start"
        :filter-method="passAll"
      />

      <el-table-column
        prop="status"
        label="Status"
        width="140"
        column-key="status"
        :filters="statusFilters"
        :filter-multiple="true"
        :filtered-value="filters.status ? [filters.status] : []"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag size="small" :type="statusTagType(row.status)" :effect="isDark ? 'plain' : 'light'">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column label="Nodes x GPUs" width="120">
        <template #default="{ row }">
          {{ row.nodeCount ?? '-' }} x {{ row.gpuPerNode ?? '-' }}
        </template>
      </el-table-column>

      <el-table-column prop="parameterSummary" label="Key Params" min-width="240" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="font-mono text-xs">{{ row.parameterSummary || '-' }}</span>
        </template>
      </el-table-column>

      <el-table-column prop="latestLoss" label="Latest Loss" width="110">
        <template #default="{ row }">
          {{ row.latestLoss != null ? Number(row.latestLoss).toFixed(4) : '-' }}
        </template>
      </el-table-column>

      <el-table-column label="Output" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <template v-if="row.modelId">
            <el-link type="primary" @click.stop="router.push(`/model-square/detail/${row.modelId}`)">
              {{ row.modelDisplayName || row.modelId }}
              <el-tag v-if="row.modelPhase" size="small" class="ml-1" effect="plain">{{ row.modelPhase }}</el-tag>
            </el-link>
          </template>
          <span v-else>{{ row.outputPath ? 'Exported' : '-' }}</span>
        </template>
      </el-table-column>

      <el-table-column prop="createdAt" label="Created" width="170">
        <template #default="{ row }">{{ formatTimeStr(row.createdAt) }}</template>
      </el-table-column>

      <el-table-column prop="duration" label="Duration" width="110">
        <template #default="{ row }">{{ row.duration || '-' }}</template>
      </el-table-column>

      <el-table-column prop="userName" label="Owner" width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.userName || '-' }}</template>
      </el-table-column>

      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <template v-for="act in getActions(row).slice(0, 2)" :key="act.key">
            <el-tooltip :content="act.label" placement="top">
              <el-button
                circle
                size="default"
                :class="act.btnClass"
                :icon="act.icon"
                @click.stop="act.onClick(row)"
              />
            </el-tooltip>
          </template>

          <el-popover
            v-if="getActions(row).length > 2"
            placement="bottom-start"
            trigger="click"
            :width="240"
            :teleported="true"
            :enterable="true"
            popper-class="actions-menu"
            :visible="moreOpenId === row.runId"
            @hide="moreOpenId === row.runId && (moreOpenId = null)"
          >
            <template #reference>
              <el-button
                circle
                class="btn-primary-plain"
                :icon="MoreFilled"
                size="default"
                @click.stop="toggleMore(row.runId)"
              />
            </template>

            <ul class="menu-col">
              <li
                v-for="act in getActions(row).slice(2)"
                :key="act.key"
                class="menu-item"
                @click.stop="handleMenuClick(act, row)"
              >
                <component :is="act.icon" class="menu-ico" />
                <span class="menu-label">{{ act.label }}</span>
              </li>
            </ul>
          </el-popover>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="m-t-2"
      :current-page="pagination.page"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      @current-change="(p: number) => { pagination.page = p; onSearch({ resetPage: false }) }"
      @size-change="(s: number) => { pagination.pageSize = s; pagination.page = 1; onSearch({ resetPage: false }) }"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[20, 50, 100]"
    />
  </el-card>

  <!-- Create Training Dialog (reuse from ModelSquare) -->
  <CreateTrainingDialog
    v-model:visible="showCreateDialog"
    :model="null"
    @success="handleCreateSuccess"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, nextTick, h, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, Refresh, Delete, Monitor, MoreFilled } from '@element-plus/icons-vue'
import RefreshIcon from '@/components/icons/ResetIcon.vue'
import { useDark } from '@vueuse/core'
import { formatTimeStr } from '@/utils'
import { getPostTrainRuns, deletePostTrainRun } from '@/services/posttrain'
import type { PostTrainRunItem, PostTrainListParams } from '@/services/posttrain'
import { useWorkspaceStore } from '@/stores/workspace'
import CreateTrainingDialog from '@/pages/ModelSquare/Components/CreateTrainingDialog.vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'

dayjs.extend(utc)

const router = useRouter()
const isDark = useDark()
const wsStore = useWorkspaceStore()

const loading = ref(false)
const tableData = ref<PostTrainRunItem[]>([])
const tableRef = ref()
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const showCreateDialog = ref(false)

const workspaceItems = wsStore.items ?? []

const filters = reactive({
  trainType: '',
  strategy: '',
  status: '',
  workspace: '',
  baseModel: '',
  owner: '',
  dateRange: '' as any,
})

const trainTypeFilters = [
  { text: 'SFT', value: 'sft' },
  { text: 'RL', value: 'rl' },
]

const strategyFilters = [
  { text: 'full', value: 'full' },
  { text: 'lora', value: 'lora' },
  { text: 'fsdp2', value: 'fsdp2' },
  { text: 'megatron', value: 'megatron' },
]

const statusFilters = [
  { text: 'Pending', value: 'Pending' },
  { text: 'Running', value: 'Running' },
  { text: 'Succeeded', value: 'Succeeded' },
  { text: 'Failed', value: 'Failed' },
  { text: 'Stopped', value: 'Stopped' },
]

const wsFilters = workspaceItems.map((ws) => ({
  text: ws.workspaceName,
  value: ws.workspaceId,
}))

const passAll = () => true

const handleFilterChange = (columnFilters: Record<string, string[]>) => {
  if ('trainType' in columnFilters) {
    filters.trainType = columnFilters.trainType?.[0] || ''
  }
  if ('strategy' in columnFilters) {
    filters.strategy = columnFilters.strategy?.[0] || ''
  }
  if ('status' in columnFilters) {
    filters.status = columnFilters.status?.[0] || ''
  }
  if ('workspace' in columnFilters) {
    filters.workspace = columnFilters.workspace?.[0] || ''
  }
  onSearch({ resetPage: true })
}

const statusTagType = (status: string) => {
  const map: Record<string, string> = {
    Running: 'primary', Succeeded: 'success', Failed: 'danger', Pending: 'info', Stopped: 'info',
  }
  return map[status] || 'info'
}

type Action = {
  key: string
  label: string
  icon: Component
  btnClass: string
  onClick: (row: PostTrainRunItem) => void | Promise<void>
}

const getActions = (row: PostTrainRunItem): Action[] => {
  const actions: Action[] = [
    {
      key: 'workload',
      label: 'Workload',
      icon: Monitor,
      btnClass: 'btn-primary-plain',
      onClick: (r) => goWorkload(r),
    },
    {
      key: 'delete',
      label: 'Delete Record',
      icon: Delete,
      btnClass: 'btn-danger-plain',
      onClick: (r) => handleDelete(r),
    },
  ]
  return actions
}

const moreOpenId = ref<string | null>(null)

const toggleMore = async (id: string) => {
  if (moreOpenId.value === id) {
    moreOpenId.value = null
    return
  }
  moreOpenId.value = null
  await nextTick()
  moreOpenId.value = id
}

const closeMore = () => {
  moreOpenId.value = null
}

const handleMenuClick = async (act: Action, row: PostTrainRunItem) => {
  await act.onClick(row)
  closeMore()
}

const fetchData = async () => {
  loading.value = true
  try {
    const [start, end] = filters.dateRange ?? []
    const params: PostTrainListParams = {
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
    }
    if (filters.trainType) params.trainType = filters.trainType
    if (filters.strategy) params.strategy = filters.strategy
    if (filters.status) params.status = filters.status
    if (filters.workspace) params.workspace = filters.workspace
    if (filters.baseModel) params.baseModel = filters.baseModel
    if (filters.owner) params.owner = filters.owner
    if (start) params.since = dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]')
    if (end) params.until = dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]')

    const res = (await getPostTrainRuns(params)) as unknown as { total: number; items: PostTrainRunItem[] }
    tableData.value = res.items || []
    pagination.total = res.total || 0
  } catch {
    ElMessage.error('Failed to load training runs')
  } finally {
    loading.value = false
  }
}

const onSearch = (opts?: { resetPage?: boolean }) => {
  if (opts?.resetPage) pagination.page = 1
  fetchData()
}

const tableHeight = computed(() => `calc(100vh - 245px)`)

const resetFilters = () => {
  Object.assign(filters, { trainType: '', strategy: '', status: '', workspace: '', baseModel: '', owner: '', dateRange: '' })
  tableRef.value?.clearFilter()
  pagination.page = 1
  fetchData()
}

const goDetail = (row: PostTrainRunItem) => {
  router.push({ path: '/posttrain/detail', query: { runId: row.runId } })
}

const goWorkload = (row: PostTrainRunItem) => {
  const path = row.trainType === 'rl' ? '/rayjob/detail' : '/training/detail'
  router.push({ path, query: { id: row.workloadId } })
}

const handleDelete = async (row: PostTrainRunItem) => {
  const msg = h('span', null, [
    'Delete training record ',
    h('b', null, row.displayName),
    '? This only removes the record, not the actual workload.',
  ])
  try {
    await ElMessageBox.confirm(msg, 'Delete Record', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await deletePostTrainRun(row.runId)
    ElMessage.success('Record deleted')
    onSearch({ resetPage: false })
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error('Failed to delete record')
    }
  }
}

const handleCreateSuccess = (workloadId: string) => {
  showCreateDialog.value = false
  router.push({ path: '/training/detail', query: { id: workloadId } })
}

defineOptions({ name: 'PostTrainPage' })

const onAnyScroll = () => closeMore()
const onAnyPointerDown = (e: Event) => {
  const el = e.target as HTMLElement
  if (!el.closest('.actions-menu') && !el.closest('.btn-primary-plain')) closeMore()
}

onMounted(() => {
  fetchData()
  window.addEventListener('scroll', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('wheel', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('touchmove', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('pointerdown', onAnyPointerDown, { capture: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as any)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as any)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as any)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as any)
})
</script>

<style scoped>
.font-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
