<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Nodes</el-text>
    <el-button
      v-if="isManager"
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="
        () => {
          curAction = 'Create'
          curId = ''
          addVisible = true
        }
      "
    >
      Create Node
    </el-button>
  </div>
  <el-row class="m-t-4" :gutter="20">
    <el-col :span="6">
      <el-input
        v-model="searchParams.search"
        style="width: 100%"
        size="default"
        placeholder="Search by name or IP address"
        clearable
        @input="handleSearchInput"
        @clear="onSearch({ resetPage: true })"
      />
    </el-col>
    <el-col :span="18" class="text-right">
      <el-button :loading="exportLoading" class="btn-ghost" @click="onExport">
        <el-icon class="mr-1"><Download /></el-icon>
        Export
      </el-button>
    </el-col>
  </el-row>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="tableHeight"
      :data="tableData"
      @selection-change="onSelectionChange"
      size="large"
      class="m-t-2"
      v-loading="loading"
      ref="tblRef"
      :element-loading-text="$loadingText"
      @filter-change="handleFilterChange"
    >
      <el-table-column type="selection" width="56" v-if="isManager" />
      <el-table-column prop="nodeId" label="Name/ID" width="200" :fixed="true" align="left">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" @click="jumpToDetail(row.nodeId)">{{ row.nodeName }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.nodeId }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.nodeId)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="internalIP" label="Internal IP" width="140" />
      <el-table-column
        v-if="isManager"
        prop="workspace"
        label="Workspace"
        width="120"
        column-key="wsFilter"
        :filters="wsFilters"
        :filtered-value="filterSelectedIds"
        :filter-multiple="false"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          {{ row.workspace?.name || '-' }}
        </template>
      </el-table-column>
      <el-table-column
        prop="clusterId"
        label="Cluster"
        width="120"
        v-if="isManager"
        column-key="clusterFilter"
        :filters="clusterFilters"
        :filtered-value="filterClusterIds"
        :filter-multiple="false"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          {{ row.clusterId || '-' }}
        </template>
      </el-table-column>
      <el-table-column
        prop="available"
        label="Available"
        width="130"
        :filters="boolFilters"
        :filter-multiple="false"
        column-key="available"
        :filtered-value="searchParams.available === null ? [] : [searchParams.available]"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <AvailableStatusTag
            :available="!!row.available"
            :display="row.available"
            :message="row.message"
          />
        </template>
      </el-table-column>
      <el-table-column
        v-if="isManager"
        prop="phase"
        label="Status"
        width="200"
        column-key="phaseFilters"
        :filters="phaseFilters"
        :filtered-value="phaseSelectedIds"
        :filter-multiple="true"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag :type="row.phase === 'Ready' ? 'success' : 'danger'">
            {{ row.phase }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="workloads" label="Workloads" width="130">
        <template #default="{ row }">
          <div v-if="!row.workloads">-</div>
          <el-tooltip
            v-else
            placement="top"
            :enterable="true"
            :hide-after="0"
            popper-class="wl-rich"
          >
            <template #content>
              <ul class="wl-list">
                <li v-for="(w, i) in row.workloads" :key="w.id ?? i">
                  <span class="wl-id">workload: {{ w.id }}</span>
                  <span class="wl-sub">user: {{ w.userId || '-' }}</span>
                </li>
              </ul>
            </template>

            <div v-if="row.workloads?.length" class="flex items-center">
              running ({{ row.workloads?.length }})<el-icon class="mt-1 ml-1"><Loading /></el-icon>
            </div>
            <span v-else>-</span>
          </el-tooltip>
        </template>
      </el-table-column>

      <el-table-column
        prop="amd.com/gpu"
        label="GPU ( available / total )"
        :sortable="true"
        :sort-orders="['ascending', 'descending', null]"
        :sort-method="(a: any, b: any) => avail(a, 'amd.com/gpu') - avail(b, 'amd.com/gpu')"
        width="210"
      >
        <template #default="{ row }">{{
          ` ${row.availResources?.['amd.com/gpu'] || 0} / ${row.totalResources?.['amd.com/gpu'] || 0}`
        }}</template>
      </el-table-column>
      <el-table-column prop="gpuUtilization" label="GPU Utilization" width="200">
        <template #default="{ row }">
          <el-progress
            v-if="row.gpuUtilization !== undefined && row.gpuUtilization !== null"
            :percentage="parseFloat(row.gpuUtilization) || 0"
            :status="
              parseFloat(row.gpuUtilization) >= 80
                ? 'success'
                : parseFloat(row.gpuUtilization) >= 50
                  ? undefined
                  : 'warning'
            "
            :stroke-width="10"
          >
            <span class="text-xs">{{ row.gpuUtilization }}%</span>
          </el-progress>
          <span v-else class="text-gray-400">-</span>
        </template>
      </el-table-column>
      <el-table-column
        prop="cpu"
        label="CPU ( available / total )"
        :sortable="true"
        :sort-orders="['ascending', 'descending', null]"
        :sort-method="(a: any, b: any) => avail(a, 'cpu') - avail(b, 'cpu')"
        width="210"
      >
        <template #default="{ row }">
          {{ row.availResources?.cpu || 0 }} / {{ row.totalResources?.cpu || 0 }}
        </template>
      </el-table-column>

      <el-table-column
        prop="ephemeral-storage"
        label="ephemeral-storage ( available / total )"
        :sortable="true"
        :sort-orders="['ascending', 'descending', null]"
        :sort-method="
          (a: any, b: any) => avail(a, 'ephemeral-storage') - avail(b, 'ephemeral-storage')
        "
        width="300"
      >
        <template #default="{ row }">
          {{
            `${byte2Gi(row.availResources?.['ephemeral-storage'])} / ${byte2Gi(row.totalResources?.['ephemeral-storage'])}`
          }}
        </template>
      </el-table-column>
      <el-table-column
        label="Memory ( available / total )"
        :sortable="true"
        :sort-orders="['ascending', 'descending', null]"
        :sort-method="(a: any, b: any) => avail(a, 'memory') - avail(b, 'memory')"
        min-width="230"
      >
        <template #default="{ row }">
          {{ byte2Gi(row.availResources?.memory) }} / {{ byte2Gi(row.totalResources?.memory) }}
        </template>
      </el-table-column>

      <el-table-column
        prop="rdma/hca"
        label="rdma/hca ( available / total )"
        :sortable="true"
        :sort-orders="['ascending', 'descending', null]"
        :sort-method="(a: any, b: any) => avail(a, 'rdma/hca') - avail(b, 'rdma/hca')"
        width="240"
      >
        <template #default="{ row }">
          {{
            ` ${row.availResources?.['rdma/hca'] || 0} / ${row.totalResources?.['rdma/hca'] || 0}`
          }}
        </template>
      </el-table-column>
      <el-table-column prop="isControlPlane" label="Control Plane" width="120">
        <template #default="{ row }">
          <el-tag :type="row.isControlPlane ? 'success' : 'danger'">{{
            row.isControlPlane
          }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column
        prop="isAddonsInstalled"
        label="Addons Installed"
        width="150"
        column-key="isAddonsInstalled"
        v-if="isManager"
        :filters="boolFilters"
        :filter-multiple="false"
        :filtered-value="
          searchParams.isAddonsInstalled === null ? [] : [searchParams.isAddonsInstalled]
        "
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag :type="row.isAddonsInstalled ? 'success' : 'danger'">{{
            row.isAddonsInstalled
          }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="180" fixed="right">
        <template #default="{ row }">
          <!-- First 3 inline -->
          <template v-for="act in visibleActs(row).slice(0, 2)" :key="act.key">
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
            v-if="visibleActs(row).length > 2"
            placement="bottom-start"
            trigger="click"
            :width="240"
            :teleported="true"
            :enterable="true"
            popper-class="actions-menu"
            :visible="moreOpenId === row.nodeId"
            @hide="moreOpenId === row.nodeId && (moreOpenId = null)"
          >
            <template #reference>
              <el-button
                circle
                class="btn-primary-plain"
                :icon="MoreFilled"
                size="default"
                @click.stop="toggleMore(row.nodeId)"
              />
            </template>

            <ul class="menu-col">
              <li
                v-for="act in visibleActs(row).slice(2)"
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
            >Selected {{ selectedRows.length }} item{{ selectedRows.length === 1 ? '' : 's' }}</span
          >
        </div>

        <div class="right">
          <el-button type="primary" :disabled="rebootDis" plain @click="handleReboot(true)"
            >Reboot</el-button
          >
          <span class="sep-ghost" aria-hidden="true"></span>
          <el-button
            type="danger"
            :disabled="unManageDis"
            plain
            @click="openManage('Unmanage', true)"
            >UnManage</el-button
          >
          <el-button type="success" :disabled="manageDis" plain @click="openManage('Manage', true)"
            >Manage</el-button
          >
          <el-button type="primary" plain @click="handleRetryBatch">Retry</el-button>
          <span class="sep-ghost" aria-hidden="true"></span>
          <el-button type="warning" :disabled="unBindDis" plain @click="openBindWl('remove', true)"
            >UnBind</el-button
          >
          <el-button type="success" :disabled="bindDis" plain @click="openBindWl('add', true)"
            >Bind</el-button
          >
          <span class="sep-ghost" aria-hidden="true"></span>
          <el-button type="danger" plain @click="onDelete(true)">Delete</el-button>
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
  </el-card>
  <AddNodeDialog
    v-model:visible="addVisible"
    :nodeid="curId"
    :action="curAction"
    @success="onSearch({ resetPage: true })"
  />

  <BindDialog
    v-model:visible="bindVisible"
    :action="bindAction"
    :wsId="selectedWsId"
    :nodeIds="curNodeId"
    @success="onSearch({ resetPage: false })"
  />

  <ManageDialog
    v-model:visible="manageVisible"
    :action="manageAction"
    :clusterId="selectedClusterId"
    :nodeIds="curNodeId"
    @success="onSearch({ resetPage: false })"
  />
</template>

<script lang="ts" setup>
import { onMounted, ref, computed, watch, reactive, h, nextTick, type Component } from 'vue'
import {
  getNodesList,
  deleteNode,
  NODE_PHASE,
  rebootNodes,
  exportNodes,
  deleteNodes,
} from '@/services'
import type { NodesParams } from '@/services'
import {
  CopyDocument,
  Loading,
  MoreFilled,
  Delete,
  Edit,
  Plus,
  Minus,
  Refresh,
  RefreshRight,
  Download,
} from '@element-plus/icons-vue'
import { copyText, byte2Gi } from '@/utils/index'
import AddNodeDialog from './Components/AddNodeDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import AvailableStatusTag from '@/components/Base/AvailableStatusTag.vue'
import BindDialog from './Components/BindDialog.vue'
import ManageDialog from './Components/ManageDialog.vue'
import { useClusterStore } from '@/stores/cluster'
import { useNodeRetry } from '@/composables/useNodeRetry'

const route = useRoute()
const router = useRouter()

const store = useClusterStore()
const wsStore = useWorkspaceStore()
const userStore = useUserStore()
const isManager = computed(() => userStore.isManager) // Manager role

// nodes table initial val
const loading = ref(false)
const tableData = ref([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})

// add & edit
const curId = ref('')
const curAction = ref('Create')
const addVisible = ref(false)
// bind
const bindVisible = ref(false)
const bindAction = ref('add')
// manage
const manageVisible = ref(false)
const manageAction = ref('Manage')

const exportLoading = ref(false)

const selectedWsId = ref(wsStore.currentWorkspaceId || '')
const selectedClusterId = ref('')
const curNodeId = ref<string[]>([''])

// Search parameters
const searchParams = reactive({
  search: '',
  workspaceId: '',
  clusterId: '',
  available: null as boolean | null,
  isAddonsInstalled: null as boolean | null,
  phase: null as string | null,
})

const SELECTION_BAR_H = 56
// const BASE_OFFSET = 245

const onExport = async () => {
  try {
    await ElMessageBox.confirm(
      'This will export the current node list as a CSV based on your filters. Continue?',
      'Export nodes',
      {
        type: 'warning',
        confirmButtonText: 'Export',
        cancelButtonText: 'Cancel',
      },
    )
  } catch {
    return
  }

  exportLoading.value = true
  try {
    await exportNodes({
      ...(searchParams.available !== null ? { available: searchParams.available } : {}),
      ...(searchParams.isAddonsInstalled !== null
        ? { isAddonsInstalled: searchParams.isAddonsInstalled }
        : {}),
      ...(searchParams.workspaceId
        ? { workspaceId: searchParams.workspaceId === 'UNASSIGNED' ? '' : searchParams.workspaceId }
        : {}),
      ...(searchParams.search ? { search: searchParams.search } : {}),
      ...(searchParams.phase ? { phase: searchParams.phase } : {}),
      ...(searchParams.clusterId
        ? { clusterId: searchParams.clusterId === 'UNASSIGNED' ? '' : searchParams.clusterId }
        : {}),
    })
  } finally {
    exportLoading.value = false
  }
}

// Multi-select
type UnknownRecord = Record<string, unknown>
const isRecord = (v: unknown): v is UnknownRecord => typeof v === 'object' && v !== null
const isNonEmptyString = (v: unknown): v is string => typeof v === 'string' && v.trim().length > 0
const getSelectedNodeIds = () => selectedRows.value.map((r) => r['nodeId']).filter(isNonEmptyString)
const selectedRows = ref<UnknownRecord[]>([])
function onSelectionChange(rows: UnknownRecord[]) {
  selectedRows.value = rows
}

// ===========Multi-select reboot disable conditions
const isRebootable = (r: UnknownRecord) => {
  const workloads = r['workloads']
  if (!Array.isArray(workloads) || workloads.length === 0) return true
  const first = workloads[0]
  if (!isRecord(first)) return true
  return !first['id']
}
// const isRebootable = (r: any) => !r?.available || r?.phase !== 'Ready'
const allRebootable = computed(() => {
  const rows = selectedRows.value
  if (!rows?.length) return false
  return rows.every(isRebootable)
})
const isRebootMixed = computed(() => {
  const rows = selectedRows.value
  return rows.some(isRebootable) && rows.some((r) => !isRebootable(r))
})
const rebootDis = computed(() => !allRebootable.value || isRebootMixed.value)

// ===========Multi-select bind disable conditions
const hasWs = (row: UnknownRecord) => {
  const ws = row['workspace']
  return isRecord(ws) && Boolean(ws['id'])
}
const allBindable = computed(() => {
  if (selectedRows.value.length === 0) return false
  const clusters = selectedRows.value.map((r) => (r['clusterId'] as string) ?? '')
  return new Set(clusters).size === 1 && selectedRows.value.every((r) => !hasWs(r))
})
const allUnbindableSameWs = computed(() => {
  if (selectedRows.value.length === 0) return false
  const ids = selectedRows.value
    .map((r) => {
      const ws = r['workspace']
      if (!isRecord(ws)) return null
      return (ws['id'] as string) ?? null
    })
    .filter(Boolean) as string[]
  // Every row must have workspace.id, and all IDs must be the same
  return ids.length === selectedRows.value.length && new Set(ids).size === 1
})
// Whether mixed selection
const isMixed = computed(
  () => selectedRows.value.some(hasWs) && selectedRows.value.some((r) => !hasWs(r)),
)
const bindDis = computed(() => !allBindable.value || isMixed.value)
const unBindDis = computed(() => !allUnbindableSameWs.value || isMixed.value)
// workspaceId when unbinding
const unbindWorkspaceId = computed(() => {
  if (!allUnbindableSameWs.value) return null
  const ws = selectedRows.value[0]?.['workspace']
  if (!isRecord(ws)) return null
  return (ws['id'] as string) ?? null
})

// ===========Multi-select manage disable conditions
const hasCluster = (row: UnknownRecord) => Boolean(row['clusterId'])
const allManageable = computed(() => {
  return selectedRows.value.length !== 0 && selectedRows.value.every((r) => !hasCluster(r))
})
const allUnManageableSameWs = computed(() => {
  if (selectedRows.value.length === 0) return false
  const ids = selectedRows.value.map((r) => r['clusterId']).filter(Boolean) as string[]
  // Every row must have clusterId, and all IDs must be the same
  return ids.length === selectedRows.value.length && new Set(ids).size === 1
})
// Whether mixed selection
const isManagesMixed = computed(
  () => selectedRows.value.some(hasCluster) && selectedRows.value.some((r) => !hasCluster(r)),
)
const manageDis = computed(() => !allManageable.value || isManagesMixed.value)
const unManageDis = computed(() => !allUnManageableSameWs.value || isManagesMixed.value)
// clusterId when unmanaging
const unManageClusterId = computed(() => {
  if (!allUnManageableSameWs.value) return null
  return (selectedRows.value[0]?.['clusterId'] as string) ?? null
})

// Batch action bar placeholder related
const hasSelection = computed(() => selectedRows.value.length > 0)
const hasBarSpace = ref(false)
watch(hasSelection, (v) => {
  if (v) hasBarSpace.value = true // Space reserved for selection bar
})
function onBarAfterLeave() {
  hasBarSpace.value = false
}
const tableHeight = computed(() => {
  const extra = hasBarSpace.value ? SELECTION_BAR_H : 0
  return `calc(100vh - ${extra}px - ${isManager.value ? 295 : 255}px)`
})

// Sorting
const tblRef = ref()
const n = (v: unknown) => (Number.isFinite(Number(v)) ? Number(v) : 0)
const avail = (row: UnknownRecord, key: string) => {
  const ar = row['availResources']
  if (!isRecord(ar)) return 0
  return n(ar[key])
}
// Available and addons filter
const boolFilters = computed(() => [
  { text: 'true', value: 'true' },
  { text: 'false', value: 'false' },
])
// statusFilter
const phaseSelectedIds = ref<string[]>([])
type NodePhase = (typeof NODE_PHASE)[number]
const phaseFilters = computed(() => NODE_PHASE.map((p) => ({ text: p, value: p as NodePhase })))

// Workspace filter options
const filterSelectedIds = ref<string[]>([])
const wsFilters = computed(() => [
  { text: 'Unassigned', value: 'UNASSIGNED' },
  ...(wsStore.items || []).map((ws) => ({
    text: ws.workspaceName,
    value: ws.workspaceId,
  })),
])

// Cluster filter options
const filterClusterIds = ref<string[]>([])
const clusterFilters = computed(() => [
  { text: 'Unassigned', value: 'UNASSIGNED' },
  ...(store.items || []).map((ws) => ({
    text: ws.clusterId,
    value: ws.clusterId,
  })),
])

// Debounced search
let searchTimer: ReturnType<typeof setTimeout> | null = null
const handleSearchInput = () => {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  searchTimer = setTimeout(() => {
    onSearch({ resetPage: true })
  }, 500)
}

const passAll = () => true
const handleFilterChange = (filters: Record<string, string[]>) => {
  if ('clusterFilter' in filters) {
    searchParams.clusterId = filters.clusterFilter?.[0]
  }
  if ('wsFilter' in filters) {
    searchParams.workspaceId = filters.wsFilter?.[0]
  }
  if ('available' in filters) {
    searchParams.available = filters.available?.length ? filters.available[0] === 'true' : null
  }
  if ('isAddonsInstalled' in filters) {
    searchParams.isAddonsInstalled = filters.isAddonsInstalled?.length
      ? filters.isAddonsInstalled[0] === 'true'
      : null
  }
  if ('phaseFilters' in filters) {
    const arr = filters.phaseFilters ?? []
    searchParams.phase = arr.length ? arr.join(',') : null
  }

  onSearch({ resetPage: true })
}

const openBindWl = async (action: string, isBatch: boolean, nodeId?: string, wsid?: string) => {
  bindAction.value = action
  selectedWsId.value =
    (isBatch ? unbindWorkspaceId.value : wsid) ?? wsStore.currentWorkspaceId ?? ''
  curNodeId.value = isBatch ? getSelectedNodeIds() : nodeId ? [nodeId] : []
  bindVisible.value = true
}

const openManage = async (
  action: string,
  isBatch: boolean,
  nodeId?: string,
  clusterId?: string,
) => {
  manageAction.value = action
  curNodeId.value = isBatch ? getSelectedNodeIds() : nodeId ? [nodeId] : []
  selectedClusterId.value = (isBatch ? unManageClusterId.value : clusterId) ?? ''
  manageVisible.value = true
}

const handleReboot = async (isBatch: boolean, nodeId?: string) => {
  curNodeId.value = isBatch ? getSelectedNodeIds() : nodeId ? [nodeId] : []

  const msg = h('span', null, [
    'Are you sure you want to reboot node(s): ',
    h(
      'span',
      { style: 'color: var(--el-color-primary); font-weight: 600' },
      curNodeId.value.join(','),
    ),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Reboot node', {
    confirmButtonText: 'Reboot',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      const batchNodeIds = Array.from(new Set(getSelectedNodeIds()))
      const payload = {
        type: 'reboot',
        inputs: isBatch
          ? batchNodeIds.map((id) => ({
              name: 'node',
              value: id,
            }))
          : [
              {
                name: 'node',
                value: nodeId ?? '',
              },
            ],
        name: `reboot-${Date.now()}`,
      }

      await rebootNodes(payload)
      ElMessage({
        type: 'success',
        message: 'Reboot completed',
      })
      onSearch({ resetPage: false })
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Reboot canceled')
      }
    })
}

// Use Node Retry composable
const { handleRetry } = useNodeRetry({
  onRefresh: () => onSearch({ resetPage: false }),
})

// Batch retry
const handleRetryBatch = () => {
  handleRetry(true, undefined, getSelectedNodeIds())
}

const fetchData = async (params?: NodesParams) => {
  try {
    loading.value = true

    const res = await getNodesList({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      ...(!isManager.value ? { workspaceId: wsStore.currentWorkspaceId } : {}),
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

const onSearch = (options?: { resetPage?: boolean }) => {
  const reset = options?.resetPage ?? true
  if (reset) pagination.page = 1

  router.replace({
    query: {
      ...route.query,
      search: searchParams.search || undefined,
      workspaceId: searchParams.workspaceId || undefined,
      clusterId: searchParams.clusterId || undefined,
      phase: searchParams.phase || undefined,
      available: searchParams.available === null ? undefined : String(searchParams.available),
      isAddonsInstalled:
        searchParams.isAddonsInstalled === null
          ? undefined
          : String(searchParams.isAddonsInstalled),
      page: String(pagination.page),
      pageSize: String(pagination.pageSize),
    },
  })

  fetchData({
    ...(searchParams.available !== null ? { available: searchParams.available } : {}),
    ...(searchParams.isAddonsInstalled !== null
      ? { isAddonsInstalled: searchParams.isAddonsInstalled }
      : {}),
    ...(searchParams.workspaceId
      ? { workspaceId: searchParams.workspaceId === 'UNASSIGNED' ? '' : searchParams.workspaceId }
      : {}),
    ...(searchParams.search ? { search: searchParams.search } : {}),
    ...(searchParams.phase ? { phase: searchParams.phase } : {}),
    ...(searchParams.clusterId
      ? { clusterId: searchParams.clusterId === 'UNASSIGNED' ? '' : searchParams.clusterId }
      : {}),
  })
}

const parseBool = (v?: string | null) => (v === 'true' ? true : v === 'false' ? false : null)
function applyQueryToParams() {
  // Restore query params when navigating back from detail page
  const q = route.query
  searchParams.search = (q.search as string) || ''
  searchParams.workspaceId = (q.workspaceId as string) || ''
  searchParams.clusterId = (q.clusterId as string) || ''
  searchParams.phase = (q.phase as string) || null
  searchParams.available = parseBool((q.available as string) ?? null)
  searchParams.isAddonsInstalled = parseBool((q.isAddonsInstalled as string) ?? null)

  pagination.page = Number(q.page || 1)
  pagination.pageSize = Number(q.pageSize || pagination.pageSize)
}

const jumpToDetail = (id: string) => {
  router.push({ path: '/nodedetail', query: { id } })
}

// Collapse action buttons
const moreOpenId = ref<string | null>(null) // Currently open popover row ID
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

type Row = { nodeId: string; phase: string; workspace: { id: string }; clusterId: string }
type Action = {
  key: string
  label: string
  icon: Component
  btnClass?: string
  disabled?: (row: Row) => boolean
  onClick: (row: Row) => void | Promise<void>
  show?: boolean | ((r: Row) => boolean)
}
const isVisible = (act: Action) => act.show !== false
const visibleActs = (row: Row) => getActions(row).filter(isVisible)

const isAssigned = (r: Row) => !!(r.workspace?.id && r.workspace.id !== 'UNASSIGNED')
const isUnManage = (r: Row) => !!r.clusterId
const getActions = (row: Row): Action[] => [
  {
    key: 'delete',
    label: 'Delete',
    icon: Delete,
    btnClass: 'btn-danger-plain',
    onClick: (r: Row) => onDelete(false, r.nodeId),
    show: !!isManager.value,
  },
  {
    key: 'edit',
    label: 'Edit',
    icon: Edit,
    btnClass: 'btn-primary-plain',
    onClick: (r: Row) => {
      curAction.value = 'Edit'
      curId.value = r.nodeId
      addVisible.value = true
    },
  },
  {
    key: 'bind',
    label: 'Bind',
    icon: Plus,
    btnClass: 'btn-success-plain',
    onClick: async (r: Row) => openBindWl('add', false, r.nodeId),
    show: !isAssigned(row) && !!isManager.value,
  },
  {
    key: 'unbind',
    label: 'UnBind',
    icon: Minus,
    btnClass: 'btn-warning-plain',
    onClick: async (r: Row) => openBindWl('remove', false, r.nodeId, r.workspace?.id),
    show: isAssigned(row) && !!isManager.value,
  },

  {
    key: 'manage',
    label: 'Manage',
    icon: Plus,
    btnClass: 'btn-success-plain',
    onClick: async (r: Row) => openManage('Manage', false, r.nodeId, r.clusterId),
    show: !isUnManage(row) && !!isManager.value,
  },
  {
    key: 'unmanage',
    label: 'UnManage',
    icon: Minus,
    btnClass: 'btn-danger-plain',
    onClick: async (r: Row) => openManage('Unmanage', false, r.nodeId, r.clusterId),
    show: isUnManage(row) && !!isManager.value,
  },
  {
    key: 'retry',
    label: 'Retry',
    icon: RefreshRight,
    btnClass: 'btn-primary-plain',
    onClick: async (r: Row) => handleRetry(false, r.nodeId),
    show: !!isManager.value,
  },
  {
    key: 'reboot',
    label: 'Reboot',
    icon: Refresh,
    btnClass: 'btn-primary-plain',
    onClick: async (r: Row) => handleReboot(false, r.nodeId),
    show: isRebootable(row),
  },
]

const onDelete = async (isBatch: boolean, id?: string) => {
  const ids: string[] = isBatch ? Array.from(new Set(getSelectedNodeIds())) : id ? [id] : []

  if (!ids.length) {
    ElMessage.warning('Please select at least one node.')
    return
  }

  const msg = h('span', null, [
    'Are you sure you want to delete node(s): ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, ids.join(',')),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, 'Delete node', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
  } catch (err) {
    if (err === 'cancel' || err === 'close') {
      ElMessage.info('Delete canceled')
    }
    return
  }

  try {
    if (isBatch) {
      await deleteNodes({ nodeIds: ids })
    } else {
      await deleteNode(ids[0])
    }
    ElMessage.success('Delete completed')
    onSearch({ resetPage: false })
  } catch {}
}

onMounted(() => {
  applyQueryToParams()
  onSearch({ resetPage: false })
})

watch(
  // Refresh on workspace dropdown change - update list data immediately
  () => wsStore.currentWorkspaceId,
  (id) => {
    if (id && !isManager.value) onSearch({ resetPage: true })
  },
)

defineOptions({
  name: 'NodesPage',
})
</script>

<style scoped>
.cell-ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.wl-rich {
  max-width: 420px;
  padding: 8px 10px;
}
.wl-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.wl-list li + li {
  margin-top: 6px;
}
.wl-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-weight: 600;
}
.wl-sub {
  font-size: 12px;
  opacity: 0.8;
  margin-left: 10px;
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
  /* Optional: tighten left-right content spacing */
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

.sep-ghost {
  position: relative;
  display: inline-block;
  width: 14px;
  height: 18px;
  margin: 0 50px;
  vertical-align: middle;
}
.sep-ghost::before {
  content: '';
  position: absolute;
  left: 50%;
  top: 0;
  bottom: 0;
  width: 1px;
  transform: translateX(-50%);
  background: color-mix(in oklab, var(--safe-border) 80%, var(--safe-text) 20%);
  opacity: 0.6;
}
.sep-ghost::after {
  content: '';
  position: absolute;
  inset: 0;
  box-shadow:
    0 0 0 999px transparent,
    /* placeholder */ -6px 0 8px -7px rgba(0, 0, 0, 0.25),
    6px 0 8px -7px rgba(0, 0, 0, 0.25); /* Soft shadow on both sides */
  pointer-events: none;
}
.btn-ghost {
  background: var(--safe-card, var(--el-fill-color-blank));
  border: 1px solid var(--el-border-color);
  color: var(--el-text-color-regular);
}
.btn-ghost:hover {
  filter: brightness(0.98);
  border-color: var(--safe-primary, var(--el-color-primary));
  color: var(--el-color-primary);
}
</style>
