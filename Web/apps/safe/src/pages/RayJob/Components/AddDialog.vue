<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} RayJob`"
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
      <el-form ref="ruleFormRef" :model="form" :rules="rules" label-width="120px">
        <!-- ===== Basic Information ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Basic Information</div>
              <div class="section-subtitle">Name and description</div>
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

          <el-form-item label="entryPoint">
            <el-input v-model="form.jobEntrypoint" type="textarea" :rows="2" placeholder="RayJob entrypoint" />
            <el-text size="small" type="info" class="mt-1">
              <el-icon class="mr-1"><InfoFilled /></el-icon>
              {{ JOB_ENTRYPOINT_INFO }}
            </el-text>
          </el-form-item>
        </div>

        <!-- ===== Ray Cluster ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div class="flex-1">
              <div class="section-title">Ray Cluster</div>
              <div class="section-subtitle">Configure header and worker resources</div>
            </div>
          </div>

          <!-- Header -->
          <div class="mb-4">
            <div class="resource-group-title mb-3">Header</div>
            <el-row :gutter="16">
              <el-col :span="24">
                <el-form-item label="image" prop="header.image">
                  <ImageInput v-model="form.header.image" />
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item label="entryPoint">
                  <el-input v-model="form.header.entryPoint" type="textarea" :rows="2" :placeholder="CLUSTER_ENTRYPOINT_INFO" />
                </el-form-item>
              </el-col>

              <el-col :span="12">
                <el-form-item label="cpu" prop="header.cpu">
                  <el-input v-model="form.header.cpu" :placeholder="placeholders.cpu" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="gpu" prop="header.gpu">
                  <el-input v-model="form.header.gpu" :placeholder="placeholders.gpu" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="memory" prop="header.memory">
                  <el-input v-model="form.header.memory" :placeholder="placeholders.memory">
                    <template #append>Gi</template>
                  </el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="ephemeral" prop="header.ephemeralStorage">
                  <el-input
                    v-model="form.header.ephemeralStorage"
                    :placeholder="placeholders.ephemeralStorage"
                  >
                    <template #append>Gi</template>
                  </el-input>
                </el-form-item>
              </el-col>
            </el-row>
          </div>

          <el-divider />

          <!-- Workers -->
          <div class="mb-4">
            <div class="flex items-center justify-between mb-3">
              <div class="resource-group-title">Worker</div>
              <div class="flex items-center gap-2">
                <el-button
                  size="small"
                  type="primary"
                  plain
                  @click="addWorker"
                  :disabled="isEdit || form.workers.length >= 2"
                >
                  Add Worker
                </el-button>
              </div>
            </div>

            <template v-for="(w, idx) in form.workers" :key="idx">
              <div class="mb-4">
                <div class="flex items-center justify-between mb-2">
                  <div class="text-sm font-600 text-[var(--el-text-color-primary)]">
                    Worker {{ idx + 1 }}
                  </div>
                  <el-button
                    size="small"
                    type="danger"
                    plain
                    @click="removeWorker(idx)"
                    :disabled="isEdit || form.workers.length <= 0"
                  >
                    Remove
                  </el-button>
                </div>

                <el-row :gutter="16">
                  <el-col :span="24">
                    <el-form-item :label="`image`" :prop="`workers.${idx}.image`">
                      <ImageInput v-model="w.image" />
                    </el-form-item>
                  </el-col>
                  <el-col :span="24">
                    <el-form-item :label="`entryPoint`">
                      <el-input v-model="w.entryPoint" type="textarea" :rows="2" :placeholder="CLUSTER_ENTRYPOINT_INFO" />
                    </el-form-item>
                  </el-col>



                  <el-col :span="12">

                  </el-col>
                  <el-col :span="12">
                    <el-form-item :label="`cpu`" :prop="`workers.${idx}.cpu`">
                      <el-input v-model="w.cpu" :placeholder="placeholders.cpu" />
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item :label="`gpu`" :prop="`workers.${idx}.gpu`">
                      <el-input v-model="w.gpu" :placeholder="placeholders.gpu" />
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item :label="`memory`" :prop="`workers.${idx}.memory`">
                      <el-input v-model="w.memory" :placeholder="placeholders.memory">
                        <template #append>Gi</template>
                      </el-input>
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item :label="`ephemeral`" :prop="`workers.${idx}.ephemeralStorage`">
                      <el-input
                        v-model="w.ephemeralStorage"
                        :placeholder="placeholders.ephemeralStorage"
                      >
                        <template #append>Gi</template>
                      </el-input>
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item :label="`replica`" :prop="`workers.${idx}.replica`">
                      <el-input-number
                        v-model="w.replica"
                        :min="1"
                        :step="1"
                        controls-position="right"
                        :disabled="isEdit"
                        style="width: 100%"
                      />
                    </el-form-item>
                  </el-col>
                </el-row>
              </div>
              <el-divider v-if="idx !== form.workers.length - 1" border-style="dashed" />
            </template>

            <!-- Common excludedNodes -->
            <el-divider />
            <el-row :gutter="16">
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
            </el-row>
          </div>
        </div>

        <!-- ===== Advanced Options (collapsible) ===== -->
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
                    <el-switch v-model="form.preheat" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ PREHEAT_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>

                <!-- privileged -->
                <el-col :span="12" v-if="isManager || store.isCurrentWorkspaceAdmin()">
                  <el-form-item label="privileged">
                    <el-switch v-model="form.privileged" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ PRIVILEGED_INFO }}
                    </el-text>
                  </el-form-item>
                </el-col>

                <!-- stickyNodes -->
                <el-col :span="12" v-if="!isEdit">
                  <el-form-item label="stickyNodes">
                    <el-switch v-model="form.stickyNodes" class="mr-2" />
                    <el-text size="small" type="info">
                      <el-icon class="mr-1"><InfoFilled /></el-icon>
                      {{ STICKY_NODES_INFO }}
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
import { getNodesList, getWorkloadsList } from '@/services'
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
import { encodeToBase64String, decodeFromBase64String, toUTCISOString, decodeScheduleFromApi } from '@/utils'
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

const excludedNodeOptions = ref(
  [] as Array<{ nodeId: string; available: boolean; internalIP?: string }>,
)
const wlOptions = ref([] as Array<{ label: string; value: string }>)
// Use composable to fetch secrets
const { secretOptions, fetchSecrets } = useSecrets('image')
const excludedNodesSelectRef = ref()
const excludedNodesSearchQuery = ref('')

const AUTO_RETRY_INFO = 'automatically retry after workload failure'
const TIMEOUT_INFO = 'timeout duration in seconds'
const SCHEDULER_INFO = 'Scheduled execution time'
const RETRY_TIMES_INFO = 'Maximum retries:50'
const HANG_CHECK_INFO = 'workload fails if the last node(by rank) has no logs for 20 minutes'
const PREHEAT_INFO = 'preheat: When enabled, preheats the image, which increases workload duration.'
const STICKY_NODES_INFO = 'When enabled, it will prefer the last-used nodes.'
const JOB_ENTRYPOINT_INFO = 'RayJob entrypoint, will be passed as env variable RAY_JOB_ENTRYPOINT'
const CLUSTER_ENTRYPOINT_INFO = 'Ray Cluster entrypoint, used for initialization during cluster creation'
const PRIVILEGED_INFO = 'Whether to run in privileged mode'
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

const showAdvanced = ref(false)
const fetchWorkspaceOption = () => store.fetchWorkspace(true)

const curPriority = computed(() => (isManager.value || store.isCurrentWorkspaceAdmin() ? 2 : 1))

const initialForm = () => ({
  displayName: '',
  groupVersionKind: {
    kind: 'RayJob',
    version: 'v1',
  },
  description: '',
  jobEntrypoint: '',
  isSupervised: false,
  maxRetry: 5,
  priority: unref(curPriority),
  // RayJob specific fields (UI uses per-resource entryPoint/image)
  header: {
    cpu: '',
    gpu: '',
    memory: '',
    ephemeralStorage: '',
    entryPoint: '',
    image: '',
  },
  workers: [] as Array<{
    cpu: string
    gpu: string
    memory: string
    ephemeralStorage: string
    entryPoint: string
    image: string
    replica: number
  }>,
  // Worker Group
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

  dependencies: [],
  excludedNodes: [] as string[],

  timeout: undefined,
  schedulerTime: '',

  secretIds: [] as string[],
  preheat: false,
  stickyNodes: false,
  privileged: false,
  forceHostNetwork: false,
})
const form = reactive({ ...initialForm() })
const isRetry = ref(false) // isAutoRetry
const canMutateWorkers = computed(() => props.action !== 'Edit')

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

const ruleFormRef = ref<FormInstance>()
const optRequiredIfHasWorker = (index: number, label: string) => ({
  validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
    if ((form.workers?.length ?? 0) <= index) return callback()
    if (value == null || String(value).trim() === '')
      return callback(new Error(`Please input ${label}`))
    callback()
  },
  trigger: 'blur',
})
const optRequiredIfHasWorker2 = (label: string) => optRequiredIfHasWorker(1, label)
const rules = reactive({
  displayName: [
    { required: true, message: 'Please input name', trigger: 'blur' },
    {
      pattern: nameRegex,
      message: 'Must start with lowercase letter, only a-z, 0-9, and "-" allowed, max 45 chars',
      trigger: 'blur',
    },
  ],
  // Header
  'header.image': [{ required: true, message: 'Please input image', trigger: 'blur' }],
  'header.cpu': [{ required: true, message: 'Please input cpu', trigger: 'blur' }],
  'header.gpu': [{ required: true, message: 'Please input gpu', trigger: 'blur' }],
  'header.memory': [{ required: true, message: 'Please input memory', trigger: 'blur' }],
  'header.ephemeralStorage': [
    { required: true, message: 'Please input ephemeral storage', trigger: 'blur' },
  ],
  // Worker 1 (required only when exists)
  'workers.0.image': [optRequiredIfHasWorker(0, 'image')],
  'workers.0.cpu': [optRequiredIfHasWorker(0, 'cpu')],
  'workers.0.gpu': [optRequiredIfHasWorker(0, 'gpu')],
  'workers.0.memory': [optRequiredIfHasWorker(0, 'memory')],
  'workers.0.ephemeralStorage': [optRequiredIfHasWorker(0, 'ephemeral storage')],
  'workers.0.replica': [optRequiredIfHasWorker(0, 'replica')],
  // Worker 2 (only required when exists)
  'workers.1.image': [optRequiredIfHasWorker2('image')],
  'workers.1.cpu': [optRequiredIfHasWorker2('cpu')],
  'workers.1.gpu': [optRequiredIfHasWorker2('gpu')],
  'workers.1.memory': [optRequiredIfHasWorker2('memory')],
  'workers.1.ephemeralStorage': [optRequiredIfHasWorker2('ephemeral storage')],
  'workers.1.replica': [optRequiredIfHasWorker2('replica')],
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
      schedulerTime,
      timeout,
      secretIds,
      excludedNodes,
      resource,
      workers,
      header,
      jobEntrypoint,
      ...addPayload
    } = form

    const headerRes = {
      cpu: header.cpu,
      gpu: Number(header.gpu) === 0 ? '' : (header.gpu ?? ''),
      memory: `${header.memory}Gi`,
      ephemeralStorage: `${header.ephemeralStorage}Gi`,
      replica: 1,
    }
    const workerResources = (workers ?? []).map((w) => ({
      cpu: w.cpu,
      gpu: Number(w.gpu) === 0 ? '' : (w.gpu ?? ''),
      memory: `${w.memory}Gi`,
      ephemeralStorage: `${w.ephemeralStorage}Gi`,
      replica: w.replica ?? 1,
    }))

    const resources = [headerRes, ...workerResources]
    const images = [header.image, ...(workers ?? []).map((w) => w.image)].filter(Boolean)
    const entryPoints = [
      encodeToBase64String(header.entryPoint),
      ...(workers ?? []).map((w) => encodeToBase64String(w.entryPoint)),
    ].filter(Boolean)

    const excludedNodesPayload = (() => {
      const arr = (excludedNodes ?? []).filter(Boolean)
      return arr.length ? arr : undefined
    })()

    const mergedEnv = {
      ...convertListToKeyValueMap(envList),
      ...(jobEntrypoint ? { RAY_JOB_ENTRYPOINT: encodeToBase64String(jobEntrypoint) } : {}),
    }

    if (!isEdit.value) {
      await addWorkload({
        ...addPayload,
        resources,
        workspace: props.action === 'Clone' ? pendingWorkspaceId.value : store.currentWorkspaceId!,
        env: mergedEnv,
        customerLabels: convertListToKeyValueMap(labelList),
        maxRetry: isRetry.value ? form.maxRetry : 0,
        entryPoints,
        images,
        ...(form.schedulerTime
          ? { cronJobs: [{ schedule: toUTCISOString(form.schedulerTime), action: 'start' }] }
          : {}),
        ...(form.timeout ? { timeout: form.timeout } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
        ...(excludedNodesPayload ? { excludedNodes: excludedNodesPayload } : {}),
        stickyNodes: form.stickyNodes,
        ...(cachedUseWorkspaceStorage.value !== undefined ? { useWorkspaceStorage: cachedUseWorkspaceStorage.value } : {}),
      })
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      const {
        displayName: _n,
        groupVersionKind,
        isSupervised,
        envList,
        labelList,
        schedulerTime,
        timeout,
        secretIds,
        excludedNodes: _excludedNodes,
        jobEntrypoint,
        header,
        workers,
        resource,
        forceHostNetwork: _fhn,
        ...editPayload
      } = form
      if (!props.wlid) return

      await editWorkload(props.wlid, {
        ...editPayload,
        resources,
        env: mergedEnv,
        maxRetry: isRetry.value ? form.maxRetry : 0,
        entryPoints,
        images,
        ...(form.schedulerTime
          ? { cronJobs: [{ schedule: form.schedulerTime, action: 'start' }] }
          : {}),
        ...(form.timeout !== undefined ? { timeout: form.timeout } : {}),
        ...(secrets.length > 0 ? { secrets: secrets } : {}),
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
    ;(rules['header.cpu'] as FormItemRule[]).push(createBetweenRule(1, res.cpu))
    ;(rules['header.gpu'] as FormItemRule[]).push(createBetweenRule(0, res['amd.com/gpu'] ?? 0))
    ;(rules['header.memory'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res.memory ?? 0, 0, false))),
    )
    ;(rules['header.ephemeralStorage'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res['ephemeral-storage'] ?? 0, 0, false))),
    )
    ;(rules['workers.0.cpu'] as FormItemRule[]).push(createBetweenRule(1, res.cpu))
    ;(rules['workers.0.gpu'] as FormItemRule[]).push(createBetweenRule(0, res['amd.com/gpu'] ?? 0))
    ;(rules['workers.0.memory'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res.memory ?? 0, 0, false))),
    )
    ;(rules['workers.0.ephemeralStorage'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res['ephemeral-storage'] ?? 0, 0, false))),
    )
    ;(rules['workers.1.cpu'] as FormItemRule[]).push(createBetweenRule(1, res.cpu))
    ;(rules['workers.1.gpu'] as FormItemRule[]).push(createBetweenRule(0, res['amd.com/gpu'] ?? 0))
    ;(rules['workers.1.memory'] as FormItemRule[]).push(
      createBetweenRule(1, Number(byte2Gi(res.memory ?? 0, 0, false))),
    )
    ;(rules['workers.1.ephemeralStorage'] as FormItemRule[]).push(
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
  form.isSupervised = res.isSupervised ?? false
  form.maxRetry = res.maxRetry ?? 0
  form.timeout = res.timeout
  form.stickyNodes = res.stickyNodes ?? false
  form.privileged = res.privileged ?? false
  form.schedulerTime = decodeScheduleFromApi(res.cronJobs?.[0]?.schedule) ?? ''
  form.dependencies = res.dependencies ?? []

  // Regular users cannot select high priority; auto-downgrade to medium when cloning
  form.priority =
    isManager.value || store.isCurrentWorkspaceAdmin()
      ? res.priority
      : res.priority === 2
        ? 1
        : res.priority

  // RayJob: resources[0]=header, resources[1..]=workers (max 2)
  const resources = Array.isArray(res.resources)
    ? res.resources
    : res.resource
      ? [res.resource]
      : []
  const header = resources[0] || {}
  form.header.cpu = header.cpu ?? form.header.cpu
  form.header.gpu = header.gpu ?? form.header.gpu
  form.header.memory = header.memory?.replace(/Gi$/i, '') ?? form.header.memory
  form.header.ephemeralStorage =
    header.ephemeralStorage?.replace(/Gi$/i, '') ?? form.header.ephemeralStorage

  const images = Array.isArray(res.images) ? res.images : res.image ? [res.image] : []
  const entryPoints = Array.isArray(res.entryPoints)
    ? res.entryPoints
    : res.entryPoint
      ? [res.entryPoint]
      : []

  form.header.image = images[0] ?? ''
  form.header.entryPoint = entryPoints[0] ?? ''

  const workerCountFromApi = Math.min(Math.max(resources.length - 1, 0), 2)
  form.workers = Array.from({ length: workerCountFromApi }, (_, i) => {
    const r = resources[i + 1] || {}
    return {
      cpu: r.cpu ?? '2',
      gpu: r.gpu ?? '1',
      memory: (r.memory?.replace(/Gi$/i, '') ?? '4') as string,
      ephemeralStorage: (r.ephemeralStorage?.replace(/Gi$/i, '') ?? '20') as string,
      image: images[i + 1] ?? '',
      entryPoint: entryPoints[i + 1] ?? '',
      replica: r.replica ?? 1,
    }
  })

  // Extract RAY_JOB_ENTRYPOINT from env into jobEntrypoint, keep the rest in envList
  const envMap = res.env ?? {}
  form.jobEntrypoint = envMap.RAY_JOB_ENTRYPOINT ? decodeFromBase64String(envMap.RAY_JOB_ENTRYPOINT) : ''
  const { RAY_JOB_ENTRYPOINT: _jobEp, ...restEnv } = envMap
  form.envList = convertKeyValueMapToList(restEnv)
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
const syncHeaderToWorkers = () => {
  if (props.action !== 'Create') return
  if ((form.workers?.length ?? 0) === 0) return
  const header = form.header
  form.workers.forEach((w) => {
    if (!w.image && header.image) w.image = header.image
    if (!w.entryPoint && header.entryPoint) w.entryPoint = header.entryPoint
    if (!w.cpu && header.cpu) w.cpu = header.cpu
    if (!w.gpu && header.gpu) w.gpu = header.gpu
    if (!w.memory && header.memory) w.memory = header.memory
    if (!w.ephemeralStorage && header.ephemeralStorage) w.ephemeralStorage = header.ephemeralStorage
  })
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

const onOpen = async () => {
  showAdvanced.value = false
  cachedUseWorkspaceStorage.value = undefined
  pendingWorkspaceId.value = store.currentWorkspaceId ?? store.firstWorkspace ?? ''
  fetchNodes()
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
}

watch(
  () => form.header,
  () => {
    syncHeaderToWorkers()
  },
  { deep: true },
)

const addWorker = () => {
  if (!canMutateWorkers.value) return
  if ((form.workers?.length ?? 0) >= 2) return
  form.workers.push({
    cpu: '',
    gpu: '',
    memory: '',
    ephemeralStorage: '',
    entryPoint: '',
    image: '',
    replica: 1,
  })
  syncHeaderToWorkers()
}

const removeWorker = (idx: number) => {
  if (!canMutateWorkers.value) return
  if ((form.workers?.length ?? 0) <= 0) return
  form.workers.splice(idx, 1)
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

/* Leave some space above the advanced expanded content */
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
