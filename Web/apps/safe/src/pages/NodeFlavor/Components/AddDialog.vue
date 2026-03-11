<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} NodeFlavor`"
    width="700"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      :rules="rules"
      label-position="top"
      class="p-4"
      :disabled="isDetail"
    >
      <!-- Basic -->
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="Name" prop="name" class="!mb-3">
            <el-input v-model="form.name" :disabled="isEdit" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Memory" prop="memory" class="!mb-3">
            <el-input v-model="form.memory">
              <template #append>
                <el-select v-model="form.memoryAppend" style="width: 96px">
                  <el-option v-for="v in CAP_UNITS" :key="v" :label="v" :value="v" />
                </el-select>
              </template>
            </el-input>
          </el-form-item>
        </el-col>
      </el-row>

      <div class="flex items-center m-b-6 mt-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">CPU</span>
      </div>
      <!-- <el-divider content-position="left">CPU</el-divider> -->
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="Quantity" prop="cpu.quantity" class="!mb-3">
            <el-input v-model="form.cpu.quantity">
              <template #append>core</template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Product" class="!mb-3">
            <el-input v-model="form.cpu.product" />
          </el-form-item>
        </el-col>
      </el-row>

      <!-- <el-divider content-position="left">GPU</el-divider> -->
      <div class="flex items-center m-b-6 mt-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">GPU</span>
      </div>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="Quantity" class="!mb-3">
            <el-input v-model="form.gpu.quantity">
              <template #append>card</template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Product" class="!mb-3">
            <el-input v-model="form.gpu.product" />
          </el-form-item>
        </el-col>
        <el-col :span="24">
          <el-form-item label="Resource Name" class="!mb-3">
            <el-input v-model="form.gpu.resourceName" />
          </el-form-item>
        </el-col>
      </el-row>

      <div class="flex items-center m-b-6 mt-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Root Disk</span>
      </div>
      <!-- <el-divider content-position="left">Root Disk</el-divider> -->
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="Quantity" prop="rootDisk.quantity" class="!mb-3">
            <el-input v-model="form.rootDisk.quantity">
              <template #append>
                <el-select v-model="form.rootDiskAppend" style="width: 96px">
                  <el-option v-for="v in CAP_UNITS" :key="v" :label="v" :value="v" />
                </el-select>
              </template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Type" prop="rootDisk.type" class="!mb-3">
            <el-select
              v-model="form.rootDisk.type"
              filterable
              allow-create
              clearable
              placeholder="Select or enter disk type"
              @change="onDiskTypeChange('rootDisk')"
            >
              <el-option
                v-for="opt in diskTypeOptions"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value"
              />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Count" prop="rootDisk.count" class="!mb-3">
            <el-input v-model="form.rootDisk.count" />
          </el-form-item>
        </el-col>
      </el-row>

      <div class="flex items-center m-b-6 mt-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Data Disk</span>
      </div>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="Quantity" prop="dataDisk.quantity" class="!mb-3">
            <el-input v-model="form.dataDisk.quantity">
              <template #append>
                <el-select v-model="form.dataDiskAppend" style="width: 96px">
                  <el-option v-for="v in CAP_UNITS" :key="v" :label="v" :value="v" />
                </el-select>
              </template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Type" prop="dataDisk.type" class="!mb-3">
            <el-select
              v-model="form.dataDisk.type"
              filterable
              allow-create
              clearable
              placeholder="Select or enter disk type"
              @change="onDiskTypeChange('dataDisk')"
            >
              <el-option
                v-for="opt in diskTypeOptions"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value"
              />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="Count" prop="dataDisk.count" class="!mb-3">
            <el-input v-model="form.dataDisk.count" />
          </el-form-item>
        </el-col>
      </el-row>

      <div class="flex items-center m-b-6 mt-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Extended Resources</span>
      </div>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="ephemeralStorage" class="!mb-3">
            <el-input v-model="form.extendedResources['ephemeral-storage']">
              <template #append>
                <el-select v-model="form.storageAppend" style="width: 96px">
                  <el-option v-for="v in CAP_UNITS" :key="v" :label="v" :value="v" />
                </el-select>
              </template>
            </el-input>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="rdma/hca" class="!mb-3">
            <el-input v-model="form.extendedResources['rdma/hca']" />
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>
    <template #footer v-if="!isDetail">
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, ref, computed, nextTick, toRaw, watch } from 'vue'
import type { FlavorOptionsType, CreateFlavorPayload } from '@/services'
import { addNodeFlavor, editNodeFlavor } from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { CAP_UNITS, parseQuantityWithUnit, applyQuantityWithUnit } from '@/utils'

const props = defineProps<{
  visible: boolean
  action: string
  flavor?: FlavorOptionsType
}>()
const emit = defineEmits(['update:visible', 'success'])
const isEdit = computed(() => props.action === 'Edit')
const isDetail = computed(() => props.action === 'Detail')

const createInitialForm = () => ({
  name: '',
  cpu: {
    quantity: '',
    product: '',
  },
  gpu: {
    quantity: '',
    product: '',
    resourceName: '',
  },

  memory: '',
  rootDisk: { type: '', quantity: '', count: undefined as number | undefined },
  dataDisk: { type: '', quantity: '', count: undefined as number | undefined },
  extendedResources: {
    'ephemeral-storage': '',
    'rdma/hca': '',
  },

  // Units
  memoryAppend: 'Gi',
  rootDiskAppend: 'Gi',
  dataDiskAppend: 'Gi',
  storageAppend: 'Gi',
})
const form = reactive(createInitialForm())

const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: 'Please input flavor name', trigger: 'change' }],
  'cpu.quantity': [{ required: true, message: 'Please input cpu', trigger: 'change' }],
  memory: [{ required: true, message: 'Please input memory', trigger: 'change' }],
}))

// Dropdown for disk type also supports custom input
const diskTypeOptions = [
  { label: 'NVMe', value: 'nvme' },
  { label: 'SSD', value: 'ssd' },
  { label: 'HDD', value: 'hdd' },
]
function onDiskTypeChange(which: 'rootDisk' | 'dataDisk') {
  const v = String(form[which].type || '')
    .trim()
    .toLowerCase()
  form[which].type = v
}

const isBlank = (v: any) =>
  v === undefined || v === null || (typeof v === 'string' && v.trim() === '')

function checkTripleDisk(
  group: { type?: string; quantity?: string; count?: any },
  label = 'RootDisk',
) {
  const { type, quantity, count } = group || {}
  const anyFilled = !isBlank(type) || !isBlank(quantity) || !isBlank(count)
  const allFilled = !isBlank(type) && !isBlank(quantity) && !isBlank(count)

  if (anyFilled && !allFilled) {
    ElMessage.warning(`${label}: Please fill in Type / Quantity / Count completely`)
    return false
  }
  return true
}

function checkTripleGPU(
  group: { product?: string; quantity?: string; resourceName?: string },
  label = 'GPU',
) {
  const { product, quantity, resourceName } = group || {}
  const anyFilled = !isBlank(product) || !isBlank(quantity) || !isBlank(resourceName)
  const allFilled = !isBlank(product) && !isBlank(quantity) && !isBlank(resourceName)

  if (anyFilled && !allFilled) {
    ElMessage.warning(`${label}: Please fill in Type / Quantity / Count completely`)
    return false
  }
  return true
}

const addUnit = (v: string | undefined, unit: string) => (isBlank(v) ? undefined : `${v}${unit}`)

// Filter disk object content
function buildDisk(
  disk: { type?: string; quantity?: string; count?: number | string },
  unit: string,
) {
  const out: Record<string, any> = {}

  // Filter out empty type
  if (disk.type && disk.type.trim() !== '') {
    out.type = disk.type.trim()
  }

  // Filter out empty quantity
  if (disk.quantity && disk.quantity.trim() !== '') {
    out.quantity = `${disk.quantity}${unit}`
  }

  // Filter out undefined / empty / NaN count
  if (disk.count !== undefined && disk.count !== null && String(disk.count).trim() !== '') {
    const n = Number(disk.count)
    if (!Number.isNaN(n)) {
      out.count = n
    }
  }

  // Return undefined if no valid fields
  return Object.keys(out).length > 0 ? out : undefined
}
const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    const raw = toRaw(form)
    const gpuQtyNum = Number(String(raw.gpu?.quantity ?? '').trim())
    if (Number.isFinite(gpuQtyNum) && gpuQtyNum > 0) {
      if (!checkTripleGPU(raw.gpu)) return
    }

    // Extract rootDisk / dataDisk / extendedResources / gpu from raw
    const {
      memoryAppend,
      rootDiskAppend,
      dataDiskAppend,
      storageAppend,
      gpu: rawGpu, // Extract gpu from base
      rootDisk: rawRootDisk,
      dataDisk: rawDataDisk,
      extendedResources: rawExtRes,
      ...base
    } = raw

    const memory = addUnit(base.memory as string, memoryAppend)
    const rootDisk = buildDisk(rawRootDisk ?? {}, rootDiskAppend)
    const dataDisk = buildDisk(rawDataDisk ?? {}, dataDiskAppend)

    // Keep only non-empty extendedResources
    const extendedResources: Record<string, string> = {}
    const eph = addUnit(rawExtRes?.['ephemeral-storage'], storageAppend)
    if (!isBlank(eph)) extendedResources['ephemeral-storage'] = eph as string
    if (!isBlank(rawExtRes?.['rdma/hca'])) {
      extendedResources['rdma/hca'] = String(rawExtRes['rdma/hca']).trim()
    }

    const formatForm: Record<string, any> = { ...base }
    if (!isBlank(memory)) formatForm.memory = memory
    if (rootDisk) formatForm.rootDisk = rootDisk
    if (dataDisk) formatForm.dataDisk = dataDisk
    if (Object.keys(extendedResources).length) {
      formatForm.extendedResources = extendedResources
    }

    // Include gpu only when quantity is valid; otherwise omit entirely
    if (!isEdit.value) {
      const q = Number(String(rawGpu?.quantity ?? '').trim())
      if (Number.isFinite(q) && q > 0) {
        formatForm.gpu = { ...rawGpu, quantity: q }
      }
    } else if (rawGpu) {
      formatForm.gpu = rawGpu
    }

    if (!isEdit.value) {
      await addNodeFlavor(formatForm as CreateFlavorPayload)
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      const { name, ...editForm } = formatForm
      if (!props.flavor) return
      await editNodeFlavor(props.flavor.flavorId, editForm)
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

// Populate form for editing
const setInitialFormValues = async () => {
  if (!props.flavor) return

  form.name = props.flavor.flavorId

  form.cpu = props.flavor.cpu
  form.gpu = props.flavor.gpu

  // memory
  const { val: memVal, unit: memUnit } = parseQuantityWithUnit(props.flavor.memory)
  form.memory = memVal
  form.memoryAppend = memUnit

  // rootDisk
  applyQuantityWithUnit(
    form.rootDisk,
    props.flavor.rootDisk?.quantity,
    (u) => (form.rootDiskAppend = u),
  )
  form.rootDisk.type = props.flavor.rootDisk?.type ?? ''
  form.rootDisk.count = props.flavor.rootDisk?.count ?? undefined

  // dataDisk
  applyQuantityWithUnit(
    form.dataDisk,
    props.flavor.dataDisk?.quantity,
    (u) => (form.dataDiskAppend = u),
  )
  form.dataDisk.type = props.flavor.dataDisk?.type ?? ''
  form.dataDisk.count = props.flavor.dataDisk?.count ?? undefined

  // extendedResources
  const parsed = props.flavor.extendedResources?.['ephemeral-storage']
    ? parseQuantityWithUnit(props.flavor.extendedResources['ephemeral-storage'])
    : { val: '', unit: 'Gi' }

  form.extendedResources['ephemeral-storage'] = parsed.val
  form.storageAppend = parsed.unit
  form.extendedResources['rdma/hca'] = props.flavor.extendedResources?.['rdma/hca'] ?? ''
}

const onOpen = async () => {
  if (isEdit.value || isDetail.value) {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, createInitialForm())
  }
  await nextTick()
}

watch(
  // Auto-fill rdma with 1k when gpu has a value
  () => form.gpu?.quantity,
  (val) => {
    if (isEdit.value) return
    const hasVal = val !== null && val !== undefined && String(val).trim() !== ''

    form.extendedResources['rdma/hca'] = hasVal ? '1k' : ''
    form.gpu.resourceName = hasVal ? 'amd.com/gpu' : ''
  },
  { immediate: true },
)
</script>
