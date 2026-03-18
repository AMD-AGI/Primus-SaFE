<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Images Registries</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 color-black"
      @click="
        () => {
          state.showDialog = true
          state.curAction = 'Create'
        }
      "
    >
      Create Registry
    </el-button>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 205px)'"
      :data="state.rowData"
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
            <el-link type="primary" class="pointer-events-none">{{ row.name }}</el-link>
            <div class="text-[13px] text-gray-400">
              {{ row.id }}
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition"
                size="11"
                @click="copyText(row.id)"
              >
                <CopyDocument />
              </el-icon>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="default" label="Default" min-width="120" />
      <el-table-column prop="created_at" label="Created At" min-width="120">
        <template #default="{ row }">
          {{ row.created_at ? dayjs(row.created_at * 1000).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="updated_at" label="Updated At" min-width="120">
        <template #default="{ row }">
          {{ row.updated_at ? dayjs(row.updated_at * 1000).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="url" label="URL" min-width="120" />
      <el-table-column prop="username" label="User Name" min-width="120" />
      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="Edit" placement="top">
            <el-button
              circle
              class="btn-primary-plain"
              :icon="Edit"
              size="default"
              @click="
                () => {
                  state.curAction = 'Edit'
                  state.showDialog = true
                  state.curReg = {
                    id: row.id,
                    name: row.name,
                    url: row.url,
                    username: row.username,
                  }
                }
              "
            />
          </el-tooltip>
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
  </el-card>

  <AddDialog
    v-model:visible="state.showDialog"
    :regData="state.curReg"
    :action="state.curAction"
    @success="getRegs()"
  />
</template>

<script lang="ts" setup>
import { ref, onMounted, h, reactive } from 'vue'
import { deleteImageReg, getImageRegList } from '@/services'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, CopyDocument, Plus, Edit } from '@element-plus/icons-vue'
import { copyText } from '@/utils/index'
import { useUserStore } from '@/stores/user'

import AddDialog from './Components/AddDialog.vue'
import { useDark } from '@vueuse/core'
import dayjs from 'dayjs'

const isDark = useDark()
const userStore = useUserStore()

const loading = ref(false)

const state = reactive({
  showDialog: false,
  curAction: '',
  rowData: [] as any[],
  curReg: {
    id: 0,
    name: '',
    username: '',
    url: '',
  },
})

const onDelete = (id: string, name: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete Registry: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, name),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete Image Registry', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteImageReg(id)
      ElMessage({
        type: 'success',
        message: 'Delete completed',
      })
      getRegs()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const getRegs = async () => {
  loading.value = true
  const res = await getImageRegList({})
  state.rowData = res || []
  loading.value = false
}

onMounted(() => {
  getRegs()
})

defineOptions({
  name: 'ImageRegistriesPage',
})
</script>

<style scoped>
.expand-inner-table {
  width: 100%;
  box-sizing: border-box;
}
</style>
