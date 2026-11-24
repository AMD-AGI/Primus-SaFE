<template>
  <el-dialog
    :model-value="visible"
    :title="`Low utilization exemption · ${user?.name || user?.id || ''}`"
    width="640px"
    @open="onOpen"
    @close="emit('update:visible', false)"
  >
    <el-form label-position="top" @submit.prevent>
      <el-form-item label="Exempt workspaces">
        <el-select
          v-model="selected"
          multiple
          filterable
          clearable
          placeholder="Select workspaces"
          class="w-full"
        >
          <el-option v-for="item in options" :key="item.id" :label="item.name" :value="item.id" />
        </el-select>
      </el-form-item>
      <el-text type="info" size="small">
        The user's workloads in these workspaces are not checked by the workspace low utilization
        policy.
      </el-text>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:visible', false)">Cancel</el-button>
      <el-button type="primary" :loading="saving" @click="onSave">Save</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { editUser, type UserSelfData } from '@/services'
import { buildUserLowUtilization, exemptWorkspaceIds } from './lowUtilExemption'

const props = defineProps<{ visible: boolean; user: UserSelfData | null }>()
const emit = defineEmits(['update:visible', 'success'])

const workspaceStore = useWorkspaceStore()

const saving = ref(false)
const selected = ref<string[]>([])

/** options lists the known workspaces, plus stored ones that are no longer listed so they can be removed. */
const options = computed(() => {
  const items = workspaceStore.items
    .filter((item) => item.workspaceId)
    .map((item) => ({ id: item.workspaceId, name: item.workspaceName || item.workspaceId }))
  for (const id of selected.value) {
    if (!items.some((item) => item.id === id)) items.push({ id, name: id })
  }
  return items
})

/** onOpen loads the user's exempt workspaces into the form. */
const onOpen = async () => {
  if (!workspaceStore.isFetched) await workspaceStore.fetchWorkspace()
  selected.value = exemptWorkspaceIds(props.user?.lowUtilization)
}

/** onSave writes this user's exemptions. Other user fields stay unchanged. */
const onSave = async () => {
  if (!props.user?.id) return
  saving.value = true
  try {
    await editUser(props.user.id, { lowUtilization: buildUserLowUtilization(selected.value) })
    ElMessage.success('Saved')
    emit('update:visible', false)
    emit('success')
  } finally {
    saving.value = false
  }
}
</script>
