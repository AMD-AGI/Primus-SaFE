<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Pending approvals</el-text>
    <el-text type="info" class="block mt-1">
      Workspace access requests you can approve as a manager of the workspace or a system
      administrator.
    </el-text>
    <el-card
      v-loading="loading"
      :element-loading-text="$loadingText"
      class="safe-card mt-4"
      shadow="never"
    >
      <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
        <el-link
          v-if="teamApprovalsLink"
          type="primary"
          :href="teamApprovalsLink"
          target="_blank"
          rel="noopener"
        >
          Team join requests to approve
        </el-link>
        <span v-else />
        <el-button :icon="Refresh" @click="load">Refresh</el-button>
      </div>
      <el-table :data="items" size="large" empty-text="No requests waiting for you">
        <el-table-column label="Requester" min-width="200">
          <template #default="{ row }">
            <div>{{ row.userName || row.userId }}</div>
            <div v-if="row.userEmail" class="text-[13px] text-gray-400">{{ row.userEmail }}</div>
          </template>
        </el-table-column>
        <el-table-column label="Workspace" min-width="180">
          <template #default="{ row }">{{ row.workspaceName || row.workspace }}</template>
        </el-table-column>
        <el-table-column label="Reason" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">{{ row.reason || '-' }}</template>
        </el-table-column>
        <el-table-column label="Creation Time" width="180">
          <template #default="{ row }">{{ formatTimeStr(row.creationTime) }}</template>
        </el-table-column>
        <el-table-column label="Actions" width="120" fixed="right">
          <template #default="{ row }">
            <router-link :to="approvalPath(row.id)" class="text-[var(--safe-primary)]">
              Review
            </router-link>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { listWorkspaceAccessRequests, type WorkspaceAccessRequest } from '@/services'
import { useUserStore } from '@/stores/user'
import { formatTimeStr } from '@/utils'
import { teamApprovalsUrl } from '@/utils/identityConsole'
import { approvalPath } from '@/utils/workspaceAccess'

const userStore = useUserStore()

const loading = ref(false)
const items = ref<WorkspaceAccessRequest[]>([])

// Team join requests the caller may decide in the identity console, empty when no console address is set.
const teamApprovalsLink = computed(() => teamApprovalsUrl(userStore.envs?.identityConsoleUrl))

// Loads the pending requests the caller may decide.
const load = async () => {
  loading.value = true
  try {
    const res = await listWorkspaceAccessRequests('approver')
    items.value = res?.items ?? []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})

defineOptions({
  name: 'WorkspaceAccessApprovalsPage',
})
</script>
