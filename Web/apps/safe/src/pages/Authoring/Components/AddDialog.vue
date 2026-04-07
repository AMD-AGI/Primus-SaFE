<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} Authoring`"
    :close-on-click-modal="false"
    size="820px"
    :before-close="cancelAdd"
    destroy-on-close
    direction="rtl"
    :z-index="100000"
    append-to-body
    class="authoring-drawer"
    @open="onOpen"
  >
    <!-- Middle content area: scrollable -->
    <div class="drawer-body">
      <el-form
        ref="ruleFormRef"
        :model="form"
        label-width="auto"
        :rules="rules"
        :validate-on-rule-change="false"
      >
        <!-- ===== Basic Information ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Basic Information</div>
              <div class="section-subtitle">Name, description and image</div>
            </div>
          </div>

          <el-form-item label="name" prop="displayName" data-tour="authoring-field-name">
            <el-input v-model="form.displayName" :disabled="isEdit || isResume" />
          </el-form-item>
          <el-form-item label="description">
            <el-input v-model="form.description" :rows="2" type="textarea" />
          </el-form-item>
          <el-form-item label="image" prop="image" data-tour="authoring-field-image">
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
        <div class="section-card" data-tour="authoring-field-resource">
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
                v-model="form.resourceType"
                :options="['replicas', 'nodes']"
                :disabled="isEdit"
              />
            </div>
          </div>

          <el-text class="mx-1 mb-2 block" size="small" type="info">
            <el-icon class="mr-1"><InfoFilled /></el-icon>{{ REPLICA_INFO }}
          </el-text>

          <el-row :gutter="20">
            <el-col :span="24" v-if="form.resourceType === 'replicas'">
              <el-form-item label="replica" prop="resource.replica">
                <el-input placeholder="1" disabled />
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
                  :disabled="isEdit"
                  placeholder="Select or paste nodes to exclude (comma-separated)"
                  ref="excludedNodesSelectRef"
                  :filter-method="filterExcludedNodes"
                  :loading="nodesLoading"
                  @visible-change="
                    async (visible: boolean) => {
                      if (visible) await fetchNodesOnDropdown()
                      handleExcludedNodesVisibleChange(excludedNodesSelectRef, visible)
                    }
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
            <el-col :span="24" v-else>
              <el-form-item label="Node" prop="nodeId">
                <el-select
                  v-model="form.nodeId"
                  clearable
                  filterable
                  :disabled="isEdit"
                  placeholder="Select one node (required)"
                  :loading="nodesLoading"
                  @visible-change="async (visible: boolean) => { if (visible) await fetchNodesOnDropdown() }"
                >
                  <el-option v-for="n in nodeOptions" :key="n.value" :label="n.label" :value="n.value">
                    <div class="flex items-center justify-between w-full">
                      <span class="truncate">{{ n.label }}</span>
                      <el-tag :type="n.available ? 'success' : 'danger'" size="small" effect="plain">
                        {{ n.available ? 'Available' : 'Unavailable' }}
                      </el-tag>
                    </div>
                  </el-option>
                </el-select>
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

          <div v-if="persistentStoragePaths" class="persistent-storage-hint">
            <el-icon class="mr-1"><InfoFilled /></el-icon>
            <span>persistentStoragePath: {{ persistentStoragePaths }}</span>
          </div>
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
              <div class="section-subtitle">Toleration, timeout, environment variables and secrets</div>
            </div>
            <el-icon :class="['section-chevron', { 'is-open': advancedOpen }]">
              <ArrowRight />
            </el-icon>
          </div>

          <transition name="fade-slide">
            <div v-show="advancedOpen" class="advanced-body">
              <el-row :gutter="16">
                <el-col :span="12">
                  <el-form-item label="toleration">
                    <el-switch v-model="form.isTolerateAll" :disabled="isEdit" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ TOLERATE_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>
                <el-col :span="12" v-if="isManager || store.isCurrentWorkspaceAdmin()">
                  <el-form-item label="privileged">
                    <el-switch v-model="form.privileged" :disabled="isEdit" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ PRIVILEGED_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>
                <el-col :span="12" v-if="isEdit">
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
                <!-- nodesAffinity -->
                <el-col :span="12" v-if="!isEdit && (form.resourceType === 'nodes' || props.action === 'Clone' || props.action === 'Resume')">
                  <el-form-item label="nodesAffinity">
                    <el-radio-group v-model="form.nodesAffinity" size="small">
                      <el-radio-button value="" :disabled="form.resourceType === 'nodes' && !clonedLastNodes.length">Disabled</el-radio-button>
                      <el-radio-button value="required">Required</el-radio-button>
                      <el-radio-button value="preferred">Preferred</el-radio-button>
                    </el-radio-group>
                    <el-text size="small" type="info" class="ml-2">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ NODES_AFFINITY_INFO }}
                    </el-text>
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
                <el-col :span="12">
                  <el-form-item label="secret" prop="secretIds">
                    <el-select v-model="form.secretIds" multiple :disabled="isEdit" placeholder="Please select secrets">
                      <el-option
                        v-for="item in secretOptions"
                        :key="item.value"
                        :label="item.label"
                        :value="item.value"
                      />
                    </el-select>
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
  editWorkload,
  getNodeFlavorAvail,
  getWorkloadDetail,
} from '@/services/workload/index'
import { getWorkspaceDetail } from '@/services/workspace/index'
import { getNodesList } from '@/services'
import { useSecrets, useSelectPaste } from '@/composables'
import { type FormInstance, ElMessage, ElMessageBox } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import ImageInput from '@/components/Base/ImageInput.vue'
import {
  byte2Gi,
  convertKeyValueMapToList,
  convertListToKeyValueMap,
} from '@/utils/index'
import type { FormItemRule } from 'element-plus'
import { InfoFilled, ArrowRight } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'

const props = defineProps<{
  visible: boolean
  wlid?: string
  action: string
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
const excludedNodesSearchQuery = ref('')
const nodesLoading = ref(false)
const excludedNodesSelectRef = ref()

// Use composable to fetch secrets
const { secretOptions, fetchSecrets } = useSecrets('image')

const TOLERATE_INFO = 'If enabled, workloads can be scheduled to nodes with taints'
const REPLICA_INFO = 'If a node is specified, the replica cannot be modified.'
const PRIVILEGED_INFO = 'Whether to run in privileged mode'
const TIMEOUT_INFO = 'timeout duration in seconds'
const FORCE_HOST_NETWORK_INFO = 'Force host network (default: auto-based on resources)'
const NODES_AFFINITY_INFO = 'Node affinity: Required (strict) or Preferred (best-effort)'

const advancedOpen = ref(false)

const persistentStoragePaths = ref('')

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
const fetchWorkspaceOption = () => store.fetchWorkspace(true)

const curPriority = computed(() => (isManager.value || store.isCurrentWorkspaceAdmin() ? 2 : 1))

const initialForm = () => ({
  displayName: '',
  groupVersionKind: {
    kind: 'Authoring',
    version: 'v1',
  },
  description: '',
  image: '',
  priority: unref(curPriority),
  resource: {
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
  isTolerateAll: true,

  resourceType: 'replicas',
  nodeId: '',
  excludedNodes: [] as string[],
  secretIds: [] as string[],
  privileged: false,
  timeout: undefined as number | undefined,
  nodesAffinity: '' as '' | 'required' | 'preferred',
  forceHostNetwork: false,
})
const form = reactive({ ...initialForm() })

const clonedLastNodes = ref<string[]>([])
watch(() => form.nodesAffinity, (newVal, oldVal) => {
  if (!clonedLastNodes.value.length) return
  if (newVal && !oldVal) {
    form.resourceType = 'nodes'
    form.nodeId = clonedLastNodes.value[0] || ''
  } else if (!newVal && oldVal) {
    form.resourceType = 'replicas'
    form.nodeId = ''
  }
})
watch(() => form.resourceType, (newType) => {
  if (newType === 'nodes' && !form.nodesAffinity) {
    form.nodesAffinity = 'required'
  }
})

const flavorMaxVal = ref()
const placeholders = computed(() => {
  const val = flavorMaxVal.value ?? {}
  return {
    cpu: `1 to ${Math.min(val.cpu ?? 102, 102)}`,
    gpu: `0 to ${val['amd.com/gpu'] ?? '-'}`,
    memory: `1 to ${Math.min(Number(byte2Gi(val.memory, undefined, false)) ?? 2000, 2000)}`,
    ephemeralStorage: `1 to ${Math.min(Number(byte2Gi(val['ephemeral-storage'], undefined, false)) ?? 6000, 6000)}`,
  }
})

const nameRegex = /^[a-z](?:[-a-z0-9]{0,42}[a-z0-9])?$/

const ruleFormRef = ref<FormInstance>()
const rules = reactive({
  displayName: [
    { required: true, message: 'Please input name', trigger: 'blur' },
    {
      pattern: nameRegex,
      message: 'Must start with lowercase letter, only a-z, 0-9, and "-" allowed, max 45 chars',
      trigger: 'blur',
    },
  ],
  image: [{ required: true, message: 'Please input image', trigger: 'blur' }],
  'resource.cpu': [{ required: true, message: 'Please input cpu', trigger: 'blur' }],
  'resource.memory': [{ required: true, message: 'Please input memory', trigger: 'blur' }],
  'resource.ephemeralStorage': [
    { required: true, message: 'Please input ephemeral storage', trigger: 'blur' },
  ],
  nodeId: [
    {
      required: true,
      message: 'Please select node',
      trigger: 'change',
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

    const { envList, resourceType, nodeId, resource, excludedNodes, image, timeout, nodesAffinity: _nodesAffinity, ...addPayload } = form

    const baseResource = {
      cpu: form.resource.cpu,
      gpu: Number(form.resource.gpu) === 0 ? '' : (form.resource.gpu ?? ''),
      memory: `${form.resource.memory}Gi`,
      ephemeralStorage: `${form.resource.ephemeralStorage}Gi`,
    }

    // Build secrets array
    const secrets = form.secretIds.map((id) => ({ id }))

    // Build resources array - Authoring uses a single resource with replica = 1
    const resources = [{ ...baseResource, replica: 1 }]

    const nodePayload = resourceType !== 'replicas' ? { specifiedNodes: [nodeId] } : {}
    // excludedNodes only applies in replica mode; mutually exclusive with specifiedNodes
    const excludedNodesPayload =
      resourceType === 'replicas'
        ? (() => {
            const arr = (excludedNodes ?? []).filter(Boolean)
            return arr.length ? arr : undefined
          })()
        : undefined

    if (!isEdit.value) {
      await addWorkload({
        ...addPayload,
        ...nodePayload,
        images: [image],
        resources,
        workspace: props.action === 'Clone' ? pendingWorkspaceId.value : store.currentWorkspaceId!,
        env: convertListToKeyValueMap(envList),
        ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
        ...(form.nodesAffinity ? { nodesAffinity: form.nodesAffinity as 'required' | 'preferred' } : {}),
        privileged: form.privileged,
        ...(props.action === 'Resume' ? { workloadId: props.wlid } : {}),
        ...(cachedUseWorkspaceStorage.value !== undefined ? { useWorkspaceStorage: cachedUseWorkspaceStorage.value } : {}),
      })
      ElMessage({ message: `${props.action} successful`, type: 'success' })
    } else {
      if (!props.wlid) return

      await editWorkload(props.wlid, {
        description: form.description,
        priority: form.priority,
        images: [form.image],
        resources,
        env: convertListToKeyValueMap(form.envList),
        ...(form.timeout !== undefined ? { timeout: form.timeout } : {}),
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
    // Only handle form validation errors; API errors are handled by request wrapper, no duplicate toasts
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey)
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

  if (props.action === 'Clone') {
    form.secretIds = []
    fetchWorkspaceOption()
  }

  const res = await getWorkloadDetail(props.wlid)
  cachedUseWorkspaceStorage.value = res.useWorkspaceStorage

  form.displayName = res.displayName
  form.description = res.description
  // workload now supports images array; UI keeps a single image
  form.image = (Array.isArray(res.images) ? res.images[0] : undefined) ?? res.image ?? ''

  // Regular users cannot select high priority; auto-downgrade to medium when cloning
  form.priority =
    isManager.value || store.isCurrentWorkspaceAdmin()
      ? res.priority
      : res.priority === 2
        ? 1
        : res.priority

  if (res.specifiedNodes?.length) {
    form.resourceType = 'nodes'
    form.nodeId = res.specifiedNodes[0]
    form.nodesAffinity = res.nodesAffinity || 'required'
    clonedLastNodes.value = []
  } else {
    form.resourceType = 'replicas'
    form.nodeId = ''
    form.nodesAffinity = ''
    const lastNodes = res.nodes?.length ? (res.nodes[res.nodes.length - 1] ?? []) : []
    clonedLastNodes.value = lastNodes
  }

  // resources is now an array; Authoring takes the first element
  const firstResource = res.resources?.[0] || res.resource || {}
  const { gpuName, rdmaResource, ...clearResource } = firstResource
  form.resource = clearResource
  form.resource.memory = firstResource?.memory?.replace(/Gi$/i, '') ?? ''
  form.resource.ephemeralStorage = firstResource?.ephemeralStorage?.replace(/Gi$/i, '') ?? ''

  form.isTolerateAll = res.isTolerateAll ?? false
  form.privileged = res.privileged ?? false
  form.timeout = res.timeout

  form.envList = convertKeyValueMapToList(res.env)

  // Handle excludedNodes
  form.excludedNodes = res.excludedNodes ?? []

  // Handle secrets - clear on Clone/Resume, keep on Edit
  if (props.action === 'Edit' && res.secrets && res.secrets.length > 0) {
    form.secretIds = res.secrets.map((s: { id: string }) => s.id)
  } else if (props.action === 'Clone' || props.action === 'Resume') {
    form.secretIds = []
  }

  form.forceHostNetwork = res.forceHostNetwork ?? false
  await nextTick()
  ruleFormRef.value?.clearValidate()
}

const fetchNodes = async () => {
  const nodes = await getNodesList({
    workspaceId: store.currentWorkspaceId,
    limit: -1,
    brief: true,
  }).catch(() => ({ items: [] }))
  nodeOptions.value = (nodes?.items ?? []).map(
    (n: {
      hostname?: string
      nodeName?: string
      nodeId?: string
      name?: string
      available?: boolean
      internalIP?: string
    }) => ({
      label: n.hostname ?? n.nodeName ?? n.nodeId ?? n.name,
      value: n.nodeId ?? n.name ?? n.hostname,
      available: Boolean(n.available),
    }),
  )
  // Also populate excludedNodeOptions with the same data
  excludedNodeOptions.value = (nodes?.items ?? []).map(
    (n: {
      hostname?: string
      nodeName?: string
      nodeId?: string
      name?: string
      available?: boolean
      internalIP?: string
    }) => ({
      nodeId: n.nodeId ?? n.name ?? n.hostname,
      available: Boolean(n.available),
      internalIP: n.internalIP,
    }),
  )
}

const fetchNodesOnDropdown = async () => {
  if (nodesLoading.value) return
  nodesLoading.value = true
  try {
    await fetchNodes()
  } finally {
    nodesLoading.value = false
  }
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

watch(
  () => store.currentWorkspaceId,
  (cur) => {
    if (!pendingWorkspaceId.value || pendingWorkspaceId.value === cur) {
      pendingWorkspaceId.value = cur ?? ''
    }
  },
  { immediate: true },
)

const fetchPersistentStoragePaths = async () => {
  try {
    const wsId = store.currentWorkspaceId
    if (!wsId) return
    const res = await getWorkspaceDetail(wsId)
    const paths = (res.volumes ?? [])
      .map((v: { mountPath?: string }) => v.mountPath)
      .filter(Boolean)
    persistentStoragePaths.value = paths.length ? paths.join(', ') : ''
  } catch {
    persistentStoragePaths.value = ''
  }
}

const onOpen = async () => {
  showAdvanced.value = false
  cachedUseWorkspaceStorage.value = undefined
  clonedLastNodes.value = []
  pendingWorkspaceId.value = store.currentWorkspaceId ?? store.firstWorkspace ?? ''

  const fetches = [fetchSecrets(), userStore.fetchEnvs(), fetchPersistentStoragePaths()]

  if (props.action !== 'Create') {
    fetches.push(setInitialFormValues())
  }

  await Promise.all(fetches)

  if (props.action === 'Create') {
    Object.assign(form, initialForm())
    await nextTick()
    ruleFormRef.value?.clearValidate()
  }
  await nextTick()
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

/* Leave some space above the advanced expanded content */
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

/* persistentStoragePath hint bar */
.persistent-storage-hint {
  display: flex;
  align-items: center;
  margin: 4px 4px 6px;
  padding: 6px 12px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
  border-left: 3px solid var(--el-border-color);
  border-radius: 4px;
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
