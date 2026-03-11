<template>
  <div v-if="detailData" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
          Back
        </el-button>
        <h1 class="w-name">{{ detailData.clusterId }}</h1>

        <el-tag class="ml-4" :type="detailData.phase === 'Ready' ? 'success' : 'danger'">{{
          detailData.phase
        }}</el-tag>
      </div>
    </div>

    <div class="w-meta">
      <span class="item">
        <span class="label">ID</span>
        <code class="code">{{ detailData.clusterId }}</code>
        <el-icon
          class="copy"
          size="12"
          style="color: var(--safe-primary)"
          @click="copyText(detailData.clusterId)"
        >
          <CopyDocument />
        </el-icon>
      </span>
      <span class="sep">•</span>
      <span class="item"><span class="label">user</span>{{ detailData.userId || '-' }}</span>
      <span class="sep">•</span>
      <span class="item"
        ><span class="label">creationTime</span
        >{{
          detailData.creationTime
            ? dayjs(detailData.creationTime).format('YYYY-MM-DD HH:mm:ss')
            : '-'
        }}</span
      >
      <span class="sep">•</span>
      <span class="item"
        ><span class="label">description</span>
        <span class="truncate max-w-[42ch]" :title="detailData.description || '-'">
          {{ detailData.description || '-' }}
        </span>
      </span>
    </div>
  </div>

  <el-card class="mt-4 safe-card" shadow="never">
    <el-descriptions
      v-if="detailData"
      v-loading="detailLoading"
      :element-loading-text="$loadingText"
      border
    >
      <el-descriptions-item label="endpoint" :span="2">{{
        detailData.endpoint
      }}</el-descriptions-item>
      <el-descriptions-item label="labels">
        <span v-if="Object.keys(detailData.kubeApiServerArgs).length === 0">-</span>
        <span v-else>
          <span v-for="(item, index) in serverArgsList" :key="item[0]">
            {{ item[0] }}: {{ item[1]
            }}<span v-if="index < serverArgsList.length - 1"><br /> </span>
          </span>
        </span>
      </el-descriptions-item>
      <el-descriptions-item label="imageSecret" :span="2">{{
        detailData.imageSecretId
      }}</el-descriptions-item>

      <el-descriptions-item label="description" :span="2">{{
        detailData.description
      }}</el-descriptions-item>
      <el-descriptions-item label="isProtected">{{ detailData.isProtected }}</el-descriptions-item>

      <el-descriptions-item label="kubeNetworkPlugin">{{
        detailData.kubeNetworkPlugin
      }}</el-descriptions-item>

      <el-descriptions-item label="kubePodsSubnet">{{
        detailData.kubePodsSubnet
      }}</el-descriptions-item>

      <el-descriptions-item label="kubeServiceAddress">{{
        detailData.kubeServiceAddress
      }}</el-descriptions-item>

      <el-descriptions-item label="kubeSprayImage">{{
        detailData.kubeSprayImage
      }}</el-descriptions-item>

      <el-descriptions-item label="kubernetesVersion">{{
        detailData.kubernetesVersion
      }}</el-descriptions-item>

      <el-descriptions-item label="sshSecretId">{{ detailData.sshSecretId }}</el-descriptions-item>

      <el-descriptions-item label="nodes">{{ detailData.nodes?.join(', ') }}</el-descriptions-item>
      <el-descriptions-item label="storage" v-if="detailData.storage?.length">{{
        detailData.storage
      }}</el-descriptions-item>
    </el-descriptions>
  </el-card>
  <!-- Detail page node list -->
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :data="tableData"
      @selection-change="onSelectionChange"
      size="large"
      class="m-t-2"
      v-loading="loading"
      ref="tblRef"
      :element-loading-text="$loadingText"
      @filter-change="handleFilterChange"
    >
      <el-table-column type="selection" width="56" />
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
          <el-button type="danger" :disabled="unManageDis" plain @click="openManage"
            >UnManage</el-button
          >
          <el-button type="primary" plain @click="handleRetryBatch">Retry</el-button>
          <span class="sep-ghost" aria-hidden="true"></span>
          <el-button type="success" :disabled="bindDis" plain @click="openBindWl('add', true)"
            >Bind</el-button
          >
          <el-button type="warning" :disabled="unBindDis" plain @click="openBindWl('remove', true)"
            >UnBind</el-button
          >
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
  <BindDialog
    v-model:visible="bindVisible"
    :action="bindAction"
    :wsId="selectedWsId"
    :nodeIds="curNodeId"
    @success="onSearch({ resetPage: false })"
  />
  <ManageDialog
    v-model:visible="manageVisible"
    action="Unmanage"
    :clusterId="selectedClusterId"
    :nodeIds="curNodeId"
    @success="onSearch({ resetPage: false })"
  />
</template>
<script lang="ts" setup>
import { getClusterDetail, getNodesList, type NodesParams, NODE_PHASE } from '@/services'
import { onMounted, ref, computed, reactive, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import dayjs from 'dayjs'
import { copyText, byte2Gi } from '@/utils/index'
import { CopyDocument, ArrowLeft, Loading } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import AvailableStatusTag from '@/components/Base/AvailableStatusTag.vue'
import BindDialog from '@/pages/Nodes/Components/BindDialog.vue'
import ManageDialog from '@/pages/Nodes/Components/ManageDialog.vue'
import { useNodeRetry } from '@/composables/useNodeRetry'

const route = useRoute()
const wsId = computed(() => route.query.id as string | undefined)

const detailData = ref()
const detailLoading = ref(false)

const serverArgsList = computed(() => {
  const labels = detailData.value.kubeApiServerArgs || {}
  return Object.entries(labels)
})

const getDetail = async () => {
  if (!wsId.value) return
  detailLoading.value = true
  detailData.value = await getClusterDetail(wsId.value)
  detailLoading.value = false
}

onMounted(async () => {
  await getDetail()
  await onSearch({ resetPage: true })
})

// ==================== Node list section =====================

// nodes table initial val

const router = useRouter()
const wsStore = useWorkspaceStore()
const loading = ref(false)
const tableData = ref([])
const bindVisible = ref(false)
const bindAction = ref('add')
const manageVisible = ref(false)
const selectedWsId = ref(wsStore.currentWorkspaceId || '')
const selectedClusterId = ref('')
const curNodeId = ref<string[]>([''])

const searchParams = reactive({
  nodeId: '',
  workspaceId: '',
  clusterId: '',
  available: null as boolean | null,
  isAddonsInstalled: null as boolean | null,
  phase: null as string | null,
})
const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0,
})

type UnknownRecord = Record<string, unknown>
const isRecord = (v: unknown): v is UnknownRecord => typeof v === 'object' && v !== null
const isNonEmptyString = (v: unknown): v is string => typeof v === 'string' && v.trim().length > 0
const getSelectedNodeIds = () => selectedRows.value.map((r) => r['nodeId']).filter(isNonEmptyString)
const getSelectedClusterIds = () =>
  selectedRows.value.map((r) => r['clusterId']).filter(isNonEmptyString)
const getSelectedWorkspaceIds = () =>
  selectedRows.value
    .map((r) => {
      const ws = r['workspace']
      if (!isRecord(ws)) return null
      return ws['id']
    })
    .filter(isNonEmptyString)

const selectedRows = ref<UnknownRecord[]>([])
function onSelectionChange(rows: UnknownRecord[]) {
  selectedRows.value = rows
}

// =========== Multi-select bind disable conditions ===========
const hasWs = (row: UnknownRecord) => {
  const ws = row['workspace']
  return isRecord(ws) && Boolean(ws['id'])
}
const allBindable = computed(() => {
  if (selectedRows.value.length === 0) return false
  const clusters = getSelectedClusterIds()
  return new Set(clusters).size === 1 && selectedRows.value.every((r) => !hasWs(r))
})
const allUnbindableSameWs = computed(() => {
  if (selectedRows.value.length === 0) return false
  const ids = getSelectedWorkspaceIds()
  // Every row must have workspace.id, and all ids must be the same
  return ids.length === selectedRows.value.length && new Set(ids).size === 1
})
// Whether mixed selection
const isMixed = computed(
  () => selectedRows.value.some(hasWs) && selectedRows.value.some((r) => !hasWs(r)),
)
const bindDis = computed(() => !allBindable.value || isMixed.value)
const unBindDis = computed(() => !allUnbindableSameWs.value || isMixed.value)
// workspaceId for unbind operation
const unbindWorkspaceId = computed(() => {
  if (!allUnbindableSameWs.value) return null
  const ws = selectedRows.value[0]?.['workspace']
  if (!isRecord(ws)) return null
  const id = ws['id']
  return isNonEmptyString(id) ? id : null
})

// =========== Multi-select manage disable conditions ===========
const allUnManageableSameWs = computed(() => {
  if (selectedRows.value.length === 0) return false
  const ids = getSelectedClusterIds()
  // Every row must have clusterId, and all ids must be the same
  return ids.length === selectedRows.value.length && new Set(ids).size === 1
})

const unManageDis = computed(() => !allUnManageableSameWs.value)
// clusterId for unmanage operation
const unManageClusterId = computed(() => {
  if (!allUnManageableSameWs.value) return null
  const id = selectedRows.value[0]?.['clusterId']
  return isNonEmptyString(id) ? id : null
})

// Batch action bar placeholder
const hasSelection = computed(() => selectedRows.value.length > 0)
const hasBarSpace = ref(false)
watch(hasSelection, (v) => {
  if (v) hasBarSpace.value = true // Reserve space when selected
})
function onBarAfterLeave() {
  hasBarSpace.value = false
}
const openBindWl = async (action: string, isBatch: boolean, nodeId?: string, wsid?: string) => {
  bindAction.value = action
  selectedWsId.value =
    (isBatch ? unbindWorkspaceId.value : wsid) ?? wsStore.currentWorkspaceId ?? ''
  curNodeId.value = isBatch ? getSelectedNodeIds() : nodeId ? [nodeId] : []
  bindVisible.value = true
}
const openManage = async () => {
  curNodeId.value = getSelectedNodeIds()
  selectedClusterId.value = unManageClusterId.value ?? ''
  manageVisible.value = true
}

const fetchData = async (params?: NodesParams) => {
  try {
    loading.value = true
    const res = await getNodesList({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      clusterId: detailData.value?.clusterId,
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

  fetchData({
    ...(searchParams.available !== null ? { available: searchParams.available } : {}),
    ...(searchParams.isAddonsInstalled !== null
      ? { isAddonsInstalled: searchParams.isAddonsInstalled }
      : {}),
    ...(searchParams.nodeId ? { nodeId: searchParams.nodeId } : {}),
    ...(searchParams.phase ? { phase: searchParams.phase } : {}),
    ...(searchParams.workspaceId
      ? { workspaceId: searchParams.workspaceId === 'UNASSIGNED' ? '' : searchParams.workspaceId }
      : {}),
  })
}
const jumpToDetail = (id: string) => {
  router.push({ path: '/nodedetail', query: { id } })
}

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

const passAll = () => true
const handleFilterChange = (filters: Record<string, string[]>) => {
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

// Use Node Retry composable
const { handleRetry } = useNodeRetry({
  onRefresh: () => onSearch({ resetPage: false }),
})

// Batch retry
const handleRetryBatch = () => {
  handleRetry(true, undefined, getSelectedNodeIds())
}
</script>
<style lang="css" scoped>
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  margin-right: 10px;
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
</style>
