import dayjs from 'dayjs'
import request from '@/services/request'
import { formatBytes } from '@/utils'
import type {
  ConfigResp,
  DiskUsage,
  Experiment,
  ExperimentListResp,
  ExperimentRequest,
  ImportConfig,
  ImportPreview,
  ImportResult,
  ImportScanStatus,
  ImportStage,
  ImportToken,
  ImportTokenStatus,
  MetricPanel,
  MetricsResp,
  ModelsResp,
  NullTime,
  PreviewResp,
  RunDetail,
  SubmitRequest,
  SubmitResp,
  PostTrainingRun,
  RunListParams,
  RunListResp,
  RunSample,
  RunStage,
  SampleKind,
  SamplesResp,
  SegmentState,
  StageEntry,
  StageSegment,
  TemplateBody,
  TemplateItem,
  TemplateListResp,
} from './types'

export * from './types'

/** Lists the models of a workspace with their Post Training support; a 404 means Post Training is off. */
export function getPostTrainingModels(workspace: string) {
  return request.get<ModelsResp>('/post-training/models', {
    params: { workspace },
    skipErrorHandler: true,
  })
}

/** Returns the frameworks and wizard options of a model for a train type and method. */
export function getPostTrainingConfig(params: {
  workspace: string
  modelId: string
  trainType: string
  method: string
  framework?: string
}) {
  return request.get<ConfigResp>('/post-training/config', { params })
}

/** Compiles a request without creating anything; errors are reported, not thrown. */
export function previewPostTrainingRecipe(body: SubmitRequest) {
  return request.post<PreviewResp>('/post-training/recipes/preview', body, {
    skipErrorHandler: true,
  })
}

/** Submits a run; diagnostics of a request that does not compile come back as the error body. */
export function submitPostTrainingRun(body: SubmitRequest) {
  return request.post<SubmitResp>('/post-training/runs', body, { skipErrorHandler: true })
}

/** Returns the wizard step, 1 to 5, that edits the request field a diagnostic names, or 0 when none does. */
export function fieldStep(field?: string): number {
  if (!field) return 0
  const top = field.split('.')[0]
  if (['framework', 'trainType', 'method', 'modelId', 'workspace', 'preset'].includes(top)) return 1
  if (field === 'choices.advantageEstimator') return 1
  if (
    ['datasetId', 'evalDatasetId'].includes(top) ||
    field.startsWith('choices.fieldMap') ||
    field === 'choices.valSplit'
  )
    return 2
  if (
    ['resources.nodes', 'resources.gpusPerNode'].includes(field) ||
    /^choices\.(layout|rolloutGpus|gpusPerEngine|tp|pp|cp|ep|sequenceParallel|pd)/.test(field)
  )
    return 3
  if (top === 'choices') return 4
  if (['export', 'evaluation', 'resources', 'tracking'].includes(top)) return 5
  return 0
}

/** Lists the runs of a workspace, newest first. */
export function listPostTrainingRuns(params: RunListParams, quiet = false) {
  return request.get<RunListResp>('/post-training/runs', { params, skipErrorHandler: quiet })
}

/** Returns a run with its recipe, stage history, output model and evaluations. */
export function getPostTrainingRun(runId: string, quiet = false) {
  return request.get<RunDetail>(`/post-training/runs/${runId}`, { skipErrorHandler: quiet })
}

/**
 * Loads once why Train with Post Training is unavailable for the models of a workspace and returns the reason of a
 * model, or an empty string when the model can be trained.
 */
export async function postTrainingEntryReasons(
  workspace: string,
): Promise<(modelId: string) => string> {
  try {
    const res = (await getPostTrainingModels(workspace)) as unknown as ModelsResp
    const paused = res.pausedReason
      ? `New runs are paused on this cluster. ${res.pausedReason}`
      : ''
    const global = res.unavailableReason || paused
    const byId = new Map(
      (res.items ?? []).map((m) => [m.modelId, m.supported ? '' : m.reason || 'Not supported']),
    )
    return (id) => global || (byId.get(id) ?? 'No Post Training spec matches this model.')
  } catch (err) {
    const status = (err as { response?: { status?: number } })?.response?.status
    const reason =
      status === 404
        ? 'Post Training is not enabled on this cluster.'
        : 'Post Training is unavailable.'
    return () => reason
  }
}

/** Returns a run's metric series, all of them unless names lists patterns, and the markers on them. */
export function getPostTrainingMetrics(
  runId: string,
  params?: { names?: string; maxPoints?: number },
  quiet = false,
) {
  return request.get<MetricsResp>(`/post-training/runs/${runId}/metrics`, {
    params,
    skipErrorHandler: quiet,
  })
}

/** Returns one step of a run's RL samples, the latest unless step is given; signal cancels a superseded read. */
export function getPostTrainingSamples(
  runId: string,
  params: { kind: SampleKind; step?: number; dataset?: string },
  signal?: AbortSignal,
  quiet = false,
) {
  return request.get<SamplesResp>(`/post-training/runs/${runId}/samples`, {
    params,
    signal,
    skipErrorHandler: quiet,
  })
}

/** A rollout's prompt group: its responses in order, their mean reward and whether they all scored the same. */
export interface SampleGroup {
  group: number
  prompt: string
  label: string
  samples: RunSample[]
  rewardMean: number | null
  correct: number
  wrong: number
  noSignal: boolean
}

/** Groups a rollout's samples by prompt group in order; a response with a reward above 0 counts as correct. */
export function groupSamples(samples: RunSample[]): SampleGroup[] {
  const groups = new Map<number, RunSample[]>()
  for (const s of samples) groups.set(s.group, [...(groups.get(s.group) ?? []), s])
  return [...groups.entries()].map(([group, list]) => {
    const rewards = list.filter((s) => s.reward.Valid).map((s) => s.reward.Float64)
    const correct = rewards.filter((r) => r > 0).length
    return {
      group,
      prompt: list[0].prompt,
      label: list[0].label.Valid ? list[0].label.String : '',
      samples: list,
      rewardMean: rewards.length ? rewards.reduce((a, b) => a + b, 0) / rewards.length : null,
      correct,
      wrong: rewards.length - correct,
      noSignal: rewards.length > 1 && rewards.every((r) => r === rewards[0]),
    }
  })
}

const OMITTED = / … \[(\d+) characters omitted\] … /

/** Splits a text the launcher truncated into the kept head, the number of omitted characters and the kept tail. */
export function splitTruncated(
  text: string,
): { head: string; omitted: number; tail: string } | null {
  const m = OMITTED.exec(text)
  if (!m) return null
  return {
    head: text.slice(0, m.index),
    omitted: Number(m[1]),
    tail: text.slice(m.index + m[0].length),
  }
}

/** The last non-empty line of a response, as the samples table shows it. */
export function lastLine(text: string): string {
  const lines = text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  return lines.length ? lines[lines.length - 1] : ''
}

/** The reward of a sample as shown, and the whole reward dict for the tooltip when the reward was one. */
export function rewardOf(s: RunSample): { value: string; detail: string } {
  const value = s.reward.Valid ? String(Math.round(s.reward.Float64 * 1000) / 1000) : '—'
  return { value, detail: s.rewardDetail.Valid ? s.rewardDetail.String : '' }
}

/** The training command of a recipe, one option with its values per line: `head \` then `  --flag value \`. */
export function formatCommand(args: string[], head = 'python3 train.py'): string {
  const lines: string[][] = []
  for (const a of args) {
    if (a.startsWith('--') || !lines.length) lines.push([a])
    else lines[lines.length - 1].push(a)
  }
  return [head, ...lines.map((l) => `  ${l.join(' ')}`)].join(' \\\n')
}

/** The URL of a run's metric points as CSV. */
export function metricsCsvUrl(runId: string) {
  return `/api/v1/post-training/runs/${runId}/metrics/export`
}

/** Evaluates a model of a run again: its output model unless modelId names a model exported from it. */
export function evaluatePostTrainingRun(
  runId: string,
  body: { modelId?: string; benchmarks: string[]; keepService?: boolean },
) {
  return request.post<{ runId: string; modelId: string }>(
    `/post-training/runs/${runId}/evaluate`,
    body,
  )
}

export const NOT_RETRIED_EVENT = 'not-retried'
export const CHECKPOINTS_CLEANED_EVENT = 'checkpoints-cleaned'
export const LOST_MESSAGE = 'The launcher has not reported'
const CLEANUP_HINT_DAYS = 7

/** Exports a kept checkpoint of a finished run as a new model, in its own one-node job. */
export function exportPostTrainingCheckpoint(
  runId: string,
  body: { step: number; displayName?: string; peftAdapter?: boolean },
) {
  return request.post<{ runId: string; step: number; workloadId: string }>(
    `/post-training/runs/${runId}/exports`,
    body,
  )
}

/** Removes a finished run's checkpoints, keeping the one with the best validation loss when keep is 'best'. */
export function cleanupPostTrainingCheckpoints(runId: string, keep: 'best' | 'none') {
  return request.post<{ runId: string; keep: string }>(
    `/post-training/runs/${runId}/checkpoints/cleanup`,
    { keep },
  )
}

/** Submits an A/B experiment; a variant that does not compile refuses the whole experiment. */
export function submitPostTrainingExperiment(body: ExperimentRequest) {
  return request.post<{ experimentId: string; runIds: string[] }>(
    '/post-training/experiments',
    body,
    {
      skipErrorHandler: true,
    },
  )
}

/** Lists a workspace's experiments, or the ones experimentIds names, with their runs. */
export function listPostTrainingExperiments(
  params: { workspace: string; experimentIds?: string; limit?: number },
  quiet = false,
) {
  return request.get<ExperimentListResp>('/post-training/experiments', {
    params,
    skipErrorHandler: quiet,
  })
}

/** Returns an experiment with its variants and runs. */
export function getPostTrainingExperiment(experimentId: string, quiet = false) {
  return request.get<Experiment>(`/post-training/experiments/${experimentId}`, {
    skipErrorHandler: quiet,
  })
}

/** Stops every unfinished run of an experiment. */
export function stopPostTrainingExperiment(experimentId: string) {
  return request.post<{ experimentId: string; stopped: string[] }>(
    `/post-training/experiments/${experimentId}/stop`,
  )
}

/** Deletes an experiment's runs, their output models when deleteModel is set, and then the experiment. */
export function deletePostTrainingExperiment(experimentId: string, deleteModel = false) {
  return request.delete(`/post-training/experiments/${experimentId}`, {
    params: { deleteModel },
    skipErrorHandler: true,
  })
}

/** Lists a workspace's templates, optionally of one train type. */ /** Lists a workspace's templates, optionally of one train type. */
export function listPostTrainingTemplates(params: { workspace: string; trainType?: string }) {
  return request.get<TemplateListResp>('/post-training/templates', { params })
}

/** Returns one template. */
export function getPostTrainingTemplate(templateId: string) {
  return request.get<TemplateItem>(`/post-training/templates/${templateId}`)
}

/** Saves a template in a workspace. */
export function createPostTrainingTemplate(body: TemplateBody) {
  return request.post<TemplateItem>('/post-training/templates', body)
}

/** Edits a template's name, description or content; the version goes up by one. */
export function updatePostTrainingTemplate(templateId: string, body: TemplateBody) {
  return request.put<{ templateId: string; version: number }>(
    `/post-training/templates/${templateId}`,
    body,
  )
}

/** Deletes a template; runs created from it are not affected. */
export function deletePostTrainingTemplate(templateId: string) {
  return request.delete(`/post-training/templates/${templateId}`)
}

/** Stops a run's training workload. */
export function stopPostTrainingRun(runId: string) {
  return request.post<{ runId: string }>(`/post-training/runs/${runId}/stop`)
}

/** Deletes a run; deleteModel also deletes its output model and weights. */
export function deletePostTrainingRun(runId: string, deleteModel = false) {
  return request.delete<void>(`/post-training/runs/${runId}`, { params: { deleteModel } })
}

/** Parses a JSON column of a run, or returns undefined when it is empty or not JSON. */
export function parseJSON<T>(raw?: string): T | undefined {
  if (!raw) return undefined
  try {
    return JSON.parse(raw) as T
  } catch {
    return undefined
  }
}

/** Returns the time of a nullable timestamp column, or undefined when it is not set. */
export function timeOf(t?: NullTime): string | undefined {
  return t?.Valid ? t.Time : undefined
}

export const STAGE_LABELS: Record<RunStage, string> = {
  queued: 'Queued',
  prepare: 'Prepare',
  convert: 'Convert',
  check: 'Check',
  train: 'Train',
  export: 'Export',
  register: 'Register',
  eval: 'Evaluate',
  done: 'Done',
}

/** Returns the seven stages of a run in its own order: a run with a convert workload converts first. */
export function stageOrder(run: Pick<PostTrainingRun, 'convertWorkloadId'>): RunStage[] {
  return run.convertWorkloadId
    ? ['convert', 'prepare', 'check', 'train', 'export', 'register', 'eval']
    : ['prepare', 'convert', 'check', 'train', 'export', 'register', 'eval']
}

/**
 * Returns the stage bar of a run: stages before the current one are done, the current one is active, stopped
 * or failed, later ones are pending; a queued run has no stage started, and Evaluate is skipped without
 * evaluation.
 */
export function stageSegments(
  run: Pick<
    PostTrainingRun,
    'status' | 'stage' | 'failedStage' | 'evalEnabled' | 'convertWorkloadId'
  >,
): StageSegment[] {
  const order = stageOrder(run)
  const current = run.stage === 'done' ? order.length : order.indexOf(run.stage)
  const failedIndex = order.indexOf((run.failedStage || '') as RunStage)
  const failedAt = failedIndex >= 0 ? failedIndex : current
  return order.map((stage, i) => {
    let state: SegmentState = 'pending'
    if (stage === 'eval' && !run.evalEnabled) {
      state = 'skipped'
    } else if (run.status === 'Failed') {
      state = i < failedAt ? 'done' : i === failedAt ? 'failed' : 'pending'
    } else if (i < current) {
      state = 'done'
    } else if (i === current) {
      state = run.status === 'Stopped' ? 'stopped' : 'active'
    }
    return { stage, label: STAGE_LABELS[stage], state }
  })
}

/** Returns the el-tag type of a run status. */
export function statusTagType(status: string): 'primary' | 'success' | 'danger' | 'info' {
  const map: Record<string, 'primary' | 'success' | 'danger' | 'info'> = {
    Running: 'primary',
    Succeeded: 'success',
    Failed: 'danger',
    Pending: 'info',
    Stopped: 'info',
  }
  return map[status] || 'info'
}

/** Formats seconds as "5h 10m", "12m" or "40s". */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m`
  return `${Math.floor(seconds)}s`
}

/**
 * Smooths [x, y] points with a debiased exponential moving average, the way TensorBoard does; weight 0 keeps the
 * points as they are.
 */
export function smoothPoints(points: [number, number][], weight: number): [number, number][] {
  if (weight <= 0) return points
  let last = 0
  return points.map(([x, y], i) => {
    last = last * weight + (1 - weight) * y
    return [x, last / (1 - Math.pow(weight, i + 1))]
  })
}

/**
 * Returns the time a run spent in each stage of its history, from first entering it to entering a later stage;
 * events and resume entries don't start a stage.
 */
export function stageDurations(
  history: StageEntry[],
  until: number,
): Map<string, { start: number; end: number }> {
  const entries = history.filter((e) => !e.event && e.resumeStep === undefined && e.time)
  const out = new Map<string, { start: number; end: number }>()
  entries.forEach((e, i) => {
    if (out.has(e.stage)) return
    const next = entries.slice(i + 1).find((n) => n.stage !== e.stage)
    out.set(e.stage, { start: Date.parse(e.time), end: next ? Date.parse(next.time) : until })
  })
  return out
}

/** The bytes a run's checkpoints take, actor and critic together. */
export function checkpointBytes(disk?: DiskUsage): number {
  return (disk?.checkpoints ?? 0) + (disk?.criticCheckpoints ?? 0)
}

/** The bytes a run's checkpoints still take a week after it ended without a cleanup, or 0. */
export function cleanupHintBytes(
  run: Pick<PostTrainingRun, 'endTime' | 'diskUsage' | 'cleanupRequest'> & {
    stageHistory?: string | StageEntry[]
  },
  now: number,
): number {
  const end = timeOf(run.endTime)
  if (!end || run.cleanupRequest || now - Date.parse(end) < CLEANUP_HINT_DAYS * 86400000) return 0
  const history =
    typeof run.stageHistory === 'string'
      ? parseJSON<StageEntry[]>(run.stageHistory)
      : run.stageHistory
  if (history?.some((e) => e.event === CHECKPOINTS_CLEANED_EVENT)) return 0
  return checkpointBytes(parseJSON<DiskUsage>(run.diskUsage))
}

/** The short stall note of a running run: no progress for some minutes, or a launcher that stopped reporting. */
export function stallNote(
  run: Pick<PostTrainingRun, 'status' | 'stalledSince' | 'message'>,
  now: number,
): string {
  const since = timeOf(run.stalledSince)
  if (run.status !== 'Running' || !since) return ''
  if (run.message?.startsWith(LOST_MESSAGE)) return 'Launcher not reporting'
  return `No progress for ${Math.max(1, Math.round((now - Date.parse(since)) / 60000))} min`
}

/** Recipe fields that differ between any two runs by construction, or repeat other fields. */
const DIFF_SKIPPED = new Set([
  'runId',
  'output.base',
  'data.train',
  'data.eval',
  'files',
  'frameworkArgs',
  'convert.dir',
])

/** Readable names of the recipe fields people compare most. */
const DIFF_LABELS: Record<string, string> = {
  'choices.lr': 'Learning rate',
  'choices.minLr': 'Min learning rate',
  'choices.batchSize': 'Batch size',
  'choices.numEpoch': 'Epochs',
  'choices.numRollout': 'Steps / rollouts',
  'choices.maxTokensPerGpu': 'Max tokens per GPU',
  'choices.saveInterval': 'Save interval',
  'model.id': 'Base model',
  'data.trainId': 'Dataset',
  'provenance.recipeHash': 'Recipe hash',
  'provenance.image': 'Image',
}

/** Flattens a recipe snapshot into dotted field paths and display values; lists are joined with commas. */
export function flattenRecipe(value: unknown, prefix = '', out: Record<string, string> = {}) {
  if (value === null || value === undefined) return out
  if (Array.isArray(value)) {
    out[prefix] = value
      .map((v) => (typeof v === 'object' ? JSON.stringify(v) : String(v)))
      .join(', ')
  } else if (typeof value === 'object') {
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      const path = prefix ? `${prefix}.${k}` : k
      if (!DIFF_SKIPPED.has(path)) flattenRecipe(v, path, out)
    }
  } else if (prefix) {
    out[prefix] = String(value)
  }
  return out
}

/** How many recipe fields differ, as the compare and experiment pages say it. */
export function fieldsDiffer(n: number): string {
  return n === 1 ? '1 field differs' : `${n} fields differ`
}

export interface DiffRow {
  field: string
  label: string
  values: string[]
  differs: boolean
}

/** Lines up the recipe fields of several runs, the recipe hash first; a missing field shows as —. */
export function configDiff(recipes: (Record<string, unknown> | undefined)[]): DiffRow[] {
  const flat = recipes.map((r) => flattenRecipe(r ?? {}))
  const fields = [...new Set(flat.flatMap((f) => Object.keys(f)))].sort((a, b) =>
    a === 'provenance.recipeHash' ? -1 : b === 'provenance.recipeHash' ? 1 : a.localeCompare(b),
  )
  return fields.map((field) => {
    const values = flat.map((f) => f[field] ?? '—')
    const short = field === 'provenance.recipeHash' ? values.map((v) => v.slice(0, 7)) : values
    return {
      field,
      label: DIFF_LABELS[field] ?? field,
      values: short,
      differs: new Set(values).size > 1,
    }
  })
}

/**
 * Compares a variant's benchmark score with the baseline's, all fractions in [0, 1]. The difference is from the mean
 * of the baseline's evaluations; it is significant when larger than both the gap between the baseline's two
 * evaluations and two standard errors at n questions.
 */
export function significance(baseline: number[], variant: number, n?: number) {
  const mean = baseline.reduce((a, b) => a + b, 0) / baseline.length
  const gap = baseline.length > 1 ? Math.abs(baseline[0] - baseline[1]) : 0
  const se = n ? 2 * Math.sqrt((mean * (1 - mean)) / n + (variant * (1 - variant)) / n) : 0
  const threshold = Math.max(gap, se)
  const diff = variant - mean
  return { mean, diff, threshold, significant: Math.abs(diff) > threshold }
}

/** A request field an A/B variant may change: its label and where a recipe snapshot keeps it. */
export interface VariantField {
  key: string
  label: string
  recipeKey: string
  trainType?: 'sft' | 'rl'
}

/** The fields an A/B variant may change from its baseline. */
export const VARIANT_FIELDS: VariantField[] = [
  { key: 'choices.lr', label: 'Learning rate', recipeKey: 'choices.lr' },
  { key: 'choices.batchSize', label: 'Batch size', recipeKey: 'choices.batchSize' },
  { key: 'choices.numRollout', label: 'Steps / rollouts', recipeKey: 'choices.numRollout' },
  { key: 'method', label: 'Method', recipeKey: 'method' },
  { key: 'choices.lora.rank', label: 'LoRA rank', recipeKey: 'choices.lora.rank' },
  { key: 'datasetId', label: 'Dataset', recipeKey: 'data.trainId' },
  { key: 'modelId', label: 'Base model', recipeKey: 'model.id' },
  { key: 'resources.nodes', label: 'Nodes', recipeKey: 'layout.nodes' },
  {
    key: 'choices.nSamplesPerPrompt',
    label: 'Samples per prompt',
    recipeKey: 'choices.nSamplesPerPrompt',
    trainType: 'rl',
  },
  {
    key: 'choices.maxResponseLen',
    label: 'Response length',
    recipeKey: 'choices.maxResponseLen',
    trainType: 'rl',
  },
  {
    key: 'choices.correction',
    label: 'Train–inference correction',
    recipeKey: 'choices.correction',
    trainType: 'rl',
  },
  {
    key: 'choices.klLossCoef',
    label: 'KL coefficient',
    recipeKey: 'choices.klLossCoef',
    trainType: 'rl',
  },
]

/** Flattens a variant's changes into dotted field keys. */
export function flattenChanges(
  changes: Record<string, unknown> = {},
  prefix = '',
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(changes)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      Object.assign(out, flattenChanges(v as Record<string, unknown>, key))
    } else {
      out[key] = v
    }
  }
  return out
}

/** Nests dotted field keys back into a changes object. */
export function nestChanges(flat: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(flat)) {
    const parts = key.split('.')
    let node = out
    parts.slice(0, -1).forEach((p) => {
      node[p] = (node[p] as Record<string, unknown>) ?? {}
      node = node[p] as Record<string, unknown>
    })
    node[parts[parts.length - 1]] = value
  }
  return out
}

/** Matches a metric name against a pattern whose * matches within one path segment, as the registry does. */
export function matchName(pattern: string, name: string): boolean {
  const pp = pattern.split('/')
  const np = name.split('/')
  if (!pattern || pp.length !== np.length) return false
  return pp.every((seg, i) => {
    const re = new RegExp(
      `^${seg
        .split('*')
        .map((p) => p.replace(/[.+?^${}()|[\]\\]/g, '\\$&'))
        .join('.*')}$`,
    )
    return re.test(np[i])
  })
}

/** How specific a pattern is: its characters outside the wildcards. */
const specificity = (pattern: string) => pattern.replace(/\*/g, '').length

/** The metrics a panel draws: its series, its band's low and high series, and its rest series. */
export interface PanelMetrics {
  series: string[]
  band: [string?, string?]
  rest?: string
}

/**
 * Assigns every metric name to the panel whose pattern matches it most specifically, across the series, band and
 * rest patterns of all panels. Returns the metrics of each panel by id and the names no panel draws.
 */
export function assignPanels(panels: MetricPanel[], names: string[]) {
  type Role = 'series' | 'low' | 'high' | 'rest'
  const byPanel = new Map<string, PanelMetrics>(
    panels.map((p) => [p.id, { series: [], band: [undefined, undefined] }]),
  )
  const unassigned: string[] = []
  const order = new Map<string, number>()
  for (const name of names) {
    let best: { panel: string; role: Role; score: number; index: number } | undefined
    const consider = (panel: string, role: Role, pattern: string | undefined, index = 0) => {
      if (pattern && matchName(pattern, name) && (!best || specificity(pattern) > best.score)) {
        best = { panel, role, score: specificity(pattern), index }
      }
    }
    for (const p of panels) {
      p.series.forEach((pattern, i) => consider(p.id, 'series', pattern, i))
      consider(p.id, 'low', p.band?.[0])
      consider(p.id, 'high', p.band?.[1])
      consider(p.id, 'rest', p.rest)
    }
    if (!best) {
      unassigned.push(name)
      continue
    }
    const m = byPanel.get(best.panel)!
    if (best.role === 'series') {
      m.series.push(name)
      order.set(name, best.index)
    } else if (best.role === 'low') m.band[0] = name
    else if (best.role === 'high') m.band[1] = name
    else m.rest = name
  }
  for (const m of byPanel.values()) {
    m.series.sort((a, b) => order.get(a)! - order.get(b)! || a.localeCompare(b))
  }
  return { byPanel, unassigned }
}

/** Whether a panel shows for a run: when: present needs data, grpo, ppo and async need that kind of run. */
export function panelVisible(
  panel: MetricPanel,
  metrics: PanelMetrics,
  run: { algorithm?: string; schedule?: string },
): boolean {
  const hasData = metrics.series.length > 0 || !!metrics.band[0] || !!metrics.band[1]
  switch (panel.when) {
    case 'present':
      return hasData
    case 'grpo':
      return hasData && run.algorithm !== 'ppo'
    case 'ppo':
      return hasData && run.algorithm === 'ppo'
    case 'async':
      return hasData && run.schedule === 'async'
    default:
      return hasData
  }
}

/** Which direction is better for a metric, by the most specific matching pattern. */
export function betterOf(better: Record<string, 'lower' | 'higher'> | undefined, name: string) {
  let best: { dir: 'lower' | 'higher'; score: number } | undefined
  for (const [pattern, dir] of Object.entries(better ?? {})) {
    if (matchName(pattern, name) && (!best || specificity(pattern) > best.score)) {
      best = { dir, score: specificity(pattern) }
    }
  }
  return best?.dir
}

/** The panels a run shows, with their metrics, and the metrics no panel draws. */
export function panelsOf(
  panels: MetricPanel[],
  names: string[],
  run: { algorithm?: string; schedule?: string },
) {
  const { byPanel, unassigned } = assignPanels(panels, names)
  return {
    shown: panels
      .map((panel) => ({ panel, metrics: byPanel.get(panel.id)! }))
      .filter(({ panel, metrics }) => panelVisible(panel, metrics, run)),
    unassigned,
  }
}

/** Reads whether the mounted-path import works in a workspace's cluster and whether the caller may import. */
export function getImportConfig(workspace: string) {
  return request.get<ImportConfig>('/post-training/imports/config', { params: { workspace } })
}

/** Issues a one-time token of the import command for a workspace. */
export function issueImportToken(workspace: string) {
  return request.post<ImportToken>('/post-training/imports/tokens', { workspace })
}

/** Reads whether the command used a token, for the import dialog's polling. */
export function getImportToken(tokenId: string) {
  return request.get<ImportTokenStatus>(`/post-training/imports/tokens/${tokenId}`, {
    skipErrorHandler: true,
  })
}

/** Starts a scan of a run directory under the cluster's mount root. */
export function startImportScan(workspace: string, path: string) {
  return request.post<{ scanId: string; workloadId: string }>(
    '/post-training/imports/scans',
    { workspace, path },
    { skipErrorHandler: true },
  )
}

/** Reads a scan's state, for the import dialog's polling. */
export function getImportScan(scanId: string) {
  return request.get<ImportScanStatus>(`/post-training/imports/scans/${scanId}`, {
    skipErrorHandler: true,
  })
}

/** Imports a scanned directory as a run. */
export function commitImportScan(scanId: string) {
  return request.post<ImportResult>(`/post-training/imports/scans/${scanId}/commit`, undefined, {
    skipErrorHandler: true,
  })
}

/** Tells whether a run was imported rather than trained on the platform. */
export function isImportedRun(run: Pick<PostTrainingRun, 'source'>): boolean {
  return !!run.source && run.source !== 'safe'
}

/** Quotes a word for a POSIX shell unless it holds only characters a shell leaves alone. */
export function shellQuote(word: string): string {
  return /^[\w@%+=:,./-]+$/.test(word) ? word : `'${word.replace(/'/g, `'\\''`)}'`
}

/** The import command with the run directory in place of <run-dir>, which stays while none is entered. */
export function importCommandFor(command: string, dir: string): string {
  const d = dir.trim()
  return d ? command.replace('<run-dir>', shellQuote(d)) : command
}

/** Shortens a Spur path to its first and last directories, as /shared_nfs/…/j2r7-242118. */
export function shortSpurPath(path: string): string {
  const parts = path.split('/').filter(Boolean)
  return parts.length > 2 ? `/${parts[0]}/…/${parts[parts.length - 1]}` : path
}

/** Counts things, as "1 node" or "2 nodes". */
const count = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`

/** A file size as the import command prints it: 131 KB, 2.7 MB. */
const sizeText = (bytes: number) => formatBytes(bytes, 1).replace(/\.0 /, ' ')

/** The rows of the import dialog's preview of a run directory. */
export function importPreviewRows(p: ImportPreview): { label: string; value: string }[] {
  const g = p.failedGate
  let verdict = p.verdict || 'No verdict'
  if (p.verdict === 'FAIL' && g) verdict = `FAIL ${g.gate} ${g.result} on ${g.node}`
  const run = [
    p.trainType.toUpperCase(),
    p.slurmJobId ? `Slurm job ${p.slurmJobId}` : '',
    count(p.nodes, 'node'),
    p.startTime ? `began ${dayjs(p.startTime).format('MM-DD HH:mm')}` : '',
  ]
  const log = p.mainLog
  let checkpoint = '--save not set'
  if (p.saveSet)
    checkpoint = p.checkpoints ? `${count(p.checkpoints, 'checkpoint')} in ckpt/` : 'None in ckpt/'
  return [
    { label: 'Verdict', value: verdict },
    { label: 'Run', value: run.filter(Boolean).join(' · ') },
    {
      label: 'Metrics',
      value: log
        ? `${log.name} · ${sizeText(log.bytes)} · ${count(log.metricLines, 'step line')}`
        : 'No main log',
    },
    { label: 'Gates', value: `${count(p.gates, 'result')} · ${count(p.markers, 'marker')}` },
    {
      label: 'Arguments',
      value: `${p.arguments.toLocaleString('en-US')} parsed · ${p.keptArguments} kept`,
    },
    {
      label: 'Resources',
      value: `${count(p.samples, 'sample')} from ${count(p.sampleNodes, 'node')}`,
    },
    { label: 'Checkpoint', value: checkpoint },
  ]
}

/** The hints under an import preview: warnings change what is imported, notes only explain. */
export function importHints(p: ImportPreview): { type: 'warning' | 'info'; text: string }[] {
  const out: { type: 'warning' | 'info'; text: string }[] = []
  if (!p.verdict) {
    out.push({
      type: 'warning',
      text: 'No PASS or FAIL in the run directory, so the run is imported as Stopped.',
    })
  }
  if (!p.series) {
    out.push({
      type: 'warning',
      text: `No step, rollout or eval lines in ${p.mainLog?.name ?? 'the main log'}, so there are no curves.`,
    })
  }
  if (p.mainLog && p.mainLog.bytes >= 1e9) {
    out.push({
      type: 'info',
      text: `${p.mainLog.name} is ${sizeText(p.mainLog.bytes)}; only its metric lines are kept.`,
    })
  }
  for (const w of p.warnings ?? [])
    out.push({ type: w.includes('is not a known gate') ? 'info' : 'warning', text: w })
  return out
}

/** The stage bar of an imported run: the stages its directory shows, one cut short shown stopped. */
export function importedSegments(run: Pick<PostTrainingRun, 'stageHistory'>): StageSegment[] {
  return (parseJSON<ImportStage[]>(run.stageHistory) ?? []).map((s) => ({
    stage: s.stage,
    label: s.label || s.stage,
    state: s.result === 'pass' ? 'done' : s.result === 'fail' ? 'failed' : 'stopped',
  }))
}

/** The caption under an imported run's stage bar: its verdict with the steps or rollouts logged up to step. */
export function importedCaption(
  run: Pick<PostTrainingRun, 'status' | 'trainType' | 'failedStage' | 'import'>,
  step?: number,
): string {
  if (run.status === 'Failed')
    return `FAIL at ${run.import?.failedGate?.gate || run.failedStage || 'a gate'}`
  const logged = count(step == null ? 0 : step + 1, run.trainType === 'rl' ? 'rollout' : 'step')
  return run.status === 'Succeeded' ? `PASS · ${logged}` : `No verdict · ${logged} logged`
}

/** The line under an imported run's status: why its check failed, or that its directory has no verdict. */
export function importedStatusNote(run: Pick<PostTrainingRun, 'status' | 'message'>): string {
  if (run.status === 'Stopped') return 'No verdict in the run directory'
  if (run.status !== 'Failed') return ''
  const text = (run.message ?? '')
    .split('\n')[0]
    .trim()
    .replace(/^FAIL:\s*/, 'Check failed: ')
  return text && !/[.!?]$/.test(text) ? `${text}.` : text
}

/** The tooltip of an imported run's tag: when and from where it was imported. */
export function importedTip(run: Pick<PostTrainingRun, 'import' | 'outputPath'>): string {
  const at = run.import?.importedAt ? ` ${dayjs(run.import.importedAt).format('MM-DD HH:mm')}` : ''
  const path = run.import?.sourcePath || run.outputPath
  return `Imported${at}${path ? ` from ${shortSpurPath(path)}` : ''}. Read-only, and not counted in Verified.`
}

/** The line under an imported run's type: its Slurm job and nodes on Spur. */
export function spurLine(run: Pick<PostTrainingRun, 'import' | 'nodeCount'>): string {
  return [
    'Spur',
    run.import?.slurmJobId ? `job ${run.import.slurmJobId}` : '',
    run.nodeCount ? count(run.nodeCount, 'node') : '',
  ]
    .filter(Boolean)
    .join(' · ')
}
