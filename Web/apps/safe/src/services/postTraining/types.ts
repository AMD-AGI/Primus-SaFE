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
  /** URLs the launcher reported, such as the W&B run, by name. */
  links?: Record<string, string>
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
  /** How many runs of this recipe succeeded on the platform, and the latest of them; list items only. */
  verifiedRuns?: number
  latestVerifiedRun?: string
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

/** The LoRA options of a run. */
export interface LoraChoice {
  rank?: number
  alpha?: number
  dropout?: number
  targets?: string[]
  experts?: boolean
  expertMode?: string
  adapterOnlyCheckpoint?: boolean
}

/** The training options of a run; zero values take the framework's defaults. */
export interface Choices {
  fieldMap?: Record<string, string>
  numEpoch?: number
  numRollout?: number
  batchSize?: number
  maxTokensPerGpu?: number
  lr?: number
  minLr?: number
  warmupIters?: number
  saveInterval?: number
  valInterval?: number
  valMaxSamples?: number
  valSplit?: number
  tp?: number
  pp?: number
  cp?: number
  ep?: number
  sequenceParallel?: boolean
  lora?: LoraChoice
  freeze?: { trainOnly?: string[] }
  evalInterval?: number
  advantageEstimator?: string
  rewardType?: string
  rewardHook?: string
  rewardRemote?: { url?: string; secretId?: string; rewardKey?: string }
  generateHook?: string
  nSamplesPerPrompt?: number
  maxPromptLen?: number
  maxResponseLen?: number
  layout?: string
  rolloutGpus?: number
  gpusPerEngine?: number
  useKlLoss?: boolean
  klLossCoef?: number
  correction?: string
  rolloutPrecision?: string
  kvCacheDtype?: string
  schedule?: string
  concurrency?: number
  pd?: { enabled?: boolean; prefillGpus?: number }
  criticLr?: number
  criticWarmupRollouts?: number
}

/** The body of the preview and submit APIs. */
export interface SubmitRequest {
  workspace: string
  displayName: string
  trainType: 'sft' | 'rl'
  method: string
  framework?: string
  modelId: string
  datasetId: string
  evalDatasetId?: string
  preset?: string
  choices: Choices
  resources: {
    nodes: number
    gpusPerNode: number
    priority?: number
    timeoutSeconds?: number
    maxRetry?: number
    image?: string
  }
  export: { hf?: boolean; peftAdapter?: boolean }
  tracking: { wandb: { enabled?: boolean; secretId?: string; project?: string; team?: string } }
  evaluation: { enabled?: boolean; benchmarks?: string[]; keepService?: boolean }
  /** The template the request started from; the run records it with the template's version. */
  templateId?: string
}

/** One allowed value of a wizard field, with why it is disabled. */
export interface OptionValue {
  value: string
  label: string
  disabled?: boolean
  reason?: string
}

/** The wizard's choices for one framework: defaults, presets and per-field values. */
export interface RecipeOptions {
  defaults: Choices
  presets?: { id: string; label: string; trainType: string; method: string; choices: Choices }[]
  fields?: Record<string, { values?: OptionValue[]; disabled?: boolean; reason?: string }>
}

/** GET /post-training/config: the frameworks and the wizard options of one model. */
export interface ConfigResp {
  frameworks: { name: string; default: boolean }[]
  framework: string
  spec?: string
  supported: boolean
  reason?: string
  options?: RecipeOptions
}

/** One compiler finding: an error blocks submission, a warning is confirmed at the last step. */
export interface Diagnostic {
  level: 'error' | 'warning'
  field?: string
  message: string
}

/** POST /post-training/recipes/preview. */
export interface PreviewResp {
  diagnostics: Diagnostic[]
  layout?: {
    nodes: number
    gpusPerNode: number
    colocate: boolean
    actorGpus: number
    rolloutGpus: number
  }
  frameworkArgs?: string[]
  recipe?: Record<string, unknown>
  /** How many runs of the compiled recipe succeeded on the platform, and the latest of them. */
  verifiedRuns: number
  latestVerifiedRun?: string
}

/** POST /post-training/runs. */
export interface SubmitResp {
  runId: string
  workloadId: string
  warnings?: Diagnostic[]
}

/** A nullable text column, as the API encodes evaluation tasks. */
export interface NullString {
  String: string
  Valid: boolean
}

/** An evaluation task of a run; the API returns the database row with its Go field names. */
export interface EvaluationTaskRow {
  TaskId: string
  TaskName: string
  ServiceId: string
  Status: string
  Benchmarks: string
  ResultSummary?: NullString
  ReportS3Path?: NullString
  OpsJobId?: NullString
  ModelId?: NullString
  CreationTime?: NullTime
  EndTime?: NullTime
}

/** One exported checkpoint of a run: its latest export job and, once registered, its model. */
export interface ExportItem {
  step: number
  workloadId?: string
  status?: string
  message?: string
  modelId?: string
  displayName?: string
  modelPhase?: string
  createdAt?: string
}

/** GET /post-training/runs/:id: the run with its parsed recipe, history, summary, model and evaluations. */
export interface RunDetail extends Omit<PostTrainingRun, 'stageHistory' | 'metricSummary'> {
  recipe?: Record<string, unknown>
  stageHistory?: StageEntry[]
  metricSummary?: MetricSummary
  outputModel?: { id: string; displayName: string; phase: string }
  evaluations: EvaluationTaskRow[]
  exports?: ExportItem[]
}

/** One downsampled metric series: [step, value] points on the train, rollout or time axis. */
export interface MetricSeries {
  name: string
  axis: 'step' | 'rollout' | 'time'
  points: [number, number][]
  count: number
  last?: [number, number]
  downsampled?: boolean
  nonFinite?: [number, string][]
}

/** The checkpoints, best checkpoint, resume points and failure drawn on a run's curves. */
export interface MetricMarkers {
  checkpoints: { step: number; deleted?: boolean; valLoss?: number }[]
  bestCheckpoint?: { step: number; valLoss?: number } | null
  resumes: { step: number; time?: string }[]
  failure?: { step: number; reason?: string } | null
}

/** A chart of the metrics tab as the registry defines it; series, band and rest are name patterns. */
export interface MetricPanel {
  id: string
  series: string[]
  band?: string[]
  smooth?: boolean
  logY?: boolean
  stack?: boolean
  per?: string
  when?: string
  rest?: string
  axis?: string
}

export interface MetricsResp {
  series: MetricSeries[]
  markers: MetricMarkers
  /** The registry's panels for the run's framework and train type, the GPU panels, and the better directions. */
  panels?: MetricPanel[]
  system?: MetricPanel[]
  better?: Record<string, 'lower' | 'higher'>
}

/** The sizes in bytes of the run directory parts the launcher reports. */
export interface DiskUsage {
  checkpoints?: number
  criticCheckpoints?: number
  finalModel?: number
  finalAdapter?: number
  exports?: number
  total?: number
}

/** What a template stores of a request: the options and settings, and optionally the base model and datasets. */
export type TemplateContent = Partial<
  Pick<
    SubmitRequest,
    | 'framework'
    | 'trainType'
    | 'method'
    | 'preset'
    | 'choices'
    | 'resources'
    | 'export'
    | 'evaluation'
    | 'tracking'
    | 'modelId'
    | 'datasetId'
    | 'evalDatasetId'
  >
>

/** A recipe template of a workspace, with how many of its runs succeeded. */
export interface TemplateItem {
  templateId: string
  workspace: string
  name: string
  description?: string
  trainType: 'sft' | 'rl'
  method: string
  framework: string
  /** The spec the template was saved for; empty means any spec the model matches. */
  spec?: string
  version: number
  content: TemplateContent
  sourceRunId?: string
  userId: string
  userName: string
  createdAt?: NullTime
  updatedAt?: NullTime
  succeededRuns: number
}

export interface TemplateListResp {
  total: number
  items: TemplateItem[]
}

/** The body that creates a template, or the fields that edit one. */
export interface TemplateBody {
  workspace?: string
  name?: string
  description?: string
  trainType?: string
  method?: string
  framework?: string
  spec?: string
  content?: TemplateContent
  sourceRunId?: string
}

/** A variant of an A/B experiment: its name and the request fields it changes from the baseline. */
export interface ExperimentVariant {
  name: string
  changes?: Record<string, unknown>
}

/** An A/B experiment with its runs in variant order, the baseline first. */
export interface Experiment {
  experimentId: string
  workspace: string
  displayName: string
  trainType: 'sft' | 'rl'
  baselineVariant: string
  variants: ExperimentVariant[]
  seed: number
  userId: string
  userName: string
  createdAt?: NullTime
  runs: PostTrainingRun[]
}

export interface ExperimentListResp {
  total: number
  items: Experiment[]
}

/** POST /post-training/experiments: a baseline request and the variants that change it. */
export interface ExperimentRequest {
  displayName: string
  baseline: SubmitRequest
  variants: ExperimentVariant[]
}

/** A compile finding of one variant of a refused experiment. */
export interface VariantDiagnostic extends Diagnostic {
  variant: string
}
