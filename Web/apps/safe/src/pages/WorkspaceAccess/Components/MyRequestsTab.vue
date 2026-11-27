<template>
  <div v-loading="loading" :element-loading-text="$loadingText">
    <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
      <el-link
        v-if="teamRequestsLink"
        type="primary"
        :href="teamRequestsLink"
        target="_blank"
        rel="noopener"
      >
        My team join requests
      </el-link>
      <span v-else />
      <el-button :icon="Refresh" @click="load">Refresh</el-button>
    </div>
    <el-table :data="items" size="large" empty-text="No requests">
      <el-table-column label="Workspace" min-width="200">
        <template #default="{ row }">
          <router-link :to="approvalPath(row.id)" class="text-[var(--safe-primary)]">
            {{ row.workspaceName || row.workspace }}
          </router-link>
        </template>
      </el-table-column>
      <el-table-column label="Status" width="130">
        <template #default="{ row }">
          <el-tag :type="requestStatusTag(row.status)">{{ requestStatusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Reason" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">{{ row.reason || '-' }}</template>
      </el-table-column>
      <el-table-column label="Decision comment" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">
          <span>{{ row.decisionComment || '-' }}</span>
          <span v-if="row.decidedByName || row.decidedBy" class="ml-1 text-[13px] text-gray-400">
            by {{ row.decidedByName || row.decidedBy }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="Creation Time" width="180">
        <template #default="{ row }">{{ formatTimeStr(row.creationTime) }}</template>
      </el-table-column>
      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Copy approval link" placement="top">
            <el-button circle size="default" :icon="CopyDocument" @click="copyLink(row)" />
          </el-tooltip>
          <el-tooltip v-if="isPendingRequest(row.status)" content="Cancel" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Close"
              @click="onCancel(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, CopyDocument, Refresh } from '@element-plus/icons-vue'
import {
  cancelWorkspaceAccessRequest,
  listWorkspaceAccessRequests,
  type WorkspaceAccessRequest,
} from '@/services'
import { useUserStore } from '@/stores/user'
import { copyText, formatTimeStr } from '@/utils'
import { teamRequestsUrl } from '@/utils/identityConsole'
import {
  approvalLink,
  approvalPath,
  isPendingRequest,
  requestStatusLabel,
  requestStatusTag,
} from '@/utils/workspaceAccess'

const userStore = useUserStore()

const loading = ref(false)
const items = ref<WorkspaceAccessRequest[]>([])

// The caller's team join requests in the identity console, empty when no console address is set.
const teamRequestsLink = computed(() => teamRequestsUrl(userStore.envs?.identityConsoleUrl))

// Loads the caller's own requests.
const load = async () => {
  loading.value = true
  try {
    const res = await listWorkspaceAccessRequests('mine')
    items.value = res?.items ?? []
  } finally {
    loading.value = false
  }
}

// Copies the approval link of a request.
const copyLink = (req: WorkspaceAccessRequest) => copyText(approvalLink(req, location.origin))

// Withdraws a pending request after confirmation.
const onCancel = async (req: WorkspaceAccessRequest) => {
  const ok = await ElMessageBox.confirm(
    `Cancel your request for workspace ${req.workspaceName || req.workspace}?`,
    'Cancel request',
    { confirmButtonText: 'Cancel request', cancelButtonText: 'Keep', type: 'warning' },
  ).then(
    () => true,
    () => false,
  )
  if (!ok) return
  try {
    await cancelWorkspaceAccessRequest(req.id)
    ElMessage.success('Request cancelled.')
  } catch (err) {
    console.error(err)
  }
  await load()
}

onMounted(() => {
  void load()
})
</script>
