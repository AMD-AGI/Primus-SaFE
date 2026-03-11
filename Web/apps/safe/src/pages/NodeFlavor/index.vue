<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">NodeFlavors</el-text>
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
      Create NodeFlavor
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
      <el-table-column prop="flavorId" label="ID" min-width="120" :fixed="true">
        <template #default="{ row }">
          <el-link type="primary" @click="openDialog('Detail', row)">{{ row.flavorId }}</el-link>
        </template>
      </el-table-column>
      <el-table-column prop="memory" label="Memory" min-width="100" />
      <el-table-column prop="cpu" label="CPU" min-width="100">
        <template #default="{ row }">
          {{ row.cpu?.quantity ?? '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="amd.com/gpu" label="GPU" min-width="100">
        <template #default="{ row }">
          {{ row.gpu?.quantity ?? '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="ephemeral-storage" label="ephemeral-storage" min-width="100">
        <template #default="{ row }">
          {{ row.extendedResources?.['ephemeral-storage'] ?? '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="rdma/hca" label="rdma/hca" min-width="100">
        <template #default="{ row }">
          {{ row.extendedResources?.['rdma/hca'] ?? '-' }}
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
              @click="onDelete(row.flavorId)"
            />
          </el-tooltip>
          <el-tooltip content="Edit" placement="top">
            <el-button
              circle
              class="btn-primary-plain"
              :icon="Edit"
              size="default"
              @click="openDialog('Edit', row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <AddDialog
    v-model:visible="state.showDialog"
    :action="state.curAction"
    :flavor="state.curFlavor"
    @success="fetchData()"
  />
</template>
<script lang="ts" setup>
import { ref, onMounted, reactive, h } from 'vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { getNodeFlavors, type FlavorOptionsType, deleteNodeFlavor } from '@/services'
import AddDialog from './Components/AddDialog.vue'
import { Edit, Delete, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '@/stores/user'
// import FlavorDetail from './FlavorDetail.vue'
// import { useFlavorStore } from '@/stores/flavor'
// import { useRouter } from 'vue-router'

// const router = useRouter()
// const store = useFlavorStore()

dayjs.extend(utc)

const userStore = useUserStore()
const loading = ref(false)
const tableData = ref([] as FlavorOptionsType[])

const state = reactive({
  showDialog: false,
  curAction: '',
  curFlavor: {} as FlavorOptionsType,
})

// const jumpToDetail = (item) => {
//   store.set(item)
//   router.push({ name: 'nodeFlavorDetail', params: { id: item.flavorId } })
// }

const fetchData = async () => {
  try {
    loading.value = true

    const res = await getNodeFlavors()
    tableData.value = res?.items || []
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}

const onDelete = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete nodeflavor: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete nodeflavor', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteNodeFlavor(id)
    ElMessage({
      type: 'success',
      message: 'Delete completed',
    })
    fetchData()
  })
}

const openDialog = (action: string, row: FlavorOptionsType) => {
  state.curAction = action
  state.showDialog = true
  state.curFlavor = row
}

onMounted(() => {
  fetchData()
})

defineOptions({
  name: 'nodeFlavorPage',
})
</script>
<style scoped></style>
