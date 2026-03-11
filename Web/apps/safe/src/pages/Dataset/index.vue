<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Dataset</el-text>
    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center gap-2">
        <el-button
          type="primary"
          round
          :icon="Plus"
          @click="showCreateDialog = true"
          class="text-black"
        >
          Create Dataset
        </el-button>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <el-input
          v-model="searchText"
          placeholder="Search by name"
          clearable
          :prefix-icon="Search"
          style="max-width: 300px"
          @input="debouncedSearch"
          @clear="handleClearSearch"
        />
      </div>
    </div>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 240px)'"
      :data="items"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @filter-change="handleFilterChange"
    >
      <el-table-column
        prop="displayName"
        label="Name/ID"
        min-width="220"
        :fixed="true"
        align="left"
      >
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" @click="jumpToDetail(row.datasetId)">{{
              row.displayName
            }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.datasetId }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.datasetId)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        prop="source"
        label="Source"
        min-width="150"
        column-key="source"
        :filters="sourceFilters"
        :filter-multiple="false"
        :filtered-value="sourceFilter ? [sourceFilter] : []"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag
            v-if="row.source === 'huggingface'"
            type="warning"
            :effect="isDark ? 'plain' : 'light'"
          >
            HuggingFace
          </el-tag>
          <el-tag
            v-else-if="row.source === 'upload'"
            type="info"
            :effect="isDark ? 'plain' : 'light'"
          >
            Upload
          </el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column
        prop="datasetType"
        label="Type"
        min-width="120"
        column-key="datasetType"
        :filters="datasetTypeFilters"
        :filter-multiple="false"
        :filtered-value="datasetType ? [datasetType] : []"
        :filter-method="passAll"
      >
        <template #default="{ row }">
          <el-tag v-if="row.datasetType" :effect="isDark ? 'plain' : 'light'">{{
            row.datasetType
          }}</el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="workspace" label="Workspace" min-width="150">
        <template #default="{ row }">
          {{ row.workspace || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="description" label="Description" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.description || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="totalSize" label="Size" min-width="120">
        <template #default="{ row }">
          {{ row.totalSizeStr || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="status" label="Status" min-width="120">
        <template #default="{ row }">
          <el-tag
            v-if="row.status"
            :type="row.status === 'Ready' ? 'success' : row.status === 'Failed' ? 'danger' : ''"
          >
            {{ row.status }}
          </el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" width="180">
        <template #default="{ row }">
          {{ row.creationTime ? dayjs(row.creationTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="80" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="onDelete(row.datasetId, row.displayName)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
  <el-pagination
    v-model:current-page="currentPage"
    v-model:page-size="pageSize"
    :total="totalCount"
    :page-sizes="[10, 20, 50, 100]"
    layout="total, sizes, prev, pager, next, jumper"
    class="m-t-4"
    @size-change="handlePageChange"
    @current-change="handlePageChange"
  />

  <CreateDialog v-model:visible="showCreateDialog" @success="handleCreateSuccess" />
  <DetailDialog v-model:visible="showDetailDialog" :dataset-id="selectedDatasetId" />
</template>

<script lang="ts" setup>
import { ref, onMounted, onBeforeUnmount, h, computed, watch } from 'vue'
import { getDatasets, deleteDataset, getDatasetTypes } from '@/services/dataset/index'
import type { DatasetItem, DatasetType } from '@/services/dataset/type'
import { CopyDocument, Search, Plus, Delete } from '@element-plus/icons-vue'
import { copyText } from '@/utils/index'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import { useDark } from '@vueuse/core'
import { debounce } from 'lodash'
import CreateDialog from './Components/CreateDialog.vue'
import DetailDialog from './Components/DetailDialog.vue'
import { useWorkspaceStore } from '@/stores/workspace'

const isDark = useDark()
const workspaceStore = useWorkspaceStore()
const loading = ref(false)
const items = ref<DatasetItem[]>([])
const totalCount = ref(0)
const currentPage = ref(1)
const pageSize = ref(10)
const datasetType = ref<string>('')
const workspace = ref<string>('')
const searchText = ref('')
const orderBy = ref<string>('')
const order = ref<'asc' | 'desc'>('desc')
const sourceFilter = ref<string>('')
const showCreateDialog = ref(false)
const showDetailDialog = ref(false)
const selectedDatasetId = ref('')
const datasetTypes = ref<DatasetType[]>([])
let pollingTimer: ReturnType<typeof setInterval> | null = null

const datasetTypeFilters = computed(() => {
  return datasetTypes.value.map((type) => ({
    text: type.name,
    value: type.name,
  }))
})

const sourceFilters = [
  { text: 'HuggingFace', value: 'huggingface' },
  { text: 'Upload', value: 'upload' },
]

const passAll = () => true

const jumpToDetail = (datasetId: string) => {
  selectedDatasetId.value = datasetId
  showDetailDialog.value = true
}

const fetchData = async () => {
  try {
    loading.value = true
    const res = await getDatasets({
      datasetType: datasetType.value || undefined,
      workspace: workspace.value || undefined,
      search: searchText.value || undefined,
      source: sourceFilter.value || undefined,
      pageNum: currentPage.value,
      pageSize: pageSize.value,
      orderBy: orderBy.value || undefined,
      order: order.value,
    })
    items.value = res.items || []
    totalCount.value = res.total || 0

    // Check if polling is needed (non-terminal datasets exist)
    checkAndStartPolling()
  } catch (error) {
    console.error('Failed to fetch datasets:', error)
    ElMessage.error('Failed to load datasets')
  } finally {
    loading.value = false
  }
}

// Check for non-terminal datasets and start polling
const checkAndStartPolling = () => {
  const hasNonTerminalStatus = items.value.some(
    (item) =>
      item.status === 'Pending' ||
      item.status === 'Uploading' ||
      item.status === 'Downloading'
  )

  if (hasNonTerminalStatus && !pollingTimer) {
    // Poll every 10 seconds
    pollingTimer = setInterval(() => {
      fetchData()
    }, 10000)
  } else if (!hasNonTerminalStatus && pollingTimer) {
    // All datasets are in terminal state, stop polling
    clearInterval(pollingTimer)
    pollingTimer = null
  }
}


const debouncedSearch = debounce(() => {
  currentPage.value = 1
  fetchData()
}, 300)

const handleClearSearch = () => {
  currentPage.value = 1
  fetchData()
}

const handleFilterChange = (filters: Record<string, string[]>) => {
  if (Object.prototype.hasOwnProperty.call(filters, 'datasetType')) {
    datasetType.value = filters.datasetType?.[0] || ''
    currentPage.value = 1
    fetchData()
  }
  if (Object.prototype.hasOwnProperty.call(filters, 'source')) {
    sourceFilter.value = filters.source?.[0] || ''
    currentPage.value = 1
    fetchData()
  }
}

const handlePageChange = () => {
  fetchData()
}

const onDelete = (datasetId: string, displayName: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete dataset: ',
    h(
      'span',
      { style: 'color: var(--el-color-primary); font-weight: 600' },
      displayName || datasetId,
    ),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete dataset', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteDataset(datasetId)
      ElMessage({
        type: 'success',
        message: 'Delete completed',
      })
      fetchData()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const handleCreateSuccess = () => {
  currentPage.value = 1
  fetchData()
}

const fetchDatasetTypes = async () => {
  try {
    const response = await getDatasetTypes()
    datasetTypes.value = response.types || []
  } catch (error) {
    console.error('Failed to fetch dataset types:', error)
  }
}

// Watch for workspace changes and auto-refresh list
watch(
  () => workspaceStore.currentWorkspaceId,
  (newWorkspaceId, oldWorkspaceId) => {
    if (newWorkspaceId !== oldWorkspaceId) {
      workspace.value = newWorkspaceId || ''
      currentPage.value = 1
      fetchData()
    }
  },
)

onMounted(() => {
  workspace.value = workspaceStore.currentWorkspaceId || ''
  fetchDatasetTypes()
  fetchData()
})

onBeforeUnmount(() => {
  if (pollingTimer) {
    clearInterval(pollingTimer)
    pollingTimer = null
  }
})

defineOptions({
  name: 'DatasetPage',
})
</script>

<style></style>
