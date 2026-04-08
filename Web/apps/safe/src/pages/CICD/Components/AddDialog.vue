<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} CICD`"
    :close-on-click-modal="false"
    size="820px"
    :before-close="cancelAdd"
    destroy-on-close
    direction="rtl"
    :z-index="100000"
    append-to-body
    class="cicd-drawer"
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
              <div class="section-subtitle">Name, description, entry point and image</div>
            </div>
          </div>

          <el-form-item label="name" prop="displayName">
            <el-input v-model="form.displayName" :disabled="isEdit || isResume" />
          </el-form-item>
          <el-form-item label="description">
            <el-input v-model="form.description" :rows="2" type="textarea" />
          </el-form-item>
          <el-form-item label="entryPoint" prop="entryPoint">
            <el-input v-model="form.entryPoint" :rows="2" type="textarea" placeholder="If multi-line, it is best to save them in a file on NFS, and then execute it. e.g. bash run.sh" />
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
            <div>
              <div class="section-title">Resource</div>
              <div class="section-subtitle">Allocate CPU, GPU and memory</div>
            </div>
          </div>

          <el-row :gutter="20">
            <el-col :span="24">
              <el-form-item label="replica">
                <el-input placeholder="1" disabled />
              </el-form-item>
            </el-col>
            <el-col :span="24">
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

            <el-col :span="12">
              <el-form-item label="cpu" prop="resource.cpu">
                <el-input v-model="form.resource.cpu" :placeholder="placeholders.cpu" />
              </el-form-item>
            </el-col>
            <el-col :span="12" v-if="!flavorMaxVal || flavorMaxVal['amd.com/gpu']">
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

        <!-- ===== Advanced Options (collapsible) ===== -->
        <div class="section-card">
          <div
            class="section-header section-header--clickable"
            @click="advancedOpen = !advancedOpen"
          >
            <div class="section-bar"></div>
            <div class="flex-1">
              <div class="section-title">Advanced Options</div>
              <div class="section-subtitle">Toleration, retry, GitHub config and secrets</div>
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
                <el-col :span="12">
                  <el-form-item label="multiNodes" prop="unifiedJobEnable">
                    <el-switch v-model="form.unifiedJobEnable" :disabled="isEdit" />
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
                  <el-form-item label="maxRetry">
                    <el-input-number
                      v-model.number="form.maxRetry"
                      :min="0"
                      :max="50"
                      :step="1"
                      class="w-[120px] mr-2"
                    />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ RETRY_TIMES_INFO }}
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

              </el-row>

              <el-form-item label="GitHubConfigURL" prop="githubConfigUrl">
                <el-input
                  v-model="form.githubConfigUrl"
                  placeholder="Enter GitHub Config URL"
                  :disabled="isEdit"
                />
              </el-form-item>

              <el-form-item label="GitHubPAT" prop="githubPAT" v-if="!isEdit">
                <div class="flex items-center gap-2 w-full">
                  <el-input
                    v-model="form.githubPAT"
                    placeholder="Enter GitHub PAT"
                    type="password"
                    class="flex-1"
                  />
                  <el-tooltip placement="top" raw-content>
                    <template #content>
                      <div>
                        To create a PAT, visit
                        <a
                          href="https://app.docker.com/settings"
                          target="_blank"
                          rel="noopener noreferrer"
                          style="color: #409eff; text-decoration: underline"
                        >
                          https://app.docker.com/settings
                        </a>
                      </div>
                    </template>
                    <el-icon class="text-gray-500 cursor-help">
                      <InfoFilled />
                    </el-icon>
                  </el-tooltip>
                </div>
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
        <el-button type="primary" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, watch, ref, computed, nextTick, toRef } from 'vue'
import {
  addWorkload,
  getNodeFlavorAvail,
  getWorkloadDetail,
  editWorkload,
} from '@/services/workload/index'
import { getImagesList, getNodesList } from '@/services'
import { useSecrets, useSelectPaste } from '@/composables'
import { type FormInstance, ElMessage, ElMessageBox } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { byte2Gi, copyText } from '@/utils/index'
import type { FormItemRule } from 'element-plus'
import { encodeToBase64String, decodeFromBase64String } from '@/utils'
import { debounce } from 'lodash'
import { useUserStore } from '@/stores/user'
import { InfoFilled, CopyDocument, ArrowRight } from '@element-plus/icons-vue'
import ImageInput from '@/components/Base/ImageInput.vue'

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
const TOLERATE_INFO = 'If enabled, workloads can be scheduled to nodes with taints'
const RETRY_TIMES_INFO = 'Maximum retries:50'
const FORCE_HOST_NETWORK_INFO = 'Force host network (default: auto-based on resources)'

const imageOptions = ref([] as Array<{ id: number; tag: string }>)
const excludedNodeOptions = ref(
  [] as Array<{ nodeId: string; available: boolean; internalIP?: string }>,
)
const excludedNodesSearchQuery = ref('')
const nodesLoading = ref(false)
const excludedNodesSelectRef = ref()

// Use composable to fetch secrets
const { secretOptions, fetchSecrets } = useSecrets('image')

const pendingWorkspaceId = ref<string>('')
const targetWorkspaceId = computed<string>({
  get: () => pendingWorkspaceId.value || store.currentWorkspaceId || store.firstWorkspace || '',
  set: (val: string) => {
    pendingWorkspaceId.value = val
  },
})

const showAdvanced = ref(false)
const advancedOpen = ref(false)
const fetchWorkspaceOption = () => store.fetchWorkspace(true)

const initialForm = () => ({
  displayName: '',
  groupVersionKind: {
    kind: 'AutoscalingRunnerSet',
    version: 'v1',
  },
  description: '',
  entryPoint: '',
  isSupervised: false,
  image: '',
  maxRetry: 50,
  priority: 1,
  unifiedJobEnable: false,
  githubConfigUrl: '',
  githubPAT: '',
  resource: {
    replica: 1,
    cpu: '4',
    gpu: '0',
    memory: '16',
    ephemeralStorage: '100',
  },
  timeout: undefined,
  ttlSecondsAfterFinished: 10,
  excludedNodes: [] as string[],

  isTolerateAll: true,
  secretIds: [] as string[],
  forceHostNetwork: false,
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
  }
})

const nameRegex = /^[a-z](?:[-a-z0-9]{0,37}[a-z0-9])?$/

const ruleFormRef = ref<FormInstance>()
const rules: Record<string, FormItemRule[]> = reactive({
  displayName: [
    { required: true, message: 'Please input name', trigger: 'blur' },
    {
      pattern: nameRegex,
      message: 'Must start with lowercase letter, only a-z, 0-9, and "-" allowed, max 39 chars',
      trigger: 'blur',
    },
  ],
  entryPoint: [{ required: true, message: 'Please input entry point', trigger: 'blur' }],
  image: [{ required: true, message: 'Please input image', trigger: 'blur' }],
  'resource.cpu': [{ required: true, message: 'Please input cpu', trigger: 'blur' }],
  'resource.memory': [{ required: true, message: 'Please input memory', trigger: 'blur' }],
  'resource.ephemeralStorage': [
    { required: true, message: 'Please input ephemeral storage', trigger: 'blur' },
  ],
  githubConfigUrl: [{ required: true, message: 'Please input GitHub config URL', trigger: 'blur' }],
  githubPAT: [{ required: true, message: 'Please input GitHub PAT', trigger: 'blur' }],
})

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  if (!store.currentWorkspaceId) return
  try {
    await formEl.validate()

    const {
      resource,
      entryPoint,
      timeout,
      unifiedJobEnable,
      githubConfigUrl,
      githubPAT,
      image,
      ...addPayload
    } = form

    // Build user-submitted resource object for storing in env
    const userResource = {
      replica: 1,
      cpu: form.resource.cpu,
      gpu: form.resource.gpu || '0',
      memory: `${form.resource.memory}Gi`,
      sharedMemory: `${Math.floor(Number(form.resource.memory) / 2)}Gi`,
      ephemeralStorage: `${form.resource.ephemeralStorage}Gi`,
    }

    // Fixed resource values to send to the backend
    const fixedResource = {
      replica: 1,
      cpu: form.unifiedJobEnable ? '2' : '1',
      gpu: '0',
      memory: form.unifiedJobEnable ? '8Gi' : '4Gi',
      ephemeralStorage: '10Gi',
    }

    // Build environment variables with fixed keys
    const envMap: Record<string, string> = {
      UNIFIED_JOB_ENABLE: String(unifiedJobEnable),
      GITHUB_CONFIG_URL: githubConfigUrl,
      RESOURCES: JSON.stringify(userResource),
      IMAGE: form.image,
      ENTRYPOINT: encodeToBase64String(entryPoint),
    }

    // Handle GitHubPAT
    // During Create/Clone, add PAT directly to env; backend will auto-create secret
    if (!isEdit.value && githubPAT) {
      envMap.GITHUB_PAT = githubPAT
    }

    // Build secrets array
    const secrets = form.secretIds.map((id) => ({ id }))

    if (!isEdit.value) {
      // excludedNodes only works in replica mode (CICD is always in replica mode)
      const excludedNodesPayload = (() => {
        const arr = (addPayload.excludedNodes ?? []).filter(Boolean)
        return arr.length ? arr : undefined
      })()

      const payload: any = {
        ...addPayload,
        resources: [fixedResource],
        workspace: props.action === 'Clone' ? pendingWorkspaceId.value : store.currentWorkspaceId!,
        env: envMap,
        ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
        ...(props.action === 'Resume' ? { workloadId: props.wlid } : {}),
        ...(cachedUseWorkspaceStorage.value !== undefined ? { useWorkspaceStorage: cachedUseWorkspaceStorage.value } : {}),
      }

      await addWorkload(payload)
      ElMessage({ message: `${props.action} successful`, type: 'success' })
    } else {
      if (!props.wlid) return

      // During Edit, fetch existing env and update
      const res = await getWorkloadDetail(props.wlid)
      const editEnvMap = { ...res.env }

      // Update fixed keys
      editEnvMap.UNIFIED_JOB_ENABLE = String(form.unifiedJobEnable)
      editEnvMap.GITHUB_CONFIG_URL = form.githubConfigUrl
      editEnvMap.RESOURCES = JSON.stringify(userResource)
      editEnvMap.IMAGE = form.image
      editEnvMap.ENTRYPOINT = encodeToBase64String(form.entryPoint)

      await editWorkload(props.wlid, {
        description: form.description,
        priority: form.priority,
        maxRetry: form.maxRetry,
        resources: [fixedResource],
        env: editEnvMap,
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
  () => store.currentWorkspaceId,
  (cur) => {
    if (!pendingWorkspaceId.value || pendingWorkspaceId.value === cur) {
      pendingWorkspaceId.value = cur ?? ''
    }
  },
  { immediate: true },
)

const fetchFlavorAvail = async () => {
  const flavorId = store.currentNodeFlavor
  if (!flavorId) return
  const res = await getNodeFlavorAvail(flavorId)
  flavorMaxVal.value = res
  if (!res['amd.com/gpu']) form.resource.gpu = ''
  rules['resource.cpu'] = [
    { required: true, message: 'Please input cpu', trigger: 'blur' },
    createBetweenRule(1, res.cpu),
  ]
  rules['resource.memory'] = [
    { required: true, message: 'Please input memory', trigger: 'blur' },
    createBetweenRule(1, Number(byte2Gi(res.memory ?? 0, 0, false))),
  ]
  rules['resource.ephemeralStorage'] = [
    { required: true, message: 'Please input ephemeral storage', trigger: 'blur' },
    createBetweenRule(1, Number(byte2Gi(res['ephemeral-storage'] ?? 0, 0, false))),
  ]
}

const setInitialFormValues = async () => {
  if (!props.wlid) return

  const res = await getWorkloadDetail(props.wlid)
  cachedUseWorkspaceStorage.value = res.useWorkspaceStorage

  form.displayName = res.displayName
  form.description = res.description
  form.isSupervised = res.isSupervised ?? false
  form.maxRetry = res.maxRetry ?? 0
  form.timeout = res.timeout
  form.excludedNodes = res.excludedNodes ?? []
  form.isTolerateAll = res.isTolerateAll ?? false
  form.forceHostNetwork = res.forceHostNetwork ?? false

  // Regular users cannot select high priority; auto-downgrade to medium when cloning
  form.priority =
    isManager.value || store.isCurrentWorkspaceAdmin()
      ? res.priority
      : res.priority === 2
        ? 1
        : res.priority

  // Extract fixed keys from environment variables
  const envCopy = { ...res.env }
  form.unifiedJobEnable = envCopy.UNIFIED_JOB_ENABLE === 'true'
  form.githubConfigUrl =
    envCopy.GITHUB_CONFIG_URL || 'https://github.com/ROCm/unified-training-dockers'

  // Extract image and entryPoint from env (new format: IMAGES/ENTRYPOINTS as arrays; legacy: IMAGE/ENTRYPOINT as single values)
  const imagesFromEnv = envCopy.IMAGES ? (JSON.parse(envCopy.IMAGES) as string[]) : null
  const entryPointsFromEnv = envCopy.ENTRYPOINTS
    ? (JSON.parse(envCopy.ENTRYPOINTS) as string[])
    : null

  form.image =
    (imagesFromEnv && Array.isArray(imagesFromEnv) ? imagesFromEnv[0] : undefined) ??
    envCopy.IMAGE ??
    res.image ??
    ''

  const rawEntryPoint =
    (entryPointsFromEnv && Array.isArray(entryPointsFromEnv) ? entryPointsFromEnv[0] : undefined) ??
    envCopy.ENTRYPOINT ??
    res.entryPoint ??
    ''
  form.entryPoint = rawEntryPoint ? decodeFromBase64String(rawEntryPoint) : ''

  // Parse resource data from env.RESOURCES
  if (envCopy.RESOURCES) {
    try {
      const resourcesFromEnv = JSON.parse(envCopy.RESOURCES)
      form.resource.cpu = resourcesFromEnv.cpu || '4'
      form.resource.gpu = resourcesFromEnv.gpu || '0'
      form.resource.memory = (resourcesFromEnv.memory || '16Gi').replace(/Gi$/i, '')
      form.resource.ephemeralStorage = (resourcesFromEnv.ephemeralStorage || '100Gi').replace(
        /Gi$/i,
        '',
      )
    } catch (e) {
      // Fall back to defaults if parsing fails
      form.resource.cpu = '4'
      form.resource.gpu = '0'
      form.resource.memory = '16'
      form.resource.ephemeralStorage = '100'
    }
  } else {
    // Backward compatibility: read from original resource object if no RESOURCES env (legacy data)
    // resources changed to array, use first element
    const firstResource = res.resources?.[0] || res.resource || {}
    const { gpuName, rdmaResource, ...clearResource } = firstResource
    form.resource = clearResource
    form.resource.memory = firstResource?.memory?.replace(/Gi$/i, '') ?? ''
    form.resource.ephemeralStorage = firstResource?.ephemeralStorage?.replace(/Gi$/i, '') ?? ''
  }

  // Handle special logic for Edit, Clone, and Resume
  if (props.action === 'Edit' && res.secrets && res.secrets.length > 0) {
    form.secretIds = res.secrets.map((s: any) => s.id)
  } else if (props.action === 'Clone' || props.action === 'Resume') {
    // Clear sensitive info during Clone/Resume so users can re-select
    form.secretIds = []
    form.githubPAT = ''
  }

  if (props.action === 'Clone') {
    fetchWorkspaceOption()
  }
  await nextTick()
  ruleFormRef.value?.clearValidate()
}

const fetchNodes = async () => {
  const nodes = await getNodesList({
    workspaceId: store.currentWorkspaceId,
    limit: -1,
    brief: true,
  }).catch(() => ({ items: [] }))
  excludedNodeOptions.value = (nodes?.items ?? []).map((n: any) => ({
    nodeId: n.nodeId ?? n.name ?? n.hostname,
    available: Boolean(n.available),
    internalIP: n.internalIP,
  }))
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

const fetchImage = async (tag?: string) => {
  const res = await getImagesList({ flat: true, tag })
  imageOptions.value = res ?? []
}

const filterImageOptions = debounce(async (query: string) => {
  if (!query) {
    await fetchImage()
  } else {
    await fetchImage(query)
  }
}, 300)

const onOpen = async () => {
  showAdvanced.value = false
  cachedUseWorkspaceStorage.value = undefined
  pendingWorkspaceId.value = store.currentWorkspaceId ?? store.firstWorkspace ?? ''
  fetchFlavorAvail()
  fetchImage()
  fetchSecrets()

  if (props.action !== 'Create') {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())
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
