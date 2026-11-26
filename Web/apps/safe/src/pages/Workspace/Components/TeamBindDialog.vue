<template>
  <el-dialog
    :model-value="visible"
    title="Bind team"
    width="480px"
    destroy-on-close
    @update:model-value="(v: boolean) => emit('update:visible', v)"
    @open="onOpen"
  >
    <el-form label-position="top" @submit.prevent>
      <el-form-item label="Team">
        <el-select
          v-if="teams.length"
          v-model="teamId"
          filterable
          placeholder="Search by name or id"
          class="w-full"
          :loading="loading"
        >
          <el-option
            v-for="t in teams"
            :key="t.id"
            :value="t.id"
            :label="t.displayName || t.id"
            :disabled="isLocked(t.id)"
          >
            <span>{{ t.displayName || t.id }}</span>
            <span class="ml-2 text-[13px] text-gray-400"
              >{{ t.id }} · {{ t.memberCount }} members</span
            >
          </el-option>
        </el-select>
        <template v-else>
          <el-input v-model.trim="teamId" placeholder="Team id" :disabled="loading" />
          <el-text v-if="teamId && !idValid" type="danger" size="small">
            Use lowercase letters, digits and '-', up to 63 characters.
          </el-text>
        </template>
      </el-form-item>
      <el-form-item label="Role">
        <el-radio-group v-model="role">
          <el-radio-button value="member">Member</el-radio-button>
          <el-radio-button v-if="isAdmin" value="manager">Manager</el-radio-button>
        </el-radio-group>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:visible', false)">Cancel</el-button>
      <el-button
        type="primary"
        :disabled="!teamId || !idValid || isLocked(teamId)"
        :loading="saving"
        @click="onSubmit"
      >
        Bind
      </el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { updateWorkspaceTeams, type WorkspaceTeamRole } from '@/services'
import { useTeams } from '@/composables/useTeams'

const props = defineProps<{
  visible: boolean
  workspaceId: string
  // True for a system administrator, who may bind with the manager role
  isAdmin: boolean
  // Roles of the teams already bound to the workspace, by team id
  bound: Record<string, WorkspaceTeamRole>
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'success'): void
}>()

const { teams, loading, loadTeams } = useTeams()
const teamId = ref('')
const role = ref<WorkspaceTeamRole>('member')
const saving = ref(false)

// A team id is a DNS-1123 label.
const idValid = computed(
  () => /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(teamId.value) && teamId.value.length <= 63,
)

// A workspace manager may not change a team that is bound with the manager role.
const isLocked = (id: string) => !props.isAdmin && props.bound[id] === 'manager'

// Resets the form and loads the team list.
const onOpen = async () => {
  teamId.value = ''
  role.value = 'member'
  await loadTeams()
}

// Binds the team, or changes its role when it is already bound.
const onSubmit = async () => {
  saving.value = true
  try {
    await updateWorkspaceTeams(props.workspaceId, {
      add: [{ name: teamId.value, role: role.value }],
    })
    ElMessage.success('Team bound successfully.')
    emit('update:visible', false)
    emit('success')
  } finally {
    saving.value = false
  }
}
</script>
