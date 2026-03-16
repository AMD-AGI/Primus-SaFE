<template>
  <el-text class="block textx-18 font-500" tag="b">Authoring</el-text>

  <div class="flex flex-wrap items-center mt-4">
    <!-- Left actions -->
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        data-tour="authoring-create-btn"
        @click="
          () => {
            addVisible = true
            curWlId = ''
            curAction = 'Create'
          }
        "
        class="mb-2 text-black"
      >
        Create Authoring
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
    <div class="flex flex-wrap items-center mt-2 mb-2 sm:mt-0 ml-auto" data-tour="authoring-filters">
      <el-date-picker
        v-model="searchParams.dateRange"
        size="default"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start date"
        end-placeholder="End date"
        class="mr-3"
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
            fetchData()
          }
        "
      ></el-button>
    </div>
  </div>

  <el-card class="mt-4 safe-card" shadow="never" data-tour="authoring-table">
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
              <el-link type="primary" @click="jumpToDetail(row.workloadId)">{{
                row.displayName
              }}</el-link>
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
</template>
<script lang="ts" setup>
import { ref, reactive, watch, nextTick, onMounted, onBeforeUnmount, h, computed } from 'vue'
import {
  getWorkloadsList,
  deleteWorkload,
  stopWorkload,
  batchDelWorkload,
  batchStopWorkload,
  batchCloneWorkload,
  getWorkloadDetail,
} from '@/services/workload/index'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  WorkloadKind,
  WorkloadPhase,
  phaseFilters,
  WorkloadPhaseButtonType,
} from '@/services/workload/type'
import { Search, Refresh, CopyDocument, Plus, InfoFilled } from '@element-plus/icons-vue'
import { copyText, formatTimeStr } from '@/utils/index'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useRoute, useRouter } from 'vue-router'
import { useRouteAction, ROUTE_ACTIONS } from '@/composables/useRouteAction'
import AddDialog from './Components/AddDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Delete,
  DocumentCopy,
  Close,
  MoreFilled,
  Link,
  VideoPlay,
  Edit,
} from '@element-plus/icons-vue'
import { type WorkloadParams, PRIORITY_LABEL_MAP, type PriorityValue } from '@/services'
import { useUserStore } from '@/stores/user'
import { useDark } from '@vueuse/core'
import { usePageTour, waitForEl } from '@/composables/usePageTour'

dayjs.extend(utc)

const tableRef = ref()
const isDark = useDark()
const route = useRoute()
const router = useRouter()
const store = useWorkspaceStore()
const userStore = useUserStore()

const addVisible = ref(false)
const initialSearchParams = {
  userName: '',
  description: '',
  phase: [] as WorkloadPhase[],
  dateRange: '',
  workloadId: '',
  onlyMyself: 'All',
  userId: '',
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
const curAction = ref<'Create' | 'Edit' | 'Clone' | 'Resume'>('Create')

/* ── Page Tours (tourId-driven from Quick Reference ?tour=<id>) ── */
const { getDriver } = usePageTour((tourId) => {
  switch (tourId) {
    /* ─ "Start dev pod" — full create-form walkthrough ─ */
    case 'create':
    default:
      return [
        {
          element: '[data-tour="authoring-create-btn"]',
          popover: {
            title: 'Create Authoring',
            description:
              'Start a new interactive dev pod. You can choose Jupyter, VS Code, or a plain shell.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="authoring-create-btn"]',
          popover: {
            title: 'Walk Through the Form',
            description: 'Click Next to open the creation form and see each key field.',
            side: 'bottom' as const,
            align: 'start' as const,
            onNextClick: async () => {
              addVisible.value = true
              curWlId.value = ''
              curAction.value = 'Create'
              await waitForEl('[data-tour="authoring-field-name"]')
              getDriver()?.moveNext()
            },
          },
        },
        {
          element: '[data-tour="authoring-field-name"]',
          popover: {
            title: 'Pod Name',
            description: 'Give your authoring pod a descriptive name.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="authoring-field-image"]',
          popover: {
            title: 'Container Image',
            description:
              'Pick the base image for your dev environment — e.g. a PyTorch or TensorFlow image.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="authoring-field-resource"]',
          popover: {
            title: 'Resource Allocation',
            description: 'Set CPU, GPU, and memory. Authoring pods always use 1 replica.',
            side: 'left' as const,
            align: 'start' as const,
            onNextClick: () => {
              addVisible.value = false
              getDriver()?.moveNext()
            },
          },
        },
      ]

    /* ─ "Connect to pod" — locate SSH action button ─ */
    case 'connect':
      return [
        {
          element: '[data-tour="authoring-table"]',
          popover: {
            title: 'Find Your Pod',
            description:
              'Locate a pod that is in "Running" state. Only running pods can accept SSH connections.',
            side: 'top' as const,
          },
        },
        {
          element:
            '[data-tour="authoring-table"] .el-table__body-wrapper tr:first-child td:last-child .btn-primary-plain',
          popover: {
            title: 'SSH Button',
            description:
              'This is the SSH action button (🔗). Click it to connect via WebShell (browser terminal) or copy the SSH command for your local terminal.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          popover: {
            title: 'WebShell vs SSH',
            description:
              'WebShell: zero-setup browser terminal — choose container & shell ("sh" if unsure).\nSSH: connect on port 2222 from your local machine. VS Code / Cursor Remote-SSH also supported.',
          },
        },
      ]

    /* ─ "Upload files" — scp / sftp on port 2222 ─ */
    case 'upload':
      return [
        {
          element: '[data-tour="authoring-table"]',
          popover: {
            title: 'Find Your Running Pod',
            description: 'Locate the pod you want to upload files to. It must be in "Running" state.',
            side: 'top' as const,
          },
        },
        {
          element:
            '[data-tour="authoring-table"] .el-table__body-wrapper tr:first-child td:last-child .btn-primary-plain',
          popover: {
            title: 'Get the SSH Address',
            description:
              'Click the SSH button (🔗) to reveal the connection info — you\'ll need the host and session ID.',
            side: 'left' as const,
            align: 'start' as const,
          },
        },
        {
          popover: {
            title: 'Upload via scp / sftp',
            description:
              'Use scp to upload:\n  scp -P 2222 <file> <session>@<host>:<path>\n\nOr open the folder via VS Code / Cursor Remote-SSH file explorer.',
          },
        },
      ]

    /* ─ "Save custom image" — pod detail → Images tab → Save ─ */
    case 'save-image':
      return [
        {
          element: '[data-tour="authoring-table"]',
          popover: {
            title: 'Find Your Pod',
            description:
              'Locate the running pod whose environment you want to snapshot as a custom image.',
            side: 'top' as const,
          },
        },
        {
          element:
            '[data-tour="authoring-table"] .el-table__body-wrapper tr:first-child td:first-child .el-link',
          popover: {
            title: 'Open Pod Detail',
            description:
              'Click the pod name to enter the detail page where you can save images.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
        {
          popover: {
            title: 'Save Image',
            description:
              'In the detail page, switch to the "Images" tab and click "Save Image". Enter a reason, and the snapshot will be saved for future use.',
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
  router.push({ path: '/authoring/detail', query: { id } })
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
      kind: WorkloadKind.Authoring,
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

  const [start, end] = searchParams.dateRange
  fetchData({
    userName: searchParams.userName,
    description: searchParams.description,
    phase: searchParams.phase && searchParams.phase.length ? searchParams.phase.join(',') : '',
    since: start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : '',
    until: end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : '',
    workloadId: searchParams.workloadId,
    userId: searchParams.userId,
  })
}

type Row = { workloadId: string; phase: string; pods: { podId?: string }[]; displayName?: string }
type Action = {
  key: string
  label: string
  icon: any
  btnClass?: string
  disabled?: (row: Row) => boolean
  onClick: (row: Row) => void | Promise<void>
}

const getActions = (row: Row): Action[] => [
  {
    key: 'ssh',
    label: 'SSH',
    icon: Link,
    btnClass: 'btn-primary-plain',
    disabled: (r: Row) => !['Running'].includes((r as any).phase),
    onClick: (r: Row) => openSSH(r),
  },

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
  {
    key: 'resume',
    label: 'Resume',
    icon: VideoPlay,
    btnClass: 'btn-success-plain',
    disabled: (r: Row) => !['Stopped','Failed','Succeeded'].includes((r as any).phase),
    onClick: (r: Row) => {
      curAction.value = 'Resume'
      curWlId.value = r.workloadId
      addVisible.value = true
    },
  },
  {
    key: 'edit',
    label: 'Edit',
    icon: Edit,
    btnClass: 'btn-primary-plain',
    disabled: (r: Row) => !['Running','Pending'].includes((r as any).phase),
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
]

const openSSH = async (r: Row) => {
  try {
    const res = await getWorkloadDetail(r.workloadId)
    const sshCommand = res.pods?.[0]?.sshCommand

    if (!sshCommand) {
      ElMessage.warning('SSH command not found')
      return
    }

    // Copy to clipboard
    copyText(sshCommand)
  } catch (error) {
    console.error('Failed to generate SSH command:', error)
    ElMessage.error('Failed to generate SSH command')
  }
}

type BatchAction = 'delete' | 'stop' | 'clone'

const apiMap: Record<BatchAction, (body: any) => Promise<any>> = {
  delete: batchDelWorkload,
  stop: batchStopWorkload,
  clone: batchCloneWorkload,
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
  name: 'authoringPage',
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

watch(
  // Refresh list data immediately when workspace dropdown changes
  () => store.currentWorkspaceId,
  (id) => {
    if (id) fetchData()
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
