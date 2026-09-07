<template>
  <el-dialog
    :model-value="visible"
    title="Model Prewarm"
    width="720"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="formRef"
      :model="form"
      label-width="140px"
      class="p-5"
      :rules="rules"
    >
      <el-form-item label="Model Path" prop="modelPath">
        <el-input
          v-model="form.modelPath"
          placeholder="/shared_nfs/models/GLM-5.3"
          clearable
        />
        <div class="text-[12px] text-gray-400 mt-1">
          Required NFS absolute path on the target nodes.
        </div>
      </el-form-item>

      <el-form-item label="Generated Name">
        <el-input :model-value="generatedName" disabled />
      </el-form-item>

      <el-form-item label="Scope Type" prop="scopeType">
        <el-select v-model="form.scopeType" class="w-full" @change="onScopeTypeChange">
          <el-option label="node" value="node" />
          <el-option label="workspace" value="workspace" />
          <el-option label="cluster" value="cluster" />
        </el-select>
      </el-form-item>

      <el-form-item label="Scope Value" prop="scopeValue">
        <el-select
          v-if="form.scopeType === 'node'"
          v-model="form.scopeValue"
          multiple
          filterable
          clearable
          collapse-tags
          collapse-tags-tooltip
          :max-collapse-tags="4"
          placeholder="Select target nodes"
          class="w-full"
        >
          <el-option
            v-for="node in nodeOptions"
            :key="node.nodeId"
            :label="node.nodeId"
            :value="node.nodeId"
          />
        </el-select>
        <el-select
          v-else-if="form.scopeType === 'workspace'"
          v-model="form.scopeValue"
          filterable
          clearable
          placeholder="Select workspace"
          class="w-full"
          @change="onWorkspaceScopeChange"
        >
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
        <el-select
          v-else
          v-model="form.scopeValue"
          filterable
          clearable
          placeholder="Select cluster"
          class="w-full"
          @change="onClusterScopeChange"
        >
          <el-option
            v-for="cluster in clusterStore.items"
            :key="cluster.clusterId"
            :label="cluster.clusterId"
            :value="cluster.clusterId"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Excluded Nodes">
        <el-select
          v-model="form.excludedNodes"
          multiple
          filterable
          clearable
          collapse-tags
          collapse-tags-tooltip
          :max-collapse-tags="4"
          placeholder="Select nodes to exclude"
          class="w-full"
        >
          <el-option
            v-for="node in excludedNodeOptions"
            :key="node.nodeId"
            :label="node.nodeId"
            :value="node.nodeId"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Timeout (seconds)">
        <el-input-number v-model="form.timeoutSecond" :min="60" :max="86400" :step="60" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">Confirm</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { addOpsjobs, getNodesList } from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'
import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import {
  generateModelPrewarmName,
  type ModelPrewarmScopeType,
} from '../utils/modelPrewarm'

const props = defineProps<{
  visible: boolean
  initialModelPath?: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'success'): void
}>()

const wsStore = useWorkspaceStore()
const clusterStore = useClusterStore()
const userStore = useUserStore()
const isManager = computed(() => userStore.isManager)
const formRef = ref<FormInstance>()
const submitting = ref(false)

interface NodeOption {
  nodeId: string
}

const nodeOptions = ref<NodeOption[]>([])
const excludedNodeOptions = ref<NodeOption[]>([])

const form = reactive({
  modelPath: '',
  scopeType: 'workspace' as ModelPrewarmScopeType,
  scopeValue: '' as string | string[],
  excludedNodes: [] as string[],
  timeoutSecond: 7200,
})

const generatedName = computed(() => generateModelPrewarmName(form.modelPath || ''))

const rules: FormRules = {
  modelPath: [
    { required: true, message: 'Model path is required', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        const path = String(value || '').trim()
        if (!path.startsWith('/')) {
          callback(new Error('Model path must be an absolute NFS path'))
          return
        }
        if (path.includes('..')) {
          callback(new Error('Model path cannot contain ..'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
  scopeType: [{ required: true, message: 'Scope type is required', trigger: 'change' }],
  scopeValue: [
    {
      validator: (_rule, value, callback) => {
        const values = Array.isArray(value) ? value : value ? [value] : []
        if (!values.length) {
          callback(new Error('Scope value is required'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}

const mapNodes = (items: Array<{ nodeId: string }> = []) =>
  items.map((node) => ({ nodeId: node.nodeId }))

// System admins see all nodes when scope type is node; others are limited to current workspace.
const nodeScopeFetchParams = () =>
  isManager.value ? {} : { workspaceId: wsStore.currentWorkspaceId }

const resolveSubmitError = (err: unknown): string => {
  if (err instanceof Error) {
    const e = err as Error & { errorMessage?: string }
    return e.errorMessage || e.message || 'Failed to create model prewarm task'
  }
  if (err && typeof err === 'object') {
    const axiosErr = err as {
      response?: { data?: { errorMessage?: string; message?: string } }
      message?: string
    }
    if (axiosErr.response?.data?.errorMessage) {
      return axiosErr.response.data.errorMessage
    }
    if (axiosErr.response?.data?.message) {
      return axiosErr.response.data.message
    }
    const fields = err as Record<string, Array<{ message?: string }>>
    const firstKey = Object.keys(fields)[0]
    if (firstKey && Array.isArray(fields[firstKey])) {
      return fields[firstKey]?.[0]?.message || 'Invalid form'
    }
    if (axiosErr.message) {
      return axiosErr.message
    }
  }
  if (typeof err === 'string' && err) {
    return err
  }
  return 'Failed to create model prewarm task'
}

const fetchNodes = async (params: { workspaceId?: string; clusterId?: string } = {}) => {
  const res = await getNodesList({ ...params, limit: -1 })
  const items = mapNodes(res?.items || [])
  nodeOptions.value = items
  excludedNodeOptions.value = items
}

const onScopeTypeChange = () => {
  form.scopeValue = form.scopeType === 'node' ? [] : ''
  form.excludedNodes = []
  if (form.scopeType === 'workspace' && wsStore.currentWorkspaceId) {
    form.scopeValue = wsStore.currentWorkspaceId
    fetchNodes({ workspaceId: wsStore.currentWorkspaceId })
  } else if (form.scopeType === 'cluster' && clusterStore.currentClusterId) {
    form.scopeValue = clusterStore.currentClusterId
    fetchNodes({ clusterId: clusterStore.currentClusterId })
  } else if (form.scopeType === 'node') {
    fetchNodes(nodeScopeFetchParams())
  }
}

const onWorkspaceScopeChange = (workspaceId: string) => {
  form.excludedNodes = []
  if (workspaceId) fetchNodes({ workspaceId })
}

const onClusterScopeChange = (clusterId: string) => {
  form.excludedNodes = []
  if (clusterId) fetchNodes({ clusterId })
}

const onOpen = async () => {
  formRef.value?.resetFields()
  form.modelPath = props.initialModelPath || ''
  form.scopeType = 'workspace'
  form.scopeValue = wsStore.currentWorkspaceId || ''
  form.excludedNodes = []
  form.timeoutSecond = 7200

  if (!wsStore.items?.length) {
    await wsStore.fetchWorkspace()
  }
  if (!clusterStore.isFetched) {
    await clusterStore.fetchClusters()
  }
  if (form.scopeType === 'workspace' && form.scopeValue) {
    await fetchNodes({ workspaceId: String(form.scopeValue) })
  } else if (form.scopeType === 'cluster' && clusterStore.currentClusterId) {
    form.scopeValue = clusterStore.currentClusterId
    await fetchNodes({ clusterId: clusterStore.currentClusterId })
  } else {
    await fetchNodes({ workspaceId: wsStore.currentWorkspaceId })
  }
}

const onSubmit = async () => {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
    submitting.value = true

    const scopeValues = Array.isArray(form.scopeValue)
      ? form.scopeValue
      : [String(form.scopeValue)]
    const inputs = [
      { name: 'model.path', value: form.modelPath.trim() },
      ...scopeValues.filter(Boolean).map((value) => ({
        name: form.scopeType,
        value: String(value),
      })),
    ]

    const payload: Record<string, unknown> = {
      name: generatedName.value,
      type: 'model-prewarm',
      inputs,
    }
    if (form.timeoutSecond > 0) {
      payload.timeoutSecond = form.timeoutSecond
    }
    if (form.excludedNodes.length) {
      payload.excludedNodes = form.excludedNodes
    }

    await addOpsjobs(payload as any, { skipErrorHandler: true })
    ElMessage.success('Model prewarm task created successfully')
    emit('update:visible', false)
    emit('success')
  } catch (err) {
    ElMessage.error(resolveSubmitError(err))
  } finally {
    submitting.value = false
  }
}
</script>
