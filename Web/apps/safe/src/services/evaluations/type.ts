export type EvaluationStatus = 'pending' | 'running' | 'completed' | 'failed'

export interface EvaluationBenchmark {
  datasetId: string
  datasetName: string
  limit?: number
}

export interface EvaluationTaskItem {
  taskId: string
  taskName: string
  description?: string
  serviceId?: string
  serviceType?: string
  serviceName?: string
  benchmarks?: EvaluationBenchmark[]
  status?: string
  progress?: number
  workspace?: string
  userId?: string
  userName?: string
  creationTime?: string
  startTime?: string
}

export interface GetEvaluationTasksParams {
  workspace?: string
  status?: EvaluationStatus
  serviceId?: string
  limit?: number
  offset?: number
}

export interface GetEvaluationTasksResponse {
  items: EvaluationTaskItem[]
  totalCount: number
}

export interface EvaluationResultSummary {
  overall_score?: number
  benchmarks?: Record<
    string,
    {
      accuracy?: number
      samples?: number
    }
  >
}

export interface EvaluationTaskDetail extends EvaluationTaskItem {
  evalParams?: Record<string, unknown>
  opsJobId?: string
  resultSummary?: EvaluationResultSummary
  reportS3Path?: string
  endTime?: string
  evaluationType?: string
  judgeServiceId?: string
  judgeServiceType?: string
  judgeServiceName?: string
  concurrency?: number
  timeout?: number
}

export interface AvailableService {
  serviceId: string
  serviceType: string
  displayName: string
  modelName: string
  status: string
  workspace: string
  endpoint: string
}
