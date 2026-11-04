<template>
  <el-drawer
    :model-value="modelValue"
    title="Submit as A/B experiment"
    size="760px"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @open="onOpen"
  >
    <el-form label-position="top">
      <el-form-item label="Experiment name" required>
        <el-input v-model="name" maxlength="96" />
      </el-form-item>
    </el-form>

    <div class="text-[13px] mb-2">Runs baseline + up to {{ MAX_VARIANTS }} variants</div>
    <el-table :data="rows" size="small">
      <el-table-column label="Variant" width="170">
        <template #default="{ row }">
          <span v-if="row.baseline" class="font-500">baseline</span>
          <el-input v-else v-model="row.variant.name" size="small" />
        </template>
      </el-table-column>
      <el-table-column label="Changes from Baseline" min-width="220">
        <template #default="{ row }">
          <span v-if="row.baseline">This recipe</span>
          <template v-else>
            <div
              v-for="[key, value] in Object.entries(row.variant.changes)"
              :key="key"
              class="flex items-center gap-1"
            >
              <span>{{ labelOf(key) }}: {{ baselineValue(key) }} → {{ value }}</span>
              <el-button link type="danger" @click="removeChange(row.variant, key)">×</el-button>
            </div>
            <span v-if="!Object.keys(row.variant.changes).length" class="text-[var(--safe-muted)]">
              No changes (repeat of baseline)
            </span>
          </template>
        </template>
      </el-table-column>
      <el-table-column label="Preflight" width="120">
        <template #default="{ row }">
          <span v-if="row.check.checking">Checking…</span>
          <el-popover v-else-if="row.check.errors.length" trigger="click" width="320">
            <template #reference>
              <el-button link type="danger">
                {{ row.check.errors.length }} error{{ row.check.errors.length === 1 ? '' : 's' }}
              </el-button>
            </template>
            <div v-for="(d, i) in row.check.errors" :key="i" class="text-[12px]">
              · {{ d.message }}
            </div>
          </el-popover>
          <span v-else class="text-[var(--el-color-success)]">OK</span>
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="130">
        <template #default="{ row }">
          <template v-if="!row.baseline">
            <el-button link type="primary" @click="editing = row.variant">Edit</el-button>
            <el-button link type="danger" @click="removeVariant(row.variant)">Remove</el-button>
          </template>
        </template>
      </el-table-column>
    </el-table>

    <el-tooltip content="Up to 5 runs per experiment." :disabled="variants.length < MAX_VARIANTS">
      <span>
        <el-button
          class="mt-2"
          round
          :disabled="variants.length >= MAX_VARIANTS"
          @click="addVariant"
        >
          Add variant
        </el-button>
      </span>
    </el-tooltip>

    <el-card v-if="editing" class="mt-3" shadow="never">
      <div class="font-500 mb-2">Edit variant {{ editing.name }}</div>
      <div class="flex items-end gap-2 flex-wrap">
        <el-form-item label="Field" class="mb-0">
          <el-select v-model="draft.key" style="width: 220px">
            <el-option v-for="f in fields" :key="f.key" :label="f.label" :value="f.key">
              <span>{{ f.label }}</span>
              <span class="float-right text-[12px] text-[var(--safe-muted)]">
                {{ baselineValue(f.key) }}
              </span>
            </el-option>
            <el-option disabled label="Training type" value="trainType">
              Training type
              <span class="float-right text-[12px]">Must match the baseline</span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="Value" class="mb-0">
          <el-select
            v-if="options(draft.key)"
            v-model="draft.value"
            style="width: 240px"
            filterable
          >
            <el-option
              v-for="o in options(draft.key)!"
              :key="String(o.value)"
              :label="o.label"
              :value="o.value"
              :disabled="o.disabled"
            />
          </el-select>
          <el-input-number v-else v-model="draft.number" :step="numberStep(draft.key)" />
        </el-form-item>
        <el-button type="primary" :disabled="!draft.key" @click="addChange">
          {{ Object.keys(editing.changes).length ? 'Change another field' : 'Set' }}
        </el-button>
      </div>
    </el-card>

    <div class="text-[13px] text-[var(--safe-muted)] mt-3">
      All runs share the same seeds, data shuffle and benchmarks. The baseline model is evaluated
      twice to estimate evaluation noise.
    </div>
    <div v-if="submitError" class="text-[13px] text-[var(--el-color-danger)] mt-2">
      {{ submitError }}
    </div>

    <template #footer>
      <el-button @click="emit('update:modelValue', false)">Cancel</el-button>
      <el-tooltip :content="blocked" :disabled="!blocked" placement="top">
        <span>
          <el-button type="primary" :disabled="!!blocked" :loading="submitting" @click="submit">
            Submit Experiment ({{ variants.length + 1 }} runs)
          </el-button>
        </span>
      </el-tooltip>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import debounce from 'lodash/debounce'
import { getDatasets } from '@/services/dataset'
import type { DatasetItem } from '@/services/dataset/type'
import {
  VARIANT_FIELDS,
  getPostTrainingModels,
  nestChanges,
  previewPostTrainingRecipe,
  submitPostTrainingExperiment,
  type Diagnostic,
  type ModelSupport,
  type ModelsResp,
  type PreviewResp,
  type SubmitRequest,
  type VariantDiagnostic,
} from '@/services/postTraining'
import { DATASET_TYPE, PREVIEW_DEBOUNCE_MS } from '../Create/wizard'

const MAX_VARIANTS = 4

const props = defineProps<{
  modelValue: boolean
  /** The baseline request: the wizard's recipe, or a run's recipe snapshot. */
  baseline: SubmitRequest
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const router = useRouter()

type Check = { checking: boolean; errors: Diagnostic[] }
type Variant = { name: string; changes: Record<string, unknown>; check: Check }

const name = ref('')
const variants = ref<Variant[]>([])
const baselineCheck = reactive<Check>({ checking: false, errors: [] })
const editing = ref<Variant>()
const draft = reactive({
  key: '',
  value: undefined as unknown,
  number: undefined as number | undefined,
})
const models = ref<ModelSupport[]>([])
const datasets = ref<DatasetItem[]>([])
const submitting = ref(false)
const submitError = ref('')

const rows = computed(() => [
  { baseline: true, variant: undefined as unknown as Variant, check: baselineCheck },
  ...variants.value.map((v) => ({ baseline: false, variant: v, check: v.check })),
])

const fields = computed(() =>
  VARIANT_FIELDS.filter((f) => !f.trainType || f.trainType === props.baseline.trainType),
)
const labelOf = (key: string) => VARIANT_FIELDS.find((f) => f.key === key)?.label ?? key

/** The baseline's value of a dotted request field. */
const valueAt = (req: unknown, key: string) =>
  key
    .split('.')
    .reduce<unknown>((node, k) => (node as Record<string, unknown> | undefined)?.[k], req)
const baselineValue = (key: string) => {
  const v = valueAt(props.baseline, key)
  return v === undefined || v === '' ? 'default' : String(v)
}

type Choice = { label: string; value: string; disabled?: boolean }

/** The choices a field offers, or undefined for a number. */
const options = (key: string): Choice[] | undefined => {
  const spec = models.value.find((m) => m.modelId === props.baseline.modelId)?.spec
  switch (key) {
    case 'method':
      return [
        { label: 'Full', value: 'full' },
        { label: 'LoRA', value: 'lora' },
      ]
    case 'choices.correction':
      return [
        { label: 'Off', value: 'none' },
        { label: 'TIS', value: 'tis' },
      ]
    case 'datasetId':
      return datasets.value.map((d) => ({ label: d.displayName, value: d.datasetId }))
    case 'modelId':
      return models.value.map((m) => ({
        label: m.spec === spec ? m.displayName : `${m.displayName} · spec ${m.spec ?? 'none'}`,
        value: m.modelId,
        disabled: !m.supported || m.spec !== spec,
      }))
    default:
      return undefined
  }
}
const numberStep = (key: string) =>
  key === 'choices.lr' ? 1e-6 : key === 'choices.klLossCoef' ? 0.001 : 1

/** A variant name from its first change, such as lr2e-6. */
const nameFrom = (key: string, value: unknown) =>
  `${key.split('.').pop()}${value}`
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 32) || `variant-${variants.value.length + 1}`

/** Merges a variant's changes into a copy of the baseline request. */
const merged = (changes: Record<string, unknown>): SubmitRequest => {
  const out = JSON.parse(JSON.stringify(props.baseline)) as SubmitRequest
  const merge = (dst: Record<string, unknown>, src: Record<string, unknown>) => {
    for (const [k, v] of Object.entries(src)) {
      if (v && typeof v === 'object' && !Array.isArray(v) && dst[k] && typeof dst[k] === 'object') {
        merge(dst[k] as Record<string, unknown>, v as Record<string, unknown>)
      } else {
        dst[k] = v
      }
    }
  }
  merge(out as unknown as Record<string, unknown>, nestChanges(changes))
  return out
}

/** Runs the preflight of one request and records its errors. */
const preflight = async (check: Check, req: SubmitRequest) => {
  check.checking = true
  try {
    const res = (await previewPostTrainingRecipe(req)) as unknown as PreviewResp
    check.errors = (res.diagnostics ?? []).filter((d) => d.level === 'error')
  } catch {
    check.errors = [{ level: 'error', message: 'Preflight unavailable' } as Diagnostic]
  } finally {
    check.checking = false
  }
}
const checkAll = debounce(() => {
  for (const v of variants.value) preflight(v.check, merged(v.changes))
}, PREVIEW_DEBOUNCE_MS)
watch(
  () => variants.value.map((v) => JSON.stringify(v.changes)),
  () => checkAll(),
)

const onOpen = async () => {
  name.value = `${props.baseline.displayName || 'experiment'}-ab`
  variants.value = []
  editing.value = undefined
  submitError.value = ''
  preflight(baselineCheck, props.baseline)
  const [m, d] = await Promise.all([
    getPostTrainingModels(props.baseline.workspace).catch(() => undefined),
    getDatasets({
      workspace: props.baseline.workspace,
      datasetType: DATASET_TYPE[props.baseline.trainType],
      pageSize: 200,
    }).catch(() => undefined),
  ])
  models.value = (m as unknown as ModelsResp | undefined)?.items ?? []
  datasets.value = d?.items ?? []
}

const addVariant = () => {
  const v: Variant = reactive({
    name: `variant-${variants.value.length + 1}`,
    changes: {},
    check: { checking: false, errors: [] },
  })
  variants.value.push(v)
  editing.value = v
  Object.assign(draft, { key: '', value: undefined, number: undefined })
}

const removeVariant = (v: Variant) => {
  variants.value = variants.value.filter((x) => x !== v)
  if (editing.value === v) editing.value = undefined
}

const addChange = () => {
  const v = editing.value
  if (!v || !draft.key) return
  const value = options(draft.key) ? draft.value : draft.number
  if (value === undefined) return
  if (!Object.keys(v.changes).length && /^variant-\d+$/.test(v.name))
    v.name = nameFrom(draft.key, value)
  v.changes = { ...v.changes, [draft.key]: value }
  Object.assign(draft, { key: '', value: undefined, number: undefined })
}

const removeChange = (v: Variant, key: string) => {
  const rest = { ...v.changes }
  delete rest[key]
  v.changes = rest
}

const blocked = computed(() => {
  if (!name.value.trim()) return 'Name the experiment.'
  if (!variants.value.length) return 'Add a variant.'
  const failing = rows.value.filter((r) => r.check.errors.length).length
  if (failing) return `Fix ${failing} variant${failing === 1 ? '' : 's'} to submit`
  return ''
})

type SubmitFailure = {
  response?: { data?: { errorMessage?: string; diagnostics?: VariantDiagnostic[] } }
}

const submit = async () => {
  submitting.value = true
  submitError.value = ''
  try {
    const res = (await submitPostTrainingExperiment({
      displayName: name.value.trim(),
      baseline: props.baseline,
      variants: variants.value.map((v) => ({ name: v.name, changes: nestChanges(v.changes) })),
    })) as unknown as { experimentId: string; runIds: string[] }
    ElMessage.success(`Experiment submitted · ${res.runIds.length} runs`)
    emit('update:modelValue', false)
    router.push({
      path: '/post-training/experiments/detail',
      query: { experimentId: res.experimentId },
    })
  } catch (err) {
    const data = (err as SubmitFailure)?.response?.data
    submitError.value = data?.errorMessage ?? `Couldn't submit: ${String(err)}`
    for (const d of data?.diagnostics ?? []) {
      const check =
        d.variant === 'baseline'
          ? baselineCheck
          : variants.value.find((v) => v.name === d.variant)?.check
      check?.errors.push(d)
    }
  } finally {
    submitting.value = false
  }
}
</script>
