import request from '@/services/request'
import type {
  ModelsResp,
  NullTime,
  PostTrainingRun,
  RunListParams,
  RunListResp,
  RunStage,
  SegmentState,
  StageSegment,
} from './types'

export * from './types'

/** Lists the models of a workspace with their Post Training support; a 404 means Post Training is off. */
export function getPostTrainingModels(workspace: string) {
  return request.get<ModelsResp>('/post-training/models', {
    params: { workspace },
    skipErrorHandler: true,
  })
}

/** Lists the runs of a workspace, newest first. */
export function listPostTrainingRuns(params: RunListParams, quiet = false) {
  return request.get<RunListResp>('/post-training/runs', { params, skipErrorHandler: quiet })
}

/** Returns a run with its recipe, stage history, output model and evaluations. */
export function getPostTrainingRun(runId: string) {
  return request.get<PostTrainingRun>(`/post-training/runs/${runId}`)
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
