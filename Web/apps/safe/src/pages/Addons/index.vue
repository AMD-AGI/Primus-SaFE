<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Addons</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="
        () => {
          state.showDialog = true
          state.curAction = 'Create'
        }
      "
    >
      Create Addon
    </el-button>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 215px)'"
      :data="state.rowData"
      size="large"
      class="m-t-4"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="releaseName" label="Name/Release Name">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <el-link type="primary" class="pointer-events-none">{{ row.name }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.releaseName }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.releaseName)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="namespace" label="Namespace">
        <template #default="{ row }">
          {{ row.namespace || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="status" label="Status">
        <template #default="{ row }">
          <el-tag :type="row.status?.status === 'deployed' ? 'success' : 'danger'">
            {{ row.status?.status || row?.phase || '-' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="cluster" label="Cluster">
        <template #default="{ row }">
          {{ row.cluster || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="template" label="Template">
        <template #default="{ row }">
          {{ row.template || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time">
        <template #default="{ row }">
          {{ formatTimeStr(row.creationTime) }}
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
              @click="onDelete(row)"
            />
          </el-tooltip>
          <el-tooltip content="Edit" placement="top">
            <el-button
              circle
              class="btn-primary-plain"
              :icon="Edit"
              size="default"
              @click="openDialog(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <DeployDialog
    v-model:visible="state.showDialog"
    :action="state.curAction"
    :name="state.curName"
    @success="getAddonList"
  />
</template>
<script lang="ts" setup>
import { ref, onMounted, reactive, h } from 'vue'
import { Edit, Delete, Plus, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getAddonsList, type AddonsData, deleteAddon } from '@/services'
import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import DeployDialog from '../AddonTemp/Components/DeployDialog.vue'
import { copyText, formatTimeStr } from '@/utils'
// import AddDialog from './Components/AddDialog.vue'

const state = reactive({
  showDialog: false,
  curAction: '',
  curName: '',
  rowData: [] as AddonsData[],
})

const clusterStore = useClusterStore()
const userStore = useUserStore()
const loading = ref(false)

const getAddonList = async () => {
  loading.value = true
  const res = await getAddonsList(clusterStore.currentClusterId ?? '')
  state.rowData = res.items || []
  loading.value = false
}

const openDialog = (row: AddonsData) => {
  state.curAction = 'Edit'
  state.showDialog = true
  state.curName = row.name
  getAddonList()
}

const onDelete = (row: AddonsData) => {
  const msg = h('span', null, [
    'Are you sure you want to delete addon: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.name),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete addon', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteAddon(row.cluster, row.name)
    ElMessage({
      type: 'success',
      message: 'Delete completed',
    })
    getAddonList()
  })
}

onMounted(() => {
  getAddonList()
})
defineOptions({
  name: 'AddonsPage',
})
</script>
