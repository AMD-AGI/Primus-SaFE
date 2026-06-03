<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} Dynamo`"
    :close-on-click-modal="false"
    size="860px"
    destroy-on-close
    direction="rtl"
    append-to-body
    class="dynamo-drawer"
    @open="onOpen"
    @close="emit('update:visible', false)"
  >
    <div class="drawer-body" v-loading="loading">
      <el-form
        ref="ruleFormRef"
        :model="form"
        :rules="rules"
        label-width="140px"
        :validate-on-rule-change="false"
      >
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Basic Information</div>
              <div class="section-subtitle">Name, image and priority</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="16">
              <el-form-item label="name" prop="displayName">
                <el-input v-model="form.displayName" :disabled="isEdit" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="priority">
                <el-select v-model="form.priority" placeholder="priority">
                  <el-option label="Low" :value="0" />
                  <el-option label="Medium" :value="1" />
                  <el-option label="High" :value="2" v-if="isManager || store.isCurrentWorkspaceAdmin()" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="description">
            <el-input v-model="form.description" type="textarea" :rows="2" />
          </el-form-item>
          <el-form-item label="image" prop="image">
            <ImageInput v-model="form.image" />
          </el-form-item>
        </div>

        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Dynamo Mode</div>
              <div class="section-subtitle">Choose PD disaggregation or aggregation mode</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="PD">
                <el-switch v-model="form.enablePd" active-text="Enabled" inactive-text="Disabled" />
              </el-form-item>
            </el-col>
            <el-col :span="12" v-if="form.enablePd">
              <el-form-item label="KV Backend">
                <el-select v-model="form.kvTransferBackend">
                  <el-option label="nixl" value="nixl" />
                  <el-option label="mooncake" value="mooncake" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-alert
            :closable="false"
            type="info"
            show-icon
            :title="modeHint"
          />
        </div>

        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Resources</div>
              <div class="section-subtitle">Configure backend role resources</div>
            </div>
          </div>

          <template v-for="(section, index) in resourceSections" :key="section.key">
            <el-divider v-if="index > 0" />
            <div class="resource-title-row">
              <div>
                <div class="resource-title">{{ section.title }}</div>
                <el-text
                  v-if="shouldShowRoleAggregationTip(section.key)"
                  size="small"
                  type="warning"
                  class="block mt-1"
                >
                  TP size is greater than 8. Enable Aggregation for this role if it should span nodes.
                </el-text>
              </div>
              <div class="resource-aggregation">
                <span class="text-sm text-[var(--el-text-color-regular)]">Aggregation</span>
                <el-switch
                  :model-value="isRoleAggregated(section.key)"
                  :disabled="isRoleAggregationDisabled(section.key)"
                  @update:model-value="(value: boolean) => setRoleAggregation(section.key, value)"
                />
              </div>
            </div>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="replicas" :prop="`${section.key}.replica`">
                  <el-input-number
                    v-model="section.resource.replica"
                    :min="1"
                    controls-position="right"
                    class="w-full"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="cpu" :prop="`${section.key}.cpu`">
                  <el-input v-model="section.resource.cpu" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="gpu">
                  <el-input v-model="section.resource.gpu" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="memory" :prop="`${section.key}.memory`">
                  <el-input v-model="section.resource.memory" placeholder="256">
                    <template #append>Gi</template>
                  </el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="rdmaResource">
                  <el-input v-model="section.resource.rdmaResource" placeholder="Leave empty if not needed" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="sharedMemory">
                  <el-input v-model="section.resource.sharedMemory" placeholder="200">
                    <template #append>Gi</template>
                  </el-input>
                </el-form-item>
              </el-col>
            </el-row>
          </template>
        </div>

        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Entrypoint Preview</div>
              <div class="section-subtitle">These commands will be base64 encoded before submit</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="24">
              <el-form-item label="modelPath" prop="modelPath">
                <el-input v-model="form.modelPath" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="backend">
                <el-select v-model="form.backendEngine">
                  <el-option label="sglang" value="sglang" />
                  <el-option label="vllm" value="vllm" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="tpSize" prop="worker.tpSize">
                <el-input-number v-model="form.worker.tpSize" :min="1" controls-position="right" class="w-full" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="epSize" prop="worker.epSize">
                <el-input-number v-model="form.worker.epSize" :min="1" controls-position="right" class="w-full" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="memFraction" prop="memFractionStatic">
                <el-input v-model="form.memFractionStatic" />
              </el-form-item>
            </el-col>
          </el-row>

          <pre class="entry-preview"><template
              v-for="(token, index) in backendPreviewTokens"
              :key="`${index}-${token.text}`"
            ><span :class="{ 'entry-preview-token--editable': token.editable }">{{ token.text }}</span></template></pre>
        </div>

        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Environment</div>
              <div class="section-subtitle">Additional environment variables for Dynamo roles</div>
            </div>
          </div>

          <el-form-item label="env vars" class="kv-full">
            <KeyValueList v-model="envList" :max="20" keyMode="input" info="Add up to 20 envs" />
          </el-form-item>
        </div>
      </el-form>
    </div>

    <template #footer>
      <div class="drawer-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit(ruleFormRef)">
          {{ isEdit ? 'Save' : props.action }}
        </el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import { computed, reactive, ref, watch } from 'vue'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import ImageInput from '@/components/Base/ImageInput.vue'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import { addWorkload, editWorkload, getWorkloadDetail } from '@/services/workload'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import {
  convertKeyValueMapToList,
  convertListToKeyValueMap,
  decodeFromBase64String,
} from '@/utils'
import {
  DYNAMO_SERVICE,
  buildDynamoEntrypointPreviewTokens,
  buildDynamoCreatePayload,
  buildDynamoWorkerEntrypoint,
  createDefaultDynamoForm,
  getDynamoDefaultTpSize,
  type DynamoBackendEngine,
  type DynamoEntrypointPreviewToken,
  type DynamoFormModel,
  type DynamoKvTransferBackend,
  type DynamoPdAggregationRole,
  type DynamoRoleResourceForm,
} from '../dynamoPayload'

const props = defineProps<{
  visible: boolean
  wlid?: string
  action: string
}>()

const emit = defineEmits(['update:visible', 'success'])

const store = useWorkspaceStore()
const userStore = useUserStore()
const isManager = computed(() => userStore.isManager)
const isEdit = computed(() => props.action === 'Edit')

const ruleFormRef = ref<FormInstance>()
const form = reactive<DynamoFormModel>(createDefaultDynamoForm())
const envList = ref(convertKeyValueMapToList(form.env))
const loading = ref(false)
const submitting = ref(false)

const nameRegex = /^[a-z](?:[-a-z0-9]{0,38}[a-z0-9])?$/
const required = (message: string) => ({ required: true, message, trigger: 'blur' })

const rules = reactive<FormRules>({
  displayName: [
    required('Please input name'),
    {
      pattern: nameRegex,
      message: 'Must start with lowercase letter, only a-z, 0-9, and "-" allowed, max 40 chars',
      trigger: 'blur',
    },
  ],
  image: [required('Please input image')],
  modelPath: [required('Please input model path')],
  memFractionStatic: [required('Please input mem fraction')],
  'worker.tpSize': [
    required('Please input TP size'),
    {
      validator: (_rule, value, callback) => {
        if (form.enableAggregation && Number(value) <= 8) {
          callback(new Error('Aggregation TP size must be greater than 8'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
  'worker.replica': [required('Please input replica')],
  'worker.cpu': [required('Please input cpu')],
  'worker.memory': [required('Please input memory')],
  'prefill.replica': [required('Please input replica')],
  'prefill.cpu': [required('Please input cpu')],
  'prefill.memory': [required('Please input memory')],
  'decode.replica': [required('Please input replica')],
  'decode.cpu': [required('Please input cpu')],
  'decode.memory': [required('Please input memory')],
  'service.port': [required('Please input service port')],
  'service.targetPort': [required('Please input target port')],
})

interface DynamoDetail {
  workloadId?: string
  displayName?: string
  description?: string
  priority?: number
  image?: string
  images?: string[]
  env?: Record<string, string>
  entryPoints?: string[]
  resources?: Array<Partial<DynamoRoleResourceForm>>
  service?: Partial<DynamoFormModel['service']>
  dynamoOptions?: {
    serviceRoles?: string[]
    multinodeRoles?: string[]
    kvTransferBackend?: DynamoKvTransferBackend
  }
}

const backendPreview = computed(() => buildDynamoWorkerEntrypoint(form, form.worker))
const backendPreviewTokens = computed<DynamoEntrypointPreviewToken[]>(() =>
  buildDynamoEntrypointPreviewTokens(backendPreview.value),
)

const modeHint = computed(() => {
  if (form.enablePd) {
    return 'PD mode submits frontend, prefill and decode roles. Aggregation can target prefill, decode, or both when their replicas are greater than 1.'
  }
  if (form.enableAggregation) {
    return 'Aggregation mode marks worker as multinodeRoles. TP size defaults to GPU x replica and must be greater than 8.'
  }
  return 'Non-PD mode submits frontend and worker roles. Replica > 1 creates independent worker replicas unless Aggregation is enabled.'
})

const resourceSections = computed(() => {
  if (form.enablePd) {
    return [
      { key: 'prefill', title: 'Prefill', resource: form.prefill },
      { key: 'decode', title: 'Decode', resource: form.decode },
    ]
  }
  return [{ key: 'worker', title: 'Worker', resource: form.worker }]
})

const shouldShowRoleAggregationTip = (role: string) => {
  return Number(form.worker.tpSize || 0) > 8 && !isRoleAggregated(role)
}

const isRoleAggregated = (role: string) => {
  if (form.enablePd) {
    return form.enableAggregation && form.pdAggregationRoles.includes(role as DynamoPdAggregationRole)
  }
  return role === 'worker' && form.enableAggregation
}

const isRoleAggregationDisabled = (role: string) => {
  if (role === 'worker') return Number(form.worker.replica || 0) <= 1
  if (role === 'prefill') return Number(form.prefill.replica || 0) <= 1
  if (role === 'decode') return Number(form.decode.replica || 0) <= 1
  return true
}

const setRoleAggregation = (role: string, enabled: boolean) => {
  if (isRoleAggregationDisabled(role)) return

  if (!form.enablePd) {
    form.enableAggregation = role === 'worker' && enabled
    return
  }

  const pdRole = role as DynamoPdAggregationRole
  const nextRoles = new Set(form.pdAggregationRoles)
  if (enabled) nextRoles.add(pdRole)
  else nextRoles.delete(pdRole)
  form.pdAggregationRoles = Array.from(nextRoles).filter(
    (item): item is DynamoPdAggregationRole => item === 'prefill' || item === 'decode',
  )
  form.enableAggregation = form.pdAggregationRoles.length > 0
}

watch(
  () => [form.worker.replica, form.enablePd] as const,
  ([replica, enablePd]) => {
    if (!enablePd && Number(replica) <= 1) form.enableAggregation = false
  },
)

watch(
  () => [form.prefill.replica, form.decode.replica, form.enablePd, form.enableAggregation] as const,
  () => {
    if (!form.enablePd || !form.enableAggregation) return
    form.pdAggregationRoles = form.pdAggregationRoles.filter((role) => {
      return Number(form[role].replica || 0) > 1
    })
    form.enableAggregation = form.pdAggregationRoles.length > 0
  },
)

watch(
  () => [form.enableAggregation, form.worker.replica, form.worker.gpu] as const,
  ([enabled]) => {
    if (!enabled) return
    const nextTpSize = getDynamoDefaultTpSize(form.worker)
    form.worker.tpSize = nextTpSize
    form.worker.epSize = nextTpSize
  },
)

const onOpen = async () => {
  resetForm()
  if (!props.wlid) return
  loading.value = true
  try {
    const detail = await getWorkloadDetail(props.wlid)
    hydrateFormFromDetail(detail)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to load workload detail')
  } finally {
    loading.value = false
  }
}

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl || !store.currentWorkspaceId || submitting.value) return
  try {
    await formEl.validate()
    submitting.value = true
    form.env = convertListToKeyValueMap(envList.value)

    const payload = buildDynamoCreatePayload(form, store.currentWorkspaceId)
    if (isEdit.value) {
      if (!props.wlid) return
      const { workspaceId: _workspaceId, displayName: _displayName, groupVersionKind: _gvk, ...editPayload } = payload
      await editWorkload(props.wlid, editPayload)
      ElMessage.success('Edit successful')
    } else {
      await addWorkload(payload)
      ElMessage.success(`${props.action} successful`)
    }
    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err instanceof Error) ElMessage.error(err.message)
  } finally {
    submitting.value = false
  }
}

function resetForm() {
  const next = createDefaultDynamoForm()
  Object.assign(form, next)
  envList.value = convertKeyValueMapToList(next.env)
}

function hydrateFormFromDetail(detail: DynamoDetail) {
  const next = createDefaultDynamoForm()
  next.displayName =
    props.action === 'Clone'
      ? `${String(detail.displayName || detail.workloadId || 'dynamo')}-clone`.slice(0, 40)
      : String(detail.displayName || '')
  next.description = String(detail.description || '')
  next.priority = Number(detail.priority ?? next.priority)
  next.image = detail.images?.[0] || detail.image || next.image
  next.env = detail.env || next.env

  const dynamoOptions = detail.dynamoOptions || {}
  const serviceRoles = dynamoOptions.serviceRoles || inferServiceRoles(detail.resources)
  const multinodeRoles = dynamoOptions.multinodeRoles || []
  next.enablePd = serviceRoles.includes('prefill') && serviceRoles.includes('decode')
  next.enableAggregation = multinodeRoles.length > 0
  next.pdAggregationRoles = multinodeRoles.filter(
    (role): role is DynamoPdAggregationRole => role === 'prefill' || role === 'decode',
  )
  next.kvTransferBackend = dynamoOptions.kvTransferBackend || next.kvTransferBackend

  const entryPoint = detail.entryPoints?.[1] ? decodeFromBase64String(detail.entryPoints[1]) : ''
  applyEntrypointToForm(next, entryPoint)

  next.service = { ...DYNAMO_SERVICE }

  if (next.enablePd) {
    next.prefill = mergeResource(next.prefill, detail.resources?.[1])
    next.decode = mergeResource(next.decode, detail.resources?.[2])
  } else {
    next.worker = mergeResource(next.worker, detail.resources?.[1])
  }

  Object.assign(form, next)
  envList.value = convertKeyValueMapToList(next.env)
}

function inferServiceRoles(resources?: unknown[]) {
  return Array.isArray(resources) && resources.length === 3
    ? ['frontend', 'prefill', 'decode']
    : ['frontend', 'worker']
}

function mergeResource(
  base: DynamoRoleResourceForm,
  source?: Partial<DynamoRoleResourceForm>,
): DynamoRoleResourceForm {
  if (!source) return base
  return {
    ...base,
    replica: Number(source.replica ?? base.replica),
    cpu: String(source.cpu ?? base.cpu),
    gpu: source.gpu ?? base.gpu,
    memory: stripGi(source.memory ?? base.memory),
    sharedMemory: source.sharedMemory ? stripGi(source.sharedMemory) : base.sharedMemory,
    rdmaResource: source.rdmaResource ?? base.rdmaResource,
  }
}

function stripGi(value: string) {
  return String(value).replace(/Gi$/i, '')
}

function applyEntrypointToForm(target: DynamoFormModel, command: string) {
  if (!command) return
  target.backendEngine = readBackendEngine(command) || target.backendEngine
  target.modelPath = readFlag(command, '--model-path') || target.modelPath
  target.worker.tpSize = Number(readFlag(command, '--tp-size') || target.worker.tpSize)
  target.worker.epSize = Number(readFlag(command, '--ep-size') || target.worker.epSize)
  target.attentionBackend = readFlag(command, '--attention-backend') || target.attentionBackend
  target.memFractionStatic = readFlag(command, '--mem-fraction-static') || target.memFractionStatic
  target.kvTransferBackend =
    (readFlag(command, '--disaggregation-transfer-backend') as DynamoFormModel['kvTransferBackend']) ||
    target.kvTransferBackend
}

function readFlag(command: string, flag: string) {
  const match = command.match(new RegExp(`${flag}\\s+([^\\s]+)`))
  return match?.[1] || ''
}

function readBackendEngine(command: string): DynamoBackendEngine | '' {
  const match = command.match(/\bdynamo\.(sglang|vllm)\b/)
  return (match?.[1] as DynamoBackendEngine | undefined) || ''
}

</script>

<style scoped>
.drawer-body {
  max-height: calc(100vh - 140px);
  overflow-y: auto;
  padding-right: 4px;
}

.section-card {
  background: var(--el-bg-color-overlay);
  border-radius: 10px;
  padding: 14px 16px 10px;
  margin-bottom: 20px;
  border: 1px solid var(--el-border-color-lighter);
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.08),
    0 1px 3px rgba(0, 0, 0, 0.04);
}

html.dark .section-card {
  border: 1px solid rgba(255, 255, 255, 0.03);
  box-shadow:
    0 12px 35px rgba(0, 0, 0, 0.55),
    0 0 0 1px rgba(0, 0, 0, 0.7);
}

.section-card:hover {
  box-shadow:
    0 4px 12px rgba(0, 0, 0, 0.12),
    0 2px 6px rgba(0, 0, 0, 0.06);
  transform: translateY(-1px);
  transition: all 0.16s ease-out;
}

html.dark .section-card:hover {
  box-shadow:
    0 14px 40px rgba(0, 0, 0, 0.55),
    0 0 1px rgba(0, 0, 0, 0.9);
}

.section-header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 10px;
}

.section-bar {
  width: 4px;
  height: 18px;
  border-radius: 999px;
  margin-top: 2px;
  background-color: var(--safe-primary);
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  line-height: 1.2;
}

.section-subtitle {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.resource-title {
  font-size: calc(14px * var(--scale, 1));
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.resource-title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 10px;
}

.resource-aggregation {
  display: flex;
  align-items: center;
  gap: 10px;
  white-space: nowrap;
}

.fixed-role {
  margin-bottom: 8px;
}

.entry-preview {
  white-space: pre-wrap;
  word-break: break-word;
  padding: 12px;
  margin: 0;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
}

.entry-preview-token--editable {
  color: var(--safe-primary);
  font-weight: 700;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 10px 24px;
}

.kv-full {
  margin-bottom: 8px;
}

.kv-full :deep(.key-value-list-root) {
  width: 100%;
}
</style>
