import {
  computed,
  getCurrentScope,
  inject,
  onScopeDispose,
  provide,
  reactive,
  ref,
  watch,
  type InjectionKey,
} from 'vue'
import debounce from 'lodash/debounce'
import { getDatasets } from '@/services/dataset'
import type { DatasetItem } from '@/services/dataset/type'
import { getNodeFlavorAvail } from '@/services/workload'
import {
  errorMessageOf,
  fieldStep,
  getPostTrainingConfig,
  getPostTrainingModels,
  previewPostTrainingRecipe,
  type Choices,
  type ConfigResp,
  type Diagnostic,
  type ModelsResp,
  type OptionValue,
  type PreviewResp,
  type RunDetail,
  type SubmitRequest,
} from '@/services/postTraining'

export const STEPS = [
  'Task & Model',
  'Data',
  'Resources & Layout',
  'Training',
  'Outputs',
  'Review & Submit',
] as const

export const PREVIEW_DEBOUNCE_MS = 400

/** The longest run name the API takes. */
export const MAX_NAME_LENGTH = 128

/** The name of a clone of a run: the run's name with -clone, cut so that it stays within MAX_NAME_LENGTH. */
export function cloneName(name: string): string {
  const suffix = '-clone'
  // Cut by code point, as the API counts, so a character outside the BMP is not split.
  return [...name].slice(0, MAX_NAME_LENGTH - suffix.length).join('') + suffix
}

/** The dataset type each train type trains on. */
export const DATASET_TYPE: Record<'sft' | 'rl', string> = { sft: 'sft', rl: 'rlhf' }

/** The target fields of the dataset mapping, with their default source columns. */
export const FIELD_TARGETS: Record<'sft' | 'rl', string[]> = {
  sft: ['instruction', 'input', 'output'],
  rl: ['prompt', 'label'],
}

/** Returns a request with the wizard's starting values. */
export function newRequest(workspace: string): SubmitRequest {
  return {
    workspace,
    displayName: '',
    trainType: 'sft',
    method: 'full',
    modelId: '',
    datasetId: '',
    choices: {},
    resources: { nodes: 1, gpusPerNode: 8, maxRetry: 1, priority: 1 },
    export: { hf: true },
    tracking: { wandb: {} },
  }
}

/** A deep copy of plain JSON data. */
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T

/** The recipe snapshot fields of a run that a copy made from it starts from. */
interface RunSnapshot {
  trainType?: 'sft' | 'rl'
  method?: string
  framework?: string
  model?: { id?: string }
  data?: { trainId?: string }
  choices?: Choices
  layout?: { nodes?: number }
  output?: { exportHf?: boolean }
  tracking?: SubmitRequest['tracking']
}

/**
 * Returns the request a run's recipe snapshot gives a copy, under the run's name; the image, priority and timeout
 * are the caller's to decide.
 */
export function requestFromRun(run: RunDetail): SubmitRequest {
  const rec = (run.recipe ?? {}) as RunSnapshot
  return {
    workspace: run.workspace,
    displayName: run.displayName,
    trainType: rec.trainType ?? run.trainType,
    method: rec.method ?? run.method,
    framework: rec.framework ?? run.framework,
    modelId: rec.model?.id ?? run.baseModelId,
    datasetId: rec.data?.trainId ?? run.datasetId,
    evalDatasetId: run.evalDatasetId || undefined,
    choices: clone(rec.choices ?? {}),
    resources: { nodes: rec.layout?.nodes ?? run.nodeCount, gpusPerNode: run.gpuPerNode },
    export: { hf: rec.output?.exportHf ?? true },
    tracking: rec.tracking ? clone(rec.tracking) : { wandb: {} },
  }
}

/** Groups the errors and warnings of a preview by the wizard step that fixes them; step 0 holds the rest. */
export function diagnosticsByStep(diagnostics: Diagnostic[]): Map<number, Diagnostic[]> {
  const out = new Map<number, Diagnostic[]>()
  for (const d of diagnostics) {
    const step = fieldStep(d.field)
    out.set(step, [...(out.get(step) ?? []), d])
  }
  return out
}

/** Creates the state of one wizard: the request, what it is checked against and the latest preview. */
export function createWizard(workspace: string, nodeFlavor?: string) {
  const form = reactive<SubmitRequest>(newRequest(workspace))
  const models = ref<ModelsResp>()
  const config = ref<ConfigResp>()
  const trainDatasets = ref<DatasetItem[]>([])
  const preview = ref<PreviewResp>()
  const previewError = ref('')
  const checking = ref(false)
  const checkedAt = ref(0)
  // Whether the request changed since the last preview started; with checking, the preview shown may be stale.
  const dirty = ref(false)
  // The GPU count of the workspace's nodes, zero until it is read.
  const nodeGpus = ref(0)
  let previewSeq = 0
  let modelsSeq = 0
  // The train type and method the models were read for.
  let modelsFor = ''

  /**
   * Reads which models of the workspace can be trained with the request's train type and method, and why the
   * workspace cannot run now; it reads again only when they changed, and drops an answer for older ones.
   */
  const loadModels = async () => {
    const key = `${form.trainType}/${form.method}`
    if (key === modelsFor && models.value) return
    const seq = ++modelsSeq
    const res = (await getPostTrainingModels(workspace, {
      trainType: form.trainType,
      method: form.method,
    })) as unknown as ModelsResp
    if (seq !== modelsSeq) return
    models.value = res
    modelsFor = key
  }

  /** Reads the GPU count of the workspace's nodes; the request starts with all of them. */
  const loadNodeGpus = async () => {
    if (!nodeFlavor) return
    const avail = (await getNodeFlavorAvail(nodeFlavor)) as Record<string, number>
    const gpus = Number(avail['amd.com/gpu'] ?? avail['nvidia.com/gpu'] ?? 0)
    if (gpus > 0) {
      nodeGpus.value = gpus
      form.resources.gpusPerNode = gpus
    }
  }
  // A copied run cannot ask for more GPUs than a node has.
  watch([() => form.resources.gpusPerNode, nodeGpus], ([gpus, max]) => {
    if (max > 0 && gpus > max) form.resources.gpusPerNode = max
  })

  /** Reads the frameworks and options of the selected model, train type and method. */
  const loadConfig = async () => {
    if (!form.modelId) {
      config.value = undefined
      return
    }
    config.value = (await getPostTrainingConfig({
      workspace,
      modelId: form.modelId,
      trainType: form.trainType,
      method: form.method,
      framework: form.framework,
    })) as unknown as ConfigResp
    if (!form.framework && config.value?.framework) form.framework = config.value.framework
  }

  /** Reads the ready datasets of the train type. */
  const loadDatasets = async () => {
    const train = await getDatasets({
      workspace,
      datasetType: DATASET_TYPE[form.trainType],
      pageSize: 200,
    })
    trainDatasets.value = (train.items ?? []).filter((d) => !d.status || d.status === 'Ready')
  }

  /** Compiles the request; an answer to an older request is dropped. */
  const runPreview = async () => {
    const seq = ++previewSeq
    // The request is read now: an edit from here on marks it dirty again and schedules another preview.
    dirty.value = false
    if (!form.modelId) {
      // The compiler needs the base model; a request without one is refused.
      preview.value = undefined
      previewError.value = ''
      checking.value = false
      return
    }
    checking.value = true
    try {
      const res = (await previewPostTrainingRecipe(
        JSON.parse(JSON.stringify(form)),
      )) as unknown as PreviewResp
      if (seq !== previewSeq) return
      preview.value = res
      previewError.value = ''
      checkedAt.value = Date.now()
    } catch (err) {
      if (seq !== previewSeq) return
      previewError.value = errorMessageOf(err, 'The preflight check')
    } finally {
      if (seq === previewSeq) checking.value = false
    }
  }
  const schedulePreview = debounce(runPreview, PREVIEW_DEBOUNCE_MS)
  watch(
    form,
    () => {
      dirty.value = true
      schedulePreview()
    },
    { deep: true },
  )
  /** Whether a preview of the current request is still to come: one waits for the debounce or is in flight. */
  const previewPending = computed(() => dirty.value || checking.value)

  /** Cancels the preview waiting for the debounce and drops the answer of one in flight. */
  const dispose = () => {
    schedulePreview.cancel()
    previewSeq++
    modelsSeq++
  }
  // The page that creates the wizard disposes of it when it unmounts.
  if (getCurrentScope()) onScopeDispose(dispose)

  const diagnostics = computed(() => preview.value?.diagnostics ?? [])
  const errors = computed(() => diagnostics.value.filter((d) => d.level === 'error'))
  const warnings = computed(() => diagnostics.value.filter((d) => d.level === 'warning'))
  const byStep = computed(() => diagnosticsByStep(errors.value))

  /** Returns the options of a config field, or none. */
  const fieldValues = (name: string): OptionValue[] =>
    config.value?.options?.fields?.[name]?.values ?? []

  return {
    workspace,
    form,
    models,
    config,
    trainDatasets,
    preview,
    previewError,
    checking,
    checkedAt,
    previewPending,
    nodeGpus,
    errors,
    warnings,
    byStep,
    fieldValues,
    loadModels,
    loadNodeGpus,
    loadConfig,
    loadDatasets,
    runPreview,
    schedulePreview,
    dispose,
  }
}

export type Wizard = ReturnType<typeof createWizard>

const WIZARD_KEY: InjectionKey<Wizard> = Symbol('post-training-wizard')

/** Makes a wizard available to the step components below. */
export function provideWizard(wizard: Wizard) {
  provide(WIZARD_KEY, wizard)
}

/** Returns the wizard of the page. */
export function useWizard(): Wizard {
  const wizard = inject(WIZARD_KEY)
  if (!wizard) throw new Error('useWizard needs provideWizard')
  return wizard
}
