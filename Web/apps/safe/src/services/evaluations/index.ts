import request from '@/services/request'
import type {
  EvaluationTaskDetail,
  GetEvaluationTasksParams,
  GetEvaluationTasksResponse,
  AvailableService,
} from './type'

export const getEvaluationTasks = (
  params?: GetEvaluationTasksParams,
): Promise<GetEvaluationTasksResponse> => request.get('/evaluations/tasks', { params })

export const getEvaluationTaskDetail = (taskId: string): Promise<EvaluationTaskDetail> =>
  request.get(`/evaluations/tasks/${taskId}`)

export const deleteEvaluationTask = (taskId: string): Promise<{ message: string }> =>
  request.delete(`/evaluations/tasks/${taskId}`)

export const getAvailableServices = (params?: {
  workspace?: string
}): Promise<{ items: AvailableService[] }> =>
  request.get('/evaluations/available-services', { params })

export const getEvaluationReport = (taskId: string): Promise<any> =>
  request.get(`/evaluations/tasks/${taskId}/report`)

export const stopEvaluationTask = (taskId: string): Promise<{ message: string }> =>
  request.post(`/evaluations/tasks/${taskId}/stop`)
