<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} ${workloadLabel}`"
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
          <div class="section-header section-header--split">
            <div class="section-bar"></div>
            <div class="section-header-content">
              <div>
                <div class="section-title">{{ isOptimus ? 'Role Configuration' : 'Resources' }}</div>
                <div class="section-subtitle">
                  {{ isOptimus ? 'Configure each role resources and entrypoint together' : 'Configure role resources' }}
                </div>
              </div>
              <el-segmented v-model="modeValue" :options="['Default', 'PD']" />
            </div>
          </div>

          <template v-if="isOptimus">
            <el-form-item label="modelPath" prop="modelPath">
              <el-input v-model="form.modelPath" />
            </el-form-item>

            <div
              v-for="section in optimusRoleSections"
              :key="section.key"
              class="optimus-role-card"
            >
              <div class="resource-title-row">
                <div>
                  <div class="resource-title">{{ section.title }}</div>
                  <el-text
                    v-if="section.key !== 'frontend' && getAggregationWarning(section.key)"
                    size="small"
                    type="warning"
                    class="block mt-1"
                  >
                    {{ getAggregationWarning(section.key) }}
                  </el-text>
                </div>
                <div v-if="section.key !== 'frontend'" class="resource-aggregation">
                  <span class="text-sm text-[var(--el-text-color-regular)]">Aggregation</span>
                  <el-tooltip :content="getAggregationHint(section.key)" placement="top">
                    <el-icon class="aggregation-info"><InfoFilled /></el-icon>
                  </el-tooltip>
                  <el-switch
                    :model-value="isRoleAggregated(section.key)"
                    :disabled="isRoleAggregationDisabled(section.key)"
                    @update:model-value="(value: boolean) => setRoleAggregation(section.key, value)"
                  />
                </div>
              </div>

              <div class="role-subsection-title">Resources</div>
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
                <el-col v-if="section.key !== 'frontend'" :span="12">
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
                <el-col v-if="section.key !== 'frontend'" :span="12">
                  <el-form-item label="rdmaResource">
                    <el-input v-model="section.resource.rdmaResource" placeholder="Leave empty if not needed" />
                  </el-form-item>
                </el-col>
                <el-col v-if="section.key !== 'frontend'" :span="12">
                  <el-form-item label="sharedMemory">
                    <el-input v-model="section.resource.sharedMemory" placeholder="200">
                      <template #append>Gi</template>
                    </el-input>
                  </el-form-item>
                </el-col>
              </el-row>

              <template v-if="section.key === 'frontend'">
                <div class="role-subsection-title">Entrypoint</div>
                <el-form-item label="routerPolicy">
                  <el-select v-model="form.routerPolicy">
                    <el-option label="kv-aware" value="kv-aware" />
                    <el-option label="round-robin" value="round-robin" />
                  </el-select>
                </el-form-item>
                <el-input
                  :model-value="frontendPreview"
                  type="textarea"
                  readonly
                  :autosize="{ minRows: 3, maxRows: 6 }"
                  class="entry-editor"
                />
              </template>

              <template v-else>
                <div class="role-subsection-header">
                  <div>
                    <div class="role-subsection-title">EntryPoint Parameters</div>
                    <el-text size="small" type="info">
                      Edit parameters here, or edit the full command below.
                    </el-text>
                  </div>
                  <el-button size="small" @click="resetRoleEntrypointFromOptions(section.key)">
                    Reset from options
                  </el-button>
                </div>
                <el-row :gutter="16">
                  <el-col :span="12">
                    <el-form-item label="backend">
                      <el-select v-model="form.backendEngine">
                        <el-option label="sglang" value="sglang" />
                        <el-option label="vllm" value="vllm" />
                      </el-select>
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item label="tpSize" :prop="`${section.key}.tpSize`">
                      <el-input-number
                        v-model="section.resource.tpSize"
                        :min="1"
                        controls-position="right"
                        class="w-full"
                      />
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item label="epSize" :prop="`${section.key}.epSize`">
                      <el-input-number
                        v-model="section.resource.epSize"
                        :min="1"
                        controls-position="right"
                        class="w-full"
                      />
                    </el-form-item>
                  </el-col>
                </el-row>
                <el-input
                  :model-value="getRoleEntrypoint(section.key)"
                  type="textarea"
                  :autosize="{ minRows: 4, maxRows: 8 }"
                  class="entry-editor"
                  @input="(value: string) => setRoleEntrypoint(section.key, value)"
                />
              </template>
            </div>
          </template>

          <template v-else>
            <template v-for="(section, index) in resourceSections" :key="section.key">
              <el-divider v-if="index > 0" />
              <div class="resource-title-row">
                <div>
                  <div class="resource-title">{{ section.title }}</div>
                  <el-text
                    v-if="section.key !== 'frontend' && getAggregationWarning(section.key)"
                    size="small"
                    type="warning"
                    class="block mt-1"
                  >
                    {{ getAggregationWarning(section.key) }}
                  </el-text>
                </div>
                <div v-if="section.key !== 'frontend'" class="resource-aggregation">
                  <span class="text-sm text-[var(--el-text-color-regular)]">Aggregation</span>
                  <el-tooltip :content="getAggregationHint(section.key)" placement="top">
                    <el-icon class="aggregation-info"><InfoFilled /></el-icon>
                  </el-tooltip>
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
                <el-col v-if="section.key !== 'frontend'" :span="12">
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
                <el-col v-if="section.key !== 'frontend'" :span="12">
                  <el-form-item label="rdmaResource">
                    <el-input v-model="section.resource.rdmaResource" placeholder="Leave empty if not needed" />
                  </el-form-item>
                </el-col>
                <el-col v-if="section.key !== 'frontend'" :span="12">
                  <el-form-item label="sharedMemory">
                    <el-input v-model="section.resource.sharedMemory" placeholder="200">
                      <template #append>Gi</template>
                    </el-input>
                  </el-form-item>
                </el-col>
              </el-row>
            </template>
          </template>
        </div>

        <div v-if="!isOptimus" class="section-card">
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
            <el-col v-if="!isOptimus" :span="12">
              <el-form-item label="tpSize" prop="worker.tpSize">
                <el-input-number v-model="form.worker.tpSize" :min="1" controls-position="right" class="w-full" />
              </el-form-item>
            </el-col>
            <el-col v-if="!isOptimus" :span="12">
              <el-form-item label="epSize" prop="worker.epSize">
                <el-input-number v-model="form.worker.epSize" :min="1" controls-position="right" class="w-full" />
              </el-form-item>
            </el-col>
          </el-row>

          <template v-if="isOptimus">
            <el-divider />
            <div class="entry-role-title">Frontend</div>
            <el-form-item label="routerPolicy">
              <el-select v-model="form.routerPolicy">
                <el-option label="kv-aware" value="kv-aware" />
                <el-option label="round-robin" value="round-robin" />
              </el-select>
            </el-form-item>
            <el-input
              :model-value="frontendPreview"
              type="textarea"
              readonly
              :autosize="{ minRows: 3, maxRows: 6 }"
              class="entry-editor"
            />

            <template v-for="section in optimusBackendEntrySections" :key="section.key">
              <el-divider />
              <div class="entry-editor-toolbar">
                <div>
                  <div class="entry-role-title">{{ section.title }}</div>
                  <el-text size="small" type="info">
                    Edit this role command directly, or reset it from the options above.
                  </el-text>
                </div>
                <el-button size="small" @click="resetRoleEntrypointFromOptions(section.key)">
                  Reset from options
                </el-button>
              </div>
              <el-input
                :model-value="getRoleEntrypoint(section.key)"
                type="textarea"
                :autosize="{ minRows: 4, maxRows: 8 }"
                class="entry-editor"
                @input="(value: string) => setRoleEntrypoint(section.key, value)"
              />
            </template>
          </template>

          <template v-else>
            <div class="entry-editor-toolbar">
              <el-text size="small" type="info">
                Edit the full command directly, or reset it from the options above.
              </el-text>
              <el-button size="small" @click="resetWorkerEntrypointFromOptions">
                Reset from options
              </el-button>
            </div>
            <el-input
              v-model="form.workerEntrypoint"
              type="textarea"
              :autosize="{ minRows: 4, maxRows: 8 }"
              class="entry-editor"
              @input="markWorkerEntrypointCustomized"
            />
          </template>
        </div>

        <div class="section-card">
          <div
            class="section-header section-header--clickable"
            @click="advancedOpen = !advancedOpen"
          >
            <div class="section-bar"></div>
            <div class="flex-1">
              <div class="section-title">Advanced Options</div>
              <div class="section-subtitle">KV backend and environment variables</div>
            </div>
            <el-icon :class="['section-chevron', { 'is-open': advancedOpen }]">
              <ArrowRight />
            </el-icon>
          </div>

          <transition name="fade-slide">
            <div v-show="advancedOpen" class="advanced-body">
              <el-row :gutter="16">
                <el-col :span="12">
                  <el-form-item label="KV Backend">
                    <el-select v-model="form.kvTransferBackend">
                      <el-option
                        v-for="option in kvBackendOptions"
                        :key="option"
                        :label="option"
                        :value="option"
                      />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="24">
                  <el-form-item label="env vars" class="kv-full">
                    <KeyValueList v-model="envList" :max="20" keyMode="input" info="Add up to 20 envs" />
                  </el-form-item>
                </el-col>
              </el-row>
            </div>
          </transition>
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
import { ArrowRight, InfoFilled } from '@element-plus/icons-vue'
import {
  convertKeyValueMapToList,
  convertListToKeyValueMap,
  decodeFromBase64String,
} from '@/utils'
import {
  DYNAMO_SERVICE,
  buildDynamoCreatePayload,
  buildDynamoWorkerEntrypoint,
  createDefaultDynamoForm,
  getDynamoDefaultTpSize,
  type DynamoBackendEngine,
  type DynamoFormModel,
  type DynamoKvTransferBackend,
  type DynamoPdAggregationRole,
  type DynamoRoleResourceForm,
} from '../dynamoPayload'
import {
  OPTIMUS_SERVICE,
  buildOptimusCreatePayload,
  buildOptimusFrontendEntrypoint,
  buildOptimusWorkerEntrypoint,
  createDefaultOptimusForm,
  getOptimusDefaultTpSize,
  type OptimusKvTransferBackend,
  type OptimusRouterPolicy,
} from '@/pages/Optimus/optimusPayload'

type OptimusBackendRole = 'worker' | 'prefill' | 'decode'
type WorkloadFormModel = Omit<DynamoFormModel, 'kvTransferBackend'> & {
  frontend: DynamoRoleResourceForm
  routerPolicy: OptimusRouterPolicy
  frontendEntrypoint: string
  workerEntrypoint: string
  prefillEntrypoint: string
  decodeEntrypoint: string
  kvTransferBackend: DynamoKvTransferBackend | OptimusKvTransferBackend
}

const props = withDefaults(defineProps<{
  visible: boolean
  wlid?: string
  action: string
  workloadType?: 'dynamo' | 'optimus'
}>(), {
  workloadType: 'dynamo',
})

const emit = defineEmits(['update:visible', 'success'])

const store = useWorkspaceStore()
const userStore = useUserStore()
const isManager = computed(() => userStore.isManager)
const isEdit = computed(() => props.action === 'Edit')
const isOptimus = computed(() => props.workloadType === 'optimus')
const workloadLabel = computed(() => (isOptimus.value ? 'Optimus' : 'Dynamo'))
const workloadDefaultName = computed(() => (isOptimus.value ? 'optimus' : 'dynamo'))
const workloadService = computed(() => (isOptimus.value ? OPTIMUS_SERVICE : DYNAMO_SERVICE))
const kvBackendOptions = computed(() => (isOptimus.value ? ['mori'] : ['nixl', 'mooncake']))

const ruleFormRef = ref<FormInstance>()
const form = reactive<WorkloadFormModel>(
  (props.workloadType === 'optimus' ? createDefaultOptimusForm() : createDefaultDynamoForm()) as WorkloadFormModel,
)
const envList = ref(convertKeyValueMapToList(form.env))
const loading = ref(false)
const submitting = ref(false)
const isWorkerEntrypointCustomized = ref(false)
const customizedEntrypoints = reactive<Record<OptimusBackendRole, boolean>>({
  worker: false,
  prefill: false,
  decode: false,
})
const advancedOpen = ref(false)

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
  'worker.tpSize': [
    required('Please input TP size'),
    {
      validator: (_rule, value, callback) => {
        if (!isOptimus.value && form.enableAggregation && Number(value) <= 8) {
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
  'frontend.replica': [required('Please input replica')],
  'frontend.cpu': [required('Please input cpu')],
  'frontend.memory': [required('Please input memory')],
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
  service?: Partial<WorkloadFormModel['service']>
  dynamoOptions?: {
    serviceRoles?: string[]
    multinodeRoles?: string[]
    kvTransferBackend?: DynamoKvTransferBackend | string
  }
  optimusOptions?: {
    serviceRoles?: string[]
    multinodeRoles?: string[]
    kvTransferBackend?: string
  }
}

const modeValue = computed({
  get: () => (form.enablePd ? 'PD' : 'Default'),
  set: (value: string) => {
    form.enablePd = value === 'PD'
  },
})

const backendPreview = computed(() => buildWorkerEntrypoint(form, form.worker))
const frontendPreview = computed(() => buildOptimusFrontendEntrypoint(form))

const resourceSections = computed(() => {
  if (form.enablePd) {
    const sections = [
      { key: 'prefill', title: 'Prefill', resource: form.prefill },
      { key: 'decode', title: 'Decode', resource: form.decode },
    ]
    return isOptimus.value
      ? [{ key: 'frontend', title: 'Frontend', resource: form.frontend }, ...sections]
      : sections
  }
  const sections = [{ key: 'worker', title: 'Worker', resource: form.worker }]
  return isOptimus.value
    ? [{ key: 'frontend', title: 'Frontend', resource: form.frontend }, ...sections]
    : sections
})

const optimusRoleSections = computed(() => {
  if (form.enablePd) {
    return [
      { key: 'frontend' as const, title: 'Frontend', resource: form.frontend },
      { key: 'prefill' as const, title: 'Prefill', resource: form.prefill },
      { key: 'decode' as const, title: 'Decode', resource: form.decode },
    ]
  }
  return [
    { key: 'frontend' as const, title: 'Frontend', resource: form.frontend },
    { key: 'worker' as const, title: 'Worker', resource: form.worker },
  ]
})

const optimusBackendEntrySections = computed(() => {
  if (form.enablePd) {
    return [
      { key: 'prefill' as const, title: 'Prefill', resource: form.prefill },
      { key: 'decode' as const, title: 'Decode', resource: form.decode },
    ]
  }
  return [{ key: 'worker' as const, title: 'Worker', resource: form.worker }]
})

const getAggregationHint = (role: string) => {
  if (role === 'worker') {
    return 'Replica > 1 creates independent worker replicas unless Aggregation is enabled.'
  }
  return 'Replica > 1 creates independent role replicas unless Aggregation is enabled.'
}

const getRoleReplica = (role: string) => {
  if (role === 'prefill') return Number(form.prefill.replica || 0)
  if (role === 'decode') return Number(form.decode.replica || 0)
  return Number(form.worker.replica || 0)
}

const getAggregationWarning = (role: string) => {
  if (isRoleAggregated(role)) return ''
  if (getRoleReplica(role) <= 1) return ''
  if (!isOptimus.value && Number(form.worker.tpSize || 0) <= 8) {
    return 'Set TP size greater than 8 before enabling Aggregation.'
  }
  return 'Replica > 1 creates independent replicas unless Aggregation is enabled.'
}

const isRoleAggregated = (role: string) => {
  if (form.enablePd) {
    return form.enableAggregation && form.pdAggregationRoles.includes(role as DynamoPdAggregationRole)
  }
  return role === 'worker' && form.enableAggregation
}

const isRoleAggregationDisabled = (role: string) => {
  return getRoleReplica(role) <= 1
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
    const nextTpSize = isOptimus.value
      ? getOptimusDefaultTpSize(form.worker)
      : getDynamoDefaultTpSize(form.worker)
    form.worker.tpSize = nextTpSize
    form.worker.epSize = nextTpSize
  },
)

watch(
  backendPreview,
  (command) => {
    if (!isOptimus.value && !isWorkerEntrypointCustomized.value) {
      form.workerEntrypoint = command
    }
  },
  { immediate: true },
)

watch(
  () =>
    [
      isOptimus.value,
      buildWorkerEntrypoint(form, form.worker),
      buildWorkerEntrypoint(form, form.prefill),
      buildWorkerEntrypoint(form, form.decode),
    ] as const,
  ([optimus, workerCommand, prefillCommand, decodeCommand]) => {
    if (!optimus) return
    if (!customizedEntrypoints.worker) form.workerEntrypoint = workerCommand
    if (!customizedEntrypoints.prefill) form.prefillEntrypoint = prefillCommand
    if (!customizedEntrypoints.decode) form.decodeEntrypoint = decodeCommand
  },
  { immediate: true },
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

    const payload = buildCreatePayload(form, store.currentWorkspaceId)
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
  const next = createDefaultForm()
  Object.assign(form, next)
  envList.value = convertKeyValueMapToList(next.env)
  isWorkerEntrypointCustomized.value = false
  resetEntrypointCustomization()
  advancedOpen.value = false
  resetWorkerEntrypointFromOptions()
}

function hydrateFormFromDetail(detail: DynamoDetail) {
  const next = createDefaultForm()
  next.displayName =
    props.action === 'Clone'
      ? `${String(detail.displayName || detail.workloadId || workloadDefaultName.value)}-clone`.slice(0, 40)
      : String(detail.displayName || '')
  next.description = String(detail.description || '')
  next.priority = Number(detail.priority ?? next.priority)
  next.image = detail.images?.[0] || detail.image || next.image
  next.env = detail.env || next.env

  const dynamoOptions = getDetailOptions(detail)
  const serviceRoles = dynamoOptions.serviceRoles || inferServiceRoles(detail.resources)
  const multinodeRoles = dynamoOptions.multinodeRoles || []
  next.enablePd = serviceRoles.includes('prefill') && serviceRoles.includes('decode')
  next.enableAggregation = multinodeRoles.length > 0
  next.pdAggregationRoles = multinodeRoles.filter(
    (role): role is DynamoPdAggregationRole => role === 'prefill' || role === 'decode',
  )
  next.kvTransferBackend = (
    dynamoOptions.kvTransferBackend || next.kvTransferBackend
  ) as WorkloadFormModel['kvTransferBackend']

  next.service = { ...workloadService.value }

  if (isOptimus.value) {
    const frontendEntryPoint = detail.entryPoints?.[0]
      ? decodeFromBase64String(detail.entryPoints[0])
      : ''
    next.frontend = mergeResource(next.frontend, detail.resources?.[0])
    next.routerPolicy =
      (readFlag(frontendEntryPoint, '--router-policy') as OptimusRouterPolicy) || next.routerPolicy
    next.frontendEntrypoint = frontendEntryPoint || buildOptimusFrontendEntrypoint(next)

    if (next.enablePd) {
      const prefillEntryPoint = detail.entryPoints?.[1]
        ? decodeFromBase64String(detail.entryPoints[1])
        : ''
      const decodeEntryPoint = detail.entryPoints?.[2]
        ? decodeFromBase64String(detail.entryPoints[2])
        : ''
      next.prefill = mergeResource(next.prefill, detail.resources?.[1])
      next.decode = mergeResource(next.decode, detail.resources?.[2])
      applyEntrypointToResource(next, next.prefill, prefillEntryPoint)
      applyEntrypointToResource(next, next.decode, decodeEntryPoint)
      next.prefillEntrypoint = prefillEntryPoint || buildWorkerEntrypoint(next, next.prefill)
      next.decodeEntrypoint = decodeEntryPoint || buildWorkerEntrypoint(next, next.decode)
      customizedEntrypoints.prefill = Boolean(prefillEntryPoint)
      customizedEntrypoints.decode = Boolean(decodeEntryPoint)
    } else {
      const workerEntryPoint = detail.entryPoints?.[1]
        ? decodeFromBase64String(detail.entryPoints[1])
        : ''
      next.worker = mergeResource(next.worker, detail.resources?.[1])
      applyEntrypointToResource(next, next.worker, workerEntryPoint)
      next.workerEntrypoint = workerEntryPoint || buildWorkerEntrypoint(next, next.worker)
      customizedEntrypoints.worker = Boolean(workerEntryPoint)
    }
  } else if (next.enablePd) {
    const entryPoint = detail.entryPoints?.[1] ? decodeFromBase64String(detail.entryPoints[1]) : ''
    applyEntrypointToForm(next, entryPoint)
    next.workerEntrypoint = entryPoint || buildWorkerEntrypoint(next, next.worker)
    next.prefill = mergeResource(next.prefill, detail.resources?.[1])
    next.decode = mergeResource(next.decode, detail.resources?.[2])
  } else {
    const entryPoint = detail.entryPoints?.[1] ? decodeFromBase64String(detail.entryPoints[1]) : ''
    applyEntrypointToForm(next, entryPoint)
    next.workerEntrypoint = entryPoint || buildWorkerEntrypoint(next, next.worker)
    next.worker = mergeResource(next.worker, detail.resources?.[1])
  }

  Object.assign(form, next)
  envList.value = convertKeyValueMapToList(next.env)
  isWorkerEntrypointCustomized.value = !isOptimus.value && Boolean(detail.entryPoints?.[1])
}

function markWorkerEntrypointCustomized() {
  isWorkerEntrypointCustomized.value = form.workerEntrypoint !== backendPreview.value
}

function resetWorkerEntrypointFromOptions() {
  if (isOptimus.value) {
    resetAllRoleEntrypointsFromOptions()
    return
  }
  form.workerEntrypoint = backendPreview.value
  isWorkerEntrypointCustomized.value = false
}

function getRoleEntrypoint(role: OptimusBackendRole) {
  if (role === 'prefill') return form.prefillEntrypoint
  if (role === 'decode') return form.decodeEntrypoint
  return form.workerEntrypoint
}

function setRoleEntrypoint(role: OptimusBackendRole, value: string) {
  if (role === 'prefill') form.prefillEntrypoint = value
  else if (role === 'decode') form.decodeEntrypoint = value
  else form.workerEntrypoint = value
  customizedEntrypoints[role] = value !== buildWorkerEntrypoint(form, form[role])
}

function resetRoleEntrypointFromOptions(role: OptimusBackendRole) {
  const command = buildWorkerEntrypoint(form, form[role])
  if (role === 'prefill') form.prefillEntrypoint = command
  else if (role === 'decode') form.decodeEntrypoint = command
  else form.workerEntrypoint = command
  customizedEntrypoints[role] = false
}

function resetAllRoleEntrypointsFromOptions() {
  resetRoleEntrypointFromOptions('worker')
  resetRoleEntrypointFromOptions('prefill')
  resetRoleEntrypointFromOptions('decode')
}

function resetEntrypointCustomization() {
  isWorkerEntrypointCustomized.value = false
  customizedEntrypoints.worker = false
  customizedEntrypoints.prefill = false
  customizedEntrypoints.decode = false
}

function createDefaultForm(): WorkloadFormModel {
  return (isOptimus.value ? createDefaultOptimusForm() : createDefaultDynamoForm()) as WorkloadFormModel
}

function buildCreatePayload(target: WorkloadFormModel, workspace: string) {
  return isOptimus.value
    ? buildOptimusCreatePayload(target as any, workspace)
    : buildDynamoCreatePayload(target as DynamoFormModel, workspace)
}

function buildWorkerEntrypoint(target: WorkloadFormModel, resource: DynamoRoleResourceForm) {
  return isOptimus.value
    ? buildOptimusWorkerEntrypoint(target as any, resource)
    : buildDynamoWorkerEntrypoint(target as DynamoFormModel, resource)
}

function getDetailOptions(detail: DynamoDetail) {
  return isOptimus.value ? detail.optimusOptions || {} : detail.dynamoOptions || {}
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

function applyEntrypointToForm(target: WorkloadFormModel, command: string) {
  if (!command) return
  target.backendEngine = readBackendEngine(command) || target.backendEngine
  target.modelPath = readFlag(command, '--model-path') || target.modelPath
  applyEntrypointToResource(target, target.worker, command)
  target.attentionBackend = readFlag(command, '--attention-backend') || target.attentionBackend
  target.memFractionStatic = readFlag(command, '--mem-fraction-static') || target.memFractionStatic
  target.kvTransferBackend =
    (readFlag(command, '--disaggregation-transfer-backend') as WorkloadFormModel['kvTransferBackend']) ||
    target.kvTransferBackend
}

function applyEntrypointToResource(
  target: WorkloadFormModel,
  resource: DynamoRoleResourceForm,
  command: string,
) {
  if (!command) return
  target.backendEngine = readBackendEngine(command) || target.backendEngine
  target.modelPath = readFlag(command, '--model-path') || target.modelPath
  resource.tpSize = Number(readFlag(command, '--tp-size') || resource.tpSize)
  resource.epSize = Number(readFlag(command, '--ep-size') || resource.epSize)
  target.attentionBackend = readFlag(command, '--attention-backend') || target.attentionBackend
  target.memFractionStatic = readFlag(command, '--mem-fraction-static') || target.memFractionStatic
}

function readFlag(command: string, flag: string) {
  const match = command.match(new RegExp(`${flag}\\s+([^\\s]+)`))
  return match?.[1] || ''
}

function readBackendEngine(command: string): DynamoBackendEngine | '' {
  const match = command.match(/\b(?:dynamo|rocserve\.engine)\.(sglang|vllm)\b/)
  return (match?.[1] as DynamoBackendEngine | undefined) || ''
}

</script>

<style scoped>
.drawer-body {
  padding-right: 4px;
}

:deep(.dynamo-drawer .el-drawer__body) {
  overflow-y: auto;
  padding-bottom: 0;
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

.section-header--split {
  align-items: center;
}

.section-header--clickable {
  cursor: pointer;
  user-select: none;
}

.section-header-content {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
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

.section-chevron {
  transition: transform 0.18s ease-out;
  font-size: 16px;
  color: var(--el-text-color-secondary);
}

.section-chevron.is-open {
  transform: rotate(90deg);
}

.advanced-body {
  margin-top: 4px;
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
  gap: 8px;
  white-space: nowrap;
}

.aggregation-info {
  color: var(--el-text-color-secondary);
  cursor: help;
}

.optimus-role-card {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 14px 14px 12px;
  margin-top: 14px;
  background: var(--el-fill-color-blank);
}

html.dark .optimus-role-card {
  background: rgba(255, 255, 255, 0.02);
}

.role-subsection-title {
  margin: 12px 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.role-subsection-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-top: 12px;
  margin-bottom: 8px;
}

.role-subsection-header .role-subsection-title {
  margin: 0 0 2px;
}

.fixed-role {
  margin-bottom: 8px;
}

.entry-editor-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.entry-editor :deep(.el-textarea__inner) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New',
    monospace;
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
