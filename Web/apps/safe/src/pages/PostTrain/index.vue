<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <el-text class="block textx-18 font-500" tag="b">PostTrain</el-text>

  <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        @click="showCreateDialog = true"
        class="text-black"
      >
        Create Training
      </el-button>
    </div>

    <!-- Filters -->
    <div class="flex flex-wrap items-center gap-2">
      <el-select v-model="filters.trainType" placeholder="Train Type" clearable style="width: 110px" @change="onSearch({ resetPage: true })">
        <el-option label="All" value="" />
        <el-option label="SFT" value="sft" />
        <el-option label="RL" value="rl" />
      </el-select>
      <el-select v-model="filters.strategy" placeholder="Strategy" clearable style="width: 130px" @change="onSearch({ resetPage: true })">
        <el-option label="All" value="" />
        <el-option label="full" value="full" />
        <el-option label="lora" value="lora" />
        <el-option label="fsdp2" value="fsdp2" />
        <el-option label="megatron" value="megatron" />
      </el-select>
      <el-select v-model="filters.status" placeholder="Status" clearable style="width: 120px" @change="onSearch({ resetPage: true })">
        <el-option label="All" value="" />
        <el-option label="Pending" value="Pending" />
        <el-option label="Running" value="Running" />
        <el-option label="Succeeded" value="Succeeded" />
        <el-option label="Failed" value="Failed" />
        <el-option label="Stopped" value="Stopped" />
      </el-select>
      <el-select v-model="filters.workspace" placeholder="Workspace" clearable style="width: 140px" @change="onSearch({ resetPage: true })">
        <el-option v-for="ws in workspaceItems" :key="ws.workspaceId" :label="ws.workspaceName" :value="ws.workspaceId" />
      </el-select>
      <el-input v-model="filters.baseModel" placeholder="Base Model" clearable style="width: 140px" @keyup.enter="onSearch({ resetPage: true })" @clear="onSearch({ resetPage: true })" />
      <el-input v-model="filters.owner" placeholder="Owner" clearable style="width: 120px" @keyup.enter="onSearch({ resetPage: true })" @clear="onSearch({ resetPage: true })" />
      <el-date-picker
        v-model="filters.dateRange"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start"
        end-placeholder="End"
        clearable
        @change="onSearch({ resetPage: true })"
      />
      <el-button :icon="Search" type="primary" @click="onSearch({ resetPage: true })" />
      <el-tooltip content="Reset" placement="top">
        <el-button :icon="RefreshIcon" @click="resetFilters" />
      </el-tooltip>
      <el-tooltip content="Refresh" placement="top">
        <el-button :icon="Refresh" @click="onSearch({ resetPage: false })" />
      </el-tooltip>
    </div>
  </div>

  <!-- Table -->
  <el-card class="mt-4 safe-card" shadow="never">
    <el-table
      :data="tableData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @row-click="openDrawer"
      row-class-name="cursor-pointer"
    >
      <el-table-column prop="displayName" label="Run Name / ID" min-width="200" :fixed="true">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" @click.stop="openDrawer(row)">{{ row.displayName }}</el-link>
            <span class="text-[13px] text-gray-400">{{ row.runId }}</span>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="trainType" label="Type" width="80">
        <template #default="{ row }">
          <el-tag size="small" :type="row.trainType === 'sft' ? 'success' : 'warning'" effect="plain">
            {{ row.trainType?.toUpperCase() }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="strategy" label="Strategy" width="110">
        <template #default="{ row }">{{ row.strategy || '-' }}</template>
      </el-table-column>

      <el-table-column prop="baseModelName" label="Base Model" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ row.baseModelName || '-' }}</template>
      </el-table-column>

      <el-table-column prop="datasetName" label="Dataset" min-width="130" show-overflow-tooltip>
        <template #default="{ row }">{{ row.datasetName || '-' }}</template>
      </el-table-column>

      <el-table-column prop="workspace" label="Workspace" min-width="120" show-overflow-tooltip />

      <el-table-column prop="status" label="Status" width="120">
        <template #default="{ row }">
          <el-tag size="small" :type="statusTagType(row.status)" :effect="isDark ? 'plain' : 'light'">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column label="Nodes x GPUs" width="120">
        <template #default="{ row }">
          {{ row.nodeCount ?? '-' }} x {{ row.gpuPerNode ?? '-' }}
        </template>
      </el-table-column>

      <el-table-column prop="parameterSummary" label="Key Params" min-width="240" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="font-mono text-xs">{{ row.parameterSummary || '-' }}</span>
        </template>
      </el-table-column>

      <el-table-column prop="latestLoss" label="Latest Loss" width="110">
        <template #default="{ row }">
          {{ row.latestLoss != null ? Number(row.latestLoss).toFixed(4) : '-' }}
        </template>
      </el-table-column>

      <el-table-column label="Output" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <template v-if="row.modelId">
            <el-link type="primary" @click.stop="goModel(row.modelId)">
              {{ row.modelDisplayName || row.modelId }}
              <el-tag v-if="row.modelPhase" size="small" class="ml-1" effect="plain">{{ row.modelPhase }}</el-tag>
            </el-link>
          </template>
          <span v-else>{{ row.outputPath ? 'Exported' : '-' }}</span>
        </template>
      </el-table-column>

      <el-table-column prop="createdAt" label="Created" width="170">
        <template #default="{ row }">{{ formatTimeStr(row.createdAt) }}</template>
      </el-table-column>

      <el-table-column prop="duration" label="Duration" width="110">
        <template #default="{ row }">{{ row.duration || '-' }}</template>
      </el-table-column>

      <el-table-column prop="userName" label="Owner" width="100" show-overflow-tooltip>
        <template #default="{ row }">{{ row.userName || '-' }}</template>
      </el-table-column>

      <el-table-column label="Actions" width="200" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Detail" placement="top">
            <el-button circle size="small" class="btn-primary-plain" :icon="View" @click.stop="openDrawer(row)" />
          </el-tooltip>
          <el-tooltip content="Logs" placement="top">
            <el-button circle size="small" class="btn-primary-plain" :icon="Document" @click.stop="goLogs(row)" />
          </el-tooltip>
          <el-tooltip content="Workload" placement="top">
            <el-button circle size="small" class="btn-primary-plain" :icon="Monitor" @click.stop="goWorkload(row)" />
          </el-tooltip>
          <el-tooltip v-if="row.modelId" content="Model" placement="top">
            <el-button circle size="small" class="btn-success-plain" :icon="Goods" @click.stop="goModel(row.modelId)" />
          </el-tooltip>
          <el-tooltip content="Delete Record" placement="top">
            <el-button circle size="small" class="btn-danger-plain" :icon="Delete" @click.stop="handleDelete(row)" />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="m-t-2"
      :current-page="pagination.page"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      @current-change="(p: number) => { pagination.page = p; onSearch({ resetPage: false }) }"
      @size-change="(s: number) => { pagination.pageSize = s; pagination.page = 1; onSearch({ resetPage: false }) }"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[20, 50, 100]"
    />
  </el-card>

  <!-- Detail Drawer -->
  <RunDetailDrawer
    v-model:visible="drawerVisible"
    :run-id="drawerRunId"
  />

  <!-- Create Training Dialog (reuse from ModelSquare) -->
  <CreateTrainingDialog
    v-model:visible="showCreateDialog"
    :model="null"
    @success="handleCreateSuccess"
  />
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, h } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, Refresh, View, Delete, Document, Monitor, Goods } from '@element-plus/icons-vue'
import RefreshIcon from '@/components/icons/ResetIcon.vue'
import { useDark } from '@vueuse/core'
import { formatTimeStr } from '@/utils'
import { getPostTrainRuns, deletePostTrainRun } from '@/services/posttrain'
import type { PostTrainRunItem, PostTrainListParams } from '@/services/posttrain'
import { useWorkspaceStore } from '@/stores/workspace'
import RunDetailDrawer from './Components/RunDetailDrawer.vue'
import CreateTrainingDialog from '@/pages/ModelSquare/Components/CreateTrainingDialog.vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'

dayjs.extend(utc)

const router = useRouter()
const isDark = useDark()
const wsStore = useWorkspaceStore()

const loading = ref(false)
const tableData = ref<PostTrainRunItem[]>([])
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const drawerVisible = ref(false)
const drawerRunId = ref('')
const showCreateDialog = ref(false)

const workspaceItems = wsStore.items ?? []

const filters = reactive({
  trainType: '',
  strategy: '',
  status: '',
  workspace: '',
  baseModel: '',
  owner: '',
  dateRange: '' as any,
})

const statusTagType = (status: string) => {
  const map: Record<string, string> = {
    Running: 'primary', Succeeded: 'success', Failed: 'danger', Pending: 'info', Stopped: 'info',
  }
  return map[status] || 'info'
}

const fetchData = async () => {
  loading.value = true
  try {
    const [start, end] = filters.dateRange ?? []
    const params: PostTrainListParams = {
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
    }
    if (filters.trainType) params.trainType = filters.trainType
    if (filters.strategy) params.strategy = filters.strategy
    if (filters.status) params.status = filters.status
    if (filters.workspace) params.workspace = filters.workspace
    if (filters.baseModel) params.baseModel = filters.baseModel
    if (filters.owner) params.owner = filters.owner
    if (start) params.since = dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]')
    if (end) params.until = dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]')

    const res = (await getPostTrainRuns(params)) as unknown as { total: number; items: PostTrainRunItem[] }
    tableData.value = res.items || []
    pagination.total = res.total || 0
  } catch {
    ElMessage.error('Failed to load training runs')
  } finally {
    loading.value = false
  }
}

const onSearch = (opts?: { resetPage?: boolean }) => {
  if (opts?.resetPage) pagination.page = 1
  fetchData()
}

const resetFilters = () => {
  Object.assign(filters, { trainType: '', strategy: '', status: '', workspace: '', baseModel: '', owner: '', dateRange: '' })
  pagination.page = 1
  fetchData()
}

const openDrawer = (row: PostTrainRunItem) => {
  drawerRunId.value = row.runId
  drawerVisible.value = true
}

const goLogs = (row: PostTrainRunItem) => {
  router.push({ path: '/training/detail', query: { id: row.workloadId, tab: 'logs' } })
}

const goWorkload = (row: PostTrainRunItem) => {
  router.push({ path: '/training/detail', query: { id: row.workloadId } })
}

const goModel = (modelId: string) => {
  router.push(`/model-square/detail/${modelId}`)
}

const handleDelete = async (row: PostTrainRunItem) => {
  const msg = h('span', null, [
    'Delete training record ',
    h('b', null, row.displayName),
    '? This only removes the record, not the actual workload.',
  ])
  try {
    await ElMessageBox.confirm(msg, 'Delete Record', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await deletePostTrainRun(row.runId)
    ElMessage.success('Record deleted')
    onSearch({ resetPage: false })
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error('Failed to delete record')
    }
  }
}

const handleCreateSuccess = (workloadId: string) => {
  showCreateDialog.value = false
  router.push({ path: '/training/detail', query: { id: workloadId } })
}

defineOptions({ name: 'PostTrainPage' })

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.font-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.cursor-pointer {
  cursor: pointer;
}
</style>
