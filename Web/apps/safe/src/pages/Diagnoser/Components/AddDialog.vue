<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} Bench`"
    width="800"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 800px"
      class="p-5"
      :rules="rules"
    >
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Basic Information</span>
      </div>
      <el-form-item label="name" prop="name">
        <el-input v-model="form.name" />
      </el-form-item>
      <el-form-item label="entryPoint" prop="entryPoint">
        <el-input v-model="form.entryPoint" :rows="2" type="textarea" />
      </el-form-item>
      <el-form-item label="image" prop="image">
        <ImageInput v-model="form.image" />
      </el-form-item>

      <div class="flex items-center m-b-2">
        <div class="w-0.8 hx-13 bg-[var(--safe-muted)] mr-2 rounded-sm"></div>
        <span class="textx-13 font-medium">Resource</span>
      </div>

      <el-form-item label="Type" prop="inputsName">
        <el-select v-model="form.inputsName" @change="onTypeChange">
          <el-option
            v-for="type in props.config.availableTypes"
            :key="type"
            :label="type"
            :value="type"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Value" prop="inputsValue">
        <el-input
          v-if="form.inputsName === 'workload'"
          v-model="form.inputsValue"
          placeholder="Enter workload"
          clearable
          @change="onWorkloadInputDone"
          @keyup.enter="onWorkloadInputDone"
          @blur="onWorkloadInputDone"
        />
        <!-- Node type: use enhanced node selector -->
        <el-select
          v-else-if="form.inputsName === 'node'"
          v-model="form.inputsValue"
          multiple
          clearable
          filterable
          :filter-method="filterValueNodes"
          collapse-tags
          collapse-tags-tooltip
          :max-collapse-tags="5"
          placeholder="Select or paste nodes (comma-separated)"
          ref="valueNodesSelectRef"
          @visible-change="
            (visible: boolean) => handleValueNodesVisibleChange(valueNodesSelectRef, visible)
          "
        >
          <el-option
            v-for="n in filteredValueNodeOptions"
            :key="n.nodeId"
            :label="n.nodeId"
            :value="n.nodeId"
          >
            <div class="flex items-center justify-between w-full">
              <div class="truncate">
                <span>{{ n.nodeId }}</span>
                <span
                  v-if="valueNodeSearchQuery && n.internalIP"
                  class="text-gray-400 text-xs ml-2"
                >
                  ({{ n.internalIP }})
                </span>
              </div>
              <div class="flex items-center gap-1">
                <el-tag :type="n.available ? 'success' : 'danger'" size="small" effect="plain">
                  {{ n.available ? 'Available' : 'Unavailable' }}
                </el-tag>
                <el-tag :type="n.occupied ? 'warning' : 'info'" size="small" effect="plain">
                  {{ n.occupied ? 'Occupied' : 'Free' }}
                </el-tag>
              </div>
            </div>
          </el-option>
        </el-select>
        <!-- Other types: regular selector -->
        <el-select
          v-else
          v-model="form.inputsValue"
          :multiple="isMulti"
          :placeholder="placeholders[form.inputsName!]"
          :disabled="props.config.fixedWorkspace && form.inputsName === 'workspace'"
          filterable
          clearable
          @change="onValueChange"
        >
          <el-option
            v-for="opt in options"
            :key="opt.value"
            :label="opt.label"
            :value="opt.value"
          />
        </el-select>
      </el-form-item>

      <el-row :gutter="20">
        <el-col :span="12">
          <el-form-item label="cpu" prop="resource.cpu">
            <el-input v-model="form.resource.cpu" :placeholder="resourcePlaceholders.cpu" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="gpu">
            <el-input v-model="form.resource.gpu" :placeholder="resourcePlaceholders.gpu" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="memory" prop="resource.memory">
            <el-input v-model="form.resource.memory" :placeholder="resourcePlaceholders.memory">
              <template #append>Gi</template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="ephemeralStorage" prop="resource.ephemeralStorage">
            <el-input
              v-model="form.resource.ephemeralStorage"
              :placeholder="resourcePlaceholders.ephemeralStorage"
            >
              <template #append>Gi</template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="24" v-if="form.inputsName === 'workspace'">
          <el-form-item label="replica">
            <el-input-number
              v-model="form.replica"
              :min="0"
              :max="selectedWorkspaceNodeCount"
            />
            <el-text size="small" type="info" class="ml-2">
              Optional. Max = workspace node count ({{ selectedWorkspaceNodeCount ?? '-' }})
            </el-text>
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="20">
        <el-col :span="12">
          <el-form-item label="Toleration">
            <el-switch v-model="form.isTolerateAll" class="m-r-2" />
            <el-text class="mx-1" size="small" type="info"
              ><el-icon class="m-r-1"><InfoFilled /></el-icon>{{ TOLERATE_INFO }}</el-text
            >
          </el-form-item>
        </el-col>
        <el-col :span="12" v-if="form.inputsName === 'cluster' || form.inputsName === 'workspace'">
          <el-form-item label="securityOperation">
            <el-switch v-model="form.securityOperation" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item label="Timeout Second">
        <el-input-number v-model="form.timeoutSecond" />
      </el-form-item>

      <el-form-item label="environmentVariables">
        <KeyValueList v-model="form.envList" :max="20" keyMode="input" info="Add up to 20 envs" />
      </el-form-item>

      <el-form-item label="excludedNodes" v-if="form.inputsName !== 'node'">
        <el-select
          v-model="form.excludedNodes"
          multiple
          clearable
          filterable
          :filter-method="filterNodes"
          collapse-tags
          collapse-tags-tooltip
          :max-collapse-tags="5"
          placeholder="Select or paste nodes to exclude (comma-separated)"
          ref="excludedNodesSelectRef"
          @visible-change="
            (visible: boolean) => handleExcludedNodesVisibleChange(excludedNodesSelectRef, visible)
          "
        >
          <el-option
            v-for="n in filteredExcludedOptions"
            :key="n.nodeId"
            :label="n.nodeId"
            :value="n.nodeId"
          >
            <div class="flex items-center justify-between w-full">
              <div class="truncate">
                <span>{{ n.nodeId }}</span>
                <span v-if="searchQuery && n.internalIP" class="text-gray-400 text-xs ml-2">
                  ({{ n.internalIP }})
                </span>
              </div>
              <div class="flex items-center gap-1">
                <el-tag :type="n.available ? 'success' : 'danger'" size="small" effect="plain">
                  {{ n.available ? 'Available' : 'Unavailable' }}
                </el-tag>
                <el-tag :type="n.occupied ? 'warning' : 'info'" size="small" effect="plain">
                  {{ n.occupied ? 'Occupied' : 'Free' }}
                </el-tag>
              </div>
            </div>
          </el-option>
        </el-select>
      </el-form-item>
      <el-form-item label="hostpath">
        <el-select
          v-model="form.hostpath"
          multiple
          clearable
          filterable
          allow-create
          default-first-option
          collapse-tags
          collapse-tags-tooltip
          :max-collapse-tags="4"
          :reserve-keyword="false"
          placeholder="Enter a single hostpath (Enter to add)"
          style="width: 100%"
          size="default"
          @change="dedupeHostpath"
        >
        </el-select>
      </el-form-item>

      <el-form-item label="Workspace" v-if="props.action === 'Clone'">
        <el-select v-model="targetWorkspaceId" class="w-[200px]" @change="onWorkspaceChange">
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="onCancel">Cancel</el-button>
        <el-button type="primary" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import {
  defineProps,
  defineEmits,
  reactive,
  onMounted,
  ref,
  computed,
  nextTick,
  watch,
  toRef,
} from 'vue'
import {
  getNodesList,
  getNodeFlavorAvail,
  addOpsjobs,
  getWorkloadDetail,
  getOpsjobsDetail,
  getImagesList,
} from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import ImageInput from '@/components/Base/ImageInput.vue'
import { useClusterStore } from '@/stores/cluster'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  byte2Gi,
  encodeToBase64String,
  convertListToKeyValueMap,
  decodeFromBase64String,
  convertKeyValueMapToList,
  copyText,
} from '@/utils/index'
import { debounce } from 'lodash'
import { useSelectPaste } from '@/composables'

const clusterStore = useClusterStore()
const wsStore = useWorkspaceStore()

const pendingWorkspaceId = ref<string>('')
const targetWorkspaceId = computed<string>({
  get: () => pendingWorkspaceId.value || wsStore.currentWorkspaceId || wsStore.firstWorkspace || '',
  set: (val: string) => {
    pendingWorkspaceId.value = val
  },
})
const fetchWorkspaceOption = () => wsStore.fetchWorkspace(true)

const TOLERATE_INFO = 'If enabled, opsjobs can be scheduled to nodes with taints'

type Option = { label: string; value: string }
type ImageOption = { id: number; tag: string }

export interface PreflightConfig {
  mode: 'system' | 'workspace'
  allowCluster: boolean
  fixedWorkspace: boolean
  workspaceId?: string
  availableTypes: Array<'node' | 'cluster' | 'workspace' | 'workload'>
  defaultType?: 'node' | 'cluster' | 'workspace' | 'workload'
}

const props = withDefaults(
  defineProps<{
    visible: boolean
    jobid?: string
    action: string
    config?: PreflightConfig
  }>(),
  {
    config: () => ({
      mode: 'system',
      allowCluster: true,
      fixedWorkspace: false,
      availableTypes: ['node', 'cluster', 'workspace', 'workload'],
    }),
  },
)
const emit = defineEmits(['update:visible', 'success'])

type NodeOption = {
  nodeId: string
  available: boolean
  internalIP?: string
  occupied: boolean // Whether occupied (determined by workloads array)
}

// Raw node data returned from API
interface NodeRawData {
  nodeId: string
  available: boolean
  internalIP?: string
  workloads?: Array<{ id: string; userId: string; [key: string]: unknown }>
  [key: string]: unknown
}

const state = reactive({
  nodeOptions: [] as NodeOption[], // Full node data (for display and search)
  excludedOptions: [] as NodeOption[], // Full excluded node data (for display and search)
  imageOptions: [] as ImageOption[],
  // Simplified node data (for paste matching only)
  allNodesForPaste: [] as Array<{ label: string; value: string }>,
  allExcludedNodesForPaste: [] as Array<{ label: string; value: string }>,
})

const searchQuery = ref('')
const valueNodeSearchQuery = ref('')
const excludedNodesSelectRef = ref()
const valueNodesSelectRef = ref()

// Generic node filtering function
const filterNodesByQuery = (nodes: NodeOption[], query: string) => {
  if (!query) return nodes
  const lowerQuery = query.toLowerCase()
  return nodes.filter(
    (n) =>
      n.nodeId.toLowerCase().includes(lowerQuery) ||
      (n.internalIP && n.internalIP.toLowerCase().includes(lowerQuery)),
  )
}

// Filtered node options
const filteredValueNodeOptions = computed(() =>
  filterNodesByQuery(state.nodeOptions, valueNodeSearchQuery.value),
)

const filteredExcludedOptions = computed(() =>
  filterNodesByQuery(state.excludedOptions, searchQuery.value),
)

// Filter methods
const filterValueNodes = (query: string) => {
  valueNodeSearchQuery.value = query
}

const filterNodes = (query: string) => {
  searchQuery.value = query
}

const fetchImage = async (tag?: string) => {
  const res = await getImagesList({ flat: true, tag })
  state.imageOptions = res ?? []
}

type InputsNameType = 'node' | 'cluster' | 'workspace' | 'workload' | undefined
const initialForm = () => ({
  name: '',
  entryPoint: '',
  image: '',

  inputsName: undefined as InputsNameType,
  inputsValue: undefined as string | string[] | undefined,
  type: 'preflight',
  isTolerateAll: false,
  timeoutSecond: 3600,
  envList: [
    {
      key: '',
      value: '',
    },
  ],
  resource: {
    cpu: '96',
    gpu: '8',
    memory: '1000',
    ephemeralStorage: '400',
  },
  excludedNodes: [],
  hostpath: [] as string[],
  securityOperation: false,
  replica: 0 as number | undefined,
})
const form = reactive({ ...initialForm() })

// Fetch all node data
const fetchAllNodes = async (
  params: { clusterId?: string; workspaceId?: string } = {},
  target: 'nodes' | 'excluded' | 'both' = 'both',
) => {
  try {
    // In workspace mode, always add workspace filter condition
    const finalParams =
      props.config.fixedWorkspace && props.config.workspaceId
        ? { ...params, workspaceId: props.config.workspaceId }
        : params

    const res = await getNodesList({
      ...finalParams,
      limit: -1, // Fetch all data
    })

    const nodesFull = (res?.items || []).map(
      (node: NodeRawData): NodeOption => ({
        nodeId: node.nodeId,
        available: node.available,
        internalIP: node.internalIP,
        occupied: Array.isArray(node.workloads) && node.workloads.length > 0,
      }),
    )

    const nodesForPaste = nodesFull.map((node: NodeOption) => ({
      label: node.nodeId,
      value: node.nodeId,
    }))

    // Update corresponding data based on target
    if (target === 'both' || target === 'nodes') {
      state.nodeOptions = nodesFull
      state.allNodesForPaste = nodesForPaste
    }
    if (target === 'both' || target === 'excluded') {
      state.excludedOptions = nodesFull
      state.allExcludedNodesForPaste = nodesForPaste
    }
  } catch (error) {
    console.error('Failed to fetch nodes:', error)
  }
}

// Use composable for nodes paste functionality (using full data)
const { handleSelectVisibleChange: handleValueNodesVisibleChange } = useSelectPaste({
  options: computed(() => state.allNodesForPaste),
  modelValue: computed({
    get: () => (Array.isArray(form.inputsValue) ? form.inputsValue : []),
    set: (val) => {
      form.inputsValue = val
    },
  }),
  successMessagePrefix: 'Matched and selected',
  warningMessagePrefix: 'Could not find nodes',
})

// Use composable for excludedNodes paste functionality (using full data)
const { handleSelectVisibleChange: handleExcludedNodesVisibleChange } = useSelectPaste({
  options: computed(() => state.allExcludedNodesForPaste),
  modelValue: toRef(form, 'excludedNodes'),
  successMessagePrefix: 'Matched and excluded',
  warningMessagePrefix: 'Could not find nodes',
})

const copyImage = async () => {
  if (!form.image) return
  await copyText(form.image)
}

const isMulti = computed(() => form.inputsName === 'node')
const placeholders: Record<'node' | 'cluster' | 'workspace', string> = {
  node: 'Select node(s)',
  cluster: 'Select cluster',
  workspace: 'Select workspace',
}

const dedupeHostpath = (val: string[]) => {
  form.hostpath = [...new Set(val.map((s) => s.trim()).filter(Boolean))]
}

const flavorMaxVal = ref()
const resourcePlaceholders = computed(() => {
  const val = flavorMaxVal.value ?? {}
  return {
    cpu: `1 to ${val.cpu ?? '-'}`,
    gpu: `0 to ${val['amd.com/gpu'] ?? '-'}`,
    memory: val.memory ? `1 to ${byte2Gi(val.memory, undefined, false)}` : '1 to - Gi',
    ephemeralStorage: val['ephemeral-storage']
      ? `1 to ${byte2Gi(val['ephemeral-storage'], undefined, false)}`
      : '1 to - Gi',
    replica: `Replica count`,
  }
})

const ruleFormRef = ref<FormInstance>()
const rules = reactive<FormRules>({
  name: [{ required: true, message: 'Please input name', trigger: 'blur' }],
  inputsName: [{ required: true, message: 'Please select type', trigger: 'change' }],
  entryPoint: [{ required: true, message: 'Please input entry point', trigger: 'blur' }],
  image: [{ required: true, message: 'Please input image', trigger: 'blur' }],
  inputsValue: [
    {
      // Dynamic validation by type: node requires at least 1 selection; others need non-empty string
      validator: (_r, v, cb) => {
        if (form.inputsName === 'node') {
          return Array.isArray(v) && v.length > 0
            ? cb()
            : cb(new Error('Please select at least one node'))
        }
        return typeof v === 'string' && v ? cb() : cb(new Error('Please select value'))
      },
      trigger: ['change', 'blur'],
    },
  ],
})

async function onTypeChange() {
  // Clear current selection value
  form.inputsValue = undefined
  ruleFormRef.value?.clearValidate('inputsValue')

  switch (form.inputsName) {
    case 'cluster':
      form.inputsValue = clusterStore.currentClusterId || ''
      await fetchAllNodes({ clusterId: clusterStore.currentClusterId }, 'both')
      break
    case 'workspace':
      if (props.action === 'Clone' && pendingWorkspaceId.value) {
        form.inputsValue = pendingWorkspaceId.value
        await fetchAllNodes({ workspaceId: pendingWorkspaceId.value }, 'both')
      } else if (props.config.fixedWorkspace) {
        form.inputsValue = props.config.workspaceId
        await fetchAllNodes({ workspaceId: props.config.workspaceId }, 'both')
      }
      break
    case 'node':
      // Node type: node data already loaded on mount, no need to re-fetch
      break
    default:
      // Clear excludedOptions for other types
      state.excludedOptions = []
      state.allExcludedNodesForPaste = []
  }
}

async function onValueChange() {
  if (form.inputsName === 'workspace') {
    // In fixed workspace mode, use configured workspaceId
    const workspaceId = props.config.fixedWorkspace
      ? props.config.workspaceId
      : typeof form.inputsValue === 'string'
        ? form.inputsValue
        : undefined
    await fetchAllNodes({ workspaceId }, 'excluded')
  }
}

const onWorkloadInputDone = async () => {
  if (form.inputsName !== 'workload') return

  const workloadId = typeof form.inputsValue === 'string' ? form.inputsValue.trim() : ''
  if (!workloadId) {
    state.excludedOptions = []
    state.allExcludedNodesForPaste = []
    return
  }

  try {
    // In workspace mode, use current workspace directly
    if (props.config.fixedWorkspace && props.config.workspaceId) {
      await fetchAllNodes({ workspaceId: props.config.workspaceId }, 'excluded')
      return
    }

    // System mode: query workspace via workloadId
    const detail = await getWorkloadDetail(workloadId)
    if (detail?.workspaceId) {
      await fetchAllNodes({ workspaceId: detail.workspaceId }, 'excluded')
    } else {
      state.excludedOptions = []
      state.allExcludedNodesForPaste = []
      ElMessage.warning('Workspace not found for this workload.')
    }
  } catch (_e) {
    state.excludedOptions = []
    state.allExcludedNodesForPaste = []
  }
}

// Convert both strings and objects to {label, value}
const toOption = (
  x: string | Record<string, unknown>,
  labelKey: string,
  valueKey: string,
): Option => {
  if (typeof x === 'string') return { label: x, value: x }
  const obj = x as Record<string, unknown>
  return {
    label: String(obj[labelKey] ?? obj.name ?? obj.label ?? obj.id ?? ''),
    value: String(obj[valueKey] ?? obj.id ?? obj.value ?? ''),
  }
}
const options = computed<Option[]>(() => {
  switch (form.inputsName) {
    case 'workspace':
      return (wsStore.items ?? []).map((w) => toOption(w, 'workspaceName', 'workspaceId'))
    case 'cluster':
      // Compatible with clusterId/clusterName or string arrays
      return (clusterStore.items ?? []).map((c) => toOption(c, 'clusterId', 'clusterId'))
    case 'node':
      // Node type no longer uses options, uses filteredValueNodeOptions instead
      return []
    default:
      return []
  }
})

const selectedWorkspaceNodeCount = computed(() => {
  if (form.inputsName !== 'workspace') return undefined
  const wsId = typeof form.inputsValue === 'string' ? form.inputsValue : undefined
  if (!wsId) return undefined
  return wsStore.items?.find((w) => w.workspaceId === wsId)?.currentNodeCount
})

const onCancel = () => {
  ruleFormRef.value?.clearValidate()
  emit('update:visible', false)
}

const onWorkspaceChange = (newWsId: string) => {
  if (form.inputsName === 'workspace') {
    form.inputsValue = newWsId
  } else if (form.inputsName === 'node' || form.inputsName === 'workload') {
    form.inputsValue = form.inputsName === 'node' ? [] : undefined
    form.excludedNodes = []
  }
  ruleFormRef.value?.clearValidate('inputsValue')
  fetchAllNodes({ workspaceId: newWsId }, 'both')
}

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    const { envList, inputsName, inputsValue, excludedNodes, resource, entryPoint, ...rest } = form

    const normalizeToArray = (v: unknown) =>
      Array.isArray(v) ? v : v == null || v === '' ? [] : [v]

    const inputsArr = normalizeToArray(inputsValue)
      .filter(Boolean)
      .map((v) => ({ name: String(inputsName), value: String(v) }))

    // Clean up resource / excludedNodes
    const pickNonEmpty = <T extends Record<string, unknown>>(obj?: T) => {
      if (!obj) return undefined
      const out: Record<string, unknown> = {}
      for (const [k, v] of Object.entries(obj)) {
        if (typeof v === 'string') {
          if (v.trim() !== '') out[k] = v
        } else if (v !== null && v !== undefined) {
          out[k] = v
        }
      }
      return Object.keys(out).length ? (out as T) : undefined
    }

    const baseResource = {
      cpu: form.resource.cpu,
      gpu: form.resource.gpu,
      memory: `${form.resource.memory}Gi`,
      ephemeralStorage: `${form.resource.ephemeralStorage}Gi`,
    }

    const resourcePayload = pickNonEmpty(baseResource)
    const excludedNodesPayload = (() => {
      const arr = (excludedNodes ?? []).filter(Boolean)
      return arr.length ? arr : undefined
    })()

    const resolveWorkspaceId = () => {
      if (props.action === 'Clone' && pendingWorkspaceId.value) {
        return pendingWorkspaceId.value
      }
      if (props.config.fixedWorkspace && props.config.workspaceId) {
        return props.config.workspaceId
      }
      return undefined
    }

    const workspaceId = resolveWorkspaceId()

    const payload = {
      ...rest,
      ...(inputsArr.length ? { inputs: inputsArr } : {}),
      ...(resourcePayload ? { resource: resourcePayload } : {}),
      ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
      ...(rest.hostpath?.length ? { hostpath: rest.hostpath } : {}),
      ...(workspaceId ? { workspaceId } : {}),
      ...(form.securityOperation ? { securityOperation: true } : {}),
      ...(form.replica ? { replica: form.replica } : {}),
      env: convertListToKeyValueMap(envList),
      entryPoint: encodeToBase64String(entryPoint),
    }

    await addOpsjobs(payload)
    ElMessage.success('Create successful')

    if (props.action === 'Clone' && pendingWorkspaceId.value && pendingWorkspaceId.value !== wsStore.currentWorkspaceId) {
      wsStore.setCurrentWorkspace(pendingWorkspaceId.value)
      await wsStore.fetchWorkspace(true)
    }

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey)
      ElMessage.error(firstMsg)
    }
  }
}

onMounted(() => {
  // Load initial full data
  fetchAllNodes({}, 'both')
})

const parseInputs = (inputs?: Array<{ name: string; value: string }>) => ({
  inputsName: inputs?.[0]?.name || '',
  inputsValue: inputs?.map((i) => i.value ?? '') || [],
})
const setInitialFormValues = async () => {
  if (!props.jobid) return

  const res = await getOpsjobsDetail(props.jobid)

  // name-jobName
  // entryPoint - decode base64
  // image
  // inputs split into inputsName and value
  // type
  // isTolerateAll
  // timeoutSecond
  // envList - convert env
  // resource-cpu gpuSame as memory — strip unit from ephemeralStorage
  // excludedNodes - unknown
  // hostpath

  form.name = res.jobName
  form.entryPoint = decodeFromBase64String(res.entryPoint)
  form.image = res.image
  form.type = res.type

  Object.assign(form, parseInputs(res.inputs))

  const { gpuName: _gpuName, rdmaResource: _rdmaResource, ...clearResource } = res.resource
  form.resource = clearResource
  form.resource.memory = res.resource?.memory?.replace(/Gi$/i, '') ?? ''
  form.resource.ephemeralStorage = res.resource?.ephemeralStorage?.replace(/Gi$/i, '') ?? ''

  form.isTolerateAll = res.isTolerateAll ?? false
  form.timeoutSecond = res.timeoutSecond
  form.hostpath = res.hostpath
  form.excludedNodes = res.excludedNodes ?? []

  if (props.config.fixedWorkspace && form.inputsName === 'workspace') {
    form.inputsValue = wsStore.currentWorkspaceId
  }

  form.securityOperation = res.securityOperation ?? false
  form.replica = res.replica ?? undefined
  form.envList = convertKeyValueMapToList(res.env ?? [])

  if (props.action === 'Clone') {
    fetchWorkspaceOption()
  }
}

const filterImageOptions = debounce((query: string) => fetchImage(query || undefined), 300)

const onOpen = async () => {
  pendingWorkspaceId.value = wsStore.currentWorkspaceId ?? wsStore.firstWorkspace ?? ''
  // Load all node data
  fetchAllNodes()

  if (props.action !== 'Create') {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())

    // Set default type
    if (props.config.defaultType && props.action === 'Create') {
      form.inputsName = props.config.defaultType
      await onTypeChange()
    }
  }
  await nextTick()
}

watch(
  () => wsStore.currentWorkspaceId,
  (cur) => {
    if (!pendingWorkspaceId.value || pendingWorkspaceId.value === cur) {
      pendingWorkspaceId.value = cur ?? ''
    }
  },
  { immediate: true },
)

watch(
  () => wsStore.currentNodeFlavor,
  async (flavorId) => {
    if (!flavorId) return

    const res = await getNodeFlavorAvail(flavorId)
    flavorMaxVal.value = res
  },
  { immediate: true },
)
</script>
