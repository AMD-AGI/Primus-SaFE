import request from '@/services/request'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// Helper to get cluster and append to params
const withCluster = <T extends Record<string, any>>(params?: T): T & { cluster?: string } => {
  const { selectedCluster } = useGlobalCluster()
  const cluster = selectedCluster.value
  return {
    ...params,
    ...(cluster ? { cluster } : {}),
  } as T & { cluster?: string }
}

export interface JobExecutionHistory {
  id: number
  jobName: string
  jobType: string
  status: 'running' | 'success' | 'failed' | 'cancelled' | 'timeout'
  clusterName: string
  hostname: string
  startedAt: string
  finishedAt: string | null
  duration: number
  exitCode: number
  errorMessage: string | null
  metadata: Record<string, any>
  createdAt?: string
  updatedAt?: string
}

export interface JobExecutionHistoryListParams {
  pageNum?: number
  pageSize?: number
  jobName?: string
  jobType?: string
  status?: string
  clusterName?: string
  hostname?: string
  startTimeFrom?: string
  startTimeTo?: string
  minDuration?: number
  maxDuration?: number
  orderBy?: string
}

export interface JobExecutionHistoryListRes {
  data: JobExecutionHistory[]
  total: number
  pageNum: number
  pageSize: number
}

export interface JobStatistics {
  jobName: string
  totalExecutions: number
  successfulExecutions: number
  failedExecutions: number
  cancelledExecutions: number
  timeoutExecutions: number
  successRate: number
  averageDuration: number
  minDuration: number
  maxDuration: number
  medianDuration: number
  stdDuration: number
  lastExecutionAt: string
  lastSuccessAt: string
  lastFailureAt: string
  commonErrors: Array<{
    errorMessage: string
    count: number
  }>
}

// Get job execution history list
export function getJobExecutionHistories(params?: JobExecutionHistoryListParams) {
  return request.get<any, JobExecutionHistoryListRes>('/job-execution-histories', {
    params: withCluster(params)
  })
}

// Get single job execution history detail
export function getJobExecutionHistory(id: number) {
  return request.get<any, JobExecutionHistory>(`/job-execution-histories/${id}`, {
    params: withCluster()
  })
}

// Get recent failure records
export function getRecentFailures(limit?: number) {
  return request.get<any, JobExecutionHistory[]>('/job-execution-histories/recent-failures', {
    params: withCluster({ limit })
  })
}

// Get statistics for a specific job
export function getJobStatistics(jobName: string) {
  return request.get<any, JobStatistics>(`/job-execution-histories/statistics/${encodeURIComponent(jobName)}`, {
    params: withCluster()
  })
}