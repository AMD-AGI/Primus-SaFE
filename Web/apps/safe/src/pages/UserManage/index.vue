<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">User Manage</el-text>
    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center">
        <el-button
          type="primary"
          round
          :icon="Plus"
          class="text-black"
          @click="
            () => {
              state.showDialog = true
              state.curAction = 'Create'
            }
          "
        >
          Create User
        </el-button>
      </div>
      <div class="flex flex-wrap items-center">
        <el-input
          v-model="searchName"
          placeholder="Search by name"
          clearable
          :prefix-icon="Search"
          style="max-width: 300px"
          @input="onSearchChange"
        />
      </div>
    </div>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 205px)'"
      :data="state.rowData"
      size="large"
      class="m-t-4"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="id" label="ID" />
      <el-table-column prop="name" label="Name">
        <template #default="{ row }">
          {{ row.name || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="email" label="Email">
        <template #default="{ row }">
          {{ row.email || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="type" label="Type" />
      <el-table-column prop="creationTime" label="Creation Time">
        <template #default="{ row }">
          {{ formatTimeStr(row.creationTime) }}
        </template>
      </el-table-column>
      <el-table-column prop="workspaces" label="Workspaces" :formatter="wsFormatter" />
      <el-table-column prop="roles" label="Roles" min-width="100">
        <template #default="{ row }">
          {{
            (Array.isArray(row.roles) ? row.roles : [row.roles]).filter(Boolean).join(', ') || '-'
          }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="120" fixed="right">
        <template #default="{ row }">
          <template v-if="userStore.isManager">
            <el-tooltip content="Delete" placement="top">
              <el-button
                circle
                size="default"
                class="btn-danger-plain"
                :icon="Delete"
                @click="onDelete(row.id)"
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
          <template v-else>
            <el-tooltip
              v-if="isInCurrentWorkspace(row)"
              content="Remove from workspace"
              placement="top"
            >
              <el-button
                circle
                size="default"
                class="btn-danger-plain"
                :icon="Minus"
                @click="onRemoveFromWorkspace(row)"
              />
            </el-tooltip>
            <el-tooltip v-else content="Add to workspace" placement="top">
              <el-button
                circle
                size="default"
                class="btn-primary-plain"
                :icon="Plus"
                @click="onAddToWorkspace(row)"
              />
            </el-tooltip>
          </template>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <AddDialog
    v-model:visible="state.showDialog"
    :action="state.curAction"
    :user="state.curUser"
    @success="getUserList()"
  />
</template>
<script lang="ts" setup>
import { ref, onMounted, reactive, h, computed } from 'vue'
import { Edit, Delete, Plus, Minus, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type TableColumnCtx } from 'element-plus'
import {
  getUserDataList,
  type UserSelfData,
  deleteUser,
  editUser,
  type EditUserResp,
} from '@/services'
import AddDialog from './Components/AddDialog.vue'
import { formatTimeStr } from '@/utils'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import type { Workspace } from '@/services'
import { useRouter } from 'vue-router'

const searchName = ref('')
const allUserData = ref<UserSelfData[]>([])

const state = reactive({
  showDialog: false,
  curAction: '',
  curUser: {} as UserSelfData,
  rowData: [] as UserSelfData[],
})

const router = useRouter()
const userStore = useUserStore()
const wsStore = useWorkspaceStore()
const isInCurrentWorkspace = (row: UserSelfData) => {
  return row.workspaces?.some((ws: Workspace) => ws.id === wsStore.currentWorkspaceId)
}
const loading = ref(false)

type Ws = { id: string; name: string }
const wsFormatter = (_row: any, _column: TableColumnCtx<any>, cellValue?: Ws[]) =>
  cellValue?.map((w) => w.name).join(', ') || '-'

const getUserList = async () => {
  loading.value = true
  const res = await getUserDataList()
  allUserData.value = res.items || []
  filterUsers()
  loading.value = false
}

// Filter users by search keyword
const filterUsers = () => {
  if (!searchName.value.trim()) {
    state.rowData = allUserData.value
  } else {
    const keyword = searchName.value.toLowerCase().trim()
    state.rowData = allUserData.value.filter((user) => user.name?.toLowerCase().includes(keyword))
  }
}

// Handle search input change
const onSearchChange = () => {
  filterUsers()
}

const openDialog = (row: UserSelfData) => {
  state.curAction = 'Edit'
  state.showDialog = true
  state.curUser = {
    id: row?.id,
    name: row?.name,
    roles: row?.roles,
    workspaces: row?.workspaces || [],
    managedWorkspaces: row?.managedWorkspaces || [],
    email: row?.email,
    type: row?.type,
    restrictedType: row?.restrictedType,
    creationTime: row?.creationTime,
  }
  getUserList()
}

const onDelete = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete user: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete user', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteUser(id)
    ElMessage({
      type: 'success',
      message: 'Delete completed',
    })
    getUserList()
  })
}

const handleUpdateWorkspace = async (row: UserSelfData, action: 'add' | 'remove') => {
  const currentWorkspaceId = wsStore.currentWorkspaceId
  if (!currentWorkspaceId) {
    return
  }

  const currentIds = row.workspaces?.map((ws) => ws.id) ?? []

  let nextIds: string[] = []

  if (action === 'add') {
    nextIds = [...currentIds, currentWorkspaceId]
  } else {
    if (row.managedWorkspaces?.some((ws) => ws.id === currentWorkspaceId)) {
      ElMessage.warning(`Please remove the user's workspace management first.`)
      return
    }

    nextIds = currentIds.filter((id) => id !== currentWorkspaceId)
  }

  const content =
    action === 'add'
      ? 'Add this user to the current workspace?'
      : 'Remove this user from the current workspace?'

  try {
    await ElMessageBox.confirm(content, `${action === 'add' ? 'Add' : 'Remove'} to workspace`, {
      type: 'warning',
      confirmButtonText: action === 'add' ? 'Add' : 'Remove',
      cancelButtonText: 'Cancel',
    })

    const payload: EditUserResp = {
      workspaces: nextIds,
    }

    await editUser(row.id, payload)

    ElMessage.success(
      action === 'add'
        ? 'Added to workspace successfully.'
        : 'Removed from workspace successfully.',
    )

    await getUserList()
  } catch (err) {
    console.error(err)
    if (err !== 'cancel') {
      ElMessage.error('Operation failed.')
    }
  }
}

const onRemoveFromWorkspace = (row: UserSelfData) => {
  handleUpdateWorkspace(row, 'remove')
}
const onAddToWorkspace = (row: UserSelfData) => {
  handleUpdateWorkspace(row, 'add')
}

onMounted(() => {
  getUserList()
})
defineOptions({
  name: 'UserManagePage',
})
</script>
