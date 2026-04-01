<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Datasync</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="
        () => {
          addVisible = true
          curJobId = ''
          curAction = 'Create'
        }
      "
    >
      Create Datasync
    </el-button>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 200px)'"
      :data="tableData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="jobId" label="Name/ID" min-width="200" :fixed="true">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" v-route="{ path: '/download/detail', query: { id: row.jobId } }">{{ row.jobName }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.jobId }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.jobId)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="phase" label="Phase" min-width="120" header-align="center">
        <template #default="{ row }">
          <div class="text-center">
            <el-tag :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'">{{
              row.phase
            }}</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="userName" label="User" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.userName || '-' }}
        </template>
      </el-table-column>

      <el-table-column prop="creationTime" label="creationTime" min-width="180">
        <template #default="{ row }">
          {{ formatTimeStr(row.creationTime) }}
        </template>
      </el-table-column>
      <el-table-column prop="endTime" label="endTime" min-width="180">
        <template #default="{ row }">
          {{ formatTimeStr(row.endTime) }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              @click="onDelete(row.jobId)"
            />
          </el-tooltip>
          <el-tooltip content="Stop" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :icon="Close"
              @click="onStop(row.jobId)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
  <AddDialog
    v-model:visible="addVisible"
    :jobid="curJobId"
    :action="curAction"
    @success="onSuccess"
  />
</template>
<script setup lang="ts">
import { onMounted, ref, h, watch, defineOptions } from 'vue'

defineOptions({
  name: 'WorkspaceDownload',
})
import { useRouter } from 'vue-router'
import { getOpsjobs, deleteOpsjobs, stopOpsjob } from '@/services'
import { Plus, CopyDocument, Delete, Close } from '@element-plus/icons-vue'
import AddDialog from './Components/AddDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { copyText, formatTimeStr } from '@/utils'
import { WorkloadPhaseButtonType } from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'

const router = useRouter()
const wsStore = useWorkspaceStore()

const loading = ref(false)
const tableData = ref([])

const addVisible = ref(false)
const curJobId = ref('')
const curAction = ref('Create')

const jumpToDetail = (jobId: string) => {
  router.push({ path: '/download/detail', query: { id: jobId } })
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getOpsjobs({
      type: 'download',
      workspaceId: wsStore.currentWorkspaceId,
    })

    tableData.value = res?.items || []
  } catch (err) {
    console.error('Failed to fetch download data:', err)
    tableData.value = []
  } finally {
    loading.value = false
  }
}

const onSuccess = () => {
  fetchData()
}

const onDelete = (jobId: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete download: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, jobId),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete download', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteOpsjobs(jobId)
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

const onStop = (jobId: string) => {
  const msg = h('span', null, [
    'Are you sure you want to stop download: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, jobId),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Stop download', {
    confirmButtonText: 'Stop',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await stopOpsjob(jobId)
      ElMessage({
        type: 'success',
        message: 'Stop completed',
      })
      fetchData()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Stop canceled')
      }
    })
}

onMounted(() => {
  fetchData()
})

// Watch for workspace changes and reload data
watch(
  () => wsStore.currentWorkspaceId,
  () => {
    fetchData()
  },
)
</script>
