<template>
  <el-text class="block textx-18 font-500" tag="b">Workload Management</el-text>

  <div class="flex flex-wrap items-center gap-2 mt-4">
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
      />
      <el-tooltip content="Reset filters" placement="top">
        <el-button
          :icon="ResetIcon"
          size="default"
          @click="resetAndSearch"
        />
      </el-tooltip>
      <el-tooltip content="Refresh" placement="top">
        <el-button
          :icon="Refresh"
          size="default"
          @click="onSearch({ resetPage: false })"
        />
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

        <el-table-column prop="workloadId" label="Name/ID" min-width="240" :fixed="true">
          <template #default="{ row }">
            <div class="flex flex-col items-start">
              <div class="flex items-center">
                <el-link type="primary" @click="jumpToDetail(row)">{{ row.displayName }}</el-link>
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
          prop="groupVersionKind"
          label="Kind"
          width="180"
          column-key="kindFilter"
          :filters="kindFilters"
          :filtered-value="filterSelectedKind"
          :filter-multiple="false"
          filter-placement="bottom-start"
          :filter-method="passAll"
        >
          <template #default="{ row }">
            <el-tag size="small" :effect="isDark ? 'plain' : 'light'">
              {{ getRowKind(row) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column
          prop="workspaceId"
          label="Workspace"
          width="160"
          column-key="wsFilter"
          :filters="wsFilters"
          :filtered-value="filterSelectedWs"
          :filter-multiple="false"
          filter-placement="bottom-start"
          :filter-method="passAll"
          show-overflow-tooltip
        />

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
                >{{ row.phase }}</el-tag>
                <el-tooltip
                  v-if="row.phase === 'Failed' && isTrainingLike(getRowKind(row))"
                  content="Root Cause Analysis"
                  placement="top"
                >
                  <el-icon
                    class="root-cause-icon"
                    :size="18"
                    @click.stop="router.push({ path: '/training/root-cause', query: { id: row.workloadId } })"
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
                    @click.stop="router.push({ path: '/workload/pending-cause', query: { id: row.workloadId } })"
                  >
                    <InfoFilled />
                  </el-icon>
                </el-tooltip>
              </div>
              <div class="text-sm" v-if="row.phase === 'Pending' && !!row.queuePosition">
                position in queue:{{ row.queuePosition }}
              </div>
            </div>
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

        <el-table-column prop="priority" label="Priority" min-width="110">
          <template #default="{ row }">
            <span class="priority-cell">
              <span class="priority-dot" :class="priorityColorClass(row.priority)" />
              {{ PRIORITY_LABEL_MAP[row.priority as PriorityValue] }}
            </span>
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

        <el-table-column label="Actions" width="180" fixed="right">
          <template #default="{ row }">
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
            <span class="ml-2">
              Selected {{ selectedRows.length }} item{{ selectedRows.length === 1 ? '' : 's' }}
            </span>
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

  <!-- Dynamically rendered edit dialog based on workload kind -->
  <component
    :is="activeDialogComp"
    v-if="activeDialogComp"
    v-model:visible="addVisible"
    :wlid="curWlId"
    :action="curAction"
    @success="onDialogSuccess"
  />
</template>

<script lang="ts" setup>
import { ref, reactive, watch, nextTick, onMounted, onBeforeUnmount, h, computed, shallowRef } from 'vue'
import {
  getWorkloadsList,
  deleteWorkload,
  stopWorkload,
  batchDelWorkload,
  batchStopWorkload,
  getWorkloadDetail,
} from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  WorkloadKind,
  WorkloadPhase,
  phaseFilters,
  WorkloadPhaseButtonType,
  KindPathMap,
} from '@/services/workload/type'
import {
  Search, Refresh, CopyDocument, Timer, WarningFilled, InfoFilled,
} from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
import { copyText, formatTimeStr } from '@/utils/index'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Close, MoreFilled, VideoPlay, Link } from '@element-plus/icons-vue'
import { type WorkloadParams, type PriorityValue, PRIORITY_LABEL_MAP } from '@/services'
import { useUserStore } from '@/stores/user'
import { useDark } from '@vueuse/core'

dayjs.extend(utc)

defineOptions({ name: 'workloadManagePage' })

const tableRef = ref()
const isDark = useDark()
const router = useRouter()
const wsStore = useWorkspaceStore()
const userStore = useUserStore()

// ── Extract kind from groupVersionKind ──
const getRowKind = (row: any): string => row.groupVersionKind?.kind || row.kind || ''

// ── Kind config (includes extra types only visible in admin page) ──
const ALL_KINDS = [...Object.values(WorkloadKind), 'Job']
const kindFilters = ALL_KINDS.map((k) => ({ text: k, value: k }))
const filterSelectedKind = ref<string[]>([])

const isTrainingLike = (kind: string) =>
  [WorkloadKind.PyTorchJob, WorkloadKind.TorchFT, WorkloadKind.RayJob].includes(kind as WorkloadKind)

const hasResume = (kind: string) =>
  [WorkloadKind.Authoring, WorkloadKind.Deployment, WorkloadKind.StatefulSet,
   WorkloadKind.AutoscalingRunnerSet, WorkloadKind.EphemeralRunner, WorkloadKind.UnifiedJob].includes(kind as WorkloadKind)

const hasSSH = (kind: string) => kind === WorkloadKind.Authoring

// ── Priority color ──
const priorityColorClass = (p: number) => {
  if (p === 2) return 'dot-high'
  if (p === 1) return 'dot-medium'
  return 'dot-low'
}

// ── Workspace header filter (same pattern as Nodes page) ──
const filterSelectedWs = ref<string[]>([])
const wsFilters = computed(() =>
  (wsStore.items || []).map((ws) => ({
    text: ws.workspaceId,
    value: ws.workspaceId,
  })),
)

// ── Lazy dialog import map per kind ──
const dialogImportMap: Record<string, () => Promise<any>> = {
  PyTorchJob: () => import('@/pages/Training/Components/AddDialog.vue'),
  Authoring: () => import('@/pages/Authoring/Components/AddDialog.vue'),
  TorchFT: () => import('@/pages/TorchFT/Components/AddDialog.vue'),
  RayJob: () => import('@/pages/RayJob/Components/AddDialog.vue'),
  Deployment: () => import('@/pages/Infer/Components/AddDialog.vue'),
  StatefulSet: () => import('@/pages/Infer/Components/AddDialog.vue'),
  AutoscalingRunnerSet: () => import('@/pages/CICD/Components/AddDialog.vue'),
  EphemeralRunner: () => import('@/pages/CICD/Components/AddDialog.vue'),
  UnifiedJob: () => import('@/pages/CICD/Components/AddDialog.vue'),
}

// ── State ──
const addVisible = ref(false)
const curWlId = ref('')
const curAction = ref<'Edit' | 'Resume'>('Edit')
const activeDialogComp = shallowRef<any>(null)
const savedWorkspaceId = ref<string>()

const initialSearchParams = {
  workspaceId: '',
  kind: '',
  userName: '',
  description: '',
  phase: [] as WorkloadPhase[],
  dateRange: '' as any,
  workloadId: '',
  userId: '',
}
const searchParams = reactive({ ...initialSearchParams })

const loading = ref(false)
const tableData = ref<any[]>([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

// ── Table height & selection bar ──
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

// ── Detail navigation ──
const jumpToDetail = (row: any) => {
  const kind = getRowKind(row) as WorkloadKind
  const basePath = KindPathMap[kind]
  if (basePath) {
    router.push({ path: `${basePath}/detail`, query: { id: row.workloadId } })
  }
}

// ── Data fetching ──
const fetchData = async (params?: WorkloadParams) => {
  try {
    loading.value = true
    if (!params?.phase) {
      tableRef.value?.clearFilter(['phase'])
    }
    const res = await getWorkloadsList({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
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
  if ('wsFilter' in filters) {
    searchParams.workspaceId = filters.wsFilter?.[0] || ''
  }
  if ('kindFilter' in filters) {
    searchParams.kind = filters.kindFilter?.[0] || ''
  }
  if ('phase' in filters) {
    searchParams.phase = (filters.phase || []) as WorkloadPhase[]
  }
  onSearch({ resetPage: true })
}

const onSearch = (options?: { resetPage?: boolean }) => {
  if (options?.resetPage) pagination.page = 1

  const [start, end] = searchParams.dateRange ?? []
  const since = start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const until = end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const phaseStr = searchParams.phase?.length ? searchParams.phase.join(',') : ''

  fetchData({
    workspaceId: searchParams.workspaceId || undefined,
    kind: searchParams.kind || undefined,
    userName: searchParams.userName,
    description: searchParams.description,
    phase: phaseStr,
    since,
    until,
    workloadId: searchParams.workloadId,
    userId: searchParams.userId,
  })
}

const resetAndSearch = () => {
  Object.assign(searchParams, { ...initialSearchParams })
  filterSelectedWs.value = []
  tableRef.value?.clearFilter()
  pagination.page = 1
  onSearch({ resetPage: true })
}

// ── Actions ──
type Row = {
  workloadId: string
  phase: string
  groupVersionKind?: { kind: string; version?: string }
  kind?: string
  workspaceId: string
  maxRetry?: number
  queuePosition?: number
  displayName?: string
  pods?: { podId?: string }[]
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

const getEditDisabled = (row: Row): boolean => {
  const kind = getRowKind(row)
  if (kind === WorkloadKind.Deployment || kind === WorkloadKind.StatefulSet) return false
  if (isTrainingLike(kind)) {
    const maxRetry = row.maxRetry ?? 0
    if (maxRetry > 0) return !['Running', 'Pending'].includes(row.phase)
    return !(row.phase === 'Pending' && (row.queuePosition ?? 0) > 0)
  }
  return !['Running', 'Pending'].includes(row.phase)
}

const getActions = (row: Row): Action[] => {
  const kind = getRowKind(row)
  const actions: Action[] = []

  if (hasSSH(kind)) {
    actions.push({
      key: 'ssh',
      label: 'SSH',
      icon: Link,
      btnClass: 'btn-primary-plain',
      disabled: (r) => r.phase !== 'Running',
      onClick: (r) => openSSH(r),
    })
  }

  if (hasResume(kind)) {
    actions.push({
      key: 'resume',
      label: 'Resume',
      icon: VideoPlay,
      btnClass: 'btn-success-plain',
      disabled: (r) => !['Stopped', 'Failed', 'Succeeded'].includes(r.phase),
      onClick: (r) => {
        const endTime = (r as any).endTime
        if (endTime && dayjs().diff(dayjs.utc(endTime), 'second') < 15) {
          ElMessage.warning('Please wait 15 seconds after stopping before resuming the workload.')
          return
        }
        openDialog(r, 'Resume')
      },
    })
  }

  actions.push(
    {
      key: 'edit',
      label: 'Edit',
      icon: Edit,
      btnClass: 'btn-primary-plain',
      disabled: getEditDisabled,
      tooltip: (r) => getEditDisabled(r) ? 'Edit is unavailable for this workload state' : 'Edit workload configuration',
      onClick: (r) => openDialog(r, 'Edit'),
    },
    {
      key: 'stop',
      label: 'Stop',
      icon: Close,
      btnClass: 'btn-warning-plain',
      disabled: (r) => !['Running', 'Pending'].includes(r.phase),
      onClick: async (r) => {
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
    {
      key: 'delete',
      label: 'Delete',
      icon: Delete,
      btnClass: 'btn-danger-plain',
      onClick: async (r) => {
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
  )

  return actions
}

const openSSH = async (r: Row) => {
  try {
    const res = await getWorkloadDetail(r.workloadId)
    const sshCommand = res.pods?.[0]?.sshCommand
    if (!sshCommand) {
      ElMessage.warning('SSH command not found')
      return
    }
    copyText(sshCommand)
  } catch {
    ElMessage.error('Failed to generate SSH command')
  }
}

const openDialog = async (row: Row, action: 'Edit' | 'Resume') => {
  const kind = getRowKind(row)
  const loader = dialogImportMap[kind]
  if (!kind || !loader) {
    ElMessage.warning('Unsupported workload kind for this action')
    return
  }
  savedWorkspaceId.value = wsStore.currentWorkspaceId
  if (row.workspaceId && row.workspaceId !== wsStore.currentWorkspaceId) {
    await wsStore.setCurrentWorkspace(row.workspaceId)
  }

  // Load the dialog component chunk, then mount with visible=false first
  const mod = await loader()
  curWlId.value = row.workloadId
  curAction.value = action
  addVisible.value = false
  activeDialogComp.value = mod.default
  await nextTick()
  // Now the drawer is mounted (visible=false), transition false→true triggers @open
  addVisible.value = true
}

const onDialogSuccess = () => {
  onSearch({ resetPage: false })
}

watch(addVisible, async (v) => {
  if (!v) {
    if (savedWorkspaceId.value && savedWorkspaceId.value !== wsStore.currentWorkspaceId) {
      await wsStore.setCurrentWorkspace(savedWorkspaceId.value)
    }
    savedWorkspaceId.value = undefined
    activeDialogComp.value = null
  }
})

// ── Batch actions ──
type BatchAction = 'delete' | 'stop'
const apiMap: Record<BatchAction, (body: any) => Promise<any>> = {
  delete: batchDelWorkload,
  stop: batchStopWorkload,
}
const batchLoading = ref(false)
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

function previewIds(ids: string[], max = 5) {
  return ids.length <= max ? ids.join(', ') : `${ids.slice(0, max).join(', ')} +${ids.length - max} more`
}

async function onBatch(action: BatchAction) {
  const ids = selectedRows.value.map((r) => r.workloadId).filter(Boolean)
  if (!ids.length) return ElMessage.warning('Please select at least one workload')

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
    batchLoading.value = true
    await apiMap[action]({ workloadIds: ids })
    ElMessage.success(`${cap(action)} completed`)
    await onSearch({ resetPage: false })
    selectedRows.value = []
  } catch (err: any) {
    if (err === 'cancel' || err === 'close') {
      ElMessage.info(`${cap(action)} canceled`)
    } else if (err?.message) {
      ElMessage.error(err.message)
    }
  } finally {
    batchLoading.value = false
  }
}

const onBatchDelete = () => onBatch('delete')
const onBatchStop = () => onBatch('stop')

// ── Popover (more actions) ──
const moreOpenId = ref<string | null>(null)
const toggleMore = async (id: string) => {
  if (moreOpenId.value === id) { moreOpenId.value = null; return }
  moreOpenId.value = null
  await nextTick()
  moreOpenId.value = id
}
const closeMore = () => { moreOpenId.value = null }
const handleMenuClick = async (act: Action, row: Row) => {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

const onAnyScroll = () => closeMore()
const onAnyPointerDown = (e: Event) => {
  const el = e.target as HTMLElement
  if (!el.closest('.actions-menu') && !el.closest('.btn-primary-plain')) closeMore()
}

onMounted(() => {
  window.addEventListener('scroll', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('wheel', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('touchmove', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('pointerdown', onAnyPointerDown, { capture: true })
  fetchData()
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as any)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as any)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as any)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as any)
})
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
  letter-spacing: 0.2px;
}
.sep {
  opacity: 0.55;
  margin: 0 0.6ch;
  font-weight: 700;
}

/* Priority dot */
.priority-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.priority-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.dot-high {
  background-color: var(--el-color-danger);
}
.dot-medium {
  background-color: var(--el-color-warning);
}
.dot-low {
  background-color: var(--el-color-success);
}

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

.root-cause-icon {
  cursor: pointer;
  color: var(--el-color-danger);
  transition: all 0.2s ease;
}
.root-cause-icon:hover {
  color: var(--el-color-warning);
  transform: scale(1.2);
}
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
