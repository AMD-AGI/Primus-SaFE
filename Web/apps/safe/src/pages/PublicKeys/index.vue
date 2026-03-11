<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">SSH Keys</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4"
      @click="
        () => {
          addVisible = true
        }
      "
    >
      Create SSH Key
    </el-button>
  </div>

  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="`calc(100vh - 245px)`"
      :data="tableData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="description" label="Label" min-width="140">
        <template #default="{ row }">
          {{ row.description }}
          <el-link
            class="ml-2"
            :icon="Edit"
            type="primary"
            @click="
              () => {
                curId = row.id
                curDesc = row.description
                editVisible = true
              }
            "
          />
        </template>
      </el-table-column>
      <el-table-column prop="publicKey" label="Key" min-width="420">
        <template #default="{ row }">
          <div class="flex items-center gap-2">
            <el-icon class="text-gray-400">
              <Key />
            </el-icon>

            <el-tooltip :content="row.publicKey" placement="top" :show-after="300">
              <span class="font-mono text-gray-300 select-text">
                <span class="opacity-70">{{ parseSsh(row.publicKey).type }}</span>
                &nbsp;
                <span>{{ middleEllipsis(parseSsh(row.publicKey).body, 12, 14) }}</span>
                <span v-if="parseSsh(row.publicKey).comment" class="font-semibold">
                  &nbsp;{{ parseSsh(row.publicKey).comment }}
                </span>
              </span>
            </el-tooltip>

            <el-icon
              class="cursor-pointer hover:text-blue-500 transition text-cyan-400"
              size="11"
              @click="copyText(row.publicKey)"
            >
              <CopyDocument />
            </el-icon>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="createTime" label="Create Time" width="170">
        <template #default="{ row }">
          {{ formatTimeStr(row.createTime) }}
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
              @click="onDelete(row.id, row.description)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      class="m-t-2"
      :current-page="pagination.page"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      @current-change="handlePageChange"
      @size-change="handlePageSizeChange"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50, 100]"
    />
  </el-card>
  <AddDialog v-model:visible="addVisible" @success="onSearch({ resetPage: true })" />
  <el-dialog
    v-model="editVisible"
    title="Edit Description"
    width="520px"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div class="space-y-3">
      <div class="textx-12 mt-2 flex" style="font-weight: 500; align-items: center">
        Description:
        <el-input v-model="curDesc" class="mt-3 mb-3 ml-2" style="width: 300px" />
      </div>
    </div>

    <template #footer>
      <el-button @click="editVisible = false">Cancel</el-button>
      <el-button type="primary" :loading="editLoading" :disabled="!curDesc" @click="onEditConfirm"
        >Confirm</el-button
      >
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { onMounted, ref, reactive, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getPublicKeysList, deletePublickey, editPublickeyDesc } from '@/services'
import type { NodesParams } from '@/services'
import AddDialog from './Components/AddDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Key, CopyDocument } from '@element-plus/icons-vue'
import { copyText, formatTimeStr } from '@/utils/index'

// nodes table initial val
const loading = ref(false)
const tableData = ref([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})

// add & edit
const curId = ref('')
const curDesc = ref('')
const addVisible = ref(false)
const editVisible = ref(false)
const editLoading = ref(false)

const searchParams = reactive({
  nodeId: '',
  workspaceId: '',
  available: null as boolean | null,
  isAddonsInstalled: null as boolean | null,
})

// Parse SSH public key: ssh-rsa <body> <comment>
function parseSsh(pk: string) {
  const [type = '', body = '', ...rest] = (pk ?? '').trim().split(/\s+/)
  return { type, body, comment: rest.join(' ') }
}

// Truncate middle: keep left chars on left, right chars on right
function middleEllipsis(s: string, left = 12, right = 14) {
  if (!s) return ''
  if (s.length <= left + right + 1) return s
  return `${s.slice(0, left)} … ${s.slice(-right)}`
}

const onEditConfirm = async () => {
  try {
    editLoading.value = true
    await editPublickeyDesc(curId.value, { description: curDesc.value })
    ElMessage({
      type: 'success',
      message: 'Edit completed',
    })
    editVisible.value = false
    curId.value = ''
    curDesc.value = ''
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    editLoading.value = false
    onSearch({ resetPage: false })
  }
}

const fetchData = async (params?: NodesParams) => {
  try {
    loading.value = true

    const res = await getPublicKeysList({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      ...params,
    })
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}
const handlePageChange = (newPage: number) => {
  pagination.page = newPage
  onSearch({ resetPage: false })
}

const handlePageSizeChange = (newSize: number) => {
  pagination.pageSize = newSize
  pagination.page = 1
  onSearch({ resetPage: false })
}

const onSearch = (options?: { resetPage?: boolean }) => {
  const reset = options?.resetPage ?? true
  if (reset) pagination.page = 1

  fetchData({
    ...(searchParams.available !== null ? { available: searchParams.available } : {}),
    ...(searchParams.isAddonsInstalled !== null
      ? { isAddonsInstalled: searchParams.isAddonsInstalled }
      : {}),
    ...(searchParams.workspaceId
      ? { workspaceId: searchParams.workspaceId === 'UNASSIGNED' ? '' : searchParams.workspaceId }
      : {}),
    ...(searchParams.nodeId ? { nodeId: searchParams.nodeId } : {}),
  })
}

const onDelete = (id: string, desc: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete pubilc key: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, desc),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete Pubilc Key', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deletePublickey(id)
      ElMessage({
        type: 'success',
        message: 'Delete completed',
      })
      onSearch({ resetPage: false })
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const route = useRoute()
const router = useRouter()

onMounted(() => {
  onSearch({ resetPage: true })
  // Auto-open create dialog when navigated from Secrets AddDialog
  if (route.query.create === '1') {
    addVisible.value = true
    router.replace({ path: '/publickeys' }) // Clear query to prevent re-opening on refresh
  }
})

defineOptions({
  name: 'SSH Keys Page',
})
</script>

<style scoped>
.cell-ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.wl-rich {
  max-width: 420px;
  padding: 8px 10px;
}
.wl-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.wl-list li + li {
  margin-top: 6px;
}
.wl-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-weight: 600;
}
.wl-sub {
  font-size: 12px;
  opacity: 0.8;
  margin-left: 10px;
}
</style>
