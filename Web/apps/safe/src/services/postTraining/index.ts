import type { Router } from 'vue-router'
import request from '@/services/request'
import type {
  Choices,
  ConfigResp,
  DiskUsage,
  FreezeOptions,
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
  RunStage,
  SegmentState,
  StageEntry,
  StageSegment,
} from './types'

export * from './types'

/**
 * Lists the models of a workspace with whether they can be trained, for a train type and method when given; a 404
 * means Post Training is off.
 */
export function getPostTrainingModels(
  workspace: string,
  support?: { trainType?: string; method?: string },
) {
  return request.get<ModelsResp>('/post-training/models', {
    params: { workspace, ...support },
    skipErrorHandler: true,
  })
}

/**
 * Whether Post Training is on in the cluster, read from one run of the workspace's list, the cheapest call of its
 * API. Only an answer says it is on: where it is off the API answers 404, and a call that fails otherwise (no
 * access to the workspace, a server error) cannot tell, so the legacy Post Train entries stay.
 */
export async function postTrainingIsOn(workspace: string): Promise<boolean> {
  try {
    await request.get<RunListResp>('/post-training/runs', {
      params: { workspace, limit: 1 },
      skipErrorHandler: true,
    })
    return true
  } catch {
    return false
  }
}

/** The message of a failed call: the API's errorMessage, or what failed when the answer has none. */
export function errorMessageOf(err: unknown, what: string): string {
  const e = err as { response?: { status?: number; data?: { errorMessage?: string } } } | undefined
  if (e?.response?.data?.errorMessage) return e.response.data.errorMessage
  if (e?.response?.status) return `${what} returned ${e.response.status}.`
  return err instanceof Error ? err.message : String(err)
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

// A submit compiles the request, records the run and creates its workload, which can take longer than the 10 s of
// the other calls; a submit cut off by the client may still have created the run.
export const SUBMIT_TIMEOUT_MS = 60_000

/** Whether a call failed without an answer, cut off by its timeout or the network, so its outcome is unknown. */
export function noAnswer(err: unknown): boolean {
  const e = err as { response?: unknown; code?: string } | undefined
  return !e?.response && ['ECONNABORTED', 'ETIMEDOUT', 'ERR_NETWORK'].includes(e?.code ?? '')
}

/** Submits a run; diagnostics of a request that does not compile come back as the error body. */
export function submitPostTrainingRun(body: SubmitRequest) {
  return request.post<SubmitResp>('/post-training/runs', body, {
    skipErrorHandler: true,
    timeout: SUBMIT_TIMEOUT_MS,
  })
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
    /^choices\.(layout|rolloutGpus|gpusPerEngine|tp|pp|cp|ep|sequenceParallel)/.test(field)
  )
    return 3
  if (top === 'choices') return 4
  if (['export', 'resources', 'tracking'].includes(top)) return 5
  return 0
}

/** Lists the runs of a workspace, newest first. */
export function listPostTrainingRuns(params: RunListParams, quiet = false) {
  return request.get<RunListResp>('/post-training/runs', { params, skipErrorHandler: quiet })
}

/** Returns a run with its recipe, stage history and output model. */
export function getPostTrainingRun(runId: string, quiet = false) {
  return request.get<RunDetail>(`/post-training/runs/${runId}`, { skipErrorHandler: quiet })
}

/**
 * Loads once why Train with Post Training is unavailable for the models of a workspace and returns the reason of a
 * model, or an empty string when the model can be trained; there is none where Post Training is off.
 */
export async function postTrainingEntryReasons(
  workspace: string,
): Promise<((modelId: string) => string) | undefined> {
  try {
    const res = (await getPostTrainingModels(workspace)) as unknown as ModelsResp
    const paused = res.pausedReason
      ? `New runs are paused on this cluster. ${res.pausedReason}`
      : ''
    const global = res.unavailableReason || paused
    const byId = new Map(
      (res.items ?? []).map((m) => [m.modelId, m.supported ? '' : m.reason || 'Not supported']),
    )
    // The list leaves out the models not ready in the workspace.
    return (id) => global || (byId.get(id) ?? 'The model is not ready in this workspace.')
  } catch {
    // As for postTrainingIsOn, only an answer says Post Training is on: a 404 says it is off, and any other
    // failure cannot tell, so the page keeps the legacy Post Train entries as the menu does.
    return undefined
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

/** Returns to the run list as it was left, its filters and page included, or opens it. */
export function backToRuns(router: Router) {
  const back = (window.history.state as { back?: unknown } | null)?.back
  if (typeof back === 'string' && /^\/post-training(\?|$)/.test(back)) router.back()
  else router.push('/post-training')
}

/** The tabs of a run that show its curves or the markers read with them. */
const CURVE_TABS = ['overview', 'metrics']
// A little under three of a run page's 10 s ticks, so the curves are read on every third one.
const CURVE_READ_MS = 25_000

/** Whether a run page reads its curves on this tick: its tab shows them and they were last read at readAt. */
export function curvesDue(tab: string, readAt: number, now = Date.now()): boolean {
  return CURVE_TABS.includes(tab) && now - readAt >= CURVE_READ_MS
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

export const NOT_RETRIED_EVENT = 'not-retried'
export const LOST_MESSAGE = 'The launcher has not reported'

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
  done: 'Done',
}

/** Returns the six stages of a run in its own order: a run with a convert workload converts first. */
export function stageOrder(run: Pick<PostTrainingRun, 'convertWorkloadId'>): RunStage[] {
  return run.convertWorkloadId
    ? ['convert', 'prepare', 'check', 'train', 'export', 'register']
    : ['prepare', 'convert', 'check', 'train', 'export', 'register']
}

/**
 * Returns the stage bar of a run: stages before the current one are done, the current one is active, stopped
 * or failed, later ones are pending; a queued run has no stage started.
 */
export function stageSegments(
  run: Pick<PostTrainingRun, 'status' | 'stage' | 'failedStage' | 'convertWorkloadId'>,
): StageSegment[] {
  const order = stageOrder(run)
  const current = run.stage === 'done' ? order.length : order.indexOf(run.stage)
  const failedIndex = order.indexOf((run.failedStage || '') as RunStage)
  const failedAt = failedIndex >= 0 ? failedIndex : current
  return order.map((stage, i) => {
    let state: SegmentState = 'pending'
    if (run.status === 'Failed') {
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

/** How a full run's freeze choice relates to the spec's freezing: its default patterns, others, or none. */
export type FreezeState = 'default' | 'custom' | 'off'

/** Returns the freezing of a choice: an omitted one takes the spec's default and an empty one trains all. */
export function freezeState(choice: Choices['freeze'], options?: FreezeOptions): FreezeState {
  if (!choice) return 'default'
  const patterns = choice.trainOnly ?? []
  if (!patterns.length) return 'off'
  const def = options?.default ?? []
  return patterns.length === def.length && patterns.every((p, i) => p === def[i])
    ? 'default'
    : 'custom'
}

/** Returns how many parameters a full run trains: ≈ 95.91B of 744B, custom patterns, or all of them. */
export function trainableText(state: FreezeState, options: FreezeOptions): string {
  if (state === 'custom') return 'Custom patterns'
  if (state === 'off') return options.totalB ? `All ${options.totalB}B` : 'All parameters'
  return options.trainableB && options.totalB
    ? `≈ ${options.trainableB}B of ${options.totalB}B`
    : 'Spec default'
}

/** Returns the review line of a full run's freezing, e.g. "Experts in layers 0–69 frozen · 95.91B trained". */
export function freezingText(state: FreezeState, options: FreezeOptions): string {
  if (state === 'off') return 'Nothing frozen'
  if (state === 'custom') return 'Custom patterns to train'
  const label = options.label
    ? options.label[0].toUpperCase() + options.label.slice(1)
    : 'Spec default'
  return options.trainableB ? `${label} frozen · ${options.trainableB}B trained` : `${label} frozen`
}

/** Returns why the patterns JavaScript cannot compile are invalid; the check stage decides with Python's re. */
export function regexErrors(patterns: string[]): string[] {
  const out: string[] = []
  patterns.forEach((p, i) => {
    try {
      new RegExp(p)
    } catch (e) {
      out.push(`Line ${i + 1} is not a valid regex: ${(e as Error).message}`)
    }
  })
  return out
}
