<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">API Keys</el-text>
    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center">
        <el-button
          type="primary"
          round
          :icon="Plus"
          class="m-t-4 text-black"
          @click="
            () => {
              addVisible = true
            }
          "
        >
          Create API Key
        </el-button>
      </div>
      <div class="flex flex-wrap items-center">
        <el-input
          v-model="searchName"
          placeholder="Search by name"
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
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 240px)'"
      :data="items"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
      @sort-change="handleSortChange"
    >
      <el-table-column prop="name" label="Name" min-width="200" :fixed="true" align="left" />
      <el-table-column prop="apiKey" label="API Key" min-width="180">
        <template #default="{ row }">
          <div class="flex items-center gap-2">
            <el-icon class="text-gray-400">
              <Key />
            </el-icon>
            <span class="font-mono text-gray-300">{{ row.keyHint ?? '-' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="expirationTime" label="Expiration Time" width="180" sortable="custom">
        <template #default="{ row }">
          {{ row.expirationTime ? dayjs(row.expirationTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="status" label="Status" min-width="100">
        <template #default="{ row }">
          <el-tag :type="isExpired(row.expirationTime) ? 'danger' : 'success'">
            {{ isExpired(row.expirationTime) ? 'Expired' : 'Active' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="whitelist" label="IP Whitelist" min-width="180">
        <template #default="{ row }">
          <div v-if="row.whitelist && row.whitelist.length > 0">
            <el-tag
              v-for="ip in row.whitelist.slice(0, 2)"
              :key="ip"
              effect="plain"
              size="small"
              class="mr-1"
            >
              {{ ip }}
            </el-tag>
            <el-tooltip v-if="row.whitelist.length > 2" placement="top" raw-content>
              <template #content>
                <div v-for="ip in row.whitelist.slice(2)" :key="ip">{{ ip }}</div>
              </template>
              <el-tag size="small" effect="plain" type="info" class="cursor-pointer">
                +{{ row.whitelist.length - 2 }}
              </el-tag>
            </el-tooltip>
          </div>
          <el-text v-else type="info">No restriction</el-text>
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" width="180" sortable="custom">
        <template #default="{ row }">
          {{ formatTimeStr(row.creationTime) }}
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="100" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="onDelete(row.id, row.name)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      v-model:current-page="pagination.page"
      v-model:page-size="pagination.pageSize"
      :total="pagination.total"
      class="m-t-2"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50, 100]"
      @current-change="handlePageChange"
      @size-change="handlePageSizeChange"
    />
  </el-card>
  <AddDialog v-model:visible="addVisible" @success="refetch" />
</template>

<script lang="ts" setup>
import { ref, onMounted, h, reactive } from 'vue'
import { listAPIKeys, deleteAPIKey } from '@/services/apikeys'
import type { APIKey } from '@/services/apikeys/type'
import { Plus, Delete, Key, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import { debounce } from 'lodash'
import AddDialog from './Components/AddDialog.vue'
import { formatTimeStr } from '@/utils/index'

const addVisible = ref(false)
const loading = ref(false)
const items = ref<APIKey[]>([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const sortBy = ref<'creationTime' | 'expirationTime'>('creationTime')
const order = ref<'asc' | 'desc'>('desc')
const searchName = ref('')

const isExpired = (expirationTime: string) => {
  return dayjs(expirationTime).isBefore(dayjs())
}

const handlePageChange = (newPage: number) => {
  pagination.page = newPage
  refetch()
}

const handlePageSizeChange = (newSize: number) => {
  pagination.pageSize = newSize
  pagination.page = 1
  refetch()
}

const handleSortChange = ({
  prop,
  order: sortOrder,
}: {
  prop: string
  order: 'ascending' | 'descending' | null
}) => {
  if (sortOrder && (prop === 'creationTime' || prop === 'expirationTime')) {
    sortBy.value = prop
    order.value = sortOrder === 'ascending' ? 'asc' : 'desc'
  } else {
    sortBy.value = 'creationTime'
    order.value = 'desc'
  }
  pagination.page = 1
  refetch()
}

const onDelete = (id: number, name: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete API Key: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, name),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete API Key', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteAPIKey(id)
      ElMessage({
        type: 'success',
        message: 'Delete completed',
      })
      refetch()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const refetch = async (name?: string) => {
  try {
    loading.value = true
    const res = await listAPIKeys({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      sortBy: sortBy.value,
      order: order.value,
      ...(name ? { name } : {}),
    })
    items.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}

const debouncedSearch = debounce((name: string) => {
  pagination.page = 1
  refetch(name.trim())
}, 300)

onMounted(() => {
  refetch()
})

defineOptions({
  name: 'APIKeysPage',
})
</script>

<style></style>
