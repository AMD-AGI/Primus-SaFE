<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Slurm Clusters</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      data-testid="create-slurm-cluster"
      @click="onCreate"
    >
      Create Slurm Cluster
    </el-button>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh / var(--zoom) - 215px)'"
      :data="rowData"
      size="large"
      class="m-t-4"
      v-loading="loading"
      :element-loading-text="$loadingText"
      data-testid="slurm-cluster-table"
    >
      <el-table-column prop="name" label="Name">
        <template #default="{ row }">
          <el-link
            type="primary"
            data-testid="slurm-name-link"
            v-route="{
              path: '/slurm/detail',
              query: {
                name: row.name,
                workspaceId: row.workspace || workspaceStore.currentWorkspaceId,
                clusterId: clusterStore.currentClusterId,
              },
            }"
            >{{ row.name }}</el-link
          >
        </template>
      </el-table-column>
      <el-table-column prop="phase" label="Status">
        <template #default="{ row }">
          <el-tag :type="phaseTagType(row.phase)" data-testid="slurm-phase">
            {{ row.phase || '-' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="partitions" label="Partitions">
        <template #default="{ row }">
          <span data-testid="slurm-partitions">
            {{ (row.partitions || []).join(', ') || '-' }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="Nodes (ready / desired)">
        <template #default="{ row }">
          <span data-testid="slurm-nodes">{{ row.nodesReady ?? 0 }} / {{ row.nodesDesired ?? 0 }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="accountingEnabled" label="Accounting">
        <template #default="{ row }">{{ row.accountingEnabled ? 'On' : 'Off' }}</template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time">
        <template #default="{ row }">{{ formatTimeStr(row.creationTime) }}</template>
      </el-table-column>
      <el-table-column label="Actions" width="280" fixed="right">
        <template #default="{ row }">
          <el-tooltip
            :content="row.phase === 'Running' ? 'SSH to login node' : 'Available when Running'"
            placement="top"
          >
            <span style="display: inline-block">
              <el-button
                circle
                size="default"
                :icon="Connection"
                :disabled="row.phase !== 'Running'"
                data-testid="slurm-ssh"
                @click="onSsh(row)"
              />
            </span>
          </el-tooltip>
          <el-tooltip content="Clone" placement="top">
            <el-button
              circle
              size="default"
              class="btn-success-plain"
              :icon="DocumentCopy"
              data-testid="slurm-clone"
              @click="onClone(row)"
            />
          </el-tooltip>
          <el-tooltip v-if="!isStopped(row)" content="Edit" placement="top">
            <el-button
              circle
              size="default"
              :icon="Edit"
              data-testid="slurm-edit"
              @click="onEdit(row)"
            />
          </el-tooltip>
          <el-tooltip v-if="isStopped(row)" content="Resume" placement="top">
            <el-button
              circle
              size="default"
              class="btn-success-plain"
              :icon="VideoPlay"
              data-testid="slurm-resume"
              @click="onResume(row)"
            />
          </el-tooltip>
          <el-tooltip v-else content="Stop" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :icon="VideoPause"
              data-testid="slurm-stop"
              @click="onStop(row)"
            />
          </el-tooltip>
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              data-testid="slurm-delete"
              @click="onDelete(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
      <template #empty>
        <span>No Slurm clusters in this workspace yet. Click "Create Slurm Cluster" to deploy one.</span>
      </template>
    </el-table>
  </el-card>

  <AddDialog
    v-model:visible="showDialog"
    :edit-item="editItem"
    :clone-item="cloneItem"
    @success="getList"
  />
  <SshDialog
    v-model:visible="showSshDialog"
    :info="sshInfo"
    :cluster-name="sshClusterName"
    :loading="sshLoading"
  />
</template>

<script lang="ts" setup>
import { ref, onMounted, watch, h } from 'vue'
import {
  Plus,
  Delete,
  Edit,
  Connection,
  DocumentCopy,
  VideoPlay,
  VideoPause,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getSlurmClusterList,
  deleteSlurmCluster,
  getSlurmClusterLogin,
  stopSlurmCluster,
  resumeSlurmCluster,
} from '@/services'
import type { SlurmClusterItem, SlurmLoginInfo } from '@/services/slurm/type'
import { useClusterStore } from '@/stores/cluster'
import { useWorkspaceStore } from '@/stores/workspace'
import { formatTimeStr } from '@/utils'
import AddDialog from './Components/AddDialog.vue'
import SshDialog from './Components/SshDialog.vue'

const clusterStore = useClusterStore()
const workspaceStore = useWorkspaceStore()
const loading = ref(false)
const showDialog = ref(false)
const editItem = ref<SlurmClusterItem | null>(null)
const cloneItem = ref<SlurmClusterItem | null>(null)
const rowData = ref<SlurmClusterItem[]>([])
const showSshDialog = ref(false)
const sshInfo = ref<SlurmLoginInfo | null>(null)
const sshClusterName = ref('')
const sshLoading = ref(false)

const isStopped = (row: SlurmClusterItem) => row.stopped === true || row.phase === 'Stopped'

const phaseTagType = (phase: string) => {
  if (phase === 'Running') return 'success'
  if (phase === 'Failed') return 'danger'
  return 'info'
}

const getList = async () => {
  const workspaceId = workspaceStore.currentWorkspaceId
  if (!workspaceId) {
    rowData.value = []
    return
  }
  loading.value = true
  try {
    const res = await getSlurmClusterList(clusterStore.currentClusterId ?? '', workspaceId)
    rowData.value = res.items || []
  } finally {
    loading.value = false
  }
}

const onCreate = () => {
  editItem.value = null
  cloneItem.value = null
  showDialog.value = true
}

const onEdit = (row: SlurmClusterItem) => {
  cloneItem.value = null
  editItem.value = row
  showDialog.value = true
}

const onClone = (row: SlurmClusterItem) => {
  editItem.value = null
  cloneItem.value = row
  showDialog.value = true
}

const onStop = (row: SlurmClusterItem) => {
  const msg = h('span', null, [
    'Stop Slurm cluster ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.name),
    '? Its components scale to zero (freeing compute); it stays in the list and can be resumed. Running jobs are lost.',
  ])
  ElMessageBox.confirm(msg, 'Stop Slurm cluster', {
    confirmButtonText: 'Stop',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await stopSlurmCluster(
      clusterStore.currentClusterId ?? '',
      row.name,
      row.workspace || workspaceStore.currentWorkspaceId || '',
    )
    ElMessage.success('Stopping cluster')
    getList()
  })
}

const onResume = async (row: SlurmClusterItem) => {
  await resumeSlurmCluster(
    clusterStore.currentClusterId ?? '',
    row.name,
    row.workspace || workspaceStore.currentWorkspaceId || '',
  )
  ElMessage.success('Resuming cluster')
  getList()
}

const onSsh = async (row: SlurmClusterItem) => {
  sshClusterName.value = row.name
  sshInfo.value = null
  showSshDialog.value = true
  sshLoading.value = true
  try {
    sshInfo.value = await getSlurmClusterLogin(
      clusterStore.currentClusterId ?? '',
      row.name,
      row.workspace || workspaceStore.currentWorkspaceId || '',
    )
    if (!sshInfo.value?.ready && sshInfo.value?.message) {
      ElMessage.warning(sshInfo.value.message)
    }
  } catch (e) {
    showSshDialog.value = false
    ElMessage.error('Failed to fetch the login SSH command')
  } finally {
    sshLoading.value = false
  }
}

const onDelete = (row: SlurmClusterItem) => {
  const msg = h('span', null, [
    'Are you sure you want to delete Slurm cluster: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.name),
    ' ?',
  ])
  ElMessageBox.confirm(msg, 'Delete Slurm cluster', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteSlurmCluster(
      clusterStore.currentClusterId ?? '',
      row.name,
      row.workspace || workspaceStore.currentWorkspaceId || '',
    )
    ElMessage.success('Delete completed')
    getList()
  })
}

watch(
  () => workspaceStore.currentWorkspaceId,
  () => getList(),
)

onMounted(() => {
  getList()
})

defineOptions({ name: 'SlurmClusterPage' })
</script>
