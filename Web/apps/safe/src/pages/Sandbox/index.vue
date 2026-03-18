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
      <el-table-column label="Name" min-width="180" show-overflow-tooltip fixed="left">
        <template #default="{ row }">
          <el-link type="primary" :underline="false" @click="showTemplateDetail(row)">
            {{ row.metadata?.name }}
          </el-link>
        </template>
      </el-table-column>
      <el-table-column label="Namespace" prop="metadata.namespace" min-width="120" />
      <el-table-column label="Status" width="100" header-align="center">
        <template #default="{ row }">
          <div class="text-center">
            <el-tag :type="row.status?.ready ? 'success' : 'danger'" size="small" effect="light">
              {{ row.status?.ready ? 'Ready' : 'NotReady' }}
            </el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Image" min-width="240" show-overflow-tooltip>
        <template #default="{ row }">
          <el-text class="font-mono text-xs" type="info">
            {{ row.spec?.template?.fromImage || '-' }}
          </el-text>
        </template>
      </el-table-column>
      <el-table-column label="Resources" min-width="200">
        <template #default="{ row }">
          <div class="flex flex-wrap gap-1">
            <el-tag
              v-if="row.spec?.gpu?.product"
              size="small"
              type="warning"
              effect="light"
            >
              GPU: {{ row.spec.gpu.product }} × {{ row.spec.gpu.count }}
            </el-tag>
            <el-tag
              v-if="row.spec?.warmPoolSize"
              size="small"
              type="info"
              effect="light"
            >
              Pool: {{ row.spec.warmPoolSize }}
            </el-tag>
            <el-tag
              v-if="row.spec?.sessionTimeout"
              size="small"
              type="info"
              effect="plain"
            >
              Timeout: {{ row.spec.sessionTimeout }}
            </el-tag>
            <span v-if="!row.spec?.gpu?.product && !row.spec?.warmPoolSize && !row.spec?.sessionTimeout">-</span>
          </div>
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
      <el-table-column label="Session ID" min-width="280" show-overflow-tooltip fixed="left">
        <template #default="{ row }">
          <el-link type="primary" :underline="false" @click="showSessionDetail(row)">
            {{ row.sessionId }}
          </el-link>
        </template>
      </el-table-column>
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

  <!-- Template Detail Dialog -->
  <el-dialog
    v-model="templateDetailVisible"
    :title="'Template: ' + (currentTemplate?.metadata?.name || '')"
    width="720px"
    destroy-on-close
  >
    <el-descriptions :column="2" border>
      <el-descriptions-item label="Name">{{ currentTemplate?.metadata?.name }}</el-descriptions-item>
      <el-descriptions-item label="Namespace">{{ currentTemplate?.metadata?.namespace }}</el-descriptions-item>
      <el-descriptions-item label="Status">
        <el-tag
          :type="currentTemplate?.status?.ready ? 'success' : 'danger'"
          size="small"
          effect="light"
        >
          {{ currentTemplate?.status?.ready ? 'Ready' : 'NotReady' }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="Auth Mode">{{ currentTemplate?.spec?.authMode || '-' }}</el-descriptions-item>
      <el-descriptions-item label="Image" :span="2">
        <el-text class="font-mono text-xs">{{ currentTemplate?.spec?.template?.fromImage || '-' }}</el-text>
      </el-descriptions-item>
      <el-descriptions-item label="GPU" v-if="currentTemplate?.spec?.gpu?.product">
        {{ currentTemplate.spec.gpu.product }} × {{ currentTemplate.spec.gpu.count }}
      </el-descriptions-item>
      <el-descriptions-item label="Warm Pool Size">{{ currentTemplate?.spec?.warmPoolSize ?? '-' }}</el-descriptions-item>
      <el-descriptions-item label="Session Timeout">{{ currentTemplate?.spec?.sessionTimeout || '-' }}</el-descriptions-item>
      <el-descriptions-item label="Max Duration">{{ currentTemplate?.spec?.maxSessionDuration || '-' }}</el-descriptions-item>
      <el-descriptions-item label="Creator" :span="2">
        {{ currentTemplate?.metadata?.annotations?.['runtime.agent-sandbox.io/user.name'] || '-' }}
      </el-descriptions-item>
      <el-descriptions-item label="Created" :span="2">
        {{ formatTimeStr(currentTemplate?.metadata?.creationTimestamp) }}
      </el-descriptions-item>
    </el-descriptions>

    <el-divider content-position="left">JSON</el-divider>
    <el-input
      type="textarea"
      :model-value="templateDetailJson"
      readonly
      :autosize="{ minRows: 6, maxRows: 20 }"
      class="font-mono"
    />
    <template #footer>
      <el-button @click="copyJson(templateDetailJson)">Copy JSON</el-button>
      <el-button @click="templateDetailVisible = false">Close</el-button>
    </template>
  </el-dialog>

  <!-- Session Detail Dialog -->
  <el-dialog
    v-model="sessionDetailVisible"
    :title="'Session: ' + (currentSession?.sessionId || '')"
    width="720px"
    destroy-on-close
  >
    <el-descriptions :column="2" border>
      <el-descriptions-item label="Session ID" :span="2">
        <el-text class="font-mono text-xs">{{ currentSession?.sessionId }}</el-text>
      </el-descriptions-item>
      <el-descriptions-item label="Sandbox Name">{{ currentSession?.sandboxName }}</el-descriptions-item>
      <el-descriptions-item label="Namespace">{{ currentSession?.namespace }}</el-descriptions-item>
      <el-descriptions-item label="Status">
        <el-tag
          :type="currentSession?.status === 'running' ? 'success' : 'info'"
          size="small"
        >
          {{ currentSession?.status || 'Unknown' }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="Pod IP">{{ currentSession?.podIp || '-' }}</el-descriptions-item>
      <el-descriptions-item label="User">{{ currentSession?.userName || '-' }}</el-descriptions-item>
      <el-descriptions-item label="User ID">{{ currentSession?.userId || '-' }}</el-descriptions-item>
      <el-descriptions-item label="Created">{{ formatTimeStr(currentSession?.createdAt) }}</el-descriptions-item>
      <el-descriptions-item label="Last Activity">{{ formatTimeStr(currentSession?.lastActivity) }}</el-descriptions-item>
      <el-descriptions-item label="Expires At" :span="2">{{ formatTimeStr(currentSession?.expiresAt) }}</el-descriptions-item>
      <el-descriptions-item v-if="currentSession?.entryPoints" label="Entry Points" :span="2">
        <div class="flex flex-wrap gap-1">
          <el-tag
            v-for="(url, key) in currentSession.entryPoints"
            :key="key"
            size="small"
            effect="plain"
          >
            {{ key }}
          </el-tag>
        </div>
      </el-descriptions-item>
    </el-descriptions>

    <el-divider content-position="left">JSON</el-divider>
    <el-input
      type="textarea"
      :model-value="sessionDetailJson"
      readonly
      :autosize="{ minRows: 6, maxRows: 20 }"
      class="font-mono"
    />
    <template #footer>
      <el-button @click="copyJson(sessionDetailJson)">Copy JSON</el-button>
      <el-button @click="sessionDetailVisible = false">Close</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { useDebounceFn, useClipboard } from '@vueuse/core'
import { getSandboxTemplates, getSandboxSessions } from '@/services/sandbox'
import { formatTimeStr } from '@/utils'
import type { SandboxTemplate, SandboxSession } from '@/services/sandbox/type'

const route = useRoute()
const router = useRouter()
const { copy } = useClipboard()

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

// ========== Template Detail ==========
const templateDetailVisible = ref(false)
const currentTemplate = ref<SandboxTemplate | null>(null)
const templateDetailJson = computed(() =>
  currentTemplate.value ? JSON.stringify(currentTemplate.value, null, 2) : '',
)

const showTemplateDetail = (row: SandboxTemplate) => {
  currentTemplate.value = row
  templateDetailVisible.value = true
}

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

// ========== Session Detail ==========
const sessionDetailVisible = ref(false)
const currentSession = ref<SandboxSession | null>(null)
const sessionDetailJson = computed(() =>
  currentSession.value ? JSON.stringify(currentSession.value, null, 2) : '',
)

const showSessionDetail = (row: SandboxSession) => {
  currentSession.value = row
  sessionDetailVisible.value = true
}

// ========== Shared ==========
const copyJson = async (json: string) => {
  try {
    await copy(json)
    ElMessage.success('Copied to clipboard')
  } catch {
    ElMessage.error('Failed to copy')
  }
}

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
