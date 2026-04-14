import request from '@/services/request'
import type { RlConfigResponse, CreateRlJobRequest, CreateRlJobResponse } from './types'

export * from './types'

export function getRlConfig(modelId: string, params: { workspace: string; strategy?: string }) {
  return request.get<RlConfigResponse>(`/playground/models/${modelId}/rl-config`, { params })
}

export function createRlJob(data: CreateRlJobRequest) {
  return request.post<CreateRlJobResponse>('/rl/jobs', data, { timeout: 60000 })
}
