<template>
  <el-text class="block textx-18 font-500" tag="b">SGLangRouter</el-text>

  <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        :disabled="!canWrite"
        class="text-black"
        @click="openCreate"
      >
        Create SGLangRouter
      </el-button>
      <el-segmented
        v-model="searchParams.onlyMyself"
        :options="['All', 'My Workloads']"
        class="ml-2 mt-2 sm:mt-0 mb-2"
        @change="filterByMyself"
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
      <el-button :icon="Search" size="default" type="primary" @click="onSearch({ resetPage: true })" />
      <el-tooltip content="Reset filters" placement="top">
        <el-button :icon="ResetIcon" size="default" @click="resetAndSearch" />
      </el-tooltip>
      <el-tooltip content="Refresh" placement="top">
        <el-button :icon="Refresh" size="default" @click="onSearch({ resetPage: false })" />
      </el-tooltip>
    </div>
  </div>

  <el-card class="mt-4 safe-card" shadow="never">
    <div class="table-wrap">
      <el-table
        ref="tableRef"
        :height="tableHeight"
        :data="tableData"
        size="large"
        class="m-t-2"
        v-loading="loading"
        :element-loading-text="$loadingText"
        @filter-change="handleFilterChange"
      >
        <el-table-column prop="workloadId" label="Name/ID" min-width="220" :fixed="true">
          <template #default="{ row }">
            <div class="flex flex-col items-start">
              <div class="flex items-center">
                <el-link type="primary" v-route="{ path: '/sglang-router/detail', query: { id: row.workloadId } }">
                  {{ row.displayName }}
                </el-link>
                <el-tooltip
                  v-if="row.secondsUntilTimeout > 0"
                  :content="`${row.secondsUntilTimeout} seconds remaining until the task ends`"
                  placement="top"
                >
                  <el-icon class="ml-1 mt-1 cursor-default text-yellow-500">
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
            <el-tag
              :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'"
              :effect="isDark ? 'plain' : 'light'"
            >
              {{ row.phase }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Roles" min-width="220">
          <template #default="{ row }">
            <div class="flex flex-wrap gap-1">
              <el-tag v-for="role in roleReplicas(row)" :key="role" size="small" effect="plain">
                {{ role }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Replicas" width="100">
          <template #default="{ row }">{{ totalReplicas(row) }}</template>
        </el-table-column>
        <el-table-column label="Priority" width="110">
          <template #default="{ row }">
            {{ PRIORITY_LABEL_MAP[row.priority as PriorityValue] || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="creationTime" label="Created" width="180">
          <template #default="{ row }">{{ formatTimeStr(row.creationTime) }}</template>
        </el-table-column>
        <el-table-column label="Actions" width="180" fixed="right">
          <template #default="{ row }">
            <div class="action-cell">
              <template v-for="act in getActions(row).slice(0, 2)" :key="act.key">
                <el-tooltip :content="act.tooltip?.(row) ?? act.label" placement="top">
                  <span>
                    <el-button
                      circle
                      size="default"
                      :class="act.btnClass"
                      :icon="act.icon"
                      :disabled="act.disabled?.(row) ?? false"
                      @click="act.onClick(row)"
                    />
                  </span>
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
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        class="m-t-2"
        :current-page="pagination.page"
        :page-size="pagination.pageSize"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next"
        :page-sizes="[10, 20, 50, 100]"
        @current-change="handlePageChange"
        @size-change="handlePageSizeChange"
      />
    </div>
  </el-card>

  <AddDialog
    v-model:visible="addVisible"
    :wlid="curWlId"
    :action="curAction"
    @success="onSearch({ resetPage: false })"
  />
</template>

<script lang="ts" setup>
import { computed, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useDark } from '@vueuse/core'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Close,
  CopyDocument,
  Delete,
  DocumentCopy,
  MoreFilled,
  Plus,
  Refresh,
  Search,
  Timer,
  VideoPlay,
} from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
import { useWorkloadListQuery } from '@/composables/useWorkloadListQuery'
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'
import {
  ensureResumeCooldownElapsed,
  useWorkloadResumePermission,
} from '@/composables/useWorkloadResumePermission'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  deleteWorkload,
  getWorkloadsList,
  stopWorkload,
  PRIORITY_LABEL_MAP,
  type PriorityValue,
} from '@/services'
import {
  WorkloadKind,
  WorkloadPhaseButtonType,
  phaseFilters,
  type WorkloadParams,
  type WorkloadPhase,
} from '@/services/workload/type'
import { copyText, formatTimeStr } from '@/utils/index'
import AddDialog from './Components/AddDialog.vue'

dayjs.extend(utc)

const store = useWorkspaceStore()
const userStore = useUserStore()
const { canWrite } = useWorkloadWriteGuard()
const { getResumeDisabled, getResumeTooltip } = useWorkloadResumePermission(canWrite)
const tableRef = ref()
const isDark = useDark()

type DateRange = '' | [Date | '', Date | '']
const initialSearchParams = {
  phase: [] as WorkloadPhase[],
  dateRange: '' as DateRange,
  workloadId: '',
  onlyMyself: 'My Workloads',
  userId: userStore.userId,
}
const searchParams = reactive({ ...initialSearchParams })
const loading = ref(false)
const tableData = ref<SGLangRouterRow[]>([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const { readQuery, writeQuery, syncUserId } = useWorkloadListQuery({
  searchParams,
  pagination,
  defaultScope: 'My Workloads',
  serializeFilters: () => {
    const [start, end] = searchParams.dateRange ?? []
    return {
      phase: searchParams.phase?.length ? searchParams.phase.join(',') : undefined,
      since: start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : undefined,
      until: end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : undefined,
      workloadId: searchParams.workloadId || undefined,
    }
  },
  parseFilters: (q) => {
    searchParams.workloadId = (q.workloadId as string) || ''
    const phaseStr = (q.phase as string) || ''
    searchParams.phase = phaseStr ? (phaseStr.split(',') as WorkloadPhase[]) : []
    const since = q.since as string | undefined
    const until = q.until as string | undefined
    searchParams.dateRange = (since || until
      ? [since ? dayjs(since).toDate() : '', until ? dayjs(until).toDate() : '']
      : '') as DateRange
  },
})

const addVisible = ref(false)
const curWlId = ref('')
const curAction = ref<'Create' | 'Clone' | 'Resume'>('Create')
const moreOpenId = ref<string | null>(null)
const tableHeight = computed(() => 'calc(100vh / var(--zoom) - 245px)')

const ROLE_NAMES = ['router', 'prefill', 'decode']

interface SGLangRouterRow {
  workloadId: string
  displayName?: string
  phase?: string
  priority?: number
  creationTime?: string
  endTime?: string
  secondsUntilTimeout?: number
  userId?: string
  workspaceId?: string
  workspace?: string
  resources?: Array<{ replica?: number | string }>
}

/** Per-role replica tags, in router, prefill, decode order. */
function roleReplicas(row: SGLangRouterRow) {
  const resources = row.resources || []
  return ROLE_NAMES.map((name, index) => `${name} x${resources[index]?.replica ?? '-'}`)
}

/** Sum of role replicas. */
function totalReplicas(row: SGLangRouterRow) {
  const resources = Array.isArray(row.resources) ? row.resources : []
  return resources.reduce((sum, item) => sum + (Number(item?.replica) || 0), 0)
}

async function fetchData(params?: WorkloadParams) {
  loading.value = true
  try {
    if (!params?.phase) tableRef.value?.clearFilter(['phase'])
    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      kind: WorkloadKind.SGLangRouter,
      ...params,
    })
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } finally {
    loading.value = false
  }
}

function onSearch(options?: { resetPage?: boolean }) {
  if (options?.resetPage) pagination.page = 1
  const [start, end] = searchParams.dateRange ?? []
  const since = start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const until = end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  writeQuery()
  fetchData({
    phase: searchParams.phase?.length ? searchParams.phase.join(',') : '',
    since,
    until,
    workloadId: searchParams.workloadId,
    userId: searchParams.userId,
  })
}

function resetAndSearch() {
  Object.assign(searchParams, { ...initialSearchParams })
  pagination.page = 1
  tableRef.value?.clearFilter()
  onSearch({ resetPage: true })
}

function filterByMyself() {
  syncUserId()
  onSearch({ resetPage: true })
}

function passAll() {
  return true
}

function handlePageChange(page: number) {
  pagination.page = page
  onSearch({ resetPage: false })
}

function handlePageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.page = 1
  onSearch({ resetPage: false })
}

function handleFilterChange(filters: Record<string, string[]>) {
  if (Object.prototype.hasOwnProperty.call(filters, 'phase')) {
    searchParams.phase = (filters.phase || []) as WorkloadPhase[]
    onSearch({ resetPage: true })
  }
}

function openCreate() {
  curWlId.value = ''
  curAction.value = 'Create'
  addVisible.value = true
}

function openClone(row: SGLangRouterRow) {
  curAction.value = 'Clone'
  curWlId.value = row.workloadId
  addVisible.value = true
}

function openResume(row: SGLangRouterRow) {
  if (!ensureResumeCooldownElapsed(row.endTime)) return
  curAction.value = 'Resume'
  curWlId.value = row.workloadId
  addVisible.value = true
}

type Action = {
  key: string
  label: string
  icon: unknown
  btnClass?: string
  disabled?: (row: SGLangRouterRow) => boolean
  tooltip?: (row: SGLangRouterRow) => string
  onClick: (row: SGLangRouterRow) => void | Promise<void>
}

/** Phases that still own live resources and can be stopped. */
const STOPPABLE_PHASES = ['Running', 'Pending', 'NotReady', 'Updating']

/** Inline Resume and Clone, with Stop and Delete under the more menu. */
function getActions(_row: SGLangRouterRow): Action[] {
  return [
    {
      key: 'resume',
      label: 'Resume',
      icon: VideoPlay,
      btnClass: 'btn-primary-plain',
      disabled: getResumeDisabled,
      tooltip: getResumeTooltip,
      onClick: (row) => openResume(row),
    },
    {
      key: 'clone',
      label: 'Clone',
      icon: DocumentCopy,
      btnClass: 'btn-success-plain',
      disabled: () => !canWrite.value,
      onClick: (row) => openClone(row),
    },
    {
      key: 'stop',
      label: 'Stop',
      icon: Close,
      btnClass: 'btn-danger-plain',
      disabled: (row) => !canWrite.value || !STOPPABLE_PHASES.includes(row.phase || ''),
      tooltip: (row) => {
        if (STOPPABLE_PHASES.includes(row.phase || '')) return 'Stop workload'
        return 'Stop is only available for Running, Pending, NotReady, or Updating workloads'
      },
      onClick: (row) => onStop(row),
    },
    {
      key: 'delete',
      label: 'Delete',
      icon: Delete,
      btnClass: 'btn-danger-plain',
      disabled: () => !canWrite.value,
      onClick: (row) => onDelete(row),
    },
  ]
}

async function toggleMore(id: string) {
  if (moreOpenId.value === id) {
    moreOpenId.value = null
    return
  }
  moreOpenId.value = null
  await nextTick()
  moreOpenId.value = id
}

function closeMore() {
  moreOpenId.value = null
}

async function handleMenuClick(act: Action, row: SGLangRouterRow) {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

async function onStop(row: SGLangRouterRow) {
  await ElMessageBox.confirm(
    h('span', null, [
      'Are you sure you want to stop workload: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.workloadId),
      ' ?',
    ]),
    'Stop workload',
    { confirmButtonText: 'Stop', cancelButtonText: 'Cancel', type: 'warning' },
  )
  await stopWorkload(row.workloadId)
  ElMessage.success('Stop complete')
  onSearch({ resetPage: false })
}

async function onDelete(row: SGLangRouterRow) {
  await ElMessageBox.confirm(
    h('span', null, [
      'Are you sure you want to delete workload: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.workloadId),
      ' ?',
    ]),
    'Delete workload',
    { confirmButtonText: 'Delete', cancelButtonText: 'Cancel', type: 'warning' },
  )
  await deleteWorkload(row.workloadId)
  ElMessage.success('Deleted')
  onSearch({ resetPage: false })
}

function onAnyScroll() {
  closeMore()
}

function onAnyPointerDown(e: Event) {
  const el = e.target as HTMLElement
  const inMenu = el.closest('.actions-menu') !== null
  const inRefBtn = el.closest('.btn-primary-plain') !== null
  if (!inMenu && !inRefBtn) closeMore()
}

watch(
  () => store.currentWorkspaceId,
  () => onSearch({ resetPage: true }),
)

onMounted(() => {
  window.addEventListener('scroll', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('wheel', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('touchmove', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('pointerdown', onAnyPointerDown, { capture: true })
  readQuery()
  onSearch({ resetPage: false })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as EventListenerOptions)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as EventListenerOptions)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as EventListenerOptions)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as EventListenerOptions)
})
</script>
