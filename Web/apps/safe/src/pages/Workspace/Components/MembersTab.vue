<template>
  <div v-loading="loading" :element-loading-text="$loadingText">
    <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
      <div class="flex items-center gap-2">
        <el-button v-if="canEdit" type="primary" plain :icon="Plus" @click="bindVisible = true">
          Bind team
        </el-button>
      </div>
      <div v-if="isAdmin" class="flex items-center gap-2">
        <span class="text-[13px] text-gray-400">Access policy</span>
        <el-select
          :model-value="policy"
          size="default"
          style="width: 140px"
          :loading="policySaving"
          @change="onPolicyChange"
        >
          <el-option label="closed" value="closed" />
          <el-option label="approval" value="approval" />
        </el-select>
      </div>
    </div>

    <h4 class="section-title">Users</h4>
    <el-table :data="members.users" size="default" empty-text="No directly granted users">
      <el-table-column label="Name" min-width="160">
        <template #default="{ row }">{{ row.name || row.preferredName || '-' }}</template>
      </el-table-column>
      <el-table-column prop="email" label="Email" min-width="200">
        <template #default="{ row }">{{ row.email || '-' }}</template>
      </el-table-column>
      <el-table-column prop="id" label="ID" min-width="200" />
      <el-table-column prop="role" label="Role" width="100" />
      <el-table-column label="Source" min-width="140">
        <template #default="{ row }">
          <el-tag size="small" type="info">{{ sourceLabel(row.source) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column v-if="canEdit" label="Actions" width="100" fixed="right">
        <template #default="{ row }">
          <el-tooltip
            :content="row.role === 'manager' ? 'Remove the user from the managers first' : 'Remove'"
            placement="top"
          >
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              :disabled="row.role === 'manager'"
              @click="onRemoveUser(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <h4 class="section-title">Teams</h4>
    <el-table
      :data="members.teams"
      row-key="id"
      size="default"
      empty-text="No bound teams"
      @expand-change="onExpand"
    >
      <el-table-column type="expand">
        <template #default="{ row }">
          <div class="px-6 py-2">
            <el-text v-if="teamState[row.id]?.loading" type="info">Loading members...</el-text>
            <el-text v-else-if="teamState[row.id]?.forbidden" type="info">
              Member count: {{ row.memberCount ?? '-' }}. Only members of this team can see its
              members.
            </el-text>
            <el-text v-else-if="teamState[row.id]?.error" type="warning">
              {{ teamState[row.id].error }}
            </el-text>
            <el-table
              v-else
              :data="teamState[row.id]?.items ?? []"
              size="small"
              empty-text="No members"
            >
              <el-table-column label="Name" min-width="140">
                <template #default="{ row: m }">{{ m.name || m.preferredName || '-' }}</template>
              </el-table-column>
              <el-table-column prop="email" label="Email" min-width="200" />
              <el-table-column prop="userId" label="ID" min-width="200" />
              <el-table-column prop="role" label="Team role" width="110" />
            </el-table>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Team" min-width="200">
        <template #default="{ row }">
          <el-link
            v-if="teamLink(row.id)"
            type="primary"
            :href="teamLink(row.id)"
            target="_blank"
            rel="noopener"
          >
            {{ row.displayName || row.id }}
          </el-link>
          <span v-else>{{ row.displayName || row.id }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="id" label="ID" min-width="160" />
      <el-table-column prop="role" label="Role" width="100" />
      <el-table-column label="Members" width="100">
        <template #default="{ row }">{{ row.memberCount ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="Managers" min-width="200">
        <template #default="{ row }">{{ managerNames(row) }}</template>
      </el-table-column>
      <el-table-column v-if="canEdit" label="Actions" width="100" fixed="right">
        <template #default="{ row }">
          <el-tooltip
            :content="
              isAdmin || row.role === 'member'
                ? 'Unbind'
                : 'Only a system administrator can unbind a manager team'
            "
            placement="top"
          >
            <el-button
              circle
              size="default"
              class="btn-danger-plain"
              :icon="Delete"
              :disabled="!isAdmin && row.role !== 'member'"
              @click="onUnbindTeam(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <h4 class="section-title">Groups</h4>
    <el-table :data="members.groups" size="default" empty-text="No platform groups">
      <el-table-column prop="name" label="Group" min-width="200" />
      <el-table-column prop="role" label="Role" width="100" />
      <el-table-column label="Source" min-width="160">
        <template #default="{ row }">
          <el-tag size="small" type="info">{{ sourceLabel(row.source) }}</el-tag>
        </template>
      </el-table-column>
    </el-table>
  </div>

  <TeamBindDialog
    v-model:visible="bindVisible"
    :workspace-id="workspaceId"
    :bound="boundRoles"
    @success="loadMembers"
  />
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus } from '@element-plus/icons-vue'
import {
  editUser,
  getTeamMembers,
  getWorkspaceMembers,
  patchWorkspaceAccessPolicy,
  updateWorkspaceTeams,
  type WorkspaceAccessPolicy,
  type WorkspaceMemberTeam,
  type WorkspaceMemberUser,
  type WorkspaceMembersResp,
  type WorkspaceTeamRole,
} from '@/services'
import { useUserStore } from '@/stores/user'
import { useTeams } from '@/composables/useTeams'
import { teamUrl } from '@/utils/identityConsole'
import TeamBindDialog from './TeamBindDialog.vue'
import { loadTeamMembers, refreshTeamMembers, type TeamStateMap } from './teamMembersCache'

const props = defineProps<{
  workspaceId: string
  accessPolicy?: WorkspaceAccessPolicy
}>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const userStore = useUserStore()
const { sourceLabel } = useTeams()

// System administrators manage every binding and the access policy.
const isAdmin = computed(() => userStore.isManager)
// A workspace manager may manage member-role bindings and directly granted users.
const canEdit = computed(
  () =>
    isAdmin.value ||
    (userStore.profile?.managedWorkspaces ?? []).some((ws) => ws.id === props.workspaceId),
)

const loading = ref(false)
const members = reactive<WorkspaceMembersResp>({ users: [], teams: [], groups: [] })
const bindVisible = ref(false)

const policy = ref<WorkspaceAccessPolicy>(props.accessPolicy || 'closed')
const policySaving = ref(false)
watch(
  () => props.accessPolicy,
  (v) => {
    policy.value = v || 'closed'
  },
)

const boundRoles = computed(
  () =>
    Object.fromEntries(members.teams.map((t) => [t.id, t.role])) as Record<
      string,
      WorkspaceTeamRole
    >,
)

// Link to the team page of the identity console, empty when no console address is set.
const teamLink = (id: string) => teamUrl(userStore.envs?.identityConsoleUrl, id)

// Display names of the managers of a bound team.
const managerNames = (team: WorkspaceMemberTeam) =>
  (team.managers ?? []).map((m) => m.preferredName || m.name || m.email || m.id).join(', ') || '-'

// Member lists of expanded teams, loaded on first expansion.
const teamState = reactive<TeamStateMap>({})
// IDs of the team rows that are expanded.
const expandedTeamIds = ref<string[]>([])

// Loads the effective members of the workspace and reloads the member lists of expanded teams.
const loadMembers = async () => {
  loading.value = true
  try {
    const res = await getWorkspaceMembers(props.workspaceId)
    members.users = res?.users ?? []
    members.teams = res?.teams ?? []
    members.groups = res?.groups ?? []
    const boundIds = members.teams.map((t) => t.id)
    expandedTeamIds.value = expandedTeamIds.value.filter((id) => boundIds.includes(id))
    void refreshTeamMembers(teamState, boundIds, expandedTeamIds.value, getTeamMembers)
  } finally {
    loading.value = false
  }
}

// Records the expanded rows and lazily loads the members of a team when its row is expanded.
const onExpand = (row: WorkspaceMemberTeam, expandedRows: WorkspaceMemberTeam[]) => {
  expandedTeamIds.value = expandedRows.map((r) => r.id)
  if (expandedTeamIds.value.includes(row.id))
    void loadTeamMembers(teamState, row.id, getTeamMembers)
}

// Asks for confirmation. Resolves false when the user cancels.
const confirmAction = (message: string, title: string, confirmButtonText: string) =>
  ElMessageBox.confirm(message, title, {
    confirmButtonText,
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(
    () => true,
    () => false,
  )

// Removes a directly granted user from the workspace.
const onRemoveUser = async (user: WorkspaceMemberUser) => {
  if (
    !(await confirmAction(
      `Remove user ${user.name || user.id} from this workspace?`,
      'Remove user',
      'Remove',
    ))
  )
    return
  try {
    await editUser(user.id, { removeWorkspaces: [props.workspaceId] })
    ElMessage.success('Removed from workspace successfully.')
    await loadMembers()
  } catch (err) {
    console.error(err)
  }
}

// Removes the binding of a team. The team itself and its members are untouched.
const onUnbindTeam = async (team: WorkspaceMemberTeam) => {
  const label = team.displayName || team.id
  if (!(await confirmAction(`Unbind team ${label} from this workspace?`, 'Unbind team', 'Unbind')))
    return
  try {
    await updateWorkspaceTeams(props.workspaceId, { remove: [team.id] })
    ElMessage.success('Team unbound successfully.')
    await loadMembers()
  } catch (err) {
    console.error(err)
  }
}

// Saves a new access policy. System administrators only.
const onPolicyChange = async (value: WorkspaceAccessPolicy) => {
  policySaving.value = true
  try {
    await patchWorkspaceAccessPolicy(props.workspaceId, value)
    policy.value = value
    ElMessage.success('Access policy updated.')
    emit('changed')
  } catch (err) {
    console.error(err)
  } finally {
    policySaving.value = false
  }
}

onMounted(() => {
  void loadMembers()
})
</script>

<style scoped>
.section-title {
  margin: 16px 0 8px;
  font-size: 14px;
  font-weight: 600;
}
</style>
