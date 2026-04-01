<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Deployment Management</el-text>
    <div class="flex items-center gap-3 mt-4">
      <el-button
        type="primary"
        round
        :icon="Plus"
        class="text-black"
        @click="
          () => {
            createAction = 'Create'
            createVisible = true
          }
        "
      >
        Create Deployment
      </el-button>
      <el-segmented
        v-model="currentType"
        :options="typeSegOptions"
        @change="handleTypeChange"
        class="myself-seg"
        style="background: none"
      />
    </div>
  </div>

  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 255px)'"
      :data="tableData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @filter-change="handleTableFilterChange"
    >
      <el-table-column prop="id" label="ID" width="80" fixed="left" align="center">
        <template #default="{ row }">
          <el-link type="primary" v-route="getDeployDetailRoute(row.id)">{{ row.id }}</el-link>
        </template>
      </el-table-column>

      <el-table-column
        prop="status"
        label="Status"
        width="150"
        column-key="status"
        :filters="statusFilters"
        :filter-multiple="true"
        :filtered-value="searchParams.status"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag :type="getStatusType(row.status)" :effect="isDark ? 'plain' : 'light'">{{
            row.status
          }}</el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="deploy_name" label="Deploy Name" width="200" show-overflow-tooltip />

      <el-table-column prop="description" label="Description" min-width="300">
        <template #default="{ row }">
          <el-text class="line-clamp-2">{{ row.description || '-' }}</el-text>
        </template>
      </el-table-column>

      <el-table-column prop="rollback_from_id" label="Rollback From" width="130" align="center">
        <template #default="{ row }">
          <el-link
            v-if="row.rollback_from_id"
            type="primary"
            v-route="getDeployDetailRoute(row.rollback_from_id)"
          >
            #{{ row.rollback_from_id }}
          </el-link>
          <el-text v-else type="info">-</el-text>
        </template>
      </el-table-column>

      <el-table-column prop="created_at" label="Created At" width="180">
        <template #default="{ row }">
          {{ formatTimeStr(row.created_at) }}
        </template>
      </el-table-column>

      <el-table-column prop="approver_name" label="Approver" width="150">
        <template #default="{ row }">
          {{ row.approver_name || '-' }}
        </template>
      </el-table-column>

      <el-table-column prop="approved_at" label="Approved At" width="180">
        <template #default="{ row }">
          {{ row.approved_at ? formatTimeStr(row.approved_at) : '-' }}
        </template>
      </el-table-column>

      <el-table-column prop="approval_result" label="Approval Result" width="140">
        <template #default="{ row }">
          <div v-if="row.approval_result === 'approved'" class="flex items-center gap-1">
            <el-icon color="#67c23a" :size="16"><CircleCheck /></el-icon>
            <el-text type="success">Approved</el-text>
          </div>
          <el-tooltip
            v-else-if="row.approval_result === 'rejected'"
            :content="row.rejection_reason || 'No reason provided'"
            placement="top"
            :disabled="!row.rejection_reason"
          >
            <div class="flex items-center gap-1 cursor-pointer">
              <el-icon color="#f56c6c" :size="16"><CircleClose /></el-icon>
              <el-text type="danger">Rejected</el-text>
            </div>
          </el-tooltip>
          <el-text v-else type="info">-</el-text>
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="130" fixed="right">
        <template #default="{ row }">
          <el-tooltip v-if="row.status === 'pending_approval' && userStore.cdRequireApproval && row.deploy_name === userStore.profile?.name" content="Share Approval Link" placement="top">
            <el-button circle size="default" class="btn-success-plain" :icon="Share" @click="handleShare(row)" />
          </el-tooltip>
          <el-tooltip v-else-if="row.status === 'pending_approval'" content="Approve/Reject" placement="top">
            <el-button circle size="default" class="btn-success-plain" :icon="Check" @click="handleApprove(row)" />
          </el-tooltip>
          <el-tooltip v-else-if="row.status === 'failed'" content="Retry" placement="top">
            <el-button circle size="default" class="btn-primary-plain" :icon="Refresh" @click="handleRetry(row)" />
          </el-tooltip>
          <el-tooltip v-else-if="row.status === 'deployed'" content="Rollback" placement="top">
            <el-button circle size="default" class="btn-warning-plain" :icon="RefreshLeft" @click="handleRollback(row)" />
          </el-tooltip>
          <el-button v-else circle size="default" :icon="Check" disabled />

          <el-tooltip content="Cancel" placement="top">
            <el-button circle size="default" class="btn-danger-plain" :icon="Close" :disabled="row.status !== 'pending_approval' || row.deploy_name !== userStore.profile?.name" @click="handleCancel(row)" />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="pagination.page"
      v-model:page-size="pagination.pageSize"
      :total="pagination.total"
      :page-sizes="[10, 20, 50, 100]"
      layout="total, sizes, prev, pager, next, jumper"
      class="mt-4"
      @current-change="fetchData"
      @size-change="fetchData"
    />
  </el-card>

  <CreateDialog
    v-model:visible="createVisible"
    :action="createAction"
    :rollback-data="rollbackData"
    :default-type="currentType"
    @success="handleCreateSuccess"
  />

  <ApprovalDialog
    v-model:visible="approvalVisible"
    :deployment-data="currentDeployment"
    @success="fetchData"
  />
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Check, Close, RefreshLeft, Refresh, CircleCheck, CircleClose, Share } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { getDeployments, rollbackDeployment, retryDeployment, approveDeployment } from '@/services/deploy'
import { formatTimeStr, copyText } from '@/utils'
import type { DeploymentRequest, DeploymentStatus, DeploymentType } from '@/services/deploy/type'
import CreateDialog from './Components/CreateDialog.vue'
import ApprovalDialog from './Components/ApprovalDialog.vue'
import { useUserStore } from '@/stores/user'
import { useDark } from '@vueuse/core'

const isDark = useDark()
const userStore = useUserStore()
const route = useRoute()
const router = useRouter()

const loading = ref(false)
const tableData = ref<DeploymentRequest[]>([])

const currentType = ref<DeploymentType>('safe')
const typeSegOptions = [
  { label: 'Safe', value: 'safe' },
  { label: 'Lens', value: 'lens' },
] as const

const searchParams = reactive<{
  status: DeploymentStatus[]
}>({
  status: [],
})

const statusFilters = [
  { text: 'Pending Approval', value: 'pending_approval' },
  { text: 'Approved', value: 'approved' },
  { text: 'Rejected', value: 'rejected' },
  { text: 'Deploying', value: 'deploying' },
  { text: 'Deployed', value: 'deployed' },
  { text: 'Failed', value: 'failed' },
]

const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})

const createVisible = ref(false)
const createAction = ref<'Create' | 'Rollback'>('Create')
const rollbackData = ref<DeploymentRequest | null>(null)

const approvalVisible = ref(false)
const currentDeployment = ref<DeploymentRequest | null>(null)

const getStatusType = (status: DeploymentStatus) => {
  const typeMap: Record<DeploymentStatus, string> = {
    pending_approval: 'warning',
    approved: 'info',
    rejected: 'danger',
    deploying: 'primary',
    deployed: 'success',
    failed: 'danger',
  }
  return typeMap[status] || ''
}

const passAll = () => {
  return true
}

const fetchData = async () => {
  try {
    loading.value = true
    const params: {
      offset: number
      limit: number
      sortBy: string
      order: 'asc' | 'desc'
      status?: DeploymentStatus
      type?: DeploymentType
    } = {
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      sortBy: 'created_at',
      order: 'desc',
    }

    if (searchParams.status.length > 0) {
      params.status = searchParams.status[0]
    }
    params.type = currentType.value

    const res = await getDeployments(params)
    tableData.value = res.items || []
    pagination.total = res.total_count || 0
  } catch (error) {
    console.error('Failed to fetch deployments:', error)
  } finally {
    loading.value = false
  }
}

const handleTableFilterChange = (filters: Record<string, DeploymentStatus[]>) => {
  if (filters.status) {
    searchParams.status = filters.status
  }
  pagination.page = 1
  fetchData()
}

const handleTypeChange = () => {
  pagination.page = 1
  // keep selected type in URL so returning from detail can restore it
  router.replace({ query: { ...route.query, type: currentType.value } })
  fetchData()
}

const handleCreateSuccess = (type?: DeploymentType) => {
  if (type) currentType.value = type
  pagination.page = 1
  router.replace({ query: { ...route.query, type: currentType.value } })
  fetchData()
}

const getDeployDetailRoute = (id: number) => ({
  path: '/deploy/detail',
  query: { id: String(id), type: currentType.value },
})

const handleViewDetail = (id: number) => {
  router.push({ path: '/deploy/detail', query: { id: String(id), type: currentType.value } })
}

const handleApprove = (row: DeploymentRequest) => {
  currentDeployment.value = row
  approvalVisible.value = true
}

const handleShare = (row: DeploymentRequest) => {
  const url = new URL(window.location.href)
  url.searchParams.set('approvalId', String(row.id))
  url.searchParams.set('type', String(row.deploy_type || currentType.value))
  copyText(url.toString())
}

const handleRollback = async (row: DeploymentRequest) => {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to rollback deployment #${row.id}? This will create a new deployment request.`,
      'Confirm Rollback',
      {
        confirmButtonText: 'Confirm',
        cancelButtonText: 'Cancel',
        type: 'warning',
      },
    )

    loading.value = true
    await rollbackDeployment(String(row.id))
    ElMessage.success('Rollback request created successfully')
    await fetchData()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Rollback failed:', error)
      ElMessage.error((error as Error)?.message || 'Rollback failed')
    }
  } finally {
    loading.value = false
  }
}

const handleCancel = async (row: DeploymentRequest) => {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to cancel deployment #${row.id}?`,
      'Confirm Cancel',
      {
        confirmButtonText: 'Confirm',
        cancelButtonText: 'Back',
        type: 'warning',
      },
    )

    loading.value = true
    await approveDeployment(String(row.id), {
      approved: false,
      reason: 'Cancelled by requester',
    })
    ElMessage.success('Deployment cancelled successfully')
    await fetchData()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Cancel failed:', error)
      ElMessage.error((error as Error)?.message || 'Cancel failed')
    }
  } finally {
    loading.value = false
  }
}

const handleRetry = async (row: DeploymentRequest) => {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to retry deployment #${row.id}?`,
      'Confirm Retry',
      {
        confirmButtonText: 'Confirm',
        cancelButtonText: 'Cancel',
        type: 'warning',
      },
    )

    loading.value = true
    await retryDeployment(String(row.id))
    ElMessage.success('Deployment retry initiated successfully')
    await fetchData()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Retry failed:', error)
      ElMessage.error((error as Error)?.message || 'Retry failed')
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  // Check if there's an approval ID in the query params
  const approvalId = route.query.approvalId
  const approvalType = route.query.type as DeploymentType | undefined
  if (approvalType === 'safe' || approvalType === 'lens') {
    currentType.value = approvalType
  }

  await fetchData()

  if (approvalId) {
    const deployment = tableData.value.find((item) => String(item.id) === String(approvalId))
    if (deployment && deployment.status === 'pending_approval') {
      currentDeployment.value = deployment
      approvalVisible.value = true

      // Clean up the URL after opening the dialog
      const url = new URL(window.location.href)
      url.searchParams.delete('approvalId')
      url.searchParams.delete('type')
      window.history.replaceState({}, '', url.toString())
    } else if (deployment) {
      ElMessage.warning('This deployment is no longer pending approval')
    } else {
      ElMessage.warning('Deployment not found')
    }
  }
})

defineOptions({
  name: 'DeployPage',
})
</script>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>

<style>
/* Reuse project-wide segmented component styles */
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}
</style>
