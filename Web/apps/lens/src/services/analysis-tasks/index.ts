import request from '../request'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// ========== Helper ==========

const withCluster = <T extends Record<string, any>>(params?: T): T & { cluster?: string } => {
  const { selectedCluster } = useGlobalCluster()
  const cluster = selectedCluster.value
  return {
    ...params,
    ...(cluster ? { cluster } : {}),
  } as T & { cluster?: string }
}

// ========== Types ==========

export type AnalysisTaskType = 'github_proactive_analysis' | 'github_failure_analysis' | 'github_regression_analysis'
export type AnalysisTaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
export type AnalysisRiskLevel = 'high' | 'medium' | 'low'

export interface AnalysisFinding {
  file: string
  risk: AnalysisRiskLevel
  reason: string
}

export interface AnalysisResult {
  summary: string
  riskLevel?: AnalysisRiskLevel
  findingsCount?: number
  categories?: string[]
  reportUrl?: string
  details?: AnalysisFinding[]
}

export interface AnalysisError {
  code: string
  message: string
  retryCount?: number
}

export interface AnalysisTask {
  id: number
  type: AnalysisTaskType
  typeDisplay: string
  status: AnalysisTaskStatus
  statusDisplay: string
  createdAt: string
  startedAt?: string
  completedAt?: string
  durationMs?: number
  runId: number
  githubRunId: number
  commitSha?: string
  repoName?: string
  workflowName?: string
  branch?: string
  result?: AnalysisResult
  error?: AnalysisError
}

export interface AnalysisTaskSummary {
  total: number
  pending: number
  running: number
  completed: number
  failed: number
}

export interface AnalysisTasksResponse {
  runId: number
  githubRunId?: number
  workflowName?: string
  repoName?: string
  tasks: AnalysisTask[]
  summary: AnalysisTaskSummary
}

export interface AnalysisTaskListResponse {
  tasks: AnalysisTask[]
  total: number
  limit: number
  offset: number
}

// ========== API Functions ==========

/**
 * Get all analysis tasks for a specific workflow run
 */
export const getAnalysisTasksByRunId = (runId: number): Promise<AnalysisTasksResponse> =>
  request.get(`/github-workflow-metrics/runs/${runId}/analysis-tasks`, { params: withCluster() })

/**
 * Get a single analysis task by ID
 */
export const getAnalysisTaskById = (taskId: number): Promise<AnalysisTask> =>
  request.get(`/github-workflow-metrics/analysis-tasks/${taskId}`, { params: withCluster() })

/**
 * List all analysis tasks with optional filters
 */
export const listAnalysisTasks = (params?: {
  type?: AnalysisTaskType
  status?: AnalysisTaskStatus
  repoName?: string
  startTime?: string
  endTime?: string
  limit?: number
  offset?: number
}): Promise<AnalysisTaskListResponse> =>
  request.get('/github-workflow-metrics/analysis-tasks', { params: withCluster(params) })

/**
 * Retry a failed analysis task
 */
export const retryAnalysisTask = (taskId: number): Promise<AnalysisTask> =>
  request.post(`/github-workflow-metrics/analysis-tasks/${taskId}/retry`, null, { params: withCluster() })

// ========== Helper Functions ==========

/**
 * Get display icon for task type
 */
export const getTaskTypeIcon = (type: AnalysisTaskType): string => {
  switch (type) {
    case 'github_proactive_analysis':
      return '🔮'
    case 'github_failure_analysis':
      return '🔍'
    case 'github_regression_analysis':
      return '📈'
    default:
      return '📋'
  }
}

/**
 * Get display color class for status
 */
export const getStatusColor = (status: AnalysisTaskStatus): string => {
  switch (status) {
    case 'pending':
      return 'info'
    case 'running':
      return 'primary'
    case 'completed':
      return 'success'
    case 'failed':
      return 'danger'
    case 'cancelled':
      return 'info'
    default:
      return 'info'
  }
}

/**
 * Get display color class for risk level
 */
export const getRiskLevelColor = (riskLevel?: AnalysisRiskLevel): string => {
  switch (riskLevel) {
    case 'high':
      return 'danger'
    case 'medium':
      return 'warning'
    case 'low':
      return 'success'
    default:
      return 'info'
  }
}

/**
 * Format duration in milliseconds to human readable string
 */
export const formatDuration = (durationMs?: number): string => {
  if (!durationMs) return '-'
  
  const seconds = Math.floor(durationMs / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  
  if (hours > 0) {
    return `${hours}h ${minutes % 60}m ${seconds % 60}s`
  } else if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`
  } else {
    return `${seconds}s`
  }
}

/**
 * Check if there are any running or pending tasks
 */
export const hasActiveTasks = (tasks: AnalysisTask[]): boolean =>
  tasks.some(t => t.status === 'running' || t.status === 'pending')
