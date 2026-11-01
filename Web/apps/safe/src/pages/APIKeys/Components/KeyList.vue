<template>
  <template v-if="present">
    <div v-if="title" class="mt-6">
      <el-text class="block font-500" tag="b">{{ title }}</el-text>
      <el-text v-if="hint" type="info" class="block mt-1">{{ hint }}</el-text>
    </div>
    <el-card class="safe-card" :class="title ? 'mt-3' : 'mt-6'" shadow="never">
      <el-table
        :height="height"
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
        <el-table-column
          prop="expirationTime"
          label="Expiration Time"
          width="180"
          sortable="custom"
        >
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
  </template>
</template>

<script lang="ts" setup>
import { ref, onMounted, h, reactive, watch } from 'vue'
import { listAPIKeys, deleteAPIKey } from '@/services/apikeys'
import type { APIKey } from '@/services/apikeys/type'
import { Delete, Key } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import { formatTimeStr } from '@/utils/index'

/**
 * One list of API keys, from one source: the identity provider's route when `providerUrl` is set, the
 * apiserver's own otherwise (see services/apikeys). A key is deleted where it is listed.
 */
const props = withDefaults(
  defineProps<{
    // The identity provider's route for the keys; '' for the apiserver's own
    providerUrl?: string
    // The name filter, already debounced by the page
    name?: string
    height?: string
    title?: string
    hint?: string
    // Show nothing until an unfiltered fetch finds a key: a section that is only there when it has
    // something to offer
    hideUntilPresent?: boolean
  }>(),
  { providerUrl: '', name: '', height: undefined, title: '', hint: '', hideUntilPresent: false },
)
const emit = defineEmits<{ present: [boolean] }>()

const loading = ref(false)
const present = ref(!props.hideUntilPresent)
const items = ref<APIKey[]>([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const sortBy = ref<'creationTime' | 'expirationTime'>('creationTime')
const order = ref<'asc' | 'desc'>('desc')

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
      await deleteAPIKey(id, props.providerUrl)
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

const refetch = async () => {
  try {
    loading.value = true
    const res = await listAPIKeys(
      {
        offset: (pagination.page - 1) * pagination.pageSize,
        limit: pagination.pageSize,
        sortBy: sortBy.value,
        order: order.value,
        ...(props.name ? { name: props.name } : {}),
      },
      props.providerUrl,
    )
    items.value = res?.items || []
    pagination.total = res?.totalCount || 0
    if (props.hideUntilPresent && !props.name) {
      present.value = pagination.total > 0
    }
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
    emit('present', present.value)
  }
}

watch(
  () => props.name,
  () => {
    pagination.page = 1
    refetch()
  },
)

onMounted(() => {
  refetch()
})

defineExpose({ refetch })
</script>
