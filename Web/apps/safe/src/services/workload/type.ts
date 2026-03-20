export enum WorkloadPhase {
  Succeeded = 'Succeeded',
  Failed = 'Failed',
  Pending = 'Pending',
  Running = 'Running',
  Stopped = 'Stopped',
}
export const phaseFilters = Object.values(WorkloadPhase).map((p) => ({
  text: p,
  value: p,
}))

type WorkloadTagType = 'success' | 'danger' | 'info' | 'primary'

export const WorkloadPhaseButtonType: Record<string, { type: WorkloadTagType }> = {
  Succeeded: {
    type: 'success',
  },
  Failed: {
    type: 'danger',
  },
  Pending: {
    type: 'info',
  },
  Running: {
    type: 'primary',
  },
  Stopped: {
    type: 'info',
  },
}

export enum WorkloadKind {
  Deployment = 'Deployment',
  StatefulSet = 'StatefulSet',

  AutoscalingRunnerSet = 'AutoscalingRunnerSet',
  EphemeralRunner = 'EphemeralRunner',
  UnifiedJob='UnifiedJob',

  PyTorchJob = 'PyTorchJob',
  Authoring = 'Authoring',
  TorchFT = 'TorchFT',
  RayJob = 'RayJob',
}
// kind -> base path
export const KindPathMap: Record<WorkloadKind, `/${string}`> = {
  [WorkloadKind.PyTorchJob]: '/training',
  [WorkloadKind.Authoring]: '/authoring',
  [WorkloadKind.Deployment]: '/infer',
  [WorkloadKind.StatefulSet]: '/infer',
  [WorkloadKind.AutoscalingRunnerSet]: '/cicd',
  [WorkloadKind.EphemeralRunner]: '/cicd',
  [WorkloadKind.UnifiedJob]: '/cicd',
  [WorkloadKind.TorchFT]: '/torchft',
  [WorkloadKind.RayJob]: '/rayjob',
} as const

export type PriorityValue = 0 | 1 | 2

export const PRIORITY_LABEL_MAP: Record<PriorityValue, 'Low' | 'Medium' | 'High'> = {
  0: 'Low',
  1: 'Medium',
  2: 'High',
}

export interface WorkloadParams {
  workspaceId?: string
  clusterId?: string
  kind?: WorkloadKind | string

  phase?: WorkloadPhase | string | string[]
  userName?: string
  description?: string

  offset?: number
  limit?: number
  sortBy?: string
  order?: 'asc' | 'desc'

  since?: string
  until?: string

  workloadId?: string
  userId?: string
  scaleRunnerSet?: string
  scaleRunnerId?: string
}

export interface GetWorkloadPodLogResponse {
  workloadId: string
  podId: string
  namespace: string
  logs: string[]
}
// Edit + Create
export interface EditWorkloadRequest {
  description?: string
  entryPoint?: string
  entryPoints?: string[]
  image?: string
  images?: string[]
  priority?: number
  env?: Record<string, string>
  resources?: Array<{
    replica?: number
    cpu: string
    gpu: string
    memory: string
    ephemeralStorage: string
  }>
  maxRetry?: number
  excludedNodes?: string[]
  privileged?: boolean
}

export interface SubmitWorkloadRequest {
  workspace: string
  displayName: string
  groupVersionKind: {
    kind: string
    version: string
  }
  description?: string
  entryPoint?: string
  entryPoints?: string[]
  isSupervised?: boolean
  image?: string
  images?: string[]
  maxRetry?: number
  priority?: number
  resources?: Array<{
    replica?: number
    cpu: string
    gpu: string
    memory: string
    ephemeralStorage: string
  }>
  specifiedNodes?: string[]
  env?: Record<string, string>
  customerLabels?: Record<string, string>
  isTolerateAll?: boolean
  workloadId?: string
  dependencies?: string[]
  excludedNodes?: string[]
  stickyNodes?: boolean | string[]
  stickyNodesMode?: 'required' | 'preferred'
  privileged?: boolean
  useWorkspaceStorage?: boolean
}

// workload-detail-log
export interface GetLogParams {
  since?: string
  until?: string
  offset?: number
  limit?: number
  order?: 'asc' | 'desc'
  keywords?: string[]
  dispatchCount?: number
  nodeNames?: string
}

export interface GetLogResponse {
  took: number
  hits: {
    total: {
      value: number
    }
    hits: LogHit[]
  }
}

export interface LogHit {
  _id: string
  _source: LogSource // Log content
}

export interface LogSource {
  '@timestamp': string
  stream: string
  line?: number // Negative for preceding lines, positive for following lines
  message: string
  kubernetes: {
    pod_name: string
    labels: Record<string, string>
    host: string
    container_name: string
  }
}

export interface LogTableRow {
  id: string
  timestamp: string
  message: string
  pod_name: string
  host: string
}
export interface LogTableResult {
  rows: LogTableRow[]
  total: number
  took: number
}

// download
interface DownloadInputs {
  name: string
  value: string
}
export interface DownloadParams {
  name: string
  inputs: DownloadInputs[]
  type: string
  timeoutSecond: number
}

// Root Cause Analysis
export interface RootCauseStep {
  step_name: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  started_at?: string
  completed_at?: string
  duration?: number
  progress: number
}

export interface RootCauseAnalysisResult {
  job_id: string
  workload_name: string
  problem_description: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  progress: number
  current_step: string
  created_at?: string
  started_at?: string
  completed_at?: string
  steps: RootCauseStep[]
  result?: {
    success: boolean
    workload_name: string
    problem_description: string
    report: string
    total_analysis_time: number
    task_times: Record<string, any>
  }
}

// Pending Cause Analysis
export interface PendingCauseStep {
  step_name: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  started_at?: string
  completed_at?: string
  duration?: number
  progress: number
}

export interface PendingCauseAnalysisResult {
  job_id: string
  workload_id: string
  problem_description: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  progress: number
  current_step: string
  created_at?: string
  started_at?: string
  completed_at?: string
  steps: PendingCauseStep[]
  result?: {
    success: boolean
    workload_id: string
    problem_description: string
    report: string
    total_analysis_time: number
    task_times: Record<string, any>
    task_outputs?: {
      event_collection?: string
      workload_collection?: string
      [key: string]: any
    }
    early_exit?: string
  }
}

export interface CreatePendingCauseJobRequest {
  workload_id: string
}

export interface CreatePendingCauseJobResponse {
  job_id: string
}
