<template>
  <el-drawer
    :model-value="visible"
    :title="`${props.action} NodeFlavor`"
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
      <el-form
        ref="ruleFormRef"
        :model="form"
        :rules="rules"
        label-width="120px"
        :validate-on-rule-change="false"
        :disabled="isDetail"
      >
        <!-- ===== Basic Information ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Basic Information</div>
              <div class="section-subtitle">Name and memory capacity</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Name" prop="name">
                <el-input v-model="form.name" :disabled="isEdit" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Memory" prop="memory">
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
        </div>

        <!-- ===== CPU ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">CPU</div>
              <div class="section-subtitle">CPU quantity and product</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Quantity" prop="cpu.quantity">
                <el-input v-model="form.cpu.quantity">
                  <template #append>core</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Product">
                <el-input v-model="form.cpu.product" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== GPU ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">GPU</div>
              <div class="section-subtitle">GPU quantity, product and resource name</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Quantity">
                <el-input v-model="form.gpu.quantity">
                  <template #append>card</template>
                </el-input>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Product">
                <el-input v-model="form.gpu.product" />
              </el-form-item>
            </el-col>
            <el-col :span="24">
              <el-form-item label="Resource Name">
                <el-input v-model="form.gpu.resourceName" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== Root Disk ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Root Disk</div>
              <div class="section-subtitle">Root disk quantity, type and count</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Quantity" prop="rootDisk.quantity">
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
              <el-form-item label="Type" prop="rootDisk.type">
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
              <el-form-item label="Count" prop="rootDisk.count">
                <el-input v-model="form.rootDisk.count" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== Data Disk ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Data Disk</div>
              <div class="section-subtitle">Data disk quantity, type and count</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Quantity" prop="dataDisk.quantity">
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
              <el-form-item label="Type" prop="dataDisk.type">
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
              <el-form-item label="Count" prop="dataDisk.count">
                <el-input v-model="form.dataDisk.count" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- ===== Extended Resources ===== -->
        <div class="section-card">
          <div class="section-header">
            <div class="section-bar"></div>
            <div>
              <div class="section-title">Extended Resources</div>
              <div class="section-subtitle">Ephemeral storage and RDMA</div>
            </div>
          </div>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="ephemeralStorage">
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
              <el-form-item label="rdma/hca">
                <el-input v-model="form.extendedResources['rdma/hca']" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </el-form>
    </div>

    <!-- Footer fixed at bottom -->
    <template #footer v-if="!isDetail">
      <div class="drawer-footer">
        <el-button @click="cancelAdd">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, ref, computed, nextTick, toRaw, watch } from 'vue'
import type { FlavorOptionsType, CreateFlavorPayload } from '@/services'
import { addNodeFlavor, editNodeFlavor } from '@/services'
import { type FormInstance, type FormRules, ElMessage, ElMessageBox } from 'element-plus'
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
const submitting = ref(false)
const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  if (submitting.value) return
  try {
    await formEl.validate()
    submitting.value = true

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
  } finally {
    submitting.value = false
  }
}

const cancelAdd = () => {
  // In Detail (read-only) mode, close directly without confirmation
  if (isDetail.value) {
    emit('update:visible', false)
    return
  }
  ElMessageBox.confirm('All fields will be cleared.', 'Clear form & close?', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(() => {
    emit('update:visible', false)
    Object.assign(form, createInitialForm())
  })
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

/* Drawer footer */
.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 10px 24px;
  border-top: 1px solid var(--el-border-color-lighter);
}
</style>
