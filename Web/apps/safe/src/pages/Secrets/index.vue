<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Secrets</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="
        () => {
          state.showDialog = true
          state.curAction = 'Create'
          state.curId = ''
        }
      "
    >
      Create Secret
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
      <!-- <el-table-column prop="secretId" label="ID" min-width="120" :fixed="true" /> -->
      <el-table-column prop="secretName" label="Secret Name" min-width="100">
        <template #default="{ row }">
          {{ row.secretName || '-' }}
        </template>
      </el-table-column>

      <el-table-column
        prop="type"
        label="Type"
        min-width="100"
        :filters="[
          { text: 'ssh', value: 'ssh' },
          { text: 'image', value: 'image' },
          { text: 'general', value: 'general' },
        ]"
        :filter-method="filterType"
        :filter-multiple="false"
        column-key="type"
      >
        <template #default="{ row }">
          <el-tag
            :type="row.type === 'ssh' ? 'success' : row.type === 'image' ? 'primary' : 'warning'"
            :effect="isDark ? 'plain' : 'light'"
            >{{ row.type }}</el-tag
          >
        </template>
      </el-table-column>

      <el-table-column prop="userName" label="User Name" min-width="120">
        <template #default="{ row }">
          {{ row.userName || '-' }}
        </template>
      </el-table-column>

      <el-table-column prop="creationTime" label="Creation Time" width="180">
        <template #default="{ row: subRow }">
          {{ formatTimeStr(subRow.creationTime) }}
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
              :disabled="row.type === 'ssh' ? !userStore.isManager : false"
              @click="onDelete(row.secretId)"
            />
          </el-tooltip>
          <el-tooltip content="Edit" placement="top">
            <el-button
              circle
              class="btn-primary-plain"
              :icon="Edit"
              size="default"
              :disabled="row.type === 'ssh' ? !userStore.isManager : false"
              @click="openDialog(row.secretId)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <AddDialog
    v-model:visible="state.showDialog"
    :id="state.curId"
    :action="state.curAction"
    @success="fetchData()"
  />
</template>
<script lang="ts" setup>
import { ref, onMounted, reactive, h } from 'vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { getSecrets, deleteSecret } from '@/services'
import AddDialog from './Components/AddDialog.vue'
import { Delete, Edit, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDark } from '@vueuse/core'
import { useUserStore } from '@/stores/user'
import { formatTimeStr } from '@/utils'

dayjs.extend(utc)

const isDark = useDark()
const userStore = useUserStore()
const loading = ref(false)
const tableData = ref([] as any[])

const state = reactive({
  showDialog: false,
  curAction: '',
  curId: '',
})

const filterType = (value: string, row: any) => row.type === value

const fetchData = async () => {
  try {
    loading.value = true

    const res = await getSecrets({ type: 'ssh,image,general' })
    tableData.value = res?.items || []
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}

const openDialog = (id: string) => {
  state.curAction = 'Edit'
  state.showDialog = true
  state.curId = id
}

const onDelete = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete secret: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete secret', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteSecret(id)
    ElMessage({
      type: 'success',
      message: 'Delete completed',
    })
    // Add 500ms delay to ensure backend data sync completes
    setTimeout(() => {
      fetchData()
    }, 500)
  })
}

onMounted(() => {
  fetchData()
})

defineOptions({
  name: 'sshSecretsPage',
})
</script>
<style scoped></style>
