<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} Workspace`"
    width="600"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
      :rules="rules"
    >
      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" :disabled="isEdit" />
      </el-form-item>

      <el-form-item label="Description">
        <el-input v-model="form.description" :rows="2" type="textarea" />
      </el-form-item>

      <el-form-item label="Node Flavor" prop="flavorId">
        <el-select v-model="form.flavorId" placeholder="please select flavor name">
          <el-option v-for="item in state.flavorOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>

      <el-form-item label="Scopes" prop="scopes">
        <el-checkbox-group v-model="form.scopes">
          <el-checkbox v-for="item in SCOPES_KEYS" :key="item" :label="item" :value="item" />
        </el-checkbox-group>
      </el-form-item>
      <el-form-item label="Max Runtime" v-if="selectedScopes.length">
        <div class="w-full">
          <el-row v-for="scope in selectedScopes" :key="scope" :gutter="12" class="mb-2">
            <el-col :span="12" class="flex items-center">
              <span class="text-gray-600">{{ scope }}</span>
            </el-col>
            <el-col :span="12">
              <el-input
                v-model.number="form.maxRuntime[scope]"
                type="number"
                placeholder="Optional"
              >
                <template #append>hours</template>
              </el-input>
            </el-col>
          </el-row>
        </div>
      </el-form-item>

      <el-form-item label="Cluster" prop="clusterId">
        <el-select v-model="form.clusterId" placeholder="please select cluster">
          <el-option
            v-for="item in store.items"
            :key="item.clusterId"
            :label="item.clusterId"
            :value="item.clusterId"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Image Secret">
        <el-select
          v-model="form.imageSecretIds"
          multiple
          clearable
          placeholder="please select image secret(s)"
        >
          <el-option
            v-for="item in state.imageSecretOptions"
            :key="item"
            :label="item"
            :value="item"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Managers" v-if="isEdit">
        <el-select v-model="form.managers" filterable placeholder="please select managers" multiple>
          <el-option
            v-for="item in state.userOptions"
            :key="item.id"
            :label="item.name"
            :value="item.id"
          />
        </el-select>
      </el-form-item>

      <el-row :gutter="20">
        <el-col :span="12">
          <el-form-item label="Replica" prop="replica">
            <el-input v-model.number="form.replica" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Queue Policy" prop="queuePolicy">
            <el-select v-model="form.queuePolicy" placeholder="please select queue policy">
              <el-option label="fifo" value="fifo" />
              <el-option label="balance" value="balance" />
            </el-select>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="20">
        <el-col :span="12">
          <el-form-item label="Preemption">
            <el-switch v-model="form.enablePreempt" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Default Accessible">
            <el-switch v-model="form.isDefault" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item label="Volumes" label-position="top">
        <div class="w-full">
          <!-- {{ form.volumes }} -->
          <el-card v-for="(v, i) in form.volumes" :key="v.uid" class="mb-3">
            <template #header>
              <div class="flex items-center justify-between">
                <div>
                  <!-- <b>{{ v.storageType?.toUpperCase() }}</b> -->
                  <b>{{ v.type?.toUpperCase() }}</b>
                  <span class="text-gray-500 ml-2">#{{ i + 1 }}</span>
                </div>
                <el-button type="danger" link @click="removeVolume(i)">Remove</el-button>
              </div>
            </template>

            <!-- Common: mountPath required -->
            <el-form-item
              :prop="`volumes.${i}.mountPath`"
              :rules="[{ required: true, message: 'mountPath is required', trigger: 'blur' }]"
              label="Mount Path"
            >
              <el-input v-model="v.mountPath" :disabled="v.disabled" />
            </el-form-item>

            <!-- hostpath Exclusive: hostPath is required -->
            <!-- <template v-if="v.storageType === 'hostpath'"> -->
            <template v-if="v.type === 'hostpath'">
              <el-form-item
                :prop="`volumes.${i}.hostPath`"
                :rules="[{ required: true, message: 'hostPath is required', trigger: 'blur' }]"
                label="Host Path"
              >
                <el-input v-model="v.hostPath" :disabled="v.disabled" />
              </el-form-item>
            </template>

            <!-- Non-hostpath: capacity / storageClass required, subPath optional -->
            <!-- New form item provisioningStrategy for choosing between storageClass/PV Selector, where PV Selector is a key-value input -->
            <template v-else>
              <el-form-item
                :prop="`volumes.${i}.capacity`"
                :rules="[{ required: true, message: 'capacity is required', trigger: 'blur' }]"
                label="Capacity"
              >
                <el-input v-model="v.capacity" :disabled="v.disabled">
                  <!-- Unit suffix optional, default Pi -->
                  <template #append>
                    <el-select v-model="v.capacityAppend" placeholder="Select" style="width: 85px">
                      <el-option v-for="v in CAP_UNITS" :key="v" :label="v" :value="v" />
                    </el-select>
                  </template>
                </el-input>
              </el-form-item>

              <el-form-item
                :prop="`volumes.${i}.provisioningStrategy`"
                label="Provisioning Strategy"
              >
                <el-segmented
                  v-model="v.provisioningStrategy"
                  :options="['storageClass', 'PV Selector']"
                  :disabled="v.disabled"
                />
              </el-form-item>

              <el-form-item
                v-if="v.provisioningStrategy === 'storageClass'"
                :prop="`volumes.${i}.storageClass`"
                :rules="[{ required: true, message: 'storageClass is required', trigger: 'blur' }]"
                label="Storage Class"
              >
                <el-input v-model="v.storageClass" :disabled="v.disabled" />
              </el-form-item>

              <el-form-item
                v-else
                :prop="`volumes.${i}.selectorKV`"
                :rules="[{ required: true, message: 'selector is required', trigger: 'blur' }]"
                label="PV Selector"
              >
                <div class="flex gap-2 w-full items-center" v-if="v.selectorKV">
                  <el-input
                    v-model="v.selectorKV.key"
                    placeholder="Label Key"
                    :disabled="v.disabled"
                  />
                  <el-input
                    v-model="v.selectorKV.value"
                    placeholder="Label Value"
                    :disabled="v.disabled"
                  />
                  <el-tooltip
                    content="selector is a label query over volumes to consider for binding."
                    placement="top"
                  >
                    <el-icon><InfoFilled /></el-icon>
                  </el-tooltip>
                </div>
              </el-form-item>

              <el-form-item label="Sub Path" :prop="`volumes.${i}.subPath`">
                <el-input v-model="v.subPath" :disabled="v.disabled" />
              </el-form-item>

              <el-form-item label="Access Mode" :prop="`volumes.${i}.accessMode`">
                <el-select v-model="v.accessMode" style="width: 220px" :disabled="v.disabled">
                  <el-option label="ReadWriteOnce" value="ReadWriteOnce" />
                  <el-option label="ReadOnlyMany" value="ReadOnlyMany" />
                  <el-option label="ReadWriteMany" value="ReadWriteMany" />
                  <el-option label="ReadWriteOncePod" value="ReadWriteOncePod" />
                </el-select>
              </el-form-item>
            </template>
          </el-card>
        </div>
      </el-form-item>
      <el-space wrap class="m-t-[-20px]">
        <el-button @click="addVolume('pfs')">+ PFS</el-button>
        <el-button @click="addVolume('hostpath')">+ HostPath</el-button>
      </el-space>
    </el-form>
    <!-- <el-button @click="addVolume('rbd')">+ RBD</el-button>
    <el-button @click="addVolume('obs')">+ OBS</el-button>
    <el-button @click="addVolume('cephfs')">+ CephFS</el-button> -->
    <!-- <el-button @click="addVolume('juicefs')">+ JuiceFS</el-button> -->

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, onMounted, ref, computed, nextTick, watch } from 'vue'
import { getNodeFlavors, getSecrets } from '@/services'
import type { FlavorOptionsType } from '@/services'
import type {
  Volume,
  VolumeType,
  QueuePolicy,
  UserSelfData,
  CapUnit,
  PersistentVolumeResponse,
  PvPrefill,
  SelectorKV,
  UserT,
  VolumeWithStrategy,
  PfsVolumeWithStrategy,
} from '@/services'
import {
  addWorkspace,
  editWorkspace,
  getPersistentVolumes,
  getWorkspaceDetail,
  SCOPES_KEYS,
} from '@/services'
import { getUserDataList } from '@/services/login'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { InfoFilled } from '@element-plus/icons-vue'
import { useClusterStore } from '@/stores/cluster'

const props = defineProps<{
  visible: boolean
  action: string
  wsid: string
}>()
const emit = defineEmits(['update:visible', 'success'])
const isEdit = computed(() => props.action === 'Edit')

const store = useClusterStore()

const state = reactive({
  flavorOptions: [] as string[],
  imageSecretOptions: [] as string[],
  userOptions: [] as UserT[],
  pvPrefill: null as PvPrefill | null,
})
const initialForm = reactive({
  name: '',
  description: '',
  clusterId: '',
  imageSecretIds: [],
  flavorId: '',
  replica: undefined,
  queuePolicy: 'fifo' as QueuePolicy,
  enablePreempt: false,
  isDefault: false,
  managers: [],
  volumes: [] as Volume[],
  scopes: ['Train', 'TorchFT', 'Monarch', 'Infer', 'Authoring'],
  maxRuntime: {} as Record<string, number | undefined>,
})

const form = reactive({ ...initialForm })

const ruleFormRef = ref<FormInstance>()
const rules = reactive<FormRules>({
  flavorId: [{ required: true, message: 'Please select flavor name', trigger: 'change' }],
  scopes: [{ required: true, message: 'Please select scope', trigger: 'change' }],
  clusterId: [{ required: true, message: 'Please select cluster', trigger: 'change' }],
  name: [{ required: true, message: 'Please input workspace name', trigger: 'change' }],
})

const newVolume = (t: VolumeType): Volume => {
  if (t === 'hostpath') {
    return {
      uid: Date.now().toString(),
      // storageType: 'hostpath',
      type: 'hostpath',
      mountPath: '',
      hostPath: '',
    }
  }
  return {
    uid: Date.now().toString(),
    // storageType: t,
    type: t,
    mountPath: '',
    subPath: '',
    capacity: '',
    capacityAppend: 'Pi',
    storageClass: '',
    accessMode: 'ReadWriteMany',
    provisioningStrategy: 'storageClass',
    selectorKV: { key: '', value: '' },
  }
}

const addVolume = (t: VolumeType) => {
  const volume = newVolume(t)
  form.volumes.push(volume)
}

const removeVolume = (i: number) => {
  form.volumes.splice(i, 1)
}

const toOneRecord = (kv?: SelectorKV): Record<string, string> | undefined => {
  const k = kv?.key?.trim()
  const v = kv?.value?.trim()
  if (!k || !v) return undefined
  return { [k]: v }
}
const toOneKV = (rec?: Record<string, string>) => {
  const [k, v] = Object.entries(rec ?? {})[0] ?? []
  return k && v ? { key: k, value: v } : undefined
}

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    const volumesPayload = form.volumes?.map(({ uid, ...rest }) => {
      const clone = { ...rest }

      if (clone.type !== 'hostpath' && clone.capacity) {
        clone.capacity = `${clone.capacity}${clone.capacityAppend ?? 'Pi'}`
      }

      // Format selectorKV and handle extra fields
      if (clone.type !== 'hostpath' && clone.provisioningStrategy === 'PV Selector') {
        clone.selector = toOneRecord(clone.selectorKV)
        clone.storageClass = ''
      } else if (clone.type !== 'hostpath' && clone.provisioningStrategy === 'storageClass') {
        clone.selector = undefined
      }

      delete (clone as any).provisioningStrategy
      delete (clone as any).selectorKV
      delete (clone as any).capacityAppend
      delete (clone as any).disabled

      return clone
    }) as unknown as Volume[]

    const maxRuntime: Record<string, number> = {}
    selectedScopes.value.forEach((scope) => {
      const value = form.maxRuntime?.[scope]
      if (typeof value === 'number' && Number.isFinite(value)) {
        maxRuntime[scope] = value
      }
    })
    const { managers, ...payload } = form

    if (!isEdit.value) {
      await addWorkspace({
        ...payload,
        volumes: volumesPayload,
        maxRuntime,
      })
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      const { name: _n, ...payload } = form
      if (!props.wsid) return
      await editWorkspace(props.wsid, { ...payload, volumes: volumesPayload, maxRuntime })
      ElMessage({ message: 'Edit successful', type: 'success' })
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

// Parse and split capacity return value into val + unit
const CAP_UNITS: CapUnit[] = ['Pi', 'Ti', 'Gi']
const parseCapacity = (raw?: string): { val: string; unit: CapUnit } => {
  if (!raw) return { val: '', unit: 'Pi' }
  const s = String(raw).trim()

  // Match: number (including decimal) + optional space + unit (Gi/Ti/Pi), case-insensitive
  const m = s.match(/^(\d+(?:\.\d+)?)(?:\s*)(Gi|Ti|Pi)?$/i)

  if (!m) {
    // Invalid format: treat all as value, default Pi
    return { val: s.replace(/[^\d.]/g, ''), unit: 'Pi' }
  }

  const val = m[1]
  const unit = m[2] ?? 'Pi'
  const unitNorm = (unit[0].toUpperCase() + unit.slice(1).toLowerCase()) as CapUnit
  return { val, unit: CAP_UNITS.includes(unitNorm) ? unitNorm : 'Pi' }
}

const setInitialFormValues = async () => {
  if (!props.wsid) return

  const res = await getWorkspaceDetail(props.wsid)

  form.name = res.workspaceName
  form.scopes = res.scopes
  form.clusterId = res.clusterId
  form.description = res.description
  form.flavorId = res.flavorId
  form.imageSecretIds = res.imageSecretIds
  form.replica = res.targetNodeCount
  form.queuePolicy = res.queuePolicy
  form.enablePreempt = res.enablePreempt
  form.isDefault = res.isDefault
  form.managers = res.managers?.map((v: any) => v.id)
  form.maxRuntime = res.maxRuntime ?? {}

  form.volumes = (res.volumes ?? []).map((v: any) => {
    if (v.type === 'hostpath')
      return {
        ...v,
        disabled: !!v.id, // Edit mode: existing volumes are read-only
      }

    // Parse capacity -> capacity + capacityAppend
    const { val, unit } = parseCapacity(v.capacity)

    const hasSelector = v.selector && Object.keys(v.selector).length > 0

    return {
      ...v,
      capacity: val,
      capacityAppend: unit,
      provisioningStrategy: hasSelector ? 'PV Selector' : 'storageClass',
      selectorKV: hasSelector ? toOneKV(v.selector) : undefined,
      disabled: !!v.id,
    }
  })
}

async function loadPersistentVolumes() {
  if (!props.wsid) return
  try {
    const res = (await getPersistentVolumes(props.wsid)) as PersistentVolumeResponse
    const first = res?.items?.[0]
    if (!first) {
      state.pvPrefill = null
      return
    }

    const [key, value] = Object.entries(first.labels ?? {})[0] ?? []
    const { val, unit } = parseCapacity(first.capacity?.storage)
    state.pvPrefill = {
      storageClassName: first.storageClassName,
      labelKV: key && value ? { key, value } : undefined,
      capacity: val ? { value: val, unit } : undefined,
      accessMode: first.accessModes?.[0],
    }
  } catch {
    state.pvPrefill = null
  }
}

function isEditableNewPfs(v: VolumeWithStrategy): v is PfsVolumeWithStrategy {
  return isEdit.value && v?.type === 'pfs' && !v?.disabled
}

function applyPvPrefill(v: VolumeWithStrategy) {
  if (!isEditableNewPfs(v) || !state.pvPrefill) return

  const prefill = state.pvPrefill
  const capacityPrefill = prefill.capacity
  const shouldFillCapacity = !v.capacity && !!capacityPrefill
  v.capacity = shouldFillCapacity ? capacityPrefill.value : v.capacity
  v.capacityAppend = shouldFillCapacity ? capacityPrefill.unit : v.capacityAppend
  v.accessMode = !v.accessMode && prefill.accessMode ? prefill.accessMode : v.accessMode

  const isSelector = v.provisioningStrategy === 'PV Selector'
  const isStorageClass = v.provisioningStrategy === 'storageClass'
  const hasSelector = v.selectorKV?.key || v.selectorKV?.value
  v.selectorKV =
    isSelector && !hasSelector && prefill.labelKV ? { ...prefill.labelKV } : v.selectorKV
  v.storageClass =
    isStorageClass && !v.storageClass && prefill.storageClassName
      ? prefill.storageClassName
      : v.storageClass
}
const initSelectOptions = async () => {
  const flavors = await getNodeFlavors()
  state.flavorOptions = flavors?.items?.map((item: FlavorOptionsType) => item.flavorId)
  const users = await getUserDataList()
  const imageSecrets = await getSecrets({ type: 'image' }).catch(() => ({ items: [] }))

  state.userOptions = users?.items?.map((item: UserSelfData) => ({ id: item.id, name: item.name }))
  form.flavorId = state.flavorOptions?.[0] ?? ''
  state.imageSecretOptions = (imageSecrets?.items ?? []).map(
    (s: any) => s.secretId ?? s.name ?? s.id,
  )
}

onMounted(async () => {
  initSelectOptions()
})

const onOpen = async () => {
  if (isEdit.value) {
    await setInitialFormValues()
    await loadPersistentVolumes()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm)
    initSelectOptions()
  }
  await nextTick()
}

const selectedScopes = computed(() => form.scopes ?? [])

// Watch volumes strategy and KV changes
watch(
  () => form.volumes,
  (vols: VolumeWithStrategy[]) => {
    if (!Array.isArray(vols)) return
    vols.forEach((v: VolumeWithStrategy) => {
      if (v.provisioningStrategy === 'PV Selector') {
        if (!v.selectorKV) v.selectorKV = { key: '', value: '' } as SelectorKV
        v.storageClass = ''
      } else if (v.provisioningStrategy === 'storageClass') {
        v.selector = undefined
        v.selectorKV = undefined
      }
      applyPvPrefill(v)
    })
  },
  { deep: true, immediate: true },
)
</script>
