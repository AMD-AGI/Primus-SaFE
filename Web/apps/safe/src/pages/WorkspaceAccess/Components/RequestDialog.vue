<template>
  <el-dialog
    :model-value="visible"
    :title="created ? 'Request submitted' : 'Request workspace access'"
    width="520px"
    destroy-on-close
    @update:model-value="(v: boolean) => emit('update:visible', v)"
    @open="onOpen"
  >
    <template v-if="!created">
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="Workspace">
          <span>{{ workspace?.workspaceName || workspace?.workspaceId }}</span>
        </el-form-item>
        <el-form-item label="Reason (optional)">
          <el-input
            v-model="reason"
            type="textarea"
            :rows="4"
            maxlength="1000"
            show-word-limit
            placeholder="What you need the workspace for"
          />
        </el-form-item>
      </el-form>
      <el-text type="info" size="small">
        You are asking for the member role. The managers of the workspace are notified.
      </el-text>
    </template>
    <template v-else>
      <ApprovalLink :link="link" />
    </template>
    <template #footer>
      <template v-if="!created">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="saving" :disabled="!workspace" @click="onSubmit">
          Submit
        </el-button>
      </template>
      <el-button v-else type="primary" @click="emit('update:visible', false)">Done</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'
import {
  createWorkspaceAccessRequest,
  type CatalogWorkspace,
  type WorkspaceAccessRequest,
} from '@/services'
import { approvalLink } from '@/utils/workspaceAccess'
import ApprovalLink from './ApprovalLink.vue'

const props = defineProps<{
  visible: boolean
  workspace: CatalogWorkspace | null
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'submitted', req: WorkspaceAccessRequest): void
}>()

const reason = ref('')
const saving = ref(false)
const created = ref<WorkspaceAccessRequest | null>(null)

const link = computed(() => (created.value ? approvalLink(created.value, location.origin) : ''))

// Resets the form each time the dialog opens.
const onOpen = () => {
  reason.value = ''
  created.value = null
}

// Creates the request and switches the dialog to its approval link.
const onSubmit = async () => {
  if (!props.workspace) return
  saving.value = true
  try {
    const req = await createWorkspaceAccessRequest({
      workspace: props.workspace.workspaceId,
      reason: reason.value.trim(),
    })
    created.value = req
    emit('submitted', req)
  } finally {
    saving.value = false
  }
}
</script>
