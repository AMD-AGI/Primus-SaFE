<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} TorchFT`"
    :close-on-click-modal="false"
    size="820px"
    :before-close="cancelAdd"
    destroy-on-close
    direction="rtl"
    :z-index="100000"
    append-to-body
    class="training-drawer"
    @open="onOpen"
  >
    <!-- Middle content area: scrollable -->
    <div class="drawer-body">
      <el-form ref="ruleFormRef" :model="form" :rules="rules" label-width="120px" :validate-on-rule-change="false">
        <!-- ===== Basic Information ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Basic Information</div>
              <div class="section-subtitle">Name, description, entry point and image</div>
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
                  <el-option
                    label="High"
                    :value="2"
                    v-if="isManager || store.isCurrentWorkspaceAdmin()"
                  />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="description">
            <el-input v-model="form.description" type="textarea" :rows="2" />
          </el-form-item>

          <el-form-item label="entryPoint" prop="entryPoint">
            <el-input v-model="form.entryPoint" type="textarea" :rows="2" />
          </el-form-item>

          <el-form-item label="image" prop="image">
            <ImageInput v-model="form.image" />
          </el-form-item>
        </div>

        <!-- ===== Resource ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div class="flex-1">
              <div class="section-title">Resource</div>
              <div class="section-subtitle">Configure Worker Group and Lighthouse resources</div>
            </div>
          </div>

          <!-- Lighthouse -->
          <div class="mb-4">
            <div class="resource-group-title mb-3">Lighthouse</div>
            <el-row :gutter="16">
              <!-- lighthouse replica -->
              <el-col :span="24">
                <el-form-item label="replica" prop="lighthouseResource.replica">
                  <el-input v-model.number="form.lighthouseResource.replica" placeholder="1" />
                </el-form-item>
              </el-col>

              <!-- lighthouse resources (no GPU) -->
              <el-col :span="12">
                <el-form-item label="cpu" prop="lighthouseResource.cpu">
                  <el-input v-model="form.lighthouseResource.cpu" :placeholder="placeholders.cpu" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="memory" prop="lighthouseResource.memory">
                  <el-input
                    v-model="form.lighthouseResource.memory"
                    :placeholder="placeholders.memory"
                  >
                    <template #append>Gi</template>
                  </el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="ephemeral" prop="lighthouseResource.ephemeralStorage">
                  <el-input
                    v-model="form.lighthouseResource.ephemeralStorage"
                    :placeholder="placeholders.ephemeralStorage"
                  >
                    <template #append>Gi</template>
                  </el-input>
                </el-form-item>
              </el-col>
            </el-row>
          </div>

          <el-divider />

          <!-- Worker Group -->
          <div class="mb-4">
            <div class="flex items-center justify-between mb-3">
              <div class="resource-group-title">Worker Group</div>
              <el-segmented
                v-model="form.resourceType"
                :options="isEdit ? ['replicas'] : ['replicas', 'nodes']"
                size="small"
              />
            </div>
            <el-row :gutter="16">
              <!-- replicas / nodes selection section -->
              <el-col :span="24" v-if="form.resourceType === 'replicas'">
                <el-form-item label="nodePerGroup" prop="resource.replica">
                  <el-input
                    v-model.number="form.resource.replica"
                    :placeholder="placeholders.replica"
                    :disabled="isEdit"
                  />
                </el-form-item>
              </el-col>

              <!-- Node selection when creating or cloning -->
              <el-col :span="24" v-if="!isEdit && form.resourceType === 'nodes'">
                <el-form-item label="nodes" prop="nodeList">
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
                      :loading="nodesLoading"
                      @visible-change="
                        async (visible: boolean) => {
                          if (visible) await fetchNodesOnDropdown()
                          handleNodesVisibleChange(nodeSelectRef, visible)
                        }
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
                          <el-tag
                            :type="n.available ? 'success' : 'danger'"
                            size="small"
                            effect="plain"
                          >
                            {{ n.available ? 'Available' : 'Unavailable' }}
                          </el-tag>
                        </div>
                      </el-option>
                    </el-select>
                  </div>
                </el-form-item>
              </el-col>

              <!-- excludedNodes -->
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
                        <el-tag
                          :type="n.available ? 'success' : 'danger'"
                          size="small"
                          effect="plain"
                        >
                          {{ n.available ? 'Available' : 'Unavailable' }}
                        </el-tag>
                      </div>
                    </el-option>
                  </el-select>
                </el-form-item>
              </el-col>

              <el-col :span="24">
                <el-form-item label="replicaCount" prop="replicaCount">
                  <el-input v-model.number="form.replicaCount" placeholder="Replica Count" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="minReplicaCount" prop="minReplicaCount">
                  <el-input
                    v-model.number="form.minReplicaCount"
                    placeholder="Min Replica Count"
                    :disabled="isEdit"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="maxReplicaCount" prop="maxReplicaCount">
                  <el-input
                    v-model.number="form.maxReplicaCount"
                    placeholder="Max Replica Count"
                    :disabled="isEdit"
                  />
                </el-form-item>
              </el-col>

              <!-- Resource 2x2 grid -->
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
                <el-form-item label="ephemeral" prop="resource.ephemeralStorage">
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
        </div>

        <!-- ===== Advanced Options (collapsible header) ===== -->
        <div class="section-card">
          <!-- Card header: click entire row to expand/collapse -->
          <div
            class="section-header section-header--clickable"
            @click="advancedOpen = !advancedOpen"
          >
            <div class="section-bar"></div>
            <div class="flex-1">
              <div class="section-title">Advanced Options</div>
              <div class="section-subtitle">Retry, timeout, scheduler and metadata</div>
            </div>
            <el-icon :class="['section-chevron', { 'is-open': advancedOpen }]">
              <ArrowRight />
            </el-icon>
          </div>

          <!-- Expanded content -->
          <transition name="fade-slide">
            <div v-show="advancedOpen" class="advanced-body">
              <el-row :gutter="16">
                <!-- hangCheck -->
                <el-col :span="12" v-if="!isEdit">
                  <el-form-item label="hangCheck">
                    <el-switch v-model="form.isSupervised" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ HANG_CHECK_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>

                <!-- preheat -->
                <el-col :span="12">
                  <el-form-item label="preheat">
                    <el-switch v-model="form.preheat" :disabled="isEdit" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ PREHEAT_INFO }}
                    </el-text>
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


                <!-- autoRecovery -->
                <el-col :span="12">
                  <el-form-item label="autoRecovery">
                    <el-switch v-model="isRetry" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ AUTO_RETRY_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>

                <!-- maxRetry -->
                <el-col :span="12" v-if="isRetry">
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
                <el-col :span="12" v-else />

                   <!-- timeout -->
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


                <!-- schedulerTime -->
                <el-col :span="12">
                  <el-form-item label="schedulerTime">
                    <el-date-picker
                      v-model="form.schedulerTime"
                      type="datetime"
                      placeholder="Pick scheduler time"
                      format="YYYY-MM-DD HH:mm:ss"
                      value-format="YYYY-MM-DD HH:mm"
                      date-format="MMM DD, YYYY"
                      time-format="HH:mm"
                      :disabled-date="disabledDate"
                      :disabled-hours="disabledHours"
                      :disabled-minutes="disabledMinutes"
                      :disabled-seconds="disabledSeconds"
                      :default-time="midnightDefault"
                    />
                    <el-text size="small" type="info" class="ml-2">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ SCHEDULER_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>

                <!-- dependencies -->
                <el-col :span="12">
                  <el-form-item label="dependencies">
                    <el-select
                      v-model="form.dependencies"
                      multiple
                      clearable
                      filterable
                      collapse-tags
                      collapse-tags-tooltip
                      :max-collapse-tags="5"
                      :disabled="isEdit"
                      placeholder="Select one or more dependencies"
                    >
                      <el-option
                        v-for="n in wlOptions"
                        :key="n.value"
                        :label="n.label"
                        :value="n.value"
                      />
                    </el-select>
                  </el-form-item>
                </el-col>

                <!-- Workspace -->
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

                <!-- secret -->
                <el-col :span="12">
                  <el-form-item label="secret" prop="secretIds">
                    <el-select
                      v-model="form.secretIds"
                      multiple
                      :disabled="isEdit"
                      placeholder="Please select secrets"
                    >
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

              <!-- labels / env full width -->
              <el-form-item label="labels" v-if="!isEdit" class="kv-full">
                <KeyValueList
                  v-model="form.labelList"
                  :max="20"
                  keyMode="input"
                  info="Add up to 20 labels"
                  :validate="true"
                />
              </el-form-item>

              <el-form-item label="env vars" class="kv-full">
                <KeyValueList
                  v-model="form.envList"
                  :max="20"
                  keyMode="input"
                  info="Add up to 20 envs"
                />
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
import { encodeToBase64String, toUTCISOString, decodeScheduleFromApi } from '@/utils'
import { debounce } from 'lodash'
import { useDatetimeLimit } from '@/composables/useDatetimeLimit'
import dayjs from 'dayjs'

const props = defineProps<{
  visible: boolean
  wlid?: string
  action: string
}>()
const emit = defineEmits(['update:visible', 'success'])

const isEdit = computed(() => props.action === 'Edit')
const isCustomNodes = ref(false) // Flag for custom nodes
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
const nodesLoading = ref(false)

const AUTO_RETRY_INFO = 'automatically retry after workload failure'
const TIMEOUT_INFO = 'timeout duration in seconds'
const SCHEDULER_INFO = 'Scheduled execution time'
const RETRY_TIMES_INFO = 'Maximum retries:50'
const HANG_CHECK_INFO = 'workload fails if the last node(by rank) has no logs for 20 minutes'
const PREHEAT_INFO = 'preheat: When enabled, preheats the image, which increases workload duration.'
const NODES_AFFINITY_INFO = 'Node affinity: Required (strict) or Preferred (best-effort)'
const FORCE_HOST_NETWORK_INFO = 'Force host network (default: auto-based on resources)'

const advancedOpen = ref(false)

// Prevent directly overwriting store data
const pendingWorkspaceId = ref<string>('')
// Advanced options (keep data synced)
const targetWorkspaceId = computed<string>({
  get: () => pendingWorkspaceId.value || store.currentWorkspaceId || store.firstWorkspace || '',
  set: (val: string) => {
    pendingWorkspaceId.value = val
  },
})

const copyImage = async () => {
  if (!form.image) return
  await copyText(form.image)
}

const showAdvanced = ref(false)
const fetchWorkspaceOption = () => store.fetchWorkspace(true)

const curPriority = computed(() => (isManager.value || store.isCurrentWorkspaceAdmin() ? 2 : 1))

const initialForm = () => ({
  displayName: '',
  groupVersionKind: {
    kind: 'TorchFT',
    version: 'v1',
  },
  description: '',
  entryPoint: '',
  isSupervised: false,
  image: '',
  maxRetry: 5,
  priority: unref(curPriority),
  // Worker Group
  resource: {
    replica: undefined as number | undefined,
    cpu: '',
    gpu: '',
    memory: '',
    ephemeralStorage: '',
  },
  // Lighthouse
  lighthouseResource: {
    replica: 1,
    cpu: '2',
    memory: '4',
    ephemeralStorage: '20',
  },
  // TorchFT specific fields
  replicaCount: undefined as number | undefined,
  minReplicaCount: undefined as number | undefined,
  maxReplicaCount: undefined as number | undefined,

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
  dependencies: [],
  excludedNodes: [] as string[],

  timeout: undefined,
  schedulerTime: '',

  secretIds: [] as string[],
  preheat: false,
  nodesAffinity: '' as '' | 'required' | 'preferred',
  forceHostNetwork: false,
})
const form = reactive({ ...initialForm() })
const isRetry = ref(false) // isAutoRetry

const clonedLastNodes = ref<string[]>([])
watch(() => form.nodesAffinity, (newVal, oldVal) => {
  if (!clonedLastNodes.value.length) return
  if (newVal && !oldVal) {
    isCustomNodes.value = true
    form.resourceType = 'nodes'
    form.nodeList = [...clonedLastNodes.value]
  } else if (!newVal && oldVal) {
    isCustomNodes.value = false
    form.resourceType = 'replicas'
    form.nodeList = []
  }
})

// Watch nodeList changes, sync update replica if in nodes mode
watch(
  () => form.nodeList,
  (newList) => {
    if (form.resourceType === 'nodes') {
      form.resource.replica = newList.length || undefined
    }
  },
  { deep: true },
)

// If today is selected, auto-fill current time
const midnightDefault = ref(new Date(2000, 0, 1, 0, 0, 0))
watch(
  () => form.schedulerTime,
  (val) => {
    if (!val) return

    const picked = dayjs(val, 'YYYY-MM-DD HH:mm')
    const now = dayjs()
    const sameDay =
      picked.year() === now.year() && picked.month() === now.month() && picked.date() === now.date()
    const isMidnight = picked.hour() === 0 && picked.minute() === 0

    // If today + still at 00:00, auto-fill with current time
    if (sameDay && isMidnight) {
      form.schedulerTime = now.format('YYYY-MM-DD HH:mm')
    }
  },
)

const { disabledDate, disabledHours, disabledMinutes, disabledSeconds } = useDatetimeLimit(
  toRef(form, 'schedulerTime'),
  1,
)

const flavorMaxVal = ref()
const placeholders = computed(() => {
  const val = flavorMaxVal.value ?? {}
  return {
    cpu: `1 to ${Math.min(val.cpu ?? 102, 102)}`,
    gpu: `0 to ${val['amd.com/gpu'] ?? '-'}`,
    memory: `1 to ${Math.min(Number(byte2Gi(val.memory, undefined, false)) ?? 2000, 2000)}`,
    ephemeralStorage: `1 to ${Math.min(Number(byte2Gi(val['ephemeral-storage'], undefined, false)) ?? 6000, 6000)}`,
    replica: `Group node ratio`,
  }
})

const nameRegex = /^[a-z](?:[-a-z0-9]{0,34}[a-z0-9])?$/

// Shared divisibility validation logic (for nodes mode only)
const validateNodesDivisibility = (
  workerReplica: number,
  callback: (err?: Error) => void,
): void => {
  const replicaCount = Number(form.replicaCount)
  if (form.resourceType === 'nodes' && replicaCount && workerReplica % replicaCount !== 0) {
    callback(
      new Error(
        `Worker nodes count (${workerReplica}) must be divisible by REPLICA_COUNT (${replicaCount})`,
      ),
    )
    return
  }
  callback()
}

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
  // Worker Group
  'resource.replica': [{ required: true, message: 'Please input nodePerGroup', trigger: 'blur' }],
  nodeList: [
    {
      type: 'array',
      required: true,
      message: 'Please select at least one node',
      trigger: 'change',
    },
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        const nodeList = value as string[]
        const workerReplica = nodeList.length

        // Validate whether node count is divisible by replicaCount
        validateNodesDivisibility(workerReplica, callback)
      },
      trigger: 'change',
    },
  ],
  'resource.cpu': [{ required: true, message: 'Please input cpu', trigger: 'blur' }],
  'resource.memory': [{ required: true, message: 'Please input memory', trigger: 'blur' }],
  'resource.ephemeralStorage': [
    { required: true, message: 'Please input ephemeral storage', trigger: 'blur' },
  ],
  // Lighthouse
  'lighthouseResource.replica': [
    { required: true, message: 'Please input lighthouse replica', trigger: 'blur' },
  ],
  'lighthouseResource.cpu': [
    { required: true, message: 'Please input lighthouse cpu', trigger: 'blur' },
  ],
  'lighthouseResource.memory': [
    { required: true, message: 'Please input lighthouse memory', trigger: 'blur' },
  ],
  'lighthouseResource.ephemeralStorage': [
    { required: true, message: 'Please input lighthouse ephemeral storage', trigger: 'blur' },
  ],
  // TorchFT Specific
  replicaCount: [
    { required: true, message: 'Please input replica count', trigger: 'blur' },
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        const replicaCount = Number(value)
        const minReplicaCount = Number(form.minReplicaCount)
        const maxReplicaCount = Number(form.maxReplicaCount)
        const workerReplica = Number(form.resource.replica)

        // Validate replicaCount must be between min and max (inclusive)
        if (
          minReplicaCount &&
          maxReplicaCount &&
          (replicaCount < minReplicaCount || replicaCount > maxReplicaCount)
        ) {
          callback(
            new Error(
              `REPLICA_COUNT must be between MIN_REPLICA_COUNT (${minReplicaCount}) and MAX_REPLICA_COUNT (${maxReplicaCount})`,
            ),
          )
          return
        }

        // Validate whether node count is divisible by replicaCount
        validateNodesDivisibility(workerReplica, callback)
      },
      trigger: 'blur',
    },
  ],
  minReplicaCount: [
    { required: true, message: 'Please input min replica count', trigger: 'blur' },
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        const minReplicaCount = Number(value)
        const maxReplicaCount = Number(form.maxReplicaCount)

        if (maxReplicaCount && minReplicaCount >= maxReplicaCount) {
          callback(
            new Error(`MIN_REPLICA_COUNT must be less than MAX_REPLICA_COUNT (${maxReplicaCount})`),
          )
          return
        }

        callback()
      },
      trigger: 'blur',
    },
  ],
  maxReplicaCount: [
    {
      validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
        if (!value && value !== 0) {
          callback()
          return
        }
        const maxReplicaCount = Number(value)
        const minReplicaCount = Number(form.minReplicaCount)

        if (minReplicaCount && maxReplicaCount <= minReplicaCount) {
          callback(
            new Error(
              `MAX_REPLICA_COUNT must be greater than MIN_REPLICA_COUNT (${minReplicaCount})`,
            ),
          )
          return
        }

        callback()
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

    if (props.action === 'Clone' && form.schedulerTime) {
      const picked = dayjs(form.schedulerTime, 'YYYY-MM-DD HH:mm')
      const now = dayjs()

      if (picked.isBefore(now)) {
        ElMessage.warning('Selected schedule time is in the past. Please choose a future time.')
        return
      }
    }

    // Build secrets array
    const secrets = form.secretIds.map((id) => ({ id }))

    const {
      envList,
      labelList,
      resourceType,
      nodeList,
      resource,
      lighthouseResource,
      replicaCount,
      minReplicaCount,
      maxReplicaCount,
      entryPoint,
      schedulerTime,
      timeout,
      secretIds,
      excludedNodes,
      image,
      nodesAffinity: _nodesAffinity,
      ...addPayload
    } = form

    // Worker Group Resource
    // Calculate the actual replica value
    let actualReplica: number
    if (form.resourceType === 'nodes') {
      // Custom node mode: replica = nodeList.length
      actualReplica = form.nodeList.length
    } else {
      // Replicas mode: replica = nodePerGroup * replicaCount
      const nodePerGroup = Number(form.resource.replica) || 1
      const replicaCount = Number(form.replicaCount) || 1
      actualReplica = nodePerGroup * replicaCount
    }

    const workerResource = {
      cpu: form.resource.cpu,
      gpu: Number(form.resource.gpu) === 0 ? '' : (form.resource.gpu ?? ''),
      memory: `${form.resource.memory}Gi`,
      ephemeralStorage: `${form.resource.ephemeralStorage}Gi`,
      replica: actualReplica,
    }

    // Lighthouse Resource
    const lighthouseRes = {
      cpu: form.lighthouseResource.cpu,
      gpu: '', // Lighthouse has no GPU
      memory: `${form.lighthouseResource.memory}Gi`,
      ephemeralStorage: `${form.lighthouseResource.ephemeralStorage}Gi`,
      replica: form.lighthouseResource.replica || 1,
    }

    // TorchFT always returns two resources: Lighthouse and Worker Group
    const resources = [lighthouseRes, workerResource]

    // nodeList only effective in nodes mode, mutually exclusive with excludedNodes
    const nodeListPayload = resourceType !== 'replicas' ? { specifiedNodes: nodeList } : {}

    // excludedNodes only effective in replica mode
    const excludedNodesPayload = (() => {
      const arr = (excludedNodes ?? []).filter(Boolean)
      return arr.length ? arr : undefined
    })()

    // Add TorchFT specific fields to env
    const torchFTEnv = {
      REPLICA_COUNT: String(form.replicaCount),
      MIN_REPLICA_COUNT: String(form.minReplicaCount),
      MAX_REPLICA_COUNT: String(form.maxReplicaCount ?? form.replicaCount),
    }
    const mergedEnv = { ...convertListToKeyValueMap(envList), ...torchFTEnv }

    if (!isEdit.value) {
      await addWorkload({
        ...addPayload,
        resources,
        workspace: props.action === 'Clone' ? pendingWorkspaceId.value : store.currentWorkspaceId!,
        env: mergedEnv,
        customerLabels: convertListToKeyValueMap(labelList),
        maxRetry: isRetry.value ? form.maxRetry : 0,
        entryPoints: [encodeToBase64String(entryPoint), encodeToBase64String(entryPoint)],
        images: [form.image, form.image],
        ...(form.schedulerTime
          ? { cronJobs: [{ schedule: toUTCISOString(form.schedulerTime), action: 'start' }] }
          : {}),
        ...(form.timeout ? { timeout: form.timeout } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
        ...nodeListPayload,
        ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
        ...(form.nodesAffinity ? { nodesAffinity: form.nodesAffinity as 'required' | 'preferred' } : {}),
        ...(cachedUseWorkspaceStorage.value !== undefined ? { useWorkspaceStorage: cachedUseWorkspaceStorage.value } : {}),
      })
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      if (!props.wlid) return

      await editWorkload(props.wlid, {
        description: form.description,
        priority: form.priority,
        resources,
        env: mergedEnv,
        maxRetry: isRetry.value ? form.maxRetry : 0,
        entryPoints: [encodeToBase64String(form.entryPoint), encodeToBase64String(form.entryPoint)],
        images: [form.image, form.image],
        ...(form.schedulerTime
          ? { cronJobs: [{ schedule: form.schedulerTime, action: 'start' }] }
          : {}),
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

  isRetry.value = true

  form.displayName = res.displayName
  form.description = res.description
  // workload now supports entryPoints/images arrays; UI keeps a single entryPoint/image
  form.entryPoint =
    (Array.isArray(res.entryPoints) ? res.entryPoints[0] : undefined) ?? res.entryPoint ?? ''
  form.isSupervised = res.isSupervised ?? false
  form.image = (Array.isArray(res.images) ? res.images[0] : undefined) ?? res.image ?? ''
  form.maxRetry = res.maxRetry ?? 0
  form.timeout = res.timeout
  form.schedulerTime = decodeScheduleFromApi(res.cronJobs?.[0]?.schedule) ?? ''
  form.dependencies = res.dependencies ?? []

  // Regular users cannot select high priority; auto-downgrade to medium when cloning
  form.priority =
    isManager.value || store.isCurrentWorkspaceAdmin()
      ? res.priority
      : res.priority === 2
        ? 1
        : res.priority

  // Check if custom nodes (determined by specifiedNodes array)
  const customNodesList = res.specifiedNodes ?? []
  isCustomNodes.value = customNodesList.length > 0

  // If custom nodes and in edit mode, editing is not allowed
  if (isCustomNodes.value && isEdit.value) {
    ElMessage.warning('Custom nodes workload cannot be edited')
    emit('update:visible', false)
    return
  }

  // If custom nodes, set to nodes mode
  if (isCustomNodes.value) {
    form.resourceType = 'nodes'
    if (!isEdit.value) {
      form.nodeList = customNodesList
    }
    form.nodesAffinity = res.nodesAffinity || 'required'
    clonedLastNodes.value = []
  } else {
    form.resourceType = 'replicas'
    form.nodesAffinity = ''
    const lastNodes = res.nodes?.length ? (res.nodes[res.nodes.length - 1] ?? []) : []
    clonedLastNodes.value = lastNodes
  }

  // TorchFT: load two resources - Lighthouse (index 0) and Worker Group (index 1)
  if (res.resources && Array.isArray(res.resources)) {
    // Lighthouse
    if (res.resources[0]) {
      const lighthouseRes = res.resources[0]
      form.lighthouseResource.replica = lighthouseRes.replica || 1
      form.lighthouseResource.cpu = lighthouseRes.cpu
      form.lighthouseResource.memory = lighthouseRes.memory?.replace(/Gi$/i, '') ?? ''
      form.lighthouseResource.ephemeralStorage =
        lighthouseRes.ephemeralStorage?.replace(/Gi$/i, '') ?? ''
    }
    // Worker Group
    if (res.resources[1]) {
      const workerRes = res.resources[1]
      const workerReplica = workerRes.replica
      form.resource.cpu = workerRes.cpu
      form.resource.gpu = workerRes.gpu || ''
      form.resource.memory = workerRes.memory?.replace(/Gi$/i, '') ?? ''
      form.resource.ephemeralStorage = workerRes.ephemeralStorage?.replace(/Gi$/i, '') ?? ''

      // If not custom nodes, calculate nodePerGroup (needed for both edit and clone)
      if (!isCustomNodes.value) {
        // nodePerGroup = replica / replicaCount
        const replicaCount = res.env?.REPLICA_COUNT ? Number(res.env.REPLICA_COUNT) : 1
        form.resource.replica =
          replicaCount > 0 ? Math.round(workerReplica / replicaCount) : workerReplica
      } else {
        // Custom nodes use replica directly
        form.resource.replica = workerReplica
      }
    }
  }

  // Extract TorchFT specific fields from env
  const envCopy = { ...res.env }
  form.replicaCount = envCopy.REPLICA_COUNT ? Number(envCopy.REPLICA_COUNT) : undefined
  form.minReplicaCount = envCopy.MIN_REPLICA_COUNT ? Number(envCopy.MIN_REPLICA_COUNT) : undefined
  form.maxReplicaCount = envCopy.MAX_REPLICA_COUNT ? Number(envCopy.MAX_REPLICA_COUNT) : undefined

  // Remove these three fields from env, put the rest into envList
  delete envCopy.REPLICA_COUNT
  delete envCopy.MIN_REPLICA_COUNT
  delete envCopy.MAX_REPLICA_COUNT

  form.envList = convertKeyValueMapToList(envCopy)
  form.labelList = convertKeyValueMapToList(res.customerLabels)

  // Handle excludedNodes
  form.excludedNodes = res.excludedNodes ?? []

  // Handle secrets — clear when cloning, keep when editing
  if (props.action === 'Edit' && res.secrets && res.secrets.length > 0) {
    form.secretIds = res.secrets.map((s: any) => s.id)
  } else if (props.action === 'Clone') {
    form.secretIds = [] // Clear secrets when cloning so user can re-select
  }

  form.forceHostNetwork = res.forceHostNetwork ?? false

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

const fetchImage = async (tag?: string) => {
  const res = await getImagesList({ flat: true, tag })
  imageOptions.value = res ?? []
}

// Use composable for nodes paste functionality
const { handleSelectVisibleChange: handleNodesVisibleChange } = useSelectPaste({
  options: nodeOptions,
  modelValue: toRef(form, 'nodeList'),
  successMessagePrefix: 'Matched and selected',
  warningMessagePrefix: 'Could not find nodes',
})

// Use composable for excludedNodes paste functionality
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

const onOpen = async () => {
  showAdvanced.value = false
  cachedUseWorkspaceStorage.value = undefined
  clonedLastNodes.value = []
  pendingWorkspaceId.value = store.currentWorkspaceId ?? store.firstWorkspace ?? ''
  fetchImage()
  fetchWlOptions()
  fetchSecrets()
  if (props.action !== 'Create') {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())
    isCustomNodes.value = false
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
  /* padding: 8px 24px 16px; */
}

/* Wrap each group in a card */
.section-card {
  background: var(--el-bg-color-overlay);
  border-radius: 10px;
  padding: 14px 16px 10px;
  margin-bottom: 20px; /* Add spacing between cards */

  /* Subtle border: barely visible stroke + shadow */
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

/* Slight lift on hover for card effect (optional) */
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
.section-header--clickable {
  cursor: pointer;
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

/* Slightly tighten top of collapsed area */
.advanced-collapse :deep(.el-collapse-item__header) {
  padding: 0;
}

/* Resource Group Title */
.resource-group-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  padding: 6px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  border-left: 3px solid var(--safe-primary);
}

html.dark .resource-group-title {
  background: rgba(255, 255, 255, 0.05);
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
