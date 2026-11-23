<template>
  <el-dialog
    :model-value="visible"
    :title="`QoS · ${user?.name || user?.id || ''}`"
    width="640px"
    @open="onOpen"
    @close="emit('update:visible', false)"
  >
    <el-text v-if="!workspaces.length" type="info">No workspace.</el-text>
    <el-radio-group v-if="workspaces.length > 1" v-model="target" class="mb-6">
      <el-radio-button v-for="item in workspaces" :key="item.workspaceId" :value="item.workspaceId">
        {{ item.workspaceName || item.workspaceId }}
      </el-radio-button>
    </el-radio-group>
    <QosScopeForm v-if="drafts[target]" v-model="drafts[target]" />
    <template #footer>
      <el-button @click="emit('update:visible', false)">Cancel</el-button>
      <el-button type="primary" :loading="saving" @click="onSave">Save</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { editUser, type UserSelfData } from '@/services'
import {
  draftToScope,
  emptyDraft,
  scopeToDraft,
  type ScopeDraft,
  type UserQos,
} from '@/services/qos/type'
import QosScopeForm from '@/pages/Qos/QosScopeForm.vue'

const props = defineProps<{ visible: boolean; user: UserSelfData | null }>()
const emit = defineEmits(['update:visible', 'success'])

const workspaceStore = useWorkspaceStore()
const workspaces = computed(() => workspaceStore.items.filter((item) => item.workspaceId))

const saving = ref(false)
const target = ref('')
const drafts = reactive<Record<string, ScopeDraft>>({})

/** resetDrafts loads the user's per-workspace ceilings into the form. */
const resetDrafts = () => {
  for (const key of Object.keys(drafts)) delete drafts[key]
  const qos = props.user?.qos
  for (const [workspaceId, scope] of Object.entries(qos?.workspaces ?? {})) {
    drafts[workspaceId] = scopeToDraft(scope)
  }
  for (const item of workspaces.value) {
    if (!drafts[item.workspaceId]) drafts[item.workspaceId] = emptyDraft()
  }
  const ids = workspaces.value.map((item) => item.workspaceId)
  const current = workspaceStore.currentWorkspaceId ?? ''
  target.value = ids.includes(current) ? current : (ids[0] ?? '')
}

/** onOpen prepares the form when the dialog is shown. */
const onOpen = async () => {
  if (!workspaceStore.isFetched) await workspaceStore.fetchWorkspace()
  resetDrafts()
}

/** buildQos keeps only per-workspace ceilings that set a value. */
const buildQos = (): UserQos => {
  const workspaces: UserQos['workspaces'] = {}
  for (const [name, draft] of Object.entries(drafts)) {
    const scope = draftToScope(draft)
    if (scope) workspaces[name] = scope
  }
  return Object.keys(workspaces).length ? { workspaces } : {}
}

/** onSave writes this user's ceilings. Other user fields stay unchanged. */
const onSave = async () => {
  if (!props.user?.id) return
  saving.value = true
  try {
    await editUser(props.user.id, { qos: buildQos() })
    ElMessage.success('Saved')
    emit('update:visible', false)
    emit('success')
  } finally {
    saving.value = false
  }
}
</script>
