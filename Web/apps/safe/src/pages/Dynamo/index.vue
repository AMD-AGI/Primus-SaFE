<template>
  <el-text class="block textx-18 font-500" tag="b">{{ workloadConfig.label }}</el-text>

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
        Create {{ workloadConfig.label }}
      </el-button>
      <el-segmented
        v-model="searchParams.onlyMyself"
        :options="['All', 'My Workloads']"
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
                <el-link type="primary" v-route="{ path: workloadConfig.detailPath, query: { id: row.workloadId } }">
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
            <div class="flex flex-col gap-1">
              <div class="flex items-center gap-2">
                <el-tag
                  :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'"
                  :effect="isDark ? 'plain' : 'light'"
                >
                  {{ row.phase }}
                </el-tag>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Mode" width="140">
          <template #default="{ row }">
            <el-tag size="small" :type="getMode(row) === 'PD' ? 'warning' : 'info'">
              {{ getMode(row) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Roles" min-width="180">
          <template #default="{ row }">
            <div class="flex flex-wrap gap-1">
              <el-tag v-for="role in getRoles(row)" :key="role" size="small" effect="plain">
                {{ role }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Replicas" width="100">
          <template #default="{ row }">{{ getTotalReplicas(row) }}</template>
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
    :workload-type="props.workloadType"
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
} from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'
import { useWorkloadListQuery } from '@/composables/useWorkloadListQuery'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import {
  deleteWorkload,
  getWorkloadsList,
  stopWorkload,
  type PriorityValue,
  PRIORITY_LABEL_MAP,
} from '@/services'
import {
  phaseFilters,
  WorkloadKind,
  WorkloadPhase,
  WorkloadPhaseButtonType,
  type WorkloadParams,
} from '@/services/workload/type'
import { copyText, formatTimeStr } from '@/utils'
import AddDialog from './Components/AddDialog.vue'

dayjs.extend(utc)

defineOptions({ name: 'dynamoPage' })

const props = withDefaults(defineProps<{
  workloadType?: 'dynamo' | 'infera'
}>(), {
  workloadType: 'dynamo',
})

const workloadConfig = computed(() =>
  props.workloadType === 'infera'
    ? {
        label: 'Infera',
        kind: WorkloadKind.InferaDeployment,
        detailPath: '/infera/detail',
      }
    : {
        label: 'Dynamo',
        kind: WorkloadKind.DynamoDeployment,
        detailPath: '/dynamo/detail',
      },
)

const store = useWorkspaceStore()
const userStore = useUserStore()
const { canWrite } = useWorkloadWriteGuard()
const tableRef = ref()
const isDark = useDark()

type DateRange = '' | [Date | '', Date | '']
const initialSearchParams = {
  userName: '',
  description: '',
  phase: [] as WorkloadPhase[],
  dateRange: '' as DateRange,
  workloadId: '',
  onlyMyself: 'My Workloads',
  userId: userStore.userId,
}
const searchParams = reactive({ ...initialSearchParams })
const loading = ref(false)
interface DynamoRow {
  workloadId: string
  displayName?: string
  phase?: string
  priority?: number
  creationTime?: string
  secondsUntilTimeout?: number
  resources?: Array<{ replica?: number | string }>
  dynamoOptions?: {
    serviceRoles?: string[]
    multinodeRoles?: string[]
  }
  inferaOptions?: {
    serviceRoles?: string[]
    multinodeRoles?: string[]
  }
}

const tableData = ref<DynamoRow[]>([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const { readQuery, writeQuery, syncUserId } = useWorkloadListQuery({
  searchParams,
  pagination,
  defaultScope: 'My Workloads',
  serializeFilters: () => {
    const [start, end] = searchParams.dateRange ?? []
    return {
      userName: searchParams.userName || undefined,
      description: searchParams.description || undefined,
      phase: searchParams.phase?.length ? searchParams.phase.join(',') : undefined,
      since: start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : undefined,
      until: end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : undefined,
      workloadId: searchParams.workloadId || undefined,
    }
  },
  parseFilters: (q) => {
    searchParams.userName = (q.userName as string) || ''
    searchParams.description = (q.description as string) || ''
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
const curAction = ref<'Create' | 'Clone'>('Create')
const tableHeight = computed(() => 'calc(100vh - 245px)')
const moreOpenId = ref<string | null>(null)

const openCreate = () => {
  curAction.value = 'Create'
  curWlId.value = ''
  addVisible.value = true
}

const openClone = (row: DynamoRow) => {
  curAction.value = 'Clone'
  curWlId.value = row.workloadId
  addVisible.value = true
}

const fetchData = async (params?: WorkloadParams) => {
  try {
    loading.value = true
    if (!params?.phase) tableRef.value?.clearFilter(['phase'])
    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      kind: workloadConfig.value.kind,
      ...params,
    })
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } finally {
    loading.value = false
  }
}

const onSearch = (options?: { resetPage?: boolean }) => {
  if (options?.resetPage) pagination.page = 1
  const [start, end] = searchParams.dateRange ?? []
  const since = start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const until = end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const phaseStr = searchParams.phase?.length ? searchParams.phase.join(',') : ''

  writeQuery()

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

const resetAndSearch = () => {
  Object.assign(searchParams, { ...initialSearchParams })
  pagination.page = 1
  tableRef.value?.clearFilter()
  onSearch({ resetPage: true })
}

const filterByMyself = () => {
  syncUserId()
  onSearch({ resetPage: true })
}

const passAll = () => true
const handleFilterChange = (filters: Record<string, string[]>) => {
  if (Object.prototype.hasOwnProperty.call(filters, 'phase')) {
    searchParams.phase = (filters.phase || []) as WorkloadPhase[]
    onSearch({ resetPage: true })
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

const getRoles = (row: DynamoRow) => {
  const roles = getWorkloadOptions(row)?.serviceRoles
  if (Array.isArray(roles) && roles.length) return roles
  return row.resources?.length === 3 ? ['frontend', 'prefill', 'decode'] : ['frontend', 'worker']
}

const getMode = (row: DynamoRow) => {
  const roles = getRoles(row)
  if (roles.includes('prefill')) return 'PD'
  if (getWorkloadOptions(row)?.multinodeRoles?.includes('worker')) return 'Aggregation'
  if (props.workloadType === 'infera' && Number(row.resources?.[1]?.replica || 0) > 1) return 'Aggregation'
  return 'Standard'
}

const getWorkloadOptions = (row: DynamoRow) =>
  props.workloadType === 'infera' ? row.inferaOptions : row.dynamoOptions

const getTotalReplicas = (row: DynamoRow) => {
  const resources = Array.isArray(row.resources) ? row.resources : []
  return resources.reduce((sum, item) => sum + (Number(item?.replica) || 0), 0)
}

type Action = {
  key: string
  label: string
  icon: any
  btnClass?: string
  disabled?: (row: DynamoRow) => boolean
  tooltip?: (row: DynamoRow) => string
  onClick: (row: DynamoRow) => void | Promise<void>
}

const getActions = (_row: DynamoRow): Action[] => [
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
    disabled: (row) => !canWrite.value || !['Running', 'Pending'].includes(row.phase || ''),
    tooltip: (row) => {
      if (['Running', 'Pending'].includes(row.phase || '')) return 'Stop workload'
      return 'Stop is only available for Running or Pending workloads'
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

const handleMenuClick = async (act: Action, row: DynamoRow) => {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

const onStop = async (row: DynamoRow) => {
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

const onDelete = async (row: DynamoRow) => {
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

const onAnyScroll = () => closeMore()
const onAnyPointerDown = (e: Event) => {
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
  userStore.fetchEnvs()
  readQuery()
  onSearch({ resetPage: false })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as any)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as any)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as any)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as any)
})
</script>

