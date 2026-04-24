import request from '@/services/request'
import type {
  OptimizationTaskParams,
  CreateOptimizationTaskPayload,
  OptimizationTaskListResponse,
  OptimizationTask,
  ArtifactItem,
  ApplyPayload,
} from './type'

const BASE = '/optimization'

export const listOptimizationTasks = (params?: OptimizationTaskParams): Promise<OptimizationTaskListResponse> =>
  request.get(`${BASE}/tasks`, { params })

export const getOptimizationTask = (id: string): Promise<OptimizationTask> =>
  request.get(`${BASE}/tasks/${id}`)

export const createOptimizationTask = (data: CreateOptimizationTaskPayload): Promise<OptimizationTask> =>
  request.post(`${BASE}/tasks`, data)

export const batchCreateOptimizationTasks = (data: { tasks: CreateOptimizationTaskPayload[] }): Promise<any> =>
  request.post(`${BASE}/tasks/batch`, data)

export const deleteOptimizationTask = (id: string): Promise<any> =>
  request.delete(`${BASE}/tasks/${id}`)

export const interruptOptimizationTask = (id: string): Promise<any> =>
  request.post(`${BASE}/tasks/${id}/interrupt`)

export const retryOptimizationTask = (id: string): Promise<any> =>
  request.post(`${BASE}/tasks/${id}/retry`)

export const listOptimizationArtifacts = (id: string): Promise<ArtifactItem[]> =>
  request.get(`${BASE}/tasks/${id}/artifacts`)

export const downloadOptimizationArtifact = (id: string, path: string) => {
  const url = `${request.defaults.baseURL}${BASE}/tasks/${id}/artifacts/download?path=${encodeURIComponent(path)}`
  window.open(url, '_blank')
}

export const applyOptimizationTask = (id: string, data: ApplyPayload): Promise<any> =>
  request.post(`${BASE}/tasks/${id}/apply`, data)

/**
 * Subscribe to SSE events for a task.
 * Uses native EventSource (cookie-based auth, same-origin).
 */
export function subscribeTaskEvents(
  id: string,
  afterEventId?: string,
): EventSource {
  const baseURL = request.defaults.baseURL ?? ''
  let url = `${baseURL}${BASE}/tasks/${id}/events`
  if (afterEventId) url += `?after_event_id=${encodeURIComponent(afterEventId)}`
  return new EventSource(url, { withCredentials: true })
}
