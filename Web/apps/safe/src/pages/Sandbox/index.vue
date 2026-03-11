<template>
  <el-text class="block textx-18 font-500" tag="b">Sandbox</el-text>
  <div class="flex flex-wrap items-center gap-2 mt-4">
    <el-segmented
      v-model="activeTab"
      :options="tabSegOptions"
      class="myself-seg"
      style="background: none"
    />

    <!-- Templates tab search -->
    <div v-if="activeTab === 'templates'" class="flex flex-wrap items-center ml-auto">
      <el-input
        v-model="templateSearch"
        placeholder="Search by name"
        clearable
        style="width: 300px"
        size="default"
        :prefix-icon="Search"
        @input="handleTemplateSearchDebounced"
      />
    </div>

    <!-- Sandboxes tab search -->
    <div v-if="activeTab === 'sandboxes'" class="flex flex-wrap items-center ml-auto">
      <el-input
        v-model="sessionSearch"
        placeholder="Search by session ID or sandbox name"
        clearable
        style="width: 300px"
        size="default"
        :prefix-icon="Search"
        @input="handleSessionSearchDebounced"
      />
    </div>
  </div>

  <!-- Templates list -->
  <el-card v-show="activeTab === 'templates'" class="mt-4 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 240px)'"
      :data="templateState.rowData"
      size="large"
      v-loading="templateLoading"
      :element-loading-text="$loadingText"
    >
      <el-table-column label="Name" prop="metadata.name" min-width="180" show-overflow-tooltip fixed="left" />
      <el-table-column label="Namespace" prop="metadata.namespace" min-width="120" />
      <el-table-column label="Image" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.spec?.template?.fromImage || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Resources" min-width="180">
        <template #default="{ row }">
          <span v-if="row.spec?.template?.resources?.limits">
            CPU: {{ row.spec.template.resources.limits.cpu || '-' }},
            Mem: {{ row.spec.template.resources.limits.memory || '-' }}
          </span>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="GPU" min-width="140">
        <template #default="{ row }">
          <span v-if="row.spec?.gpu">
            {{ row.spec.gpu.product }} x{{ row.spec.gpu.count }}
          </span>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="Warm Pool" prop="spec.warmPoolSize" width="110" />
      <el-table-column label="Session Timeout" min-width="130">
        <template #default="{ row }">
          {{ row.spec?.sessionTimeout || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Ready" width="90">
        <template #default="{ row }">
          <el-tag :type="row.status?.ready ? 'success' : 'danger'" size="small">
            {{ row.status?.ready ? 'Yes' : 'No' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Creator" min-width="140" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.metadata?.annotations?.['runtime.agent-sandbox.io/user.name'] || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Created" min-width="170">
        <template #default="{ row }">
          {{ formatTimeStr(row.metadata?.creationTimestamp) }}
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="mt-4"
      v-model:current-page="templateState.page"
      v-model:page-size="templateState.pageSize"
      :page-sizes="[10, 20, 50, 100]"
      layout="total, sizes, prev, pager, next"
      :total="templateState.total"
      @size-change="fetchTemplates"
      @current-change="fetchTemplates"
    />
  </el-card>

  <!-- Sandboxes list -->
  <el-card v-show="activeTab === 'sandboxes'" class="mt-4 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 240px)'"
      :data="sessionState.rowData"
      size="large"
      v-loading="sessionLoading"
      :element-loading-text="$loadingText"
    >
      <el-table-column
        label="Session ID"
        prop="sessionId"
        min-width="280"
        show-overflow-tooltip
        fixed="left"
      />
      <el-table-column label="Sandbox Name" prop="sandboxName" min-width="220" show-overflow-tooltip />
      <el-table-column label="Namespace" prop="namespace" min-width="120" />
      <el-table-column label="Status" width="110">
        <template #default="{ row }">
          <el-tag :type="row.status === 'running' ? 'success' : 'info'" size="small">
            {{ row.status || 'Unknown' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="User" prop="userName" min-width="140" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.userName || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Created" min-width="170">
        <template #default="{ row }">
          {{ formatTimeStr(row.createdAt) }}
        </template>
      </el-table-column>
      <el-table-column label="Last Activity" min-width="170">
        <template #default="{ row }">
          {{ formatTimeStr(row.lastActivity) }}
        </template>
      </el-table-column>
      <el-table-column label="Expires" min-width="170">
        <template #default="{ row }">
          {{ formatTimeStr(row.expiresAt) }}
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="mt-4"
      v-model:current-page="sessionState.page"
      v-model:page-size="sessionState.pageSize"
      :page-sizes="[10, 20, 50, 100]"
      layout="total, sizes, prev, pager, next"
      :total="sessionState.total"
      @size-change="fetchSessions"
      @current-change="fetchSessions"
    />
  </el-card>
</template>

<script lang="ts" setup>
import { ref, reactive, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { useDebounceFn } from '@vueuse/core'
import { getSandboxTemplates, getSandboxSessions } from '@/services/sandbox'
import { formatTimeStr } from '@/utils'
import type { SandboxTemplate, SandboxSession } from '@/services/sandbox/type'

const route = useRoute()
const router = useRouter()

const activeTab = ref((route.query.tab as string) || 'templates')

watch(activeTab, (newTab) => {
  router.replace({ query: { ...route.query, tab: newTab } })
  if (newTab === 'sandboxes' && sessionState.rowData.length === 0) {
    fetchSessions()
  }
})

const tabSegOptions = [
  { label: 'Templates', value: 'templates' },
  { label: 'Sandboxes', value: 'sandboxes' },
] as const

// ========== Templates ==========
const templateLoading = ref(false)
const templateSearch = ref('')
const templateState = reactive({
  rowData: [] as SandboxTemplate[],
  page: 1,
  pageSize: 20,
  total: 0,
})

const fetchTemplates = async () => {
  templateLoading.value = true
  try {
    const params: Record<string, any> = {
      offset: (templateState.page - 1) * templateState.pageSize,
      limit: templateState.pageSize,
      sortBy: 'createdAt',
      order: 'desc',
    }
    if (templateSearch.value) {
      params.name = templateSearch.value
    }
    const res = await getSandboxTemplates(params)
    templateState.rowData = res.items || []
    templateState.total = res.totalCount || 0
  } catch (e) {
    ElMessage.error((e as string) || 'Failed to fetch templates')
  } finally {
    templateLoading.value = false
  }
}

const handleTemplateSearchDebounced = useDebounceFn(() => {
  templateState.page = 1
  fetchTemplates()
}, 500)

// ========== Sandboxes ==========
const sessionLoading = ref(false)
const sessionSearch = ref('')
const sessionState = reactive({
  rowData: [] as SandboxSession[],
  page: 1,
  pageSize: 20,
  total: 0,
})

const fetchSessions = async () => {
  sessionLoading.value = true
  try {
    const params: Record<string, any> = {
      offset: (sessionState.page - 1) * sessionState.pageSize,
      limit: sessionState.pageSize,
      sortBy: 'createdAt',
      order: 'desc',
    }
    if (sessionSearch.value) {
      params.sessionId = sessionSearch.value
    }
    const res = await getSandboxSessions(params)
    sessionState.rowData = res.items || []
    sessionState.total = res.totalCount || 0
  } catch (e) {
    ElMessage.error((e as string) || 'Failed to fetch sandboxes')
  } finally {
    sessionLoading.value = false
  }
}

const handleSessionSearchDebounced = useDebounceFn(() => {
  sessionState.page = 1
  fetchSessions()
}, 500)

onMounted(() => {
  fetchTemplates()
  if (activeTab.value === 'sandboxes') {
    fetchSessions()
  }
})

defineOptions({
  name: 'SandboxPage',
})
</script>
