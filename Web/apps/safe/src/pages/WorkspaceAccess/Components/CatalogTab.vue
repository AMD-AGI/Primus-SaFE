<template>
  <div v-loading="loading" :element-loading-text="$loadingText">
    <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
      <el-text type="info">
        Workspaces that accept access requests. When a workspace is bound to a team you belong to,
        ask to join the team; its managers approve it.
      </el-text>
      <el-button :icon="Refresh" @click="load">Refresh</el-button>
    </div>
    <el-table :data="items" size="large" empty-text="No workspace accepts access requests">
      <el-table-column label="Workspace" min-width="240">
        <template #default="{ row }">
          <div class="font-500">{{ row.workspaceName || row.workspaceId }}</div>
          <div class="text-[13px] text-gray-400">{{ row.workspaceId }}</div>
          <div v-if="row.description" class="text-[13px] mt-1 whitespace-pre-line">
            {{ row.description }}
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Managers" min-width="200">
        <template #default="{ row }">
          <template v-if="row.managers?.length">
            <div v-for="m in row.managers" :key="m.id">
              <span>{{ m.name || m.id }}</span>
              <span v-if="m.email" class="ml-1 text-[13px] text-gray-400">{{ m.email }}</span>
            </div>
          </template>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="Teams" min-width="200">
        <template #default="{ row }">
          <template v-if="row.teams?.length">
            <div v-for="t in row.teams" :key="t.id">
              <el-link
                v-if="teamLink(t.id)"
                type="primary"
                :href="teamLink(t.id)"
                target="_blank"
                rel="noopener"
              >
                Join team {{ t.displayName || t.id }}
              </el-link>
              <span v-else>{{ t.displayName || t.id }}</span>
            </div>
          </template>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="Access" width="200" fixed="right">
        <template #default="{ row }">
          <el-tag v-if="row.myStatus === 'member'" type="success">You have access</el-tag>
          <div v-else-if="row.myStatus === 'pending'" class="flex items-center gap-2">
            <el-tag type="warning">Pending</el-tag>
            <router-link
              v-if="row.pendingRequestId"
              :to="approvalPath(row.pendingRequestId)"
              class="text-[var(--safe-primary)] text-[13px]"
            >
              View request
            </router-link>
          </div>
          <el-button v-else type="primary" plain @click="openDialog(row)">
            {{ row.teams?.length ? 'Request directly' : 'Request access' }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>

  <RequestDialog v-model:visible="dialogVisible" :workspace="selected" @submitted="load" />
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { getWorkspaceCatalog, type CatalogWorkspace } from '@/services'
import { useUserStore } from '@/stores/user'
import { teamUrl } from '@/utils/identityConsole'
import { approvalPath } from '@/utils/workspaceAccess'
import RequestDialog from './RequestDialog.vue'

const userStore = useUserStore()

const loading = ref(false)
const items = ref<CatalogWorkspace[]>([])
const dialogVisible = ref(false)
const selected = ref<CatalogWorkspace | null>(null)

// Link to the team page of the identity console, empty when no console address is set.
const teamLink = (id: string) => teamUrl(userStore.envs?.identityConsoleUrl, id)

// Loads the catalog of workspaces that accept access requests.
const load = async () => {
  loading.value = true
  try {
    const res = await getWorkspaceCatalog()
    items.value = res?.items ?? []
  } finally {
    loading.value = false
  }
}

// Opens the request dialog for a workspace.
const openDialog = (ws: CatalogWorkspace) => {
  selected.value = ws
  dialogVisible.value = true
}

onMounted(() => {
  void load()
})
</script>
