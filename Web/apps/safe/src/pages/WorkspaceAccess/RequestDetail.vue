<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Workspace access request</el-text>
    <el-card
      v-loading="loading"
      :element-loading-text="$loadingText"
      class="safe-card mt-4"
      shadow="never"
    >
      <el-result
        v-if="state === 'forbidden'"
        icon="warning"
        title="You are not an approver of this request"
        sub-title="Only the requester, the managers of the workspace and system administrators can open it."
      >
        <template #extra>
          <el-button @click="router.push('/workspace-access')">Request workspace access</el-button>
        </template>
      </el-result>
      <el-result v-else-if="state === 'error'" icon="error" title="Failed to load the request">
        <template #extra>
          <el-button type="primary" @click="load">Retry</el-button>
        </template>
      </el-result>
      <template v-else-if="req">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="Requester">
            <span>{{ req.userName || req.userId }}</span>
            <span v-if="req.userEmail" class="ml-2 text-gray-400">{{ req.userEmail }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="Workspace">
            {{ req.workspaceName || req.workspace }}
            <span v-if="req.workspaceName" class="ml-2 text-gray-400">{{ req.workspace }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="Role">member</el-descriptions-item>
          <el-descriptions-item label="Reason">
            <span class="whitespace-pre-line">{{ req.reason || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="Creation Time">
            {{ formatTimeStr(req.creationTime) }}
          </el-descriptions-item>
          <el-descriptions-item label="Status">
            <el-tag :type="requestStatusTag(req.status)">{{
              requestStatusLabel(req.status)
            }}</el-tag>
          </el-descriptions-item>
          <template v-if="!pending">
            <el-descriptions-item label="Decided by">
              {{ req.decidedByName || req.decidedBy || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Decision comment">
              <span class="whitespace-pre-line">{{ req.decisionComment || '-' }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="Decided at">
              {{ formatTimeStr(req.decidedAt) }}
            </el-descriptions-item>
          </template>
        </el-descriptions>

        <div v-if="pending && req.canDecide" class="mt-6">
          <el-form label-position="top" @submit.prevent>
            <el-form-item label="Comment (optional)">
              <el-input
                v-model="comment"
                type="textarea"
                :rows="3"
                maxlength="1000"
                show-word-limit
                placeholder="Shown to the requester"
              />
            </el-form-item>
          </el-form>
          <div class="flex gap-3">
            <el-button
              type="primary"
              :loading="deciding === 'approve'"
              :disabled="!!deciding"
              @click="decide('approve')"
            >
              Approve
            </el-button>
            <el-button
              type="danger"
              plain
              :loading="deciding === 'reject'"
              :disabled="!!deciding"
              @click="decide('reject')"
            >
              Reject
            </el-button>
          </div>
        </div>

        <div v-else-if="pending && req.isRequester" class="mt-6">
          <ApprovalLink :link="link" />
          <el-button class="mt-4" :loading="cancelling" @click="onCancel">Cancel request</el-button>
        </div>

        <el-text v-else-if="pending" type="info" class="block mt-6">
          Waiting for a manager of the workspace to decide.
        </el-text>
      </template>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  approveWorkspaceAccessRequest,
  cancelWorkspaceAccessRequest,
  getWorkspaceAccessRequest,
  rejectWorkspaceAccessRequest,
  type WorkspaceAccessRequestDetail,
} from '@/services'
import { formatTimeStr } from '@/utils'
import { statusOf } from '@/utils/httpStatus'
import {
  approvalLink,
  isPendingRequest,
  requestStatusLabel,
  requestStatusTag,
} from '@/utils/workspaceAccess'
import ApprovalLink from './Components/ApprovalLink.vue'

const route = useRoute()
const router = useRouter()

const id = computed(() => String(route.params.id ?? ''))
const loading = ref(false)
// loaded: the request is shown; forbidden: the server refused it (403); error: any other failure
const state = ref<'loaded' | 'forbidden' | 'error' | ''>('')
const req = ref<WorkspaceAccessRequestDetail | null>(null)
const comment = ref('')
const deciding = ref<'' | 'approve' | 'reject'>('')
const cancelling = ref(false)

const pending = computed(() => isPendingRequest(req.value?.status))
const link = computed(() => (req.value ? approvalLink(req.value, location.origin) : ''))

// Loads the request. A 403 means the caller may not see it; nothing of the request is kept then.
const load = async () => {
  if (!id.value) return
  loading.value = true
  try {
    req.value = await getWorkspaceAccessRequest(id.value)
    state.value = 'loaded'
  } catch (err) {
    req.value = null
    state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
  } finally {
    loading.value = false
  }
}

// Approves or rejects the request with the optional comment, then shows its new state.
const decide = async (action: 'approve' | 'reject') => {
  if (!req.value) return
  deciding.value = action
  try {
    const call = action === 'approve' ? approveWorkspaceAccessRequest : rejectWorkspaceAccessRequest
    await call(req.value.id, comment.value.trim())
    ElMessage.success(action === 'approve' ? 'Request approved.' : 'Request rejected.')
  } catch (err) {
    console.error(err)
  } finally {
    deciding.value = ''
  }
  await load()
}

// Withdraws the caller's own pending request after confirmation.
const onCancel = async () => {
  if (!req.value) return
  const ok = await ElMessageBox.confirm('Cancel this request?', 'Cancel request', {
    confirmButtonText: 'Cancel request',
    cancelButtonText: 'Keep',
    type: 'warning',
  }).then(
    () => true,
    () => false,
  )
  if (!ok) return
  cancelling.value = true
  try {
    await cancelWorkspaceAccessRequest(req.value.id)
    ElMessage.success('Request cancelled.')
  } catch (err) {
    console.error(err)
  } finally {
    cancelling.value = false
  }
  await load()
}

watch(
  id,
  () => {
    comment.value = ''
    void load()
  },
  { immediate: true },
)

defineOptions({
  name: 'WorkspaceAccessRequestPage',
})
</script>
