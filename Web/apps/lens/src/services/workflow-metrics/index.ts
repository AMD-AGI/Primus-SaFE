import request from '../request'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// ========== Helper ==========

// Get current cluster and append to params
const withCluster = <T extends Record<string, any>>(params?: T): T & { cluster?: string } => {
  const { selectedCluster } = useGlobalCluster()
  const cluster = selectedCluster.value
  return {
    ...params,
    ...(cluster ? { cluster } : {}),
  } as T & { cluster?: string }
}

// ========== Types ==========

export interface DisplaySettings {
  defaultChartGroupMode?: 'none' | 'dimension' | 'metric'  // Chart grouping mode
  defaultChartGroupBy?: string      // Default dimension for chart grouping (when mode is 'dimension')
  showRawDataByDefault?: boolean    // Whether to expand raw data table by default
  defaultChartType?: 'line' | 'bar' // Default chart type
}

export interface WorkflowConfig {
  id: number
  name: string
  description?: string
  runnerSetNamespace: string
  runnerSetName: string
  runnerSetUid?: string
  githubOwner: string
  githubRepo: string
  workflowFilter?: string
  branchFilter?: string
  filePatterns: string  // Base64 encoded JSON array
  decodedFilePatterns?: string[]  // Decoded patterns (added by frontend)
  metricSchemaId?: number
  enabled: boolean
  clusterName: string
  displaySettings?: DisplaySettings  // Display customization settings
  lastCheckedAt?: string
  createdAt: string
  updatedAt: string
}

export interface CreateConfigRequest {
  name: string
  description?: string
  runnerSetNamespace: string
  runnerSetName: string
  githubOwner: string
  githubRepo: string
  workflowFilter?: string
  branchFilter?: string
  filePatterns: string[]
  enabled?: boolean
  displaySettings?: DisplaySettings
}

export interface WorkflowRun {
  id: number
  configId: number
  configName?: string
  workloadUid: string
  workloadName: string
  workloadNamespace: string
  githubRunId?: number
  githubRunNumber?: number
  headSha?: string
  headBranch?: string
  workflowName?: string
  // Legacy status field (for backward compatibility)
  status: 'pending' | 'collecting' | 'extracting' | 'completed' | 'failed' |
          'workload_pending' | 'workload_running' | 'collection_pending' | 'collection_running'
  // Workflow execution status (from GitHub)
  workflowStatus?: 'queued' | 'in_progress' | 'completed' | 'waiting' | 'pending' | 'requested'
  // Workflow execution conclusion (from GitHub)
  workflowConclusion?: 'success' | 'failure' | 'cancelled' | 'skipped' | 'neutral' | 'timed_out' | 'action_required'
  // Collection status (our internal status)
  collectionStatus?: 'pending' | 'collecting' | 'completed' | 'failed' | 'skipped'
  // Progress tracking
  currentJobName?: string
  currentStepName?: string
  progressPercent?: number
  lastSyncedAt?: string
  triggerSource: 'realtime' | 'backfill' | 'manual'
  filesFound: number
  filesProcessed: number
  metricsCount: number
  workloadStartedAt?: string
  workloadCompletedAt?: string
  collectionStartedAt?: string
  collectionCompletedAt?: string
  errorMessage?: string
  retryCount: number
  createdAt: string
  updatedAt: string
}

export interface SchemaField {
  name: string
  type: string
  unit?: string
  description?: string
}

export interface WorkflowSchema {
  id: number
  configId: number
  name: string
  version: number
  schemaHash: string
  fields: SchemaField[]
  dimensionFields: string[]
  metricFields: string[]
  isWideTable: boolean
  dateColumns: string[]
  isActive: boolean
  generatedBy: 'ai' | 'user' | 'system'
  recordCount: number
  firstSeenAt: string
  lastSeenAt: string
  createdAt: string
}

export interface SchemaChange {
  fromVersion: number
  toVersion: number
  changedAt: string
  addedDimensions: string[]
  removedDimensions: string[]
  addedMetrics: string[]
  removedMetrics: string[]
  renamedMetrics?: { from: string; to: string }[]
}

export interface SchemasResponse {
  schemas: WorkflowSchema[]
  currentVersion: number
  total: number
}

export interface MetricRecord {
  id: number
  runId: number
  schemaId: number
  dimensions: Record<string, any>
  metrics: Record<string, number>
  rawData?: Record<string, any>
  sourceFile: string
  collectedAt: string
}

export interface ConfigStats {
  configId: number
  configName: string
  enabled: boolean
  pendingRuns: number
  completedRuns: number
  failedRuns: number
  totalMetrics: number
  activeSchemaId: number
  lastCheckedAt?: string
}

export interface BackfillTask {
  id: string
  configId: number
  status: 'pending' | 'in_progress' | 'completed' | 'cancelled' | 'failed'
  total: number
  processed: number
  failed: number
  createdAt: string
  startedAt?: string
  completedAt?: string
  errorMessage?: string
}

// ========== Config APIs ==========

export const getConfigs = (params: {
  offset?: number
  limit?: number
  name?: string
  enabled?: boolean
  githubOwner?: string
  githubRepo?: string
}): Promise<{ configs: WorkflowConfig[]; total: number }> =>
  request.get('/github-workflow-metrics/configs', { params: withCluster(params) })

export const getConfig = (id: number): Promise<WorkflowConfig> =>
  request.get(`/github-workflow-metrics/configs/${id}`, { params: withCluster() })

export const createConfig = (data: CreateConfigRequest): Promise<{ configId: number }> =>
  request.post('/github-workflow-metrics/configs', data, { params: withCluster() })

export const updateConfig = (id: number, data: Partial<CreateConfigRequest>): Promise<WorkflowConfig> =>
  request.put(`/github-workflow-metrics/configs/${id}`, data, { params: withCluster() })

export const deleteConfig = (id: number): Promise<{ deleted: boolean }> =>
  request.delete(`/github-workflow-metrics/configs/${id}`, { params: withCluster() })

export const getConfigStats = (id: number): Promise<ConfigStats> =>
  request.get(`/github-workflow-metrics/configs/${id}/stats`, { params: withCluster() })

// ========== Run APIs ==========

export const getRunsByConfig = (configId: number, params: {
  offset?: number
  limit?: number
  status?: string
  triggerSource?: string
}): Promise<{ runs: WorkflowRun[]; total: number }> =>
  request.get(`/github-workflow-metrics/configs/${configId}/runs`, { params: withCluster(params) })

export const getRun = (id: number): Promise<WorkflowRun> =>
  request.get(`/github-workflow-metrics/runs/${id}`, { params: withCluster() })

export const getRunMetrics = (runId: number, params?: {
  offset?: number
  limit?: number
}): Promise<{ metrics: MetricRecord[]; total: number }> =>
  request.get(`/github-workflow-metrics/runs/${runId}/metrics`, { params: withCluster(params) })

// ========== Schema APIs ==========

export const getSchemasByConfig = (configId: number): Promise<SchemasResponse> =>
  request.get(`/github-workflow-metrics/configs/${configId}/schemas`, { params: withCluster() })

export const getSchema = (id: number): Promise<WorkflowSchema> =>
  request.get(`/github-workflow-metrics/schemas/${id}`, { params: withCluster() })

export const activateSchema = (id: number): Promise<{ activated: boolean }> =>
  request.post(`/github-workflow-metrics/schemas/${id}/activate`, null, { params: withCluster() })

export const regenerateSchema = (configId: number, data?: {
  sampleFiles?: { path: string; name: string; fileType: string; content: string }[]
  customPrompt?: string
}): Promise<{ schemaId: number; version: number; name: string; fields: SchemaField[] }> =>
  request.post(`/github-workflow-metrics/configs/${configId}/schemas/regenerate`, data, { params: withCluster() })

export const getSchemaChanges = (configId: number): Promise<{ changes: SchemaChange[] }> =>
  request.get(`/github-workflow-metrics/configs/${configId}/schemas/changes`, { params: withCluster() })

// ========== Metrics Query APIs ==========

export interface MetricsQuery {
  start?: string
  end?: string
  schemaId?: number  // schema_id for filtering by schema
  dimensions?: Record<string, any>
  metricFilters?: Record<string, any>
  sortBy?: string
  sortOrder?: string
  offset?: number
  limit?: number
}

export interface AggregationQuery {
  start?: string
  end?: string
  dimensions?: Record<string, any>
  groupBy?: string[]
  metricField: string
  aggFunc?: 'avg' | 'sum' | 'min' | 'max' | 'count'
  interval?: string
}

export interface TrendsQuery {
  start?: string
  end?: string
  schemaId?: number  // schema_id for filtering by schema
  dimensions?: Record<string, any>
  metricFields: string[]
  interval?: string
  groupBy?: string[]
}

export const queryMetrics = (configId: number, params: MetricsQuery): Promise<{
  metrics: MetricRecord[]
  total: number
}> => request.post(`/github-workflow-metrics/configs/${configId}/metrics/query`, params, { params: withCluster() })

export const aggregateMetrics = (configId: number, params: AggregationQuery): Promise<{
  results: any[]
  metricField: string
  aggFunc: string
  interval: string
  groupBy: string[]
}> => request.post(`/github-workflow-metrics/configs/${configId}/metrics/aggregate`, params, { params: withCluster() })

export interface TrendsSeries {
  name: string
  field: string
  dimensions?: Record<string, any>
  values: number[]
  counts?: number[]
}

export interface TrendsResponse {
  timestamps: string[]
  series: TrendsSeries[]
  interval: string
}

export const getMetricsTrends = (configId: number, params: TrendsQuery): Promise<TrendsResponse> =>
  request.post(`/github-workflow-metrics/configs/${configId}/metrics/trends`, params, { params: withCluster() })

export const getMetricsSummary = (configId: number, params?: {
  start?: string
  end?: string
}): Promise<any> => request.get(`/github-workflow-metrics/configs/${configId}/summary`, { params: withCluster(params) })

export const getDimensions = (configId: number, params?: {
  start?: string
  end?: string
  schemaId?: number  // schema_id for filtering by schema
}): Promise<{
  dimensions: Record<string, string[]>  // API returns { dimensions: { "Framework": [...], "GPU": [...] } }
  values?: Record<string, string[]>     // Legacy field name (for backward compatibility)
  schemaId?: number
  availableSchemaIds?: number[]
}> =>
  request.get(`/github-workflow-metrics/configs/${configId}/dimensions`, { params: withCluster(params) })

export const getMetricFields = (configId: number, params?: {
  schemaId?: number  // schema_id for filtering by schema
}): Promise<{
  dimensionFields: string[]
  metricFields: string[]
  schemaId?: number
  availableSchemaIds?: number[]
}> => request.get(`/github-workflow-metrics/configs/${configId}/fields`, { params: withCluster(params) })

// ========== Backfill APIs ==========

export const triggerBackfill = (configId: number, data: {
  startTime: string
  endTime: string
  workloadUids?: string[]
  dryRun?: boolean
}): Promise<{ taskId: string; configId: number; status: string }> =>
  request.post(`/github-workflow-metrics/configs/${configId}/backfill`, data, { params: withCluster() })

export const getBackfillStatus = (configId: number): Promise<BackfillTask> =>
  request.get(`/github-workflow-metrics/configs/${configId}/backfill/status`, { params: withCluster() })

export const cancelBackfill = (configId: number): Promise<{ cancelled: number }> =>
  request.post(`/github-workflow-metrics/configs/${configId}/backfill/cancel`, null, { params: withCluster() })

export const getBackfillTasks = (configId: number): Promise<{ tasks: BackfillTask[]; total: number }> =>
  request.get(`/github-workflow-metrics/configs/${configId}/backfill/tasks`, { params: withCluster() })

export const retryFailedRuns = (configId: number): Promise<{ retried: number; total: number }> =>
  request.post(`/github-workflow-metrics/configs/${configId}/runs/batch-retry`, null, { params: withCluster() })

// ========== Runner Sets APIs ==========

export interface RunnerSet {
  id: number
  uid: string
  name: string
  namespace: string
  githubConfigUrl?: string
  githubConfigSecret?: string
  runnerGroup?: string
  githubOwner?: string
  githubRepo?: string
  minRunners: number
  maxRunners: number
  status: 'active' | 'inactive' | 'deleted'
  currentRunners: number
  desiredRunners: number
  lastSyncAt?: string
  createdAt: string
  updatedAt: string
  // Joined fields
  config?: WorkflowConfig
}

// Runner Set with statistics (returned by ?with_stats=true)
export interface RunnerSetWithStats extends RunnerSet {
  totalRuns: number
  pendingRuns: number
  completedRuns: number
  failedRuns: number
  hasConfig: boolean
  configId?: number
  configName?: string
}

// Runner Set statistics
export interface RunnerSetStats {
  total: number
  pending: number
  completed: number
  failed: number
  collecting: number
  metricsCollected: number
  hasConfig: boolean
  configId?: number
  configName?: string
}

export const getRunnerSets = (params?: {
  namespace?: string
  with_stats?: boolean
}): Promise<{ runnerSets: RunnerSet[] | RunnerSetWithStats[] }> =>
  request.get('/github-runners/runner-sets', { params: withCluster(params) })

export const getRunnerSet = (namespace: string, name: string): Promise<RunnerSet> =>
  request.get(`/github-runners/runner-sets/${namespace}/${name}`, { params: withCluster() })

// ========== Runner Set Centric APIs (New) ==========

/**
 * Get runner set by ID
 */
export const getRunnerSetById = (id: number): Promise<RunnerSet> =>
  request.get(`/github-runners/runner-sets/by-id/${id}`, { params: withCluster() })

/**
 * Get runs for a runner set by ID (no config required)
 */
export const getRunsByRunnerSetId = (runnerSetId: number, params?: {
  offset?: number
  limit?: number
  status?: string
  trigger_source?: string
  start_date?: string
  end_date?: string
}): Promise<{ runs: WorkflowRun[]; total: number }> =>
  request.get(`/github-runners/runner-sets/by-id/${runnerSetId}/runs`, { params: withCluster(params) })

/**
 * Get config associated with a runner set (may return null if no config)
 */
export const getConfigByRunnerSetId = (runnerSetId: number): Promise<WorkflowConfig | null> =>
  request.get(`/github-runners/runner-sets/by-id/${runnerSetId}/config`, { params: withCluster() })
    .catch(err => {
      // Return null if 404 (no config found)
      if (err?.response?.status === 404) return null
      throw err
    })

/**
 * Get statistics for a runner set
 */
export const getStatsByRunnerSetId = (runnerSetId: number): Promise<RunnerSetStats> =>
  request.get(`/github-runners/runner-sets/by-id/${runnerSetId}/stats`, { params: withCluster() })

/**
 * Create config for a runner set
 */
export const createConfigForRunnerSet = (runnerSetId: number, data: {
  name: string
  description?: string
  filePatterns: string[]
  workflowFilter?: string
  branchFilter?: string
  enabled?: boolean
  displaySettings?: DisplaySettings
}): Promise<{ configId: number }> =>
  request.post(`/github-runners/runner-sets/by-id/${runnerSetId}/config`, data, { params: withCluster() })

/**
 * Trigger backfill for a runner set
 */
export const triggerBackfillByRunnerSetId = (runnerSetId: number, data: {
  startTime: string
  endTime: string
  workloadUids?: string[]
  dryRun?: boolean
}): Promise<{ taskId: string; runnerSetId: number; status: string }> =>
  request.post(`/github-runners/runner-sets/by-id/${runnerSetId}/backfill`, data, { params: withCluster() })

// Get runs for a runner set (by matching runner_set_name in config) - deprecated, use getRunsByRunnerSetId
/** @deprecated Use getRunsByRunnerSetId instead */
export const getRunsByRunnerSet = (runnerSetName: string, params?: {
  offset?: number
  limit?: number
  status?: string
}): Promise<{ runs: WorkflowRun[]; total: number }> =>
  request.get('/github-workflow-metrics/runs', { params: withCluster({ ...params, runner_set: runnerSetName }) })

// Get all runs globally
export const getAllRuns = (params?: {
  offset?: number
  limit?: number
  status?: string
  configId?: number
  runner_set_id?: number
  no_config?: boolean
}): Promise<{ runs: WorkflowRun[]; total: number }> =>
  request.get('/github-workflow-metrics/runs', { params: withCluster(params) })

// ========== Run Live State APIs (Real-time Workflow Sync) ==========

export interface WorkflowStep {
  number: number
  name: string
  status: 'queued' | 'in_progress' | 'completed' | 'waiting' | 'pending'
  conclusion?: 'success' | 'failure' | 'cancelled' | 'skipped' | 'neutral' | 'timed_out' | 'action_required'
  startedAt?: string
  completedAt?: string
  durationSeconds?: number
}

export interface WorkflowJob {
  id: number
  githubJobId: number
  name: string
  status: 'queued' | 'in_progress' | 'completed' | 'waiting' | 'pending'
  conclusion?: 'success' | 'failure' | 'cancelled' | 'skipped' | 'neutral' | 'timed_out' | 'action_required'
  startedAt?: string
  completedAt?: string
  durationSeconds?: number
  runnerName?: string
  currentStepNumber?: number
  currentStepName?: string
  steps?: WorkflowStep[]
}

export interface WorkflowLiveState {
  runId: number
  githubRunId: number
  workflowName: string
  headSha?: string
  headBranch?: string
  workflowStatus: 'queued' | 'in_progress' | 'completed' | 'waiting' | 'pending' | 'requested'
  workflowConclusion?: 'success' | 'failure' | 'cancelled' | 'skipped' | 'neutral' | 'timed_out' | 'action_required'
  collectionStatus?: string
  currentJobName?: string
  currentStepName?: string
  progressPercent: number
  elapsedSeconds?: number
  startedAt?: string
  lastSyncedAt?: string
  updatedAt?: string
  jobs?: WorkflowJob[]
}

/**
 * Get current workflow run state (non-streaming)
 */
export const getRunLiveState = (runId: number): Promise<WorkflowLiveState> =>
  request.get(`/github-workflow-metrics/runs/${runId}/state`, { params: withCluster() })

/**
 * Start sync for a workflow run
 */
export const startRunSync = (runId: number): Promise<{ status: string; runId: number }> =>
  request.post(`/github-workflow-metrics/runs/${runId}/sync/start`, null, { params: withCluster() })

/**
 * Stop sync for a workflow run
 */
export const stopRunSync = (runId: number): Promise<{ status: string; runId: number }> =>
  request.post(`/github-workflow-metrics/runs/${runId}/sync/stop`, null, { params: withCluster() })

/**
 * Get jobs for a workflow run
 */
export const getRunJobs = (runId: number): Promise<{ jobs: WorkflowJob[]; total: number }> =>
  request.get(`/github-workflow-metrics/runs/${runId}/jobs`, { params: withCluster() })

/**
 * Job logs response
 */
export interface JobLogsResponse {
  run_id: number
  job_id: number
  logs: string
  source: 'cache' | 'github'
  fetched_at?: string
}

/**
 * Step logs response
 */
export interface StepLogsResponse {
  run_id: number
  job_id: number
  step_number: number
  logs: string
  source: 'cache' | 'github'
  fetched_at?: string
}

/**
 * Get logs for a specific job
 */
export const getJobLogs = (runId: number, jobId: number): Promise<JobLogsResponse> =>
  request.get(`/github-workflow-metrics/runs/${runId}/jobs/${jobId}/logs`, { params: withCluster() })

/**
 * Get logs for a specific step within a job
 */
export const getStepLogs = (runId: number, jobId: number, stepNumber: number): Promise<StepLogsResponse> =>
  request.get(`/github-workflow-metrics/runs/${runId}/jobs/${jobId}/steps/${stepNumber}/logs`, { params: withCluster() })

/**
 * Create SSE connection for live workflow updates
 */
export const createRunLiveStream = (runId: number, cluster?: string): EventSource => {
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
  const params = new URLSearchParams()
  if (cluster) params.set('cluster', cluster)
  return new EventSource(`${baseUrl}/v1/github-workflow-metrics/runs/${runId}/live?${params.toString()}`)
}

// ========== Run Detail with Extended Info ==========

export interface RunDetailExtended extends WorkflowRun {
  runnerSetId?: number
  runnerSetName?: string
  runnerSetNamespace?: string
  currentJobName?: string
  currentStepName?: string
  progressPercent?: number
  conclusion?: string
  lastSyncedAt?: string
  jobs?: WorkflowJob[]
}

/**
 * Get run with extended details including jobs
 */
export const getRunDetail = async (runId: number): Promise<RunDetailExtended> => {
  const [run, jobsRes] = await Promise.all([
    getRun(runId),
    getRunJobs(runId).catch(() => ({ jobs: [], total: 0 }))
  ])
  return {
    ...run,
    jobs: jobsRes.jobs
  }
}

/**
 * Get running runs for a runner set (for banner display)
 */
export const getRunningRunsByRunnerSetId = (runnerSetId: number): Promise<{ runs: WorkflowRun[] }> =>
  request.get(`/github-runners/runner-sets/by-id/${runnerSetId}/runs`, {
    params: withCluster({ status: 'workload_running', limit: 10 })
  })

// ========== Run Summary APIs (Run-level aggregation) ==========

/**
 * Workflow Run Summary - aggregates multiple jobs into one run view
 */
export interface WorkflowRunSummary {
  id: number
  githubRunId: number
  githubRunNumber: number
  githubRunAttempt: number
  owner: string
  repo: string
  workflowName?: string
  workflowPath?: string
  workflowId?: number
  headSha?: string
  headBranch?: string
  baseBranch?: string
  eventName?: string
  actor?: string
  triggeringActor?: string
  status: 'queued' | 'in_progress' | 'completed' | 'waiting' | 'requested'
  conclusion?: 'success' | 'failure' | 'cancelled' | 'skipped' | 'timed_out' | 'action_required' | 'neutral'
  runStartedAt?: string
  runCompletedAt?: string
  // Job aggregation stats
  totalJobs: number
  completedJobs: number
  successfulJobs: number
  failedJobs: number
  cancelledJobs: number
  skippedJobs: number
  inProgressJobs: number
  queuedJobs: number
  // Progress tracking
  currentJobName?: string
  currentStepName?: string
  progressPercent: number
  // Collection stats
  totalFilesProcessed: number
  totalMetricsCount: number
  collectionStatus?: 'pending' | 'partial' | 'completed' | 'failed'
  // Config association
  primaryRunnerSetId?: number
  configId?: number
  // Sync metadata
  lastSyncedAt?: string
  syncErrorMessage?: string
  // Graph and analysis flags
  graphFetched: boolean
  graphFetchedAt?: string
  codeAnalysisTriggered: boolean
  codeAnalysisTriggeredAt?: string
  failureAnalysisTriggered: boolean
  failureAnalysisTriggeredAt?: string
  // Timestamps
  createdAt: string
  updatedAt: string
}

/**
 * Run Summary filter parameters
 */
export interface RunSummaryFilter {
  status?: string
  conclusion?: string
  collectionStatus?: string
  workflowPath?: string
  headBranch?: string
  eventName?: string
  runnerSetId?: number
  offset?: number
  limit?: number
}

/**
 * Get run summaries for a repository
 */
export const getRunSummaries = (owner: string, repo: string, params?: RunSummaryFilter): Promise<{
  runSummaries: WorkflowRunSummary[]
  total: number
}> => request.get(`/github-runners/repositories/${owner}/${repo}/run-summaries`, { params: withCluster(params) })

/**
 * Get a single run summary by ID
 */
export const getRunSummary = (id: number): Promise<WorkflowRunSummary> =>
  request.get(`/github-runners/run-summaries/${id}`, { params: withCluster() })

/**
 * Get jobs for a run summary
 */
export const getRunSummaryJobs = (id: number): Promise<{ jobs: WorkflowRun[]; total: number }> =>
  request.get(`/github-runners/run-summaries/${id}/jobs`, { params: withCluster() })

/**
 * GitHub Job Node for DAG visualization
 */
export interface GithubJobNode {
  id: number
  githubJobId: number
  name: string
  status: 'queued' | 'in_progress' | 'completed' | 'waiting' | 'pending'
  conclusion?: 'success' | 'failure' | 'cancelled' | 'skipped' | 'neutral' | 'timed_out' | 'action_required'
  needs?: string[]
  startedAt?: string
  completedAt?: string
  durationSeconds: number
  stepsCount: number
  stepsCompleted: number
  stepsFailed: number
  htmlUrl?: string
}

/**
 * Get workflow DAG graph for visualization
 */
export const getRunSummaryGraph = (id: number): Promise<{ jobs: GithubJobNode[]; total: number }> =>
  request.get(`/github-runners/run-summaries/${id}/graph`, { params: withCluster() })

// ========== Repository APIs ==========

/**
 * Repository summary with aggregated statistics
 */
export interface RepositorySummary {
  owner: string
  repo: string
  runnerSetCount: number
  totalRunners: number
  maxRunners: number
  totalRuns: number
  runningWorkflows: number
  pendingRuns: number
  completedRuns: number
  failedRuns: number
  lastRunAt?: string
  configuredSets: number
}

/**
 * Config metrics info for a repository
 */
export interface ConfigMetricsInfo {
  configId: number
  configName: string
  runnerSetId: number
  runnerSetName: string
  schemaId?: number
  schemaVersion?: number
  dimensionFields: string[]
  metricFields: string[]
  recordCount: number
}

/**
 * Repository metrics metadata response
 */
export interface RepositoryMetricsMetadata {
  owner: string
  repo: string
  configs: ConfigMetricsInfo[]
  commonDimensions: string[]
  commonMetrics: string[]
  allDimensions: string[]
  allMetrics: string[]
}

/**
 * Repository metrics trends query
 */
export interface RepositoryMetricsTrendsQuery {
  start?: string
  end?: string
  configIds?: number[]
  dimensions?: Record<string, any>
  metricFields: string[]
  interval?: string
  groupBy?: string[]
  aggregateAcrossConfigs?: boolean
}

/**
 * List all repositories with aggregated statistics
 */
export const getRepositories = (): Promise<{ repositories: RepositorySummary[] }> =>
  request.get('/github-runners/repositories', { params: withCluster() })

/**
 * Get repository details by owner and repo
 */
export const getRepository = (owner: string, repo: string): Promise<RepositorySummary> =>
  request.get(`/github-runners/repositories/${owner}/${repo}`, { params: withCluster() })

/**
 * Get runner sets for a repository
 */
export const getRepositoryRunnerSets = (owner: string, repo: string, withStats?: boolean): Promise<{ runnerSets: RunnerSetWithStats[] }> =>
  request.get(`/github-runners/repositories/${owner}/${repo}/runner-sets`, {
    params: withCluster({ with_stats: withStats ? 'true' : undefined })
  })

/**
 * Get metrics metadata for a repository
 */
export const getRepositoryMetricsMetadata = (owner: string, repo: string): Promise<RepositoryMetricsMetadata> =>
  request.get(`/github-runners/repositories/${owner}/${repo}/metrics/metadata`, { params: withCluster() })

/**
 * Query metrics trends for a repository
 */
export const getRepositoryMetricsTrends = (owner: string, repo: string, query: RepositoryMetricsTrendsQuery): Promise<TrendsResponse> =>
  request.post(`/github-runners/repositories/${owner}/${repo}/metrics/trends`, query, { params: withCluster() })
