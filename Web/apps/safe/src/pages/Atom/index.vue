<template>
  <div class="flex items-center justify-between">
    <h1 class="fs-title">Atom</h1>
    <el-button type="primary" :icon="Plus" :disabled="!canWrite" @click="openCreate">Create Atom</el-button>
  </div>

  <el-card class="mt-4 safe-card" shadow="never">
    <div class="flex items-center gap-2 mb-3">
      <el-input
        v-model="keyword"
        clearable
        placeholder="Workload ID"
        style="width: 280px"
        @keyup.enter="onSearch(true)"
        @clear="onSearch(true)"
      />
      <el-button :icon="Search" @click="onSearch(true)">Search</el-button>
    </div>
    <el-table
      :data="tableData"
      v-loading="loading"
      :height="tableHeight"
      @filter-change="handleFilterChange"
    >
      <el-table-column prop="workloadId" label="Name/ID" min-width="220" :fixed="true">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" @click="jumpToDetail(row.workloadId)">{{ row.displayName }}</el-link>
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
      >
        <template #default="{ row }">
          <el-tag :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'">{{ row.phase }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Roles" min-width="220">
        <template #default="{ row }">
          <span>router / prefill / decode</span>
          <div class="text-xs text-gray-400">{{ roleReplicas(row) }}</div>
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" width="180">
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
      class="mt-3"
      :current-page="pagination.page"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50]"
      @current-change="handlePageChange"
      @size-change="handlePageSizeChange"
    />
  </el-card>

  <AddDialog
    v-model:visible="addVisible"
    :wlid="curWlId"
    :action="curAction"
    @success="onSearch(false)"
  />
</template>

<script lang="ts" setup>
import { computed, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, CopyDocument, Delete, DocumentCopy, MoreFilled, Plus, Search, VideoPlay } from '@element-plus/icons-vue'
import { deleteWorkload, getWorkloadsList, stopWorkload } from '@/services'
import { WorkloadKind, WorkloadPhaseButtonType, phaseFilters, type WorkloadParams, type WorkloadPhase } from '@/services/workload/type'
import { copyText, formatTimeStr } from '@/utils/index'
import { useWorkspaceStore } from '@/stores/workspace'
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'
import {
  ensureResumeCooldownElapsed,
  useWorkloadResumePermission,
} from '@/composables/useWorkloadResumePermission'
import AddDialog from './Components/AddDialog.vue'

const router = useRouter()
const store = useWorkspaceStore()
const { canWrite } = useWorkloadWriteGuard()
const { getResumeDisabled, getResumeTooltip } = useWorkloadResumePermission(canWrite)
const loading = ref(false)
const tableData = ref<AtomRow[]>([])
const keyword = ref('')
const phase = ref<WorkloadPhase[]>([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const addVisible = ref(false)
const curWlId = ref('')
const curAction = ref<'Create' | 'Clone' | 'Resume'>('Create')
const moreOpenId = ref<string | null>(null)
const tableHeight = computed(() => 'calc(100vh / var(--zoom) - 280px)')

interface AtomRow {
  workloadId: string
  displayName?: string
  phase?: string
  endTime?: string
  userId?: string
  workspaceId?: string
  workspace?: string
  resources?: Array<{ replica?: number | string }>
}

function roleReplicas(row: AtomRow) {
  const resources = row.resources || []
  return ['router', 'prefill', 'decode']
    .map((name, index) => `${name} x${resources[index]?.replica ?? '-'}`)
    .join('  ')
}

async function fetchData() {
  loading.value = true
  try {
    const params: WorkloadParams = {
      workspaceId: store.currentWorkspaceId,
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      kind: WorkloadKind.Atom,
      ...(keyword.value.trim() ? { workloadId: keyword.value.trim() } : {}),
      ...(phase.value.length ? { phase: phase.value } : {}),
    }
    const res = await getWorkloadsList(params)
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } finally {
    loading.value = false
  }
}

function onSearch(resetPage: boolean) {
  if (resetPage) pagination.page = 1
  fetchData()
}

function handlePageChange(page: number) {
  pagination.page = page
  fetchData()
}

function handlePageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.page = 1
  fetchData()
}

function handleFilterChange(filters: Record<string, string[]>) {
  if (Object.prototype.hasOwnProperty.call(filters, 'phase')) {
    phase.value = (filters.phase || []) as WorkloadPhase[]
    onSearch(true)
  }
}

function jumpToDetail(id: string) {
  router.push({ path: '/atom/detail', query: { id } })
}

function openCreate() {
  curWlId.value = ''
  curAction.value = 'Create'
  addVisible.value = true
}

function openClone(row: AtomRow) {
  curAction.value = 'Clone'
  curWlId.value = row.workloadId
  addVisible.value = true
}

function openResume(row: AtomRow) {
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
  disabled?: (row: AtomRow) => boolean
  tooltip?: (row: AtomRow) => string
  onClick: (row: AtomRow) => void | Promise<void>
}

/** Phases that still own live resources and can be stopped. */
const STOPPABLE_PHASES = ['Running', 'Pending', 'NotReady']

/** Inline Resume and Clone, with Stop and Delete under the more menu. */
function getActions(_row: AtomRow): Action[] {
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
        return 'Stop is only available for Running, Pending, or NotReady workloads'
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

async function handleMenuClick(act: Action, row: AtomRow) {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

async function onStop(row: AtomRow) {
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
  fetchData()
}

async function onDelete(row: AtomRow) {
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
  fetchData()
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
  () => onSearch(true),
)

onMounted(() => {
  window.addEventListener('scroll', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('wheel', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('touchmove', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('pointerdown', onAnyPointerDown, { capture: true })
  fetchData()
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as EventListenerOptions)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as EventListenerOptions)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as EventListenerOptions)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as EventListenerOptions)
})
</script>
