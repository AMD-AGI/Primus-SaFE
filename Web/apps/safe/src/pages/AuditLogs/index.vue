<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Audit Logs</el-text>
    <div class="flex flex-wrap items-center mt-4">
      <el-input
        v-model="searchText"
        placeholder="Search by Username or Request Path"
        clearable
        :prefix-icon="Search"
        style="max-width: 300px"
        @input="debouncedSearch"
        @clear="
          () => {
            pagination.page = 1
            refetch()
          }
        "
      />
    </div>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      ref="tableRef"
      :height="'calc(100vh - 240px)'"
      :data="items"
      size="large"
      class="m-t-2 auditlogs-table"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @sort-change="handleSortChange"
      @filter-change="handleFilterChange"
    >
      <el-table-column prop="userName" label="User" min-width="160" fixed="left" align="left" />
      <el-table-column
        prop="userType"
        label="User Type"
        width="120"
        column-key="userTypeFilter"
        :filters="userTypeFilters"
        :filtered-value="userTypeSelectedValues"
        :filter-multiple="true"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag
            :type="
              row.userType === 'default'
                ? 'primary'
                : row.userType === 'sso'
                  ? 'success'
                  : 'warning'
            "
            size="small"
            :effect="isDark ? 'plain' : 'light'"
          >
            {{ row.userType }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="action" label="Action" width="220">
        <template #default="{ row }">
          <span class="font-medium">{{ row.action }}</span>
        </template>
      </el-table-column>
      <el-table-column
        prop="httpMethod"
        label="Method"
        width="110"
        column-key="httpMethodFilter"
        :filters="httpMethodFilters"
        :filtered-value="httpMethodSelectedValues"
        :filter-multiple="true"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag
            :type="
              row.httpMethod === 'POST'
                ? 'success'
                : row.httpMethod === 'DELETE'
                  ? 'danger'
                  : row.httpMethod === 'PUT' || row.httpMethod === 'PATCH'
                    ? 'warning'
                    : 'info'
            "
            size="small"
          >
            {{ row.httpMethod }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column
        prop="resourceType"
        label="Resource Type"
        width="160"
        column-key="resourceTypeFilter"
        :filters="resourceTypeFilters"
        :filtered-value="resourceTypeSelectedValues"
        :filter-multiple="true"
        filter-placement="bottom-start"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          {{ row.resourceType }}
        </template>
      </el-table-column>
      <el-table-column
        prop="requestPath"
        label="Request Path"
        min-width="280"
        show-overflow-tooltip
      >
        <template #default="{ row }">
          <el-link
            class="auditlogs-path-link font-mono text-sm"
            type="primary"
            :underline="true"
            @click.stop="showDetails(row)"
          >
            {{ row.requestPath }}
          </el-link>
        </template>
      </el-table-column>
      <el-table-column prop="responseStatus" label="Status" width="100">
        <template #default="{ row }">
          <el-tag
            :type="
              row.responseStatus >= 200 && row.responseStatus < 300
                ? 'success'
                : row.responseStatus >= 400 && row.responseStatus < 500
                  ? 'warning'
                  : row.responseStatus >= 500
                    ? 'danger'
                    : 'info'
            "
            size="small"
          >
            {{ row.responseStatus }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="clientIp" label="Client IP" width="160">
        <template #default="{ row }">
          {{ row.clientIp }}
        </template>
      </el-table-column>
      <el-table-column prop="latencyMs" label="Latency" width="110">
        <template #default="{ row }">
          <span class="text-sm">{{ row.latencyMs ? `${row.latencyMs}ms` : '-' }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="createTime" label="Time" width="200" sortable="custom" fixed="right">
        <template #default="{ row }">
          {{ formatTimeStr(row.createTime) }}
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      v-model:current-page="pagination.page"
      v-model:page-size="pagination.pageSize"
      :total="pagination.total"
      class="m-t-2"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[20, 50, 100]"
      @current-change="handlePageChange"
      @size-change="handlePageSizeChange"
    />
  </el-card>

  <!-- Details Dialog -->
  <el-dialog v-model="detailsVisible" title="Audit Log Details" width="900px">
    <div v-if="selectedLog" class="audit-detail-content">
      <!-- Trace ID Section -->
      <div class="detail-section">
        <div class="detail-label">
          <div class="w-1 h-4 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span>Trace ID</span>
        </div>
        <div class="trace-id-wrapper">
          <span class="trace-id-text">{{ selectedLog.traceId || '-' }}</span>
          <el-button size="small" :icon="CopyDocument" @click="copyText(selectedLog.traceId)" text>
            Copy
          </el-button>
        </div>
      </div>

      <!-- Request Body Section -->
      <div class="detail-section">
        <div class="detail-label">
          <div class="w-1 h-4 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span>Request Body</span>
        </div>
        <div class="request-body-wrapper">
          <div v-if="isJsonRequestBody(selectedLog)">
            <pre class="json-viewer">{{ formatJson(selectedLog.requestBody) }}</pre>
          </div>
          <div v-else class="plain-text-viewer">
            {{ selectedLog.requestBody || 'No request body' }}
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, onMounted, reactive } from 'vue'
import { listAuditLogs } from '@/services/auditlogs'
import type { AuditLog } from '@/services/auditlogs/type'
import { Search, CopyDocument } from '@element-plus/icons-vue'
import type { TableInstance } from 'element-plus'
import { debounce } from 'lodash'
import { useDark } from '@vueuse/core'
import { copyText, formatTimeStr } from '@/utils/index'

const isDark = useDark()
const loading = ref(false)
const items = ref<AuditLog[]>([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const sortBy = ref<string>('createTime')
const order = ref<'asc' | 'desc'>('desc')
const searchText = ref('')
const tableRef = ref<TableInstance>()
const detailsVisible = ref(false)
const selectedLog = ref<AuditLog | null>(null)

// Filter options
const userTypeSelectedValues = ref<string[]>([])
const userTypeFilters = [
  { text: 'default', value: 'default' },
  { text: 'sso', value: 'sso' },
  { text: 'apikey', value: 'apikey' },
]

const httpMethodSelectedValues = ref<string[]>([])
const httpMethodFilters = [
  { text: 'POST', value: 'POST' },
  { text: 'PUT', value: 'PUT' },
  { text: 'PATCH', value: 'PATCH' },
  { text: 'DELETE', value: 'DELETE' },
]

const resourceTypeSelectedValues = ref<string[]>([])
const resourceTypeFilters = [
  { text: 'workload', value: 'workload' },
  { text: 'secret', value: 'secret' },
  { text: 'fault', value: 'fault' },
  { text: 'nodetemplate', value: 'nodetemplate' },
  { text: 'node', value: 'node' },
  { text: 'workspace', value: 'workspace' },
  { text: 'cluster', value: 'cluster' },
  { text: 'addon', value: 'addon' },
  { text: 'nodeflavor', value: 'nodeflavor' },
  { text: 'opsjob', value: 'opsjob' },
  { text: 'user', value: 'user' },
  { text: 'publickey', value: 'publickey' },
  { text: 'apikey', value: 'apikey' },
  { text: 'auth', value: 'auth' },
  { text: 'session', value: 'session' },
  { text: 'model', value: 'model' },
  { text: 'dataset', value: 'dataset' },
  { text: 'image', value: 'image' },
  { text: 'custom-image', value: 'custom-image' },
  { text: 'imageregistry', value: 'imageregistry' },
  { text: 'deployment', value: 'deployment' },
]

const passAll = () => true

const handlePageChange = (newPage: number) => {
  pagination.page = newPage
  refetch(searchText.value)
}

const handlePageSizeChange = (newSize: number) => {
  pagination.pageSize = newSize
  pagination.page = 1
  refetch(searchText.value)
}

const handleSortChange = ({
  prop,
  order: sortOrder,
}: {
  prop: string
  order: 'ascending' | 'descending' | null
}) => {
  if (sortOrder && prop === 'createTime') {
    sortBy.value = prop
    order.value = sortOrder === 'ascending' ? 'asc' : 'desc'
  } else {
    sortBy.value = 'createTime'
    order.value = 'desc'
  }
  pagination.page = 1
  refetch(searchText.value)
}

const handleFilterChange = (filters: Record<string, string[]>) => {
  if ('userTypeFilter' in filters) {
    userTypeSelectedValues.value = filters.userTypeFilter ?? []
  }
  if ('httpMethodFilter' in filters) {
    httpMethodSelectedValues.value = filters.httpMethodFilter ?? []
  }
  if ('resourceTypeFilter' in filters) {
    resourceTypeSelectedValues.value = filters.resourceTypeFilter ?? []
  }
  pagination.page = 1
  refetch(searchText.value)
}

const showDetails = (row: AuditLog) => {
  selectedLog.value = row
  detailsVisible.value = true
}

const isJsonRequestBody = (log: AuditLog) => {
  if (!log.requestBody) return false
  const method = log.httpMethod?.toUpperCase()
  return method === 'POST' || method === 'PATCH' || method === 'PUT'
}

const formatJson = (jsonString: string) => {
  if (!jsonString) return '-'
  try {
    const obj = JSON.parse(jsonString)
    return JSON.stringify(obj, null, 2)
  } catch (_e) {
    return jsonString
  }
}
const refetch = async (search?: string) => {
  try {
    loading.value = true
    const params: Record<string, string | number> = {
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      sortBy: sortBy.value,
      order: order.value,
    }

    if (search) {
      params.keyword = search.trim()
    }

    // Add filter parameters
    if (userTypeSelectedValues.value.length > 0) {
      params.userType = userTypeSelectedValues.value.join(',')
    }
    if (httpMethodSelectedValues.value.length > 0) {
      params.httpMethod = httpMethodSelectedValues.value.join(',')
    }
    if (resourceTypeSelectedValues.value.length > 0) {
      params.resourceType = resourceTypeSelectedValues.value.join(',')
    }

    const res = await listAuditLogs(params)
    items.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}

const debouncedSearch = debounce((text: string) => {
  pagination.page = 1
  refetch(text)
}, 300)

onMounted(() => {
  refetch()
})

defineOptions({
  name: 'AuditLogsPage',
})
</script>

<style scoped>
.audit-detail-content {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-label {
  display: flex;
  align-items: center;
  font-size: 14px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.trace-id-wrapper {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background-color: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  transition: all 0.2s;
}

.trace-id-wrapper:hover {
  border-color: var(--el-border-color);
  background-color: var(--el-fill-color);
}

.trace-id-text {
  flex: 1;
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  color: var(--el-text-color-regular);
  word-break: break-all;
}

.request-body-wrapper {
  max-width: 100%;
  overflow: hidden;
}

.json-viewer {
  background-color: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 16px;
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  line-height: 1.6;
  max-height: 450px;
  /* min-height: 300px; */
  overflow-y: auto;
  overflow-x: auto;
  margin: 0;
  width: 100%;
  box-sizing: border-box;
  white-space: pre-wrap;
  word-wrap: break-word;
  word-break: break-all;
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
}

.plain-text-viewer {
  background-color: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 16px;
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  line-height: 1.6;
  max-height: 450px;
  overflow-y: auto;
  color: var(--el-text-color-regular);
  white-space: pre-wrap;
  word-break: break-all;
}

.dark .json-viewer,
.dark .plain-text-viewer {
  background-color: var(--el-fill-color-darker);
  border-color: var(--el-border-color-dark);
}

.dark .trace-id-wrapper {
  background-color: var(--el-fill-color-darker);
  border-color: var(--el-border-color-dark);
}

.dark .trace-id-wrapper:hover {
  background-color: var(--el-fill-color-dark);
  border-color: var(--el-border-color);
}

.auditlogs-path-link {
  user-select: text;
  max-width: 100%;
}

:deep(.auditlogs-path-link .el-link__inner) {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}
</style>
