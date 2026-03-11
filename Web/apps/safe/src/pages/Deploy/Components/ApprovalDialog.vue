<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="Approve Deployment Request"
    width="600px"
    :close-on-click-modal="false"
  >
    <el-form ref="formRef" :model="form" label-width="120px" :rules="rules">
      <el-form-item label="Deployment ID">
        <el-text>{{ deploymentData?.id }}</el-text>
      </el-form-item>

      <el-form-item label="Deploy Name">
        <el-text>{{ deploymentData?.deploy_name || '-' }}</el-text>
      </el-form-item>

      <el-form-item label="Description">
        <el-text>{{ deploymentData?.description || '-' }}</el-text>
      </el-form-item>

      <el-form-item label="Created At">
        <el-text>{{ formatTimeStr(deploymentData?.created_at) }}</el-text>
      </el-form-item>

      <el-form-item label="Action" prop="action">
        <el-radio-group v-model="form.action">
          <el-radio value="approve">Approve</el-radio>
          <el-radio value="reject">Reject</el-radio>
        </el-radio-group>
      </el-form-item>

      <el-form-item v-if="form.action === 'reject'" label="Reject Reason" prop="rejectReason">
        <el-input
          v-model="form.rejectReason"
          type="textarea"
          :rows="4"
          placeholder="Please provide a reason for rejection"
        />
      </el-form-item>

      <el-alert
        v-if="form.action === 'approve'"
        type="warning"
        :closable="false"
        show-icon
        class="mb-4"
        title="Warning: Approving will trigger the deployment process automatically."
      />
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:visible', false)">Cancel</el-button>
      <el-button
        :type="form.action === 'approve' ? 'primary' : 'danger'"
        @click="handleSubmit"
        :loading="loading"
      >
        {{ form.action === 'approve' ? 'Approve' : 'Reject' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { approveDeployment } from '@/services/deploy'
import { formatTimeStr } from '@/utils'
import type { DeploymentRequest } from '@/services/deploy/type'

interface Props {
  visible: boolean
  deploymentData: DeploymentRequest | null
}

const props = defineProps<Props>()
const emit = defineEmits(['update:visible', 'success'])

const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  action: 'approve' as 'approve' | 'reject',
  rejectReason: '',
})

const rules: FormRules = {
  action: [{ required: true, message: 'Please select an action', trigger: 'change' }],
  rejectReason: [
    {
      required: true,
      message: 'Please provide a reason for rejection',
      trigger: 'blur',
      validator: (rule, value, callback) => {
        if (form.action === 'reject' && !value) {
          callback(new Error('Please provide a reason for rejection'))
        } else {
          callback()
        }
      },
    },
  ],
}

const handleSubmit = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
    loading.value = true

    const payload: {
      approved: boolean
      reason?: string
    } = {
      approved: form.action === 'approve',
    }

    if (form.action === 'reject' && form.rejectReason) {
      payload.reason = form.rejectReason
    }

    if (!props.deploymentData?.id) return

    await approveDeployment(props.deploymentData.id.toString(), payload)

    ElMessage.success(
      form.action === 'approve'
        ? 'Deployment approved successfully'
        : 'Deployment rejected successfully',
    )
    emit('update:visible', false)
    emit('success')
  } catch (error) {
    console.error('Failed to approve deployment:', error)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      form.action = 'approve'
      form.rejectReason = ''
      formRef.value?.clearValidate()
    }
  },
)
</script>
