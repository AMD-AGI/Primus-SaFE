<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Workspace</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="
        () => {
          curAction = 'Create'
          curId = ''
          addVisible = true
        }
      "
    >
      Create Workspace
    </el-button>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 200px)'"
      :data="items"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column
        prop="workspaceId"
        label="Name/ID"
        min-width="220"
        :fixed="true"
        align="left"
      >
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" v-route="{ path: '/workspace/detail', query: { id: row.workspaceId } }">{{
              row.workspaceName
            }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.workspaceId }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.workspaceId)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="currentNodeCount" label="currentNode" min-width="230">
        <template #header>Node ( ready / current / target )</template>
        <template #default="{ row }">
          {{ row.currentNodeCount ? row.currentNodeCount - row.abnormalNodeCount : '0' }} /
          {{ row.currentNodeCount ?? '0' }} / {{ row.targetNodeCount ?? '0' }}
        </template>
      </el-table-column>
      <el-table-column prop="clusterId" label="Cluster" min-width="120" />
      <el-table-column
        prop="managers"
        label="Managers"
        :formatter="
          (row: any, column: import('element-plus').TableColumnCtx<any>, cellValue: ManagerObj[]) =>
            cellValue?.map((w) => w.name).join(', ') || '-'
        "
        min-width="140"
      />
      />
      <el-table-column prop="phase" label="Phase" min-width="120">
        <template #default="{ row }">
          <el-tag :type="row.phase === 'Running' ? 'success' : 'danger'">{{ row.phase }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="isDefault" label="Default Accessible" min-width="150">
        <template #default="{ row }">
          <el-tag :type="row.isDefault ? 'success' : 'danger'">{{ row.isDefault ?? false }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" width="180">
        <template #default="{ row }">
          {{ row.creationTime ? dayjs(row.creationTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="queuePolicy" label="Queue Policy" width="160" />
      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="onDelete(row.workspaceId)"
            />
          </el-tooltip>
          <el-tooltip content="Edit" placement="top">
            <el-button
              circle
              class="btn-primary-plain"
              :icon="Edit"
              size="default"
              @click="
                () => {
                  curAction = 'Edit'
                  curId = row.workspaceId
                  addVisible = true
                }
              "
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
  <!-- <el-pagination
    v-model:current-page="currentPage"
    :total="rawData.length"
    :page-size="pageSize"
    class="m-t-2"
  /> -->
  <AddDialog v-model:visible="addVisible" :action="curAction" :wsid="curId" @success="refetch" />
</template>

<script lang="ts" setup>
import { ref, onMounted, h } from 'vue'
import { deleteWorkspace } from '@/services/workspace/index'
import { CopyDocument, Plus } from '@element-plus/icons-vue'
import { copyText } from '@/utils/index'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit } from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'

import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { storeToRefs } from 'pinia'
import dayjs from 'dayjs'
import AddDialog from './Components/AddDialog.vue'

const store = useWorkspaceStore()
const userStore = useUserStore()
const { items } = storeToRefs(store)

const router = useRouter()
const addVisible = ref(false)
const loading = ref(false)
const curId = ref('')
const curAction = ref<'Create' | 'Edit'>('Create')

type ManagerObj = { id: string; name: string }

// const rawData = ref([])
// const currentPage = ref(1)
// const pageSize = 10

const jumpToDetail = (id: string) => {
  router.push({ path: '/workspace/detail', query: { id } })
}

const onDelete = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete workspace: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete workspace', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteWorkspace(id)
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

const refetch = () => store.fetchWorkspace(true)

onMounted(() => {
  refetch()
})

defineOptions({
  name: 'WorkspacePage',
})
</script>

<style></style>
