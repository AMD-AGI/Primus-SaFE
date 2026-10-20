<template>
  <el-drawer
    :model-value="visible"
    :title="`${action} SGLangRouter`"
    :close-on-click-modal="false"
    size="860px"
    destroy-on-close
    direction="rtl"
    append-to-body
    @open="onOpen"
    @update:model-value="(open: boolean) => emit('update:visible', open)"
  >
    <el-form ref="formRef" :model="form" label-width="140px" :rules="rules">
      <div class="section-title">Basic</div>
      <el-form-item label="Name" prop="displayName">
        <el-input v-model="form.displayName" :disabled="isEdit || isResume" placeholder="Display name" />
      </el-form-item>
      <el-form-item label="Description">
        <el-input v-model="form.description" type="textarea" :rows="2" />
      </el-form-item>
      <el-form-item label="Priority">
        <el-select v-model="form.priority">
          <el-option label="Low" :value="0" />
          <el-option label="Medium" :value="1" />
          <el-option label="High" :value="2" />
        </el-select>
      </el-form-item>
      <el-form-item label="Service port">
        <el-input-number v-model="form.servicePort" :min="1" :max="65535" />
        <span class="hint">Router target port stays 8000. Prefill 8010 and decode 8020 are internal.</span>
      </el-form-item>
      <el-form-item label="Service type">
        <el-select v-model="form.serviceType">
          <el-option label="ClusterIP" value="ClusterIP" />
          <el-option label="NodePort" value="NodePort" />
        </el-select>
      </el-form-item>
      <el-form-item v-if="form.serviceType === 'NodePort'" label="Node port" prop="serviceNodePort" :rules="nodePortRule">
        <el-input-number v-model="form.serviceNodePort" :min="1" :max="65535" />
      </el-form-item>

      <div v-for="role in SGLANG_ROUTER_ROLES" :key="role" class="role-card">
        <div class="section-title">{{ role }}</div>
        <el-form-item :label="'Image'" :prop="`roles.${role}.image`" :rules="requiredRule('Image is required')">
          <ImageInput v-model="form.roles[role].image" />
        </el-form-item>
        <el-form-item
          label="Entrypoint"
          :prop="`roles.${role}.entryPoint`"
          :rules="requiredRule('Entrypoint is required')"
        >
          <el-input v-model="form.roles[role].entryPoint" type="textarea" :rows="3" />
          <div v-if="role === 'router'" class="hint block">
            {prefill} and {decode} are replaced with Service URLs. Do not use atomesh.
          </div>
        </el-form-item>
        <el-form-item label="Replica">
          <el-input-number v-model="form.roles[role].replica" :min="1" />
        </el-form-item>
        <el-form-item label="CPU">
          <el-input v-model="form.roles[role].cpu" />
        </el-form-item>
        <el-form-item label="GPU">
          <el-input v-model="form.roles[role].gpu" placeholder="Empty for CPU" />
        </el-form-item>
        <el-form-item label="GPU name">
          <el-input v-model="form.roles[role].gpuName" placeholder="amd.com/gpu" />
        </el-form-item>
        <el-form-item label="RDMA">
          <el-input v-model="form.roles[role].rdmaResource" placeholder="1k" />
        </el-form-item>
        <el-form-item label="Memory (Gi)">
          <el-input v-model="form.roles[role].memory" />
        </el-form-item>
        <el-form-item label="Shared memory (Gi)">
          <el-input v-model="form.roles[role].sharedMemory" />
        </el-form-item>
        <el-form-item label="Ephemeral (Gi)">
          <el-input v-model="form.roles[role].ephemeralStorage" />
        </el-form-item>
      </div>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:visible', false)">Cancel</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">Confirm</el-button>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormItemRule } from 'element-plus'
import ImageInput from '@/components/Base/ImageInput.vue'
import { addWorkload, editWorkload, getWorkloadDetail, resumeWorkload } from '@/services/workload/index'
import { decodeFromBase64String, quantityToGi } from '@/utils'
import { useWorkspaceStore } from '@/stores/workspace'
import { SGLANG_ROUTER_ROLES, buildSGLangRouterWorkloadBody, initialSGLangRouterForm, type SGLangRouterRole } from '../sglangRouterPayload'

const props = defineProps<{
  visible: boolean
  wlid?: string
  action: string
  limitedEdit?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'success'): void
}>()

const store = useWorkspaceStore()
const formRef = ref<FormInstance>()
const form = reactive(initialSGLangRouterForm())
const submitting = ref(false)
const isEdit = computed(() => props.action === 'Edit')
const isResume = computed(() => props.action === 'Resume')

const rules = {
  displayName: [{ required: true, message: 'Name is required', trigger: 'blur' }],
}

function requiredRule(message: string): FormItemRule {
  return { required: true, message, trigger: 'blur' }
}

const nodePortRule: FormItemRule = {
  validator: (_rule, value, callback) => {
    if (form.serviceType === 'NodePort' && !(Number(value) > 0)) {
      callback(new Error('Node port is required'))
      return
    }
    callback()
  },
  trigger: 'change',
}

/** Fill the three role slots from an existing SGLangRouter workload. */
async function onOpen() {
  Object.assign(form, initialSGLangRouterForm())
  if (!props.wlid) return
  const res = await getWorkloadDetail(props.wlid)
  form.displayName = res.displayName || ''
  form.description = res.description || ''
  form.priority = res.priority ?? 1
  form.servicePort = res.service?.port || 8000
  form.serviceType = res.service?.serviceType || 'ClusterIP'
  form.serviceNodePort = Number(res.service?.nodePort) > 0 ? Number(res.service?.nodePort) : null
  form.serviceName = props.action === 'Clone' ? '' : String(res.service?.name || '')
  SGLANG_ROUTER_ROLES.forEach((role: SGLangRouterRole, index) => {
    const resource = res.resources?.[index] || {}
    form.roles[role] = {
      image: res.images?.[index] || '',
      entryPoint: decodeFromBase64String(res.entryPoints?.[index] || ''),
      replica: Number(resource.replica) || 1,
      cpu: resource.cpu != null ? String(resource.cpu) : '2',
      gpu: resource.gpu != null ? String(resource.gpu) : '',
      memory: quantityToGi(resource.memory) || '4',
      ephemeralStorage: quantityToGi(resource.ephemeralStorage) || '20',
      sharedMemory: quantityToGi(resource.sharedMemory),
      gpuName: resource.gpuName || '',
      rdmaResource: resource.rdmaResource || '',
    }
  })
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  submitting.value = true
  try {
    const body = buildSGLangRouterWorkloadBody(form)
    if (isEdit.value && props.wlid) {
      await editWorkload(props.wlid, {
        description: body.description,
        priority: body.priority,
        images: body.images,
        entryPoints: body.entryPoints,
        resources: body.resources,
        service: body.service,
      })
    } else if (isResume.value && props.wlid) {
      await resumeWorkload(props.wlid, {
        ...body,
        workspace: store.currentWorkspaceId,
      })
    } else {
      await addWorkload({
        ...body,
        workspace: store.currentWorkspaceId,
      })
    }
    ElMessage.success(`${props.action} submitted`)
    emit('update:visible', false)
    emit('success')
  } catch {
    // The request layer reports the API error.
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.section-title {
  margin: 12px 0;
  font-weight: 600;
}
.role-card {
  margin-top: 8px;
  padding-top: 4px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.hint {
  margin-left: 12px;
  color: var(--el-text-color-secondary);
}
.hint.block {
  display: block;
  margin-left: 0;
  margin-top: 6px;
  font-size: 12px;
}
</style>
