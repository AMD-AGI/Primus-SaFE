<template>
  <div v-if="detailData" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
          Back
        </el-button>
        <h1 class="w-name">{{ detailData.workspaceName }}</h1>

        <el-tag class="ml-2" :type="detailData.phase === 'Running' ? 'success' : 'danger'">{{
          detailData.phase
        }}</el-tag>
      </div>
    </div>

    <div class="w-meta">
      <span class="item">
        <span class="label">ID</span>
        <code class="code">{{ detailData.workspaceId }}</code>
        <el-icon
          class="copy"
          size="12"
          style="color: var(--safe-primary)"
          @click="copyText(detailData.workspaceId)"
        >
          <CopyDocument />
        </el-icon>
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
      <el-descriptions-item label="nodeFlavor" :span="2">{{
        detailData.flavorId
      }}</el-descriptions-item>
      <el-descriptions-item label="nodes (avail / used / total)"
        >{{
          detailData.currentNodeCount
            ? detailData.currentNodeCount - detailData.usedNodeCount - detailData.abnormalNodeCount
            : '0'
        }}
        / {{ detailData.usedNodeCount ?? '0' }} /
        {{ detailData.currentNodeCount ?? '0' }}</el-descriptions-item
      >
      <el-descriptions-item label="creation">
        {{
          detailData.creationTime
            ? dayjs(detailData.creationTime).format('YYYY-MM-DD HH:mm:ss')
            : '-'
        }}
      </el-descriptions-item>
      <el-descriptions-item v-if="detailData?.description" label="description" :span="2">{{
        detailData.description
      }}</el-descriptions-item>
      <el-descriptions-item label="scopes" :span="2">{{
        detailData.scopes?.join(',')
      }}</el-descriptions-item>
      <el-descriptions-item label="queuePolicy">{{ detailData.queuePolicy }}</el-descriptions-item>
      <el-descriptions-item label="imageSecret">
        <div style="white-space: pre-line">
          {{ detailData.imageSecretIds?.join('\n') }}
        </div>
      </el-descriptions-item>
      <el-descriptions-item label="enablePreempt">{{
        detailData.enablePreempt
      }}</el-descriptions-item>

      <el-descriptions-item :span="3">
        <template #label>
          <div>
            <div class="font-bold">quota</div>
            <div class="font-bold mt-1">(abnormal / used / avail)</div>
          </div>
        </template>
        <div class="space-y-1">
          <div v-for="r in quotaRows" :key="r.key">
            <span class="mono">{{ r.label }}:</span>
            <span>{{ r.abnormal }} / {{ r.used }} / {{ r.avail }}</span>
          </div>
        </div>
      </el-descriptions-item>

      <el-descriptions-item label="volumes" :span="3">
        <template v-if="detailData.volumes?.length">
          <el-space style="width: 100%; align-items: start">
            <el-card
              v-for="(v, i) in detailData.volumes"
              :key="v.uid ?? i"
              body-style="padding:8px 12px"
            >
              <div class="flex items-center gap-2">
                <el-tag size="small">{{ i + 1 }}</el-tag>
                <el-tag size="small" type="info">{{ v.type?.toUpperCase() }}</el-tag>
              </div>

              <div class="ml-2 text-sm">
                <div>mountPath: {{ v.mountPath }}</div>
                <template v-if="v.type === 'hostpath'">
                  <div>hostPath: {{ v.hostPath }}</div>
                </template>
                <template v-else>
                  <div>capacity: {{ v.capacity }}</div>
                  <div>storageClass: {{ v.storageClass }}</div>
                  <div>accessMode: {{ v.accessMode }}</div>
                  <div>subPath: {{ v.subPath || '—' }}</div>
                </template>
              </div>
            </el-card>
          </el-space>
        </template>
        <template v-else>
          <el-tag type="info" effect="plain">No volumes</el-tag>
        </template>
      </el-descriptions-item>
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
            <el-link type="primary" v-route="{ path: '/nodedetail', query: { id: row.nodeId } }">{{ row.nodeName }}</el-link>
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
        prop="clusterId"
        label="Cluster"
        width="120"
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
          <AvailableStatusTag :available="!!row.available" :display="row.available" :message="row.message" />
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
          <el-button type="warning" :disabled="unBindDis" plain @click="openBindWl"
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
    action="remove"
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
import { getWorkspaceDetail, getNodesList, type NodesParams, NODE_PHASE } from '@/services'
import { onMounted, ref, computed, reactive, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import dayjs from 'dayjs'
import { RESOURCE_KEYS } from './index'
import { CopyDocument, ArrowLeft, Loading } from '@element-plus/icons-vue'
import { useClusterStore } from '@/stores/cluster'
import { copyText, byte2Gi, fmtVal } from '@/utils/index'
import AvailableStatusTag from '@/components/Base/AvailableStatusTag.vue'
import BindDialog from '@/pages/Nodes/Components/BindDialog.vue'
import ManageDialog from '@/pages/Nodes/Components/ManageDialog.vue'
import { useNodeRetry } from '@/composables/useNodeRetry'

const route = useRoute()
const store = useClusterStore()
const wsId = computed(() => route.query.id as string | undefined)

const detailData = ref()
const detailLoading = ref(false)

const quotaRows = computed(() => {
  return RESOURCE_KEYS.map(({ label, key }) => ({
    key,
    label,
    abnormal: fmtVal(detailData.value.abnormalQuota?.[key], key),
    used: fmtVal(detailData.value.usedQuota?.[key], key),
    avail: fmtVal(detailData.value.availQuota?.[key], key),
  }))
})

const getDetail = async () => {
  if (!wsId.value) return
  detailLoading.value = true
  detailData.value = await getWorkspaceDetail(wsId.value)
  detailLoading.value = false
}

onMounted(async () => {
  await getDetail()
  await onSearch({ resetPage: true })
})

// ==================== Node list section =====================

// nodes table initial val
const router = useRouter()
const loading = ref(false)
const tableData = ref([])
const bindVisible = ref(false)
const manageVisible = ref(false)
const selectedWsId = ref('')
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
// Multi-select
const selectedRows = ref<Array<Record<string, any>>>([])
function onSelectionChange(rows: Array<Record<string, any>>) {
  selectedRows.value = rows
}

// =========== Multi-select bind disable conditions ===========
const allUnbindableSameWs = computed(() => {
  if (selectedRows.value.length === 0) return false
  const ids = selectedRows.value.map((r) => r?.workspace?.id).filter(Boolean)
  // Every row must have workspace.id, and all ids must be the same
  return ids.length === selectedRows.value.length && new Set(ids).size === 1
})
const unBindDis = computed(() => !allUnbindableSameWs.value)
// workspaceId for unbind operation
const unbindWorkspaceId = computed(() => {
  if (!allUnbindableSameWs.value) return null
  return selectedRows.value[0]?.workspace?.id ?? null
})

// =========== Multi-select manage disable conditions ===========
const allUnManageableSameWs = computed(() => {
  if (selectedRows.value.length === 0) return false
  const ids = selectedRows.value.map((r) => r?.clusterId).filter(Boolean)
  // Every row must have clusterId, and all ids must be the same
  return ids.length === selectedRows.value.length && new Set(ids).size === 1
})
const unManageDis = computed(() => !allUnManageableSameWs.value)
// clusterId for unmanage operation
const unManageClusterId = computed(() => {
  if (!allUnManageableSameWs.value) return null
  return selectedRows.value[0]?.clusterId ?? null
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
const openBindWl = async () => {
  selectedWsId.value = unbindWorkspaceId.value
  curNodeId.value = selectedRows.value.map((r) => r.nodeId).filter(Boolean)
  bindVisible.value = true
}

const openManage = async () => {
  curNodeId.value = selectedRows.value.map((r) => r.nodeId).filter(Boolean)
  selectedClusterId.value = unManageClusterId.value
  manageVisible.value = true
}
const fetchData = async (params?: NodesParams) => {
  try {
    loading.value = true

    const res = await getNodesList({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      workspaceId: detailData.value?.workspaceId,
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
    ...(searchParams.clusterId
      ? { clusterId: searchParams.clusterId === 'UNASSIGNED' ? '' : searchParams.clusterId }
      : {}),
  })
}
const jumpToDetail = (id: string) => {
  router.push({ path: '/nodedetail', query: { id } })
}

// Sorting
const tblRef = ref()
const n = (v: unknown) => (Number.isFinite(Number(v)) ? Number(v) : 0)
const avail = (row: any, key: string) => n(row?.availResources?.[key])
// Available and addons filter
const boolFilters = computed(() => [
  { text: 'true', value: 'true' },
  { text: 'false', value: 'false' },
])
// statusFilter
const phaseSelectedIds = ref<string[]>([])
type NodePhase = (typeof NODE_PHASE)[number]
const phaseFilters = computed(() => NODE_PHASE.map((p) => ({ text: p, value: p as NodePhase })))

// Cluster filter options
const filterClusterIds = ref<string[]>([])
const clusterFilters = computed(() => [
  { text: 'Unassigned', value: 'UNASSIGNED' },
  ...(store.items || []).map((ws) => ({
    text: ws.clusterId,
    value: ws.clusterId,
  })),
])

const passAll = () => true
const handleFilterChange = (filters: Record<string, string[]>) => {
  if ('clusterFilter' in filters) {
    searchParams.clusterId = filters.clusterFilter?.[0]
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
  const nodeIds = selectedRows.value.map((r) => r.nodeId).filter(Boolean)
  handleRetry(true, undefined, nodeIds)
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
