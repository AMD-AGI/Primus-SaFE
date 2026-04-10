<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <el-text class="block textx-18 font-500" tag="b">SandBox</el-text>

  <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
    <div class="flex flex-wrap items-center gap-2">
      <el-segmented
        v-model="searchParams.onlyMyself"
        :options="['All', 'My Workloads']"
        @change="filterByMyself"
        class="myself-seg"
        style="background: none"
      />
    </div>

    <div class="flex flex-wrap items-center">
      <el-date-picker
        v-model="searchParams.dateRange"
        size="default"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start date"
        end-placeholder="End date"
        clearable
        class="mr-3"
        @change="onSearch({ resetPage: true })"
      />
      <el-input
        v-model="searchParams.workloadId"
        size="default"
        placeholder="Name/ID"
        style="width: 200px"
        clearable
        class="mr-3"
        @keyup.enter="onSearch({ resetPage: true })"
        @clear="onSearch({ resetPage: true })"
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
              const { onlyMyself, userId } = searchParams
              Object.assign(searchParams, initialSearchParams, { onlyMyself, userId })
              pagination.page = 1
              onSearch({ resetPage: true })
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
    </div>
  </div>

  <el-card class="mt-4 safe-card" shadow="never">
    <div class="table-wrap">
      <el-table
        :height="tableHeight"
        :data="tableData"
        @selection-change="onSelectionChange"
        ref="tableRef"
        size="large"
        class="m-t-2"
        v-loading="loading"
        :element-loading-text="$loadingText"
        @filter-change="handleFilterChange"
      >
        <el-table-column type="selection" width="56" />
        <el-table-column prop="workloadId" label="Name/ID" min-width="200" :fixed="true">
          <template #default="{ row }">
            <div class="flex flex-col items-start">
              <div class="flex items-center">
                <el-link
                  type="primary"
                  v-route="{ path: '/sandbox-workload/detail', query: { id: row.workloadId } }"
                >{{ row.displayName }}</el-link>
                <el-tooltip
                  v-if="row.secondsUntilTimeout > 0"
                  :content="`${row.secondsUntilTimeout} seconds remaining until the task ends`"
                  placement="top"
                >
                  <el-icon
                    :class="[
                      'cursor-default transition',
                      row.secondsUntilTimeout < 3600 ? 'text-red-500' : 'text-yellow-500',
                    ]"
                    class="ml-1 mt-1"
                  >
                    <Timer />
                  </el-icon>
                </el-tooltip>
              </div>
              <div class="text-[13px] text-gray-400">
                {{ row.workloadId }}
                <el-icon
                  class="cursor-pointer hover:text-blue-500 transition"
                  size="11"
                  @click="copyText(row.workloadId)"
                >
                  <CopyDocument />
                </el-icon>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          prop="phase"
          label="Phase"
          width="140"
          column-key="phase"
          :filters="phaseFilters"
          :filter-multiple="true"
          :filtered-value="searchParams.phase"
          :filter-method="passAll"
        >
          <template #default="{ row }">
            <el-tag
              :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'"
              :effect="isDark ? 'plain' : 'light'"
            >{{ row.phase }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="replica" label="Replicas" min-width="90">
          <template #default="{ row }">
            {{
              (row.resources?.[0]?.replica || 0) + (row.resources?.[1]?.replica || 0) ||
              row.resource?.replica || 0
            }}
          </template>
        </el-table-column>

        <el-table-column prop="priority" label="Priority" min-width="100">
          <template #default="{ row }">
            {{ PRIORITY_LABEL_MAP[row.priority as PriorityValue] }}
          </template>
        </el-table-column>

        <el-table-column prop="userName" label="User" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ row.userName || '-' }}</template>
        </el-table-column>

        <el-table-column prop="runTime" label="Duration" min-width="120">
          <template #default="{ row }">{{ row.duration || '-' }}</template>
        </el-table-column>

        <el-table-column prop="description" label="Description" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.description || '-' }}</template>
        </el-table-column>

        <el-table-column prop="creationTime" label="Creation Time" width="180">
          <template #default="{ row }">{{ formatTimeStr(row.creationTime) }}</template>
        </el-table-column>

        <el-table-column prop="endTime" label="End Time" width="180">
          <template #default="{ row }">{{ formatTimeStr(row.endTime) }}</template>
        </el-table-column>

        <el-table-column prop="resource" label="Resource" min-width="280">
          <template #default="{ row }">
            <span class="res-line">
              <span class="t">{{ (row.resources?.[0] || row.resource)?.gpu ?? 0 }} card</span>
              <span class="sep">*</span>
              <span class="t">{{ (row.resources?.[0] || row.resource)?.cpu }} core</span>
              <span class="sep">*</span>
              <span class="t">{{ (row.resources?.[0] || row.resource)?.memory }}</span>
            </span>
          </template>
        </el-table-column>

        <el-table-column label="Actions" width="160" fixed="right">
          <template #default="{ row }">
            <el-tooltip content="Detail" placement="top">
              <el-button
                circle size="default" class="btn-primary-plain" :icon="View"
                @click="router.push({ path: '/sandbox-workload/detail', query: { id: row.workloadId } })"
              />
            </el-tooltip>
            <el-tooltip content="Edit" placement="top">
              <el-button
                circle size="default" class="btn-primary-plain" :icon="Edit"
                :disabled="!['Running', 'Pending'].includes(row.phase)"
                @click="openEdit(row.workloadId)"
              />
            </el-tooltip>
            <el-tooltip content="Stop" placement="top">
              <el-button
                circle size="default" class="btn-danger-plain" :icon="Close"
                @click="handleStop(row)"
              />
            </el-tooltip>
            <el-tooltip content="Delete" placement="top">
              <el-button
                circle size="default" class="btn-danger-plain" :icon="Delete"
                @click="handleDelete(row)"
              />
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <transition name="slide-up" @after-leave="onBarAfterLeave">
        <div v-if="selectedRows.length" class="selection-bar">
          <div class="left">
            <span class="ml-2">Selected {{ selectedRows.length }} item{{ selectedRows.length === 1 ? '' : 's' }}</span>
          </div>
          <div class="right">
            <el-button type="danger" plain @click="onBatchDelete">Delete</el-button>
            <el-button type="warning" plain @click="onBatchStop">Stop</el-button>
          </div>
        </div>
      </transition>

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
    </div>
  </el-card>

  <EditDialog
    v-model:visible="editVisible"
    :workload-id="editWlId"
    @success="onSearch({ resetPage: false })"
  />
</template>

<script lang="ts" setup>
import { ref, reactive, watch, computed, onMounted, onBeforeUnmount, h } from 'vue'
import {
  getWorkloadsList,
  deleteWorkload,
  stopWorkload,
  batchDelWorkload,
  batchStopWorkload,
} from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  WorkloadKind,
  WorkloadPhase,
  phaseFilters,
  WorkloadPhaseButtonType,
} from '@/services/workload/type'
import { Search, Refresh, CopyDocument, Timer, View, Edit, Delete, Close } from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
import { copyText, formatTimeStr } from '@/utils/index'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { type WorkloadParams, type PriorityValue, PRIORITY_LABEL_MAP } from '@/services'
import { useUserStore } from '@/stores/user'
import { useDark } from '@vueuse/core'
import EditDialog from './Components/EditDialog.vue'

dayjs.extend(utc)

const tableRef = ref()
const isDark = useDark()
const route = useRoute()
const router = useRouter()
const store = useWorkspaceStore()
const userStore = useUserStore()

const editVisible = ref(false)
const editWlId = ref('')

const initialSearchParams = {
  userName: '',
  description: '',
  phase: [] as WorkloadPhase[],
  dateRange: '',
  workloadId: '',
  onlyMyself: 'My Workloads',
  userId: userStore.userId,
}
const searchParams = reactive({ ...initialSearchParams })

const loading = ref(false)
const tableData = ref([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const SELECTION_BAR_H = 56
const BASE_OFFSET = 245

const selectedRows = ref<Array<Record<string, any>>>([])
function onSelectionChange(rows: Array<Record<string, any>>) {
  selectedRows.value = rows
}

const hasSelection = computed(() => selectedRows.value.length > 0)
const hasBarSpace = ref(false)
watch(hasSelection, (v) => { if (v) hasBarSpace.value = true })
function onBarAfterLeave() { hasBarSpace.value = false }
const tableHeight = computed(() => {
  const extra = hasBarSpace.value ? SELECTION_BAR_H : 0
  return `calc(100vh - ${BASE_OFFSET + extra}px)`
})

const openEdit = (id: string) => {
  editWlId.value = id
  editVisible.value = true
}

const fetchData = async (params?: WorkloadParams) => {
  try {
    loading.value = true
    if (!params?.phase) tableRef.value?.clearFilter(['phase'])

    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      kind: WorkloadKind.Sandbox,
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

const passAll = () => true
const handleFilterChange = (filters: Record<string, string[]>) => {
  if (Object.prototype.hasOwnProperty.call(filters, 'phase')) {
    searchParams.phase = (filters.phase || []) as WorkloadPhase[]
    onSearch({ resetPage: true })
  }
}

const filterByMyself = () => {
  searchParams.userId = searchParams.onlyMyself !== 'All' ? userStore.userId : ''
  onSearch({ resetPage: true })
}

const onSearch = (options?: { resetPage?: boolean }) => {
  const reset = options?.resetPage ?? true
  if (reset) pagination.page = 1

  const [start, end] = searchParams.dateRange ?? []
  const since = start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const until = end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const phaseStr = searchParams.phase?.length ? searchParams.phase.join(',') : ''

  router.replace({
    query: {
      ...route.query,
      userName: searchParams.userName || undefined,
      phase: phaseStr || undefined,
      since: since || undefined,
      until: until || undefined,
      workloadId: searchParams.workloadId || undefined,
      onlyMyself: searchParams.onlyMyself || undefined,
      userId: searchParams.userId,
      page: String(pagination.page),
      pageSize: String(pagination.pageSize),
    },
  })

  fetchData({
    userName: searchParams.userName,
    description: searchParams.description,
    phase: phaseStr,
    since,
    until,
    workloadId: searchParams.workloadId,
    userId: searchParams.userId,
  })
}

const handleStop = async (row: any) => {
  const msg = h('span', null, [
    'Are you sure you want to stop workload: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.workloadId),
    ' ?',
  ])
  await ElMessageBox.confirm(msg, 'Stop workload', {
    confirmButtonText: 'Stop',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
  await stopWorkload(row.workloadId)
  ElMessage.success('Stop complete')
  onSearch({ resetPage: false })
}

const handleDelete = async (row: any) => {
  const msg = h('span', null, [
    'Are you sure you want to delete workload: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.workloadId),
    ' ?',
  ])
  await ElMessageBox.confirm(msg, 'Delete workload', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
  await deleteWorkload(row.workloadId)
  ElMessage.success('Deleted')
  onSearch({ resetPage: false })
}

function previewIds(ids: string[], max = 5) {
  return ids.length <= max ? ids.join(', ') : `${ids.slice(0, max).join(', ')} +${ids.length - max} more`
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

async function onBatch(action: 'delete' | 'stop') {
  const ids = selectedRows.value.map((r) => r.workloadId).filter(Boolean)
  if (!ids.length) return ElMessage.warning('Please select at least one workload')

  const apiFn = action === 'delete' ? batchDelWorkload : batchStopWorkload
  const msg = h('span', null, [
    `Are you sure you want to ${action} workloads: `,
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, previewIds(ids)),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, `${cap(action)} workloads`, {
      confirmButtonText: cap(action),
      cancelButtonText: 'Cancel',
      type: action === 'delete' ? 'warning' : 'info',
    })
    await apiFn({ workloadIds: ids })
    ElMessage.success(`${cap(action)} completed`)
    await onSearch({ resetPage: false })
    selectedRows.value = []
  } catch (err: any) {
    if (err !== 'cancel' && err !== 'close' && err?.message) {
      ElMessage.error(err.message)
    }
  }
}

const onBatchDelete = () => onBatch('delete')
const onBatchStop = () => onBatch('stop')

defineOptions({ name: 'SandboxWorkloadPage' })

function applyQueryToParams() {
  const q = route.query
  searchParams.userName = (q.userName as string) || ''
  searchParams.description = (q.description as string) || ''
  searchParams.workloadId = (q.workloadId as string) || ''
  searchParams.onlyMyself = (q.onlyMyself as string) || 'My Workloads'
  searchParams.userId = (q.userId as string) || ''

  const phaseStr = (q.phase as string) || ''
  searchParams.phase = phaseStr ? (phaseStr.split(',') as WorkloadPhase[]) : []

  const since = q.since as string | undefined
  const until = q.until as string | undefined
  if (since || until) {
    searchParams.dateRange = [
      since ? dayjs(since).toDate() : '',
      until ? dayjs(until).toDate() : '',
    ] as any
  } else {
    searchParams.dateRange = ''
  }

  pagination.page = Number(q.page || 1)
  pagination.pageSize = Number(q.pageSize || pagination.pageSize)
}

watch(
  () => store.currentWorkspaceId,
  (id) => {
    if (id) {
      applyQueryToParams()
      if (!searchParams.userId && searchParams.onlyMyself === 'My Workloads') {
        searchParams.userId = userStore.userId
      }
      onSearch({ resetPage: false })
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.res-line {
  display: inline-flex;
  align-items: baseline;
  gap: 0;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.t { letter-spacing: 0.2px; }
.sep { opacity: 0.55; margin: 0 0.6ch; font-weight: 700; }

.selection-bar {
  position: sticky;
  bottom: 0;
  z-index: 1;
  height: 56px;
  padding: 0 16px;
  background: var(--el-bg-color);
  border-top: 1px solid var(--el-border-color);
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 -6px 12px rgba(0, 0, 0, 0.06);
  gap: 12px;
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: transform 0.18s ease, opacity 0.18s ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}
</style>
<style>
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}
</style>
