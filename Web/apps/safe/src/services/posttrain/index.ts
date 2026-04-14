import request from '@/services/request'
import type {
  PostTrainRunItem,
  PostTrainListParams,
  PostTrainListResp,
  PostTrainMetricsResp,
} from './types'

export * from './types'

export function getPostTrainRuns(params?: PostTrainListParams) {
  return request.get<PostTrainListResp>('/posttrain/runs', { params })
}

export function getPostTrainRunDetail(id: string) {
  return request.get<PostTrainRunItem>(`/posttrain/runs/${id}`)
}

export function getPostTrainRunMetrics(id: string) {
  return request.get<PostTrainMetricsResp>(`/posttrain/runs/${id}/metrics`)
}

export function deletePostTrainRun(id: string) {
  return request.delete<void>(`/posttrain/runs/${id}`)
}
