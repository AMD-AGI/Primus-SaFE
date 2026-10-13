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
      <el-table-column prop="displayName" label="Name" min-width="200">
        <template #default="{ row }">
          <el-link type="primary" @click="jumpToDetail(row.workloadId)">{{ row.displayName }}</el-link>
          <div class="text-xs text-gray-400">{{ row.workloadId }}</div>
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
          <el-button text type="primary" :disabled="!canWrite" @click="openEdit(row.workloadId)">Edit</el-button>
          <el-button text type="warning" :disabled="!canWrite" @click="onStop(row)">Stop</el-button>
          <el-button text type="danger" :disabled="!canWrite" @click="onDelete(row)">Delete</el-button>
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
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { deleteWorkload, getWorkloadsList, stopWorkload } from '@/services'
import { WorkloadKind, WorkloadPhaseButtonType, phaseFilters, type WorkloadParams, type WorkloadPhase } from '@/services/workload/type'
import { formatTimeStr } from '@/utils/index'
import { useWorkspaceStore } from '@/stores/workspace'
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'
import AddDialog from './Components/AddDialog.vue'

const router = useRouter()
const store = useWorkspaceStore()
const { canWrite } = useWorkloadWriteGuard()
const loading = ref(false)
const tableData = ref<any[]>([])
const keyword = ref('')
const phase = ref<WorkloadPhase[]>([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const addVisible = ref(false)
const curWlId = ref('')
const curAction = ref<'Create' | 'Edit'>('Create')
const tableHeight = computed(() => 'calc(100vh / var(--zoom) - 280px)')

function roleReplicas(row: any) {
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

function openEdit(id: string) {
  curWlId.value = id
  curAction.value = 'Edit'
  addVisible.value = true
}

function onStop(row: any) {
  ElMessageBox.confirm(`Stop ${row.displayName || row.workloadId}?`, 'Stop', { type: 'warning' })
    .then(async () => {
      await stopWorkload(row.workloadId)
      ElMessage.success('Stop requested')
      fetchData()
    })
    .catch(() => undefined)
}

function onDelete(row: any) {
  ElMessageBox.confirm(`Delete ${row.displayName || row.workloadId}?`, 'Delete', { type: 'warning' })
    .then(async () => {
      await deleteWorkload(row.workloadId)
      ElMessage.success('Delete requested')
      fetchData()
    })
    .catch(() => undefined)
}

watch(
  () => store.currentWorkspaceId,
  () => onSearch(true),
)

onMounted(fetchData)
</script>
