<template>
  <el-text class="block textx-18 font-500" tag="b">Training</el-text>

  <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
    <!-- Left actions -->
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        data-tour="training-create-btn"
        @click="
          () => {
            addVisible = true
            curWlId = ''
            curAction = 'Create'
          }
        "
        class="text-black"
      >
        Create Training
      </el-button>
      <el-segmented
        v-model="searchParams.onlyMyself"
        :options="['All', 'My Workloads']"
        @change="filterByMyself"
        class="myself-seg"
        style="background: none"
      />
    </div>

    <!-- Right search -->
    <div class="flex flex-wrap items-center" data-tour="training-filters">
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

  <el-card class="mt-4 safe-card" shadow="never" data-tour="training-table">
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
          width="180"
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
                  v-if="row.phase === 'Failed'"
                  content="Root Cause Analysis"
                  placement="top"
                >
                  <el-icon
                    class="root-cause-icon"
                    :size="18"
                    @click.stop="
                      router.push({ path: '/training/root-cause', query: { id: row.workloadId } })
                    "
                  >
                    <WarningFilled />
                  </el-icon>
                </el-tooltip>
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
            {{
              (row.resources?.[0]?.replica || 0) + (row.resources?.[1]?.replica || 0) ||
              row.resource?.replica ||
              0
            }}
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
            <span class="res-line">
              <span class="t">{{ (row.resources?.[0] || row.resource)?.gpu ?? 0 }} card</span>
              <span class="sep">*</span>
              <span class="t">{{ (row.resources?.[0] || row.resource)?.cpu }} core</span>
              <span class="sep">*</span>
              <span class="t">{{ (row.resources?.[0] || row.resource)?.memory }}</span>
            </span>
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
              <el-tooltip :content="act.tooltip?.(row) ?? act.label" placement="top">
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
import {
  WorkloadKind,
  WorkloadPhase,
  phaseFilters,
  WorkloadPhaseButtonType,
} from '@/services/workload/type'
import { Search, Refresh, CopyDocument, Plus, Timer, WarningFilled, InfoFilled } from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
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
import { usePageTour, waitForEl } from '@/composables/usePageTour'
import type { DriveStep } from 'driver.js'
import UtilDialog from './Components/UtilDialog.vue'

dayjs.extend(utc)

const tableRef = ref()
const isDark = useDark()
const route = useRoute()
const router = useRouter()
const store = useWorkspaceStore()
const userStore = useUserStore()

// Auto-refresh user info on page enter (permission-sensitive page)
useAutoRefreshUserInfo({ immediate: true })

const addVisible = ref(false)
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
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const curWlId = ref()
const curAction = ref<'Create' | 'Edit' | 'Clone'>('Create')

/* ── Page Tours (tourId-driven from Quick Reference ?tour=<id>) ── */
const { getDriver } = usePageTour((tourId) => {
  switch (tourId) {
    /* ─ "Launch training" — full create-form walkthrough ─ */
    case 'create':
    default:
      return [
        {
          element: '[data-tour="training-create-btn"]',
          popover: {
            title: 'Create Training',
            description:
              'Click here to create a new training job. You can also clone an existing one from the table.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="training-filters"]',
          popover: {
            title: 'Search & Filter',
            description: 'Filter training jobs by date range, name, or workload ID.',
            side: 'bottom' as const,
            align: 'end' as const,
          },
        },
        {
          element: '[data-tour="training-table"]',
          popover: {
            title: 'Training Jobs',
            description:
              'All training workloads in your workspace. Click a name to view details, or use row actions to stop / clone / delete.',
            side: 'top' as const,
          },
        },
        {
          element: '[data-tour="training-create-btn"]',
          popover: {
            title: 'Walk Through the Form',
            description: 'Click Next to open the creation form and see each key field.',
            side: 'bottom' as const,
            align: 'start' as const,
            onNextClick: async () => {
              addVisible.value = true
              curWlId.value = ''
              curAction.value = 'Create'
              await waitForEl('[data-tour="training-field-name"]')
              getDriver()?.moveNext()
            },
          },
        },
        {
          element: '[data-tour="training-field-name"]',
          popover: {
            title: 'Training Name',
            description: 'Give your training job a descriptive name.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="training-field-image"]',
          popover: {
            title: 'Container Image',
            description:
              'Select or enter the container image. Import one from the Images page first.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="training-field-entrypoint"]',
          popover: {
            title: 'Entry Point',
            description:
              'The command to execute inside the container — e.g. your training script.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="training-field-resource"]',
          popover: {
            title: 'Resource Allocation',
            description:
              'Set replicas / nodes, CPU, GPU, memory. For multi-node, NNODES & NODE_RANK are auto-injected.',
            side: 'left' as const,
            align: 'start' as const,
            onNextClick: () => {
              addVisible.value = false
              getDriver()?.moveNext()
            },
          },
        },
      ]

    /* ─ "Download logs" — find a job → open detail → download logs ─ */
    case 'logs':
      return [
        {
          element: '[data-tour="training-table"]',
          popover: {
            title: 'Find Your Job',
            description:
              'Locate the training job whose logs you want to inspect. Use the search bar above to filter by name or ID.',
            side: 'top' as const,
          },
        },
        {
          element:
            '[data-tour="training-table"] .el-table__body-wrapper tr:first-child td:first-child .el-link',
          popover: {
            title: 'Open Job Detail',
            description:
              'Click the job name to enter the detail page. You can view real-time logs, resource graphs, and events there.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
        {
          popover: {
            title: 'Download Logs',
            description:
              'In the detail page, switch to the "Logs" tab. Click "Download" to save the full log file locally.',
          },
        },
      ]
  }
})

const SELECTION_BAR_H = 56
const BASE_OFFSET = 245

// Multi-select
const selectedRows = ref<Array<Record<string, any>>>([])
function onSelectionChange(rows: Array<Record<string, any>>) {
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
  router.push({ path: '/training/detail', query: { id } })
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

const viewChart = async (r: any) => {
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
    const rows = ((res?.data?.data ?? res?.data ?? res) as any[]) || []

    // Compute avg only, converting to local time and percentage
    const x = rows.map((it) => dayjs.utc(it.stat_hour).local().format('MM-DD HH:mm'))
    const avg = rows.map((it) => {
      const v = it?.avg_utilization
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
      kind: WorkloadKind.PyTorchJob,
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
  maxRetry?: number
  queuePosition?: number
  displayName?: string
}
type Action = {
  key: string
  label: string
  icon: any
  btnClass?: string
  disabled?: (row: Row) => boolean
  tooltip?: (row: Row) => string
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
      tooltip: (r: Row) => {
        const maxRetry = r.maxRetry ?? 0
        const phase = r.phase
        const queuePosition = r.queuePosition ?? 0

        if (maxRetry > 0) {
          if (['Running', 'Pending'].includes(phase)) {
            return 'Edit workload configuration'
          }
          return 'Edit is only available for Pending or Running workloads with auto-retry enabled'
        } else {
          if (phase === 'Pending' && queuePosition > 0) {
            return 'Edit workload configuration'
          }
          return 'Edit is only available for Pending workloads in queue (without auto-retry)'
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

const apiMap: Record<BatchAction, (body: any) => Promise<any>> = {
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
  const ids = selectedRows.value.map((r) => r.workloadId).filter(Boolean)
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
  } catch (err: any) {
    if (err === 'cancel' || err === 'close') {
      ElMessage.info(Cancel)
    } else if (err?.message) {
      ElMessage.error(err.message)
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
  name: 'trainingPage',
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
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as any)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as any)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as any)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as any)
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
    ] as any
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
</style>
<style>
/* Override segmented style */
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}

/* Root Cause Analysis Icon */
.root-cause-icon {
  cursor: pointer;
  color: var(--el-color-danger);
  transition: all 0.2s ease;
}

.root-cause-icon:hover {
  color: var(--el-color-warning);
  transform: scale(1.2);
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
