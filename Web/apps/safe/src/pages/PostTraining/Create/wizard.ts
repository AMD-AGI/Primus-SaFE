import { computed, inject, provide, reactive, ref, watch, type InjectionKey } from 'vue'
import debounce from 'lodash/debounce'
import { getDatasets } from '@/services/dataset'
import type { DatasetItem } from '@/services/dataset/type'
import { getNodeFlavorAvail } from '@/services/workload'
import {
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
  type TemplateContent,
  type TemplateItem,
} from '@/services/postTraining'

export const STEPS = [
  'Task & Model',
  'Data',
  'Resources & Layout',
  'Training',
  'Outputs & Evaluation',
  'Review & Submit',
] as const

export const PREVIEW_DEBOUNCE_MS = 400

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
    resources: { nodes: 1, gpusPerNode: 8, maxRetry: 1 },
    export: { hf: true },
    tracking: { wandb: {} },
    evaluation: {},
  }
}

/** A deep copy of plain JSON data. */
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T

/** The recipe snapshot fields of a run that a copy or a template made from it starts from. */
interface RunSnapshot {
  trainType?: 'sft' | 'rl'
  method?: string
  framework?: string
  model?: { id?: string; spec?: string }
  data?: { trainId?: string }
  choices?: Choices
  layout?: { nodes?: number }
  output?: { exportHf?: boolean; peftAdapter?: boolean }
  evaluation?: SubmitRequest['evaluation']
  tracking?: SubmitRequest['tracking']
}

/**
 * Returns the request a run's recipe snapshot gives a copy or a template, under the run's name; the image, priority
 * and timeout are the caller's to decide.
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
    export: { hf: rec.output?.exportHf ?? true, peftAdapter: rec.output?.peftAdapter },
    evaluation: clone(rec.evaluation ?? {}),
    tracking: rec.tracking ? clone(rec.tracking) : { wandb: {} },
  }
}

/** The spec a run's recipe matched. */
export function specOfRun(run: RunDetail): string | undefined {
  return ((run.recipe ?? {}) as RunSnapshot).model?.spec
}

/**
 * Returns what a template keeps of a request: the options and settings, and the base model and datasets when
 * include says so. The image stays out, so runs from the template use the current default image.
 */
export function templateContent(
  form: SubmitRequest,
  include: { model: boolean; data: boolean },
): TemplateContent {
  const { nodes, gpusPerNode, priority, timeoutSeconds, maxRetry } = form.resources
  const content: TemplateContent = clone({
    framework: form.framework,
    trainType: form.trainType,
    method: form.method,
    preset: form.preset,
    choices: form.choices,
    resources: { nodes, gpusPerNode, priority, timeoutSeconds, maxRetry },
    export: form.export,
    evaluation: form.evaluation,
    tracking: form.tracking,
  })
  if (include.model) content.modelId = form.modelId
  if (include.data) {
    content.datasetId = form.datasetId
    if (form.evalDatasetId) content.evalDatasetId = form.evalDatasetId
  }
  return content
}

/** The parallel settings that follow a spec's defaults. */
const PARALLEL_CHOICES: (keyof Choices)[] = ['tp', 'pp', 'cp', 'ep', 'sequenceParallel']

/**
 * Applies a template to a request: its options and settings and, when it keeps them, its base model and datasets.
 * When the template was saved for another spec than the one modelSpec names, the parallel settings go back to the
 * spec's defaults; it returns whether they did.
 */
export function applyTemplate(
  form: SubmitRequest,
  tpl: TemplateItem,
  modelSpec?: (modelId: string) => string | undefined,
): boolean {
  const c = clone(tpl.content)
  Object.assign(form, {
    framework: c.framework ?? form.framework,
    trainType: c.trainType ?? tpl.trainType,
    method: c.method ?? tpl.method,
    preset: c.preset,
    choices: c.choices ?? {},
    export: c.export ?? form.export,
    evaluation: c.evaluation ?? {},
    tracking: c.tracking ?? { wandb: {} },
    templateId: tpl.templateId,
  })
  if (c.resources) Object.assign(form.resources, c.resources)
  if (c.modelId) form.modelId = c.modelId
  if (c.datasetId) form.datasetId = c.datasetId
  if (c.evalDatasetId) form.evalDatasetId = c.evalDatasetId
  const spec = modelSpec?.(form.modelId)
  if (!tpl.spec || !spec || spec === tpl.spec) return false
  for (const k of PARALLEL_CHOICES) delete form.choices[k]
  return true
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
  const evalDatasets = ref<DatasetItem[]>([])
  const preview = ref<PreviewResp>()
  const previewError = ref('')
  const checking = ref(false)
  const checkedAt = ref(0)
  let previewSeq = 0

  /** Reads which models of the workspace can be trained and why the workspace cannot run now. */
  const loadModels = async () => {
    models.value = (await getPostTrainingModels(workspace)) as unknown as ModelsResp
  }

  /** Reads the GPU count of the workspace's nodes. */
  const loadNodeGpus = async () => {
    if (!nodeFlavor) return
    const avail = (await getNodeFlavorAvail(nodeFlavor)) as Record<string, number>
    const gpus = Number(avail['amd.com/gpu'] ?? avail['nvidia.com/gpu'] ?? 0)
    if (gpus > 0) form.resources.gpusPerNode = gpus
  }

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

  /** Reads the ready datasets of the train type and the evaluation datasets of the workspace. */
  const loadDatasets = async () => {
    const [train, evaluation] = await Promise.all([
      getDatasets({ workspace, datasetType: DATASET_TYPE[form.trainType], pageSize: 200 }),
      getDatasets({ workspace, datasetType: 'evaluation', pageSize: 200 }),
    ])
    trainDatasets.value = (train.items ?? []).filter((d) => !d.status || d.status === 'Ready')
    evalDatasets.value = (evaluation.items ?? []).filter((d) => !d.status || d.status === 'Ready')
  }

  /** Compiles the request; an answer to an older request is dropped. */
  const runPreview = async () => {
    const seq = ++previewSeq
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
      const status = (err as { response?: { status?: number } })?.response?.status
      previewError.value = status
        ? `POST /post-training/recipes/preview returned ${status}.`
        : String(err)
    } finally {
      if (seq === previewSeq) checking.value = false
    }
  }
  const schedulePreview = debounce(runPreview, PREVIEW_DEBOUNCE_MS)
  watch(form, () => schedulePreview(), { deep: true })

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
    evalDatasets,
    preview,
    previewError,
    checking,
    checkedAt,
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
