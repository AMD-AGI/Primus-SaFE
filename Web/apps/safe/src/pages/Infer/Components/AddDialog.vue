<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} Infer`"
    :close-on-click-modal="false"
    size="820px"
    :before-close="cancelAdd"
    destroy-on-close
    direction="rtl"
    :z-index="100000"
    append-to-body
    class="infer-drawer"
    @open="onOpen"
  >
    <!-- Middle content area: scrollable -->
    <div class="drawer-body">
      <el-form
        ref="ruleFormRef"
        :model="form"
        label-width="auto"
        :rules="rules"
      >
        <!-- ===== Basic Information ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Basic Information</div>
              <div class="section-subtitle">Kind, name, description, entry point and image</div>
            </div>
          </div>

          <el-form-item label="Kind" prop="groupVersionKind.kind">
            <el-select
              v-model="form.groupVersionKind.kind"
              placeholder="Select workload type"
              :disabled="isEdit || isResume"
            >
              <el-option :label="WorkloadKind.Deployment" :value="WorkloadKind.Deployment" />
              <el-option :label="WorkloadKind.StatefulSet" :value="WorkloadKind.StatefulSet" />
            </el-select>
          </el-form-item>
          <el-form-item label="name" prop="displayName">
            <el-input v-model="form.displayName" :disabled="isEdit || isResume" />
          </el-form-item>
          <el-form-item label="description">
            <el-input v-model="form.description" :rows="2" type="textarea" />
          </el-form-item>
          <el-form-item label="entryPoint" prop="entryPoint">
            <el-input v-model="form.entryPoint" :rows="2" type="textarea" />
          </el-form-item>
          <el-form-item label="image" prop="image">
            <ImageInput v-model="form.image" />
          </el-form-item>
          <el-form-item label="priority">
            <el-select v-model="form.priority" placeholder="please select priority">
              <el-option label="Low" :value="0" />
              <el-option label="Medium" :value="1" />
              <el-option label="High" :value="2" v-if="isManager || store.isCurrentWorkspaceAdmin()" />
            </el-select>
          </el-form-item>
        </div>

        <!-- ===== Resource ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div class="flex-1 flex items-center justify-between">
              <div>
                <div class="section-title">Resource</div>
                <div class="section-subtitle">
                  Choose replicas / nodes and allocate CPU, GPU and memory
                </div>
              </div>
              <el-segmented
                v-if="!(isEdit && form.resourceType === 'nodes')"
                v-model="form.resourceType"
                :options="
                  isEdit && form.resourceType === 'replicas' ? ['replicas'] : ['replicas', 'nodes']
                "
              />
            </div>
          </div>

          <el-text class="mx-1 mb-2 block" size="small" type="info" v-if="isEdit">
            <el-icon class="mr-1"><InfoFilled /></el-icon>{{ REPLICA_INFO }}
          </el-text>

          <el-row :gutter="20">
            <el-col :span="24" v-if="form.resourceType === 'replicas'">
              <el-form-item label="replicas" prop="resource.replica">
                <el-input
                  v-model.number="form.resource.replica"
                  :placeholder="placeholders.replica"
                  :disabled="isEdit"
                />
              </el-form-item>
            </el-col>
            <el-col :span="24" v-if="form.resourceType === 'replicas'">
              <el-form-item label="excludedNodes">
                <el-select
                  v-model="form.excludedNodes"
                  multiple
                  clearable
                  filterable
                  collapse-tags
                  collapse-tags-tooltip
                  :max-collapse-tags="5"
                  placeholder="Select or paste nodes to exclude (comma-separated)"
                  ref="excludedNodesSelectRef"
                  :filter-method="filterExcludedNodes"
                  @visible-change="
                    (visible: boolean) =>
                      handleExcludedNodesVisibleChange(excludedNodesSelectRef, visible)
                  "
                >
                  <el-option
                    v-for="n in filteredExcludedNodeOptions"
                    :key="n.nodeId"
                    :label="n.nodeId"
                    :value="n.nodeId"
                  >
                    <div class="flex items-center justify-between w-full">
                      <div class="truncate">
                        <span>{{ n.nodeId }}</span>
                        <span
                          v-if="excludedNodesSearchQuery && n.internalIP"
                          class="text-gray-400 text-xs ml-2"
                        >
                          ({{ n.internalIP }})
                        </span>
                      </div>
                      <el-tag :type="n.available ? 'success' : 'danger'" size="small" effect="plain">
                        {{ n.available ? 'Available' : 'Unavailable' }}
                      </el-tag>
                    </div>
                  </el-option>
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="24" v-if="!isEdit && form.resourceType === 'nodes'">
              <el-form-item label="Nodes" prop="nodeList">
                <div class="node-select-wrapper">
                  <el-select
                    v-model="form.nodeList"
                    multiple
                    clearable
                    filterable
                    collapse-tags
                    collapse-tags-tooltip
                    :max-collapse-tags="5"
                    placeholder="Select or paste nodes (comma-separated)"
                    ref="nodeSelectRef"
                    @visible-change="
                      (visible: boolean) => handleNodesVisibleChange(nodeSelectRef, visible)
                    "
                  >
                    <el-option
                      v-for="n in nodeOptions"
                      :key="n.value"
                      :label="n.label"
                      :value="n.value"
                    >
                      <div class="flex items-center justify-between w-full">
                        <span class="truncate">{{ n.label }}</span>
                        <el-tag :type="n.available ? 'success' : 'danger'" size="small" effect="plain">
                          {{ n.available ? 'Available' : 'Unavailable' }}
                        </el-tag>
                      </div>
                    </el-option>
                  </el-select>
                </div>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="cpu" prop="resource.cpu">
                <el-input v-model="form.resource.cpu" :placeholder="placeholders.cpu" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="gpu">
                <el-input v-model="form.resource.gpu" :placeholder="placeholders.gpu" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="memory" prop="resource.memory">
                <el-input v-model="form.resource.memory" :placeholder="placeholders.memory">
                  <template #append>Gi</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="ephemeralStorage" prop="resource.ephemeralStorage">
                <el-input
                  v-model="form.resource.ephemeralStorage"
                  :placeholder="placeholders.ephemeralStorage"
                >
                  <template #append>Gi</template>
                </el-input>
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== Service Configuration ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Service Configuration</div>
              <div class="section-subtitle">Protocol, ports and service type</div>
            </div>
          </div>

          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="Protocol" prop="service.protocol">
                <el-select v-model="form.service.protocol" placeholder="Select protocol">
                  <el-option label="TCP" value="TCP" />
                  <el-option label="UDP" value="UDP" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Service Type" prop="service.serviceType">
                <el-select
                  v-model="form.service.serviceType"
                  placeholder="Select service type"
                  :disabled="isEdit"
                >
                  <el-option label="ClusterIP" value="ClusterIP" />
                  <el-option label="NodePort" value="NodePort" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="Container Port" prop="service.targetPort">
                <el-input-number
                  v-model="form.service.targetPort"
                  :min="0"
                  :max="65535"
                  placeholder="Container listening port"
                  @change="handleContainerPortChange"
                  class="w-full"
                />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Service Port" prop="service.port">
                <el-input-number
                  v-model="form.service.port"
                  :min="0"
                  :max="65535"
                  placeholder="External service port"
                  class="w-full"
                />
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="20" v-if="form.service.serviceType === 'NodePort'">
            <el-col :span="12">
              <el-form-item label="Node Port" prop="service.nodePort">
                <div class="flex items-center gap-2">
                  <el-input-number
                    v-model="form.service.nodePort"
                    :min="0"
                    :max="65535"
                    placeholder="Auto-assign if empty"
                    class="flex-1"
                  />
                  <el-tooltip
                    content="Let the system assign the NodePort automatically. If you must set it manually (it might fail), use a port above 30000."
                    placement="top"
                  >
                    <el-icon class="text-gray-500 cursor-help">
                      <InfoFilled />
                    </el-icon>
                  </el-tooltip>
                </div>
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== Health Check ===== -->
        <div class="section-card" v-if="!isEdit">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Health Check</div>
              <div class="section-subtitle">Configure liveness and readiness probes</div>
            </div>
          </div>

          <el-form-item label="Enable Health Check">
            <el-switch v-model="form.enableHealthCheck" />
          </el-form-item>

          <el-row :gutter="20" v-if="form.enableHealthCheck">
            <el-col :span="12">
              <el-form-item label="Health Check Path" prop="healthCheck.path" required>
                <el-input v-model="form.healthCheck.path" placeholder="e.g. /health" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Health Check Port" prop="healthCheck.port" required>
                <el-input-number
                  v-model="form.healthCheck.port"
                  :min="0"
                  :max="65535"
                  placeholder="Health check port"
                  class="w-full"
                />
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== Advanced Options (collapsible) ===== -->
        <div class="section-card">
          <div
            class="section-header section-header--clickable"
            @click="advancedOpen = !advancedOpen"
          >
            <div class="section-bar"></div>
            <div class="flex-1">
              <div class="section-title">Advanced Options</div>
              <div class="section-subtitle">Timeout, environment variables and secrets</div>
            </div>
            <el-icon :class="['section-chevron', { 'is-open': advancedOpen }]">
              <ArrowRight />
            </el-icon>
          </div>

          <transition name="fade-slide">
            <div v-show="advancedOpen" class="advanced-body">
              <el-row :gutter="16">
                <el-col :span="12">
                  <el-form-item label="timeout">
                    <el-input-number
                      v-model.number="form.timeout"
                      :min="0"
                      :step="1"
                      class="w-[160px] mr-2"
                    />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ TIMEOUT_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>
                <el-col :span="12" v-if="props.action === 'Clone'">
                  <el-form-item label="Workspace">
                    <el-select v-model="targetWorkspaceId" class="w-[200px]">
                      <el-option
                        v-for="ws in store.items"
                        :key="ws.workspaceId"
                        :label="ws.workspaceName"
                        :value="ws.workspaceId"
                      />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="secret" prop="secretIds">
                    <el-select v-model="form.secretIds" multiple placeholder="Please select secrets">
                      <el-option
                        v-for="item in secretOptions"
                        :key="item.value"
                        :label="item.label"
                        :value="item.value"
                      />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="12" v-if="!isEdit">
                  <el-form-item label="forceHostNetwork">
                    <el-switch v-model="form.forceHostNetwork" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ FORCE_HOST_NETWORK_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>
              </el-row>

              <el-divider border-style="dashed" class="kv-divider" />

              <el-form-item label="env vars" class="kv-full">
                <KeyValueList v-model="form.envList" :max="20" keyMode="input" info="Add up to 20 envs" />
              </el-form-item>
            </div>
          </transition>
        </div>
      </el-form>
    </div>

    <!-- Footer fixed at bottom -->
    <template #footer>
      <div class="drawer-footer">
        <el-button @click="cancelAdd">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import {
  defineProps,
  defineEmits,
  reactive,
  watch,
  ref,
  computed,
  nextTick,
  unref,
  toRef,
} from 'vue'
import {
  addWorkload,
  getNodeFlavorAvail,
  getWorkloadDetail,
  editWorkload,
} from '@/services/workload/index'
import { WorkloadKind } from '@/services/workload/type'
import { getNodesList, getImagesList, getWorkloadsList } from '@/services'
import { useSecrets, useSelectPaste } from '@/composables'
import { type FormInstance, ElMessage, ElMessageBox } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import ImageInput from '@/components/Base/ImageInput.vue'
import {
  byte2Gi,
  convertKeyValueMapToList,
  convertListToKeyValueMap,
  copyText,
} from '@/utils/index'
import type { FormItemRule } from 'element-plus'
import { InfoFilled, CopyDocument, ArrowRight } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import { encodeToBase64String } from '@/utils'
import { debounce } from 'lodash'

const props = defineProps<{
  visible: boolean
  wlid?: string
  action: string
  prefillData?: Record<string, unknown>
}>()
const emit = defineEmits(['update:visible', 'success'])

const isEdit = computed(() => props.action === 'Edit')
const isResume = computed(() => props.action === 'Resume')
const cachedUseWorkspaceStorage = ref<boolean | undefined>(undefined)

const store = useWorkspaceStore()
const userStore = useUserStore()
const isManager = computed(() => userStore.isManager)

const nodeOptions = ref([] as Array<{ label: string; value: string; available: boolean }>)
const excludedNodeOptions = ref(
  [] as Array<{ nodeId: string; available: boolean; internalIP?: string }>,
)
const imageOptions = ref([] as Array<{ id: number; tag: string }>)
const wlOptions = ref([] as Array<{ label: string; value: string }>)
// Use composable to fetch secrets
const { secretOptions, fetchSecrets } = useSecrets('image')
const nodeSelectRef = ref()
const excludedNodesSelectRef = ref()
const excludedNodesSearchQuery = ref('')

const TIMEOUT_INFO = 'timeout duration in seconds'
const REPLICA_INFO = 'If a node is specified, the replica cannot be modified.'
const FORCE_HOST_NETWORK_INFO = 'Force host network (default: auto-based on resources)'

// Prevent directly overwriting store data
const pendingWorkspaceId = ref<string>('')
// Advanced options (keep data synced)
const targetWorkspaceId = computed<string>({
  get: () => pendingWorkspaceId.value || store.currentWorkspaceId || store.firstWorkspace || '',
  set: (val: string) => {
    pendingWorkspaceId.value = val
  },
})

const showAdvanced = ref(false)
const advancedOpen = ref(false)
const fetchWorkspaceOption = () => store.fetchWorkspace(true)

const curPriority = computed(() => (isManager.value || store.isCurrentWorkspaceAdmin() ? 2 : 1))

const initialForm = () => ({
  displayName: '',
  groupVersionKind: {
    kind: WorkloadKind.Deployment,
    version: 'v1',
  },
  description: '',
  entryPoint: '',
  isSupervised: false,
  image: '',
  priority: unref(curPriority),
  resource: {
    replica: undefined as number | undefined,
    cpu: '',
    gpu: '',
    memory: '',
    ephemeralStorage: '',
  },
  envList: [
    {
      key: '',
      value: '',
    },
  ],
  labelList: [
    {
      key: '',
      value: '',
    },
  ],

  resourceType: 'replicas',
  nodeList: [] as string[],
  excludedNodes: [] as string[],

  timeout: undefined,

  secretIds: [] as string[],
  forceHostNetwork: false,

  // Service configuration
  service: {
    protocol: 'TCP',
    port: null as number | null,
    targetPort: null as number | null,
    nodePort: null as number | null,
    serviceType: 'ClusterIP',
  },

  // Health check configuration
  enableHealthCheck: false,
  healthCheck: {
    path: '',
    port: null as number | null,
  },
})
const form = reactive({ ...initialForm() })

const copyImage = async () => {
  if (!form.image) return
  await copyText(form.image)
}

const flavorMaxVal = ref()
const placeholders = computed(() => {
  const val = flavorMaxVal.value ?? {}
  return {
    cpu: `1 to ${Math.min(val.cpu ?? 102, 102)}`,
    gpu: `0 to ${val['amd.com/gpu'] ?? '-'}`,
    memory: `1 to ${Math.min(Number(byte2Gi(val.memory, undefined, false)) ?? 2000, 2000)}`,
    ephemeralStorage: `1 to ${Math.min(Number(byte2Gi(val['ephemeral-storage'], undefined, false)) ?? 6000, 6000)}`,
    replica: `Replica count`,
  }
})

const nameRegex = /^[a-z](?:[-a-z0-9]{0,38}[a-z0-9])?$/

const ruleFormRef = ref<FormInstance>()
const rules = reactive({
  hostname: [
    { required: true, message: 'Please input activity name', trigger: 'blur' },
    { max: 64, message: 'Must be less than 64 characters', trigger: 'blur' },
  ],

  displayName: [
    { required: true, message: 'Please input name', trigger: 'blur' },
    {
      pattern: nameRegex,
      message: 'Must start with lowercase letter, only a-z, 0-9, and "-" allowed, max 45 chars',
      trigger: 'blur',
    },
  ],
  entryPoint: [{ required: true, message: 'Please input entry point', trigger: 'blur' }],
  image: [{ required: true, message: 'Please input image', trigger: 'blur' }],
  'resource.replica': [{ required: true, message: 'Please input replica', trigger: 'blur' }],
  'resource.cpu': [{ required: true, message: 'Please input cpu', trigger: 'blur' }],
  'resource.memory': [{ required: true, message: 'Please input memory', trigger: 'blur' }],
  'resource.ephemeralStorage': [
    { required: true, message: 'Please input ephemeral storage', trigger: 'blur' },
  ],
  nodeList: [
    {
      type: 'array',
      required: true,
      message: 'Please select at least one node',
      trigger: 'change',
    },
  ],
  'service.port': [
    { required: true, message: 'Please input service port', trigger: 'blur' },
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        const num = Number(value)
        if (isNaN(num) || num < 1 || num > 65535) {
          callback(new Error('Service port must be between 1 and 65535'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
  'service.targetPort': [
    { required: true, message: 'Please input container port', trigger: 'blur' },
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        const num = Number(value)
        if (isNaN(num) || num < 1 || num > 65535) {
          callback(new Error('Container port must be between 1 and 65535'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
  'service.nodePort': [
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        if (!value) {
          callback()
          return
        }
        const num = Number(value)
        if (isNaN(num) || num < 1 || num > 65535) {
          callback(new Error('Node port must be between 1 and 65535'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
})

const submitting = ref(false)
const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  if (!store.currentWorkspaceId) return
  if (submitting.value) return
  try {
    await formEl.validate()
    submitting.value = true

    // Build secrets array
    const secrets = form.secretIds.map((id) => ({ id }))

    const {
      envList,
      labelList,
      resourceType,
      nodeList,
      resource,
      entryPoint,
      timeout,
      secretIds,
      healthCheck,
      excludedNodes,
      ...addPayload
    } = form

    const baseResource = {
      cpu: form.resource.cpu,
      gpu: Number(form.resource.gpu) === 0 ? '' : (form.resource.gpu ?? ''),
      memory: `${form.resource.memory}Gi`,
      ephemeralStorage: `${form.resource.ephemeralStorage}Gi`,
    }

    // Build resources array - Infer only needs a single resource
    const replica = resourceType === 'replicas' ? form.resource.replica : nodeList.length
    const resources = [{ ...baseResource, replica: replica || 1 }]

    const nodeListPayload = resourceType !== 'replicas' ? { specifiedNodes: nodeList } : {}
    // excludedNodes only works in replica mode, mutually exclusive with specifiedNodes
    const excludedNodesPayload =
      resourceType === 'replicas'
        ? (() => {
            const arr = (excludedNodes ?? []).filter(Boolean)
            return arr.length ? arr : undefined
          })()
        : undefined

    // Prepare service configuration
    const servicePayload =
      form.service.targetPort && form.service.port
        ? {
            service: {
              protocol: form.service.protocol,
              port: form.service.port,
              targetPort: form.service.targetPort,
              serviceType: form.service.serviceType,
              ...(form.service.serviceType === 'NodePort' && form.service.nodePort
                ? { nodePort: form.service.nodePort }
                : { nodePort: 0 }),
            },
          }
        : {}

    // Prepare health check configuration
    const healthCheckPayload =
      form.enableHealthCheck && healthCheck.path && healthCheck.port
        ? {
            liveness: {
              path: healthCheck.path,
              port: healthCheck.port,
            },
            readiness: {
              path: healthCheck.path,
              port: healthCheck.port,
            },
          }
        : {}

    if (!isEdit.value) {
      await addWorkload({
        ...addPayload,
        ...nodeListPayload,
        resources,
        workspace: props.action === 'Clone' ? pendingWorkspaceId.value : store.currentWorkspaceId!,
        env: convertListToKeyValueMap(envList),
        customerLabels: convertListToKeyValueMap(labelList),
        entryPoints: [encodeToBase64String(entryPoint)],
        images: [form.image],
        ...(form.timeout ? { timeout: form.timeout } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
        ...servicePayload,
        ...healthCheckPayload,
        ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
        ...(props.action === 'Resume' ? { workloadId: props.wlid } : {}),
        ...(cachedUseWorkspaceStorage.value !== undefined ? { useWorkspaceStorage: cachedUseWorkspaceStorage.value } : {}),
      })
      ElMessage({ message: `${props.action} successful`, type: 'success' })
    } else {
      const {
        displayName: _n,
        groupVersionKind,
        isSupervised,
        resource,
        envList,
        labelList,
        resourceType,
        nodeList,
        entryPoint,
        timeout,
        secretIds,
        excludedNodes: _excludedNodes,
        forceHostNetwork: _fhn,
        ...editPayload
      } = form
      if (!props.wlid) return

      await editWorkload(props.wlid, {
        ...editPayload,
        resources,
        env: convertListToKeyValueMap(envList),
        entryPoints: [encodeToBase64String(entryPoint)],
        images: [form.image],
        ...(form.timeout !== undefined ? { timeout: form.timeout } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
        ...servicePayload,
        ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
      })
      ElMessage({ message: 'Edit successful', type: 'success' })
    }

    if (props.action === 'Clone' && pendingWorkspaceId.value !== store.currentWorkspaceId) {
      store.setCurrentWorkspace(pendingWorkspaceId.value)
      await store.fetchWorkspace(true)
    }

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey as any)
      ElMessage.error(firstMsg)
    }
  } finally {
    submitting.value = false
  }
}

const cancelAdd = () => {
  ElMessageBox.confirm('All fields will be cleared.', 'Clear form & close?', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    emit('update:visible', false)
    Object.assign(form, initialForm())
  })
}

function createBetweenRule(min: number, max: number, unit?: string): FormItemRule {
  return {
    validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
      const num = Number(value)
      if (isNaN(num) || num < min || num > max) {
        callback(new Error(`Must be between ${min} and ${max}${unit ? ` ${unit}` : ''}`))
      } else {
        callback()
      }
    },
    trigger: 'blur',
  }
}
watch(
  () => store.currentNodeFlavor,
  async (flavorId) => {
    if (!flavorId) return

    const res = await getNodeFlavorAvail(flavorId)
    flavorMaxVal.value = res
    ;(rules['resource.replica'] as FormItemRule[]).push(createBetweenRule(1, 999))
    ;(rules['resource.cpu'] as FormItemRule[]).push(createBetweenRule(1, res.cpu))
    ;(rules['resource.memory'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res.memory ?? 0, 0, false))),
    )
    ;(rules['resource.ephemeralStorage'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res['ephemeral-storage'] ?? 0, 0, false))),
    )
  },
  { immediate: true },
)

const setInitialFormValues = async () => {
  if (!props.wlid) return

  const res = await getWorkloadDetail(props.wlid)
  cachedUseWorkspaceStorage.value = res.useWorkspaceStorage

  form.displayName = res.displayName
  form.description = res.description
  // workload now supports entryPoints/images arrays; UI keeps a single entryPoint/image
  form.entryPoint =
    (Array.isArray(res.entryPoints) ? res.entryPoints[0] : undefined) ?? res.entryPoint ?? ''
  form.isSupervised = res.isSupervised ?? false
  form.image = (Array.isArray(res.images) ? res.images[0] : undefined) ?? res.image ?? ''
  form.timeout = res.timeout

  // Map groupVersionKind
  if (res.groupVersionKind) {
    form.groupVersionKind = {
      kind: res.groupVersionKind.kind || WorkloadKind.Deployment,
      version: res.groupVersionKind.version || 'v1',
    }
  }

  // Map service configuration
  if (res.service) {
    form.service = {
      protocol: res.service.protocol || 'TCP',
      port: res.service.port || null,
      targetPort: res.service.targetPort || null,
      nodePort: res.service.nodePort || null,
      serviceType: res.service.serviceType || 'ClusterIP',
    }
  }

  // Map health check configuration
  if (res.liveness && res.liveness.path && res.liveness.port) {
    form.enableHealthCheck = true
    form.healthCheck = {
      path: res.liveness.path,
      port: res.liveness.port,
    }
  }

  // Regular users cannot select high priority; auto-downgrade to medium when cloning
  form.priority =
    isManager.value || store.isCurrentWorkspaceAdmin()
      ? res.priority
      : res.priority === 2
        ? 1
        : res.priority

  form.resourceType = 'replicas'
  if (!isEdit.value && res.specifiedNodes?.length) {
    form.nodeList = res.specifiedNodes
  }

  // resources changed to array, Infer uses first element
  const firstResource = res.resources?.[0] || res.resource || {}
  const { gpuName, rdmaResource, ...clearResource } = firstResource
  form.resource = clearResource
  form.resource.memory = firstResource?.memory?.replace(/Gi$/i, '') ?? ''
  form.resource.ephemeralStorage = firstResource?.ephemeralStorage?.replace(/Gi$/i, '') ?? ''

  form.envList = convertKeyValueMapToList(res.env)
  form.labelList = convertKeyValueMapToList(res.customerLabels)

  // Handle excludedNodes
  form.excludedNodes = res.excludedNodes ?? []

  // Handle secrets - clear during Clone/Resume, keep during Edit
  if (props.action === 'Edit' && res.secrets && res.secrets.length > 0) {
    form.secretIds = res.secrets.map((s: any) => s.id)
  } else if (props.action === 'Clone' || props.action === 'Resume') {
    form.secretIds = [] // Clear secrets during Clone/Resume so users can re-select
  }

  form.forceHostNetwork = res.forceHostNetwork ?? false

  if (props.action === 'Clone') {
    fetchWorkspaceOption()
  }
}

const fetchNodes = async () => {
  const nodes = await getNodesList({
    workspaceId: store.currentWorkspaceId,
    limit: -1,
    brief: true,
  }).catch(() => ({ items: [] }))
  nodeOptions.value = (nodes?.items ?? []).map((n: any) => ({
    label: n.hostname ?? n.nodeName ?? n.nodeId ?? n.name,
    value: n.nodeId ?? n.name ?? n.hostname,
    available: Boolean(n.available),
  }))
  // Also populate excludedNodeOptions with the same data
  excludedNodeOptions.value = (nodes?.items ?? []).map((n: any) => ({
    nodeId: n.nodeId ?? n.name ?? n.hostname,
    available: Boolean(n.available),
    internalIP: n.internalIP,
  }))
}

// Filter excluded nodes based on search query
const filteredExcludedNodeOptions = computed(() => {
  if (!excludedNodesSearchQuery.value) {
    return excludedNodeOptions.value
  }
  const query = excludedNodesSearchQuery.value.toLowerCase()
  return excludedNodeOptions.value.filter(
    (n) =>
      n.nodeId.toLowerCase().includes(query) ||
      (n.internalIP && n.internalIP.toLowerCase().includes(query)),
  )
})

// Custom filter method to capture search query
const filterExcludedNodes = (query: string) => {
  excludedNodesSearchQuery.value = query
}

const fetchImage = async (tag?: string) => {
  const res = await getImagesList({ flat: true, tag })
  imageOptions.value = res ?? []
}

// Handle container port change - sync to service port and node port if they are empty
const handleContainerPortChange = (val: number) => {
  // Sync to service port if it's empty
  if (!form.service.port) {
    form.service.port = val
  }
  // Sync to node port if it's empty and NodePort is selected
  if (form.service.serviceType === 'NodePort' && !form.service.nodePort) {
    form.service.nodePort = val
  }
  // Sync to health check port if it's empty and health check is enabled
  if (form.enableHealthCheck && !form.healthCheck.port) {
    form.healthCheck.port = val
  }
}

// Watch service type change - sync targetPort to nodePort when switching to NodePort
watch(
  () => form.service.serviceType,
  (newType) => {
    // When switching to NodePort, sync targetPort to nodePort if nodePort is empty
    if (newType === 'NodePort' && !form.service.nodePort && form.service.targetPort) {
      form.service.nodePort = form.service.targetPort
    }
  },
)

// Watch health check enable change - sync targetPort to health check port when enabling
watch(
  () => form.enableHealthCheck,
  (newValue) => {
    // When enabling health check, sync targetPort to health check port if it's empty
    if (newValue && !form.healthCheck.port && form.service.targetPort) {
      form.healthCheck.port = form.service.targetPort
    }
  },
)

// Use composable to handle nodes paste functionality
const { handleSelectVisibleChange: handleNodesVisibleChange } = useSelectPaste({
  options: nodeOptions,
  modelValue: toRef(form, 'nodeList'),
  successMessagePrefix: 'Matched and selected',
  warningMessagePrefix: 'Could not find nodes',
})

// Use composable to handle excludedNodes paste functionality
const { handleSelectVisibleChange: handleExcludedNodesVisibleChange } = useSelectPaste({
  options: computed(() =>
    excludedNodeOptions.value.map((n) => ({
      label: n.nodeId,
      value: n.nodeId,
    })),
  ),
  modelValue: toRef(form, 'excludedNodes'),
  successMessagePrefix: 'Matched and excluded',
  warningMessagePrefix: 'Could not find nodes',
})

const fetchWlOptions = async () => {
  const res = await getWorkloadsList({
    phase: 'Pending,Running',
    userId: userStore.userId,
    workspaceId: store.currentWorkspaceId,
  })
  wlOptions.value = res?.items?.map((v: any) => ({
    label: v.displayName,
    value: v.workloadId,
  }))
}
const filterImageOptions = debounce(async (query: string) => {
  if (!query) {
    await fetchImage()
  } else {
    await fetchImage(query)
  }
}, 300)

watch(
  () => store.currentWorkspaceId,
  (cur) => {
    if (!pendingWorkspaceId.value || pendingWorkspaceId.value === cur) {
      pendingWorkspaceId.value = cur ?? ''
    }
  },
  { immediate: true },
)

// Helper: convert object to key-value list
const objectToKeyValueList = (obj: unknown) => {
  if (!obj || typeof obj !== 'object') return undefined
  return Object.entries(obj as Record<string, unknown>).map(([key, value]) => ({
    key,
    value: String(value),
  }))
}

// Helper: apply prefill data
const applyPrefillData = () => {
  if (!props.prefillData) return

  const data = props.prefillData

  // Basic field mapping
  form.displayName = String(data.displayName ?? form.displayName)
  form.description = String(data.description ?? form.description)
  form.entryPoint = String(data.entryPoint ?? form.entryPoint)
  form.image = String(data.image ?? form.image)

  // Environment variables and labels mapping
  form.envList = objectToKeyValueList(data.env) ?? form.envList
  form.labelList = objectToKeyValueList(data.labels) ?? form.labelList

  // Resource configuration mapping
  form.resource.cpu = String(data.cpu ?? form.resource.cpu)
  form.resource.memory = String(data.memory ?? form.resource.memory)
  form.resource.gpu = String(data.gpu ?? form.resource.gpu)
  form.resource.replica = Number(data.replica ?? form.resource.replica)
  form.resource.ephemeralStorage = data.ephemeralStorage
    ? String((data.ephemeralStorage as string).replace(/Gi$/i, ''))
    : form.resource.ephemeralStorage

  // Service configuration mapping
  if (data.service) {
    const serviceData = data.service as Record<string, any>
    form.service.protocol = String(serviceData.protocol ?? form.service.protocol)
    form.service.port = serviceData.port ?? form.service.port
    form.service.targetPort = serviceData.targetPort ?? form.service.targetPort
    form.service.serviceType = String(serviceData.serviceType ?? form.service.serviceType)
    if (serviceData.nodePort) {
      form.service.nodePort = serviceData.nodePort
    }
  }
}

const onOpen = async () => {
  showAdvanced.value = false
  cachedUseWorkspaceStorage.value = undefined
  pendingWorkspaceId.value = store.currentWorkspaceId ?? store.firstWorkspace ?? ''
  fetchNodes()
  fetchImage()
  fetchWlOptions()
  fetchSecrets()

  if (props.action !== 'Create') {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())
    applyPrefillData()
  }

  await nextTick()
  if (props.action === 'Create') {
    setTimeout(() => ruleFormRef.value?.clearValidate(), 500)
  }
}
</script>

<style>
.el-drawer__header {
  padding: 12px 24px 4px;
  margin-bottom: 0;
}
.el-drawer__title {
  font-size: 18px;
  font-weight: 600;
}
.el-drawer__body {
  padding-bottom: 0;
}
</style>
<style scoped>
.drawer-body {
  max-height: 83vh;
  overflow-y: auto;
}

/* Wrap each group in a card */
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

/* Use stronger shadow in dark mode */
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

/* Section title */
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
.section-header--clickable {
  cursor: pointer;
}

/* Advanced toggle arrow */
.section-chevron {
  transition: transform 0.18s ease-out;
  font-size: 16px;
  color: var(--el-text-color-secondary);
}
.section-chevron.is-open {
  transform: rotate(90deg);
}

/* Leave some space at the top of advanced expanded content */
.advanced-body {
  margin-top: 4px;
}

/* Add some padding for labels/env section */
.kv-divider {
  margin: 4px 0 10px;
}
.kv-full {
  margin-bottom: 8px;
}
.kv-full :deep(.key-value-list-root) {
  width: 100%;
}

/* Node select wrapper */
.node-select-wrapper {
  display: flex;
  align-items: center;
  width: 100%;
}
.node-select-wrapper .el-select {
  flex: 1;
}

/* Drawer footer */
.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 10px 24px;
  border-top: 1px solid var(--el-border-color-lighter);
}

/* Keep animation */
.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: all 0.25s ease;
}
.fade-slide-enter-from,
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
.rotate-180 {
  transform: rotate(180deg);
}
</style>
