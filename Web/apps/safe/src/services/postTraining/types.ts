export type RunStatus = 'Pending' | 'Running' | 'Succeeded' | 'Failed' | 'Stopped'

export type RunStage =
  | 'queued'
  | 'prepare'
  | 'convert'
  | 'check'
  | 'train'
  | 'export'
  | 'register'
  | 'eval'
  | 'done'

/** A nullable timestamp column, as the API encodes it. */
export interface NullTime {
  Time: string
  Valid: boolean
}

/** One record of a run's stage history. */
export interface StageEntry {
  stage: RunStage
  time: string
  attempt?: string
  resumeStep?: number
  event?: string
  message?: string
  keptStep?: number
}

export interface SummaryPoint {
  step: number
  value: number
}

/** The metric summary the list reads: the main metric with its sparkline and the best validation loss. */
export interface MetricSummary {
  points: number
  capped?: boolean
  rejected?: number
  main?: { name: string; step: number; value: number; spark?: SummaryPoint[] }
  bestValLoss?: SummaryPoint
}

/** A Post Training run as the run list returns it. JSON columns arrive as strings. */
export interface PostTrainingRun {
  runId: string
  workloadId: string
  workloadUid?: string
  displayName: string
  framework: string
  frameworkVersion?: string
  trainType: 'sft' | 'rl'
  method: string
  algorithm?: string
  workspace: string
  cluster: string
  userId: string
  userName: string
  baseModelId: string
  baseModelName: string
  datasetId: string
  datasetName?: string
  evalDatasetId?: string
  image?: string
  nodeCount: number
  gpuPerNode: number
  priority: number
  timeout: number
  outputPath?: string
  outputModelId?: string
  status: RunStatus
  stage: RunStage
  failedStage?: string
  message?: string
  queuePosition: number
  stageHistory?: string
  recipeHash: string
  evalEnabled: boolean
  metricSummary?: string
  metricGap: boolean
  stalledSince?: NullTime
  diskUsage?: string
  cleanupRequest?: string
  convertWorkloadId?: string
  templateId?: string
  experimentId?: string
  variantName?: string
  startTime?: NullTime
  endTime?: NullTime
  createdAt?: NullTime
}

export interface RunListParams {
  workspace: string
  trainType?: string
  method?: string
  status?: string
  baseModelId?: string
  owner?: string
  search?: string
  limit?: number
  offset?: number
}

export interface RunListResp {
  total: number
  items: PostTrainingRun[]
}

/** Whether one model can be trained with Post Training in a workspace. */
export interface ModelSupport {
  modelId: string
  displayName: string
  spec?: string
  supported: boolean
  reason?: string
}

/** GET /post-training/models: the models of a workspace, and why the workspace cannot train or submit now. */
export interface ModelsResp {
  items: ModelSupport[]
  unavailableReason?: string
  pausedReason?: string
}

export type SegmentState = 'done' | 'active' | 'failed' | 'skipped' | 'stopped' | 'pending'

/** One segment of the stage bar. */
export interface StageSegment {
  stage: RunStage
  label: string
  state: SegmentState
}
