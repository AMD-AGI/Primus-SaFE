<template>
  <el-text class="block textx-18 font-500" tag="b">RayJob</el-text>

  <div class="flex flex-wrap items-center mt-4">
    <!-- Left actions -->
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        @click="
          () => {
            addVisible = true
            curWlId = ''
            curAction = 'Create'
          }
        "
        class="mb-2 text-black"
      >
        Create RayJob
      </el-button>
      <el-segmented
        v-model="searchParams.onlyMyself"
        :options="['All', 'My Workloads']"
        @change="filterByMyself"
        class="myself-seg ml-2 mt-2 sm:mt-0 mb-2"
        style="background: none"
      />
    </div>

    <!-- Right search, aligned right -->
    <div class="flex flex-wrap items-center mt-2 mb-2 sm:mt-0 ml-auto">
      <el-date-picker
        v-model="searchParams.dateRange"
        size="default"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start date"
        end-placeholder="End date"
        class="mr-3"
        clearable
        @change="onSearch({ resetPage: true })"
      />
      <el-input
        v-model="searchParams.workloadId"
        size="default"
        placeholder="Name/ID"
        style="width: 200px"
        class="mr-3"
        clearable
        @keyup.enter="onSearch({ resetPage: true })"
        @clear="onSearch({ resetPage: true })"
      />
      <el-button
        :icon="Search"
        size="default"
        type="primary"
        @click="onSearch({ resetPage: true })"
      ></el-button>
      <el-button
        :icon="Refresh"
        size="default"
        @click="
          () => {
            Object.assign(searchParams, initialSearchParams)
            pagination.page = 1
            onSearch({ resetPage: true })
          }
        "
      ></el-button>
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
                <el-link type="primary" @click="jumpToDetail(row.workloadId)">{{
                  row.displayName
                }}</el-link>
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
          width="160"
          column-key="phase"
          :filters="phaseFilters"
          :filter-multiple="true"
          :filtered-value="searchParams.phase"
          :filter-method="passAll"
        >
          <template #default="{ row }">
            <div class="flex flex-col gap-1">
              <div class="flex items-center gap-2">
                <el-tag
                  :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'"
                  :effect="isDark ? 'plain' : 'light'"
                  >{{ row.phase }}</el-tag
                >
                <el-tooltip
                  v-if="row.phase === 'Pending'"
                  :content="row.message ? `${row.message} - Click for Pending Cause Analysis` : 'Pending Cause Analysis'"
                  placement="top"
                >
                  <el-icon
                    class="pending-cause-icon"
                    :size="18"
                    @click.stop="
                      router.push({ path: '/workload/pending-cause', query: { id: row.workloadId } })
                    "
                  >
                    <InfoFilled />
                  </el-icon>
                </el-tooltip>
              </div>
              <div
                class="text-sm"
                v-if="row.phase === 'Pending' && !!row.queuePosition"
              >
                position in queue:{{ row.queuePosition }}
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="replica" label="Replicas" min-width="90">
          <template #default="{ row }">
            {{ getTotalReplicas(row) }}
          </template>
        </el-table-column>
        <el-table-column prop="priority" label="Priority" min-width="100">
          <template #default="{ row }">
            {{ PRIORITY_LABEL_MAP[row.priority as PriorityValue] }}
          </template>
        </el-table-column>
        <el-table-column prop="userName" label="User" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.userName || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="runTime" label="Duration" min-width="120">
          <template #default="{ row }">
            {{ row.duration || '-' }}
          </template>
        </el-table-column>
        <el-table-column
          prop="description"
          label="Description"
          min-width="180"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            {{ row.description || '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="creationTime" label="Creation Time" width="180">
          <template #default="{ row }">
            {{ formatTimeStr(row.creationTime) }}
          </template>
        </el-table-column>
        <el-table-column prop="endTime" label="End Time" width="180">
          <template #default="{ row }">
            {{ formatTimeStr(row.endTime) }}
          </template>
        </el-table-column>
        <el-table-column prop="dispatchCount" label="Dispatch Count" width="130" />

        <el-table-column prop="resource" label="Resource" min-width="280">
          <template #default="{ row }">
            <div class="flex flex-col gap-1">
              <template v-if="Array.isArray(row.resources) && row.resources.length">
                <div v-for="(r, i) in row.resources" :key="i" class="flex items-center gap-2">
                  <!-- <span class="text-xs text-gray-400 whitespace-nowrap">
                    {{ i === 0 ? 'Header' : `Worker ${i}` }}
                  </span> -->
                  <span class="res-line">
                    <span class="t">{{ r?.gpu ?? 0 }} card</span>
                    <span class="sep">*</span>
                    <span class="t">{{ r?.cpu }} core</span>
                    <span class="sep">*</span>
                    <span class="t">{{ r?.memory }}</span>
                  </span>
                </div>
              </template>
              <template v-else>
                <span class="res-line">
                  <span class="t">{{ row.resource?.gpu ?? 0 }} card</span>
                  <span class="sep">*</span>
                  <span class="t">{{ row.resource?.cpu }} core</span>
                  <span class="sep">*</span>
                  <span class="t">{{ row.resource?.memory }}</span>
                </span>
              </template>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="avgGpuUsage" label="Avg GPU Utilization(3h)" min-width="170">
          <template #default="{ row }">
            <el-link
              type="primary"
              :disabled="row.avgGpuUsage === -1"
              @click="row.avgGpuUsage !== -1 && viewChart(row)"
            >
              {{ row.avgGpuUsage === -1 ? '-' : formatPct(row.avgGpuUsage) }}
            </el-link>
          </template>
        </el-table-column>

        <el-table-column label="Actions" width="180" fixed="right">
          <template #default="{ row }">
            <!-- First 2 inline actions -->
            <template v-for="act in getActions(row).slice(0, 2)" :key="act.key">
              <el-tooltip :content="act.label" placement="top">
                <el-button
                  circle
                  size="default"
                  :class="act.btnClass"
                  :icon="act.icon"
                  :disabled="act.disabled?.(row) ?? false"
                  @click="act.onClick(row)"
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
              :visible="moreOpenId === row.workloadId"
              @hide="moreOpenId === row.workloadId && (moreOpenId = null)"
            >
              <template #reference>
                <el-button
                  circle
                  class="btn-primary-plain"
                  :icon="MoreFilled"
                  size="default"
                  @click.stop="toggleMore(row.workloadId)"
                />
              </template>

              <ul class="menu-col">
                <li
                  v-for="act in getActions(row).slice(2)"
                  :key="act.key"
                  :class="['menu-item', { disabled: act.disabled?.(row) }]"
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

      <!-- Floating bottom action bar -->
      <transition name="slide-up" @after-leave="onBarAfterLeave">
        <div v-if="selectedRows.length" class="selection-bar">
          <div class="left">
            <span class="ml-2"
              >Selected {{ selectedRows.length }} item{{
                selectedRows.length === 1 ? '' : 's'
              }}</span
            >
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

  <AddDialog
    v-model:visible="addVisible"
    :wlid="curWlId"
    :action="curAction"
    @success="onSearch({ resetPage: false })"
  />

  <UtilDialog
    v-model:visible="dialog.visible"
    :loading="dialog.loading"
    :series="dialog.series"
    :title="dialog.title"
  />
</template>
<script lang="ts" setup>
import { ref, reactive, watch, nextTick, onMounted, onBeforeUnmount, h, computed } from 'vue'
import {
  getWorkloadsList,
  deleteWorkload,
  stopWorkload,
  batchDelWorkload,
  batchStopWorkload,
  getLensHourlyStats,
  getWorkloadDetail,
  addWorkload,
} from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'
import { WorkloadPhase, phaseFilters, WorkloadPhaseButtonType } from '@/services/workload/type'
import { Search, Refresh, CopyDocument, Plus, Timer, InfoFilled } from '@element-plus/icons-vue'
import { copyText, formatTimeStr, last24hUtcExact } from '@/utils/index'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useRoute, useRouter } from 'vue-router'
import { useRouteAction, ROUTE_ACTIONS } from '@/composables/useRouteAction'
import AddDialog from './Components/AddDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, DocumentCopy, Edit, Close, MoreFilled, VideoPlay } from '@element-plus/icons-vue'
import { type WorkloadParams, type PriorityValue, PRIORITY_LABEL_MAP } from '@/services'
import { useUserStore } from '@/stores/user'
import { useDark } from '@vueuse/core'
import { useAutoRefreshUserInfo } from '@/composables/useAutoRefreshUserInfo'
import UtilDialog from './Components/UtilDialog.vue'

dayjs.extend(utc)

const tableRef = ref()
const isDark = useDark()
const route = useRoute()
const router = useRouter()
const store = useWorkspaceStore()
const userStore = useUserStore()

const getTotalReplicas = (row: unknown) => {
  const r = row as {
    resources?: Array<{ replica?: number | string } | null | undefined>
    resource?: { replica?: number | string } | null
  }
  const resources = Array.isArray(r.resources) ? r.resources : []
  if (resources.length) {
    return resources.reduce((sum, it) => sum + (Number(it?.replica) || 0), 0)
  }
  return Number(r.resource?.replica) || 0
}

// Auto-refresh user info on page enter (permission-sensitive page)
useAutoRefreshUserInfo({ immediate: true })

const addVisible = ref(false)
// typed search params to avoid `any` casts around dateRange
type DateRange = '' | [Date | '', Date | '']
type SearchParams = {
  userName: string
  description: string
  phase: WorkloadPhase[]
  dateRange: DateRange
  workloadId: string
  onlyMyself: string
  userId: string
}
const initialSearchParams: SearchParams = {
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
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const curWlId = ref()
const curAction = ref<'Create' | 'Edit' | 'Clone'>('Create')

const SELECTION_BAR_H = 56
const BASE_OFFSET = 245

// Multi-select
const selectedRows = ref<Array<Record<string, unknown>>>([])
function onSelectionChange(rows: Array<Record<string, unknown>>) {
  selectedRows.value = rows
}

// Batch action bar placeholder
const hasSelection = computed(() => selectedRows.value.length > 0)
const hasBarSpace = ref(false)
watch(hasSelection, (v) => {
  if (v) hasBarSpace.value = true // Reserve space when selected
})
function onBarAfterLeave() {
  hasBarSpace.value = false
}
const tableHeight = computed(() => {
  const extra = hasBarSpace.value ? SELECTION_BAR_H : 0
  return `calc(100vh - ${BASE_OFFSET + extra}px)`
})

const jumpToDetail = (id: string) => {
  router.push({ path: '/rayjob/detail', query: { id } })
}

// Line chart dialog
const dialog = reactive({
  visible: false,
  title: '',
  loading: false,
  series: { x: [] as string[], avg: [] as (number | null)[] },
})

const formatPct = (v: number | null | undefined) => {
  if (v == null) return '-'
  const pct = v > 1 ? v : v * 100
  return `${pct.toFixed(1)}%`
}

type ChartRow = {
  clusterId: string
  workspaceId: string
  workloadId: string
}
const viewChart = async (r: ChartRow) => {
  dialog.visible = true
  dialog.title = `${r.workspaceId}/${r.workloadId}`
  dialog.loading = true
  try {
    const { start_time, end_time } = last24hUtcExact()
    const res = await getLensHourlyStats({
      cluster: r.clusterId,
      namespace: r.workspaceId,
      workload_name: r.workloadId,
      order_by: 'time',
      order_direction: 'desc',
      start_time,
      end_time,
    })

    // Handle different response wrappers: extract array
    const rows = ((res?.data?.data ?? res?.data ?? res) as Array<Record<string, unknown>>) || []

    // Compute avg only, converting to local time and percentage
    const x = rows.map((it) => dayjs.utc(String(it.stat_hour)).local().format('MM-DD HH:mm'))
    const avg = rows.map((it) => {
      const v = it?.avg_utilization as number | null | undefined
      if (v == null) return null
      const pct = v > 1 ? v : v * 100
      return +pct.toFixed(2)
    })

    dialog.series = { x, avg }
  } finally {
    dialog.loading = false
  }
}
const fetchData = async (params?: WorkloadParams) => {
  try {
    loading.value = true

    if (!params?.phase) {
      tableRef.value?.clearFilter(['phase'])
    }

    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      kind: 'RayJob',
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
      description: searchParams.description || undefined,
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

type Row = {
  workloadId: string
  phase: string
  displayName?: string
  maxRetry?: number
  queuePosition?: number
}
type Action = {
  key: string
  label: string
  icon: unknown
  btnClass?: string
  disabled?: (row: Row) => boolean
  onClick: (row: Row) => void | Promise<void>
}

const getActions = (_row: Row) => {
  const actions: Action[] = [
    {
      key: 'clone',
      label: 'Clone',
      icon: DocumentCopy,
      btnClass: 'btn-success-plain',
      onClick: (r: Row) => {
        curAction.value = 'Clone'
        curWlId.value = r.workloadId
        addVisible.value = true
      },
    },
  ]

  actions.push(
    {
      key: 'edit',
      label: 'Edit',
      icon: Edit,
      btnClass: 'btn-primary-plain',
      disabled: (r: Row) => {
        const maxRetry = r.maxRetry ?? 0
        const phase = r.phase
        const queuePosition = r.queuePosition ?? 0

        if (maxRetry > 0) {
          // maxRetry > 0: editable when Pending or Running
          return !['Running', 'Pending'].includes(phase)
        } else {
          // maxRetry = 0: only editable when Pending and queuePosition > 0
          return !(phase === 'Pending' && queuePosition > 0)
        }
      },
      onClick: (r: Row) => {
        curAction.value = 'Edit'
        curWlId.value = r.workloadId
        addVisible.value = true
      },
    },

    {
      key: 'delete',
      label: 'Delete',
      icon: Delete,
      btnClass: 'btn-danger-plain',
      onClick: async (r: Row) => {
        const msg = h('span', null, [
          'Are you sure you want to delete workload: ',
          h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, r.workloadId),
          ' ?',
        ])

        await ElMessageBox.confirm(msg, 'Delete workload', {
          confirmButtonText: 'Delete',
          cancelButtonText: 'Cancel',
          type: 'warning',
        })
        await deleteWorkload(r.workloadId)
        ElMessage.success('Deleted')
        onSearch({ resetPage: false })
      },
    },
    {
      key: 'stop',
      label: 'Stop',
      icon: Close,
      btnClass: 'btn-danger-plain',
      onClick: async (r: Row) => {
        const msg = h('span', null, [
          'Are you sure you want to stop workload: ',
          h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, r.workloadId),
          ' ?',
        ])

        await ElMessageBox.confirm(msg, 'Stop workload', {
          confirmButtonText: 'Stop',
          cancelButtonText: 'Cancel',
          type: 'warning',
        })
        await stopWorkload(r.workloadId)
        ElMessage.success('Stop complete')
        onSearch({ resetPage: false })
      },
    },
  )

  return actions
}

type BatchAction = 'delete' | 'stop'

const apiMap: Record<BatchAction, (body: { workloadIds: string[] }) => Promise<unknown>> = {
  delete: batchDelWorkload,
  stop: batchStopWorkload,
}

const batchLoading = ref(false)

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

function previewIds(ids: string[], max = 5) {
  return ids.length <= max
    ? ids.join(', ')
    : `${ids.slice(0, max).join(', ')} +${ids.length - max} more`
}

async function onBatch(action: BatchAction) {
  const ids = selectedRows.value
    .map((r) => (r as { workloadId?: string }).workloadId)
    .filter((v): v is string => Boolean(v))
  if (!ids.length) return ElMessage.warning('Please select at least one workload')

  const Title = `${cap(action)} workloads`
  const Confirm = cap(action)
  const OkMsg = `${cap(action)} completed`
  const Cancel = `${cap(action)} canceled`

  const msg = h('span', null, [
    `Are you sure you want to ${action} workloads: `,
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, previewIds(ids)),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, Title, {
      confirmButtonText: Confirm,
      cancelButtonText: 'Cancel',
      type: action === 'delete' ? 'warning' : 'info',
    })
    batchLoading.value = true

    await apiMap[action]({ workloadIds: ids })

    ElMessage.success(OkMsg)
    await onSearch({ resetPage: false })
    selectedRows.value = []
  } catch (err: unknown) {
    if (err === 'cancel' || err === 'close') {
      ElMessage.info(Cancel)
    } else {
      const e = err as { message?: string }
      if (e?.message) ElMessage.error(e.message)
    }
  } finally {
    batchLoading.value = false
  }
}

// Reuse button handlers directly
const onBatchDelete = () => onBatch('delete')
const onBatchStop = () => onBatch('stop')

const moreOpenId = ref<string | null>(null) // ID of the currently open popover row

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

const handleMenuClick = async (act: Action, row: Row) => {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

defineOptions({
  name: 'rayJobPage',
})

// Close popover on any scroll or click outside
const onAnyScroll = () => closeMore()
const onAnyPointerDown = (e: Event) => {
  const el = e.target as HTMLElement
  const inMenu = el.closest('.actions-menu') !== null
  const inRefBtn = el.closest('.btn-primary-plain') !== null
  if (!inMenu && !inRefBtn) closeMore()
}
useRouteAction({
  [ROUTE_ACTIONS.CREATE]: () => { addVisible.value = true },
})

onMounted(() => {
  window.addEventListener('scroll', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('wheel', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('touchmove', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('pointerdown', onAnyPointerDown, { capture: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as AddEventListenerOptions)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as AddEventListenerOptions)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as AddEventListenerOptions)
  window.removeEventListener('pointerdown', onAnyPointerDown, {
    capture: true,
  } as AddEventListenerOptions)
})

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
    ]
  } else {
    searchParams.dateRange = ''
  }

  // Pagination
  pagination.page = Number(q.page || 1)
  pagination.pageSize = Number(q.pageSize || pagination.pageSize)
}

watch(
  // Refresh list data immediately when workspace dropdown changes
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
.t {
  /* font-weight: 500; */
  letter-spacing: 0.2px;
}
.sep {
  opacity: 0.55;
  margin: 0 0.6ch;
  font-weight: 700;
}

/* Bottom action bar */
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
  /* Optional: tighten left/right content spacing */
  gap: 12px;
}

/* Enter animation */
.slide-up-enter-active,
.slide-up-leave-active {
  transition:
    transform 0.18s ease,
    opacity 0.18s ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}

/* Pending Cause Analysis Icon */
.pending-cause-icon {
  cursor: pointer;
  color: var(--el-color-warning);
  transition: all 0.2s ease;
}

.pending-cause-icon:hover {
  color: var(--el-color-primary);
  transform: scale(1.2);
}
</style>
<style>
/* Override segmented style */
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}
</style>
