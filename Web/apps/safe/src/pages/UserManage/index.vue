<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">User Manage</el-text>
    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center">
        <el-button
          v-if="creatable"
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
        <el-button v-else-if="userStore.envs?.accountsUrl" type="primary" round @click="openIdentityAccounts">
          Manage accounts
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
      :height="'calc(100vh / var(--zoom) - 205px)'"
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
      <el-table-column
        v-if="showRoleAndWorkspaceColumns"
        prop="workspaces"
        label="Workspaces"
        :formatter="wsFormatter"
      />
      <el-table-column v-if="showRoleAndWorkspaceColumns" prop="roles" label="Roles" min-width="100">
        <template #default="{ row }">
          {{
            (Array.isArray(row.roles) ? row.roles : [row.roles]).filter(Boolean).join(', ') || '-'
          }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" :width="userStore.isManager ? 212 : 120" fixed="right">
        <template #default="{ row }">
          <template v-if="userStore.isManager">
            <el-tooltip content="QoS" placement="top">
              <el-button
                circle
                class="btn-primary-plain"
                :icon="Setting"
                size="default"
                @click="openQos(row)"
              />
            </el-tooltip>
            <el-tooltip content="Low utilization exemption" placement="top">
              <el-button
                circle
                class="btn-primary-plain"
                :icon="Odometer"
                size="default"
                @click="openLowUtilExemption(row)"
              />
            </el-tooltip>
            <el-tooltip v-if="creatable" content="Delete" placement="top">
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
              v-if="removeWsOptionsOf(row).some((ws) => !ws.disabled)"
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
            <el-tooltip v-if="addableWorkspaces(row).length" content="Add to workspace" placement="top">
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
  <QosDialog v-model:visible="qosVisible" :user="qosUser" @success="getUserList()" />
  <LowUtilExemptionDialog
    v-model:visible="lowUtilExemptionVisible"
    :user="lowUtilExemptionUser"
    @success="getUserList()"
  />
  <el-dialog v-model="addWs.visible" title="Add to workspace" width="420px" destroy-on-close>
    <el-form label-position="top" @submit.prevent>
      <el-form-item label="Workspace">
        <el-select v-model="addWs.workspaceId" placeholder="Select a workspace you manage" class="w-full">
          <el-option v-for="ws in addWsOptions" :key="ws.id" :label="ws.name" :value="ws.id" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="addWs.visible = false">Cancel</el-button>
      <el-button type="primary" :disabled="!addWs.workspaceId" @click="confirmAddToWorkspace">
        Add
      </el-button>
    </template>
  </el-dialog>
  <el-dialog v-model="removeWs.visible" title="Remove from workspace" width="420px" destroy-on-close>
    <el-form label-position="top" @submit.prevent>
      <el-form-item label="Workspaces">
        <el-select
          v-model="removeWs.workspaceIds"
          multiple
          placeholder="Select workspaces you manage"
          class="w-full"
        >
          <el-option
            v-for="ws in removeWsOptions"
            :key="ws.id"
            :label="ws.name"
            :value="ws.id"
            :disabled="ws.disabled"
          />
        </el-select>
      </el-form-item>
      <el-text v-if="removeWsOptions.some((ws) => ws.disabled)" type="info" size="small">
        Workspaces the user manages are disabled. Remove the user's management of them first.
      </el-text>
    </el-form>
    <template #footer>
      <el-button @click="removeWs.visible = false">Cancel</el-button>
      <el-button
        type="primary"
        :disabled="removeWs.workspaceIds.length === 0"
        @click="confirmRemoveFromWorkspace"
      >
        Remove
      </el-button>
    </template>
  </el-dialog>
</template>
<script lang="ts" setup>
import { ref, onMounted, reactive, h, computed } from 'vue'
import { Edit, Delete, Plus, Minus, Search, Setting, Odometer } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type TableColumnCtx } from 'element-plus'
import {
  getUserDataList,
  type UserSelfData,
  deleteUser,
  editUser,
  type EditUserResp,
} from '@/services'
import AddDialog from './Components/AddDialog.vue'
import QosDialog from './Components/QosDialog.vue'
import LowUtilExemptionDialog from './Components/LowUtilExemptionDialog.vue'
import { formatTimeStr } from '@/utils'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import type { Workspace } from '@/services'
import { useRouter } from 'vue-router'
import { openIdentityConsole } from '@/utils/identityConsole'

const searchName = ref('')
const qosVisible = ref(false)
const qosUser = ref<UserSelfData | null>(null)
const lowUtilExemptionVisible = ref(false)
const lowUtilExemptionUser = ref<UserSelfData | null>(null)
const allUserData = ref<UserSelfData[]>([])

const state = reactive({
  showDialog: false,
  curAction: '',
  curUser: {} as UserSelfData,
  rowData: [] as UserSelfData[],
})

const router = useRouter()
const userStore = useUserStore()
const openIdentityAccounts = () => {
  const url = userStore.envs?.accountsUrl
  if (url) void openIdentityConsole(userStore.envs, url)
}
// The server refuses registration and deletion when an external identity provider owns accounts.
const creatable = computed(() => !userStore.isExternalAuth)
const wsStore = useWorkspaceStore()
// Roles and workspaces of other users are shown to administrators only (platform or of the current workspace).
const showRoleAndWorkspaceColumns = computed(
  () => userStore.hasManagerAccess || wsStore.isCurrentWorkspaceAdmin(),
)
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

const openQos = (row: UserSelfData) => {
  qosUser.value = row
  qosVisible.value = true
}

const openLowUtilExemption = (row: UserSelfData) => {
  lowUtilExemptionUser.value = row
  lowUtilExemptionVisible.value = true
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

// Applies an incremental workspace change to the user and reloads the list.
const patchUserWorkspaces = async (row: UserSelfData, change: EditUserResp, doneMessage: string) => {
  await editUser(row.id, change)
  ElMessage.success(doneMessage)
  await getUserList()
}

// The workspaces the caller manages and the user is not in yet.
const addableWorkspaces = (row: UserSelfData | null): Workspace[] => {
  const joined = new Set(row?.workspaces?.map((ws) => ws.id) ?? [])
  return (userStore.profile?.managedWorkspaces ?? []).filter((ws) => !joined.has(ws.id))
}
const addWs = reactive({ visible: false, row: null as UserSelfData | null, workspaceId: '' })
const addWsOptions = computed<Workspace[]>(() => addableWorkspaces(addWs.row))

// The workspaces the caller manages that the user is in. Those the user also manages are disabled.
interface RemoveWsOption {
  id: string
  name: string
  disabled: boolean
}
const removeWsOptionsOf = (row: UserSelfData | null): RemoveWsOption[] => {
  const joined = new Set(row?.workspaces?.map((ws) => ws.id) ?? [])
  const managedByUser = new Set(row?.managedWorkspaces?.map((ws) => ws.id) ?? [])
  return (userStore.profile?.managedWorkspaces ?? [])
    .filter((ws) => joined.has(ws.id))
    .map((ws) => ({ id: ws.id, name: ws.name, disabled: managedByUser.has(ws.id) }))
}
const removeWs = reactive({ visible: false, row: null as UserSelfData | null, workspaceIds: [] as string[] })
const removeWsOptions = computed<RemoveWsOption[]>(() => removeWsOptionsOf(removeWs.row))
const onRemoveFromWorkspace = (row: UserSelfData) => {
  removeWs.row = row
  const current = wsStore.currentWorkspaceId ?? ''
  const preselect = removeWsOptionsOf(row).some((ws) => ws.id === current && !ws.disabled)
  removeWs.workspaceIds = preselect ? [current] : []
  removeWs.visible = true
}
const confirmRemoveFromWorkspace = async () => {
  const row = removeWs.row
  if (!row || removeWs.workspaceIds.length === 0) {
    return
  }
  try {
    await patchUserWorkspaces(
      row,
      { removeWorkspaces: [...removeWs.workspaceIds] },
      'Removed from workspace successfully.',
    )
    removeWs.visible = false
  } catch (err) {
    console.error(err)
    ElMessage.error('Operation failed.')
  }
}
const onAddToWorkspace = (row: UserSelfData) => {
  addWs.row = row
  const current = wsStore.currentWorkspaceId ?? ''
  addWs.workspaceId = addWsOptions.value.some((ws) => ws.id === current) ? current : ''
  addWs.visible = true
}
const confirmAddToWorkspace = async () => {
  const row = addWs.row
  if (!row || !addWs.workspaceId) {
    return
  }
  try {
    await patchUserWorkspaces(
      row,
      { addWorkspaces: [addWs.workspaceId] },
      'Added to workspace successfully.',
    )
    addWs.visible = false
  } catch (err) {
    console.error(err)
    ElMessage.error('Operation failed.')
  }
}

onMounted(() => {
  getUserList()
})
defineOptions({
  name: 'UserManagePage',
})
</script>
